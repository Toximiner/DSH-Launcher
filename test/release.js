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
const os = require('os');
const { execFileSync } = require('child_process');
const { main, checkVersion, releaseChangelog, setControlVersion, setJsonVersion, today } = require('../release');

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
  assert.ok(out.includes('\n## [Unreleased]\n\n<!-- сюда дописать'), 'создан новый пустой [Unreleased]');
  assert.ok(out.indexOf('## [Unreleased]') < out.indexOf('## [1.16.0] — 2026-10-13'), 'новый [Unreleased] — выше версии');
  assert.strictEqual(out.split('\n').length, CL.split('\n').length + 4, 'остальное не тронуто, добавлен новый [Unreleased]');
}
assert.throws(() => releaseChangelog(CL.replace('## [Unreleased]\n', ''), '1.16.0', '2026-10-13'), /нет раздела/, 'нет Unreleased');
assert.throws(() => releaseChangelog(CL.replace('### Исправлено\n\n- Что-то.\n', ''), '1.16.0', '2026-10-13'), /пуст/, 'пустой Unreleased');
assert.throws(() => releaseChangelog(CL.replace('- Что-то.\n', ''), '1.16.0', '2026-10-13'), /пуст/, 'только подзаголовок — пусто');
assert.throws(() => releaseChangelog(CL, '1.15.1', '2026-10-13'), /уже есть/, 'раздел версии уже есть');
assert.ok(releaseChangelog('## [Unreleased]\n- Последний раздел.\n', '1.0.0', '2026-01-01').includes('## [1.0.0] — 2026-01-01\n- Последний раздел.\n'), 'Unreleased — последний раздел');
assert.throws(() => releaseChangelog(releaseChangelog(CL, '1.16.0', '2026-10-13'), '1.16.1', '2026-10-14'), /пуст/, 'авто-созданный [Unreleased] пуст (комментарий не считается)');
{
  // Служебный комментарий [Unreleased] не переносится в раздел версии:
  // 1.17.1 — плейсхолдер «сюда дописать» ушёл в описание GitHub Release.
  const CL2 = CL.replace('## [Unreleased]\n\n### Исправлено\n',
    '## [Unreleased]\n\n<!-- сюда дописать изменения для следующего релиза (### Добавлено / Изменено / Исправлено) -->\n\n### Исправлено\n');
  const out = releaseChangelog(CL2, '1.16.0', '2026-10-13');
  const verSection = out.split('\n## [1.16.0]')[1].split('\n## [1.15.1]')[0]; // раздел версии
  assert.ok(!verSection.includes('дописать'), 'плейсхолдер не перенесён в версию');
  assert.ok(out.includes('\n## [1.16.0] — 2026-10-13\n\n### Исправлено\n\n- Что-то.\n'), 'содержимое раздела на месте');
  assert.ok(!/^<!--/m.test(verSection), 'в разделе версии комментариев нет');
  assert.strictEqual(out.split('\n').length, CL2.split('\n').length + 2, 'удалены только комментарий и лишняя пустая строка');
}

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

