'use strict';
// The «Журнал dsh» window: the last lines of dsh.log, tokens hidden.
const fs = require('fs');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc, redactLog } = require('./util');
const { LOG_FILE } = require('./env');
const dialogs = require('./dialogs');

/* ---------------- dsh log ---------------- */

// «Справка → Журнал dsh»: the tail of dsh.log in a dialog window. The log contains
// the GUI login token («dsh web: …?token=…») — we replace it and anything
// resembling a key with *** on display and on copy.



/* ===== logTail (test test/dsh-log.js) =====
 * fs — the 'fs' module (replaced in the test; the functions openSync/fstatSync/
 * readSync/closeSync themselves are used as-is). */
// The last lines of the file (we read only the tail — the log can be large).
function logTail(file, maxLines = 300, maxBytes = 256 * 1024) {
  let fd;
  try {
    fd = fs.openSync(file, 'r');
    const size = fs.fstatSync(fd).size;
    const len = Math.min(size, maxBytes);
    const buf = Buffer.alloc(len);
    fs.readSync(fd, buf, 0, len, size - len);
    let text = buf.toString('utf8');
    if (len < size) text = text.slice(text.indexOf('\n') + 1); // the first line is a fragment
    return text.split('\n').slice(-maxLines).join('\n').replace(/\s+$/, '');
  } catch { return null; } finally { if (fd !== undefined) try { fs.closeSync(fd); } catch { /* noop */ } }
}
/* ===== end logTail ===== */

function showLogDialog() {
  const title = tr('Журнал dsh', 'dsh log');
  const gen = dialogs.openDialog(title);
  if (gen === null) return;
  const render = (copied) => {
    const raw = logTail(LOG_FILE);
    const text = raw === null ? null : redactLog(raw);
    state.dialogCopyText = text || '';
    const body = `
      <p style="margin:0 0 8px"><code>${esc(LOG_FILE)}</code> — ${tr('последние строки; токены и ключи скрыты (***)', 'latest lines; tokens and keys are hidden (***)')}</p>
      ${text === null
        ? `<p>${tr('Журнала ещё нет — dsh не запускался лаунчером.', 'No log yet — dsh has not been started by the launcher.')}</p>`
        : `<pre class="log">${esc(text || '…')}</pre>`}
      <div class="actions">
        <button onclick="location.href='dshlauncher://dialog/log-refresh/'">${tr('Обновить', 'Refresh')}</button>
        ${text ? '<button onclick="location.href=\'dshlauncher://dialog/copy/\'">' + (copied ? tr('Скопировано', 'Copied') : tr('Скопировать', 'Copy')) + '</button>' : ''}
        <button onclick="location.href='dshlauncher://dialog/logs/'">${tr('Папка логов', 'Logs folder')}</button>
        ${dialogs.dialogCloseButton()}
      </div>
      <script>const l = document.querySelector('pre.log'); if (l) l.scrollTop = l.scrollHeight;</script>`;
    dialogs.dialogPage(title, body);
  };
  dialogs.dialogShow(gen, render);
}

Object.assign(module.exports, { logTail, showLogDialog }); // we extend, not replace: the modules include each other in cycles
