'use strict';
// "Hard" termination of the launcher itself: SIGTERM/SIGINT/SIGHUP (pkill,
// session end) and process exit. A SIGKILL sent to the launcher itself
// cannot be caught — in that case dsh will outlive it (this is a limitation
// of the detached process, fixable only system-wide — cgroup/supervisor).
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

// On a signal — the regular shutdown via before-quit (dsh gets SIGTERM and
// has time to save, SIGKILL after 4 s). Hard (SIGKILL the tree and exit) —
// on a repeated signal, if the app is not ready yet or if the regular
// shutdown did not finish within 8 s.
let termSignalSeen = false;
let appRef = null; // app is passed to installSignalHandlers (after ready)
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

// Install it — after ready: on startup Chromium puts its own SIGTERM/
// SIGINT/SIGHUP handler over the early process.on (which is never called),
// and after the first signal restores the default handler — a repeated
// signal killed the launcher instantly, and a slowly terminating dsh was
// left orphaned. Installed later, our handler replaces chromium's.
function installSignalHandlers(app) {
  appRef = app;
  process.on('SIGINT', () => onTermSignal('SIGINT', 130));
  process.on('SIGTERM', () => onTermSignal('SIGTERM', 143));
  process.on('SIGHUP', () => onTermSignal('SIGHUP', 129));
  process.on('exit', killDshNow);
}

module.exports = { killDshNow, installSignalHandlers };
