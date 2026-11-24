'use strict';
// prefs: the JSON settings files in userData (update-prefs.json and friends).
// src/prefs.js is not loaded whole (it requires electron): the marker blocks
// run with a fake `app` and the real fs in a temp directory.
const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { extract } = require('./_extract');

test('prefs: JSON settings in userData', (t) => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-launcher-prefs-'));
  t.after(() => { try { fs.rmSync(tmp, { recursive: true, force: true }); } catch { /* temp dir */ } });
  const app = { getPath: () => tmp };
  const prefFile = extract('prefFile', { app, path });
  const readPref = extract('readPref', { fs, prefFile });
  const writePref = extract('writePref', { fs, path, prefFile });

  assert.strictEqual(prefFile('update-prefs.json'), path.join(tmp, 'update-prefs.json'), 'the file path in userData');

  assert.strictEqual(readPref('update-prefs.json'), null, 'a missing file → null');
  fs.writeFileSync(path.join(tmp, 'bad.json'), '{oops');
  assert.strictEqual(readPref('bad.json'), null, 'a corrupted JSON → null');

  assert.strictEqual(writePref('update-prefs.json', { port: 3081 }), true, 'the write succeeded');
  assert.deepStrictEqual(readPref('update-prefs.json'), { port: 3081 }, 'the write/read round-trip');

  // a directory that is a file: mkdirSync fails — false, no exception
  fs.writeFileSync(path.join(tmp, 'block'), 'x');
  assert.strictEqual(writePref('block/nested.json', { a: 1 }), false, 'a write error → false');
});
