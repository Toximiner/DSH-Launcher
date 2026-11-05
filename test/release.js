'use strict';

/*
 * Unit tests for release preparation (release.js, npm run release X.Y.Z): the
 * CHANGELOG section, versions in package.json / package-lock.json / deb/control.
 *
 * Run: node test/release.js   (npm test)
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
assert.throws(() => checkVersion('1.16', '1.15.1'), /X\.Y\.Z/, 'not X.Y.Z');
assert.throws(() => checkVersion('v1.16.0', '1.15.1'), /X\.Y\.Z/, 'with a "v" prefix — no');
assert.throws(() => checkVersion(undefined, '1.15.1'), /X\.Y\.Z/, 'the version is not given');
assert.throws(() => checkVersion('1.15.1', '1.15.1'), /не выше/, 'the same version');
assert.throws(() => checkVersion('1.9.0', '1.15.1'), /не выше/, 'lower (numeric, not string)');

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
  assert.ok(out.includes('\n## [1.16.0] — 2026-10-13\n\n### Исправлено\n\n- Что-то.\n\n## [1.15.1]'), 'Unreleased → the version and the date');
  assert.ok(out.includes('\n## [Unreleased]\n\n<!-- сюда дописать'), 'a new empty [Unreleased] is created');
  assert.ok(out.indexOf('## [Unreleased]') < out.indexOf('## [1.16.0] — 2026-10-13'), 'the new [Unreleased] is above the version');
  assert.strictEqual(out.split('\n').length, CL.split('\n').length + 4, 'the rest is untouched, a new [Unreleased] is added');
}
assert.throws(() => releaseChangelog(CL.replace('## [Unreleased]\n', ''), '1.16.0', '2026-10-13'), /нет раздела/, 'no Unreleased');
assert.throws(() => releaseChangelog(CL.replace('### Исправлено\n\n- Что-то.\n', ''), '1.16.0', '2026-10-13'), /пуст/, 'an empty Unreleased');
assert.throws(() => releaseChangelog(CL.replace('- Что-то.\n', ''), '1.16.0', '2026-10-13'), /пуст/, 'only a subheading — empty');
assert.throws(() => releaseChangelog(CL, '1.15.1', '2026-10-13'), /уже есть/, 'the version section already exists');
assert.ok(releaseChangelog('## [Unreleased]\n- Последний раздел.\n', '1.0.0', '2026-01-01').includes('## [1.0.0] — 2026-01-01\n- Последний раздел.\n'), 'Unreleased — the last section');
assert.throws(() => releaseChangelog(releaseChangelog(CL, '1.16.0', '2026-10-13'), '1.16.1', '2026-10-14'), /пуст/, 'the auto-created [Unreleased] is empty (a comment does not count)');
{
  // The service comment of [Unreleased] is not moved into the version section:
  // 1.17.1 — the "write here" placeholder went into the GitHub Release notes.
  const CL2 = CL.replace('## [Unreleased]\n\n### Исправлено\n',
    '## [Unreleased]\n\n<!-- сюда дописать изменения для следующего релиза (### Добавлено / Изменено / Исправлено) -->\n\n### Исправлено\n');
  const out = releaseChangelog(CL2, '1.16.0', '2026-10-13');
  const verSection = out.split('\n## [1.16.0]')[1].split('\n## [1.15.1]')[0]; // the version section
  assert.ok(!verSection.includes('дописать'), 'the placeholder is not moved into the version');
  assert.ok(out.includes('\n## [1.16.0] — 2026-10-13\n\n### Исправлено\n\n- Что-то.\n'), 'the section content is in place');
  assert.ok(!/^<!--/m.test(verSection), 'no comments in the version section');
  assert.strictEqual(out.split('\n').length, CL2.split('\n').length + 2, 'only the comment and the extra blank line are removed');
}

// ---- setControlVersion ----
assert.strictEqual(setControlVersion('Package: dsh-launcher\nVersion: 1.15.1-1\nArchitecture: amd64\n', '1.16.0'),
  'Package: dsh-launcher\nVersion: 1.16.0-1\nArchitecture: amd64\n', 'deb/control: X.Y.Z-1');
assert.throws(() => setControlVersion('Package: x\n', '1.16.0'), /Version/, 'no Version line');

// ---- setJsonVersion ----
{
  const lock = { name: 'dsh-launcher', version: '1.15.1', lockfileVersion: 3, packages: { '': { name: 'dsh-launcher', version: '1.15.1' }, 'node_modules/x': { version: '1.15.1' } } };
  const out = JSON.parse(setJsonVersion(JSON.stringify(lock, null, 2) + '\n', '1.16.0', true));
  assert.strictEqual(out.version, '1.16.0', 'lock: the root');
  assert.strictEqual(out.packages[''].version, '1.16.0', 'lock: packages[""]');
  assert.strictEqual(out.packages['node_modules/x'].version, '1.15.1', 'lock: dependencies with the same version are untouched');
  assert.ok(setJsonVersion('{"version":"1.0.0"}', '1.1.0', false).endsWith('}\n'), 'a trailing newline');
}
// npm format: the real project files are rewritten byte for byte
for (const [n, lock] of [['package.json', false], ['package-lock.json', true]]) {
  const t = fs.readFileSync(path.join(__dirname, '..', n), 'utf8');
  assert.strictEqual(setJsonVersion(t, JSON.parse(t).version, lock), t, `${n}: the same version — the file is unchanged`);
}
// the real CHANGELOGs — both in the format release.js understands
for (const n of ['CHANGELOG.md', 'CHANGELOG.en.md']) {
  const t = fs.readFileSync(path.join(__dirname, '..', n), 'utf8');
  assert.ok(/^## \[\d+\.\d+\.\d+\] — \d{4}-\d{2}-\d{2}$/m.test(t), `${n}: sections in the «## [X.Y.Z] — YYYY-MM-DD» form`);
}

// ---- today ----
assert.strictEqual(today(new Date(2026, 0, 5)), '2026-01-05', 'the date with zero padding');

// ---- main: the whole release on a temporary git repository ----
{
  // its own git without the user's global settings (commit signing, hooks …)
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

    assert.throws(() => run('1.15.1'), /не выше/, 'the same version');
    assert.throws(() => run('1.16'), /X\.Y\.Z/, 'not X.Y.Z');
    assert.throws(() => run('1.16.0', '--date', '13.10.2026'), /ГГГГ-ММ-ДД/, 'the date is not ISO');
    git('checkout', '-q', '-b', 'feature');
    assert.throws(() => run('1.16.0'), /из main, а сейчас ветка feature/, 'not from main');
    git('checkout', '-q', 'main');
    write('dirty.txt', 'x');
    assert.throws(() => run('1.16.0'), /незакоммиченные/, 'a dirty tree');
    fs.unlinkSync(path.join(dir, 'dirty.txt'));
    git('tag', 'v1.16.0');
    assert.throws(() => run('1.16.0'), /тег v1\.16\.0 уже есть/, 'the tag already exists');
    git('tag', '-d', 'v1.16.0');
    // an error in one file — not a single one is changed
    write('CHANGELOG.en.md', CL.replace('- Что-то.\n', ''));
    git('commit', '-q', '-am', 'en: пустой Unreleased');
    const snap = files();
    assert.throws(() => run('1.16.0'), /пуст/, 'an empty Unreleased in CHANGELOG.en.md');
    assert.strictEqual(files(), snap, 'on an error no file is changed (including CHANGELOG.md, checked first)');
    write('CHANGELOG.en.md', CL);
    git('commit', '-q', '-am', 'en: снова с изменениями');
    assert.strictEqual(git('log', '--oneline').split('\n').length, 3, 'rejections create no commits');

    // --no-commit: files only
    quiet(() => run('1.16.0', '--no-commit', '--date', '2026-10-13'));
    assert.ok(read('CHANGELOG.md').includes('## [1.16.0] — 2026-10-13'), '--no-commit: CHANGELOG');
    assert.ok(git('status', '--porcelain'), '--no-commit: the changes are not committed');
    assert.strictEqual(git('tag', '--list'), '', '--no-commit: no tag');
    git('checkout', '-q', '--', '.');

    // the release: files and a commit (NO tag — it is applied to origin/main
    // after the PR merge, since direct push to main is closed by the PR rule)
    quiet(() => run('--date', '2026-10-13', '1.16.0'));
    assert.strictEqual(git('log', '-1', '--format=%s'), 'Version 1.16.0', 'commit "Version X.Y.Z"');
    assert.strictEqual(git('tag', '--list'), '', 'no tag (the tag comes after the PR merge)');
    assert.strictEqual(git('status', '--porcelain'), '', 'everything is committed');
    for (const n of ['CHANGELOG.md', 'CHANGELOG.en.md']) assert.ok(read(n).includes('## [1.16.0] — 2026-10-13\n\n### Исправлено'), `${n}: the version section`);
    assert.strictEqual(JSON.parse(read('package.json')).version, '1.16.0', 'package.json');
    assert.strictEqual(JSON.parse(read('package-lock.json')).packages[''].version, '1.16.0', 'package-lock.json');
    assert.ok(read('deb/control').includes('Version: 1.16.0-1'), 'deb/control');
    assert.throws(() => run('1.16.1'), /пуст/, 'a second release without changes — rejected (the auto-created Unreleased is empty)');
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

console.log('OK: release — all checks passed');
