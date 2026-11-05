'use strict';
// dshlauncher:// service scheme: connecting to the GUI of an already
// running dsh, page handlers (token paste, dsh install, updates, market,
// menu), the route table and the protocol.handle dispatcher. The module
// requires electron — unit tests cover only the pure matchRoute function
// (block marker, test/_extract.js).
const { app, clipboard, net: electronNet, shell, protocol } = require('electron');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc } = require('./util');
const { PORT, PLAIN_URL, DSH_ARGS, ROUTE_KEY } = require('./env');
const { sleep, portOpen } = require('./port');
const { launcherRoute, stripRouteKey } = require('./util');
const { loadInWin, showPastePage, showPage, showStatus, liveRedraw, onLangSwitch } = require('./pages');
const { envWithBinDir, listenerAuthUrl } = require('./system');
const { blankResponse, noContentResponse } = require('./respond'); // pure module — direct require, no registry
const { pushInputHistory } = require('./input-history'); // input history (↑/↓ in the dsh field); texts are not written to the log
const { runWithLiveLog } = require('./proc-log');
const { dshInstallCheck, pkexecNpmArgs } = require('./dsh-install');
const { DSH_NPM_PKG } = require('./updates-github');
const { retryButton } = require('./dsh-process');
const { launchDsh } = require('./launch');
const { installState, refreshInstallState, showInstallPage } = require('./install-page');
const { onMarketChoice } = require('./market');
const { onUpdateChoice } = require('./updates');
const { onToastRequest, showUpdateNowDialog, updateNow } = require('./updates-live');
const { onFindRequest } = require('./view');
const { openLogsFolder, restartDsh, onDialogGpuEnable } = require('./file-help');
const { saveReport } = require('./diagnostics');
const { showLogDialog } = require('./dsh-log');
const { onSettingsSet, onSettingsPick, onSettingsResetUpdates, relaunchLauncher, SETTINGS_SECTIONS, showSettingsDialog } = require('./settings-window');
const { onDialogClose, onDialogCopy, showAboutDialog, showUpdateCheckDialog } = require('./dialogs');
const { showRollbackDialog, rollbackTo } = require('./rollback');

const PLAIN_PROBE_TIMEOUT_MS = 1500; // how long to wait for dsh's HTTP reply (see plainUrlResponds)

async function plainUrlIsAuthed() {
  try {
    const res = await electronNet.fetch(PLAIN_URL, { redirect: 'manual' });
    return res.status === 200;
  } catch {
    return false;
  }
}

// dsh's HTTP server answers (any status — even 401): the port opened before
// the app was ready, and by the time of the reply the session cookie may
// may already have expired (market restart with a new token) — 200 is not
// required here. Timeout: socket is open but the server still does not
// answer (boot) — counted as "no".
async function plainUrlResponds() {
  try {
    const res = await electronNet.fetch(PLAIN_URL, { redirect: 'manual', signal: AbortSignal.timeout(PLAIN_PROBE_TIMEOUT_MS) });
    return res.status > 0;
  } catch {
    return false;
  }
}

// Window — to the GUI of a dsh that is already running but was not started
// by the launcher (restart from the market, terminal): cookie valid → GUI;
// otherwise the login link from the process output on the port
// (listenerAuthUrl); otherwise the cookie once more — the port opens before
// dsh is ready to check login — and only then the paste page.
const AUTH_WAIT_MS = 5000;

async function openRunningGui(why) {
  if (await plainUrlIsAuthed()) {
    console.log(`[launcher] ${why}: куки валидны — открываю GUI`);
    return loadInWin(PLAIN_URL);
  }
  const deadline = Date.now() + AUTH_WAIT_MS;
  for (;;) {
    const url = listenerAuthUrl(PORT, PLAIN_URL);
    if (url) {
      console.log(`[launcher] ${why}: ссылка входа — из вывода процесса dsh на порту`);
      return loadInWin(url);
    }
    if (Date.now() >= deadline || state.stopping) break;
    await sleep(500);
    if (await plainUrlIsAuthed()) {
      console.log(`[launcher] ${why}: куки валидны — открываю GUI`);
      return loadInWin(PLAIN_URL);
    }
  }
  console.log(`[launcher] ${why}: валидного куки и ссылки входа нет — страница вставки токена`);
  showPastePage();
}

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

