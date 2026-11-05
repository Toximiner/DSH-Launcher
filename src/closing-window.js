'use strict';
// The «DeepSeek Harness закрывается…» window while dsh is stopped on exit.
// A normal stop — SIGTERM and up to 4 s of waiting (then SIGKILL): all this
// time the window is already hidden but the process is still alive — without
// the window, exit looks hung. We show it only if the stop drags on
// (CLOSING_DELAY_MS): a fast exit goes by without flicker. Closes with the app.
const { BrowserWindow } = require('electron');
const fs = require('fs');
const path = require('path');
const { tr } = require('./i18n');
const { esc, CSP_META } = require('./util');

const CLOSING_DELAY_MS = 300;

/* ===== closingHtml (test test/pages.js) =====
   The window markup: icon, spinner, heading and explanation. No scripts. */
function closingHtml(title, text, icon) {
  const img = icon ? `<img src="${esc(icon)}" alt="">` : '';
  const html = `<!doctype html><html><head><meta charset="utf-8">${CSP_META}<title>${esc(title)}</title><style>
    html,body{margin:0;height:100%;background:#0f1115;color:#d7dce3;overflow:hidden;
      font-family:system-ui,'Segoe UI',Roboto,Ubuntu,sans-serif;user-select:none}
    body{display:flex;align-items:center;gap:16px;padding:0 22px;box-sizing:border-box;
      border:1px solid #262d3a;border-radius:10px}
    img{width:48px;height:48px;flex:none}
    .spin{width:18px;height:18px;flex:none;border:2px solid #2b3445;border-top-color:#7aa2ff;
      border-radius:50%;animation:s 0.9s linear infinite}
    @keyframes s{to{transform:rotate(360deg)}}
    h1{font-size:15px;font-weight:600;margin:0 0 4px}
    p{font-size:13px;color:#9aa4b2;margin:0;line-height:1.4}
  </style></head><body>${img}<div><h1>${esc(title)}</h1><p>${esc(text)}</p></div><div class="spin"></div></body></html>`;
  return 'data:text/html;charset=utf-8,' + encodeURIComponent(html);
}
/* ===== end closingHtml ===== */

let win = null;

function iconDataUrl() {
  try { return 'data:image/png;base64,' + fs.readFileSync(path.join(__dirname, '..', 'icon.png')).toString('base64'); } catch { return ''; }
}

function showClosingWindow(relaunch = false) {
  if (win) return;
  const title = relaunch ? tr('DeepSeek Harness перезапускается…', 'DeepSeek Harness is restarting…')
    : tr('DeepSeek Harness закрывается…', 'DeepSeek Harness is closing…');
  const text = tr('Останавливаю dsh, подождите…', 'Stopping dsh, please wait…');
  try {
    win = new BrowserWindow({
      width: 420, height: 96, frame: false, resizable: false, movable: true, minimizable: false,
      maximizable: false, fullscreenable: false, center: true, show: false, alwaysOnTop: true,
      title, backgroundColor: '#0f1115',
      webPreferences: { contextIsolation: true, nodeIntegration: false },
    });
    win.webContents.on('will-navigate', (e) => e.preventDefault());
    win.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));
    win.once('ready-to-show', () => { if (win && !win.isDestroyed()) win.show(); });
    win.on('closed', () => { win = null; });
    win.loadURL(closingHtml(title, text, iconDataUrl())).catch(() => {});
    console.log('[launcher] окно «закрывается…» показано');
  } catch (e) {
    console.error('[launcher] окно «закрывается…»:', e.message);
  }
}

module.exports = { showClosingWindow, closingHtml, CLOSING_DELAY_MS };
