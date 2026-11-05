'use strict';
// dsh process: launch (log, capture of the URL with token), exit and self-revival.
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
const { sleep, pushErr, portOpen } = require('./port'); // pure module — direct require, no registry
const launch = require('./launch');
const pages = require('./pages');
const updatesLive = require('./updates-live');
const launcher = require('./launcher'); // shared registry (populated by main.js)

/* ---------------- launch of the dsh process ---------------- */

// Chromium keeps some of its descriptors (shared memory, sockets,
// icudtl.dat, debug port…) without "close on exec" — the child process
// would inherit all of them. So long-running processes (dsh, a new
// launcher instance) are spawned through bash, which closes everything
// except 0–2 and then execs (the pid stays the same). bash, not sh:
// dash does not close descriptors numbered above 9.
const CLOSE_INHERITED_FDS = 'for f in /proc/$$/fd/*; do n=${f##*/}; if [ "$n" -gt 2 ] 2>/dev/null; then eval "exec $n>&-" 2>/dev/null; fi; done; ';

function spawnClean(cmd, args, opts) {
  return spawn('/bin/bash', ['-c', CLOSE_INHERITED_FDS + 'exec "$@"', 'dsh-launcher', cmd, ...args], opts);
}

function startDsh() {
  // dsh.log is written only by startDsh (dsh stdout) — rotate before opening the stream.
  const rotated = rotateLog(LOG_FILE);
  if (rotated) console.log(`[launcher] лог прокручен: dsh.log (${rotated} Б) -> dsh.log.1`);
  fs.mkdirSync(LOG_DIR, { recursive: true });
  const logStream = openDshLog(LOG_FILE);
  state.authUrl = null; // token of the previous process is no longer valid — waiting for a new one
  console.log(`[launcher] starting: ${state.dshBin} ${DSH_ARGS.join(' ')} (cwd=${CWD}, log=${LOG_FILE})`);
  const opts = {
    cwd: CWD,
    env: dshEnv({ DSH_STARTED_BY: 'dsh-launcher' }),
    stdio: ['ignore', 'pipe', 'pipe'],
    detached: true, // dsh becomes the leader of its own process group — the whole tree can be killed
  };
  // Via bash (spawnClean) spawn would not return a "no such program" error —
  // such a binary (and a bare command from PATH) is launched directly: 'error' as before.
  let direct = false;
  try { fs.accessSync(state.dshBin, fs.constants.X_OK); } catch { direct = true; }
  state.dshProc = direct || !path.isAbsolute(state.dshBin) ? spawn(state.dshBin, DSH_ARGS, opts) : spawnClean(state.dshBin, DSH_ARGS, opts);
  state.weStartedDsh = true;
  state.adoptedDsh = false; // our own process again — no "adopted" one left
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
    if (proc !== state.dshProc || state.restartingDsh) { // stopped by "Restart dsh" — that is not a crash
      console.log('[launcher] прежний процесс dsh завершился (перезапуск):', { code, signal });
      return;
    }
    handleDshExit(code, signal);
  });
}

/* ---------------- dsh self-revival (self-restart) ----------------
 * dsh's plugin manager restarts the process itself (after a plugin update
 * it reports "the changes take effect on the next start", and on
 * confirmation dsh brings up a new process while the old one exits calmly with
 * code=0). The port may not close at all in this case (the new process
 * grabs it before the old one exits) — the watchdog cannot see such a handover.
 * So on the exit of our child process we check the port: if it is
 * alive or comes up — we do not show an error, but "adopt" the running dsh
 * (from then on we work with it as a connection, but on window close we
 * still stop it — the pid is found via the port) and update the window. The
 * cookies are still alive then — the signing secret is stored in ~/.dsh/.credentials.yaml and
 * survives the restart.
 */
let adoptionCheck = false;

async function handleDshExit(code, signal) {
  console.log('[launcher] dsh process exited:', { code, signal });
  if (state.stopping) return;
  pushErr(tr(`dsh остановился (code=${code}, signal=${signal})`, `dsh stopped (code=${code}, signal=${signal})`));
  if (adoptionCheck) return; // just in case: only one check cycle
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
      state.weStartedDsh = false; // the new process is not our child (we hold no handles on it)
      state.adoptedDsh = true;    // but it is the same dsh of this session — we will stop it on window close
      if (state.dshProc && state.dshProc.pid !== undefined) state.adoptedOldPgid = state.dshProc.pid;
      state.dshProc = null;
      state.dshFatal = false;
      state.portState = 'up';     // sync with the watchdog — do not allow an extra restart
      await launcher.recoverWindow(true);
      return;
    }
  } finally {
    adoptionCheck = false;
  }
  // It did not come back up on its own. It crashed (not a normal exit) — restart, but not in a loop.
  const d = crashRestart(crashHistory, Date.now(), code, signal);
  crashHistory = d.history;
  if (d.restart && !state.stopping && state.win && !state.win.isDestroyed()) {
    console.log(`[launcher] dsh упал (code=${code}, signal=${signal}) — перезапускаю (${crashHistory.length} за 10 минут)`);
    state.dshCrashNotice = { code, signal };
    state.launching = true; // the port watcher does not interfere — the launch drives the window to the GUI
    try { await launch.launchDshInner(); } finally { state.launching = false; }
    return;
  }
  const repeated = code !== 0 || Boolean(signal);
  if (repeated) console.log('[launcher] dsh падает снова и снова — авто-перезапуск остановлен');
  if (repeated) state.dshFatal = true; // dsh is stopped and will no longer be restarted (status on the tray icon)
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

let crashHistory = []; // times of auto-restarts after a crash

// "dsh was restarted after a crash" toast — shown once the GUI has loaded again.
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

/* ===== retryButton (test test/pages.js) ===== */
function retryButton() {
  return `<button onclick="location.href='dshlauncher://retry/'" style="margin-bottom:16px">${tr('Проверить снова', 'Check again')}</button>`;
}
/* ===== end retryButton ===== */

Object.assign(module.exports, { CLOSE_INHERITED_FDS, startDsh, retryButton, maybeShowDshCrashNotice }); // we extend, not replace: the modules require each other cyclically
