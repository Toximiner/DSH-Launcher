'use strict';

/*
 * src/system.js — a pure module (fs/path, no electron) — required
 * directly. The most important thing here — pickKillTargets: who receives SIGTERM
 * on stop (a foreign dsh's group vs a pid in our group vs the "old"
 * group after self-resurrection).
 */

const assert = require('assert');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawn } = require('child_process');
const { test } = require('node:test');

const sys = require('../src/system');

test('envWithBinDir: binary dir — first in PATH, extra vars are added', () => {
  const env = sys.envWithBinDir('/opt/foo/bin/dsh', { FOO: '1' });
  assert.ok(env.PATH.startsWith('/opt/foo/bin' + path.delimiter), 'binary dir first');
  assert.ok(env.PATH.endsWith(process.env.PATH || ''), 'rest of PATH preserved');
  assert.strictEqual(env.FOO, '1');
});

test('withoutUnitEnv: systemd unit vars cleaned, the rest not, input not mutated', () => {
  const env = { INVOCATION_ID: 'x', LISTEN_FDS: '1', PATH: '/usr/bin', HOME: '/h' };
  const out = sys.withoutUnitEnv(env);
  for (const k of sys.SYSTEMD_UNIT_ENV) assert.strictEqual(out[k], undefined, k);
  assert.strictEqual(out.PATH, '/usr/bin');
  assert.strictEqual(out.HOME, '/h');
  assert.strictEqual(env.INVOCATION_ID, 'x', 'input not mutated');
  assert.notStrictEqual(out, env, 'a copy is returned');
});

test('writableOrCreatable: an existing dir and a non-existent path inside it', () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-sys-'));
  try {
    assert.ok(sys.writableOrCreatable(tmp), 'existing dir');
    assert.ok(sys.writableOrCreatable(path.join(tmp, 'sub', 'dir')), 'non-existent path under it (creatable)');
    if (typeof process.getuid === 'function' && process.getuid() !== 0) {
      assert.ok(!sys.writableOrCreatable('/proc/1/root/нет'), 'foreign /proc root — not ours');
    }
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

test('groupAlive and pgidOf: a live and a non-existent pid', () => {
  const pid = process.pid;
  assert.ok(sys.groupAlive(pid), 'own process is alive');
  assert.ok(!sys.groupAlive(999999999), 'non-existent pid');
  const pgid = sys.pgidOf(pid);
  assert.ok(Number.isInteger(pgid) && pgid > 0, 'pgid of own process');
  assert.strictEqual(sys.pgidOf(999999999), null, 'pgid of a non-existent process');
  // group: kill(-pid) works only if the process is the group leader
  assert.strictEqual(sys.groupAlive(pid, true), pgid === pid, 'groupOnly — precisely leadership');
});

test('portListenerPid: pid of the process listening on the port (via /proc/net/tcp and /proc/*/fd)', async () => {
  if (process.platform !== 'linux') return; // the project is Linux-only
  const child = spawn(process.execPath, ['-e',
    'const s = require("net").createServer();' +
    's.listen(0, "127.0.0.1", () => { console.log(s.address().port); setInterval(() => {}, 1000); });']);
  let port = null;
  child.stdout.on('data', (d) => { if (port === null) port = Number(String(d).trim()); });
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  try {
    for (let i = 0; i < 200 && port === null; i++) await sleep(25);
    assert.ok(port, 'child server reported its port');
    let found = null;
    for (let i = 0; i < 200 && !found; i++) { await sleep(25); found = sys.portListenerPid(port); }
    assert.strictEqual(found, child.pid, 'listener pid found');
    child.kill('SIGKILL');
    await child.exit;
    let gone = sys.portListenerPid(port);
    for (let i = 0; i < 200 && gone; i++) { await sleep(25); gone = sys.portListenerPid(port); }
    assert.strictEqual(gone, null, 'after the listener dies — null');
  } finally {
    child.kill('SIGKILL');
  }
});

test('authUrlFromOutput: the last auth link with a token on its own origin', () => {
  const plain = 'http://127.0.0.1:3080/';
  assert.strictEqual(
    sys.authUrlFromOutput('шум\ndsh web: http://127.0.0.1:3080/?token=abc\nещё\n', plain),
    'http://127.0.0.1:3080/?token=abc');
  assert.strictEqual(
    sys.authUrlFromOutput('dsh web: http://127.0.0.1:3080/?token=old\ndsh web: http://127.0.0.1:3080/?token=new\n', plain),
    'http://127.0.0.1:3080/?token=new', 'several links — the last one');
  assert.strictEqual(sys.authUrlFromOutput('dsh web: http://10.0.0.5:3080/?token=abc\n', plain), null, 'foreign origin — not ours');
  assert.strictEqual(sys.authUrlFromOutput('dsh web: http://127.0.0.1:3080/\n', plain), null, 'without a token');
  assert.strictEqual(sys.authUrlFromOutput('dsh web: http://:80/\n', plain), null, 'fragment/garbage — no crash');
  assert.strictEqual(sys.authUrlFromOutput('', plain), null, 'empty');
});

test('listenerAuthUrl: no listener — null without a crash', () => {
  assert.strictEqual(sys.listenerAuthUrl(39999, 'http://127.0.0.1:3080/'), null);
});

test('pickKillTargets: who gets the signal on stop', () => {
  assert.deepStrictEqual(sys.pickKillTargets(null, null, 1000, null), [], 'nothing is running');
  assert.deepStrictEqual(sys.pickKillTargets(null, null, 1000, 900), [{ id: 900, groupOnly: true }], 'old group — if foreign');
  assert.deepStrictEqual(sys.pickKillTargets(null, null, 1000, 1000), [], 'old group = ours — leave it alone');
  assert.deepStrictEqual(sys.pickKillTargets(55, 66, 1000, null), [{ id: 66, groupOnly: true }], 'foreign group of the port process — the whole group');
  assert.deepStrictEqual(sys.pickKillTargets(55, 1000, 1000, null), [{ id: 55, groupOnly: false }], 'port in our group — pid only (otherwise we kill the launcher)');
  assert.deepStrictEqual(sys.pickKillTargets(55, null, 1000, null), [{ id: 55, groupOnly: false }], 'pgid unknown — pid only');
  assert.deepStrictEqual(sys.pickKillTargets(55, 66, 1000, 77), [{ id: 66, groupOnly: true }, { id: 77, groupOnly: true }], 'plus the old group');
  assert.deepStrictEqual(sys.pickKillTargets(55, 66, 1000, 66), [{ id: 66, groupOnly: true }], 'old group = port group — no duplicate');
  assert.deepStrictEqual(sys.pickKillTargets(55, 1000, 1000, 55), [{ id: 55, groupOnly: false }], 'old group = pid in our group — no duplicate');
});
