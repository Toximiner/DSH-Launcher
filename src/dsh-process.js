'use strict';
// Процесс dsh: запуск (журнал, захват адреса с токеном), выход и самовозрождение.
const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');
const state = require('./state');
const { crashRestart } = require('./ui');
const i18n = require('./i18n');
const { tr } = i18n;
const { DSH_ARGS, CWD, LOG_DIR, LOG_FILE } = require('./env');
const { rotateLog, openDshLog } = require('./log');
const { dshEnv } = require('./dsh-install');
const { sleep, pushErr, portOpen } = require('./port'); // чистый модуль — прямой require, без реестра
const launch = require('./launch');
const pages = require('./pages');
const updatesLive = require('./updates-live');
const launcher = require('./launcher'); // общий реестр (наполняет main.js)

/* ---------------- запуск процесса dsh ---------------- */

// Chromium держит часть своих дескрипторов (разделяемая память, сокеты,
// icudtl.dat, порт отладки…) без «закрыть при exec» — дочерний процесс
// получал бы их все. Поэтому долгоживущие процессы (dsh, новый экземпляр
// лаунчера) запускаются через bash, который закрывает всё, кроме 0–2, и
// делает exec (pid — тот же). bash, а не sh: dash не закрывает дескрипторы с
// номером больше 9.
const CLOSE_INHERITED_FDS = 'for f in /proc/$$/fd/*; do n=${f##*/}; if [ "$n" -gt 2 ] 2>/dev/null; then eval "exec $n>&-" 2>/dev/null; fi; done; ';

function spawnClean(cmd, args, opts) {
  return spawn('/bin/bash', ['-c', CLOSE_INHERITED_FDS + 'exec "$@"', 'dsh-launcher', cmd, ...args], opts);
}

function startDsh() {
  // dsh.log пишет только startDsh (stdout dsh) — ротируем перед открытием потока.
  const rotated = rotateLog(LOG_FILE);
  if (rotated) console.log(`[launcher] лог прокручен: dsh.log (${rotated} Б) -> dsh.log.1`);
  fs.mkdirSync(LOG_DIR, { recursive: true });
  const logStream = openDshLog(LOG_FILE);
  state.authUrl = null; // токен прошлого процесса недействителен — ждём новый
  console.log(`[launcher] starting: ${state.dshBin} ${DSH_ARGS.join(' ')} (cwd=${CWD}, log=${LOG_FILE})`);
  const opts = {
    cwd: CWD,
    env: dshEnv({ DSH_STARTED_BY: 'dsh-launcher' }),
    stdio: ['ignore', 'pipe', 'pipe'],
    detached: true, // dsh становится лидером своей процесс-группы — можно убить всё дерево
  };
  // Через bash (spawnClean) ошибку «нет такой программы» spawn не вернёт —
  // такую (и команду без пути, из PATH) запускаем напрямую: 'error' как раньше.
  let direct = false;
  try { fs.accessSync(state.dshBin, fs.constants.X_OK); } catch { direct = true; }
  state.dshProc = direct || !path.isAbsolute(state.dshBin) ? spawn(state.dshBin, DSH_ARGS, opts) : spawnClean(state.dshBin, DSH_ARGS, opts);
  state.weStartedDsh = true;
  state.adoptedDsh = false; // снова собственный процесс — «усыновлённого» больше нет
  state.adoptedOldPgid = null;
  state.dshFatal = false;
  state.dshProc.stdout.on('data', (d) => {
    for (const line of d.toString().split('\n')) {
      if (!line.trim()) continue;
      console.log('[dsh]', line.trim());
      logStream.write(line + '\n');
      const m = /dsh web:\s*(https?:\/\/\S+)/.exec(line) ||
                /(https?:\/\/[^\s]*\?token=[^\s&]+)/.exec(line);
      if (m) {
        console.log('[launcher] captured auth URL');
        state.authUrl = m[1];
      }
    }
  });
  state.dshProc.stderr.on('data', (d) => {
    for (const line of d.toString().split('\n')) {
      pushErr(line);
      logStream.write(line + '\n');
    }
  });
  state.dshProc.on('error', (err) => {
    state.dshFatal = true;
    pushErr(tr(`ошибка запуска dsh: ${err.message}`, `failed to start dsh: ${err.message}`));
  });
  const proc = state.dshProc;
  state.dshProc.on('exit', (code, signal) => {
    try { logStream.end(); } catch { /* noop */ }
    if (proc !== state.dshProc || state.restartingDsh) { // остановлен «Перезапустить dsh» — это не сбой
      console.log('[launcher] прежний процесс dsh завершился (перезапуск):', { code, signal });
      return;
    }
    handleDshExit(code, signal);
  });
}

/* ---------------- самовозрождение dsh (self-restart) ----------------
 * Менеджер плагинов dsh перезапускает сам процесс (после обновления плагина
 * уведомляет «изменения вступят в силу при следующем старте», и при
 * подтверждении дш поднимает новый процесс, а старый спокойно выходит с
 * code=0). Порт при этом может не закрываться вовсе (новый процесс
 * подхватывает его до выхода старого) — watchdog такой переход не видит.
 * Поэтому по exit нашего дочернего процесса проверяем порт: если он жив
 * или оживает — не показываем ошибку, а «усыновляем» запущенный dsh
 * (дальше работаем с ним как с подключением, но при закрытии окна всё
 * равно останавливаем — pid ищется по порту) и обновляем окно. Куки при
 * этом живы — секрет подписи хранится в ~/.dsh/.credentials.yaml и
 * переживает перезапуск.
 */
