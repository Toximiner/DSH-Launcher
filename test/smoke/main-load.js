'use strict';
// Смоук: main.js грузится целиком с подменой electron, и все имена реестра
// launcher (Object.defineProperties в main.js) разрешаются. Ловит потерянный
// импорт, сбитый реестр и топовые ошибки модулей, которые юнит-тесты (чистые
// функции) не видят. Живое окно не поднимается: whenReady стаба не разрешается.
// Отдельная папка test/smoke/: не попадает в глоб npm run coverage (test/*.js),
// иначе граф main.js размыл бы отчёт о покрытии.
const { test } = require('node:test');
const assert = require('assert');
const Module = require('module');
const fs = require('fs');
const os = require('os');
const path = require('path');

const TMP = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-launcher-smoke-'));
// ДО require main.js: env.js считает LOG_DIR из DSH_LOG_DIR
process.env.DSH_LOG_DIR = TMP;

const origLoad = Module._load;
const noop = () => {};
// Стаб electron: только то, что main.js и src/* касаются при загрузке.
const electron = new Proxy({}, { get: (_t, p) => {
  if (p === 'app') return {
    getName: () => 'dsh-launcher', setName: noop,
    requestSingleInstanceLock: () => true, on: noop,
    whenReady: () => new Promise(() => {}), // не разрешается — окно не поднимается
    commandLine: { appendSwitch: noop }, disableHardwareAcceleration: noop,
    getPath: () => TMP, getLocale: () => 'en-US', quit: noop,
  };
  if (p === 'crashReporter') return { start: noop };
  if (p === 'protocol') return { registerSchemesAsPrivileged: noop, handle: noop };
  if (p === 'session') return { fromPartition: () => ({ setProxy: noop, clearCache: noop }) };
  if (p === 'BrowserWindow') return class { static getAllWindows() { return []; } };
  if (p === 'Tray') return { setToolTip: noop, setContextMenu: noop, destroy: noop, on: noop, setImage: noop, setVisible: noop };
  if (p === 'Menu') return { buildFromTemplate: () => ({ popup: noop, close: noop }), setApplicationMenu: noop };
  if (p === 'MenuItem') return {};
  if (p === 'clipboard') return { writeText: noop, readText: () => '' };
  if (p === 'shell') return { openPath: async () => '', openExternal: async () => {}, showItemInFolder: noop, beep: noop };
  if (p === 'screen') {
    const display = () => ({ workarea: { x: 0, y: 0, width: 1920, height: 1080 } });
    return { getDisplayMatching: display, getPrimaryDisplay: display };
  }
  if (p === 'nativeImage') return { createEmpty: () => ({ isEmpty: () => true }), createFromPath: () => ({ isEmpty: () => true }) };
  if (p === 'WebContentsView') return { on: noop, setBounds: noop, destroy: noop, setWebContents: noop };
  if (p === 'dialog') return {
    showMessageBox: async () => ({ response: 0 }),
    showSaveDialog: async () => ({}), showOpenDialog: async () => ({}), showErrorBox: noop,
  };
  if (p === 'net') return { fetch: async () => ({ ok: false, status: 404, json: async () => ({}) }), isOnline: () => false };
  if (p === 'ipcMain') return { handle: noop, on: noop };
  return noop;
} });
Module._load = function (req, ...rest) {
  if (req === 'electron') return electron;
  return origLoad.call(this, req, ...rest);
};

test('main.js загружается, реестр launcher разрешается', () => {
  require('../../main.js');
  const launcher = require('../../src/launcher');
  const names = Object.keys(launcher);
  assert.ok(names.length > 0, 'реестр не пуст');
  for (const n of names) {
    try {
      void launcher[n]; // геттер — вызов имени в main.js
    } catch (e) {
      assert.fail(`реестр launcher: ${n} не разрешается: ${e.message}`);
    }
  }
});
