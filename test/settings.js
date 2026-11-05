#!/usr/bin/env node
// Unit tests for the "Settings" window (src/config.js): effectiveConfig — the final
// parameters from environment variables (higher priority) and settings.json;
// settingValue — validation of field values; restartNeeded — what changes after a restart.
'use strict';
const assert = require('assert');
const i18n = require('../src/i18n');
const { effectiveConfig: cfg, settingValue, restartNeeded } = require('../src/config');

i18n.setLang('ru');
const HOME = '/home/u';

// ---- effectiveConfig: defaults ----
{
  const c = cfg({}, {}, HOME);
  assert.deepStrictEqual(c.args, ['--profile', 'web', '--no-open'], 'default: web profile, no --port');
  assert.strictEqual(c.port, 3080);
  assert.strictEqual(c.dshBin, null, 'dsh path — auto-detect');
  assert.strictEqual(c.cwd, HOME, 'cwd — home dir');
  assert.deepStrictEqual([c.noGpu, c.ozone, c.checkUpdates, c.pollUpdates, c.sandbox], [false, 'auto', true, true, false], 'sandbox off by default');
  assert.ok(Object.values(c.src).every((v) => v === 'default'), 'all — default');
  assert.deepStrictEqual(cfg({}, null, HOME).args, c.args, 'no settings file');
  assert.deepStrictEqual(cfg({}, 'мусор', HOME).args, c.args, 'settings file is not an object');
}

// ---- effectiveConfig: from settings.json ----
{
  const c = cfg({}, { profile: 'work', port: 3090, dshPath: '/opt/dsh/bin/dsh', cwd: '/srv/p', gpu: 'off', ozone: 'x11', checkUpdates: false, pollUpdates: false, sandbox: true }, HOME);
  assert.deepStrictEqual(c.args, ['--profile', 'work', '--no-open', '--port', '3090'], 'profile and port are passed to dsh');
  assert.deepStrictEqual([c.port, c.dshBin, c.cwd, c.noGpu, c.ozone, c.checkUpdates],
    [3090, '/opt/dsh/bin/dsh', '/srv/p', true, 'x11', false]);
  assert.strictEqual(c.pollUpdates, false, 'every 12 hours — disabled');
  assert.strictEqual(c.sandbox, true, 'sandbox — from the checkbox');
  assert.ok(Object.values(c.src).every((v) => v === 'settings'), 'all — from the file');
  const onlyStart = cfg({}, { checkUpdates: false }, HOME);
  assert.deepStrictEqual([onlyStart.checkUpdates, onlyStart.pollUpdates], [false, true], 'checkboxes are independent: no check on startup');
  const onlyPoll = cfg({}, { pollUpdates: false }, HOME);
  assert.deepStrictEqual([onlyPoll.checkUpdates, onlyPoll.pollUpdates], [true, false], 'checkboxes are independent: no scheduled check');
}

// ---- effectiveConfig: garbage in the file is ignored ----
{
  const c = cfg({}, { profile: 'a b;rm', port: 'x', gpu: 'maybe', ozone: 'mir', dshPath: 5, checkUpdates: 'no' }, HOME);
  assert.deepStrictEqual(c.args, ['--profile', 'web', '--no-open'], 'bad profile/port — defaults');
  assert.deepStrictEqual([c.port, c.noGpu, c.ozone, c.dshBin, c.checkUpdates], [3080, false, 'auto', null, true]);
  assert.strictEqual(cfg({}, { port: 70000 }, HOME).port, 3080, 'port out of range');
}

// ---- effectiveConfig: environment variables win over the file ----
{
  const env = { DSH_PORT: '4000', DSH_ARGS: '--profile ci --no-open', DSH_BIN: '/e/dsh', DSH_CWD: '/e',
    DSH_LAUNCHER_NO_GPU: '1', DSH_LAUNCHER_OZONE: 'wayland', DSH_LAUNCHER_NO_UPDATE_CHECK: '1' };
  const c = cfg(env, { profile: 'work', port: 3090, dshPath: '/s/dsh', cwd: '/s', gpu: 'auto', ozone: 'x11', checkUpdates: true, pollUpdates: true }, HOME);
  assert.deepStrictEqual(c.args, ['--profile', 'ci', '--no-open'], 'DSH_ARGS — as-is (no --port added)');
  assert.deepStrictEqual([c.port, c.dshBin, c.cwd, c.noGpu, c.ozone, c.checkUpdates, c.pollUpdates], [4000, '/e/dsh', '/e', true, 'wayland', false, false],
    'DSH_LAUNCHER_NO_UPDATE_CHECK — neither on startup nor on schedule');
  assert.ok(Object.entries(c.src).every(([k, v]) => v === 'env' || k === 'sandbox'), 'all — from env (the sandbox has no env var)');
  assert.deepStrictEqual(cfg({ DSH_PORT: '4000' }, {}, HOME).args, ['--profile', 'web', '--no-open', '--port', '4000'],
    'DSH_PORT without DSH_ARGS — dsh listens on that port');
  assert.strictEqual(cfg({ DSH_LAUNCHER_OZONE: 'bogus' }, { ozone: 'x11' }, HOME).ozone, 'x11', 'invalid env var — take the file');
}

