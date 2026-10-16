'use strict';

/*
 * dsh-launcher — окно, которое ведёт процесс DeepSeek Harness.
 *
 *  - если порт 3080 не слушается, запускает `dsh --profile web --no-open`;
 *  - строку `dsh web: http://127.0.0.1:3080/?token=...`, которую dsh печатает
 *    в stdout, подхватывает и открывает в окне — так выдаётся сессийный куки,
 *    который запоминается в профиле приложения;
 *  - если dsh уже запущен (например, из терминала), подключается к нему и НЕ
 *    убивает его при закрытии; если валидного куки нет, просит вставить URL
 *    с токеном из терминала, где работает dsh;
 *  - закрытие окна завершает всё дерево процессов dsh, которое запустил лаунчер
 *    (и dsh, «усыновлённый» после самовозрождения — он ищется по порту):
 *    сначала SIGTERM, через 4 с — SIGKILL;
 *  - SIGTERM/SIGINT/SIGHUP самому лаунчеру: штатное закрытие (как закрытие окна);
 *    повторный сигнал или зависание дольше 8 с — SIGKILL дереву dsh. Прямой
 *    SIGKILL лаунчеру перехватить нельзя — тот случай остаётся незакрытым.
 *
 * Разделение по модулям: src/gpu.js — GPU/EGL/ozone и маркер «GPU сломан»;
 * src/port.js — порт dsh и его журнал; src/routes.js — схема dshlauncher://
 * и хендлеры; src/watchdog.js — авто-восстановление окна; src/signals.js —
 * сигналы и жёсткая остановка dsh.
 */

const { app, BrowserWindow, crashReporter, protocol, screen, shell } = require('electron');
const fs = require('fs');

const state = require('./src/state'); // общее состояние (окно, процесс dsh, флаги)
const i18n = require('./src/i18n');
const { tr } = i18n;
const { guardNavigation, restrictPermissions } = require('./src/guard');
const { WIN_MIN, keyAction, isAltKey, windowSize } = require('./src/ui');

// Параметры запуска и журнал лаунчера — первыми: env.js убирает токен CI из
// окружения, log.js начинает писать launcher.log.
const { CONFIG, DSH_BIN_DEFAULT, PORT, PLAIN_URL, LANG_FORCED } = require('./src/env');
const { startLauncherLog } = require('./src/log');
startLauncherLog();

const gpu = require('./src/gpu'); // GPU/EGL/marker — детект при загрузке, решения до ready
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
const { inputHistoryOn, inputHistoryScript } = require('./src/input-history'); // ↑/↓: история введённого в поле dsh
const { maybeShowWhatsNew } = require('./src/whats-new');
const { maybeShowDshCrashNotice } = require('./src/dsh-process');
const { loadWindowState, trackWindowState } = require('./src/window-state');
// ---- реестр launcher: имена, нужные модулям (генерируется) ----
// В реестре остались только имена из модулей, подключающих electron (и с
// циклами require): чистые модули (port.js, gpu.js, respond.js) потребители
// подключают прямым require. Геттеры разрешаются в момент вызова — циклы
// не мешают.
const launcher = require('./src/launcher');
Object.defineProperties(launcher, {
  plainUrlIsAuthed: { get: () => plainUrlIsAuthed, enumerable: true },
  recoverWindow: { get: () => recoverWindow, enumerable: true },
  startWatchdog: { get: () => startWatchdog, enumerable: true },
});
// ---- конец реестра ----

// путь к dsh задан явно (переменная DSH_BIN или «Настройки»); иначе, если
// /usr/bin/dsh нет, — ищется в PATH, npm prefix, nvm и т.п. (см. findDsh),
// найденный путь запоминается
state.dshBin = CONFIG.dshBin || DSH_BIN_DEFAULT;

i18n.setLang(LANG_FORCED || (/^ru/i.test(process.env.LC_ALL || process.env.LC_MESSAGES || process.env.LANG || '') ? 'ru' : 'en'));

/* ============================================================== */

app.setName('dsh-launcher');
// Дампы падений Electron (minidump) — только локально, в userData/Crashpad,
// никуда не отправляются. Свежие кладутся в «Справка → Сохранить отчёт…»:
// по ним можно разобрать падение (segfault), которое в журнале — только код.
crashReporter.start({ uploadToServer: false });

