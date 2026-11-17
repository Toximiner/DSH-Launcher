'use strict';
// Exercise the GUI start path without Electron or a live backend/profile.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const vm = require('node:vm');
const { EventEmitter } = require('node:events');

function load(file, mocks) {
  const output = { exports: {} };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '..', '..', 'src', file), 'utf8'), {
    module: output, require: (name) => Object.hasOwn(mocks, name) ? mocks[name] : require(name),
    console: { log() {} }, Buffer, process,
    setInterval: () => ({ unref() {} }), clearInterval() {},
  }, { filename: file });
  return output.exports;
}

test('GUI owned start spawns with bundled overlay, not by installing into the profile', (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-gui-start-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const args = ['--profile', 'web', '--patch', '/user/custom.yml', '--no-open'];
  const overlay = path.join(root, 'bundled.patch.json');
  const { backendModuleArgs } = require('../../src/backend-module');
  const state = { dshBin: 'fixture-dsh' };
  const calls = [], errors = [];
  let missing = false;
  const child = new EventEmitter();
  const pages = { showPage: (render) => render(), showStatus: (...value) => calls.push(['status', ...value]), stderrBlock: () => errors.join('\n') };
  const module = load('dsh-process.js', {
    child_process: { spawn: (...value) => { calls.push(['spawn', ...value]); return child; } },
    './backend-module': { backendModuleArgs, prepareBackendModule: () => { if (missing) throw new Error('Reinstall DSH Launcher'); return overlay; } },
    './state': state, './ui': {}, './i18n': { tr: (ru) => ru },
    './env': { DSH_ARGS: args, CWD: root, LOG_DIR: root, LOG_FILE: path.join(root, 'dsh.log'), PORT: 3080 },
    './log': { rotateLog: () => false, openDshLog: () => ({ end() {}, write() {} }) },
    './dsh-install': { dshEnv: (value) => value },
    './port': { pushErr: (value) => errors.push(value) }, './launch': {}, './pages': pages,
    './updates-live': {}, './backend-clients': {}, './system': {}, './launcher': {},
  });
  module.startDsh();
  const spawned = calls.find((value) => value[0] === 'spawn');
  assert.equal(spawned[1], 'fixture-dsh');
  assert.deepEqual(spawned[2], ['--profile', 'web', '--patch', '/user/custom.yml', '--patch', overlay, '--no-open']);
  assert.equal(spawned[3].cwd, root);
  assert.equal(state.weStartedDsh, true);
  child.emit('error', new Error('fixture cleanup'));
  missing = true;
  const before = calls.filter((value) => value[0] === 'spawn').length;
  assert.equal(module.startDsh(), false);
  assert.equal(calls.filter((value) => value[0] === 'spawn').length, before, 'missing assets do not spawn');
  assert.equal(state.dshFatal, true);
  assert.ok(errors.some((value) => value.includes('Переустановите DSH Launcher')));
  assert.ok(calls.some((value) => value[0] === 'status' && value[3] === true));
});

test('GUI module preparation failure returns before waiting for a backend port', async () => {
  let waited = 0, watchdogs = 0;
  const module = load('launch.js', {
    './state': { dshBin: 'fixture-dsh' }, './i18n': { tr: (ru) => ru }, './util': { esc: (text) => text },
    './env': { DSH_ARGS: ['--profile', 'web'], PLAIN_URL: 'http://127.0.0.1:3080/' },
    './port': { waitForPort: async () => { waited++; return true; } },
    './dsh-process': { startDsh: () => false }, './market': {},
    './pages': { showPage: (render) => render(), showStatus() {} },
    './updates-live': {}, './updates': {}, './launcher': { startWatchdog: () => watchdogs++ },
  });
  await module.launchDshInner();
  assert.equal(waited, 0);
  assert.equal(watchdogs, 1, 'foreign/manual backend detection remains available');
});
