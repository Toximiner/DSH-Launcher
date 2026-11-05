'use strict';
/*
 * test/file-help.js — restartDsh (src/file-help.js): a dsh started not
 * by the launcher (attach) is left alone — we show an explanation; the
 * launcher's dsh — we stop and start it again (with or without
 * confirmation, e.g., from dshlauncher://menu/restart-dsh/). And
 * gpuStatusText — the GPU state strings. Both functions are extracted by
 * markers (test/_extract.js); dialog/pages/dshStop/launch/portOpen/sleep
 * are injected.
 * Run: node test/file-help.js (or npm test).
 */
const assert = require('assert');
const { extract } = require('./_extract');

const tr = (ru) => ru; // Russian wording only

function makeDeps({
  weStartedDsh = true,
  adoptedDsh = false,
  launching = false,
  stopping = false,
  restartingDsh = false,
  destroyed = false,
  dialogResponses = [],
  portUpTimes = 0,
} = {}) {
  const win = { isDestroyed: () => destroyed };
  const dialog = {
    calls: [],
    showMessageBox: async (w, opts) => {
      dialog.calls.push({ win: w, opts });
      return { response: dialogResponses.length ? dialogResponses.shift() : 0 };
    },
  };
  const pages = {
    shown: [],
    showPage: (render) => { pages.shown.push(render); },
    showStatus: (title, body, isError) => ({ title, body, isError }),
  };
  const dshStop = {
    stopped: [],
    stopDsh: async () => { dshStop.stopped.push('stopDsh'); },
    stopAdoptedDsh: async () => { dshStop.stopped.push('stopAdoptedDsh'); },
  };
  const launch = {
    started: 0,
    launchDshInner: async () => { launch.started += 1; },
  };
  const state = {
    win,
    stopping,
    launching,
    restartingDsh,
    weStartedDsh,
    adoptedDsh,
    adoptedOldPgid: adoptedDsh ? 4242 : null,
    dshProc: { pid: 777 },
    dshFatal: false,
  };
  let portUpsLeft = portUpTimes;
  const portOpen = async () => (portUpsLeft-- > 0);
  const sleep = async () => { /* no real waiting */ };
  const logs = [];
  return {
    state,
    dialog,
    pages,
    dshStop,
    launch,
    portOpen,
    sleep,
    tr,
    console: { log: (m) => logs.push(m), error: (m) => logs.push(m) },
    logs,
  };
}