// Диагностика сессии в лог — первое, что смотреть, если «окно не появляется».
if (process.platform === 'linux') {
  console.log(
    `[launcher] сессия: XDG_SESSION_TYPE=${process.env.XDG_SESSION_TYPE || '(пусто)'} ` +
    `WAYLAND_DISPLAY=${process.env.WAYLAND_DISPLAY || '(пусто)'} DISPLAY=${process.env.DISPLAY || '(пусто)'}`
  );
}

// ozone-platform / disableHardwareAcceleration — до app ready (см. src/gpu.js)
gpu.apply(app);
// Две и более смерти GPU-процесса за сессию = драйвер сломан: маркер
gpu.onChildProcessGone(app, restartLauncher);

state.win = null;
state.dshProc = null;
state.weStartedDsh = false;
state.adoptedDsh = false; // dsh «усыновлён» после самовозрождения — тоже останавливаем при закрытии
state.adoptedOldPgid = null; // группа исходного dsh до самовозрождения (могли остаться потомки)
state.dshFatal = false;
state.stopping = false;
state.authUrl = null; // URL с токеном, напечатанный dsh

/* ---------------- основной поток ---------------- */

protocol.registerSchemesAsPrivileged([
  { scheme: 'dshlauncher', privileges: { standard: false, secure: true } },
]);

let snapshotWindow = null; // снимок размеров главного окна (window-state.js)

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on('second-instance', () => showMainWindow()); // в т.ч. спрятанное в трей
  app.whenReady().then(onReady);
}