// ---- main: весь выпуск на временном git-репозитории ----
{
  // свой git без глобальных настроек пользователя (подпись коммитов, хуки …)
  Object.assign(process.env, { GIT_CONFIG_GLOBAL: '/dev/null', GIT_CONFIG_NOSYSTEM: '1',
    GIT_AUTHOR_NAME: 't', GIT_AUTHOR_EMAIL: 't@t', GIT_COMMITTER_NAME: 't', GIT_COMMITTER_EMAIL: 't@t' });
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'dshl-release-'));
  const git = (...a) => execFileSync('git', a, { cwd: dir, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
  const write = (n, t) => { fs.mkdirSync(path.dirname(path.join(dir, n)), { recursive: true }); fs.writeFileSync(path.join(dir, n), t); };
  const read = (n) => fs.readFileSync(path.join(dir, n), 'utf8');
  const json = (v) => JSON.stringify({ name: 'x', version: v }, null, 2) + '\n';
  const lock = (v) => JSON.stringify({ name: 'x', version: v, lockfileVersion: 3, packages: { '': { name: 'x', version: v } } }, null, 2) + '\n';
  const files = () => ['CHANGELOG.md', 'CHANGELOG.en.md', 'package.json', 'package-lock.json', 'deb/control'].map(read).join('\0');
  const run = (...a) => main(['node', 'release.js', ...a], dir);
  const quiet = (fn) => { const log = console.log; console.log = () => {}; try { return fn(); } finally { console.log = log; } };
  try {
    git('init', '-q', '-b', 'main');
    write('CHANGELOG.md', CL);
    write('CHANGELOG.en.md', CL);
    write('package.json', json('1.15.1'));
    write('package-lock.json', lock('1.15.1'));
    write('deb/control', 'Package: x\nVersion: 1.15.1-1\n');
    git('add', '-A');
    git('commit', '-q', '-m', 'init');

    assert.throws(() => run('1.15.1'), /не выше/, 'та же версия');
    assert.throws(() => run('1.16'), /X\.Y\.Z/, 'не X.Y.Z');
    assert.throws(() => run('1.16.0', '--date', '13.10.2026'), /ГГГГ-ММ-ДД/, 'дата не ISO');
    git('checkout', '-q', '-b', 'feature');
    assert.throws(() => run('1.16.0'), /из main, а сейчас ветка feature/, 'не из main');
    git('checkout', '-q', 'main');
    write('dirty.txt', 'x');
    assert.throws(() => run('1.16.0'), /незакоммиченные/, 'грязное дерево');
    fs.unlinkSync(path.join(dir, 'dirty.txt'));
    git('tag', 'v1.16.0');
    assert.throws(() => run('1.16.0'), /тег v1\.16\.0 уже есть/, 'тег уже есть');
    git('tag', '-d', 'v1.16.0');
    // ошибка в одном файле — не меняется ни один
    write('CHANGELOG.en.md', CL.replace('- Что-то.\n', ''));
    git('commit', '-q', '-am', 'en: пустой Unreleased');
    const snap = files();
    assert.throws(() => run('1.16.0'), /пуст/, 'пустой Unreleased в CHANGELOG.en.md');
    assert.strictEqual(files(), snap, 'при ошибке не изменён ни один файл (и CHANGELOG.md, проверенный первым)');
    write('CHANGELOG.en.md', CL);
    git('commit', '-q', '-am', 'en: снова с изменениями');
    assert.strictEqual(git('log', '--oneline').split('\n').length, 3, 'отказы не создают коммитов');

    // --no-commit: только файлы
    quiet(() => run('1.16.0', '--no-commit', '--date', '2026-10-13'));
    assert.ok(read('CHANGELOG.md').includes('## [1.16.0] — 2026-10-13'), '--no-commit: CHANGELOG');
    assert.ok(git('status', '--porcelain'), '--no-commit: изменения не закоммичены');
    assert.strictEqual(git('tag', '--list'), '', '--no-commit: тега нет');
    git('checkout', '-q', '--', '.');

    // выпуск: файлы и коммит (тега НЕТ — его накладывают на origin/main
    // после мержа PR, т.к. прямой пуш в main закрыт правилом про PR)
    quiet(() => run('--date', '2026-10-13', '1.16.0'));
    assert.strictEqual(git('log', '-1', '--format=%s'), 'Версия 1.16.0', 'коммит «Версия X.Y.Z»');
    assert.strictEqual(git('tag', '--list'), '', 'тега нет (тег — после мержа PR)');
    assert.strictEqual(git('status', '--porcelain'), '', 'всё закоммичено');
    for (const n of ['CHANGELOG.md', 'CHANGELOG.en.md']) assert.ok(read(n).includes('## [1.16.0] — 2026-10-13\n\n### Исправлено'), `${n}: раздел версии`);
    assert.strictEqual(JSON.parse(read('package.json')).version, '1.16.0', 'package.json');
    assert.strictEqual(JSON.parse(read('package-lock.json')).packages[''].version, '1.16.0', 'package-lock.json');
    assert.ok(read('deb/control').includes('Version: 1.16.0-1'), 'deb/control');
    assert.throws(() => run('1.16.1'), /пуст/, 'второй выпуск без изменений — отказ (авто-созданный Unreleased пуст)');
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

console.log('OK: release — все проверки прошли');
