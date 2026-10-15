'use strict';
// Процессы и файлы: окружение запуска, права на запись, группы процессов,
// кто слушает порт, какие процессы останавливать.
const fs = require('fs');
const path = require('path');

function envWithBinDir(bin, extra) {
  return { ...process.env, PATH: path.dirname(bin) + path.delimiter + (process.env.PATH || ''), ...extra };
}

// Переменные, которые systemd задаёт главному процессу своей службы. Лаунчеру
// они достаются по наследству от сеанса рабочего стола (GNOME запускает
// приложения с INVOCATION_ID и JOURNAL_STREAM своей службы), но dsh — не
// служба systemd. dshmarket по ним и по родителю-systemd (после своего
// перезапуска dsh усыновляет systemd --user) решает, что перезапуском
// владеет systemd, и отвечает «self-restart is disabled for this host».
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

/* Ссылка входа из вывода dsh: последняя строка «dsh web: <url>?token=…» с тем
   же адресом, что plainUrl (другой порт или хост — не наша). Нет — null. */
function authUrlFromOutput(text, plainUrl) {
  const origin = new URL(plainUrl).origin;
  let found = null;
  for (const m of String(text).matchAll(/dsh web:\s*(https?:\/\/\S+)/g)) {
    try {
      const u = new URL(m[1]);
      if (u.origin === origin && u.searchParams.get('token')) found = m[1];
    } catch { /* обрывок строки */ }
  }
  return found;
}

// dsh, перезапущенный не лаунчером (кнопкой перезапуска маркета), печатает
// ссылку входа не нам, а в свой файл (/tmp/dsh-market-restart-*.out.log).
// Берём её из stdout процесса на порту: только обычный файл своего
// пользователя и только хвост. Не вышло — null (будет страница вставки).
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

/* portPid — процесс на порту (или null), portPgid — его группа (или null),
   selfPgid — группа лаунчера, oldPgid — группа исходного dsh (или null). */
function pickKillTargets(portPid, portPgid, selfPgid, oldPgid) {
  const targets = [];
  if (portPid) {
    if (portPgid && portPgid !== selfPgid) targets.push({ id: portPgid, groupOnly: true });
    else targets.push({ id: portPid, groupOnly: false }); // группа наша/неизвестна — только сам процесс
  }
  if (oldPgid && oldPgid !== selfPgid && !targets.some((t) => t.id === oldPgid)) {
    targets.push({ id: oldPgid, groupOnly: true });
  }
  return targets;
}

module.exports = { envWithBinDir, withoutUnitEnv, writableOrCreatable, groupAlive, portListenerPid, authUrlFromOutput, listenerAuthUrl, pgidOf, pickKillTargets };
