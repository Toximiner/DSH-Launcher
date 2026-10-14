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
 */

const { app, BrowserWindow, crashReporter, protocol, net: electronNet, screen, shell, clipboard } = require('electron');
const { execFileSync } = require('child_process');
const nodeNet = require('net');
const fs = require('fs');
const path = require('path');

const state = require('./src/state'); // общее состояние (окно, процесс dsh, флаги)
const i18n = require('./src/i18n');
const { tr } = i18n;
const { esc, launcherRoute, stripRouteKey } = require('./src/util');
const { runWithLiveLog } = require('./src/proc-log');
const { guardNavigation, restrictPermissions } = require('./src/guard');
const { envWithBinDir } = require('./src/system');
const { WIN_MIN, keyAction, isAltKey, windowSize } = require('./src/ui');

// Параметры запуска и журнал лаунчера — первыми: env.js убирает токен CI из
// окружения, log.js начинает писать launcher.log.
const { CONFIG, DSH_BIN_DEFAULT, DSH_ARGS, HOST, PORT, PLAIN_URL, START_TIMEOUT, LOG_DIR, LANG_FORCED, ROUTE_KEY } = require('./src/env');
const { startLauncherLog } = require('./src/log');
startLauncherLog();
const { launchDsh } = require('./src/launch');
const { openLogsFolder, restartDsh, onDialogGpuEnable } = require('./src/file-help');
const { zoomFile, viewActions, onFindRequest, initSpellcheck } = require('./src/view');
const { buildAppMenu, showContextMenu } = require('./src/menus');
const { startUpdateChecks, updateState, onUpdateChoice, restartLauncher } = require('./src/updates');
const { marketState, onMarketChoice } = require('./src/market');
const { installState, refreshInstallState, showInstallPage } = require('./src/install-page');
const { showPage, loadLangPref, onLangSwitch, loadInWin, showStatus, showPastePage } = require('./src/pages');
const { setUpdatePolling, onToastRequest, showUpdateNowDialog, updateNow } = require('./src/updates-live');
const { onSettingsSet, onSettingsPick, onSettingsResetUpdates, relaunchLauncher, spawnNewInstance, SETTINGS_SECTIONS, showSettingsDialog } = require('./src/settings-window');
const { onDialogClose, onDialogCopy, showAboutDialog, showUpdateCheckDialog } = require('./src/dialogs');
const { maybeShowWhatsNew } = require('./src/whats-new');
const { showRollbackDialog, rollbackTo } = require('./src/rollback');
const { showLogDialog } = require('./src/dsh-log');
const { retryButton, maybeShowDshCrashNotice } = require('./src/dsh-process');
const { loadWindowState, trackWindowState } = require('./src/window-state');
const { dshInstallCheck } = require('./src/dsh-install');
const { killGroup, dshLeaderExited, stopDsh, adoptedTargets, stopAdoptedDsh } = require('./src/dsh-stop');
const { checkPreviousRun, clearRunMarker, maybeShowCrashNotice, saveReport } = require('./src/diagnostics');
const { closeToTrayOn, setTray, setTrayAttention, clearAgentNotification, showMainWindow, onMainWindowClose, AGENT_POLL_MS, pollAgentState } = require('./src/tray');
// ---- реестр launcher: имена, нужные модулям (генерируется) ----
const launcher = require('./src/launcher');
Object.defineProperties(launcher, {
  GPU_MARKER_FILE: { get: () => GPU_MARKER_FILE, enumerable: true },
  GPU_MARKER_TTL_MS: { get: () => GPU_MARKER_TTL_MS, enumerable: true },
  blankResponse: { get: () => blankResponse, enumerable: true },
  gpuDisabledByMarker: { get: () => gpuDisabledByMarker, enumerable: true },
  hasEglLibs: { get: () => hasEglLibs, enumerable: true },
  hasGpuRenderNode: { get: () => hasGpuRenderNode, enumerable: true },
  noContentResponse: { get: () => noContentResponse, enumerable: true },
  noGpuRequested: { get: () => noGpuRequested, enumerable: true },
  plainUrlIsAuthed: { get: () => plainUrlIsAuthed, enumerable: true },
  portOpen: { get: () => portOpen, enumerable: true },
  pushErr: { get: () => pushErr, enumerable: true },
  recoverWindow: { get: () => recoverWindow, enumerable: true },
  sleep: { get: () => sleep, enumerable: true },
  startWatchdog: { get: () => startWatchdog, enumerable: true },
  stderrTail: { get: () => stderrTail, enumerable: true },
  waitForAuthUrl: { get: () => waitForAuthUrl, enumerable: true },
  waitForPort: { get: () => waitForPort, enumerable: true },
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

// Есть ли в системе реальный GPU (render-узлы /dev/dri/renderD*).
// ВМ/VNC без 3D-ускорения их не имеют.
const hasGpuRenderNode = (() => {
  try { return fs.readdirSync('/dev/dri').some((f) => /^renderD/.test(f)); }
  catch { return false; }
})();

// Окно на Wayland предъявляется через EGL. Если у GPU виртуалки нет рабочего
// драйвера (ANGLE/EGL-ошибки в логе), окно «показано» по логам Electron, но на
// экране его не видно. X11/XWayland-путь деградирует до программного рендера —
// окно остаётся видимым. На X11-сессиях ничего не меняем.
// Если реальный GPU есть (renderD* в /dev/dri) — не форсим X11: нативный
// Wayland работает, а X11/XWayland-окно на KWin остаётся невидимым. Ручной
// переключатель: DSH_LAUNCHER_OZONE=x11|wayland.
const isWaylandSession = process.platform === 'linux' &&
  (String(process.env.XDG_SESSION_TYPE || '').toLowerCase() === 'wayland' ||
   Boolean(process.env.WAYLAND_DISPLAY));
if (isWaylandSession) {
  // Ошибку в значении DSH_LAUNCHER_OZONE не передаём в Electron:
  // неизвестный ozone-platform может не инициализироваться вовсе.
  const ozoneManual = CONFIG.ozone !== 'auto'; // DSH_LAUNCHER_OZONE или «Настройки»
  const ozone = ozoneManual ? CONFIG.ozone : hasGpuRenderNode ? 'wayland' : 'x11';
  app.commandLine.appendSwitch('ozone-platform', ozone);
  console.log(
    `[launcher] Wayland-сессия — ozone-platform=${ozone}` +
    (ozoneManual
      ? (CONFIG.src.ozone === 'env' ? ' (принудительно: DSH_LAUNCHER_OZONE)' : ' (выбрано в «Настройках»)')
      : hasGpuRenderNode
        ? ' (есть реальный GPU — нативный Wayland; X11/XWayland-окно на KWin невидимо)'
        : ' (render-узлов нет — окно веду через X11 (XWayland))')
  );
}

// ВМ/VNC без 3D-ускорения: render-узлов нет, EGL гарантированно не поднимется —
// идём сразу в программный рендер, минуя цикл падений GPU-процесса (ANGLE/EGL
// ошибки в логе). То же, если библиотек EGL в системе нет вовсе. Ручной
// переключатель: DSH_LAUNCHER_NO_GPU=1.
const hasEglLibs = (() => {
  try {
    const out = execFileSync('ldconfig', ['-p'], { encoding: 'utf8', timeout: 2000 });
    return /libEGL\.so\.1/.test(out) && /libGLESv2\.so\.[12]/.test(out);
  } catch { return true; } // не удалось проверить — решение не меняем
})();

// Маркер «GPU-драйвер сломан». Сломанный драйвер (SIGSEGV в GPU-процессе) на
// старте надёжно не определяется — но падение видно в рантайме. Запоминаем его
// в файл: следующие запуски сразу идут в программный рендер. Маркер живёт 30
// дней (на случай, если драйвер починили); чтобы вернуть GPU раньше — удалить
// файл.
const GPU_MARKER_FILE = path.join(LOG_DIR, 'gpu-disabled.marker');
const GPU_MARKER_TTL_MS = 30 * 24 * 60 * 60 * 1000;
const gpuMarkerAgeMs = (() => {
  try { return Date.now() - fs.statSync(GPU_MARKER_FILE).mtimeMs; }
  catch { return null; }
})();
const gpuDisabledByMarker = process.platform === 'linux' &&
  gpuMarkerAgeMs !== null && gpuMarkerAgeMs < GPU_MARKER_TTL_MS;

const noGpuRequested = CONFIG.noGpu; // DSH_LAUNCHER_NO_GPU=1 или «Настройки» → «Выключено»
if (process.platform === 'linux' && (noGpuRequested || gpuDisabledByMarker || !hasGpuRenderNode || !hasEglLibs)) {
  app.disableHardwareAcceleration();
  const why = noGpuRequested ? (CONFIG.src.gpu === 'env' ? 'DSH_LAUNCHER_NO_GPU=1' : '«Настройки»: GPU-ускорение — выключено')
    : gpuDisabledByMarker ? `маркер ${GPU_MARKER_FILE} (GPU-процесс падал, маркер живёт 30 дней; удалить файл, чтобы включить GPU)`
    : !hasGpuRenderNode ? 'render-узлов в /dev/dri нет'
    : 'EGL-библиотек (libEGL/libGLESv2) в системе нет';
  console.log(`[launcher] ${why} — отключаю GPU-ускорение (программный рендер)`);
}

// Две и более смерти GPU-процесса за сессию = драйвер сломан: пишем маркер,
// чтобы следующие запуски сразу обходили GPU.
let gpuCrashCount = 0;
app.on('child-process-gone', (_event, details) => {
  if (state.launcherUpdatedOnDisk) {
    console.log(`[launcher] процесс ${details.type} завершился после обновления на диске — перезапускаюсь`);
    restartLauncher();
    return;
  }
  if (details.type !== 'GPU' || (details.reason !== 'crashed' && details.reason !== 'abnormal-exit')) return;
  gpuCrashCount += 1;
  console.log(`[launcher] GPU-процесс завершился (reason=${details.reason}, code=${details.exitCode}, #${gpuCrashCount})`);
  if (gpuCrashCount === 2 && !gpuDisabledByMarker) {
    try {
      fs.mkdirSync(LOG_DIR, { recursive: true });
      fs.writeFileSync(GPU_MARKER_FILE, `GPU process crashed x2, reason=${details.reason}, ${new Date().toISOString()}\n`);
      console.log(`[launcher] GPU-драйвер, похоже, сломан — следующий запуск пойдёт в программном рендере (маркер ${GPU_MARKER_FILE})`);
    } catch (e) {
      console.log('[launcher] не удалось записать GPU-маркер:', e.message);
    }
  }
});

state.win = null;
state.dshProc = null;
state.weStartedDsh = false;
state.adoptedDsh = false; // dsh «усыновлён» после самовозрождения — тоже останавливаем при закрытии
state.adoptedOldPgid = null; // группа исходного dsh до самовозрождения (могли остаться потомки)
state.dshFatal = false;
state.stopping = false;
state.authUrl = null; // URL с токеном, напечатанный dsh
const stderrTail = [];

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function pushErr(line) {
  const t = line.trim();
  if (!t) return;
  stderrTail.push(t);
  if (stderrTail.length > 300) stderrTail.shift();
  console.error('[dsh]', t);
}

function portOpen() {
  return new Promise((resolve) => {
    const s = new nodeNet.Socket();
    const done = (ok) => {
      try { s.destroy(); } catch { /* noop */ }
      resolve(ok);
    };
    s.setTimeout(800);
    s.once('connect', () => done(true));
    s.once('timeout', () => done(false));
    s.once('error', () => done(false));
    s.connect(PORT, HOST);
  });
}

async function waitForPort() {
  const deadline = Date.now() + START_TIMEOUT;
  while (Date.now() < deadline) {
    if (await portOpen()) return true;
    if (state.dshProc && (state.dshProc.exitCode !== null || state.dshFatal)) return false;
    await sleep(400);
  }
  return portOpen();
}

async function waitForAuthUrl(timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  while (!state.authUrl && Date.now() < deadline) await sleep(100);
  return state.authUrl;
}

/* ---------------- основной поток ---------------- */

protocol.registerSchemesAsPrivileged([
  { scheme: 'dshlauncher', privileges: { standard: false, secure: true } },
]);

async function plainUrlIsAuthed() {
  try {
    const res = await electronNet.fetch(PLAIN_URL, { redirect: 'manual' });
    return res.status === 200;
  } catch {
    return false;
  }
}

/* ---------------- авто-восстановление после перезапуска dsh ----------------
 * GUI сам не возобновляет текущую сессию, когда процесс dsh перезапускается:
 * окно застревает на «сессия завершена». Но всё нужное для возобновления
 * сохраняется на диске: сессионный куки (секрет подписи живёт в
 * ~/.dsh/.credentials.yaml), сами сессии (~/.dsh/sessions) и состояние
 * интерфейса (localStorage). Поэтому при переходе порта down→up достаточно
 * перезагрузить страницу — приложение восстановится само.
 */
let watchdogStarted = false;
state.portState = null; // null | 'up' | 'down'
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
    } else if (await plainUrlIsAuthed()) {
      await loadInWin(PLAIN_URL); // окно на статус-странице, но куки валидны
    } else {
      showPastePage(); // куки не пережили перезапуск — нужен токен нового процесса
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

// Маршрут запроса dshlauncher://…: у нестандартной схемы первый сегмент
// (retry, install, paste, market) URL-парсер кладёт в host, а не в pathname —
// склеиваем обратно: dshlauncher://install/run/ -> '/install/run/'.


function onPasteRequest(route) {
  try {
    const pasted = decodeURIComponent(route.replace(/^\/paste\//, ''));
    if (/^https?:\/\//i.test(pasted) && state.win && !state.win.isDestroyed()) {
      console.log('[launcher] loading pasted URL');
      state.win.loadURL(pasted).catch((e) => console.error('[launcher] loadURL:', e.message));
    }
  } catch (e) {
    console.error('[launcher] paste handler:', e.message);
  }
  return blankResponse();
}

// «Проверить снова» на фатальной странице: если dsh запустили вручную
// (например, в терминале), а окно оставили открытым — возвращаемся в GUI.
async function onRetryRequest() {
  try {
    if (state.win && !state.win.isDestroyed() && !state.stopping) {
      const up = await portOpen();
      if (up) {
        state.weStartedDsh = false; // чужой dsh — при закрытии окна его не убиваем
        state.dshProc = null;
        state.dshFatal = false;
        state.portState = 'up';
        if (await plainUrlIsAuthed()) {
          console.log('[launcher] retry: dsh запущен, куки валидны — открываю GUI');
          await loadInWin(PLAIN_URL);
        } else {
          console.log('[launcher] retry: dsh запущен, валидного куки нет — страница вставки');
          showPastePage();
        }
      } else {
        // Бинарник мог исчезнуть (разустановка, другой DSH_BIN) — тогда
        // совет запускать его в терминале бессмысленен.
        if (!(await dshInstallCheck()).ok) {
          showInstallPage(() => tr('dsh не найден — установите его (команда на странице).',
            'dsh not found — install it (the command is on this page).'));
        } else {
          showPage(() => showStatus(tr('dsh не запущен', 'dsh is not running'), tr(
            `<p>Порт ${PORT} закрыт — <code>dsh</code> не запущен.</p>
               <p>Запустите его в терминале: <code>${esc(state.dshBin)} ${esc(DSH_ARGS.join(' '))}</code>
               и нажмите «Проверить снова». Либо закройте окно и запустите
               dsh-launcher снова.</p>`,
            `<p>Port ${PORT} is closed — <code>dsh</code> is not running.</p>
               <p>Start it in a terminal: <code>${esc(state.dshBin)} ${esc(DSH_ARGS.join(' '))}</code>
               and click “Check again”. Or close this window and start
               dsh-launcher again.</p>`) + retryButton(), false));
        }
      }
    }
  } catch (e) {
    console.error('[launcher] onRetryRequest:', e.message);
  }
  return blankResponse();
}

function blankResponse() {
  return new Response('<!doctype html><html><body style="background:#0f1115;margin:0"></body></html>',
    { headers: { 'content-type': 'text/html' } });
}

// «Нет содержимого»: навигация не происходит, страница остаётся как была.
function noContentResponse() {
  return new Response(null, { status: 204 });
}

// «Скопировать команду» на странице установки dsh.
async function onInstallCopy() {
  try {
    if (!installState.npmInfo) await refreshInstallState();
    clipboard.writeText(installState.npmInfo.command);
    console.log('[launcher] команда установки скопирована в буфер обмена');
    await showInstallPage(() => tr('Команда скопирована в буфер обмена — вставьте её в терминал.', 'Command copied to the clipboard — paste it into a terminal.'));
  } catch (e) {
    console.error('[launcher] onInstallCopy:', e.message);
    showInstallPage(() => tr('Не удалось скопировать команду: ' + e.message, 'Failed to copy the command: ' + e.message), true);
  }
  return blankResponse();
}

// «Проверить ещё раз»: dsh появился (поставили вручную или кнопкой) — продолжаем запуск.
async function onInstallRecheck() {
  try {
    if (installState.running) return blankResponse(); // установка идёт — страница обновится сама
    await refreshInstallState();
    if (installState.check.ok) {
      console.log(`[launcher] dsh найден (${installState.check.version}) — начинаю запуск`);
      await launchDsh();
      return blankResponse();
    }
    console.log(installState.check.reason === 'broken'
      ? `[launcher] повторная проверка: dsh найден, но не запускается (${installState.check.detail || ''})`
      : '[launcher] повторная проверка: dsh всё ещё не найден');
    showInstallPage(() => installState.check && installState.check.reason === 'broken'
      ? tr('dsh найден, но не запускается — причина ниже.', 'dsh was found but does not run — see the reason below.')
      : tr('dsh всё ещё не найден — установите его командой выше, затем проверьте снова.', 'dsh is still not found — install it with the command above, then check again.'), true);
  } catch (e) {
    console.error('[launcher] onInstallRecheck:', e.message);
    showInstallPage(() => tr('Ошибка проверки: ' + e.message, 'Check failed: ' + e.message), true);
  }
  return blankResponse();
}

// «Установить»: npm install -g прямо из окна, с живым логом.
async function onInstallRun() {
  try {
    if (installState.running || !state.win || state.win.isDestroyed() || state.stopping) return blankResponse();
    await refreshInstallState();
    if (installState.running) return blankResponse(); // пока проверяли, установку уже запустили
    const npmInfo = installState.npmInfo;
    if (!npmInfo.canAuto) {
      showInstallPage(() => tr('Кнопка «Установить» недоступна: глобальный npm prefix требует sudo — выполните команду в терминале.', '“Install” is unavailable: the global npm prefix requires sudo — run the command in a terminal.'), true);
      return blankResponse();
    }
    console.log(`[launcher] установка dsh из окна: ${npmInfo.command}`);
    const code = await runWithLiveLog(installState, npmInfo.npm, ['install', '-g', '@deepseek-ai/dsh'],
      { env: envWithBinDir(npmInfo.npm), redraw: () => showInstallPage() });
    if (code === 0) {
      console.log('[launcher] dsh установлен — продолжаю запуск');
      // Если prefix не /usr, бинарник лёг не в /usr/bin — находим и запоминаем фактический путь.
      const check = await dshInstallCheck();
      if (!check.ok) {
        showInstallPage(() => tr('npm сообщил об успехе, но dsh так и не запускается — см. детали ниже.',
          'npm reported success, but dsh still does not run — see details below.'), true);
        return blankResponse();
      }
      await launchDsh();
    } else {
      console.error(`[launcher] установка dsh завершилась с кодом ${code}`);
      showInstallPage(() => tr(`Установка завершилась с кодом ${code} (строки выше) — повторите в терминале.`, `Installation exited with code ${code} (see output above) — retry in a terminal.`), true);
    }
  } catch (e) {
    console.error('[launcher] onInstallRun:', e.message);
    showInstallPage(() => tr('Не удалось запустить установку: ' + e.message, 'Failed to start the installation: ' + e.message), true);
  }
  return blankResponse();
}

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on('second-instance', () => showMainWindow()); // в т.ч. спрятанное в трей
  app.whenReady().then(onReady);
}

async function onReady() {
  checkPreviousRun(); // блокировка «один экземпляр» уже наша
  installSignalHandlers();
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
  protocol.handle('dshlauncher', (request) => {
    let route;
    try {
      // Без ключа запуска — не со страницы лаунчера (GUI dsh, плагин, внешний
      // сайт в окне): ничего не делаем. В журнал — только путь, без параметров.
      route = stripRouteKey(launcherRoute(request.url), ROUTE_KEY);
      if (route === null) {
        console.error(`[launcher] dshlauncher: запрос без ключа запуска — отклонён: ${launcherRoute(request.url).slice(0, 80)}`);
        return noContentResponse();
      }
      if (route.startsWith('/retry/')) return onRetryRequest();
      if (route.startsWith('/lang/')) return onLangSwitch(route.split('/')[2]);
      if (route.startsWith('/install/copy/')) return onInstallCopy();
      if (route.startsWith('/install/run/')) return onInstallRun();
      if (route.startsWith('/install/recheck/')) return onInstallRecheck();
      if (route.startsWith('/market/install/')) return onMarketChoice('install');
      if (route.startsWith('/market/skip/')) return onMarketChoice('skip');
      if (route.startsWith('/market/never/')) return onMarketChoice('never');
      if (route.startsWith('/update/install/')) return onUpdateChoice('install');
      if (route.startsWith('/update/later/')) return onUpdateChoice('later');
      if (route.startsWith('/update/never/')) return onUpdateChoice('never');
      if (route.startsWith('/update/recheck/')) return onUpdateChoice('recheck');
      if (route.startsWith('/update/copy/')) return onUpdateChoice('copy');
      if (route.startsWith('/update/restart/')) return onUpdateChoice('restart');
      // О программе / проверка обновлений по URL — для Docker-сценариев
      // (driver.js не умеет кликать по нативному меню); штатно — пункт меню.
      // Ответ 204: страница, с которой перешли (GUI dsh), остаётся на месте.
      if (route.startsWith('/menu/about/')) { void showAboutDialog(); return noContentResponse(); }
      if (route.startsWith('/menu/check/')) { void showUpdateCheckDialog(); return noContentResponse(); }
      if (route.startsWith('/menu/rollback/')) { void showRollbackDialog(); return noContentResponse(); }
      if (route.startsWith('/rollback/')) { rollbackTo(new URL(request.url).searchParams.get('v')); return noContentResponse(); }
      if (route.startsWith('/menu/update/')) { showUpdateNowDialog(new URL(request.url).searchParams.get('k')); return noContentResponse(); }
      if (route.startsWith('/toast/')) return onToastRequest(route, request.url);
      if (route.startsWith('/update-now/')) { updateNow(new URL(request.url).searchParams.get('k')); return noContentResponse(); }
      if (route.startsWith('/find/')) return onFindRequest(route, request.url);
      if (route.startsWith('/dialog/logs/')) { void openLogsFolder(); return noContentResponse(); }
      if (route.startsWith('/window/close/')) { if (state.win && !state.win.isDestroyed()) state.win.close(); return noContentResponse(); }
      if (route.startsWith('/menu/report/')) { void saveReport(); return noContentResponse(); }
      if (route.startsWith('/dialog/report-show/')) { if (state.lastReportPath) shell.showItemInFolder(state.lastReportPath); return noContentResponse(); }
      if (route.startsWith('/dialog/log-refresh/')) { if (state.dialogRender) state.dialogRender(false); return noContentResponse(); }
      if (route.startsWith('/menu/log/')) { showLogDialog(); return noContentResponse(); }
      if (route.startsWith('/menu/settings/')) { showSettingsDialog(); return noContentResponse(); }
      if (route.startsWith('/settings/set/')) return onSettingsSet(request.url);
      if (route.startsWith('/settings/section/')) {
        const sec = new URL(request.url).searchParams.get('s');
        if (SETTINGS_SECTIONS.includes(sec)) state.settingsSection = sec;
        return noContentResponse();
      }
      if (route.startsWith('/settings/pick-dsh/')) return onSettingsPick('dshPath');
      if (route.startsWith('/settings/pick-cwd/')) return onSettingsPick('cwd');
      if (route.startsWith('/settings/reset-updates/')) return onSettingsResetUpdates();
      if (route.startsWith('/settings/relaunch/')) { relaunchLauncher(); return noContentResponse(); }
      if (route.startsWith('/menu/close-context/')) { if (state.openContextMenu) state.openContextMenu.closePopup(); return noContentResponse(); }
      if (route.startsWith('/dialog/gpu-enable/')) return onDialogGpuEnable();
      if (route.startsWith('/menu/restart-dsh/')) { void restartDsh({ confirm: false }); return noContentResponse(); }
      if (route.startsWith('/dialog/close/')) return onDialogClose();
      if (route.startsWith('/dialog/copy/')) return onDialogCopy();
    } catch { return blankResponse(); } // как раньше: сбой обработчика — пустая страница
    return onPasteRequest(route);
  });

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
  trackWindowState(state.win);
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
      if (new URL(state.win.webContents.getURL()).origin === new URL(PLAIN_URL).origin) { void maybeShowWhatsNew(); maybeShowCrashNotice(); maybeShowDshCrashNotice(); }
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
    if (await plainUrlIsAuthed()) {
      console.log('[launcher] валидный куки есть — открываю GUI');
      await loadInWin(PLAIN_URL);
    } else {
      console.log('[launcher] валидного куки нет — показываю страницу вставки токена');
      showPastePage();
    }
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
  // установка dsh / маркета / шаг обновления: свой процесс и таймер перерисовки
  for (const st of [installState, marketState, updateState]) {
    if (st.proc) { try { st.proc.kill('SIGTERM'); } catch { /* noop */ } }
    clearInterval(st.timer);
  }
  if (updateState.restartTimer) {
    clearTimeout(updateState.restartTimer);
    updateState.restartTimer = null;
  }
  Promise.resolve().then(async () => {
    await stopDsh();
    if (state.adoptedDsh) await stopAdoptedDsh();
  }).finally(() => {
    if (state.relaunchRequested) { try { spawnNewInstance(); } catch (e) { console.error('[launcher] перезапуск:', e.message); } }
    app.quit();
  });
});

// страховка: если before-quit почему-то не сработала — жёстко
app.on('will-quit', () => { killDshNow(); clearRunMarker(); });

// страховка на «жёсткое» завершение самого лаунчера: SIGTERM/SIGINT/SIGHUP (pkill,
// завершение сессии) и exit процесса. Поймать SIGKILL, отправленный самому
// лаунчеру, нельзя — в том случае dsh переживёт его (это ограничение
// отделимого процесса, лечится только системно — cgroup/supervisor).
function killDshNow() {
  if (state.weStartedDsh && state.dshProc && state.dshProc.pid !== undefined && !state.dshFatal) {
    killGroup(state.dshProc.pid, 'SIGKILL', dshLeaderExited());
  }
  if (state.adoptedDsh) {
    for (const t of adoptedTargets()) killGroup(t.id, 'SIGKILL', t.groupOnly);
  }
}
// По сигналу — штатное закрытие через before-quit (dsh получает SIGTERM и
// успевает сохраниться, через 4 с — SIGKILL). Жёстко (SIGKILL дереву и
// выход) — по повторному сигналу, если приложение ещё не готово или если
// штатное закрытие не уложилось в 8 с.
let termSignalSeen = false;
function onTermSignal(name, exitCode) {
  console.log(`[launcher] ${name} — останавливаю dsh`);
  if (termSignalSeen || !app.isReady()) {
    killDshNow();
    process.exit(exitCode);
  }
  termSignalSeen = true;
  setTimeout(() => {
    console.error('[launcher] штатное закрытие зависло — останавливаю dsh жёстко');
    killDshNow();
    process.exit(exitCode);
  }, 8000);
  app.quit();
}
// Ставить — после ready: Chromium при старте ставит свой обработчик
// SIGTERM/SIGINT/SIGHUP поверх раннего process.on (тот так и не вызывается),
// а после первого сигнала возвращает обработчик по умолчанию — повторный
// сигнал убивал лаунчер мгновенно, и медленно завершающийся dsh оставался
// сиротой. Поставленный позже, наш обработчик заменяет chromium-овский.
function installSignalHandlers() {
  process.on('SIGINT', () => onTermSignal('SIGINT', 130));
  process.on('SIGTERM', () => onTermSignal('SIGTERM', 143));
  process.on('SIGHUP', () => onTermSignal('SIGHUP', 129));
}
process.on('exit', killDshNow);
