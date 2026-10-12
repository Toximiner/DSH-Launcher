#!/usr/bin/env node
// Юнит-тесты отчёта для поддержки (src/ui.js): reportFileName — имя архива,
// reportInfoText — сведения «ключ: значение».
'use strict';
const assert = require('assert');
const { reportFileName, reportInfoText } = require('../src/ui');

assert.strictEqual(reportFileName(new Date(2026, 0, 5, 9, 7)), 'dsh-launcher-report-20260105-0907.tar.gz', 'нули впереди');
assert.strictEqual(reportFileName(new Date(2026, 11, 31, 23, 59)), 'dsh-launcher-report-20261231-2359.tar.gz', 'конец года');

assert.strictEqual(reportInfoText([['a', '1'], ['b', 2]]), 'a: 1\nb: 2\n', 'ключ: значение');
assert.strictEqual(reportInfoText([['x', undefined], ['y', null], ['z', '']]), 'x: —\ny: —\nz: —\n', 'нет значения — «—»');
assert.strictEqual(reportInfoText([['n', 0]]), 'n: 0\n', 'ноль — значение');

console.log('OK: reportFileName, reportInfoText — все проверки прошли');

// ---- pickCrashDumps: какие дампы падений — в отчёт ----
{
  const { pickCrashDumps } = require('../src/ui');
  const D = 86400000; const MB = 1024 * 1024; const now = 100 * D;
  const f = (name, daysAgo, mb) => ({ path: name, mtimeMs: now - daysAgo * D, size: mb * MB });
  const files = [f('old', 40, 1), f('a', 1, 2), f('b', 2, 2), f('c', 3, 2), f('d', 4, 2)];
  assert.deepStrictEqual(pickCrashDumps(files, now).map((x) => x.path), ['a', 'b', 'c'], 'свежие, новые первыми, не больше 3');
  assert.deepStrictEqual(pickCrashDumps(files, now, 30, 10).map((x) => x.path), ['a', 'b', 'c', 'd'], 'старше 30 дней — нет');
  assert.deepStrictEqual(pickCrashDumps([f('big', 1, 30), f('s', 2, 1)], now).map((x) => x.path), ['s'], 'больше 25 МБ вместе — пропускаем');
  assert.deepStrictEqual(pickCrashDumps([], now), [], 'дампов нет');
  assert.deepStrictEqual(pickCrashDumps(null, now), [], 'нет списка');
  console.log('OK: pickCrashDumps — все проверки прошли');
}
