'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawn } = require('node:child_process');
const { once } = require('node:events');
const { identity, alive, backendDirectory, rememberBackend, joinBackend } = require('../src/backend-clients');

test('backend leases preserve live peers, discard stale PIDs and only target managed groups', async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-clients-'));
  const previous = process.env.XDG_DATA_HOME;
  process.env.XDG_DATA_HOME = root;
  const backend = spawn(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], { detached: true, stdio: 'ignore' });
  try {
    await once(backend, 'spawn');
    assert.equal(identity(0), null);
    assert.equal(identity(999999999), null);
    assert.equal(alive(null), false);
    const current = identity(process.pid);
    assert.ok(alive(current));
    assert.equal(alive({ ...current, start: '0' }), false);
    assert.throws(() => rememberBackend(32100, 999999999));
    const first = joinBackend(32100);
    const second = joinBackend(32100);
    assert.equal(first.target(), null, 'unmanaged processes are not targets');
    rememberBackend(32100, backend.pid, current.group);
    rememberBackend(32100, backend.pid, current.group);
    assert.equal(first.target(), backend.pid);
    assert.deepEqual(first.targets(), [backend.pid]);
    assert.equal(first.release(), false);
    assert.equal(first.release(), false, 'later fallback keeps the original decision');
    const dir = backendDirectory(32100);
    fs.writeFileSync(path.join(dir, 'clients', 'abandoned.tmp'), '{');
    fs.writeFileSync(path.join(dir, 'clients', 'stale.json'), JSON.stringify({ pid: 999999999, start: '1', group: 999999999 }));
    assert.equal(second.release(), true);
    assert.equal(second.release(), true);
    assert.deepEqual(fs.readdirSync(path.join(dir, 'clients')), ['abandoned.tmp']);
    fs.writeFileSync(path.join(dir, 'managed.json'), JSON.stringify({ ...identity(backend.pid), start: '0' }));
    assert.equal(second.target(), null, 'reused PIDs must never be signalled');
    assert.deepEqual(second.targets(), []);
    fs.writeFileSync(path.join(dir, 'managed.json'), '{');
    assert.equal(second.target(), null);
    const third = joinBackend(32100);
    fs.writeFileSync(path.join(dir, 'clients', 'incomplete.json'), '{');
    assert.equal(third.release(), false, 'unreadable concurrent lease fails closed');
  } finally {
    backend.kill();
    await once(backend, 'exit');
    if (previous === undefined) delete process.env.XDG_DATA_HOME;
    else process.env.XDG_DATA_HOME = previous;
    fs.rmSync(root, { recursive: true, force: true });
  }
});
