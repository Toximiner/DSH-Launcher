'use strict';

/*
 * dsh-launcher — a window that manages the DeepSeek Harness process.
 *
 *  - if port 3080 is not listening, starts `dsh --profile web --no-open`;
 *  - catches the `dsh web: http://127.0.0.1:3080/?token=...` line in dsh's
 *    stdout and opens it in the window — that issues the session cookie,
 *    remembered in the app profile;
 *  - if dsh is already running (e.g. from a terminal), attaches to it and
 *    does NOT kill it on close; if there is no valid cookie, asks to paste
 *    the URL with the token from the terminal where dsh is running;
 *  - closing the window terminates the whole dsh process tree started by the
 *    launcher (and dsh "adopted" after self-revival — it is found by port):
 *    SIGTERM first, SIGKILL after 4 s;
 *  - SIGTERM/SIGINT/SIGHUP to the launcher itself: a graceful close (same as
 *    closing the window); a repeated signal or a hang longer than 8 s —
 *    SIGKILL to the dsh tree (a direct SIGKILL to the launcher is uncatchable).
 *
 * Module split: src/gpu.js — GPU/EGL/ozone and the «GPU broken» marker;
 * src/port.js — dsh's port and its log; src/routes.js — the dshlauncher://
 * scheme and handlers; src/watchdog.js — window auto-recovery; src/signals.js
 * — signals and hard stop of dsh.
 */

const { app, BrowserWindow, crashReporter, protocol, screen } = require('electron');
const fs = require('fs');

const state = require('./src/state'); // shared state (window, dsh process, flags)
const i18n = require('./src/i18n');
const { tr } = i18n;
const { guardNavigation, restrictPermissions, denyExternalOpen } = require('./src/guard');
const { WIN_MIN, keyAction, isAltKey, windowSize } = require('./src/ui');

// Launch parameters and the launcher log — first: env.js removes the CI
// token from the environment, log.js starts writing launcher.log.
const { CONFIG, DSH_BIN_DEFAULT, PORT, PLAIN_URL, LANG_FORCED } = require('./src/env');
const { startLauncherLog } = require('./src/log');
startLauncherLog();

const gpu = require('./src/gpu'); // GPU/EGL/marker — detection at load, decisions before ready
const { portOpen } = require('./src/port');
const { openRunningGui, plainUrlIsAuthed, registerProtocol } = require('./src/routes');
const { recoverWindow, startWatchdog } = require('./src/watchdog');
const { killDshNow, installSignalHandlers } = require('./src/signals');
const { launchDsh } = require('./src/launch');
const { zoomFile, viewActions, initSpellcheck } = require('./src/view');
const { buildAppMenu, showContextMenu } = require('./src/menus');
const { startUpdateChecks, restartLauncher } = require('./src/updates');
const { showPage, loadLangPref, showStatus } = require('./src/pages');
const { setUpdatePolling } = require('./src/updates-live');
const { spawnNewInstance } = require('./src/settings-window');
const { dshInstallCheck } = require('./src/dsh-install');
const { showInstallPage } = require('./src/install-page');
const { stopDsh, stopAdoptedDsh } = require('./src/dsh-stop');
const { checkPreviousRun, clearRunMarker, maybeShowCrashNotice } = require('./src/diagnostics');
const { showClosingWindow, CLOSING_DELAY_MS } = require('./src/closing-window');
const { closeToTrayOn, setTray, setTrayAttention, clearAgentNotification, showMainWindow, onMainWindowClose, AGENT_POLL_MS, pollAgentState } = require('./src/tray');
const { inputHistoryOn, inputHistoryScript } = require('./src/input-history'); // ↑/↓: input history in the dsh field
const { maybeShowWhatsNew } = require('./src/whats-new');
const { maybeShowDshCrashNotice } = require('./src/dsh-process');
const { loadWindowState, trackWindowState } = require('./src/window-state');
// ---- launcher registry: names needed by the modules (generated) ----
// The registry keeps only names from modules that require electron (and
// have require cycles): pure modules (port.js, gpu.js, respond.js) are
// included by consumers via direct require. The getters resolve at call
// time — cycles are no obstacle.
const launcher = require('./src/launcher');
Object.defineProperties(launcher, {
  plainUrlIsAuthed: { get: () => plainUrlIsAuthed, enumerable: true },
  recoverWindow: { get: () => recoverWindow, enumerable: true },
  startWatchdog: { get: () => startWatchdog, enumerable: true },
});
// ---- end registry ----

// the path to dsh is set explicitly (the DSH_BIN variable or «Настройки»);
// otherwise, if /usr/bin/dsh is absent — it is looked up in PATH, npm
// prefix, nvm, etc. (see findDsh), and the found path is remembered
state.dshBin = CONFIG.dshBin || DSH_BIN_DEFAULT;

i18n.setLang(LANG_FORCED || i18n.systemLang());

/* ============================================================== */

app.setName('dsh-launcher');
// Electron crash dumps (minidump) — local only, in userData/Crashpad,
// nowhere sent. The fresh ones are put into «Справка → Сохранить отчёт…»:
// they let you analyze a crash (segfault) that the log shows only as a code.
crashReporter.start({ uploadToServer: false });

