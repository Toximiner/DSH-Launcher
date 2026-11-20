'use strict';
/*
 * test/deferred-attach.js — deferredDshUpdateInAttach (main.js): the thin
 * wrapper the attach branch runs when «Обновить сейчас» (Update now) was
 * pressed for dsh. It builds confirmStop (a native question dialog about
 * stopping the foreign dsh: «Остановить dsh» / «Отмена») and toast
 * (toastPageUrl + showToast), hands them — together with state, UPDATE_NOW,
 * PORT, tr, portListenerPid, compareSemVer, the offer* functions, probeDsh
 * and restartDsh — to runDeferredUpdate, and swallows its errors into one
 * console.error line. Extracted by markers (test/_extract.js); everything
 * else is injected.
 * Run: node test/deferred-attach.js (or npm test).
 */
const assert = require('assert');
const { extract } = require('./_extract');

function makeDeps(over = {}) {
  const calls = [];
  const errs = [];
  const recs = [];
  const state = { win: { id: 'win' } };
  const tr = (ru) => ru; // Russian wording only
  const dialog = {
    // each recorded call returns the next queued response
    queue: [0],
    showMessageBox: async (win, opts) => { recs.push({ win, opts }); return { response: dialog.queue.shift() }; },
  };
  const deps = {
    state,
    dialog,
    recs,
    tr,
    showToast: (u) => { calls.push(u); },
    toastPageUrl: (o) => 'URL:' + o.title + '|' + o.sub,
    runDeferredUpdate: async () => { throw new Error('runDeferredUpdate should not be called in this case'); },
    UPDATE_NOW: null,
    PORT: 3080,
    portListenerPid: (port) => 4242,
    compareSemVer: (a, b) => (a === b ? 0 : (String(a) > String(b) ? 1 : -1)),
    offerLauncherUpdate: async () => { calls.push('offer-launcher'); },
    offerDshUpdate: async () => { calls.push('offer-dsh'); },
    probeDsh: async () => { calls.push('probe'); },
    restartDsh: async () => { calls.push('restart'); },
    console: { log() {}, error: (...a) => { errs.push(a); } },
    calls,
    errs,
  };
  return Object.assign(deps, over);
}

function run(d) {
  return extract('deferredDshUpdateInAttach', {
    state: d.state,
    dialog: d.dialog,
    tr: d.tr,
    showToast: d.showToast,
    toastPageUrl: d.toastPageUrl,
    runDeferredUpdate: d.runDeferredUpdate,
    UPDATE_NOW: d.UPDATE_NOW,
    PORT: d.PORT,
    portListenerPid: d.portListenerPid,
    compareSemVer: d.compareSemVer,
    offerLauncherUpdate: d.offerLauncherUpdate,
    offerDshUpdate: d.offerDshUpdate,
    probeDsh: d.probeDsh,
    restartDsh: d.restartDsh,
    console: d.console,
  })();
}

async function main() {
  // 1) Wiring: the deps object runDeferredUpdate receives carries the
  //    injected state, UPDATE_NOW, PORT, offers, probe, restartDsh and tr.
  let captured = null;
  let d = makeDeps({
    UPDATE_NOW: 'dsh',
    runDeferredUpdate: async (deps2) => { captured = deps2; },
  });
  await run(d);
  assert.ok(captured, 'runDeferredUpdate was called');
  assert.strictEqual(captured.state, d.state, 'state passed through');
  assert.strictEqual(captured.UPDATE_NOW, 'dsh', 'UPDATE_NOW passed through');
  assert.strictEqual(captured.PORT, 3080, 'PORT passed through');
  assert.strictEqual(captured.offers.offerLauncherUpdate, d.offerLauncherUpdate, 'offerLauncherUpdate passed through');
  assert.strictEqual(captured.offers.offerDshUpdate, d.offerDshUpdate, 'offerDshUpdate passed through');
  assert.strictEqual(captured.probe, d.probeDsh, 'probeDsh passed as probe');
  assert.strictEqual(captured.restartDsh, d.restartDsh, 'restartDsh passed through');
  assert.strictEqual(captured.tr, d.tr, 'tr passed through');

  // 2) confirmStop (from the captured deps): a native question dialog on the
  //    launcher window; the buttons are «Остановить dsh» / «Отмена»,
  //    response 0 → true, 1 → false, and the version is in the detail.
  d.dialog.queue.push(1);
  assert.strictEqual(await captured.confirmStop('1.2.3'), true, 'response 0 — stop confirmed');
  assert.strictEqual(d.recs.length, 1, 'one dialog shown');
  assert.strictEqual(d.recs[0].win, d.state.win, 'the dialog is shown on the launcher window');
  assert.strictEqual(d.recs[0].opts.type, 'question', 'a question dialog');
  assert.deepStrictEqual(d.recs[0].opts.buttons, ['Остановить dsh', 'Отмена'], 'stop / cancel buttons');
  assert.strictEqual(d.recs[0].opts.defaultId, 0, 'default — stop');
  assert.strictEqual(d.recs[0].opts.cancelId, 1, 'cancel is «Отмена»');
  assert.ok(d.recs[0].opts.detail.includes('1.2.3'), 'the new version is in the dialog detail');
  assert.strictEqual(await captured.confirmStop('1.2.3'), false, 'response 1 — cancelled');
  assert.strictEqual(d.recs.length, 2, 'one dialog per confirmStop call');
  assert.ok(d.recs[1].opts.detail.includes('1.2.3'), 'the version is in the detail on every call');

  // 3) toast (from the captured deps): showToast(toastPageUrl({title, sub,
  //    buttons})) — title and sub land in the URL, buttons are not.
  const before = d.calls.length;
  captured.toast({ title: 'a', sub: 'b', buttons: [] });
  assert.deepStrictEqual(d.calls.slice(before), ['URL:a|b'], 'toast → showToast(toastPageUrl(...))');

  // 4) an error inside runDeferredUpdate is swallowed: the wrapper resolves
  //    and logs one console.error line with the message.
  const d2 = makeDeps({
    UPDATE_NOW: 'dsh',
    runDeferredUpdate: async () => { throw new Error('boom'); },
  });
  await run(d2); // must not throw
  assert.strictEqual(d2.errs.length, 1, 'one console.error line');
  assert.strictEqual(d2.errs[0][0], '[launcher] обновление в режиме подключения:', 'the error prefix');
  assert.strictEqual(d2.errs[0][1], 'boom', 'the error message');
  assert.strictEqual(d2.errs[0].length, 2, 'only the prefix and the message');

  console.log('OK: deferred attach — confirmStop dialog, toast wiring, swallowed errors');
}

main().catch((e) => { console.error('FAIL:', e.message); process.exit(1); });
