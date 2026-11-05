'use strict';
// Processes and files: the launch environment, write permissions, process
// groups, who is listening on the port, which processes to stop.
const fs = require('fs');
const path = require('path');

function envWithBinDir(bin, extra) {
  return { ...process.env, PATH: path.dirname(bin) + path.delimiter + (process.env.PATH || ''), ...extra };
}

// Variables that systemd sets on the main process of its service. The
// launcher inherits them from the desktop session (GNOME starts apps with
// the INVOCATION_ID and JOURNAL_STREAM of its service), but dsh is not a
// systemd service. dshmarket, by these and by a systemd parent (after its
// own restart dsh adopts systemd --user), concludes that systemd owns the
// restart and answers "self-restart is disabled for this host".
const SYSTEMD_UNIT_ENV = ['INVOCATION_ID', 'JOURNAL_STREAM', 'NOTIFY_SOCKET', 'SYSTEMD_EXEC_PID', 'MANAGERPID',
  'WATCHDOG_PID', 'WATCHDOG_USEC', 'LISTEN_PID', 'LISTEN_FDS', 'LISTEN_FDNAMES'];

function withoutUnitEnv(env) {
  const out = { ...env };
  for (const k of SYSTEMD_UNIT_ENV) delete out[k];
  return out;
}

function writableOrCreatable(dir) {
  while (!fs.existsSync(dir) && path.dirname(dir) !== dir) dir = path.dirname(dir);
  try { fs.accessSync(dir, fs.constants.W_OK); return true; } catch { return false; }
}

function groupAlive(pid, groupOnly = false) {
  try { process.kill(-pid, 0); return true; } catch { /* noop */ }
  if (groupOnly) return false;
  try { process.kill(pid, 0); return true; } catch { return false; }
}

function portListenerPid(port) {
  const hexPort = port.toString(16).toUpperCase().padStart(4, '0');
  const inodes = new Set();
  for (const file of ['/proc/net/tcp', '/proc/net/tcp6']) {
    let data;
    try { data = fs.readFileSync(file, 'utf8'); } catch { continue; }
    for (const line of data.split('\n').slice(1)) {
      const c = line.trim().split(/\s+/);
      // c[1]=local(IP:PORT), c[3]=state ('0A'=LISTEN), c[9]=inode
      if (c.length < 10 || c[3] !== '0A') continue;
      if (!c[1].endsWith(':' + hexPort)) continue;
      inodes.add(c[9]);
    }
  }
  if (inodes.size === 0) return null;
  let pids;
  try { pids = fs.readdirSync('/proc').filter((n) => /^\d+$/.test(n)); } catch { return null; }
  for (const pid of pids) {
    let fds;
    try { fds = fs.readdirSync(`/proc/${pid}/fd`); } catch { continue; }
    for (const fd of fds) {
      let link;
      try { link = fs.readlinkSync(`/proc/${pid}/fd/${fd}`); } catch { continue; }
      const m = /^socket:\[(\d+)\]$/.exec(link);
      if (m && inodes.has(m[1])) return Number(pid);
    }
  }
  return null;
}

/* The login link from dsh's output: the last «dsh web: <url>?token=…» line
   with the same address as plainUrl (another port or host — not ours).
   If absent — null. */
function authUrlFromOutput(text, plainUrl) {
  const origin = new URL(plainUrl).origin;
  let found = null;
  for (const m of String(text).matchAll(/dsh web:\s*(https?:\/\/\S+)/g)) {
    try {
      const u = new URL(m[1]);
      if (u.origin === origin && u.searchParams.get('token')) found = m[1];
    } catch { /* fragment of a line */ }
  }
  return found;
}

// A dsh restarted by anyone other than the launcher (the market restart
// button) prints the login link not to us but to its own file
// (/tmp/dsh-market-restart-*.out.log). We take it from the stdout of the
// process on the port: only a regular file of our own user, and only the
// tail. If it fails — null (the paste page will be shown).
const OUTPUT_TAIL = 64 * 1024;

function listenerAuthUrl(port, plainUrl) {
  const pid = portListenerPid(port);
  if (!pid) return null;
  let fd;
  try {
    fd = fs.openSync(`/proc/${pid}/fd/1`, 'r');
    const st = fs.fstatSync(fd);
    if (!st.isFile() || st.uid !== process.getuid()) return null;
    const len = Math.min(st.size, OUTPUT_TAIL);
    const buf = Buffer.alloc(len);
    fs.readSync(fd, buf, 0, len, st.size - len);
    return authUrlFromOutput(buf.toString('utf8'), plainUrl);
  } catch {
    return null;
  } finally {
    if (fd !== undefined) fs.closeSync(fd);
  }
}

function pgidOf(pid) {
  try {
    const stat = fs.readFileSync(`/proc/${pid}/stat`, 'utf8');
    const pgid = Number(stat.slice(stat.lastIndexOf(')') + 2).split(' ')[2]);
    return pgid > 0 ? pgid : null;
  } catch { return null; }
}

/* portPid — the process on the port (or null), portPgid — its group (or
   null), selfPgid — the launcher's group, oldPgid — the original dsh's
   group (or null). */
function pickKillTargets(portPid, portPgid, selfPgid, oldPgid) {
  const targets = [];
  if (portPid) {
    if (portPgid && portPgid !== selfPgid) targets.push({ id: portPgid, groupOnly: true });
    else targets.push({ id: portPid, groupOnly: false }); // the group is ours/unknown — the process itself only
  }
  if (oldPgid && oldPgid !== selfPgid && !targets.some((t) => t.id === oldPgid)) {
    targets.push({ id: oldPgid, groupOnly: true });
  }
  return targets;
}

module.exports = { SYSTEMD_UNIT_ENV, envWithBinDir, withoutUnitEnv, writableOrCreatable, groupAlive, portListenerPid, authUrlFromOutput, listenerAuthUrl, pgidOf, pickKillTargets };