async function onReady() {
  checkPreviousRun(); // блокировка «один экземпляр» уже наша
  installSignalHandlers(app);
  buildAppMenu(); // «Файл», «Правка», «Вид», «Справка» (подписи следуют за
                  // языком окна, при RU|EN собирается заново)
  loadLangPref();
  // Проверки обновлений — параллельно всему остальному; перед стартом dsh
  // их ждём не дольше UPDATE_CHECK_TIMEOUT.
  startUpdateChecks();
  if (CONFIG.pollUpdates) setUpdatePolling(true);
  if (closeToTrayOn()) setTray(true);
  setInterval(() => { void pollAgentState(); }, AGENT_POLL_MS);
  restrictPermissions(); // до первого окна
  registerProtocol(); // dshlauncher:// — схема и таблица маршрутов (src/routes.js)

  const size = windowSize(loadWindowState(), screen.getPrimaryDisplay().workAreaSize);
  state.win = new BrowserWindow({
    width: size.width,
    height: size.height,
    minWidth: WIN_MIN.width,
    minHeight: WIN_MIN.height,
    title: 'DeepSeek Harness',
    backgroundColor: '#0f1115',
    autoHideMenuBar: false, // строка меню всегда видна
    show: false,
    // spellcheck включён всегда, а вкл/выкл — session.setSpellCheckerEnabled
    // (меню «Вид»): так переключается без перезапуска.
    // autoplayPolicy — звук «агент закончил» (WebAudio) без нажатия в окне
    webPreferences: { contextIsolation: true, nodeIntegration: false, spellcheck: true, autoplayPolicy: 'no-user-gesture-required' },
  });
  if (size.maximized) state.win.maximize();
  snapshotWindow = trackWindowState(state.win);
  state.win.on('close', onMainWindowClose);
  state.win.on('focus', () => { state.win.flashFrame(false); setTrayAttention(false); clearAgentNotification(); });
  initSpellcheck();
  state.win.webContents.setWindowOpenHandler(({ url }) => {
    if (/^https?:/i.test(url)) shell.openExternal(url);
    return { action: 'deny' };
  });
  guardNavigation(state.win.webContents, { gui: true }); // чужие сайты — в браузере
  // Правый клик — контекстное меню (копировать / вставить …): своего у
  // Electron нет. И в GUI dsh, и на служебных страницах лаунчера.
  state.win.webContents.on('context-menu', (_e, params) => showContextMenu(params));
  // диагностика: из лога должно быть видно, что окно показало и что загрузилось
  state.win.once('ready-to-show', () => { state.win.show(); console.log('[launcher] окно показано'); });
  let shownOnce = false;
  state.win.once('show', () => { shownOnce = true; });
  // Страховка: на нативном Wayland ready-to-show может прийти с большим
  // опозданием (или не прийти вовсе) — окно с show:false остаётся невидимым.
  // Если через 8 с оно ни разу не показано — показываем вручную. Именно «ни
  // разу»: окно, которое уже успели спрятать в трей, возвращать нельзя.
  setTimeout(() => {
    // isDestroyed: окно могли закрыть (и app.quit()) раньше таймера —
    // методы на уничтоженном окне бросают исключение.
    if (state.win && !state.win.isDestroyed() && !shownOnce && !state.win.isVisible()) {
      console.log('[launcher] ready-to-show не пришло за 8 с — показываю окно вручную');
      state.win.show();
    }
  }, 8000);
  state.win.webContents.on('did-finish-load', () => {
    // первая загрузка GUI dsh — показать «Что нового», если лаунчер обновился
    try {
      if (new URL(state.win.webContents.getURL()).origin === new URL(PLAIN_URL).origin) {
        void maybeShowWhatsNew();
        maybeShowCrashNotice();
        maybeShowDshCrashNotice();
        // ↑/↓ в поле dsh: впрыск истории введённого (тексты в журнал не пишутся)
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
    if (state.launcherUpdatedOnDisk) restartLauncher(); // новые файлы на диске — старому не подняться
  });

  // Клавиши «как в браузере» (F5, Ctrl+R, Ctrl+= / - / 0, Ctrl+F, F3, F11) —
  // keyAction → viewActions; в меню «Вид» те же действия. Ускорители меню
  // не регистрируются — только подсказки, чтобы не перебивать клавиши GUI dsh.
  try {
    const savedZoom = JSON.parse(fs.readFileSync(zoomFile(), 'utf8'));
    if (Number.isFinite(savedZoom.zoomLevel)) {
      state.win.webContents.setZoomLevel(Math.max(-5, Math.min(5, savedZoom.zoomLevel)));
      console.log(`[launcher] восстановлен масштаб: ${savedZoom.zoomLevel}`);
    }
  } catch { /* noop */ }
  state.win.webContents.on('before-input-event', (event, input) => {
    try {
      // Alt (обе клавиши, нажатие и отпускание): на Linux он переводит
      // фокус в строку меню — по нажатию подсвечивает «Файл», по
      // отпусканию подтверждает фокус. Гасим по code (не по key:
      // Shift+Alt — смена раскладки — приходит как key="GroupNext");
      // см. isAltKey в src/ui.js. Сочетания Alt+буква и GUI dsh не
      // затрагиваются — на них code не AltLeft/AltRight.
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

  // Порт закрыт — будем запускать dsh сами. Сначала проверяем, что он вообще
  // установлен: иначе вместо понятной страницы — ENOENT после попытки старта.
  const installCheck = await dshInstallCheck();
  state.dshInstalledVersion = installCheck.version || null;
  if (!installCheck.ok) {
    console.log(
      `[launcher] dsh не найден (${installCheck.reason}` +
      (installCheck.detail ? ': ' + installCheck.detail : '') +
      ') — показываю страницу установки'
    );
    showInstallPage();
    startWatchdog(); // если dsh поставят и запустят вручную — окно оживёт само
    return;
  }
  await launchDsh();
}

/* ---------------- закрытие = остановка ---------------- */

app.on('window-all-closed', () => app.quit());

app.on('before-quit', (e) => {
  if (state.stopping) return;
  state.stopping = true;
  e.preventDefault();
  // Окно прячем сразу, а если остановка dsh затянется — окошко «закрывается…»
  if (state.win && !state.win.isDestroyed()) {
    if (state.win.isVisible() && snapshotWindow) snapshotWindow();
    state.win.hide();
  }
  const closingTimer = setTimeout(() => showClosingWindow(state.relaunchRequested), CLOSING_DELAY_MS);
  // установка dsh / маркета / шаг обновления: свой процесс и таймер перерисовки
  // (функции очистки регистрируют модули через state.settleOperation)
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

// страховка: если before-quit почему-то не сработала — жёстко
app.on('will-quit', () => { killDshNow(); clearRunMarker(); });
