#!/usr/bin/env node
// Unit tests for the support report (src/ui.js): reportFileName — the archive name,
// reportInfoText — "key: value" info lines.
'use strict';
const assert = require('assert');
const { reportFileName, reportInfoText } = require('../src/ui');

assert.strictEqual(reportFileName(new Date(2026, 0, 5, 9, 7)), 'dsh-launcher-report-20260105-0907.tar.gz', 'leading zeros');
assert.strictEqual(reportFileName(new Date(2026, 11, 31, 23, 59)), 'dsh-launcher-report-20261231-2359.tar.gz', 'end of the year');

assert.strictEqual(reportInfoText([['a', '1'], ['b', 2]]), 'a: 1\nb: 2\n', 'key: value');
assert.strictEqual(reportInfoText([['x', undefined], ['y', null], ['z', '']]), 'x: —\ny: —\nz: —\n', 'no value — «—»');
assert.strictEqual(reportInfoText([['n', 0]]), 'n: 0\n', 'zero is a value');

console.log('OK: reportFileName, reportInfoText — all checks passed');

// ---- pickCrashDumps: which crash dumps go into the report ----
{
  const { pickCrashDumps } = require('../src/ui');
  const D = 86400000; const MB = 1024 * 1024; const now = 100 * D;
  const f = (name, daysAgo, mb) => ({ path: name, mtimeMs: now - daysAgo * D, size: mb * MB });
  const files = [f('old', 40, 1), f('a', 1, 2), f('b', 2, 2), f('c', 3, 2), f('d', 4, 2)];
  assert.deepStrictEqual(pickCrashDumps(files, now).map((x) => x.path), ['a', 'b', 'c'], 'the fresh ones, newest first, at most 3');
  assert.deepStrictEqual(pickCrashDumps(files, now, 30, 10).map((x) => x.path), ['a', 'b', 'c', 'd'], 'older than 30 days — excluded');
  assert.deepStrictEqual(pickCrashDumps([f('big', 1, 30), f('s', 2, 1)], now).map((x) => x.path), ['s'], 'more than 25 MB in total — skipped');
  assert.deepStrictEqual(pickCrashDumps([], now), [], 'no dumps');
  assert.deepStrictEqual(pickCrashDumps(null, now), [], 'no list');
  console.log('OK: pickCrashDumps — all checks passed');
}
