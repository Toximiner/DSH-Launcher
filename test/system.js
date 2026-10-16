'use strict';

/*
 * src/system.js — чистый модуль (fs/path, без electron) — подключается
 * напрямую. Самое важное здесь — pickKillTargets: кто получает SIGTERM
 * при остановке (группа чужого dsh vs pid в нашей группе vs «старая»
 * группа после самовозрождения).
 */

const assert = require('assert');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawn } = require('child_process');
const { test } = require('node:test');

const sys = require('../src/system');

test('envWithBinDir: каталог бинарника — первым в PATH, extra-переменные добавляются', () => {
  const env = sys.envWithBinDir('/opt/foo/bin/dsh', { FOO: '1' });
  assert.ok(env.PATH.startsWith('/opt/foo/bin' + path.delimiter), 'каталог бинарника первым');
  assert.ok(env.PATH.endsWith(process.env.PATH || ''), 'остаток PATH сохранён');
  assert.strictEqual(env.FOO, '1');
});

test('withoutUnitEnv: переменные systemd-юнита вычищены, остальное нет, вход не мутируется', () => {
  const env = { INVOCATION_ID: 'x', LISTEN_FDS: '1', PATH: '/usr/bin', HOME: '/h' };
  const out = sys.withoutUnitEnv(env);
  for (const k of sys.SYSTEMD_UNIT_ENV) assert.strictEqual(out[k], undefined, k);
  assert.strictEqual(out.PATH, '/usr/bin');
  assert.strictEqual(out.HOME, '/h');
  assert.strictEqual(env.INVOCATION_ID, 'x', 'вход не мутируется');
  assert.notStrictEqual(out, env, 'возвращена копия');
});

test('writableOrCreatable: существующий каталог и несуществующий путь внутри него', () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-sys-'));
  try {
    assert.ok(sys.writableOrCreatable(tmp), 'существующий каталог');
    assert.ok(sys.writableOrCreatable(path.join(tmp, 'sub', 'dir')), 'несуществующий путь под ним (можно создать)');
    if (typeof process.getuid === 'function' && process.getuid() !== 0) {
      assert.ok(!sys.writableOrCreatable('/proc/1/root/нет'), 'чужой корень /proc — не наш');
    }
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

test('groupAlive и pgidOf: живой и несуществующий pid', () => {
  const pid = process.pid;
  assert.ok(sys.groupAlive(pid), 'свой процесс жив');
  assert.ok(!sys.groupAlive(999999999), 'несуществующий pid');
  const pgid = sys.pgidOf(pid);
  assert.ok(Number.isInteger(pgid) && pgid > 0, 'pgid своего процесса');
  assert.strictEqual(sys.pgidOf(999999999), null, 'pgid несуществующего процесса');
  // группа: kill(-pid) сработает, только если процесс — лидер группы
  assert.strictEqual(sys.groupAlive(pid, true), pgid === pid, 'groupOnly — именно лидерство');
});

test('portListenerPid: pid процесса, слушающего порт (по /proc/net/tcp и /proc/*/fd)', async () => {
  if (process.platform !== 'linux') return; // проект Linux-only
  const child = spawn(process.execPath, ['-e',
    'const s = require("net").createServer();' +
    's.listen(0, "127.0.0.1", () => { console.log(s.address().port); setInterval(() => {}, 1000); });']);
  let port = null;
  child.stdout.on('data', (d) => { if (port === null) port = Number(String(d).trim()); });
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  try {
    for (let i = 0; i < 200 && port === null; i++) await sleep(25);
    assert.ok(port, 'дочерний сервер сообщил свой порт');
    let found = null;
    for (let i = 0; i < 200 && !found; i++) { await sleep(25); found = sys.portListenerPid(port); }
    assert.strictEqual(found, child.pid, 'pid слушателя найден');
    child.kill('SIGKILL');
    await child.exit;
    let gone = sys.portListenerPid(port);
    for (let i = 0; i < 200 && gone; i++) { await sleep(25); gone = sys.portListenerPid(port); }
    assert.strictEqual(gone, null, 'после смерти слушателя — null');
  } finally {
    child.kill('SIGKILL');
  }
});

test('authUrlFromOutput: последняя ссылка входа с токеном на своём origin', () => {
  const plain = 'http://127.0.0.1:3080/';
  assert.strictEqual(
    sys.authUrlFromOutput('шум\ndsh web: http://127.0.0.1:3080/?token=abc\nещё\n', plain),
    'http://127.0.0.1:3080/?token=abc');
  assert.strictEqual(
    sys.authUrlFromOutput('dsh web: http://127.0.0.1:3080/?token=old\ndsh web: http://127.0.0.1:3080/?token=new\n', plain),
    'http://127.0.0.1:3080/?token=new', 'несколько ссылок — последняя');
  assert.strictEqual(sys.authUrlFromOutput('dsh web: http://10.0.0.5:3080/?token=abc\n', plain), null, 'чужой origin — не наш');
  assert.strictEqual(sys.authUrlFromOutput('dsh web: http://127.0.0.1:3080/\n', plain), null, 'без токена');
  assert.strictEqual(sys.authUrlFromOutput('dsh web: http://:80/\n', plain), null, 'обрывок/мусор — без падения');
  assert.strictEqual(sys.authUrlFromOutput('', plain), null, 'пусто');
});

test('listenerAuthUrl: слушателя нет — null без падения', () => {
  assert.strictEqual(sys.listenerAuthUrl(39999, 'http://127.0.0.1:3080/'), null);
});

test('pickKillTargets: кто получает сигнал при остановке', () => {
  assert.deepStrictEqual(sys.pickKillTargets(null, null, 1000, null), [], 'ничего не запущено');
  assert.deepStrictEqual(sys.pickKillTargets(null, null, 1000, 900), [{ id: 900, groupOnly: true }], 'старая группа — если чужая');
  assert.deepStrictEqual(sys.pickKillTargets(null, null, 1000, 1000), [], 'старая группа = наша — не трогаем');
  assert.deepStrictEqual(sys.pickKillTargets(55, 66, 1000, null), [{ id: 66, groupOnly: true }], 'чужая группа процесса порта — вся группа');
  assert.deepStrictEqual(sys.pickKillTargets(55, 1000, 1000, null), [{ id: 55, groupOnly: false }], 'порт в нашей группе — только pid (иначе убьём лаунчер)');
  assert.deepStrictEqual(sys.pickKillTargets(55, null, 1000, null), [{ id: 55, groupOnly: false }], 'pgid неизвестен — только pid');
  assert.deepStrictEqual(sys.pickKillTargets(55, 66, 1000, 77), [{ id: 66, groupOnly: true }, { id: 77, groupOnly: true }], 'плюс старая группа');
  assert.deepStrictEqual(sys.pickKillTargets(55, 66, 1000, 66), [{ id: 66, groupOnly: true }], 'старая группа = группа порта — без дубля');
  assert.deepStrictEqual(sys.pickKillTargets(55, 1000, 1000, 55), [{ id: 55, groupOnly: false }], 'старая группа = pid в нашей группе — без дубля');
});
