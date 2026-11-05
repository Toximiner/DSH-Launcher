#!/usr/bin/env node
// Unit tests for the live log on status pages (src/pages.js): liveLogAppend
// — appending lines on the page (on a fake <pre>), liveLogFresh — which
// lines to deliver to the page. The page is no longer reloaded, so
// scrolling is left to the user.
'use strict';
const assert = require('assert');
const { extract } = require('./_extract');

// A fake document with a single <pre id="live-log">.
const mkDoc = () => {
  const pre = { textContent: '…', scrollTop: 0, clientHeight: 100, scrollHeight: 100 };
  return { pre, document: { getElementById: (id) => (id === 'live-log' ? pre : null) } };
};

const append = extract('liveLogAppend', {});
const fresh = extract('liveLogFresh', {});

// No <pre> on the page (already navigated away) — false
{
  const doc = mkDoc();
  doc.document.getElementById = () => null;
  assert.strictEqual(append(doc.document, ['x']), false, 'no pre — false');
}
// Log empty (placeholder "…") — the first lines are written without the placeholder
{
  const { pre, document } = mkDoc();
  assert.strictEqual(append(document, ['a', 'b']), true, 'pre exists — true');
  assert.strictEqual(pre.textContent, 'a\nb', 'the placeholder is replaced');
}
// Appending keeps the old lines
{
  const { pre, document } = mkDoc();
  append(document, ['a']);
  append(document, ['b']);
  assert.strictEqual(pre.textContent, 'a\nb', 'old lines + new lines');
}
// User pinned to the bottom — stays pinned after new lines
{
  const { pre, document } = mkDoc();
  append(document, ['a', 'b', 'c']);
  pre.scrollTop = pre.scrollHeight - pre.clientHeight;
  assert.strictEqual(append(document, ['d']), true);
  assert.strictEqual(pre.scrollTop, pre.scrollHeight, 'scroll is pinned to the bottom');
}
// User reading not at the bottom — scroll is left alone
{
  const { pre, document } = mkDoc();
  append(document, ['a', 'b', 'c']);
  pre.scrollTop = 10;
  pre.scrollHeight = 500;
  pre.clientHeight = 100;
  assert.strictEqual(append(document, ['d']), true);
  assert.strictEqual(pre.scrollTop, 10, 'the scroll position is preserved');
}
// 500-line limit: on overflow, old lines drop from the top
{
  const { pre, document } = mkDoc();
  append(document, Array.from({ length: 500 }, (_, i) => `l${i}`));
  append(document, ['last']);
  const ls = pre.textContent.split('\n');
  assert.strictEqual(ls.length, 500, 'no more than 500 lines');
  assert.strictEqual(ls[0], 'l1', 'the first old line dropped');
  assert.strictEqual(ls[499], 'last', 'the new lines are at the bottom');
}
// liveLogFresh: new lines since the last render
{
  const st = { log: ['a', 'b', 'c'], logSeq: 3 };
  assert.strictEqual(fresh(st, 3), null, 'no new lines — null');
  assert.deepStrictEqual(fresh(st, 0), ['a', 'b', 'c'], 'from the start — all of them');
  assert.deepStrictEqual(fresh(st, 2), ['c'], 'from the last shown — only the new ones');
}
// liveLogFresh: log trimmed to 500 lines (logSeq > length) — indexing from logSeq
{
  const log = Array.from({ length: 500 }, (_, i) => String(i + 101)); // lines 101…600
  const st = { log, logSeq: 600 };
  assert.strictEqual(fresh(st, 100).length, 500, 'the whole remainder of the window (1…100 left the log but were already shown)');
  assert.deepStrictEqual(fresh(st, 100).slice(-1), ['600'], 'the new lines are at the end');
  assert.strictEqual(fresh(st, 499).length, 101, 'lines 500…600 — the entire remainder after shown');
  assert.strictEqual(fresh(st, 499)[0], '500', 'the first one — after shown');
  assert.deepStrictEqual(fresh(st, 599), ['600'], 'the last line after trimming');
}

console.log('OK: liveLogAppend, liveLogFresh — all checks passed');