async function main() {
  // 1) Someone else's dsh (attach): we don't stop it and don't restart it
  //    — we explain where to restart it.
  let d = makeDeps({ weStartedDsh: false, adoptedDsh: false });
  let restartDsh = extract('restartDsh', d);
  await restartDsh({ confirm: true });
  assert.strictEqual(d.dialog.calls.length, 1, 'dialog about a foreign dsh shown');
  assert.strictEqual(d.dialog.calls[0].opts.type, 'info');
  assert.strictEqual(d.dialog.calls[0].opts.message, 'Этот dsh запущен не лаунчером');
  assert.ok(/перезапустите его там, где запускали/.test(d.dialog.calls[0].opts.detail), 'hint — restart it in the terminal');
  assert.strictEqual(d.dshStop.stopped.length, 0, 'foreign dsh NOT stopped');
  assert.strictEqual(d.launch.started, 0, 'dsh NOT started again');
  assert.strictEqual(d.state.weStartedDsh, false, 'flags unchanged');
  assert.strictEqual(d.pages.shown.length, 0, 'the «Перезапуск…» page is not shown');

  // 2) Our dsh, confirm, «Отмена» — we do nothing.
  d = makeDeps({ weStartedDsh: true, dialogResponses: [1] });
  restartDsh = extract('restartDsh', d);
  await restartDsh({ confirm: true });
  assert.strictEqual(d.dialog.calls.length, 1, 'confirmation requested');
  assert.strictEqual(d.dialog.calls[0].opts.type, 'question');
  assert.deepStrictEqual(d.dialog.calls[0].opts.buttons, ['Перезапустить', 'Отмена']);
  assert.strictEqual(d.dialog.calls[0].opts.cancelId, 1, '«Отмена» — cancelId');
  assert.strictEqual(d.dshStop.stopped.length, 0, '«Отмена» — dsh not stopped');
  assert.strictEqual(d.launch.started, 0, '«Отмена» — dsh not started');

  // 3) Our dsh, confirm, «Перезапустить» — stop, waiting for the port, start.
  d = makeDeps({ weStartedDsh: true, dialogResponses: [0], portUpTimes: 2 });
  restartDsh = extract('restartDsh', d);
  await restartDsh({ confirm: true });
  assert.strictEqual(d.dshStop.stopped.length, 1, 'our dsh stopped');
  assert.strictEqual(d.dshStop.stopped[0], 'stopDsh');
  assert.strictEqual(d.launch.started, 1, 'dsh started again');
  assert.strictEqual(d.state.weStartedDsh, false, 'after restart: not ours (until the new one comes up)');
  assert.strictEqual(d.state.adoptedDsh, false, 'adopted reset');
  assert.strictEqual(d.state.adoptedOldPgid, null, 'old pgid reset');
  assert.strictEqual(d.state.dshProc, null, 'process reset');
  assert.strictEqual(d.state.restartingDsh, false, 'register: restartingDsh cleared');
  assert.strictEqual(d.state.launching, false, 'register: launching cleared');
  assert.ok(d.logs.some((m) => /перезапуск dsh по запросу/.test(m)), 'log line about the restart');
  assert.ok(typeof d.pages.shown[0] === 'function', 'the «Перезапуск dsh…» page shown');
  const rendered = d.pages.shown[0]();
  assert.strictEqual(rendered.title, 'Перезапуск dsh…', 'page title');
  assert.strictEqual(rendered.isError, false, 'the page — not an error');

  // 4) The "adopted" dsh, confirm:false (dshlauncher://menu/restart-dsh/) —
  //    no dialog, via stopAdoptedDsh.
  d = makeDeps({ weStartedDsh: false, adoptedDsh: true });
  restartDsh = extract('restartDsh', d);
  await restartDsh({ confirm: false });
  assert.strictEqual(d.dialog.calls.length, 0, 'confirm:false — no dialog');
  assert.deepStrictEqual(d.dshStop.stopped, ['stopAdoptedDsh'], 'stopped via stopAdoptedDsh');
  assert.strictEqual(d.launch.started, 1, 'started again');

  // 5) The launcher is already starting something — the request is ignored.
  d = makeDeps({ weStartedDsh: true, launching: true });
  restartDsh = extract('restartDsh', d);
  await restartDsh({ confirm: true });
  assert.strictEqual(d.dialog.calls.length, 0, 'launching — no dialog');
  assert.strictEqual(d.dshStop.stopped.length, 0, 'launching — stop not called');
  assert.strictEqual(d.launch.started, 0, 'launching — launch not called');

  // 6) The window is already destroyed — we exit silently.
  d = makeDeps({ weStartedDsh: true, destroyed: true });
  restartDsh = extract('restartDsh', d);
  await restartDsh({ confirm: true });
  assert.strictEqual(d.dshStop.stopped.length, 0, 'destroyed — nothing is done');
  assert.strictEqual(d.launch.started, 0, 'destroyed — nothing is started');

  // 7) gpuStatusText — all branches of the wording.
  const gpuStatusText = extract('gpuStatusText', { tr });
  assert.strictEqual(gpuStatusText({ on: true, marker: false }), 'включено');
  assert.strictEqual(gpuStatusText({ on: true, marker: true }),
    'включено; со следующего запуска будет отключено — GPU-процесс падал',
    'on + marker already created — warn about it');
  assert.strictEqual(gpuStatusText({ on: false, reason: 'env' }), 'отключено (DSH_LAUNCHER_NO_GPU=1)');
  assert.strictEqual(gpuStatusText({ on: false, reason: 'setting' }), 'отключено в «Настройках»');
  assert.strictEqual(gpuStatusText({ on: false, reason: 'marker', marker: true, daysLeft: 12 }),
    'отключено после сбоев GPU-процесса (ещё 12 дн.)');
  assert.strictEqual(gpuStatusText({ on: false, reason: 'marker', marker: false }),
    'отключено до перезапуска лаунчера (включится при следующем запуске)',
    'marker just created — still running in this session');
  assert.strictEqual(gpuStatusText({ on: false, reason: 'norender' }), 'отключено (нет /dev/dri/renderD* — ВМ без 3D?)');
  assert.strictEqual(gpuStatusText({ on: false, reason: 'egl' }), 'отключено (нет библиотек EGL)');

  console.log('OK: file-help — restartDsh, gpuStatusText');
}

main().catch((e) => { console.error('FAIL:', e.message); process.exit(1); });
