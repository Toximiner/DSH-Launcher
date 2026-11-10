'use strict';
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawn } = require('node:child_process');
const { StringDecoder } = require('node:string_decoder');
const { stopProcessGroup } = require('./process');
const { InstallProgress } = require('./install-progress');

const DSH_VERSION = '0.2.2-rc.1';
const DSH_PACKAGE = `@deepseek-ai/dsh@${DSH_VERSION}`;

function supportsDsh(version) {
  const [major, minor] = version.split('.').map(Number);
  return major > 24 || (major === 24 && minor >= 9);
}

function findExecutable(command, cwd, env = process.env) {
  const candidates = command.includes('/') ? [path.resolve(cwd, command)]
    : (env.PATH || '').split(path.delimiter).map(dir => path.resolve(cwd, dir || '.', command));
  return candidates.find(file => {
    try { fs.accessSync(file, fs.constants.X_OK); return fs.statSync(file).isFile(); }
    catch { return false; }
  }) || null;
}

function installPrefix(env = process.env, home = os.homedir()) {
  const data = path.isAbsolute(env.XDG_DATA_HOME || '') ? env.XDG_DATA_HOME : path.join(home, '.local', 'share');
  return path.join(data, 'dsh-launcher-tui', 'backend');
}

// Confirmation is separate from installation: ordinary chat text never grants
// permission to run npm. Abort always removes the temporary input listener.
function confirmInstallation(terminal, signal) {
  if (signal.aborted) return Promise.resolve(false);
  return new Promise(resolve => {
    const finish = value => {
      terminal.removeListener('line', onLine);
      signal.removeEventListener('abort', onAbort);
      resolve(value);
    };
    const onAbort = () => finish(false);
    const onLine = line => {
      if (line === '/install') finish(true);
      else terminal.log('Use /install to install dsh, or /cancel to exit.');
    };
    terminal.on('line', onLine);
    signal.addEventListener('abort', onAbort, { once: true });
  });
}

class DshInstaller {
  constructor(prefix = installPrefix()) {
    this.prefix = prefix;
    this.bin = path.join(prefix, 'node_modules', '.bin', 'dsh');
    this.marker = path.join(prefix, '.installed.json');
    this.child = null;
    this.running = false;
  }

  managedBinary() {
    try {
      if (JSON.parse(fs.readFileSync(this.marker, 'utf8')).version === DSH_VERSION) {
        return findExecutable(this.bin, this.prefix);
      }
    } catch { /* Missing or incomplete installations must be confirmed again. */ }
    return null;
  }

  async ensure(config, terminal, signal) {
    if (!fs.statSync(config.cwd).isDirectory()) throw new Error('The workspace is not a directory');
    const existing = findExecutable(config.bin, config.cwd);
    if (existing) return existing;
    if (config.explicitBin) throw new Error(`Cannot start dsh (${config.bin}); check --dsh and --cwd`);
    const managed = this.managedBinary();
    if (managed) return managed;
    terminal.log(`dsh was not found. Install ${DSH_PACKAGE} for this user?\nLocation: ${this.prefix}\n/install — install and start · /cancel — exit`);
    while (!signal.aborted) {
      terminal.status = 'dsh is missing — /install or /cancel'; terminal.draw();
      if (!await confirmInstallation(terminal, signal)) return null;
      if (!supportsDsh(process.versions.node)) {
        terminal.log(`dsh requires Node.js 24.9+; this process uses ${process.versions.node}. Upgrade Node.js and restart the TUI.`);
        continue;
      }
      const npm = findExecutable('npm', process.cwd());
      if (!npm) { terminal.log('npm was not found. Install Node.js 24.9+ with npm, then restart the TUI.'); continue; }
      try {
        await this.install(npm, terminal, signal);
        if (signal.aborted) return null;
        terminal.log('dsh installed. Starting…');
        return this.bin;
      } catch (error) {
        if (signal.aborted) return null;
        terminal.log(`Installation failed: ${error.message}\n/install — try again · /cancel — exit`);
      }
    }
    return null;
  }

  async install(npm, terminal, signal) {
    if (this.running) throw new Error('Installation is already running');
    signal.throwIfAborted();
    fs.mkdirSync(this.prefix, { recursive: true, mode: 0o700 });
    fs.rmSync(this.marker, { force: true });
    const env = { ...process.env, PATH: path.dirname(process.execPath) + path.delimiter + (process.env.PATH || '') };
    delete env.DSH_LAUNCHER_GITHUB_TOKEN;
    delete env.DSH_TUI_URL;
    const child = this.child = spawn(npm, ['install', '--prefix', this.prefix, '--no-audit', '--no-fund', '--ignore-scripts', '--loglevel=http', DSH_PACKAGE], {
      cwd: this.prefix, env, detached: true, stdio: ['ignore', 'pipe', 'pipe'],
    });
    this.running = true;
    const activity = new InstallProgress();
    const progress = () => {
      terminal.status = activity.text();
      terminal.draw();
    };
    progress();
    const timer = setInterval(progress, 100);
    const flushers = [];
    for (const stream of [child.stdout, child.stderr]) {
      const decoder = new StringDecoder('utf8');
      let buffer = '';
      const output = line => {
        if (!line.trim()) return;
        if (!signal.aborted) activity.observe(line);
        terminal.log('[npm] ' + line.slice(-16000));
        progress();
      };
      const flush = () => { output(buffer); buffer = ''; };
      flushers.push(flush);
      stream.on('data', data => {
        buffer += decoder.write(data);
        let index;
        while ((index = buffer.indexOf('\n')) !== -1) {
          const line = buffer.slice(0, index); buffer = buffer.slice(index + 1);
          output(line);
        }
        if (buffer.length > 16000) flush();
      });
    }
    // Keep a single stop promise: signal handling and application shutdown may
    // both request cancellation while npm is still closing its output pipes.
    const onAbort = () => {
      activity.phase = 'Stopping installation'; progress();
      void this.stop().catch(error => terminal.log(error.message));
    };
    signal.addEventListener('abort', onAbort, { once: true });
    try {
      await new Promise((resolve, reject) => {
        child.once('error', error => reject(new Error(`Cannot run npm: ${error.message}`)));
        child.once('close', (code, exitSignal) => code === 0 ? resolve() : reject(new Error(`npm exited (${exitSignal || code})`)));
      });
      for (const flush of flushers) flush();
      signal.throwIfAborted();
      activity.phase = 'Checking installed dsh'; progress();
      if (!findExecutable(this.bin, this.prefix)) throw new Error('npm finished but the dsh executable is missing');
      fs.writeFileSync(this.marker, JSON.stringify({ version: DSH_VERSION }), { mode: 0o600 });
    } finally {
      signal.removeEventListener('abort', onAbort);
      clearInterval(timer);
      for (const flush of flushers) flush();
      this.child = null;
      this.running = false;
      this.stopping = null;
    }
  }

  stop() {
    if (!this.child) return Promise.resolve();
    if (!this.stopping) this.stopping = stopProcessGroup(this.child);
    return this.stopping;
  }
}

module.exports = { DshInstaller, DSH_PACKAGE, DSH_VERSION, supportsDsh, findExecutable, installPrefix, confirmInstallation };
