#!/usr/bin/env node
// Unit tests of the file-system helpers: writableOrCreatable (main.js)
// and pgidOf (main.js).
'use strict';
const path = require('path');
const fs = require('fs');
const os = require('os');
const assert = require('assert');
const { spawn } = require('child_process');

// ---- writableOrCreatable: W_OK of the nearest existing directory ----
const { writableOrCreatable, pgidOf } = require('../src/system');
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'dshl-test-'));
try {
  assert.strictEqual(writableOrCreatable(tmp), true, 'existing writable directory → true');
  assert.strictEqual(writableOrCreatable(path.join(tmp, 'a/b/c')), true,
    'nonexistent chain → check the nearest existing directory (no ENOENT)');
  if (typeof process.getuid === 'function' && process.getuid() !== 0) {
    // root can "do everything" — we check the negative case only as an ordinary user
    const ro = path.join(tmp, 'ro');
    fs.mkdirSync(ro);
    fs.chmodSync(ro, 0o555);
    try {
      assert.strictEqual(writableOrCreatable(path.join(ro, 'x/y')), false, 'read-only directory → false');
      assert.strictEqual(writableOrCreatable(ro), false, 'read-only directory (itself) → false');
    } finally {
      fs.chmodSync(ro, 0o755);
    }
  }
} finally {
  fs.rmSync(tmp, { recursive: true, force: true });
}

// ---- pgidOf: the pgid from /proc/<pid>/stat ----
{
  // A detached process — the leader of its own group, its pgid == its pid.
  const child = spawn(process.execPath, ['-e', 'setTimeout(()=>{},3000)'],
    { detached: true, stdio: 'ignore' });
  child.unref();
  // We give the detached child time to appear in /proc (without a busy-wait)
  const sab = new SharedArrayBuffer(4);
  Atomics.wait(new Int32Array(sab), 0, 0, 100);
  assert.strictEqual(pgidOf(child.pid), child.pid, 'group leader (detached) → pgid == its own pid');
  child.kill('SIGKILL');
}
assert.strictEqual(pgidOf(999999999), null, 'nonexistent pid → null');
assert.strictEqual(pgidOf(-1), null, 'negative pid → null');

console.log('OK: fs-helpers — all checks passed');
