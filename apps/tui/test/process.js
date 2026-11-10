'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { DshProcess } = require('../src/process');

test('buffers split login output, reaps its own process group, and leaves an attachment alone', async t => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-tui-test-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const bin = path.join(dir, 'dsh');
  fs.writeFileSync(bin, `#!/usr/bin/env node
process.stdout.write('dsh we');
setTimeout(() => process.stdout.write('b: http://127.0.0.1:43080/?token=split-token\\n'), 15);
setInterval(() => {}, 1000);
`, { mode: 0o755 });
  const server = new DshProcess();
  t.after(() => server.stop());
  assert.equal(await server.start({ bin, cwd: dir, profile: 'web', port: 43080 }), 'http://127.0.0.1:43080/?token=split-token');
  const pid = server.child.pid;
  await server.stop();
  assert.throws(() => process.kill(pid, 0), { code: 'ESRCH' });
  await new DshProcess().stop();
});

test('reports missing dsh executables without hanging', async () => {
  const server = new DshProcess();
  await assert.rejects(server.start({ bin: '/does-not-exist/dsh', cwd: '/tmp', profile: 'web', port: 3080 }), /Cannot start dsh/);
  await server.stop();
});
