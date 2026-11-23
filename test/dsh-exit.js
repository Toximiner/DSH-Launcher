'use strict';
/*
 * test/dsh-exit.js — handleDshExit (src/dsh-process.js): что делать, когда
 * наш dsh завершился. Порт снова открыт — «усыновить» новый процесс (dsh
 * перезапустил сам себя, например из-за плагина); упал (ненулевой код или
 * сигнал) — перезапустить, но не больше 3 раз за 10 минут; обычное
 * завершение (code=0) — просто страница. Извлечён маркером (test/_extract.js);
 * crashRestart — настоящий (src/ui.js), время поддельное (Date/sleep),
 * остальное — инжект.
 * Run: node test/dsh-exit.js (or npm test).
 */
const assert = require('assert');
const { extract } = require('./_extract');
const { crashRestart } = require('../src/ui');

const tr = (ru) => ru; // Russian wording only

function makeDeps({
  stopping = false,
  destroyed = false,
  noWin = false,
  portUp = false,
  stopOnPortCheck = false,
} = {}) {
  const win = noWin ? null : { isDestroyed: () => destroyed };
  const logs = [];
  const errs = [];
  let now = 1_000_000; // fake time: the 8 s adoption deadline runs in fake ticks
  const Date = { now: () => now };
  const pages = {
    shown: [],
    showPage: (render) => { pages.shown.push(render); },
    showStatus: (title, body, isError) => ({ title, body, isError }),
    stderrBlock: () => '',
  };
  const launcher = {
    recovered: 0,
    recoverWindow: async () => { launcher.recovered += 1; },
  };
  const launch = {
    started: 0,
    launchDshInner: async () => { launch.started += 1; },
  };
  const state = {
    win,
    stopping,
    launching: false,
    weStartedDsh: true,
    adoptedDsh: false,
    adoptedOldPgid: null,
    dshProc: { pid: 777 },
    dshFatal: false,
    dshCrashNotice: null,
    portState: 'down',
  };
  const portOpen = async () => {
    if (stopOnPortCheck) state.stopping = true; // the launcher stopped while we checked
    return portUp;
  };
  return {
    state,
    pages,
    launcher,
    launch,
    portOpen,
    sleep: async () => { now += 500; },
    crashRestart,
    tr,
    pushErr: (s) => { errs.push(s); },
    retryButton: () => '<button>retry</button>',
    Date,
    console: { log: (m) => logs.push(m), error: (m) => logs.push(m) },
    logs,
    errs,
  };
}

