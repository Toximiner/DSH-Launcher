'use strict';
// Smoke: main.js loads in full with electron stubbed out, and every name in the
// launcher registry (Object.defineProperties in main.js) resolves. Catches a lost
// import, a broken registry, and top-level module errors that the unit tests
// (pure functions) don't see. No live window is raised: the stub's whenReady never resolves.
// A separate test/smoke/ folder: not picked up by the global npm run coverage (test/*.js),
// otherwise main.js's graph would dilute the coverage report.
const { test } = require('node:test');
const assert = require('assert');
const Module = require('module');
const fs = require('fs');
const os = require('os');
const path = require('path');

const TMP = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-launcher-smoke-'));
// BEFORE requiring main.js: env.js computes LOG_DIR from DSH_LOG_DIR
process.env.DSH_LOG_DIR = TMP;

const origLoad = Module._load;
const noop = () => {};
// Electron stub: only what main.js and src/* touch at load time.
const electron = new Proxy({}, { get: (_t, p) => {
  if (p === 'app') return {
    getName: () => 'dsh-launcher', setName: noop,
    requestSingleInstanceLock: () => true, on: noop,
    whenReady: () => new Promise(() => {}), // never resolves — no window is raised
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

test('main.js loads, the launcher registry resolves', () => {
  require('../../main.js');
  const launcher = require('../../src/launcher');
  const names = Object.keys(launcher);
  assert.ok(names.length > 0, 'registry is not empty');
  for (const n of names) {
    try {
      void launcher[n]; // the getter — calls the name in main.js
    } catch (e) {
      assert.fail(`launcher registry: ${n} does not resolve: ${e.message}`);
    }
  }
});
