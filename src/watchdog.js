'use strict';
// Авто-восстановление окна после перезапуска dsh.
/* ---------------- авто-восстановление после перезапуска dsh ----------------
 * GUI сам не возобновляет текущую сессию, когда процесс dsh перезапускается:
 * окно застревает на «сессия завершена». Но всё нужное для возобновления
 * сохраняется на диске: сессионный куки (секрет подписи живёт в
 * ~/.dsh/.credentials.yaml), сами сессии (~/.dsh/sessions) и состояние
 * интерфейса (localStorage). Поэтому при переходе порта down→up достаточно
 * перезагрузить страницу — приложение восстановится само.
 *
 * Но порт dsh открывается РАНЬШЕ, чем приложение готово (бут плагинов,
 * сессионная подсистема, подключение bili). Перезагрузка окна в этот момент
 * — «вслепую» против бутащегося сервера — не восстанавливает сессию и
 * оставляет серое (пустое) окно; пользователь вынужден перезапускать dsh
 * руками. Поэтому при down→up сначала не перезагружаем, а ждём, пока
 * HTTP-сервер dsh действительно ОТВЕЧАЕТ (plainUrlResponds — любой статус:
 * 200 — сессионный куки валидны; 401 — dsh поднялся, но токен сменился,
 * и ссылку входа openRunningGui возьмёт из вывода процесса на порту), и
 * только тогда подключаемся тем же путём, что «dsh запущен вручную».
 * Не отвечал за READY_WAIT_MS — показываем страницу «dsh не готов» с
 * кнопками «Проверить снова» и «Перезапустить dsh» (вместо серого окна).
 */
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { PORT } = require('./env');
const { portOpen } = require('./port');
const { showPage, showStatus } = require('./pages');
const { openRunningGui, plainUrlResponds } = require('./routes');

let watchdogStarted = false;
let lastAutoReloadAt = 0;
let recovering = false; // восстановление окна уже идёт — не дублируем
// Сколько ждать готовности dsh после открытия порта: бут после обновления
// плагинов (загрузка, подключение bili) может занять несколько секунд.
const READY_WAIT_MS = 15000;

/* ===== waitForReady (тест test/watchdog.js) =====
   Порт dsh открывается раньше, чем приложение готово. Ждём, пока checkReady()
   вернёт true (dsh готов), до таймаута или остановки лаунчера. true — готов;
   false — так и не стал (таймаут или stop). checkReady/isStopping —
   инжектимые; pollMs — интервал опроса (тест — маленький). Исключение из
   checkReady (сеть ещё не отвечает) — «не готов», не пробрасывается. */
const waitReadySleep = (ms) => new Promise((r) => setTimeout(r, ms)); // только внутри блока — видит тест
async function waitForReady(checkReady, timeoutMs, isStopping, pollMs = 100) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    if (isStopping()) return false;
    let ok;
    try { ok = await checkReady(); } catch { /* сеть ещё не отвечает — не готов */ }
    if (ok) return true;
    if (Date.now() >= deadline) return false;
    await waitReadySleep(pollMs);
  }
}
/* ===== end waitForReady ===== */

// Страница «dsh не готов»: порт открыт, но приложение не отвечает (бут после
// обновления плагинов). Вместо серого окна — понятные действия: дождаться и
// «Проверить снова» (подключаемся, как «dsh запущен вручную») или перезапуск.
function showNotReady() {
  if (!state.win || state.win.isDestroyed()) return;
  const body = tr(
    `<p><code>dsh</code> запущен (порт ${PORT} открыт), но приложение пока не
       отвечает — возможно, оно ещё загружается (обновление плагинов).</p>
       <p>Дождитесь ~ минуты и нажмите «Проверить снова». Если не помогло —
       перезапустите dsh.</p>`,
    `<p><code>dsh</code> is running (port ${PORT} is open), but the app is not
       responding yet — it may still be loading (plugin update).</p>
       <p>Wait about a minute, then press “Check again”. If that does not help,
       restart dsh.</p>`);
  const buttons = `
    <button onclick="location.href='dshlauncher://retry/'">${tr('Проверить снова', 'Check again')}</button>
    <button onclick="location.href='dshlauncher://menu/restart-dsh/'"
            style="margin-left:10px;background:#2a3550">${tr('Перезапустить dsh', 'Restart dsh')}</button>`;
  showPage(() => showStatus(tr('dsh не готов', 'dsh is not ready'), body + buttons, true));
}

async function recoverWindow(force = false) {
  if (!state.win || state.win.isDestroyed() || state.stopping) return;
  if (recovering) return; // восстановление уже идёт (ждёт готовности)
  const now = Date.now();
  if (!force && now - lastAutoReloadAt < 3000) return; // защита от флаттера порта
  lastAutoReloadAt = now;
  recovering = true;
  try {
    const ready = await waitForReady(plainUrlResponds, READY_WAIT_MS, () => state.stopping);
    if (state.stopping) return;
    if (!ready) {
      console.log(`[launcher] recover: dsh не готов за ${READY_WAIT_MS} мс — страница «не готов»`);
      showNotReady();
      return;
    }
    // Готово — подключаемся тем же путём, что «dsh запущен вручную»
    // (куки валидны → GUI; иначе ссылка входа / страница вставки токена).
    await openRunningGui('dsh снова на порту');
  } catch (e) {
    console.error('[launcher] recover:', e.message);
  } finally {
    recovering = false;
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
