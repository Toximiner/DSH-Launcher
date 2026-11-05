'use strict';
/*
 * test/port.js — checks portListenerPid (used when stopping the
 * "adopted" dsh after self-resurrection) and listenerAuthUrl (the auth
 * link from the output file of the process on the port — dsh, restarted
 * by the market). Functions are from src/system.js. Run: node test/port.js
 * (or npm test). Linux only (/proc).
 */
const assert = require('assert');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawn } = require('child_process');

const { portListenerPid, listenerAuthUrl } = require('../src/system');

const PORT = 3199;
const PLAIN = `http://127.0.0.1:${PORT}/`;

// Listener on PORT; stdout — wherever it's pointed. Ready once it reported over IPC
// (the port is already listening, the auth line is already printed).
async function listener(stdout) {
  const child = spawn(process.execPath, ['-e', `
    require('http').createServer((q, s) => s.end('ok')).listen(${PORT}, '127.0.0.1', () => {
      console.log('starting…');
      console.log('dsh web: ${PLAIN}?token=t1');
      process.send('ready');
    });`], { stdio: ['ignore', stdout, 'ignore', 'ipc'] });
  await new Promise((resolve, reject) => { child.once('message', resolve); child.once('exit', reject); });
  return child;
}

async function stop(child) {
  const exited = new Promise((r) => child.once('exit', r));
  child.kill('SIGKILL');
  await exited;
}

async function main() {
  // 1) no listener → null
  assert.strictEqual(portListenerPid(PORT), null, 'no listener → null');
  assert.strictEqual(listenerAuthUrl(PORT, PLAIN), null, 'no listener → no link');

  // 2) listener with output to a file (like a restart via the market button)
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-port-'));
  const out = fs.openSync(path.join(dir, 'out.log'), 'w');
  let child = await listener(out);
  fs.closeSync(out);
  assert.strictEqual(portListenerPid(PORT), child.pid, 'portListenerPid — the pid of the child process');
  assert.strictEqual(listenerAuthUrl(PORT, PLAIN), `${PLAIN}?token=t1`, 'the auth link — from the output file');
  assert.strictEqual(listenerAuthUrl(PORT, 'http://127.0.0.1:3080/'), null, 'a link to a different port — not ours');
  await stop(child);
  assert.strictEqual(portListenerPid(PORT), null, 'after stopping the listener → null');

  // 3) output not to a file (/dev/null, terminal) → null
  child = await listener('ignore');
  assert.strictEqual(listenerAuthUrl(PORT, PLAIN), null, 'stdout is not a file → null');
  await stop(child);

  fs.rmSync(dir, { recursive: true, force: true });
  console.log('OK: portListenerPid, listenerAuthUrl — all checks passed');
}

main().catch((e) => { console.error('FAIL:', e.message); process.exit(1); });
