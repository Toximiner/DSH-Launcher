'use strict';
// The window menu and the context menu; versions for "About".
const { Menu, clipboard } = require('electron');
const fs = require('fs');
const path = require('path');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { contextMenuItems } = require('./ui');
const { IS_PKG_INSTALL } = require('./env');
const { probeDsh } = require('./dsh-install');
const diagnostics = require('./diagnostics');
const dialogs = require('./dialogs');
const dshLog = require('./dsh-log');
const fileHelp = require('./file-help');
const rollback = require('./rollback');
const settingsWindow = require('./settings-window');
const tray = require('./tray');
const updatesLive = require('./updates-live');
const updates = require('./updates');
const view = require('./view');

/* ---------------- window menu: "About", "Check for updates" ---------------- */

// The menu bar at the top of the window is always visible (autoHideMenuBar: false):
//   "File"    — Restart dsh…, Quit (normal, stops dsh);
//   "Edit"    — Cut / Copy / Paste / Select all;
//   "View"    — reload, zoom, search, spell check, full screen;
//   "Help"    — Check for updates… (dialog window: compares GitHub and npm,
//               no auto-install), Open logs folder, About
//               (dialog window: versions, paths, GPU; the block is copied for the report).
// The labels follow the window's language — on an RU|EN switch the menu is
// rebuilt (onLangSwitch → buildAppMenu).

/* ===== appMenuTemplate (pure function, test test/menu.js) =====
   The key combinations in the menu are only hints (registerAccelerator: false):
   Ctrl+X/C/V/A Chromium handles itself, the rest — keyAction in
   before-input-event; a menu intercept would override the dsh GUI hotkeys.
   "Edit" — the same actions as in the context menu, with the standard roles
   (they act on the window in focus — the main one or a dialog). */
function appMenuTemplate() {
  const hint = (key) => ({ accelerator: key, registerAccelerator: false });
  const edit = (role, ru, en, key) => ({ role, label: tr(ru, en), ...hint('CmdOrCtrl+' + key) });
  const viewItem = (act, ru, en, key) => ({ label: tr(ru, en), click: () => view.viewActions[act](), ...hint(key) });
  // an update is found (the periodic check or "Check for updates…") —
  // an "Update … to X…" item in "Help"
  const upd = (kind, name) => (state.availableUpdates[kind] ? [{
    label: tr(`Обновить ${name} до ${state.availableUpdates[kind].version}…`, `Update ${name} to ${state.availableUpdates[kind].version}…`),
    click: () => updatesLive.showUpdateNowDialog(kind),
  }] : []);
  return [
    {
      label: tr('Файл', 'File'),
      submenu: [
        { label: tr('Настройки…', 'Settings…'), click: () => settingsWindow.showSettingsDialog(), ...hint('CmdOrCtrl+,') },
        { type: 'separator' },
        { label: tr('Перезапустить dsh…', 'Restart dsh…'), click: () => { void fileHelp.restartDsh(); } },
        { type: 'separator' },
        { label: tr('Выйти', 'Quit'), role: 'quit' },
      ],
    },
    {
      label: tr('Правка', 'Edit'),
      submenu: [
        edit('cut', 'Вырезать', 'Cut', 'X'),
        edit('copy', 'Копировать', 'Copy', 'C'),
        edit('paste', 'Вставить', 'Paste', 'V'),
        { type: 'separator' },
        edit('selectAll', 'Выделить всё', 'Select all', 'A'),
      ],
    },
    {
      label: tr('Вид', 'View'),
      submenu: [
        viewItem('reload', 'Перезагрузить', 'Reload', 'F5'),
        viewItem('reloadHard', 'Перезагрузить без кэша', 'Reload ignoring cache', 'CmdOrCtrl+Shift+R'),
        { type: 'separator' },
        viewItem('zoomIn', 'Увеличить', 'Zoom in', 'CmdOrCtrl+='),
        viewItem('zoomOut', 'Уменьшить', 'Zoom out', 'CmdOrCtrl+-'),
        viewItem('zoomReset', 'Обычный масштаб', 'Actual size', 'CmdOrCtrl+0'),
        { type: 'separator' },
        viewItem('find', 'Найти на странице…', 'Find in page…', 'CmdOrCtrl+F'),
        { type: 'separator' },
        { label: tr('Проверка орфографии', 'Spell check'), type: 'checkbox', checked: state.spellcheckOn, click: (item) => view.setSpellcheck(item.checked) },
        viewItem('fullscreen', 'Полноэкранный режим', 'Full screen', 'F11'),
      ],
    },
    {
      label: tr('Справка', 'Help'),
      submenu: [
        ...upd('launcher', 'DeepSeek Harness Launcher'),
        ...upd('dsh', 'dsh'),
        ...(state.availableUpdates.launcher || state.availableUpdates.dsh ? [{ type: 'separator' }] : []),
        { label: tr('Проверить обновления…', 'Check for updates…'), click: () => { void dialogs.showUpdateCheckDialog(); } },
        { label: tr('Вернуть предыдущую версию…', 'Roll back to a previous version…'), click: () => { void rollback.showRollbackDialog(); } },
        { label: tr('Журнал dsh…', 'dsh log…'), click: () => dshLog.showLogDialog() },
        { label: tr('Открыть папку логов', 'Open logs folder'), click: () => { void fileHelp.openLogsFolder(); } },
        { label: tr('Сохранить отчёт…', 'Save report…'), click: () => { void diagnostics.saveReport(); } },
        { type: 'separator' },
        { label: tr('О программе', 'About'), click: () => { void dialogs.showAboutDialog(); } },
      ],
    },
  ];
}
/* ===== end appMenuTemplate ===== */

