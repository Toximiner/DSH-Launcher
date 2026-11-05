#!/usr/bin/env node
// Unit tests of the "agent finished / waiting for you" signal (src/ui.js):
// agentEvent — what changed in the dsh GUI between two checks.
'use strict';
const assert = require('assert');
const { agentEvent } = require('../src/ui');

const s = (running, waiting = null) => ({ running, waiting });

assert.strictEqual(agentEvent(s(false), s(false)), null, 'nothing happens');
assert.strictEqual(agentEvent(s(false), s(true)), null, 'started replying — not reported');
assert.deepStrictEqual(agentEvent(s(true), s(false)), { type: 'finished' }, 'finished replying');
assert.deepStrictEqual(agentEvent(s(true), s(true, 'approval')), { type: 'waiting', kind: 'approval' }, 'waits for command approval');
assert.deepStrictEqual(agentEvent(s(true), s(false, 'question')), { type: 'waiting', kind: 'question' }, 'asked a question (reply stopped)');
assert.deepStrictEqual(agentEvent(s(false), s(false, 'plan')), { type: 'waiting', kind: 'plan' }, 'waits for plan approval');
assert.strictEqual(agentEvent(s(true, 'approval'), s(true, 'approval')), null, 'same wait — not reported again');
assert.deepStrictEqual(agentEvent(s(true, 'approval'), s(true, 'question')), { type: 'waiting', kind: 'question' }, 'new wait of another kind');
assert.strictEqual(agentEvent(s(true, 'approval'), s(true)), null, 'approved — agent continues');
assert.deepStrictEqual(agentEvent(s(true, 'approval'), s(false)), { type: 'finished' }, 'approved, and the reply is finished');
assert.strictEqual(agentEvent(s(false, 'question'), s(false)), null, 'answered the question, the agent has not started yet');

console.log('OK: agentEvent — all checks passed');

// ---- crashRestart: whether to restart dsh after a crash ----
{
  const { crashRestart } = require('../src/ui');
  const M = 60 * 1000;
  let r = crashRestart([], 0, 1, null);
  assert.deepStrictEqual(r, { restart: true, history: [0] }, 'crashed (code 1) — restart');
  assert.strictEqual(crashRestart([], 0, null, 'SIGSEGV').restart, true, 'crashed by signal — restart');
  assert.strictEqual(crashRestart([], 0, 0, null).restart, false, 'normal exit (code 0) — no restart');
  r = crashRestart([0, 1 * M], 2 * M, 1, null);
  assert.deepStrictEqual(r, { restart: true, history: [0, 1 * M, 2 * M] }, 'third time in 10 minutes — still allowed');
  r = crashRestart([0, 1 * M, 2 * M], 3 * M, 1, null);
  assert.strictEqual(r.restart, false, 'fourth in 10 minutes — enough');
  r = crashRestart([0, 1 * M, 2 * M], 11 * M, 1, null);
  assert.deepStrictEqual(r, { restart: true, history: [2 * M, 11 * M] }, 'old ones (more than 10 minutes ago) are not counted');
  assert.strictEqual(crashRestart(null, 0, 1, null).restart, true, 'no history');
  console.log('OK: crashRestart — all checks passed');
}
