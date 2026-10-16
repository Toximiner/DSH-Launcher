'use strict';
// Служебная схема dshlauncher://: подключение к GUI уже запущенного dsh,
// страницы-хендлеры (вставка токена, установка dsh, обновления, маркет,
// меню), таблица маршрутов и диспетчер protocol.handle. Модуль подключает
// electron — из юнит-тестов проверяется только чистая функция matchRoute
// (блок-маркер, test/_extract.js).
const { app, clipboard, net: electronNet, shell, protocol } = require('electron');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc } = require('./util');
const { PORT, PLAIN_URL, DSH_ARGS, ROUTE_KEY } = require('./env');
const { sleep, portOpen } = require('./port');
const { launcherRoute, stripRouteKey } = require('./util');
const { loadInWin, showPastePage, showPage, showStatus, liveLogTick, onLangSwitch } = require('./pages');
const { envWithBinDir, listenerAuthUrl } = require('./system');
const { runWithLiveLog } = require('./proc-log');
const { dshInstallCheck } = require('./dsh-install');
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

async function plainUrlIsAuthed() {
  try {
    const res = await electronNet.fetch(PLAIN_URL, { redirect: 'manual' });
    return res.status === 200;
  } catch {
    return false;
  }
}

// Окно — на GUI уже работающего dsh, который запустил не лаунчер (перезапуск
// из маркета, терминал): куки валидны → GUI; иначе ссылка входа из вывода
// процесса на порту (listenerAuthUrl); иначе ещё раз куки — порт открывается
// раньше, чем dsh готов проверять вход, — и только потом страница вставки.
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
        await openRunningGui('retry: dsh запущен');
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
    installState.running = true; // страница «выполняется» — сразу, до первой строки
    showInstallPage();
    const code = await runWithLiveLog(installState, npmInfo.npm, ['install', '-g', '@deepseek-ai/dsh'],
      { env: envWithBinDir(npmInfo.npm), redraw: () => {
        // running — иначе установка закончилась и страница уже переключилась
        // (успех/ошибка), и доп. перерисовка затёрла бы её note.
        liveLogTick(installState).then((ok) => {
          if (!ok && installState.running) showInstallPage();
        });
      } });
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

/* ===== matchRoute (тест test/routes.js) =====
   Таблица маршрутов dshlauncher:// (порядок важен: /menu/rollback/ раньше
   /rollback/). Чистая функция: префикс маршрута → имя; null — не наш. */
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
];

function matchRoute(route) {
  for (const [prefix, name] of ROUTE_TABLE) if (String(route).startsWith(prefix)) return name;
  return null;
}
/* ===== end matchRoute ===== */

// Имена из matchRoute → действия. route — строка маршрута, url — один
// разобранный URL запроса (параметры — из url.searchParams).
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
  // О программе / проверка обновлений по URL — для Docker-сценариев
  // (driver.js не умеет кликать по нативному меню); штатно — пункт меню.
  // Ответ 204: страница, с которой перешли (GUI dsh), остаётся на месте.
  'menu/about': () => { void showAboutDialog(); return noContentResponse(); },
  'menu/check': () => { void showUpdateCheckDialog(); return noContentResponse(); },
  'menu/rollback': () => { void showRollbackDialog(); return noContentResponse(); },
  rollback: (route, url) => { rollbackTo(url.searchParams.get('v')); return noContentResponse(); },
  'menu/update': (route, url) => { showUpdateNowDialog(url.searchParams.get('k')); return noContentResponse(); },
  toast: (route, url) => onToastRequest(route, url),
  'update-now': (route, url) => { updateNow(url.searchParams.get('k')); return noContentResponse(); },
  find: (route, url) => onFindRequest(route, url),
  'dialog/logs': () => { void openLogsFolder(); return noContentResponse(); },
  'menu/quit': () => { app.quit(); return noContentResponse(); }, // «Файл → Выйти» (role: 'quit')
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
};

// Диспетчер dshlauncher:// — вызывается main.js из onReady (до первого окна
// схема уже зарегистрирована как privileged).
function registerProtocol() {
  protocol.handle('dshlauncher', (request) => {
    try {
      // Без ключа запуска — не со страницы лаунчера (GUI dsh, плагин, внешний
      // сайт в окне): ничего не делаем. В журнал — только путь, без параметров.
      const route = stripRouteKey(launcherRoute(request.url), ROUTE_KEY);
      if (route === null) {
        console.error(`[launcher] dshlauncher: запрос без ключа запуска — отклонён: ${launcherRoute(request.url).slice(0, 80)}`);
        return noContentResponse();
      }
      // URL разбираем один раз: хендлеры берут параметры из url.searchParams.
      const url = new URL(request.url);
      const handler = ROUTE_HANDLERS[matchRoute(route)];
      return handler ? handler(route, url) : onPasteRequest(route);
    } catch { return blankResponse(); } // как раньше: сбой обработчика — пустая страница
  });
}

module.exports = {
  plainUrlIsAuthed,
  openRunningGui,
  onPasteRequest,
  onRetryRequest,
  blankResponse,
  noContentResponse,
  onInstallCopy,
  onInstallRecheck,
  onInstallRun,
  matchRoute,
  registerProtocol,
};
