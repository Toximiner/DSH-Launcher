'use strict';
// The window size between starts.
const { app } = require('electron');
const fs = require('fs');
const path = require('path');

/* ---------------- the window size between starts ---------------- */

// We remember the size and "maximized" (we don't touch the position: on
// Wayland the compositor chooses it). We don't save the full-screen mode.
const windowStateFile = () => path.join(app.getPath('userData'), 'window-state.json');

function loadWindowState() {
  try { return JSON.parse(fs.readFileSync(windowStateFile(), 'utf8')); } catch { return null; }
}



function trackWindowState(w) {
  // The last known state — in memory: on a close from the page
  // (window.close()) Electron doesn't send 'close', and the window can
  // no longer be asked; 'closed' comes always — that's when we write.
  let last = null;
  let timer = null;
  const snapshot = () => {
    if (w.isDestroyed() || w.isFullScreen()) return;
    const b = w.getNormalBounds(); // the "not maximized" window size
    last = { width: b.width, height: b.height, maximized: w.isMaximized() };
  };
  const write = () => {
    if (!last) return;
    try { fs.writeFileSync(windowStateFile(), JSON.stringify(last)); } catch { /* noop */ }
  };
  const later = () => { snapshot(); clearTimeout(timer); timer = setTimeout(write, 500); };
  w.once('ready-to-show', snapshot);
  w.once('show', snapshot); // Wayland: ready-to-show may not come — the window is shown by a timer
  for (const ev of ['resize', 'maximize', 'unmaximize']) w.on(ev, later);
  // the cross / «Выйти» — the freshest sizes; a hidden window (the tray,
  // the «закрывается…» window) has the wrong sizes — the snapshot made
  // before that stays
  w.on('close', () => { if (w.isVisible()) snapshot(); });
  w.on('closed', () => { clearTimeout(timer); write(); });
  return snapshot; // the snapshot before the window is hidden
}

module.exports = { loadWindowState, trackWindowState };