// Session diagnostics to the log — the first thing to look at if "the window does not appear".
if (process.platform === 'linux') {
  console.log(
    `[launcher] сессия: XDG_SESSION_TYPE=${process.env.XDG_SESSION_TYPE || '(пусто)'} ` +
    `WAYLAND_DISPLAY=${process.env.WAYLAND_DISPLAY || '(пусто)'} DISPLAY=${process.env.DISPLAY || '(пусто)'}`
  );
}

// ozone-platform / disableHardwareAcceleration — before app ready (see src/gpu.js)
gpu.apply(app);
// Two or more GPU process deaths per session = the driver is broken: the marker
gpu.onChildProcessGone(app, restartLauncher);

// Initial state values — the defaults from src/state.js (nobody changes the
// fields before onReady; dshBin was assigned above).

/* ---------------- main flow ---------------- */

protocol.registerSchemesAsPrivileged([
  { scheme: 'dshlauncher', privileges: { standard: false, secure: true } },
]);

let snapshotWindow = null; // a snapshot of the main window size (window-state.js)

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on('second-instance', () => showMainWindow()); // including one hidden to the tray
  app.whenReady().then(onReady);
}

async function onReady() {
  checkPreviousRun(); // the "single instance" lock is already ours
  installSignalHandlers(app);
  buildAppMenu(); // «Файл», «Правка», «Вид», «Справка» (the labels follow the
                  // window language, rebuilt on RU|EN)
  loadLangPref();
  // Update checks — in parallel with everything else; before starting dsh
  // we wait for them at most UPDATE_CHECK_TIMEOUT.
  startUpdateChecks();
  if (CONFIG.pollUpdates) setUpdatePolling(true);
  if (closeToTrayOn()) setTray(true);
  setInterval(() => { void pollAgentState(); }, AGENT_POLL_MS);
  restrictPermissions(); // before the first window
  registerProtocol(); // dshlauncher:// — the scheme and the route table (src/routes.js)

  const size = windowSize(loadWindowState(), screen.getPrimaryDisplay().workAreaSize);
  state.win = new BrowserWindow({
    width: size.width,
    height: size.height,
    minWidth: WIN_MIN.width,
    minHeight: WIN_MIN.height,
    title: 'DeepSeek Harness',
    backgroundColor: '#0f1115',
    autoHideMenuBar: false, // the menu bar is always visible
    show: false,
    // spellcheck is always on, and toggling is done via session.setSpellCheckerEnabled
    // (the «Вид» menu): that way it switches without a restart.
    // autoplayPolicy — the "agent finished" sound (WebAudio) without a click in the window
    webPreferences: { contextIsolation: true, nodeIntegration: false, spellcheck: true, autoplayPolicy: 'no-user-gesture-required' },
  });
  if (size.maximized) state.win.maximize();
  snapshotWindow = trackWindowState(state.win);
  state.win.on('close', onMainWindowClose);
  state.win.on('focus', () => { state.win.flashFrame(false); setTrayAttention(false); clearAgentNotification(); });
  initSpellcheck();
  denyExternalOpen(state.win.webContents);
  guardNavigation(state.win.webContents, { gui: true }); // foreign sites — in the browser
  // Right click — a context menu (copy / paste …): Electron has none of its
  // own. Both in the dsh GUI and on the launcher's service pages.
  state.win.webContents.on('context-menu', (_e, params) => showContextMenu(params));
  // diagnostics: the log must show that the window was shown and what loaded
  state.win.once('ready-to-show', () => { state.win.show(); console.log('[launcher] окно показано'); });
  let shownOnce = false;
  state.win.once('show', () => { shownOnce = true; });
  // Safety net: on native Wayland ready-to-show may arrive much later
  // (or not at all) — a window with show:false stays invisible.
  // If it was not shown even once within 8 s — we show it manually. Exactly "not
  // even once": a window that has already been hidden to the tray must not be restored.
  setTimeout(() => {
    // isDestroyed: the window may have been closed (and app.quit()) before the timer —
    // methods on a destroyed window throw.
    if (state.win && !state.win.isDestroyed() && !shownOnce && !state.win.isVisible()) {
      console.log('[launcher] ready-to-show не пришло за 8 с — показываю окно вручную');
      state.win.show();
    }
  }, 8000);
  // The zoom restored at startup (see setZoomLevel below) does not persist
  // after navigating to the dsh page — every cross-origin navigation in
  // Chromium resets zoom to 0. So we keep it here and carry it over to the
  // GUI once, on the first load of the dsh page, so "zoom between runs" works.
  let savedZoomPending = null;
  state.win.webContents.on('did-finish-load', () => {
    // first load of the dsh GUI — show "What's new" if the launcher was updated
    try {
      if (new URL(state.win.webContents.getURL()).origin === new URL(PLAIN_URL).origin) {
        // The window is on the dsh page, not a service one: a language change
        // must not repaint an old status page («Запуск dsh…») over the GUI.
        state.currentPage = null;
        if (savedZoomPending != null) {
          state.win.webContents.setZoomLevel(savedZoomPending);
          savedZoomPending = null;
        }
        void maybeShowWhatsNew();
        maybeShowCrashNotice();
        maybeShowDshCrashNotice();
        // ↑/↓ in the dsh field: injection of the input history (the texts are not written to the log)
        if (inputHistoryOn()) void state.win.webContents.executeJavaScript(inputHistoryScript()).catch(() => {});
      }
    } catch { /* data: */ }
    try { console.log('[launcher] страница загружена:', state.win.webContents.getURL().slice(0, 120)); } catch { /* noop */ }
  });
  state.win.webContents.on('did-fail-load', (_e, code, desc, url, isMainFrame) => {
    if (isMainFrame) console.error(`[launcher] ошибка загрузки: code=${code} ${desc} url=${String(url).slice(0, 120)}`);
  });
  state.win.webContents.on('render-process-gone', (_e, details) => {
    console.error('[launcher] рендер-процесс завершился:', details.reason, 'exitCode=' + details.exitCode);
    if (state.launcherUpdatedOnDisk) restartLauncher(); // new files on disk — the old one cannot come up
  });

  // "Browser-like" keys (F5, Ctrl+R, Ctrl+= / - / 0, Ctrl+F, F3, F11) —
  // keyAction → viewActions; the same actions in the «Вид» menu. Menu
  // accelerators are not registered — only hints, so as not to override the dsh GUI keys.
  try {
    const savedZoom = JSON.parse(fs.readFileSync(zoomFile(), 'utf8'));
    if (Number.isFinite(savedZoom.zoomLevel)) {
      const z = Math.max(-5, Math.min(5, savedZoom.zoomLevel));
      state.win.webContents.setZoomLevel(z);
      savedZoomPending = z; // we will carry it over to the dsh page (see did-finish-load above)
      console.log(`[launcher] восстановлен масштаб: ${savedZoom.zoomLevel}`);
    }
  } catch { /* noop */ }
  state.win.webContents.on('before-input-event', (event, input) => {
    try {
      // Alt (both keys, press and release): on Linux it moves
      // focus to the menu bar — on press it highlights «Файл», on
      // release it confirms the focus. We swallow by code (not by key:
      // Shift+Alt — layout switch — arrives as key="GroupNext");
      // see isAltKey in src/ui.js. The Alt+letter combinations and the dsh GUI are
      // untouched — for them the code is not AltLeft/AltRight.
      if (isAltKey(input)) { event.preventDefault(); return; }
      if (input.type !== 'keyDown') return;
      const act = keyAction(input);
      if (!act) return;
      event.preventDefault();
      viewActions[act]();
    } catch (e) { console.error('[launcher] ошибка в обработчике клавиш:', e); }
  });

  const alreadyRunning = await portOpen();
  if (alreadyRunning) {
    console.log('[launcher] port already open — attaching to running dsh');
    showPage(() => showStatus(tr('Подключение…', 'Connecting…'), tr(
      `<p><code>dsh</code> уже запущен на порту ${PORT}. Подключаюсь…</p>`,
      `<p><code>dsh</code> is already running on port ${PORT}. Connecting…</p>`)));
    await openRunningGui('подключение');
    startWatchdog();
    return;
  }

  // Port is closed — we will start dsh ourselves. First check that it is
  // installed at all: otherwise ENOENT after the start attempt, not a clear page.
  const installCheck = await dshInstallCheck();
  state.dshInstalledVersion = installCheck.version || null;
  if (!installCheck.ok) {
    console.log(
      `[launcher] dsh не найден (${installCheck.reason}` +
      (installCheck.detail ? ': ' + installCheck.detail : '') +
      ') — показываю страницу установки'
    );
    showInstallPage();
    startWatchdog(); // if dsh is installed and started manually — the window will come back to life on its own
    return;
  }
  await launchDsh();
}

/* ---------------- close = stop ---------------- */

app.on('window-all-closed', () => app.quit());

app.on('before-quit', (e) => {
  if (state.stopping) return;
  state.stopping = true;
  e.preventDefault();
  // Hide the window immediately, and if stopping dsh drags on — the "closing…" window
  if (state.win && !state.win.isDestroyed()) {
    if (state.win.isVisible() && snapshotWindow) snapshotWindow();
    state.win.hide();
  }
  const closingTimer = setTimeout(() => showClosingWindow(state.relaunchRequested), CLOSING_DELAY_MS);
  // dsh install / market / update step: its own process and redraw timer
  // (the cleanup functions are registered by the modules via state.settleOperation)
  state.settleAllOperations();
  Promise.resolve().then(async () => {
    await stopDsh();
    if (state.adoptedDsh) await stopAdoptedDsh();
  }).finally(() => {
    clearTimeout(closingTimer);
    if (state.relaunchRequested) { try { spawnNewInstance(); } catch (e) { console.error('[launcher] перезапуск:', e.message); } }
    app.quit();
  });
});

// safety net: if before-quit did not fire for some reason — hard stop
app.on('will-quit', () => { killDshNow(); clearRunMarker(); });
