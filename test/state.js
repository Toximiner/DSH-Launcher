'use strict';
/*
 * test/state.js — общий стейт (src/state.js): регистрация и вызов функций
 * очистки settleOperation/settleAllOperations (before-quit вызывает их все,
 * не зная списка модулей).
 * Run: node test/state.js (or npm test).
 */
const assert = require('assert');
const state = require('../src/state');

const order = [];
state.settleOperation(() => { order.push('a'); });
state.settleOperation(() => { order.push('b'); });
state.settleAllOperations();
assert.deepStrictEqual(order, ['a', 'b'], 'клинеры вызваны в порядке регистрации');
state.settleAllOperations();
assert.deepStrictEqual(order, ['a', 'b', 'a', 'b'], 'повторный вызов — все клинеры снова');

console.log('OK: state — all checks passed');
