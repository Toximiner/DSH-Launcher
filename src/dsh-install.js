'use strict';
// Searching for and checking the dsh, npm and Node.js programs.
const { app } = require('electron');
const { execFile } = require('child_process');
const util = require('util');
const { promisify } = util;
const fs = require('fs');
const os = require('os');
const path = require('path');
const execFileAsync = promisify(execFile);
const state = require('./state');
const { firstErrLine } = require('./util');
const { envWithBinDir, withoutUnitEnv, writableOrCreatable } = require('./system');
const { DSH_BIN_DEFAULT, DSH_BIN_EXPLICIT } = require('./env');

/* ---------------- dsh installation check ---------------- */

// The environment for starting dsh: the binary's directory — first in PATH.
// dsh is a `#!/usr/bin/env node` script; installed via nvm, it must get
// node from the same directory — when started from the link, nvm is not
// in PATH. The dsh environment (and npm in its profile): without the
// systemd unit variables — see withoutUnitEnv in system.js.
function dshEnv(extra) {
  return withoutUnitEnv(envWithBinDir(state.dshBin, extra));
}



async function probeDsh(bin) {
  if (!bin || !fs.existsSync(bin)) return { ok: false, reason: 'missing' };
  try {
    const { stdout: out } = await execFileAsync(bin, ['--version'], { encoding: 'utf8', timeout: 8000, env: envWithBinDir(bin) });
    return { ok: true, version: (out.trim().split('\n').pop() || '').trim() };
  } catch (e) {
    const detail = firstErrLine(e.stderr || e.message);
    return { ok: false, reason: 'broken', detail };
  }
}

const dshBinPrefFile = () => path.join(app.getPath('userData'), 'dsh-bin.json');

// The bin directories of all node versions from nvm, the newest first.
function nvmBinDirs() {
  const nvmDir = process.env.NVM_DIR || path.join(os.homedir(), '.nvm');
  try {
    return fs.readdirSync(path.join(nvmDir, 'versions', 'node'))
      .sort((a, b) => b.localeCompare(a, undefined, { numeric: true }))
      .map((v) => path.join(nvmDir, 'versions', 'node', v, 'bin'));
  } catch { return []; } // no nvm
}

// An environment with the binary's directory first in PATH (for `#!/usr/bin/env node` scripts).


// An executable: next to dsh, in PATH, otherwise in nvm (when started from
// the link, nvm is not in PATH).
function findBin(name) {
  const dirs = [path.dirname(state.dshBin)]
    .concat(String(process.env.PATH || '').split(path.delimiter).filter(Boolean), nvmBinDirs());
  for (const dir of dirs) {
    const bin = path.join(dir, name);
    try { fs.accessSync(bin, fs.constants.X_OK); return bin; } catch { /* next */ }
  }
  return null;
}
const findNpm = () => findBin('npm');

// Where to look for dsh if DSH_BIN is not set: the remembered path,
// /usr/bin/dsh, PATH, typical user npm install directories, all nvm versions
// (the newest first). npm prefix -g — last, it is slow.
function dshCandidates() {
  const home = os.homedir();
  const list = [];
  try { list.push(JSON.parse(fs.readFileSync(dshBinPrefFile(), 'utf8')).bin); } catch { /* not remembered */ }
  list.push(DSH_BIN_DEFAULT);
  for (const dir of String(process.env.PATH || '').split(path.delimiter)) if (dir) list.push(path.join(dir, 'dsh'));
  list.push(path.join(home, '.npm-global', 'bin', 'dsh'), path.join(home, '.local', 'bin', 'dsh'), '/usr/local/bin/dsh');
  for (const dir of nvmBinDirs()) list.push(path.join(dir, 'dsh'));
  return [...new Set(list.filter(Boolean))];
}

// We check not only the presence of the file but that the binary really runs:
// a broken symlink, an incompatible Node or a missing npm give an error here,
// not a cryptic ENOENT at the moment of start. dsh --version takes ~60 ms.
// Asynchronously: on a slow system a synchronous call would freeze the window.
// If DSH_BIN is not set explicitly — we walk the candidates (dshCandidates)
// and switch DSH_BIN to the first working one, remembering it in userData/dsh-bin.json.
async function dshInstallCheck() {
  if (DSH_BIN_EXPLICIT) return probeDsh(state.dshBin);
  let firstBroken = null;
  const tryBin = async (bin) => {
    const r = await probeDsh(bin);
    if (r.ok) {
      if (bin !== state.dshBin) console.log(`[launcher] dsh найден: ${bin} (${r.version})`);
      state.dshBin = bin;
      try {
        fs.mkdirSync(path.dirname(dshBinPrefFile()), { recursive: true });
        fs.writeFileSync(dshBinPrefFile(), JSON.stringify({ bin }));
      } catch (e) { console.error('[launcher] dsh-bin.json:', e.message); }
    } else if (r.reason === 'broken' && !firstBroken) {
      firstBroken = { ...r, bin };
    }
    return r.ok ? r : null;
  };
  for (const bin of dshCandidates()) {
    const r = await tryBin(bin);
    if (r) return r;
  }
  const npm = findNpm();
  if (npm) {
    try {
      const prefix = (await execFileAsync(npm, ['prefix', '-g'], { encoding: 'utf8', timeout: 10000, env: envWithBinDir(npm) })).stdout.trim();
      const r = await tryBin(path.join(prefix, 'bin', 'dsh'));
      if (r) return r;
    } catch { /* npm does not respond */ }
  }
  if (firstBroken) {
    state.dshBin = firstBroken.bin; // we show on the page the one that is broken
    return { ok: false, reason: 'broken', detail: firstBroken.detail };
  }
  state.dshBin = DSH_BIN_DEFAULT;
  return { ok: false, reason: 'missing' };
}

