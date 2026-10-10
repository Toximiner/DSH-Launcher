#!/usr/bin/env node
// Юнит-тесты периодической проверки обновлений (main.js): updatePollMs —
// интервал; announceUpdates — пункты меню и уведомления по найденным версиям.
'use strict';
const assert = require('assert');
const { mainSrc } = require('./_extract');

const { updatePollMs, announceUpdates } = new Function(
  mainSrc.slice(mainSrc.indexOf('/* ===== announceUpdates '), mainSrc.indexOf('/* ===== end announceUpdates ===== */')) +
  ';return { updatePollMs, announceUpdates };')();

// ---- updatePollMs ----
assert.strictEqual(updatePollMs({}), 12 * 3600 * 1000, 'по умолчанию — 12 часов');
assert.strictEqual(updatePollMs({ DSH_LAUNCHER_UPDATE_POLL_SEC: '3' }), 3000, 'тесты — секунды');
for (const bad of ['0', '-5', 'abc', '', '0.5']) {
  assert.strictEqual(updatePollMs({ DSH_LAUNCHER_UPDATE_POLL_SEC: bad }), 12 * 3600 * 1000, `«${bad}» — по умолчанию`);
}

// ---- announceUpdates ----
assert.deepStrictEqual(announceUpdates({}, {}, {}), { menu: {}, notify: [] }, 'ничего не найдено');
assert.deepStrictEqual(announceUpdates({ launcher: null, dsh: undefined }, {}, {}), { menu: {}, notify: [] }, 'null — нет обновления');
assert.deepStrictEqual(announceUpdates({ launcher: '1.8.0', dsh: '0.3.0' }, {}, {}),
  { menu: { launcher: '1.8.0', dsh: '0.3.0' }, notify: ['launcher', 'dsh'] }, 'оба — меню и уведомления');
assert.deepStrictEqual(announceUpdates({ launcher: '1.8.0' }, {}, { launcher: '1.8.0' }),
  { menu: { launcher: '1.8.0' }, notify: [] }, 'уже уведомляли — только меню');
assert.deepStrictEqual(announceUpdates({ launcher: '1.9.0' }, {}, { launcher: '1.8.0' }),
  { menu: { launcher: '1.9.0' }, notify: ['launcher'] }, 'вышла ещё более новая — снова уведомить');
assert.deepStrictEqual(announceUpdates({ dsh: '0.3.0' }, { dsh: '0.3.0' }, {}),
  { menu: { dsh: '0.3.0' }, notify: [] }, '«не спрашивать больше» — без уведомления, пункт меню есть');
assert.deepStrictEqual(announceUpdates({ dsh: '0.4.0' }, { dsh: '0.3.0' }, {}),
  { menu: { dsh: '0.4.0' }, notify: ['dsh'] }, 'отклонена старая — о новой уведомить');
assert.deepStrictEqual(announceUpdates({ dsh: '0.3.0' }, null, undefined),
  { menu: { dsh: '0.3.0' }, notify: ['dsh'] }, 'нет файлов — как пустые');

console.log('OK: updatePollMs, announceUpdates — все проверки прошли');
