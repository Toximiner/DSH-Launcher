'use strict';
// Unit tests for the typed-input history (src/input-history.js): the pure ↑/↓ logic
// (historyPush/historyStep/historyExit) and the injection script text.
const assert = require('assert');
const { extract } = require('./_extract');

const historyPush = extract('historyPush', {});
const historyStep = extract('historyStep', {});
const historyExit = extract('historyExit', {});
const buildInputHistoryScript = extract('buildInputHistoryScript', {});

// ---- historyPush: newest on top, consecutive repeats not duplicated ----
assert.deepStrictEqual(historyPush([], 'привет'), ['привет']);
assert.deepStrictEqual(historyPush(['c', 'b', 'a'], 'd'), ['d', 'c', 'b', 'a']);
assert.deepStrictEqual(historyPush(['c', 'b'], 'c'), ['c', 'b'], 'a repeat of the top entry is not pushed');
assert.deepStrictEqual(historyPush(['b'], ''), ['b'], 'an empty string is not pushed');
assert.deepStrictEqual(historyPush(['b'], '   '), ['b'], 'whitespace is not pushed');
assert.deepStrictEqual(historyPush(['b'], '  b  '), ['b'], 'whitespace is trimmed');
assert.deepStrictEqual(historyPush(['b'], null), ['b']);
assert.deepStrictEqual(historyPush(['b'], 'c', 1), ['c'], 'list limit');

const fifty = Array.from({ length: 50 }, (_, i) => 'm' + i);
const capped = historyPush(fifty, 'новое');
assert.strictEqual(capped.length, 50, 'more than 50 entries are not stored');
assert.strictEqual(capped[0], 'новое');
assert.ok(!capped.includes('m49'), 'the oldest entry is evicted');

// ---- historyStep: ↑/↓ mode ----
const idle = { active: false, index: -1, saved: '' };
assert.deepStrictEqual(historyStep([], idle, -1, { canEnter: true, current: '' }).action, { type: 'none' }, 'empty history');
assert.deepStrictEqual(historyStep(['a'], idle, 1, { canEnter: true, current: '' }).action, { type: 'none' }, '↓ outside the mode does nothing');
assert.deepStrictEqual(historyStep(['a'], idle, -1, { canEnter: false, current: 'текст' }).action, { type: 'none' }, 'cursor not at the start — not intercepted');

let r = historyStep(['a', 'b'], idle, -1, { canEnter: true, current: '' });
assert.deepStrictEqual(r.nav, { active: true, index: 0, saved: '' });
assert.deepStrictEqual(r.action, { type: 'set', text: 'a' });

r = historyStep(['a', 'b'], idle, -1, { canEnter: true, current: 'черновик' });
assert.deepStrictEqual(r.nav, { active: true, index: 0, saved: 'черновик' }, 'the draft is saved');
assert.deepStrictEqual(r.action, { type: 'set', text: 'a' });

const at0 = { active: true, index: 0, saved: 'черновик' };
r = historyStep(['a', 'b'], at0, -1, { canEnter: false, current: 'a' });
assert.deepStrictEqual(r.nav, { active: true, index: 1, saved: 'черновик' });
assert.deepStrictEqual(r.action, { type: 'set', text: 'b' });

assert.deepStrictEqual(historyStep(['a', 'b'], r.nav, -1, { canEnter: false, current: 'b' }).action, { type: 'none' }, 'nothing older — do not interfere');
assert.deepStrictEqual(historyStep(['a', 'b'], r.nav, 1, { canEnter: false, current: 'b' }).action, { type: 'set', text: 'a' }, '↓ back to the newer entry');

r = historyStep(['a', 'b'], at0, 1, { canEnter: false, current: 'a' });
assert.deepStrictEqual(r.nav, { active: false, index: -1, saved: '' });
assert.deepStrictEqual(r.action, { type: 'restore', text: 'черновик' }, 'below the newest — restore the draft');

const noDraft = { active: true, index: 0, saved: '' };
assert.deepStrictEqual(historyStep(['a'], noDraft, 1, { canEnter: false, current: 'a' }).action, { type: 'restore', text: '' });

// ---- historyExit: Esc restores the draft ----
assert.deepStrictEqual(historyExit(idle).action, { type: 'none' }, 'outside the mode Esc is free');
r = historyExit({ active: true, index: 1, saved: 'черновик' });
assert.deepStrictEqual(r.nav, { active: false, index: -1, saved: '' });
assert.deepStrictEqual(r.action, { type: 'restore', text: 'черновик' });
assert.deepStrictEqual(historyExit({ active: true, index: 0, saved: '' }).action, { type: 'restore', text: '' });

// ---- buildInputHistoryScript: injection text for the dsh page ----
const js = buildInputHistoryScript('abc123', ['второе', 'первое']);
assert.strictEqual(typeof js, 'string');
assert.ok(js.includes('[data-composer-input="true"]'), 'the dsh composer selector');
assert.ok(js.includes('beforeinput'), 'insertion via beforeinput (Lexical)');
assert.ok(js.includes('abc123'), 'the route key');
assert.ok(js.includes('второе'), 'the current history is passed through');
assert.ok(js.includes('history/push/'), 'the write route into main');
assert.ok(js.includes('__dshInputHistory'), 'the control point for the settings');
assert.ok(buildInputHistoryScript('', null).includes('[]'), 'an empty history does not break the script');
assert.ok(buildInputHistoryScript().includes('"key":""'), 'no key — empty string, not undefined');

console.log('OK: input-history — historyPush/historyStep/historyExit/buildInputHistoryScript');