// ---- settingValue: field validation; restartNeeded ----

assert.deepStrictEqual(settingValue('port', ' 3090 '), { value: 3090 }, 'port');
for (const bad of ['80', '70000', 'abc', '', '30.5']) assert.ok(settingValue('port', bad).error, `port "${bad}" — error`);
assert.deepStrictEqual(settingValue('profile', 'my-profile_2.0'), { value: 'my-profile_2.0' }, 'profile');
for (const bad of ['', 'a b', 'x;rm -rf', '../etc', '.', '..', '...']) assert.ok(settingValue('profile', bad).error, `profile "${bad}" — error`);
assert.deepStrictEqual(settingValue('profile', '.web'), { value: '.web' }, 'leading dot in profile — allowed');
assert.ok(settingValue('port', null).error && settingValue('port', undefined).error, 'port: no value — error');
for (const [k, ok, bad] of [['gpu', 'off', 'on'], ['ozone', 'x11', 'xorg'], ['lang', 'en', 'de'], ['spellcheck', 'false', 'no']]) {
  assert.ok(!settingValue(k, ok).error, `${k}: "${ok}" — allowed`);
  assert.strictEqual(settingValue(k, bad).error, 'bad value', `${k}: "${bad}" — error`);
}
assert.ok(/unknown setting/.test(settingValue('nope', 'x').error), 'unknown setting');
assert.deepStrictEqual(settingValue('dshPath', '/usr/local/bin/dsh'), { value: '/usr/local/bin/dsh' }, 'path');
assert.deepStrictEqual(settingValue('dshPath', ''), { value: '' }, 'empty path — "auto"');
assert.ok(settingValue('cwd', 'relative/dir').error, 'relative dir — error');
assert.deepStrictEqual(settingValue('checkUpdates', 'false'), { value: false }, 'checkbox');
assert.deepStrictEqual(settingValue('spellcheck', 'true'), { value: true }, 'checkbox');
assert.deepStrictEqual(settingValue('pollUpdates', 'false'), { value: false }, 'checkbox');
assert.deepStrictEqual(settingValue('closeToTray', 'true'), { value: true }, '"close to tray" checkbox');
assert.deepStrictEqual(settingValue('attention', 'false'), { value: false }, '"attention" checkbox');
assert.deepStrictEqual(settingValue('attentionSound', 'true'), { value: true }, '"sound" checkbox');
assert.ok(settingValue('spellcheck', 'yes').error, 'checkbox — only true/false');
assert.ok(settingValue('market', 'true').error, 'no market checkbox in settings');
assert.deepStrictEqual(settingValue('lang', 'auto'), { value: 'auto' }, 'language');
assert.ok(settingValue('lang', 'de').error, 'unknown language');
assert.deepStrictEqual(settingValue('gpu', 'off'), { value: 'off' }, 'GPU');
assert.deepStrictEqual(settingValue('ozone', 'x11'), { value: 'x11' }, 'window mode');
assert.ok(settingValue('ozone', 'mir').error, 'unknown window mode');
assert.ok(settingValue('nope', '1').error, 'unknown setting');

const base = cfg({}, {}, HOME);
assert.deepStrictEqual(restartNeeded(base, cfg({}, {}, HOME)), [], 'nothing changed');
assert.deepStrictEqual(restartNeeded(base, cfg({}, { profile: 'w2' }, HOME)), ['args'], 'profile → restart');
assert.deepStrictEqual(restartNeeded(base, cfg({}, { port: 3091 }, HOME)).sort(), ['args', 'port'], 'port → restart');
assert.deepStrictEqual(restartNeeded(base, cfg({}, { gpu: 'off', ozone: 'x11', cwd: '/x', dshPath: '/d' }, HOME)).sort(),
  ['cwd', 'dshBin', 'noGpu', 'ozone'], 'GPU, window, dir, path → restart');
assert.deepStrictEqual(restartNeeded(base, cfg({}, { checkUpdates: false, pollUpdates: false }, HOME)), [], 'update check — no restart');
assert.deepStrictEqual(restartNeeded(base, cfg({}, { sandbox: true }, HOME)), ['sandbox'], 'sandbox → restart');
assert.ok(cfg({}, { sandbox: 'yes' }, HOME).sandbox === false, 'sandbox — only true');

console.log('OK: effectiveConfig, settingValue, restartNeeded — all checks passed');