async function main() {
  // 1) The launcher is stopping — we ignore the exit entirely.
  let d = makeDeps({ stopping: true });
  let h = extract('handleDshExit', d);
  await h(1, null);
  assert.strictEqual(d.errs.length, 0, 'stopping — not even the error line');
  assert.strictEqual(d.pages.shown.length, 0, 'stopping — no page');
  assert.strictEqual(d.launch.started, 0, 'stopping — no restart');

  // 2) The port is alive again — dsh restarted itself: adopt it.
  d = makeDeps({ portUp: true });
  h = extract('handleDshExit', d);
  await h(0, null);
  assert.strictEqual(d.launcher.recovered, 1, 'recoverWindow called');
  assert.strictEqual(d.state.weStartedDsh, false, 'the new process is not our child');
  assert.strictEqual(d.state.adoptedDsh, true, 'adopted: we will stop it on window close');
  assert.strictEqual(d.state.adoptedOldPgid, 777, 'the old process group is remembered');
  assert.strictEqual(d.state.dshProc, null, 'no handle on the new process');
  assert.strictEqual(d.state.dshFatal, false, 'adopted — not fatal');
  assert.strictEqual(d.state.portState, 'up', 'in sync with the watchdog');
  assert.strictEqual(d.launch.started, 0, 'our own process is not started');
  let page = d.pages.shown[0]();
  assert.strictEqual(page.title, 'dsh перезапускается…', 'the «restarting» page');
  assert.strictEqual(page.isError, false, 'the page — not an error');

  // 3) Stopped while the port was checked — no adopt, no restart, no page.
  d = makeDeps({ stopOnPortCheck: true });
  h = extract('handleDshExit', d);
  await h(0, null);
  assert.strictEqual(d.state.adoptedDsh, false, 'no adopt');
  assert.strictEqual(d.launch.started, 0, 'no restart');
  assert.strictEqual(d.pages.shown.length, 1, 'only the «restarting» page (shown before the stop)');
  assert.strictEqual(d.pages.shown[0]().title, 'dsh перезапускается…', 'the final page is not added');
  assert.strictEqual(d.state.dshFatal, false, 'code=0 — not fatal');

  // 4) Crashes: the first three — restart (including code=0+signal — a crash).
  d = makeDeps();
  h = extract('handleDshExit', d);
  await h(1, null);
  assert.strictEqual(d.launch.started, 1, 'crash №1 — restart');
  assert.deepStrictEqual(d.state.dshCrashNotice, { code: 1, signal: null }, 'the toast notice');
  assert.strictEqual(d.state.launching, false, 'launching is reset');
  assert.strictEqual(d.state.dshFatal, false, 'not fatal yet');
  await h(0, 'SIGTERM');
  assert.strictEqual(d.launch.started, 2, 'crash №2 (signal) — restart');
  assert.deepStrictEqual(d.state.dshCrashNotice, { code: 0, signal: 'SIGTERM' });
  await h(1, null);
  assert.strictEqual(d.launch.started, 3, 'crash №3 — restart');

  // 5) The fourth crash within 10 minutes — we give up: fatal + the page.
  await h(1, null);
  assert.strictEqual(d.launch.started, 3, 'crash №4 — no restart (3 in 10 min is the limit)');
  assert.strictEqual(d.state.dshFatal, true, 'dshFatal — the status on the tray icon');
  page = d.pages.shown[d.pages.shown.length - 1]();
  assert.strictEqual(page.title, 'dsh остановился');
  assert.strictEqual(page.isError, true, 'the page — an error');
  assert.ok(/падает снова и снова/.test(page.body), 'the text about repeated crashes');
  assert.ok(/<button>retry<\/button>/.test(page.body), 'the «Проверить снова» button');
  assert.ok(d.logs.some((m) => /авто-перезапуск остановлен/.test(m)), 'the log line');

  // 6) A normal exit (code=0), the port did not come up — a page, not a crash.
  d = makeDeps();
  h = extract('handleDshExit', d);
  await h(0, null);
  assert.strictEqual(d.launch.started, 0, 'code=0 — no restart');
  assert.strictEqual(d.state.dshFatal, false, 'code=0 — not fatal');
  page = d.pages.shown[d.pages.shown.length - 1]();
  assert.strictEqual(page.title, 'dsh остановился');
  assert.ok(/за 8 секунд/.test(page.body), 'the text: no new process within 8 s');
  assert.ok(d.errs.some((s) => /dsh остановился \(code=0/.test(s)), 'the error line');

  // 7) The window is destroyed — no restart and no page, but dshFatal is set.
  d = makeDeps({ destroyed: true });
  h = extract('handleDshExit', d);
  await h(1, null);
  assert.strictEqual(d.launch.started, 0, 'no window — no restart');
  assert.strictEqual(d.pages.shown.length, 1, 'only the «restarting» page (no final page for a destroyed window)');
  assert.strictEqual(d.pages.shown[0]().title, 'dsh перезапускается…');
  assert.strictEqual(d.state.dshFatal, true, 'dshFatal is set anyway');

  // 8) A second parallel call while the first is checking the port — it
  //    sees adoptionCheck and does nothing.
  d = makeDeps();
  h = extract('handleDshExit', d);
  let release;
  const gate = new Promise((r) => { release = r; });
  let first = true;
  const origPortOpen = d.portOpen;
  d.portOpen = async () => { if (first) { first = false; await gate; } return origPortOpen(); };
  const p1 = h(1, null); // h runs synchronously to the port check: adoptionCheck is already true
  const p2 = h(1, null);
  await p2;
  assert.strictEqual(d.launch.started, 0, 'the 2nd call — no restart');
  assert.strictEqual(d.pages.shown.length, 1, 'the 2nd call — no page of its own');
  release();
  await p1;
  assert.strictEqual(d.launch.started, 1, 'the 1st call finished — restart');
}

main().catch((e) => { console.error('FAIL:', e.message); process.exit(1); });
