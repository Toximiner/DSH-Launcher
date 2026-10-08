'use strict';

/*
 * Юнит-тесты чистых функций сравнения версий из main.js (используются
 * проверкой обновлений лаунчера и dsh). Функции извлекаются из исходника
 * между маркерами «сравнение версий» — если маркеры убрать, тест падает.
 *
 * Запуск: node test/version.js   (npm test)
 */

const assert = require('assert');
const fs = require('fs');
const path = require('path');

const src = fs.readFileSync(path.join(__dirname, '..', 'main.js'), 'utf8');
const lines = src.split('\n');
const start = lines.findIndex((l) => l.includes('сравнение версий'));
const end = lines.findIndex((l) => l.includes('конец сравнения версий'));
assert.ok(start >= 0 && end > start, 'main.js: не найдены маркеры «сравнение версий»');
const code = lines.slice(start, end + 1).join('\n');
const helpers = new Function(`${code}; return { compareDebianVer, compareSemVer, tagToPackageVersion };`)();
const { compareDebianVer, compareSemVer, tagToPackageVersion } = helpers;

// Debian (лаунчер)
assert.strictEqual(compareDebianVer('1.1.1-1', '1.1.1-1'), 0, 'debian: равные');
assert.ok(compareDebianVer('1.1.2-1', '1.1.1-1') > 0, 'debian: выше upstream');
assert.ok(compareDebianVer('1.1.1-1', '1.1.2-1') < 0, 'debian: ниже upstream');
assert.ok(compareDebianVer('1.1.1-2', '1.1.1-1') > 0, 'debian: выше ревизия');
assert.ok(compareDebianVer('1.1.1', '1.1.1-1') < 0, 'debian: без ревизии < с ревизией');
assert.ok(compareDebianVer('2.0.0-1', '1.9.9-9') > 0, 'debian: major');
assert.strictEqual(compareDebianVer('weird', '1.0.0'), 0, 'debian: не распознано → 0');

// semver (dsh)
assert.strictEqual(compareSemVer('0.2.0-rc.2', '0.2.0-rc.2'), 0, 'semver: равные');
assert.ok(compareSemVer('0.2.0', '0.2.0-rc.2') > 0, 'semver: релиз > rc');
assert.ok(compareSemVer('0.2.0-rc.2', '0.2.0') < 0, 'semver: rc < релиз');
assert.ok(compareSemVer('0.2.1-rc.1', '0.2.0') > 0, 'semver: rc следующего upstream выше');
assert.ok(compareSemVer('0.2.0-rc.10', '0.2.0-rc.2') > 0, 'semver: rc.10 > rc.2 (число, не строка)');
assert.ok(compareSemVer('0.2.0-alpha', '0.2.0-1') > 0, 'semver: буквенный сегмент > числовой');
assert.ok(compareSemVer('1.0.0-rc.1', '1.0.0-beta.2') > 0, 'semver: rc > beta');
assert.strictEqual(compareSemVer('0.2.0-rc.2', 'garbage'), 0, 'semver: не распознано → 0');

// тег GitHub → версия пакета (правило CI)
assert.strictEqual(tagToPackageVersion('v1.1.1'), '1.1.1-1', 'tag: vX.Y.Z → X.Y.Z-1');
assert.strictEqual(tagToPackageVersion('v1.1.1-R'), '1.1.1-R', 'tag: vX.Y.Z-R → X.Y.Z-R');
assert.strictEqual(tagToPackageVersion('2.0.0'), '2.0.0-1', 'tag: без v');

console.log('OK: сравнение версий — все проверки прошли');
