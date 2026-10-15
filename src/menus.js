'use strict';
// Меню окна и контекстное меню; версии для «О программе».
const { Menu, clipboard } = require('electron');
const fs = require('fs');
const path = require('path');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { contextMenuItems } = require('./ui');
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

/* ---------------- меню окна: «О программе», «Проверить обновления» ---------------- */

// Строка меню сверху окна всегда видна (autoHideMenuBar: false):
//   «Файл»    — Перезапустить dsh…, Выйти (штатно, останавливает dsh);
//   «Правка»  — Вырезать / Копировать / Вставить / Выделить всё;
//   «Вид»     — перезагрузка, масштаб, поиск, орфография, полный экран;
//   «Справка» — Проверить обновления… (окно-диалог: сверяет GitHub и npm,
//               без авто-установки), Открыть папку логов, О программе
//               (окно-диалог: версии, пути, GPU; блок копируется для отчёта).
// Подписи следуют за языком окна — при переключении RU|EN меню собирается
// заново (onLangSwitch → buildAppMenu).

/* ===== appMenuTemplate (чистая функция, тест test/menu.js) =====
   Сочетания клавиш в меню — только подсказки (registerAccelerator: false):
   Ctrl+X/C/V/A Chromium обрабатывает сам, остальные — keyAction в
   before-input-event; перехват меню перебивал бы горячие клавиши GUI dsh.
   «Правка» — те же действия, что в контекстном меню, стандартными ролями
   (действуют на окно в фокусе — главное или диалог). */
function appMenuTemplate() {
  const hint = (key) => ({ accelerator: key, registerAccelerator: false });
  const edit = (role, ru, en, key) => ({ role, label: tr(ru, en), ...hint('CmdOrCtrl+' + key) });
  const viewItem = (act, ru, en, key) => ({ label: tr(ru, en), click: () => view.viewActions[act](), ...hint(key) });
  // найдено обновление (периодическая проверка или «Проверить обновления…») —
  // пункт «Обновить … до X…» в «Справке»
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
  // На Linux setApplicationMenu ставит меню всем окнам — у окна-диалога его быть не должно.
  if (state.tray) state.tray.setContextMenu(tray.trayMenu());
}



// Открытое контекстное меню — закрыть его по маршруту /menu/close-context/
// (Docker-сценарии: события CDP идут в страницу мимо нативного меню, и само
// оно в контейнере не закрывается).


/* ===== showContextMenu (тест test/menu.js, с подставными win/Menu/clipboard) ===== */
// wc — чьё содержимое (главное окно или слой диалога); меню — над окном w.
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
  // В лог — что показали (Docker-сценарий context_menu проверяет по нему:
  // кликнуть по нативному меню driver не может).
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

// Версия лаунчера: пакетная установка — dpkg; сборка из исходников —
// package.json корня репозитория (теперь синхронизирован с релизом, 1.2.0+).
async function launcherVersionRaw() {
  if (process.execPath.startsWith('/opt/dsh-launcher/')) return updates.launcherInstalledVersion();
  try { return require('../package.json').version; } catch { return null; }
}

// Версия dsh: если DSH_BIN исполняется — спрашиваем её (--version), иначе —
// версия, найденная installCheck при старте.
// Версия dsh для окон («О программе», «Проверить обновления…») — без запуска
// dsh --version (см. launcherInstalledVersion): из package.json пакета
// @deepseek-ai/dsh, к которому ведёт программа dsh (ссылка → …/bin.js), иначе
// — проверенная при старте; и только если её нет — запуск dsh --version.
function dshPackageVersion(bin) {
  try {
    for (let d = path.dirname(fs.realpathSync(bin)); d !== path.dirname(d); d = path.dirname(d)) {
      let j;
      try { j = JSON.parse(fs.readFileSync(path.join(d, 'package.json'), 'utf8')); } catch { continue; }
      if (j && j.name === updates.DSH_NPM_PKG && j.version) return String(j.version);
    }
  } catch { /* программы нет */ }
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

Object.assign(module.exports, { buildAppMenu, showContextMenu, launcherVersionRaw, dshVersionRaw }); // дополняем, а не заменяем: модули подключают друг друга по кругу
