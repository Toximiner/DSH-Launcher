'use strict';

/*
 * Юнит-тесты подготовки релиза (release.js, npm run release X.Y.Z): раздел
 * CHANGELOG, версии в package.json / package-lock.json / deb/control.
 *
 * Запуск: node test/release.js   (npm test)
 */

const assert = require('assert');
const fs = require('fs');
const path = require('path');
const { checkVersion, releaseChangelog, setControlVersion, setJsonVersion, today } = require('../release');

// ---- checkVersion ----
checkVersion('1.16.0', '1.15.1');
checkVersion('1.15.10', '1.15.9');
checkVersion('2.0.0', null);
assert.throws(() => checkVersion('1.16', '1.15.1'), /X\.Y\.Z/, 'не X.Y.Z');
assert.throws(() => checkVersion('v1.16.0', '1.15.1'), /X\.Y\.Z/, 'с «v» — нет');
assert.throws(() => checkVersion(undefined, '1.15.1'), /X\.Y\.Z/, 'версия не указана');
assert.throws(() => checkVersion('1.15.1', '1.15.1'), /не выше/, 'та же версия');
assert.throws(() => checkVersion('1.9.0', '1.15.1'), /не выше/, 'ниже (число, не строка)');

// ---- releaseChangelog ----
const CL = `# Изменения

Шапка.

## [Unreleased]

### Исправлено

- Что-то.

## [1.15.1] — 2026-10-12

- Старое.
`;
{
  const out = releaseChangelog(CL, '1.16.0', '2026-10-13');
  assert.ok(out.includes('\n## [1.16.0] — 2026-10-13\n\n### Исправлено\n\n- Что-то.\n\n## [1.15.1]'), 'Unreleased → версия и дата');
  assert.ok(!out.includes('Unreleased'), 'раздела Unreleased больше нет');
  assert.strictEqual(out.split('\n').length, CL.split('\n').length, 'остальное не тронуто');
}
assert.throws(() => releaseChangelog(CL.replace('## [Unreleased]\n', ''), '1.16.0', '2026-10-13'), /нет раздела/, 'нет Unreleased');
assert.throws(() => releaseChangelog(CL.replace('### Исправлено\n\n- Что-то.\n', ''), '1.16.0', '2026-10-13'), /пуст/, 'пустой Unreleased');
assert.throws(() => releaseChangelog(CL.replace('- Что-то.\n', ''), '1.16.0', '2026-10-13'), /пуст/, 'только подзаголовок — пусто');
assert.throws(() => releaseChangelog(CL, '1.15.1', '2026-10-13'), /уже есть/, 'раздел версии уже есть');
assert.ok(releaseChangelog('## [Unreleased]\n- Последний раздел.\n', '1.0.0', '2026-01-01').startsWith('## [1.0.0] — 2026-01-01\n'), 'Unreleased — последний раздел');

// ---- setControlVersion ----
assert.strictEqual(setControlVersion('Package: dsh-launcher\nVersion: 1.15.1-1\nArchitecture: amd64\n', '1.16.0'),
  'Package: dsh-launcher\nVersion: 1.16.0-1\nArchitecture: amd64\n', 'deb/control: X.Y.Z-1');
assert.throws(() => setControlVersion('Package: x\n', '1.16.0'), /Version/, 'нет строки Version');

// ---- setJsonVersion ----
{
  const lock = { name: 'dsh-launcher', version: '1.15.1', lockfileVersion: 3, packages: { '': { name: 'dsh-launcher', version: '1.15.1' }, 'node_modules/x': { version: '1.15.1' } } };
  const out = JSON.parse(setJsonVersion(JSON.stringify(lock, null, 2) + '\n', '1.16.0', true));
  assert.strictEqual(out.version, '1.16.0', 'lock: корень');
  assert.strictEqual(out.packages[''].version, '1.16.0', 'lock: packages[""]');
  assert.strictEqual(out.packages['node_modules/x'].version, '1.15.1', 'lock: зависимости с той же версией не тронуты');
  assert.ok(setJsonVersion('{"version":"1.0.0"}', '1.1.0', false).endsWith('}\n'), 'перевод строки в конце');
}
// формат как у npm: настоящие файлы проекта переписываются байт в байт
for (const [n, lock] of [['package.json', false], ['package-lock.json', true]]) {
  const t = fs.readFileSync(path.join(__dirname, '..', n), 'utf8');
  assert.strictEqual(setJsonVersion(t, JSON.parse(t).version, lock), t, `${n}: та же версия — файл не меняется`);
}
// настоящие CHANGELOG — оба в формате, который понимает release.js
for (const n of ['CHANGELOG.md', 'CHANGELOG.en.md']) {
  const t = fs.readFileSync(path.join(__dirname, '..', n), 'utf8');
  assert.ok(/^## \[\d+\.\d+\.\d+\] — \d{4}-\d{2}-\d{2}$/m.test(t), `${n}: разделы «## [X.Y.Z] — ГГГГ-ММ-ДД»`);
}

// ---- today ----
assert.strictEqual(today(new Date(2026, 0, 5)), '2026-01-05', 'дата с нулями');

console.log('OK: release — все проверки прошли');
