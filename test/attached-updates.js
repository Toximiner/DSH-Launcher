'use strict';
const assert = require('assert');
const { extract } = require('./_extract');

async function run(updateNow, installed = false, stopAt = '') {
  const calls = [];
  const state = { stopping: stopAt === 'initial' };
  const attachRunningDsh = extract('attachRunningDsh', {
    UPDATE_NOW: updateNow, state,
    offerLauncherUpdate: async () => {
      calls.push('install');
      if (stopAt === 'install') state.stopping = true;
      return installed;
    },
    openRunningGui: async () => {
      calls.push('gui');
      if (stopAt === 'gui') state.stopping = true;
    },
    startWatchdog: () => calls.push('watchdog'),
    deferredDshUpdateInAttach: () => { calls.push('deferred-dsh'); },
    refreshAvailableUpdates: (notify) => calls.push(['updates', notify]),
  });
  await attachRunningDsh();
  return calls;
}

async function main() {
  const attached = ['gui', 'watchdog', ['updates', true]];
  assert.deepStrictEqual(await run(null), attached, 'normal attach announces updates without prompting');
  assert.deepStrictEqual(await run('launcher', true), ['install'], 'explicit update installs before attaching and waits for the new version');
  assert.deepStrictEqual(await run('launcher', false), ['install', ...attached], 'cancelled, unavailable or failed update can return to the GUI');
  assert.deepStrictEqual(await run('launcher', false, 'install'), ['install'], 'closing during install never opens the GUI');
  assert.deepStrictEqual(await run(null, false, 'initial'), [], 'closing before attach does no work');
  assert.deepStrictEqual(await run(null, false, 'gui'), ['gui'], 'closing during navigation never starts notifications');
  assert.deepStrictEqual(await run('dsh'), ['gui', 'watchdog', 'deferred-dsh', ['updates', true]], 'explicit dsh update runs the deferred attach-mode update after the GUI');
  console.log('OK: attached updates — explicit installation, startup notifications and shutdown');
}
main().catch((error) => { console.error(error); process.exitCode = 1; });
