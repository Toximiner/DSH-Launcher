'use strict';
// The launcher's shared state — what different parts read and modify. The
// initial values are set by main.js (where the let declarations used to be).
//
// Operations with their own process/timer (dsh install, market install, an
// update step) have different state shapes — but on shutdown everything is
// the same: SIGTERM to the process and clearInterval. Modules register a
// cleanup function via settleOperation at load time, and before-quit calls
// all of them via settleAllOperations — without knowing the list of modules.
const operationSettlers = [];

module.exports = {
  win: null,                 // the main window
  stopping: false,           // the launcher is shutting down
  dshBin: null,              // the dsh program (DSH_BIN; the found one may change)
  dshProc: null,             // the dsh process started by the launcher
  weStartedDsh: false,       // the launcher started dsh (stop it on shutdown)
  dshFatal: false,           // dsh did not start / crashed — do not restart
  adoptedDsh: false,         // dsh was "adopted" after self-resurrection
  adoptedOldPgid: null,      // the original dsh's group before self-resurrection
  authUrl: null,             // the URL with the token, printed by dsh
  launching: false,          // dsh launch in progress (update questions, etc.)
  restarting: false,         // restart to a new version after an update
  relaunchRequested: false,  // «Перезапустить лаунчер» (Relaunch the launcher) (settings, «Обновить сейчас»/Update now)
  dshInstalledVersion: null, // the dsh version checked at start
  currentPage: null,         // redraw of the current service page (language switch)
  portState: null,           // null | 'up' | 'down' — dsh's port (auto-recovery)
  dialogView: null,          // the dialog — a layer in the main window (WebContentsView)
  dialogRender: null,        // redraw of the dialog content: (copied) => void
  dialogCopyText: '',        // the text of «Скопировать для отчёта» (Copy for the report)
  tray: null,                // the tray icon (Tray) or null
  lastReportPath: null,      // the last saved report («Показать в папке»/Show in folder)
  settingsSection: 'general', // the section of the «Настройки» (Settings) window (left menu)
  availableUpdates: {},      // found updates: { launcher, dsh } → release (.version …)
  updateChecks: { launcher: null, dsh: null }, // update checks at start (promises)
  launcherUpdatedOnDisk: false, // the launcher package is updated, but the old process is running
  openContextMenu: null,     // an open context menu (close via a route)
  findView: null,            // the page search panel (Ctrl+F)
  spellcheckOn: true,        // spell check in text fields
  restartingDsh: false,      // «Перезапустить dsh» (Restart dsh) in progress: stopping dsh is not a failure
  dshCrashNotice: null,      // dsh was restarted after a crash — a banner once the GUI loads
  settleOperation(fn) { operationSettlers.push(fn); },
  settleAllOperations() { for (const fn of operationSettlers) fn(); },
};
