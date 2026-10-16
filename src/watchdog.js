'use strict';
// Авто-восстановление окна после перезапуска dsh.
/* ---------------- авто-восстановление после перезапуска dsh ----------------
 * GUI сам не возобновляет текущую сессию, когда процесс dsh перезапускается:
 * окно застревает на «сессия завершена». Но всё нужное для возобновления
 * сохраняется на диске: сессионный куки (секрет подписи живёт в
 * ~/.dsh/.credentials.yaml), сами сессии (~/.dsh/sessions) и состояние
 * интерфейса (localStorage). Поэтому при переходе порта down→up достаточно
 * перезагрузить страницу — приложение восстановится само.
 */
const state = require('./state');
const { PORT, PLAIN_URL } = require('./env');
const { portOpen } = require('./port');
const { loadInWin } = require('./pages');
const { openRunningGui } = require('./routes');

let watchdogStarted = false;
let lastAutoReloadAt = 0;

async function recoverWindow(force = false) {
  if (!state.win || state.win.isDestroyed() || state.stopping) return;
  const now = Date.now();
  if (!force && now - lastAutoReloadAt < 3000) return; // защита от флаттера порта
  lastAutoReloadAt = now;
  try {
    let cur = 'about:blank';
    try { cur = state.win.webContents.getURL(); } catch { return; }
    const dshOrigin = new URL(PLAIN_URL).origin;
    let onDshPage = false;
    try { onDshPage = new URL(cur).origin === dshOrigin; } catch { /* data: и т.п. */ }
    if (onDshPage) {
      await loadInWin(PLAIN_URL);
    } else {
      await openRunningGui('dsh снова на порту'); // окно на статус-странице
    }
  } catch (e) {
    console.error('[launcher] recover:', e.message);
  }
}

function startWatchdog() {
  if (watchdogStarted) return;
  watchdogStarted = true;
  (async () => {
    state.portState = (await portOpen()) ? 'up' : 'down'; // базовое состояние, без срабатываний
    setInterval(async () => {
      if (state.stopping) return;
      const now = (await portOpen()) ? 'up' : 'down';
      if (now === state.portState) return;
      if (now === 'up' && state.launching) {
        state.portState = now; // окно доведёт до GUI сам launchDsh — не мешаем ему второй навигацией
        return;
      }
      if (now === 'up') {
        console.log(`[launcher] dsh снова отвечает на порту ${PORT} — обновляю окно`);
        state.portState = now;
        recoverWindow();
      } else {
        console.log(`[launcher] порт ${PORT} закрыт — если dsh перезапустят, окно восстановится само`);
        state.portState = now;
      }
    }, 1500);
  })();
}

module.exports = { recoverWindow, startWatchdog };