// Whether npm is present and the global prefix is accessible without sudo —
// this determines whether dsh can be installed by a button right from the window.
// @latest — explicitly: the bare specifier of npm 10.x (Node 22) on a latest
// prerelease resolves as the range * and fails ETARGET (all dsh versions — prereleases).
const INSTALL_CMD = 'npm install -g @deepseek-ai/dsh@latest';
// The write permission is checked on the nearest existing directory: on a
// clean system <prefix>/lib/node_modules does not exist yet (it appears on the
// first global install), and an ENOENT from it earlier wrongly meant "no npm".


// dsh needs Node.js 22+. npm installs it even on an older Node without a
// single warning (Ubuntu 24.04: Node 18 from apt), and then dsh does not start.
const MIN_NODE_MAJOR = 22;

/* ===== rootInstallMode (test test/dsh-install.js) =====
   How to install globally when the prefix is owned by root: 'pkexec' — npm
   and the prefix are system ones (not in the home directory), the install
   can be run as root via pkexec — polkit will ask for the password in a
   system dialog, as when updating the launcher itself. 'manual' —
   everything else: installing npm from nvm or ~/.npm-global under sudo is
   not possible (root would create files in the home directory, and the
   user could not update them), and pkexec may not be present anyway. */
function rootInstallMode({ npm, prefix, hasPkexec, home }) {
  if (!npm || !prefix || !home) return 'manual';
  const inHome = (p) => p === home || p.startsWith(home + path.sep);
  if (inHome(npm) || inHome(prefix)) return 'manual';
  return hasPkexec ? 'pkexec' : 'manual';
}
/* ===== end rootInstallMode ===== */

async function nodeVersionNear(npm) {
  try {
    const { stdout } = await execFileAsync('node', ['--version'], { encoding: 'utf8', timeout: 5000, env: envWithBinDir(npm) });
    return stdout.trim();
  } catch { return null; }
}

/* ===== pkexecNpmArgs (test test/dsh-install.js) =====
   The pkexec arguments for installing an npm package as root. pkexec starts
   a program with a minimal environment: a PATH without the npm directory
   breaks `#!/usr/bin/env node` (npm is a JS script). So first the npm
   directory goes first in PATH, and the arguments are passed via "$@" — the
   package version is not glued into a shell string. Result: [pkexec, ...]. */
function pkexecNpmArgs(npm, npmArgs) {
  const script = 'PATH="$1:$PATH"; export PATH; shift; exec "$@"';
  return ['/bin/sh', '-c', script, 'sh', path.dirname(npm), npm, ...npmArgs];
}
/* ===== end pkexecNpmArgs ===== */

async function npmInstallInfo() {
  const npm = findNpm();
  if (!npm) return { hasNpm: false, canAuto: false, command: INSTALL_CMD };
  const nodeVersion = await nodeVersionNear(npm);
  const major = Number((/^v(\d+)/.exec(nodeVersion || '') || [])[1]);
  const nodeOld = Number.isFinite(major) && major < MIN_NODE_MAJOR;
  const pkexec = findBin('pkexec');
  try {
    const prefix = (await execFileAsync(npm, ['prefix', '-g'], { encoding: 'utf8', timeout: 10000, env: envWithBinDir(npm) })).stdout.trim();
    const writable = writableOrCreatable(path.join(prefix, 'lib', 'node_modules')) &&
      writableOrCreatable(path.join(prefix, 'bin'));
    // root's prefix: install via pkexec (system npm) or by hand (nvm and
    // other directories in the home folder).
    const rootMode = writable ? null : rootInstallMode({ npm, prefix, hasPkexec: Boolean(pkexec), home: os.homedir() });
    return { hasNpm: true, canAuto: writable && !nodeOld, nodeVersion, nodeOld, npm, prefix, pkexec, rootMode,
      command: writable ? INSTALL_CMD : 'sudo ' + INSTALL_CMD };
  } catch (e) {
    console.error('[launcher] npm prefix -g:', e.message);
    return { hasNpm: true, canAuto: false, nodeVersion, nodeOld, npm, pkexec, rootMode: 'manual', command: 'sudo ' + INSTALL_CMD };
  }
}

module.exports = { dshEnv, probeDsh, findBin, dshInstallCheck, MIN_NODE_MAJOR, rootInstallMode, pkexecNpmArgs, npmInstallInfo };
