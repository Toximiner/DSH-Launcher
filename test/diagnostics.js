#!/usr/bin/env node
// Unit tests of src/diagnostics.js: rollbackSuggested — whether to offer
// a rollback to the previous version after a crash start of the updated
// launcher.
'use strict';
const assert = require('assert');
const { extract } = require('./_extract');

const rollbackSuggested = extract('rollbackSuggested', {});

const atIso = '2026-11-05T12:00:00.000Z'; // the update time (update.at)
const T = Date.parse(atIso);
const iso = (ms) => new Date(T + ms).toISOString();
const MIN = 60000;
const m = (version, startedAt) => ({ version, startedAt });

// no data — we don't offer
assert.strictEqual(rollbackSuggested(null, null, T), false, 'neither a mark nor an update');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(5 * MIN)), null, T), false, 'no update record');
assert.strictEqual(rollbackSuggested(null, { to: '2.0.0-1', at: atIso }, T), false, 'no mark of the previous launch');

// another app/version — we don't offer
assert.strictEqual(rollbackSuggested(m('1.1.0-1', iso(5 * MIN)), { to: '2.0.0-1', at: atIso }, T + 10 * MIN), false, 'launch version != updated version');
assert.strictEqual(rollbackSuggested({ version: 2, startedAt: iso(5 * MIN) }, { to: '2.0.0-1', at: atIso }, T + 10 * MIN), false, 'numeric version in JSON — does not match');

// broken/missing dates — we don't offer
assert.strictEqual(rollbackSuggested(m('2.0.0-1', 'битая дата'), { to: '2.0.0-1', at: atIso }, T), false, 'broken launch date');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(5 * MIN)), { to: '2.0.0-1', at: 'битая' }, T), false, 'broken update date');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(5 * MIN)), { to: '2.0.0-1' }, T), false, 'no update date');

// the 30-minute window: it fires when the check (now) is within the window
// after the update and the crash start began no earlier than the update
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(-60000)), { to: '2.0.0-1', at: atIso }, T), false, 'the crash start began before the update');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(0)), { to: '2.0.0-1', at: atIso }, T - 1000), false, 'check before the update — outside the window');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(5 * MIN)), { to: '2.0.0-1', at: atIso }, T + 10 * MIN), true, 'crash 10 minutes after the update');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(29 * MIN)), { to: '2.0.0-1', at: atIso }, T + 29 * MIN), true, 'at the window edge (29 minutes) — we offer');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(30 * MIN)), { to: '2.0.0-1', at: atIso }, T + 31 * MIN), false, '31 minutes after the update — outside the window');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(MIN)), { to: '2.0.0-1', at: atIso }, T + 10 * 60 * MIN), false, '10 hours after the update — outside the window');

console.log('OK: diagnostics — rollbackSuggested');
