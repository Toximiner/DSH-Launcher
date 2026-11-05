#!/usr/bin/env node
// Юнит-тесты src/diagnostics.js: rollbackSuggested — предлагать ли откат к
// предыдущей версии после аварийного запуска обновлённого лаунчера.
'use strict';
const assert = require('assert');
const { extract } = require('./_extract');

const rollbackSuggested = extract('rollbackSuggested', {});

const atIso = '2026-11-05T12:00:00.000Z'; // время обновления (update.at)
const T = Date.parse(atIso);
const iso = (ms) => new Date(T + ms).toISOString();
const MIN = 60000;
const m = (version, startedAt) => ({ version, startedAt });

// нет данных — не предлагаем
assert.strictEqual(rollbackSuggested(null, null, T), false, 'нет ни метки, ни обновления');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(5 * MIN)), null, T), false, 'нет записи об обновлении');
assert.strictEqual(rollbackSuggested(null, { to: '2.0.0-1', at: atIso }, T), false, 'нет метки прошлого запуска');

// другое приложение/версия — не предлагаем
assert.strictEqual(rollbackSuggested(m('1.1.0-1', iso(5 * MIN)), { to: '2.0.0-1', at: atIso }, T + 10 * MIN), false, 'версия запуска != обновлённая');
assert.strictEqual(rollbackSuggested({ version: 2, startedAt: iso(5 * MIN) }, { to: '2.0.0-1', at: atIso }, T + 10 * MIN), false, 'числовая версия в JSON — не совпадает');

// битые/отсутствующие даты — не предлагаем
assert.strictEqual(rollbackSuggested(m('2.0.0-1', 'битая дата'), { to: '2.0.0-1', at: atIso }, T), false, 'битая дата запуска');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(5 * MIN)), { to: '2.0.0-1', at: 'битая' }, T), false, 'битая дата обновления');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(5 * MIN)), { to: '2.0.0-1' }, T), false, 'нет даты обновления');

// окно 30 минут: срабатывает, когда проверка (now) в пределах окна после обновления
// И аварийный запуск начался не раньше обновления
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(-60000)), { to: '2.0.0-1', at: atIso }, T), false, 'аварийный запуск начался до обновления');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(0)), { to: '2.0.0-1', at: atIso }, T - 1000), false, 'проверка раньше обновления — вне окна');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(5 * MIN)), { to: '2.0.0-1', at: atIso }, T + 10 * MIN), true, 'падение в 10 минут после обновления');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(29 * MIN)), { to: '2.0.0-1', at: atIso }, T + 29 * MIN), true, 'на границе окна (29 минут) — предлагаем');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(30 * MIN)), { to: '2.0.0-1', at: atIso }, T + 31 * MIN), false, '31 минута после обновления — вне окна');
assert.strictEqual(rollbackSuggested(m('2.0.0-1', iso(MIN)), { to: '2.0.0-1', at: atIso }, T + 10 * 60 * MIN), false, '10 часов после обновления — вне окна');

console.log('OK: diagnostics — rollbackSuggested');
