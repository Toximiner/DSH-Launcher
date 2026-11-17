'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { backendModuleArgs, prepareBackendModule } = require('../src/backend-module');

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-launcher-module-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const assetDir = path.join(root, 'assets with spaces 世界');
  const dataDir = path.join(root, 'private launcher data');
  fs.mkdirSync(assetDir);
  fs.writeFileSync(path.join(assetDir, 'index.mjs'), 'export const inject=[]; export function apply() {}\n');
  fs.writeFileSync(path.join(assetDir, 'session-delete.mjs'), 'export function deleteSession() {}\n');
  return { root, assetDir, dataDir };
}

test('backend overlay preserves custom arguments and patch order before app flags', () => {
  const overlay = '/private modules/launcher.patch.json';
  for (const [args, expected] of [
    [[], ['--patch', overlay]],
    [['--profile', 'web', '--no-open', '--port', '3080'], ['--profile', 'web', '--patch', overlay, '--no-open', '--port', '3080']],
    [['--patch', 'user.yml', '--profile=custom', '--patch=second.yml', '--', '--port', '4020'], ['--patch', 'user.yml', '--profile=custom', '--patch=second.yml', '--patch', overlay, '--', '--port', '4020']],
    [['--from-default-profile', 'web', '--profile', 'custom', '--dump-config'], ['--from-default-profile', 'web', '--profile', 'custom', '--dump-config', '--patch', overlay]],
    [['--no-open', '--patch', 'app-owned'], ['--patch', overlay, '--no-open', '--patch', 'app-owned']],
  ]) {
    const original = [...args];
    assert.deepEqual(backendModuleArgs(args, overlay), expected);
    assert.deepEqual(args, original, 'user argument array is immutable');
  }
});

test('backend assets and overlay are immutable private copies outside the dsh profile', (t) => {
  const options = fixture(t);
  const profile = path.join(options.root, 'profile');
  fs.mkdirSync(profile);
  fs.writeFileSync(path.join(profile, 'cordis.patch.yml'), 'user-owned');
  const overlay = prepareBackendModule(options);
  const directory = path.dirname(overlay);
  assert.ok(overlay.startsWith(options.dataDir + path.sep));
  assert.deepEqual(JSON.parse(fs.readFileSync(overlay, 'utf8')), [{ insert: [{ id: 'dsh-launcher-session-management', name: pathToFileURL(path.join(directory, 'index.mjs')).href }] }]);
  for (const name of ['index.mjs', 'session-delete.mjs']) {
    assert.deepEqual(fs.readFileSync(path.join(directory, name)), fs.readFileSync(path.join(options.assetDir, name)));
    assert.equal(fs.statSync(path.join(directory, name)).mode & 0o777, 0o600);
  }
  assert.equal(fs.statSync(directory).mode & 0o777, 0o700);
  assert.equal(prepareBackendModule(options), overlay, 'same content reuses the existing complete version');
  fs.writeFileSync(path.join(options.assetDir, 'session-delete.mjs'), 'export function deleteSession() { return 2; }\n');
  const updated = prepareBackendModule(options);
  assert.notEqual(updated, overlay, 'new source gets a new immutable version');
  assert.ok(fs.existsSync(overlay), 'old running backend keeps its assets');
  assert.equal(fs.readFileSync(path.join(profile, 'cordis.patch.yml'), 'utf8'), 'user-owned');
});

test('missing or altered backend assets fail before returning a launch overlay', (t) => {
  const options = fixture(t);
  const overlay = prepareBackendModule(options);
  fs.writeFileSync(path.join(path.dirname(overlay), 'index.mjs'), 'altered');
  assert.throws(() => prepareBackendModule(options), /cache changed/);
  fs.rmSync(path.join(options.assetDir, 'index.mjs'));
  assert.throws(() => prepareBackendModule(options), /Reinstall DSH Launcher/);
});

test('overlay preparation rejects links in the immutable asset cache', (t) => {
  const options = fixture(t);
  const overlay = prepareBackendModule(options);
  const target = path.join(path.dirname(overlay), 'session-delete.mjs');
  fs.rmSync(target);
  fs.symlinkSync(path.join(options.assetDir, 'session-delete.mjs'), target);
  assert.throws(() => prepareBackendModule(options), /cache changed/);
});

test('concurrent launchers publish one complete immutable module directory', async (t) => {
  const options = fixture(t);
  const { execFile } = require('node:child_process');
  const childEnv = { ...process.env };
  delete childEnv.NODE_TEST_CONTEXT;
  const run = (name) => new Promise((resolve, reject) => {
    const marker = path.join(options.root, name);
    execFile(process.execPath, ['-e', 'require("node:fs").writeFileSync(process.argv[3], require(process.argv[1]).prepareBackendModule(JSON.parse(process.argv[2])))', require.resolve('../src/backend-module'), JSON.stringify(options), marker], { env: childEnv }, (error) => error ? reject(error) : resolve(fs.readFileSync(marker, 'utf8')));
  });
  const [first, second] = await Promise.all([run('first'), run('second')]);
  assert.equal(first, second);
  assert.deepEqual(fs.readdirSync(path.dirname(first)).sort(), ['index.mjs', 'launcher.patch.json', 'session-delete.mjs']);
  assert.equal(prepareBackendModule(options), first);
});
