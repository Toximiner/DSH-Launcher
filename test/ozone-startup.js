'use strict';
const assert = require('assert');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execFileSync } = require('child_process');
const { startupOzone, restartLauncherArgs } = require('../src/ozone-startup');

const wayland = { XDG_SESSION_TYPE: 'wayland' };
for (const argv of [['--ozone-platform=x11'], ['--ozone-platform', 'x11'], ['--ozone-platform=wayland']]) {
  assert.strictEqual(startupOzone(argv, { ...wayland, DSH_LAUNCHER_OZONE: 'wayland' }, { ozone: 'x11' }, 'linux', true), '', 'explicit CLI passes through unchanged');
}
assert.strictEqual(startupOzone([], wayland, {}, 'linux', true), 'wayland');
assert.strictEqual(startupOzone([], { WAYLAND_DISPLAY: 'wayland-0' }, {}, 'linux', false), 'x11');
assert.strictEqual(startupOzone([], {}, {}, 'linux', true), '');
assert.strictEqual(startupOzone([], wayland, {}, 'darwin', true), '');
assert.strictEqual(startupOzone([], wayland, { ozone: 'x11' }, 'linux', true), 'x11');
assert.strictEqual(startupOzone([], {}, { ozone: 'wayland' }, 'linux', false), 'wayland');
assert.strictEqual(startupOzone([], { ...wayland, DSH_LAUNCHER_OZONE: 'x11' }, { ozone: 'wayland' }, 'linux', true), 'x11');
assert.strictEqual(startupOzone([], { ...wayland, DSH_LAUNCHER_OZONE: 'invalid' }, { ozone: 'invalid' }, 'linux', false), 'x11');

const initial = ['electron', '/app', '--no-sandbox', '--ozone-platform=x11', '--remote-debugging-port=9222', '--user-data-dir=/a b'];
assert.deepStrictEqual(restartLauncherArgs(initial, 'x11'), ['--remote-debugging-port=9222', '--user-data-dir=/a b']);
assert.deepStrictEqual(restartLauncherArgs(initial, undefined), initial.slice(3), 'keep explicit override through restart');
assert.deepStrictEqual(restartLauncherArgs(['electron', '/app', '--ozone-platform', 'wayland'], ''), ['--ozone-platform', 'wayland']);
// A settings restart removes the generated mode, so the updated preference wins.
assert.strictEqual(startupOzone(restartLauncherArgs(initial, 'x11'), wayland, { ozone: 'wayland' }, 'linux', true), 'wayland');

// Exercise both real shell entrypoints with a fake Electron. Its Node preflight
// is real; the final exec records arguments instead of opening a desktop window.
const root = path.join(__dirname, '..');
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-ozone-'));
try {
  const config = path.join(tmp, 'config with spaces');
  fs.mkdirSync(path.join(config, 'dsh-launcher'), { recursive: true });
  const settings = path.join(config, 'dsh-launcher/settings.json');
  fs.writeFileSync(settings, JSON.stringify({ ozone: 'x11' }));
  for (const entry of ['run.sh', 'deb/run.sh']) {
    const app = path.join(tmp, entry === 'run.sh' ? 'dev app' : 'installed app');
    const binary = path.join(app, entry === 'run.sh' ? 'node_modules/electron/dist/electron' : 'electron/electron');
    fs.mkdirSync(path.dirname(binary), { recursive: true });
    fs.symlinkSync(path.join(root, 'src'), path.join(app, 'src'));
    fs.copyFileSync(path.join(root, entry), path.join(app, 'run.sh'));
    fs.writeFileSync(binary, '#!/bin/sh\nif [ "$ELECTRON_RUN_AS_NODE" = 1 ]; then exec node "$@"; fi\nprintf "auto=%s\\n" "$DSH_LAUNCHER_AUTO_OZONE"\nprintf "%s\\n" "$@"\n', { mode: 0o755 });
    const env = { ...process.env, XDG_CONFIG_HOME: config, DSH_LOG_DIR: path.join(tmp, 'logs'), DSH_LAUNCHER_OZONE: '', XDG_SESSION_TYPE: 'wayland' };
    const run = (args = [], overrides = {}) => execFileSync('/bin/sh', [path.join(app, 'run.sh'), ...args], { env: { ...env, ...overrides }, encoding: 'utf8' });
    assert.ok(run().includes('--ozone-platform=x11\n'), `${entry}: settings applied before native startup`);
    const explicit = run(['--ozone-platform=wayland', '--user-data-dir=/a b']);
    assert.ok(explicit.includes('--ozone-platform=wayland\n'));
    assert.strictEqual((explicit.match(/--ozone-platform=/g) || []).length, 1, 'no conflicting platform switches');
    assert.ok(explicit.includes('auto=\n'), 'explicit override is not marked automatic');
    assert.ok(explicit.includes('--user-data-dir=/a b\n'), 'arguments retain spaces');
    assert.ok(run([], { DSH_LAUNCHER_OZONE: 'wayland' }).includes('--ozone-platform=wayland\n'), 'environment overrides settings');
    fs.writeFileSync(settings, '{invalid');
    assert.doesNotThrow(() => run(), 'invalid settings do not break startup');
    fs.writeFileSync(settings, JSON.stringify({ ozone: 'x11' }));
  }
} finally {
  fs.rmSync(tmp, { recursive: true, force: true });
}
console.log('OK: ozone startup — precedence, early shell selection and settings restart');
