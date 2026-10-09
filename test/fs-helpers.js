#!/usr/bin/env node
// Юнит-тесты помощников, работающих с файловой системой: writableOrCreatable
// (main.js) и pgidOf (main.js).
'use strict';
const path = require('path');
const fs = require('fs');
const os = require('os');
const assert = require('assert');
const { spawn } = require('child_process');
const { extract } = require('./_extract');

// ---- writableOrCreatable: W_OK у ближайшего существующего каталога ----
const writableOrCreatable = extract('writableOrCreatable', { fs, path });
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'dshl-test-'));
try {
  assert.strictEqual(writableOrCreatable(tmp), true, 'существующий writable-каталог → true');
  assert.strictEqual(writableOrCreatable(path.join(tmp, 'a/b/c')), true,
    'несуществующая цепочка → проверяем ближайший существующий (не ENOENT)');
  if (typeof process.getuid === 'function' && process.getuid() !== 0) {
    // root всё «может» — отрицательный кейс проверяем только от обычного юзера
    const ro = path.join(tmp, 'ro');
    fs.mkdirSync(ro);
    fs.chmodSync(ro, 0o555);
    try {
      assert.strictEqual(writableOrCreatable(path.join(ro, 'x/y')), false, 'read-only каталог → false');
      assert.strictEqual(writableOrCreatable(ro), false, 'read-only каталог (сам) → false');
    } finally {
      fs.chmodSync(ro, 0o755);
    }
  }
} finally {
  fs.rmSync(tmp, { recursive: true, force: true });
}

// ---- pgidOf: pgid из /proc/<pid>/stat ----
const pgidOf = extract('pgidOf', { fs });
{
  // Детаченый процесс — лидер собственной группы, у него pgid == свой pid.
  const child = spawn(process.execPath, ['-e', 'setTimeout(()=>{},3000)'],
    { detached: true, stdio: 'ignore' });
  child.unref();
  // Даём детаченному ребёнку время появиться в /proc (без busy-wait)
  const sab = new SharedArrayBuffer(4);
  Atomics.wait(new Int32Array(sab), 0, 0, 100);
  assert.strictEqual(pgidOf(child.pid), child.pid, 'лидер группы (detached) → pgid == свой pid');
  child.kill('SIGKILL');
}
assert.strictEqual(pgidOf(999999999), null, 'несуществующий pid → null');
assert.strictEqual(pgidOf(-1), null, 'отрицательный pid → null');

console.log('OK: fs-helpers — все проверки прошли');
