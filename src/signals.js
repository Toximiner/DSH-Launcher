'use strict';
// «Жёсткое» завершение самого лаунчера: SIGTERM/SIGINT/SIGHUP (pkill,
// завершение сессии) и exit процесса. Поймать SIGKILL, отправленный самому
// лаунчеру, нельзя — в том случае dsh переживёт его (это ограничение
// отделимого процесса, лечится только системно — cgroup/supervisor).
const state = require('./state');
const { killGroup, dshLeaderExited, adoptedTargets } = require('./dsh-stop');

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
let appRef = null; // app передаётся в installSignalHandlers (после ready)
function onTermSignal(name, exitCode) {
  console.log(`[launcher] ${name} — останавливаю dsh`);
  if (termSignalSeen || !appRef.isReady()) {
    killDshNow();
    process.exit(exitCode);
  }
  termSignalSeen = true;
  setTimeout(() => {
    console.error('[launcher] штатное закрытие зависло — останавливаю dsh жёстко');
    killDshNow();
    process.exit(exitCode);
  }, 8000);
  appRef.quit();
}

// Ставить — после ready: Chromium при старте ставит свой обработчик
// SIGTERM/SIGINT/SIGHUP поверх раннего process.on (тот так и не вызывается),
// а после первого сигнала возвращает обработчик по умолчанию — повторный
// сигнал убивал лаунчер мгновенно, и медленно завершающийся dsh оставался
// сиротой. Поставленный позже, наш обработчик заменяет chromium-овский.
function installSignalHandlers(app) {
  appRef = app;
  process.on('SIGINT', () => onTermSignal('SIGINT', 130));
  process.on('SIGTERM', () => onTermSignal('SIGTERM', 143));
  process.on('SIGHUP', () => onTermSignal('SIGHUP', 129));
  process.on('exit', killDshNow);
}

module.exports = { killDshNow, installSignalHandlers };
