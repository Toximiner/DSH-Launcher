'use strict';
/*
 * test/watchdog.js — waitForReady (src/watchdog.js): the dsh port opens
 * before the app is ready, so the watchdog waits for checkReady() until timeout.
 * The function is extracted by markers (test/_extract.js); electron parts
 * (openRunningGui, pages) are not wired in.
 * Run: node test/watchdog.js (or npm test).
 */
const assert = require('assert');
const { extract } = require('./_extract');

const waitForReady = extract('waitForReady', {});

async function main() {
  // 1) already ready — true immediately
  assert.strictEqual(await waitForReady(async () => true, 100, () => false, 5), true, 'ready immediately');

  // 2) became ready after several polls — true
  let calls = 0;
  assert.strictEqual(await waitForReady(async () => {
    calls += 1;
    return calls >= 4;
  }, 1000, () => false, 5), true, 'ready after polls');
  assert.strictEqual(calls, 4, 'stopped at the first true');

  // 3) never became ready — timeout, false
  const t0 = Date.now();
  assert.strictEqual(await waitForReady(async () => false, 60, () => false, 5), false, 'timeout → false');
  assert.ok(Date.now() - t0 >= 55, 'waited until the timeout');

  // 4) launcher is closing — false without waiting for the timeout
  assert.strictEqual(await waitForReady(async () => false, 1000, () => true, 5), false, 'stop → false');

  // 5) checkReady throws (network not answering yet) — “not ready”, not propagated
  let threw = true;
  try {
    assert.strictEqual(await waitForReady(async () => { throw new Error('ECONNREFUSED'); }, 40, () => false, 5), false);
    threw = false;
  } catch { /* should not reach */ }
  assert.strictEqual(threw, false, 'an exception from checkReady is not propagated');

  console.log('OK: watchdog — waitForReady');
}

main().catch((e) => { console.error('FAIL:', e.message); process.exit(1); });
