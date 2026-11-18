'use strict';
// Shared Linux process identities and leases used by the desktop and terminal clients.
const fs = require('fs');
const os = require('os');
const path = require('path');
const crypto = require('crypto');

function identity(pid) {
  try {
    if (!Number.isSafeInteger(pid) || pid <= 1) return null;
    const stat = fs.readFileSync(`/proc/${pid}/stat`, 'utf8');
    const fields = stat.slice(stat.lastIndexOf(')') + 2).split(' ');
    if (fields[0] === 'Z' || fields[0] === 'X') return null;
    return { pid, start: fields[19], group: Number(fields[2]) };
  } catch { return null; }
}
function alive(value) {
  const current = identity(value?.pid);
  return !!current && current.start === value.start && current.group === value.group;
}
function backendDirectory(port) {
  const base = process.env.XDG_DATA_HOME || path.join(os.homedir(), '.local', 'share');
  return path.join(base, 'dsh-launcher', 'backends', String(port));
}
function read(file) {
  try { return JSON.parse(fs.readFileSync(file, 'utf8')); } catch { return null; }
}
function rememberBackend(port, pid, previousGroup = null) {
  const value = identity(pid);
  if (!value || value.group <= 1 || value.group === identity(process.pid)?.group) throw new Error('Backend process identity unavailable');
  const dir = backendDirectory(port);
  if (previousGroup && previousGroup !== value.group) {
    const previous = read(path.join(dir, 'managed.json'))?.previous;
    value.previous = [...(Array.isArray(previous) ? previous.filter(alive) : []), ...fs.readdirSync('/proc').filter((name) => /^\d+$/.test(name))
      .map((name) => identity(Number(name))).filter((item) => item?.group === previousGroup)];
  }
  fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
  const temporary = path.join(dir, `managed-${process.pid}.tmp`);
  fs.writeFileSync(temporary, JSON.stringify(value), { mode: 0o600 });
  fs.renameSync(temporary, path.join(dir, 'managed.json'));
}
function joinBackend(port) {
  const dir = backendDirectory(port);
  const clients = path.join(dir, 'clients');
  fs.mkdirSync(clients, { recursive: true, mode: 0o700 });
  const self = identity(process.pid);
  if (!self) throw new Error('Client process identity unavailable');
  const lease = path.join(clients, `${self.pid}-${self.start}-${crypto.randomUUID()}.json`);
  const temporary = lease + '.tmp';
  fs.writeFileSync(temporary, JSON.stringify(self), { mode: 0o600 });
  fs.renameSync(temporary, lease);
  let released = false;
  let last = false;
  return {
    release() {
      if (released) return last;
      released = true;
      fs.rmSync(lease, { force: true });
      // Remove our lease before scanning: simultaneous exits cannot both defer to one another.
      for (const name of fs.readdirSync(clients)) {
        if (!name.endsWith('.json')) continue;
        const file = path.join(clients, name);
        const value = read(file);
        if (!value || alive(value)) return false;
        fs.rmSync(file, { force: true });
      }
      last = true;
      return true;
    },
    target() {
      const value = read(path.join(dir, 'managed.json'));
      return alive(value) && value.group > 1 && value.group !== identity(process.pid)?.group ? value.group : null;
    },
    targets() {
      const value = read(path.join(dir, 'managed.json'));
      return [value, ...(Array.isArray(value?.previous) ? value.previous : [])]
        .filter((item) => alive(item) && item.group > 1 && item.group !== identity(process.pid)?.group)
        .map((item) => item.group);
    },
  };
}
module.exports = { identity, alive, backendDirectory, rememberBackend, joinBackend };
