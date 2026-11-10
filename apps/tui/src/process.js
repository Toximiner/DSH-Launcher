'use strict';
const { spawn } = require('node:child_process');
const { StringDecoder } = require('node:string_decoder');

async function stopProcessGroup(child) {
  if (!child?.pid) return;
  const signal = name => { try { process.kill(-child.pid, name); } catch (error) { if (error.code !== 'ESRCH') throw error; } };
  signal('SIGTERM');
  const deadline = Date.now() + 4000;
  while (Date.now() < deadline) {
    try { process.kill(-child.pid, 0); } catch (error) { if (error.code === 'ESRCH') return; throw error; }
    await new Promise(resolve => setTimeout(resolve, 50));
  }
  signal('SIGKILL');
}

// Only children started here belong to this client; attached servers are never killed.
class DshProcess {
  constructor() { this.child = null; this.stopping = false; }

  start({ bin, cwd, profile, port }, timeout = 120000) {
    if (this.child) throw new Error('dsh is already starting');
    const env = { ...process.env, DSH_STARTED_BY: 'dsh-launcher-tui' };
    delete env.DSH_LAUNCHER_GITHUB_TOKEN;
    delete env.DSH_TUI_URL;
    const child = this.child = spawn(bin, ['--profile', profile, '--no-open', '--port', String(port)], {
      cwd, detached: true, stdio: ['ignore', 'pipe', 'pipe'], env,
    });
    return new Promise((resolve, reject) => {
      const decoder = new StringDecoder('utf8');
      let buffer = '';
      const timer = setTimeout(() => finish(new Error('Timed out waiting for dsh; check dsh configuration')), timeout);
      const finish = (error, url) => {
        clearTimeout(timer);
        if (error) reject(error); else resolve(url);
      };
      child.stdout.on('data', (data) => {
        buffer += decoder.write(data);
        let end;
        while ((end = buffer.indexOf('\n')) !== -1) {
          const line = buffer.slice(0, end);
          buffer = buffer.slice(end + 1);
          const match = /dsh web:\s*(https?:\/\/\S+)/.exec(line);
          if (match) {
            try {
              const url = new URL(match[1]);
              if (url.origin === `http://127.0.0.1:${port}` && url.searchParams.get('token')) finish(null, url.href);
            } catch { /* Other process output is not a login URL. */ }
          }
        }
        if (buffer.length > 65536) buffer = '';
      });
      // Drain stderr without leaking tokens or tool output into the terminal.
      child.stderr.on('data', () => {});
      child.once('error', () => finish(new Error(`Cannot start dsh (${bin}); check --dsh and --cwd`)));
      child.once('exit', (code, signal) => finish(new Error(`dsh exited (${signal || code})`)));
    });
  }

  async stop() {
    if (!this.child || this.stopping) return;
    this.stopping = true;
    const child = this.child;
    // The process group can outlive its original leader after a dsh self-restart.
    await stopProcessGroup(child);
  }
}

module.exports = { DshProcess, stopProcessGroup };
