'use strict';
/*
 * test/updates-live.js — runDeferredUpdate (src/updates-live.js): the
 * «Обновить сейчас» (Update now) deferred update in attach mode. With
 * UPDATE_NOW === 'dsh' the npm package is installed (offerDshUpdate), the
 * probe checks the new version, and the user is asked once about stopping
 * the foreign dsh: yes — it is adopted (pid from the port, state.adoptedDsh)
 * and restarted without a second question; no — a toast says to restart it
 * where it was started. UPDATE_NOW === 'launcher' just offers the launcher
 * update. Extracted by markers (test/_extract.js); state/offers/probe/
 * restartDsh/confirmStop/toast/console are injected.
 * Run: node test/updates-live.js (or npm test).
 */
const assert = require('assert');
const { extract } = require('./_extract');

const tr = (ru) => ru; // Russian wording only

function makeDeps(over = {}) {
  const state = {
    stopping: false,
    launching: false,
    restarting: false,
    dshInstalledVersion: '0.2.2-rc.1',
    dshBin: '/usr/bin/dsh',
    weStartedDsh: false,
    adoptedDsh: false,
    adoptedOldPgid: null,
    dshProc: { pid: 777 },
    dshFatal: false,
    portState: 'down',
  };
  // Fresh records per case. The probe reads the queue at CALL time from the
  // deps object, because Object.assign(deps, over) may swap probeQueue in
  // after makeDeps returned.
  const probeCalls = [];
  const restartCalls = [];
  const confirmCalls = [];
  const toasts = [];
  const logs = [];
  const deps = {
    state,
    UPDATE_NOW: null,
    PORT: 3080,
    tr,
    portListenerPid: (port) => 4242,
    compareSemVer: (a, b) => (a === b ? 0 : (String(a) > String(b) ? 1 : -1)),
    offers: {
      launcherCalls: 0,
      dshCalls: 0,
      offerLauncherUpdate: async () => { deps.offers.launcherCalls += 1; },
      offerDshUpdate: async () => { deps.offers.dshCalls += 1; },
    },
    probeQueue: [],
    probe: async (bin) => {
      probeCalls.push(bin);
      const q = deps.probeQueue;
      if (!q || !q.length) throw new Error('probe called more times than queued');
      return q.shift();
    },
    restartDsh: async (opts) => { restartCalls.push(opts); },
    confirmStop: async (version) => { confirmCalls.push(version); return true; },
    toast: (t) => { toasts.push(t); },
    console: { log: (m) => logs.push(m), error: (m) => logs.push(m) },
    probeCalls,
    restartCalls,
    confirmCalls,
    toasts,
    logs,
  };
  const { state: stateOver, ...rest } = over; // state is merged into deps.state below
  if (stateOver) Object.assign(state, stateOver);
  return Object.assign(deps, rest);
}

function run(d) {
  return extract('runDeferredUpdate', { deps: d })(d);
}

