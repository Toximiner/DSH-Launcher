#!/usr/bin/env node
// Unit test for pickKillTargets (main.js): what to kill when stopping
// the "adopted" dsh. A mistake here — either the launcher's own group
// gets killed, or a dsh left alive.
'use strict';
const assert = require('assert');

const { pickKillTargets: pick } = require('../src/system');
const SELF = 100; // the launcher's group

// Port is free, no old group — nothing to kill
assert.deepStrictEqual(pick(null, null, SELF, null), [], 'nothing to kill → []');

// Process on the port in its own (foreign to us) group — kill the whole group
assert.deepStrictEqual(pick(500, 500, SELF, null), [{ id: 500, groupOnly: true }], 'leader of its own group → the group');
// Not a leader: pid ≠ pgid — kill by pgid, otherwise descendants survive
assert.deepStrictEqual(pick(501, 500, SELF, null), [{ id: 500, groupOnly: true }], 'not the leader → its pgid');

// Process on the port in OUR group — only the process itself, leave the group
assert.deepStrictEqual(pick(501, SELF, SELF, null), [{ id: 501, groupOnly: false }], 'our group → only the pid');
// pgid unknown (/proc unreadable) — only the process itself
assert.deepStrictEqual(pick(501, null, SELF, null), [{ id: 501, groupOnly: false }], 'pgid unknown → only the pid');

// The old group of the original dsh is added separately
assert.deepStrictEqual(pick(600, 600, SELF, 500),
  [{ id: 600, groupOnly: true }, { id: 500, groupOnly: true }], 'new dsh + the old group');
// New process in the same group as the old one — no duplicate
assert.deepStrictEqual(pick(601, 500, SELF, 500), [{ id: 500, groupOnly: true }], 'same group → no duplicate');
// Port already free, but descendants may remain in the old group
assert.deepStrictEqual(pick(null, null, SELF, 500), [{ id: 500, groupOnly: true }], 'only the old group');
// Old group coincided with ours (shouldn't happen, but) — we don't kill our own group
assert.deepStrictEqual(pick(null, null, SELF, SELF), [], 'old group == ours → leave it alone');

console.log('OK: pickKillTargets — all checks passed');