// «Проверить снова» (Check again) on the fatal page: if dsh was started
// manually (e.g. in a terminal) and the window was left open — return to the GUI.
async function onRetryRequest() {
  try {
    if (state.win && !state.win.isDestroyed() && !state.stopping) {
      const up = await portOpen();
      if (up) {
        state.weStartedDsh = false; // someone else's dsh — don't kill it on window close
        state.dshProc = null;
        state.dshFatal = false;
        state.portState = 'up';
        await openRunningGui('retry: dsh запущен');
      } else {
        // The binary may have disappeared (uninstall, different DSH_BIN) — in
        // that case the advice to start it in a terminal is pointless.
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

// «Скопировать команду» (Copy command) on the dsh install page.
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

// «Проверить ещё раз» (Check again): dsh appeared (installed manually or via the button) — continue launching.
async function onInstallRecheck() {
  try {
    if (installState.running) return blankResponse(); // installation in progress — the page will refresh itself
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

// «Установить» (Install): npm install -g directly from the window, with a live log.
async function onInstallRun() {
  try {
    if (installState.running || !state.win || state.win.isDestroyed() || state.stopping) return blankResponse();
    await refreshInstallState();
    if (installState.running) return blankResponse(); // while checking, an installation has already been started
    const npmInfo = installState.npmInfo;
    // System npm (root's prefix): install via pkexec — polkit will ask for
    // the password in a system dialog, as when updating the launcher.
    const viaPkexec = !npmInfo.canAuto && npmInfo.rootMode === 'pkexec';
    if (!npmInfo.canAuto && !viaPkexec) {
      showInstallPage(() => tr('Кнопка «Установить» недоступна: глобальный npm prefix требует sudo — выполните команду в терминале.', '“Install” is unavailable: the global npm prefix requires sudo — run the command in a terminal.'), true);
      return blankResponse();
    }
    console.log(`[launcher] установка dsh из окна: ${npmInfo.command}`);
    installState.running = true; // the page shows "running" — immediately, before the first line
    showInstallPage();
    // @latest — explicit: the bare spec in npm 10.x (Node 22) with a latest
    // prerelease resolves as * and fails with ETARGET (see INSTALL_CMD in
    // dsh-install.js).
    const args = ['install', '-g', `${DSH_NPM_PKG}@latest`];
    const code = await runWithLiveLog(installState,
      viaPkexec ? npmInfo.pkexec : npmInfo.npm,
      viaPkexec ? pkexecNpmArgs(npmInfo.npm, args) : args,
      {
        env: viaPkexec ? undefined : envWithBinDir(npmInfo.npm),
        // running — otherwise the installation is over and the page has
        // already switched (success/error), and the extra redraw would
        // overwrite its note.
        redraw: liveRedraw(installState, () => installState.running, showInstallPage),
      });
    if (code === 0) {
      console.log('[launcher] dsh установлен — продолжаю запуск');
      // If the prefix is not /usr, the binary landed outside /usr/bin — find and remember the actual path.
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

/* ===== matchRoute (test test/routes.js) =====
   dshlauncher:// route table (order matters: /menu/rollback/ before
   /rollback/). Pure function: route prefix → name; null — not ours. */
const ROUTE_TABLE = [
  ['/retry/', 'retry'],
  ['/lang/', 'lang'],
  ['/install/copy/', 'install/copy'],
  ['/install/run/', 'install/run'],
  ['/install/recheck/', 'install/recheck'],
  ['/market/install/', 'market/install'],
  ['/market/skip/', 'market/skip'],
  ['/market/never/', 'market/never'],
  ['/update/install/', 'update/install'],
  ['/update/later/', 'update/later'],
  ['/update/never/', 'update/never'],
  ['/update/recheck/', 'update/recheck'],
  ['/update/copy/', 'update/copy'],
  ['/update/restart/', 'update/restart'],
  ['/menu/about/', 'menu/about'],
  ['/menu/check/', 'menu/check'],
  ['/menu/rollback/', 'menu/rollback'],
  ['/rollback/', 'rollback'],
  ['/menu/update/', 'menu/update'],
  ['/toast/', 'toast'],
  ['/update-now/', 'update-now'],
  ['/find/', 'find'],
  ['/dialog/logs/', 'dialog/logs'],
  ['/menu/quit/', 'menu/quit'],
  ['/window/close/', 'window/close'],
  ['/menu/report/', 'menu/report'],
  ['/dialog/report-show/', 'dialog/report-show'],
  ['/dialog/log-refresh/', 'dialog/log-refresh'],
  ['/menu/log/', 'menu/log'],
  ['/menu/settings/', 'menu/settings'],
  ['/settings/set/', 'settings/set'],
  ['/settings/section/', 'settings/section'],
  ['/settings/pick-dsh/', 'settings/pick-dsh'],
  ['/settings/pick-cwd/', 'settings/pick-cwd'],
  ['/settings/reset-updates/', 'settings/reset-updates'],
  ['/settings/relaunch/', 'settings/relaunch'],
  ['/menu/close-context/', 'menu/close-context'],
  ['/dialog/gpu-enable/', 'dialog/gpu-enable'],
  ['/menu/restart-dsh/', 'menu/restart-dsh'],
  ['/dialog/close/', 'dialog/close'],
  ['/dialog/copy/', 'dialog/copy'],
  ['/history/push/', 'history/push'],
];

function matchRoute(route) {
  for (const [prefix, name] of ROUTE_TABLE) if (String(route).startsWith(prefix)) return name;
  return null;
}
/* ===== end matchRoute ===== */

// Names from matchRoute → actions. route — the route string, url — one
// parsed request URL (params come from url.searchParams).
const ROUTE_HANDLERS = {
  retry: () => onRetryRequest(),
  lang: (route) => onLangSwitch(route.split('/')[2]),
  'install/copy': () => onInstallCopy(),
  'install/run': () => onInstallRun(),
  'install/recheck': () => onInstallRecheck(),
  'market/install': () => onMarketChoice('install'),
  'market/skip': () => onMarketChoice('skip'),
  'market/never': () => onMarketChoice('never'),
  'update/install': () => onUpdateChoice('install'),
  'update/later': () => onUpdateChoice('later'),
  'update/never': () => onUpdateChoice('never'),
  'update/recheck': () => onUpdateChoice('recheck'),
  'update/copy': () => onUpdateChoice('copy'),
  'update/restart': () => onUpdateChoice('restart'),
  // About / update check via URL — for Docker scenarios (driver.js cannot
  // click the native menu); normally — a menu item. 204 response: the page
  // you came from (the dsh GUI) stays in place.
  'menu/about': () => { void showAboutDialog(); return noContentResponse(); },
  'menu/check': () => { void showUpdateCheckDialog(); return noContentResponse(); },
  'menu/rollback': () => { void showRollbackDialog(); return noContentResponse(); },
  rollback: (route, url) => { rollbackTo(url.searchParams.get('v')); return noContentResponse(); },
  'menu/update': (route, url) => { showUpdateNowDialog(url.searchParams.get('k')); return noContentResponse(); },
  toast: (route, url) => onToastRequest(route, url),
  'update-now': (route, url) => { updateNow(url.searchParams.get('k')); return noContentResponse(); },
  find: (route, url) => onFindRequest(route, url),
  'dialog/logs': () => { void openLogsFolder(); return noContentResponse(); },
  'menu/quit': () => { app.quit(); return noContentResponse(); }, // «Файл → Выйти» (File → Quit) (role: 'quit')
  'window/close': () => { if (state.win && !state.win.isDestroyed()) state.win.close(); return noContentResponse(); },
  'menu/report': () => { void saveReport(); return noContentResponse(); },
  'dialog/report-show': () => { if (state.lastReportPath) shell.showItemInFolder(state.lastReportPath); return noContentResponse(); },
  'dialog/log-refresh': () => { if (state.dialogRender) state.dialogRender(false); return noContentResponse(); },
  'menu/log': () => { showLogDialog(); return noContentResponse(); },
  'menu/settings': () => { showSettingsDialog(); return noContentResponse(); },
  'settings/set': (route, url) => onSettingsSet(url),
  'settings/section': (route, url) => {
    const sec = url.searchParams.get('s');
    if (SETTINGS_SECTIONS.includes(sec)) state.settingsSection = sec;
    return noContentResponse();
  },
  'settings/pick-dsh': () => onSettingsPick('dshPath'),
  'settings/pick-cwd': () => onSettingsPick('cwd'),
  'settings/reset-updates': () => onSettingsResetUpdates(),
  'settings/relaunch': () => { relaunchLauncher(); return noContentResponse(); },
  'menu/close-context': () => { if (state.openContextMenu) state.openContextMenu.closePopup(); return noContentResponse(); },
  'dialog/gpu-enable': () => onDialogGpuEnable(),
  'menu/restart-dsh': () => { void restartDsh({ confirm: false }); return noContentResponse(); },
  'dialog/close': () => onDialogClose(),
  'dialog/copy': () => onDialogCopy(),
  // Input history (dsh page → main): the text arrives as a param and is not
  // written to the log — only the length and a counter. 204 response: the
  // page stays.
  'history/push': (route, url) => {
    const text = url.searchParams.get('text') || '';
    const r = pushInputHistory(text);
    if (r.changed) console.log(`[launcher] история ввода: сохранено сообщение (${text.length} симв.), всего ${r.count}`);
    return noContentResponse();
  },
};

// dshlauncher:// dispatcher — called by main.js from onReady (before the
// first window, the scheme is already registered as privileged).
function registerProtocol() {
  protocol.handle('dshlauncher', (request) => {
    try {
      // No launch key — not from the launcher page (the dsh GUI, a plugin,
      // an external site in the window): do nothing. The log gets only the
      // path, without params.
      const route = stripRouteKey(launcherRoute(request.url), ROUTE_KEY);
      if (route === null) {
        console.error(`[launcher] dshlauncher: запрос без ключа запуска — отклонён: ${launcherRoute(request.url).slice(0, 80)}`);
        return noContentResponse();
      }
      // Parse the URL once: handlers take params from url.searchParams.
      const url = new URL(request.url);
      const handler = ROUTE_HANDLERS[matchRoute(route)];
      return handler ? handler(route, url) : onPasteRequest(route);
    } catch { return blankResponse(); } // as before: a handler failure — a blank page
  });
}

module.exports = {
  plainUrlIsAuthed,
  plainUrlResponds,
  openRunningGui,
  onPasteRequest,
  onRetryRequest,
  onInstallCopy,
  onInstallRecheck,
  onInstallRun,
  matchRoute,
  registerProtocol,
};