let adoptionCheck = false;

async function handleDshExit(code, signal) {
  console.log('[launcher] dsh process exited:', { code, signal });
  if (state.stopping) return;
  pushErr(tr(`dsh остановился (code=${code}, signal=${signal})`, `dsh stopped (code=${code}, signal=${signal})`));
  if (adoptionCheck) return; // на всякий случай: один цикл проверки
  adoptionCheck = true;
  try {
    pages.showPage(() => pages.showStatus(tr('dsh перезапускается…', 'dsh is restarting…'), tr(
      `<p>Процесс <code>dsh</code> завершился (code=${code}).<br>
         Проверяю, не запустился ли новый процесс…</p>`,
      `<p>The <code>dsh</code> process exited (code=${code}).<br>
         Checking whether a new process has started…</p>`), false));
    const deadline = Date.now() + 8000;
    let up = await portOpen();
    while (!up && Date.now() < deadline) { await sleep(500); up = await portOpen(); }
    if (up && !state.stopping && state.win && !state.win.isDestroyed()) {
      console.log('[launcher] порт снова открыт — dsh перезапустился сам, перехожу в режим подключения');
      state.weStartedDsh = false; // новый процесс — не наш дочерний (ручки на него нет)
      state.adoptedDsh = true;    // но это тот же dsh сессии — при закрытии окна остановим
      if (state.dshProc && state.dshProc.pid !== undefined) state.adoptedOldPgid = state.dshProc.pid;
      state.dshProc = null;
      state.dshFatal = false;
      state.portState = 'up';     // синхронизация со watchdog — не давать лишней перезагрузки
      await launcher.recoverWindow(true);
      return;
    }
  } finally {
    adoptionCheck = false;
  }
  // Сам не поднялся. Упал (не штатный выход) — перезапускаем, но не по кругу.
  const d = crashRestart(crashHistory, Date.now(), code, signal);
  crashHistory = d.history;
  if (d.restart && !state.stopping && state.win && !state.win.isDestroyed()) {
    console.log(`[launcher] dsh упал (code=${code}, signal=${signal}) — перезапускаю (${crashHistory.length} за 10 минут)`);
    state.dshCrashNotice = { code, signal };
    state.launching = true; // слежение за портом не вмешивается — окно до GUI доведёт запуск
    try { await launch.launchDshInner(); } finally { state.launching = false; }
    return;
  }
  const repeated = code !== 0 || Boolean(signal);
  if (repeated) console.log('[launcher] dsh падает снова и снова — авто-перезапуск остановлен');
  if (state.win && !state.win.isDestroyed() && !state.stopping) {
    pages.showPage(() => pages.showStatus(tr('dsh остановился', 'dsh has stopped'), (repeated ? tr(
      `<p>Процесс DeepSeek Harness падает снова и снова — лаунчер перезапускал его
         несколько раз за последние 10 минут и больше не пробует. Последние строки
         журнала — ниже, весь журнал — «Справка → Журнал dsh…».</p>`,
      `<p>The DeepSeek Harness process keeps crashing — the launcher restarted it
         several times in the last 10 minutes and has stopped trying. The latest
         log lines are below, the whole log is in “Help → dsh log…”.</p>`) : tr(
      `<p>Процесс DeepSeek Harness завершился, и за 8 секунд новый процесс
         не запустился.</p>`,
      `<p>The DeepSeek Harness process exited, and no new process started
         within 8 seconds.</p>`)) + retryButton() + pages.stderrBlock(), true));
  }
}

let crashHistory = []; // времена авто-перезапусков после сбоя

// Плашка «dsh перезапущен после сбоя» — когда GUI снова загрузился.
function maybeShowDshCrashNotice() {
  const c = state.dshCrashNotice;
  if (!c) return;
  state.dshCrashNotice = null;
  console.log('[launcher] уведомление: dsh перезапущен после сбоя');
  updatesLive.showToast(updatesLive.toastPageUrl({
    title: tr('dsh перезапущен после сбоя', 'dsh was restarted after a crash'),
    sub: c.signal ? tr(`сигнал ${c.signal}`, `signal ${c.signal}`) : tr(`код выхода ${c.code}`, `exit code ${c.code}`),
    buttons: [{ label: tr('Журнал dsh…', 'dsh log…'), href: 'dshlauncher://toast/dsh-log/' }],
  }));
}

/* ===== retryButton (тест test/pages.js) ===== */
function retryButton() {
  return `<button onclick="location.href='dshlauncher://retry/'" style="margin-bottom:16px">${tr('Проверить снова', 'Check again')}</button>`;
}
/* ===== end retryButton ===== */

Object.assign(module.exports, { CLOSE_INHERITED_FDS, startDsh, retryButton, maybeShowDshCrashNotice }); // дополняем, а не заменяем: модули подключают друг друга по кругу
