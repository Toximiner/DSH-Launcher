'use strict';
// Размер окна между запусками.
const { app } = require('electron');
const fs = require('fs');
const path = require('path');

/* ---------------- размер окна между запусками ---------------- */

// Запоминаем размер и «развёрнуто» (позицию не трогаем: на Wayland её
// выбирает композитор). Полноэкранный режим не сохраняем.
const windowStateFile = () => path.join(app.getPath('userData'), 'window-state.json');

function loadWindowState() {
  try { return JSON.parse(fs.readFileSync(windowStateFile(), 'utf8')); } catch { return null; }
}



function trackWindowState(w) {
  // Последнее известное состояние — в памяти: при закрытии из страницы
  // (window.close()) Electron не шлёт 'close', и окно уже не спросить;
  // 'closed' же приходит всегда — тогда и пишем.
  let last = null;
  let timer = null;
  const snapshot = () => {
    if (w.isDestroyed() || w.isFullScreen()) return;
    const b = w.getNormalBounds(); // размер «не развёрнутого» окна
    last = { width: b.width, height: b.height, maximized: w.isMaximized() };
  };
  const write = () => {
    if (!last) return;
    try { fs.writeFileSync(windowStateFile(), JSON.stringify(last)); } catch { /* noop */ }
  };
  const later = () => { snapshot(); clearTimeout(timer); timer = setTimeout(write, 500); };
  w.once('ready-to-show', snapshot);
  w.once('show', snapshot); // Wayland: ready-to-show может не прийти — окно показывают по таймеру
  for (const ev of ['resize', 'maximize', 'unmaximize']) w.on(ev, later);
  // крестик / «Выйти» — самые свежие размеры; у спрятанного окна (трей, окно
  // «закрывается…») размеры не те — остаётся снимок, сделанный до того
  w.on('close', () => { if (w.isVisible()) snapshot(); });
  w.on('closed', () => { clearTimeout(timer); write(); });
  return snapshot; // снимок перед тем, как спрятать окно
}

module.exports = { loadWindowState, trackWindowState };