async function main() {
  // 1) No «Update now» pending (UPDATE_NOW is null) — we do nothing at all.
  let d = makeDeps({});
  await run(d);
  assert.strictEqual(d.offers.launcherCalls, 0, 'offerLauncherUpdate not called');
  assert.strictEqual(d.offers.dshCalls, 0, 'offerDshUpdate not called');
  assert.strictEqual(d.probeCalls.length, 0, 'probe not called');
  assert.strictEqual(d.confirmCalls.length, 0, 'confirmStop not called');
  assert.strictEqual(d.toasts.length, 0, 'toast not called');
  assert.strictEqual(d.restartCalls.length, 0, 'restartDsh not called');
  assert.strictEqual(d.state.adoptedDsh, false, 'state untouched');

  // 2) The launcher is already busy (stopping/launching/restarting) —
  //    the deferred update is skipped.
  for (const flag of ['stopping', 'launching', 'restarting']) {
    d = makeDeps({ UPDATE_NOW: 'dsh' });
    d.state[flag] = true;
    await run(d);
    assert.strictEqual(d.offers.launcherCalls, 0, flag + ' — offerLauncherUpdate not called');
    assert.strictEqual(d.offers.dshCalls, 0, flag + ' — offerDshUpdate not called');
    assert.strictEqual(d.probeCalls.length, 0, flag + ' — probe not called');
    assert.strictEqual(d.confirmCalls.length, 0, flag + ' — confirmStop not called');
    assert.strictEqual(d.toasts.length, 0, flag + ' — toast not called');
    assert.strictEqual(d.restartCalls.length, 0, flag + ' — restartDsh not called');
    assert.strictEqual(d.state.adoptedDsh, false, flag + ' — flags unchanged');
    assert.strictEqual(d.state.portState, 'down', flag + ' — portState unchanged');
  }

  // 3) UPDATE_NOW === 'launcher' — only the launcher update is offered.
  d = makeDeps({ UPDATE_NOW: 'launcher' });
  await run(d);
  assert.strictEqual(d.offers.launcherCalls, 1, 'offerLauncherUpdate called once');
  assert.strictEqual(d.offers.dshCalls, 0, 'offerDshUpdate not called');
  assert.strictEqual(d.probeCalls.length, 0, 'probe not called');
  assert.strictEqual(d.confirmCalls.length, 0, 'confirmStop not called');
  assert.strictEqual(d.toasts.length, 0, 'toast not called');
  assert.strictEqual(d.restartCalls.length, 0, 'restartDsh not called');
  assert.strictEqual(d.state.weStartedDsh, false, 'flags unchanged');
  assert.strictEqual(d.state.adoptedDsh, false, 'flags unchanged');

  // 4) dsh: the version was not upgraded (the probe returned the same
  //    version) — we return without a question, a toast, a restart.
  d = makeDeps({
    UPDATE_NOW: 'dsh',
    probeQueue: [{ ok: true, version: '0.2.2-rc.1' }],
  });
  await run(d);
  assert.strictEqual(d.offers.dshCalls, 1, 'offerDshUpdate called');
  assert.strictEqual(d.probeCalls.length, 1, 'probe called once (version was known, no initial probe)');
  assert.strictEqual(d.confirmCalls.length, 0, 'same version — no confirmStop');
  assert.strictEqual(d.toasts.length, 0, 'same version — no toast');
  assert.strictEqual(d.restartCalls.length, 0, 'same version — no restartDsh');
  assert.strictEqual(d.state.adoptedDsh, false, 'flags unchanged');
  assert.strictEqual(d.state.portState, 'down', 'portState unchanged');

  // 5) dsh: the probe says the install is broken — the same expectations.
  d = makeDeps({
    UPDATE_NOW: 'dsh',
    probeQueue: [{ ok: false, reason: 'broken' }],
  });
  await run(d);
  assert.strictEqual(d.offers.dshCalls, 1, 'offerDshUpdate called');
  assert.strictEqual(d.probeCalls.length, 1, 'probe called once');
  assert.strictEqual(d.confirmCalls.length, 0, 'broken probe — no confirmStop');
  assert.strictEqual(d.toasts.length, 0, 'broken probe — no toast');
  assert.strictEqual(d.restartCalls.length, 0, 'broken probe — no restartDsh');
  assert.strictEqual(d.state.adoptedDsh, false, 'flags unchanged');
  assert.strictEqual(d.state.portState, 'down', 'portState unchanged');

  // 6) dsh: no probe at all in the deps — treated as «not upgraded».
  d = makeDeps({ UPDATE_NOW: 'dsh' });
  delete d.probe;
  await run(d);
  assert.strictEqual(d.offers.dshCalls, 1, 'offerDshUpdate called');
  assert.strictEqual(d.probeCalls.length, 0, 'no probe — probe not called');
  assert.strictEqual(d.confirmCalls.length, 0, 'no probe — no confirmStop');
  assert.strictEqual(d.toasts.length, 0, 'no probe — no toast');
  assert.strictEqual(d.restartCalls.length, 0, 'no probe — no restartDsh');
  assert.strictEqual(d.state.adoptedDsh, false, 'flags unchanged');
  assert.strictEqual(d.state.portState, 'down', 'portState unchanged');

  // 7) dsh, the happy path with «yes»: the old version is probed first,
  //    the update is installed, the dsh is adopted and restarted.
  d = makeDeps({
    UPDATE_NOW: 'dsh',
    state: { dshInstalledVersion: null },
    probeQueue: [{ ok: true, version: '0.2.2-rc.1' }, { ok: true, version: '0.2.2-rc.5' }],
  });
  let versionAtOfferTime = 'not-probed-yet';
  d.offers.offerDshUpdate = async () => {
    d.offers.dshCalls += 1;
    versionAtOfferTime = d.state.dshInstalledVersion;
  };
  await run(d);
  assert.strictEqual(versionAtOfferTime, '0.2.2-rc.1', 'the initial probe set the version BEFORE the offer');
  assert.strictEqual(d.probeCalls.length, 2, 'probe called twice (initial + after the offer)');
  assert.strictEqual(d.offers.dshCalls, 1, 'offerDshUpdate called once');
  assert.strictEqual(d.offers.launcherCalls, 0, 'offerLauncherUpdate not called');
  assert.strictEqual(d.confirmCalls.length, 1, 'confirmStop called once');
  assert.strictEqual(d.confirmCalls[0], '0.2.2-rc.5', 'confirmStop asked about the NEW version');
  assert.strictEqual(d.state.weStartedDsh, false, 'adopted: weStartedDsh cleared');
  assert.strictEqual(d.state.adoptedDsh, true, 'the dsh is adopted');
  assert.strictEqual(d.state.adoptedOldPgid, null, 'no old process group (it was never ours)');
  assert.strictEqual(d.state.dshProc, null, 'the process is no longer ours');
  assert.strictEqual(d.state.dshFatal, false, 'dshFatal cleared');
  assert.strictEqual(d.state.portState, 'up', 'portState — up (the dsh is still on the port)');
  assert.strictEqual(d.restartCalls.length, 1, 'restartDsh called once');
  assert.deepStrictEqual(d.restartCalls[0], { confirm: false }, 'restart without a second question');
  assert.strictEqual(d.toasts.length, 0, 'no toast on the happy path');
  assert.ok(d.logs.some((m) => m.includes('перезапускаю работающий dsh')), 'log line about restarting the running dsh');

  // 8) dsh, the same but «no»: the package is installed on disk, but the
  //    running dsh is left alone — the toast says where to restart it.
  d = makeDeps({
    UPDATE_NOW: 'dsh',
    state: { dshInstalledVersion: null },
    probeQueue: [{ ok: true, version: '0.2.2-rc.1' }, { ok: true, version: '0.2.2-rc.5' }],
  });
  d.confirmStop = async (version) => { d.confirmCalls.push(version); return false; };
  await run(d);
  assert.strictEqual(d.offers.dshCalls, 1, 'offerDshUpdate called');
  assert.strictEqual(d.confirmCalls.length, 1, 'confirmStop asked');
  assert.strictEqual(d.confirmCalls[0], '0.2.2-rc.5', 'asked about the new version');
  assert.strictEqual(d.restartCalls.length, 0, '«no» — restartDsh NOT called');
  assert.strictEqual(d.state.weStartedDsh, false, 'flags unchanged');
  assert.strictEqual(d.state.adoptedDsh, false, '«no» — the dsh is NOT adopted');
  assert.strictEqual(d.state.adoptedOldPgid, null, 'pgid unchanged');
  assert.deepStrictEqual(d.state.dshProc, { pid: 777 }, '«no» — the process is untouched');
  assert.strictEqual(d.state.dshFatal, false, 'dshFatal unchanged');
  assert.strictEqual(d.state.portState, 'down', 'portState unchanged');
  assert.strictEqual(d.toasts.length, 1, 'a toast about the installed version');
  assert.ok(d.toasts[0].title.includes('0.2.2-rc.5'), 'the toast shows the new version');
  assert.strictEqual(d.toasts[0].sub, 'перезапустите dsh там, где запускали', 'the toast: restart it where you started it');
  assert.deepStrictEqual(d.toasts[0].buttons, [], 'the toast has no buttons');

  // 9) dsh: the dsh died in the meantime (nothing on the port) — we do
  //    not ask, do not restart.
  d = makeDeps({
    UPDATE_NOW: 'dsh',
    state: { dshInstalledVersion: null },
    portListenerPid: (port) => null,
    probeQueue: [{ ok: true, version: '0.2.2-rc.1' }, { ok: true, version: '0.2.2-rc.5' }],
  });
  await run(d);
  assert.strictEqual(d.confirmCalls.length, 0, 'no dsh on the port — no confirmStop');
  assert.strictEqual(d.toasts.length, 0, 'no dsh on the port — no toast');
  assert.strictEqual(d.restartCalls.length, 0, 'no dsh on the port — no restartDsh');
  assert.strictEqual(d.state.adoptedDsh, false, 'flags unchanged');
  assert.strictEqual(d.state.portState, 'down', 'portState unchanged');

  // 10) dsh: no PORT (0) — the same as «nothing on the port».
  d = makeDeps({
    UPDATE_NOW: 'dsh',
    state: { dshInstalledVersion: null },
    PORT: 0,
    probeQueue: [{ ok: true, version: '0.2.2-rc.1' }, { ok: true, version: '0.2.2-rc.5' }],
  });
  await run(d);
  assert.strictEqual(d.confirmCalls.length, 0, 'PORT 0 — no confirmStop');
  assert.strictEqual(d.toasts.length, 0, 'PORT 0 — no toast');
  assert.strictEqual(d.restartCalls.length, 0, 'PORT 0 — no restartDsh');
  assert.strictEqual(d.state.adoptedDsh, false, 'flags unchanged');
  assert.strictEqual(d.state.portState, 'down', 'portState unchanged');

  // 11) dsh: the offer itself started the launcher's restart (stopping)
  //     — we return without probing again, asking, restarting.
  d = makeDeps({
    UPDATE_NOW: 'dsh',
    probeQueue: [],
  });
  d.offers.offerDshUpdate = async () => {
    d.offers.dshCalls += 1;
    d.state.stopping = true; // the launcher went to its own restart
  };
  await run(d);
  assert.strictEqual(d.offers.dshCalls, 1, 'offerDshUpdate called (and flipped stopping)');
  assert.strictEqual(d.probeCalls.length, 0, 'no probe (version known; stopped right after the offer)');
  assert.strictEqual(d.confirmCalls.length, 0, 'stopping — no confirmStop');
  assert.strictEqual(d.toasts.length, 0, 'stopping — no toast');
  assert.strictEqual(d.restartCalls.length, 0, 'stopping — no restartDsh');

  // 12) dsh: nothing is installed (the probe says so) — we return BEFORE
  //     offering anything.
  d = makeDeps({
    UPDATE_NOW: 'dsh',
    state: { dshInstalledVersion: null },
    probeQueue: [{ ok: false, reason: 'missing' }],
  });
  await run(d);
  assert.strictEqual(d.offers.dshCalls, 0, 'not installed — offerDshUpdate NOT called');
  assert.strictEqual(d.probeCalls.length, 1, 'the initial probe ran');
  assert.strictEqual(d.confirmCalls.length, 0, 'not installed — no confirmStop');
  assert.strictEqual(d.toasts.length, 0, 'not installed — no toast');
  assert.strictEqual(d.restartCalls.length, 0, 'not installed — no restartDsh');
  assert.strictEqual(d.state.dshInstalledVersion, null, 'the version is still unknown');

  // ------------------------------------------------------------------
  // showUpdateNowDialog (same file, src/updates-live.js): the
  // «Обновить … до X…» (Update … to X…) window. state.availableUpdates
  // must hold the entry; dialogs.openDialog must return a non-null
  // generation; the rendered page carries the version, the release notes
  // (deb scheme for the launcher, semver + the EN part for dsh), the
  // restart warning and the action buttons.
  const showDeps = (stateOver = {}, openDialogResult) => {
    const openCalls = [];
    const schemes = [];
    let nextGen = 'gen-1';
    const fakeNotes = [{ version: 'v2', body: 'B2' }];
    const notesCalled = [];
    const dshNotesCalled = [];
    const deps = {
      state: Object.assign({
        availableUpdates: {
          dsh: { version: '0.2.2-rc.5', installed: '0.2.2-rc.1', releases: [{ version: '0.2.2-rc.2', body: 'x' }] },
        },
        weStartedDsh: false,
        adoptedDsh: false,
      }, stateOver),
      updateName: (kind) => (kind === 'launcher' ? 'DeepSeek Harness Launcher' : 'dsh'),
      tr: (ru) => ru,
      dialogs: {
        openDialog: (t) => { openCalls.push(t); if (openDialogResult !== undefined) return openDialogResult; nextGen = 'gen-1'; return nextGen; },
        dialogShow: (gen, fn) => { deps.render = fn; },
        dialogPage: (t, html) => { deps.pageHtml = html; },
      },
      updates: { notesBlockHtml: (items) => 'NOTES:' + items.map((i) => i.version + '|' + i.body).join(',') },
      esc: (s) => String(s),
      notesBetween: (releases, installed, latest, scheme) => { schemes.push(scheme); notesCalled.push(scheme); return fakeNotes; },
      dshNotesPart: (b) => { dshNotesCalled.push(b); return 'EN:' + b; },
      openCalls,
      schemes,
      notesCalled,
      dshNotesCalled,
    };
    return deps;
  };
  const show = (deps, kind) => {
    extract('showUpdateNowDialog', {
      state: deps.state,
      updateName: deps.updateName,
      tr: deps.tr,
      dialogs: deps.dialogs,
      updates: deps.updates,
      esc: deps.esc,
      notesBetween: deps.notesBetween,
      dshNotesPart: deps.dshNotesPart,
    })(kind);
    deps.render(); // the page is rendered on navigation — capture the html
  };

  // 13) dsh, non-empty releases: semver scheme, the EN part of the body.
  let sd = showDeps({});
  show(sd, 'dsh');
  assert.deepStrictEqual(sd.openCalls, ['Обновление dsh'], 'the dialog is opened for dsh');
  assert.deepStrictEqual(sd.schemes, ['semver'], 'dsh notes — the semver scheme');
  assert.ok(sd.pageHtml.includes('NOTES:v2|EN:B2'), 'dsh: the notes block carries the EN part');
  assert.deepStrictEqual(sd.notesCalled, ['semver'], 'notesBetween called once');
  assert.deepStrictEqual(sd.dshNotesCalled, ['B2'], 'dshNotesPart called for the body');
  assert.deepStrictEqual(sd.openCalls, ['Обновление dsh'], 'one openDialog call');
  assert.strictEqual(sd.state.dialogCopyText, '', 'the copy buffer is cleared');
  assert.strictEqual(sd.pageHtml.split('NOTES:').length - 1, 1, 'one notes block in the page');

  // 14) dsh, empty releases: notesBetween is NOT called, the notes block
  //     is empty.
  sd = showDeps({ availableUpdates: { dsh: { version: '0.2.2-rc.5', installed: '0.2.2-rc.1', releases: [] } } });
  show(sd, 'dsh');
  assert.deepStrictEqual(sd.schemes, [], 'dsh without releases — notesBetween not called');
  assert.deepStrictEqual(sd.notesCalled, [], 'notesBetween not called');
  assert.ok(sd.pageHtml.includes('NOTES:'), 'an empty notes block is rendered');
  assert.ok(!sd.pageHtml.includes('NOTES:v2'), 'no notes content');

  // 15) launcher: the deb scheme, the notes as-is (no EN part).
  sd = showDeps({ availableUpdates: { launcher: { version: '1.19.0', installed: '1.18.4', releases: [] } } });
  show(sd, 'launcher');
  assert.deepStrictEqual(sd.openCalls, ['Обновление DeepSeek Harness Launcher'], 'the dialog is opened for the launcher');
  assert.deepStrictEqual(sd.schemes, ['deb'], 'launcher notes — the deb scheme');
  assert.deepStrictEqual(sd.dshNotesCalled, [], 'dshNotesPart never called for the launcher');
  assert.strictEqual(sd.state.dialogCopyText, '', 'the copy buffer is cleared');
  assert.ok(sd.pageHtml.includes('NOTES:v2|B2'), 'launcher: the notes are passed as-is');

  // 16) no available update for the kind — nothing is opened, no crash.
  //     (the launcher has no availableUpdates entry in the shared state)
  sd = showDeps({});
  extract('showUpdateNowDialog', {
    state: sd.state,
    updateName: sd.updateName,
    tr: sd.tr,
    dialogs: sd.dialogs,
    updates: sd.updates,
    esc: sd.esc,
    notesBetween: sd.notesBetween,
    dshNotesPart: sd.dshNotesPart,
  })('launcher');
  assert.deepStrictEqual(sd.openCalls, [], 'no update — the dialog is not opened');
  assert.strictEqual(sd.render, undefined, 'no page render is registered');
  assert.deepStrictEqual(sd.schemes, [], 'no update — no notes');

  // 17) dialogs.openDialog returned null — no dialogShow / dialogPage.
  sd = showDeps({}, null);
  extract('showUpdateNowDialog', {
    state: sd.state,
    updateName: sd.updateName,
    tr: sd.tr,
    dialogs: sd.dialogs,
    updates: sd.updates,
    esc: sd.esc,
    notesBetween: sd.notesBetween,
    dshNotesPart: sd.dshNotesPart,
  })('dsh');
  assert.deepStrictEqual(sd.openCalls, ['Обновление dsh'], 'openDialog was tried');
  assert.strictEqual(sd.render, undefined, 'no dialogShow on a null generation');

  // 18) the hint: launcher, an externally started dsh — dsh keeps running;
  //     otherwise (the launcher started the dsh, or a dsh update) — it will
  //     be stopped.
  const launcherEntry = { version: '1.19.0', installed: '1.18.4', releases: [] };
  sd = showDeps({ availableUpdates: { launcher: launcherEntry }, weStartedDsh: false, adoptedDsh: false });
  show(sd, 'launcher');
  assert.ok(sd.pageHtml.includes('продолжит работать'), 'external dsh — it keeps running');
  assert.ok(!sd.pageHtml.includes('будет остановлен'), 'no stop warning for the external dsh');
  sd = showDeps({ availableUpdates: { launcher: launcherEntry }, weStartedDsh: true, adoptedDsh: false });
  show(sd, 'launcher');
  assert.ok(sd.pageHtml.includes('будет остановлен'), 'our dsh — it will be stopped');
  sd = showDeps({});
  show(sd, 'dsh');
  assert.ok(sd.pageHtml.includes('будет остановлен'), 'a dsh update — it will be stopped');

  // 19) the page: the versions in <code>, the «Обновить сейчас» (Update
  //     now) link for the kind and the «Позже» (Later) button.
  sd = showDeps({});
  show(sd, 'dsh');
  assert.ok(sd.pageHtml.includes('<code>0.2.2-rc.5</code>'), 'the available version in <code>');
  assert.ok(sd.pageHtml.includes('<code>0.2.2-rc.1</code>'), 'the installed version in <code>');
  assert.ok(sd.pageHtml.includes("dshlauncher://update-now/?k=dsh"), 'the «Update now» link for dsh');
  assert.ok(sd.pageHtml.includes("dshlauncher://dialog/close/"), 'the dialog-close link');
  assert.ok(sd.pageHtml.includes('Позже'), 'the «Later» button');

  console.log('OK: updates-live — runDeferredUpdate');
  console.log('OK: updates-live — showUpdateNowDialog');
}

main().catch((e) => { console.error('FAIL:', e.message); process.exit(1); });
