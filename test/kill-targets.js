#!/usr/bin/env node
// Юнит-тест pickKillTargets (main.js): что убивать при остановке
// «усыновлённого» dsh. Ошибка здесь — либо убитая группа самого лаунчера,
// либо оставшийся жить dsh.
'use strict';
const assert = require('assert');
const { extract } = require('./_extract');

const pick = extract('pickKillTargets');
const SELF = 100; // группа лаунчера

// Порт пуст, старой группы нет — убивать нечего
assert.deepStrictEqual(pick(null, null, SELF, null), [], 'ничего нет → []');

// Процесс на порту в своей (чужой для нас) группе — убиваем группу целиком
assert.deepStrictEqual(pick(500, 500, SELF, null), [{ id: 500, groupOnly: true }], 'лидер своей группы → группа');
// Не лидер: pid ≠ pgid — убиваем по pgid, иначе потомки останутся
assert.deepStrictEqual(pick(501, 500, SELF, null), [{ id: 500, groupOnly: true }], 'не лидер → его pgid');

// Процесс на порту в НАШЕЙ группе — только сам процесс, группу не трогаем
assert.deepStrictEqual(pick(501, SELF, SELF, null), [{ id: 501, groupOnly: false }], 'наша группа → только pid');
// pgid неизвестен (/proc не прочитался) — только сам процесс
assert.deepStrictEqual(pick(501, null, SELF, null), [{ id: 501, groupOnly: false }], 'pgid неизвестен → только pid');

// Старая группа исходного dsh добавляется отдельно
assert.deepStrictEqual(pick(600, 600, SELF, 500),
  [{ id: 600, groupOnly: true }, { id: 500, groupOnly: true }], 'новый dsh + старая группа');
// Новый процесс в той же группе, что старый, — без дубля
assert.deepStrictEqual(pick(601, 500, SELF, 500), [{ id: 500, groupOnly: true }], 'та же группа → без дубля');
// Порт уже пуст, но в старой группе могли остаться потомки
assert.deepStrictEqual(pick(null, null, SELF, 500), [{ id: 500, groupOnly: true }], 'только старая группа');
// Старая группа совпала с нашей (не должно быть, но) — свою группу не убиваем
assert.deepStrictEqual(pick(null, null, SELF, SELF), [], 'старая группа = наша → не трогаем');

console.log('OK: pickKillTargets — все проверки прошли');