function buildAppMenu() {
  Menu.setApplicationMenu(Menu.buildFromTemplate(appMenuTemplate()));
  // On Linux setApplicationMenu sets the menu for all windows — the dialog window must not have one.
  if (state.tray) state.tray.setContextMenu(tray.trayMenu());
}



// An open context menu — close it via the route /menu/close-context/
// (Docker scenarios: the CDP events go to the page past the native menu, and the
// menu itself does not close in the container).


/* ===== showContextMenu (test test/menu.js, with stand-in win/Menu/clipboard) ===== */
// wc — whose contents (the main window or the dialog layer); the menu — over the window w.
function showContextMenu(params, w = state.win, wc = w && w.webContents) {
  if (!w || w.isDestroyed() || !wc || wc.isDestroyed()) return;
  const run = {
    cut: () => wc.cut(),
    copy: () => wc.copy(),
    paste: () => wc.paste(),
    selectAll: () => wc.selectAll(),
    copyLink: () => clipboard.writeText(params.linkURL),
    replace: (i) => wc.replaceMisspelling(i.value),
    addWord: () => wc.session.addWordToSpellCheckerDictionary(params.misspelledWord),
    none: () => {},
  };
  const items = contextMenuItems(params);
  // To the log — what was shown (the context_menu Docker scenario checks by it:
  // the driver cannot click the native menu).
  console.log('[launcher] контекстное меню: ' +
    items.map((i) => (i.type ? '|' : i.label + (i.enabled ? '' : ' (off)'))).join(', '));
  const template = items.map((i) =>
    i.type ? i : { label: i.label, enabled: i.enabled, click: () => run[i.action](i) });
  const menu = Menu.buildFromTemplate(template);
  state.openContextMenu = menu;
  menu.once('menu-will-close', () => {
    if (state.openContextMenu === menu) state.openContextMenu = null;
    console.log('[launcher] контекстное меню закрыто');
  });
  menu.popup({ window: w });
}
/* ===== end showContextMenu ===== */

/* ===== launcherVersionRaw (test test/menu.js) ===== */
// The launcher's version: a package install — dpkg; a build from source —
// the package.json at the repository root (now synchronized with the release, 1.2.0+).
async function launcherVersionRaw() {
  if (IS_PKG_INSTALL) return updates.launcherInstalledVersion();
  try { return require('../package.json').version; } catch { return null; }
}
/* ===== end launcherVersionRaw ===== */

// The dsh version: if DSH_BIN runs — we ask for it (--version), otherwise —
// the version found by installCheck at start.
// The dsh version for the windows ("About", "Check for updates…") — without running
// dsh --version (see launcherInstalledVersion): from the package.json of the
// @deepseek-ai/dsh package that the dsh program points to (a link → …/bin.js), otherwise
// — the one checked at start; and only if it is missing — run dsh --version.
function dshPackageVersion(bin) {
  try {
    for (let d = path.dirname(fs.realpathSync(bin)); d !== path.dirname(d); d = path.dirname(d)) {
      let j;
      try { j = JSON.parse(fs.readFileSync(path.join(d, 'package.json'), 'utf8')); } catch { continue; }
      if (j && j.name === updates.DSH_NPM_PKG && j.version) return String(j.version);
    }
  } catch { /* the program is missing */ }
  return null;
}

async function dshVersionRaw() {
  const v = (state.dshBin && dshPackageVersion(state.dshBin)) || state.dshInstalledVersion;
  if (v) return v;
  if (state.dshBin) {
    const r = await probeDsh(state.dshBin);
    if (r.ok) return r.version;
  }
  return null;
}

Object.assign(module.exports, { buildAppMenu, showContextMenu, launcherVersionRaw, dshVersionRaw }); // we extend, not replace: the modules require each other cyclically
