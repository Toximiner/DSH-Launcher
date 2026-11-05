'use strict';
// Auto-recovery of the window after a dsh restart.
/* ---------------- auto-recovery after a dsh restart ----------------
 * The GUI doesn't resume the current session itself when the dsh process
 * restarts: the window sticks on "session ended". But everything needed
 * for the resumption is saved on disk: the session cookie (the signing
 * secret lives in ~/.dsh/.credentials.yaml), the sessions themselves
 * (~/.dsh/sessions) and the UI state (localStorage). So on a port
 * down→up transition it is enough to reload the page — the app will
 * recover itself.
 *
 * But the dsh port opens EARLIER than the app is ready (the plugin boot,
 * the session subsystem, the bili connection). Reloading the window at
 * that moment — "blindly" against a booting server — doesn't resume the
 * session and leaves a grey (empty) window; the user has to restart dsh
 * by hand. So on down→up we first don't reload but wait until the dsh
 * HTTP server really RESPONDS (plainUrlResponds — any status: 200 — the
 * session cookie is valid; 401 — dsh is up but the token changed, and
 * the login link openRunningGui takes from the process output on the
 * port), and only then connect by the same path as "dsh started
 * manually". It didn't answer within READY_WAIT_MS — we show the "dsh
 * is not ready" page with the «Проверить снова» and «Перезапустить dsh»
 * buttons (instead of the grey window).
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
let recovering = false; // the window recovery is already in progress — don't duplicate it
// How long to wait for dsh readiness after the port opens: the boot after
// a plugin update (loading, the bili connection) can take a few seconds.
const READY_WAIT_MS = 15000;

/* ===== waitForReady (test test/watchdog.js) =====
   The dsh port opens earlier than the app is ready. We wait until
   checkReady() returns true (dsh is ready), up to a timeout or the
   launcher stop. true — ready; false — it never became (a timeout or
   stop). checkReady/isStopping — injectable; pollMs — the polling
   interval (the test — small). An exception from checkReady (the network
   doesn't answer yet) — "not ready", not propagated. */
const waitReadySleep = (ms) => new Promise((r) => setTimeout(r, ms)); // only inside the block — the test sees it
async function waitForReady(checkReady, timeoutMs, isStopping, pollMs = 100) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    if (isStopping()) return false;
    let ok;
    try { ok = await checkReady(); } catch { /* the network doesn't answer yet — not ready */ }
    if (ok) return true;
    if (Date.now() >= deadline) return false;
    await waitReadySleep(pollMs);
  }
}
/* ===== end waitForReady ===== */

// The «dsh не готов» page: the port is open but the app doesn't answer
// (the boot after a plugin update). Instead of the grey window — clear
// actions: wait and «Проверить снова» (we connect as "dsh started
// manually") or a restart.
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
  if (recovering) return; // the recovery is already in progress (waiting for readiness)
  const now = Date.now();
  if (!force && now - lastAutoReloadAt < 3000) return; // protection against port flapping
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
    // Ready — we connect by the same path as "dsh started manually"
    // (the cookie is valid → GUI; otherwise the login link / the token
    // paste page).
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
    state.portState = (await portOpen()) ? 'up' : 'down'; // the base state, without firing
    setInterval(async () => {
      if (state.stopping) return;
      const now = (await portOpen()) ? 'up' : 'down';
      if (now === state.portState) return;
      if (now === 'up' && state.launching) {
        state.portState = now; // the launchDsh itself will bring the window to the GUI — don't interfere with a second navigation
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
