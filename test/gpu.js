'use strict';
/*
 * test/gpu.js — onChildProcessGone (src/gpu.js): two GPU process deaths in
 * a session = the driver is broken — we write the gpu-disabled.marker,
 * the next starts go into software rendering. The function is extracted
 * by markers (test/_extract.js); fs/console/constants are injected, so we
 * don't touch the real disk and logs.
 * Run: node test/gpu.js (or npm test).
 */
const assert = require('assert');
const { EventEmitter } = require('events');
const { extract } = require('./_extract');

const MARKER = '/tmp/gpu-marker-test';

function makeDeps() {
  const written = [];
  const logs = [];
  return {
    written,
    logs,
    state: { launcherUpdatedOnDisk: false },
    fs: {
      mkdirSync: (dir, opts) => { written.push(['mkdir', dir, opts]); },
      writeFileSync: (file, content) => { written.push(['write', file, content]); },
    },
    console: { log: (m) => logs.push(m), error: (m) => logs.push(m) },
    GPU_MARKER_FILE: MARKER,
    gpuDisabledByMarker: false,
    LOG_DIR: '/tmp',
  };
}

async function main() {
  // 1) Two GPU process crashes → the marker is written, the text has the reason and the date.
  let d = makeDeps();
  let onGone = extract('onChildProcessGone', d);
  let restarts = 0;
  const app = new EventEmitter();
  onGone(app, () => { restarts += 1; });
  app.emit('child-process-gone', {}, { type: 'GPU', reason: 'crashed', exitCode: 11 });
  assert.strictEqual(d.written.length, 0, 'one crash — no marker yet');
  app.emit('child-process-gone', {}, { type: 'GPU', reason: 'abnormal-exit', exitCode: 42 });
  assert.strictEqual(d.written.length, 2, 'second crash — mkdir + marker write');
  assert.ok(d.written[0][0] === 'mkdir' && d.written[0][1] === '/tmp', 'LOG_DIR created (recursive)');
  assert.ok(d.written[0][2] && d.written[0][2].recursive === true, 'mkdir recursive');
  assert.strictEqual(d.written[1][0], 'write');
  assert.strictEqual(d.written[1][1], MARKER, 'marker in GPU_MARKER_FILE');
  assert.ok(/GPU process crashed x2, reason=abnormal-exit/.test(d.written[1][2]), 'marker content: reason + x2');
  assert.ok(/20\d{2}-\d{2}-\d{2}T/.test(d.written[1][2]), 'ISO date in the marker');
  assert.ok(d.logs.some((m) => /GPU-драйвер, похоже, сломан/.test(m)), 'log about the broken driver');
  assert.strictEqual(restarts, 0, 'no on-disk update — no restart');

  // 2) Non-GPU processes and harmless exits are not counted.
  d = makeDeps();
  onGone = extract('onChildProcessGone', d);
  const app2 = new EventEmitter();
  onGone(app2, () => { restarts += 1; });
  app2.emit('child-process-gone', {}, { type: 'Renderer', reason: 'crashed', exitCode: 11 });
  app2.emit('child-process-gone', {}, { type: 'Utility', reason: 'killed', exitCode: 0 });
  app2.emit('child-process-gone', {}, { type: 'GPU', reason: 'clean-exit', exitCode: 0 });
  app2.emit('child-process-gone', {}, { type: 'GPU', reason: 'killed', exitCode: 15 });
  assert.strictEqual(d.written.length, 0, 'non-GPU / clean exits — no marker written');
  assert.strictEqual(restarts, 0, 'no restarts');

  // 3) The process exited after an update on disk → restartLauncher()
  //    without counting the GPU crashes.
  d = makeDeps();
  d.state.launcherUpdatedOnDisk = true;
  onGone = extract('onChildProcessGone', d);
  const app3 = new EventEmitter();
  let called = 0;
  onGone(app3, () => { called += 1; });
  app3.emit('child-process-gone', {}, { type: 'GPU', reason: 'crashed', exitCode: 11 });
  assert.strictEqual(called, 1, 'on-disk update — launcher restarts');
  assert.strictEqual(d.written.length, 0, '…and no marker is written');
  assert.ok(d.logs.some((m) => /перезапускаюсь/.test(m)), 'log about the restart');

  // 4) The marker already lives (gpuDisabledByMarker) — a third crash doesn't append to it.
  d = makeDeps();
  d.gpuDisabledByMarker = true;
  onGone = extract('onChildProcessGone', d);
  const app4 = new EventEmitter();
  onGone(app4, () => { });
  app4.emit('child-process-gone', {}, { type: 'GPU', reason: 'crashed', exitCode: 11 });
  app4.emit('child-process-gone', {}, { type: 'GPU', reason: 'crashed', exitCode: 11 });
  assert.strictEqual(d.written.length, 0, 'marker already exists — not duplicated');

  // 5) A write error (no rights on LOG_DIR) — doesn't drop the handler.
  d = makeDeps();
  d.fs.mkdirSync = () => { throw new Error('EACCES'); };
  onGone = extract('onChildProcessGone', d);
  const app5 = new EventEmitter();
  onGone(app5, () => { });
  let threw = true;
  try {
    app5.emit('child-process-gone', {}, { type: 'GPU', reason: 'crashed', exitCode: 11 });
    app5.emit('child-process-gone', {}, { type: 'GPU', reason: 'crashed', exitCode: 11 });
    threw = false;
  } catch { /* should not get here */ }
  assert.strictEqual(threw, false, 'an exception from the marker write is not rethrown');
  assert.ok(d.logs.some((m) => /не удалось записать GPU-маркер/.test(m)), 'write error is logged');

  console.log('OK: gpu — onChildProcessGone');
}

main().catch((e) => { console.error('FAIL:', e.message); process.exit(1); });
