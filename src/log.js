'use strict';
// Logs: launcher.log (everything the launcher writes to the console — with the time,
// without the dsh lines, tokens hidden) and dsh.log (dsh's output); rotation after
// MAX_LOG_BYTES — at open time and while running.
const fs = require('fs');
const util = require('util');
const { LOG_DIR, LAUNCHER_LOG_FILE, MAX_LOG_BYTES } = require('./env');
const { redactLog } = require('./util');

// Rotation at open time: the file is bigger than MAX_LOG_BYTES — roll it to <file>.1.
function rotateLog(file) {
  try {
    const st = fs.statSync(file);
    if (st.size <= MAX_LOG_BYTES) return null;
    fs.renameSync(file, file + '.1');
    return st.size;
  } catch { return null; } // the file does not exist yet — nothing to rotate
}

// The launcher's log (launcher.log): everything the launcher prints to the console — with
// the time; when started from a shortcut the console is written nowhere. The "[dsh] …" lines
// are not duplicated — they are in dsh.log. Tokens are hidden (redactLog). We write synchronously:
// there are few lines, and on a crash the last ones are not lost.
function startLauncherLog() {
  const rotated = rotateLog(LAUNCHER_LOG_FILE);
  try { fs.mkdirSync(LOG_DIR, { recursive: true }); } catch { /* noop */ }
  let ok = true;
  let size = 0;
  try { size = fs.statSync(LAUNCHER_LOG_FILE).size; } catch { /* the new one does not exist yet */ }
  const write = (text) => {
    if (!ok) return;
    try {
      // and while running (the launcher may stay open for weeks) — before writing,
      // so that the current file is always
      const n = Buffer.byteLength(text);
      if (size > 0 && size + n > MAX_LOG_BYTES) { fs.renameSync(LAUNCHER_LOG_FILE, LAUNCHER_LOG_FILE + '.1'); size = 0; }
      fs.appendFileSync(LAUNCHER_LOG_FILE, text);
      size += n;
    } catch { ok = false; } // no access — the log works without the file, the launcher keeps working
  };
  for (const level of ['log', 'warn', 'error']) {
    const orig = console[level].bind(console);
    console[level] = (...args) => {
      orig(...args);
      const text = util.format(...args);
      if (text.startsWith('[dsh]')) return;
      const d = new Date();
      const t = new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().replace('T', ' ').slice(0, 23); // local time
      write(redactLog(text).split('\n').map((l) => `${t} ${l}`).join('\n') + '\n');
    };
  }
  let version = '?';
  try { version = require('../package.json').version; } catch { /* noop */ }
  write(`\n===== DeepSeek Harness Launcher ${version}, pid ${process.pid}, ${new Date().toISOString()} =====\n`);
  if (rotated) console.log(`[launcher] лог прокручен: launcher.log (${rotated} Б) -> launcher.log.1`);
}
// The dsh log with rotation while running too (dsh can be talkative, and the launcher
// is open for weeks): a line does not fit into MAX_LOG_BYTES — the file is rolled to
// <file>.1 and the line is written to the new one. Synchronously, via a descriptor: the stream's
// (createWriteStream) file is opened later, and after the rename the stream
// would write into the new file again.
function openDshLog(file) {
  let size = 0;
  try { size = fs.statSync(file).size; } catch { /* the file does not exist yet */ }
  let fd = null;
  try { fd = fs.openSync(file, 'a'); } catch (e) { console.error('[launcher] dsh.log:', e.message); }
  return {
    write(text) {
      if (fd === null) return;
      try {
        const n = Buffer.byteLength(text);
        if (size > 0 && size + n > MAX_LOG_BYTES) {
          fs.closeSync(fd);
          fd = null;
          fs.renameSync(file, file + '.1');
          fd = fs.openSync(file, 'a');
          size = 0;
        }
        fs.writeSync(fd, text);
        size += n;
      } catch (e) {
        console.error('[launcher] dsh.log:', e.message);
      }
    },
    end() {
      if (fd !== null) { try { fs.closeSync(fd); } catch { /* noop */ } fd = null; }
    },
  };
}

module.exports = { rotateLog, startLauncherLog, openDshLog };
