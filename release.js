'use strict';
// Release preparation: npm run release 1.16.0
//   - the `## [Unreleased]` section in CHANGELOG.md and CHANGELOG.en.md
//     becomes `## [1.16.0] — <today>` (both sections must exist and be
//     non-empty), and a new empty `## [Unreleased]` is created in its place
//     for edits before the next release;
//   - the version in package.json, package-lock.json and deb/control (1.16.0-1);
//   - the «Версия 1.16.0» commit and the v1.16.0 tag (locally; push is manual —
//     the script prints the command: the tag triggers the build and release publication).
// File edits only, no commit and tag: npm run release -- 1.16.0 --no-commit
// A different date: --date 2026-10-13.
const fs = require('fs');
const path = require('path');
const { execFileSync } = require('child_process');
const { compareDebianVer } = require('./src/versions');

/* ===== release (pure functions, test test/release.js) ===== */
function checkVersion(v, current) {
  if (!/^\d+\.\d+\.\d+$/.test(String(v || ''))) throw new Error(`версия «${v}» не в формате X.Y.Z (напр. 1.16.0)`);
  if (current && compareDebianVer(`${v}-1`, `${current}-1`) <= 0) {
    throw new Error(`версия ${v} не выше текущей ${current}`);
  }
}

/* `## [Unreleased]` → `## [version] — date`; the section must be non-empty
   and no section for this version must exist yet. HTML comments (single-line)
   are service ones: they do not count as emptiness and are not carried over
   into the version section — otherwise they end up in the release description
   (1.17.1: the «write here» placeholder went into the GitHub Release). Above
   the released version — a new empty `## [Unreleased]` for edits before the next. */
function releaseChangelog(text, version, date) {
  const lines = text.split('\n');
  const at = lines.findIndex((l) => /^## \[Unreleased\]\s*$/i.test(l));
  if (at < 0) throw new Error('нет раздела «## [Unreleased]»');
  if (lines.some((l) => l.startsWith(`## [${version}]`))) throw new Error(`раздел [${version}] уже есть`);
  const next = lines.findIndex((l, i) => i > at && l.startsWith('## ['));
  const body = lines.slice(at + 1, next < 0 ? lines.length : next);
  const kept = [];
  for (const l of body) {
    if (l.trim().startsWith('<!--') && l.trim().endsWith('-->')) continue; // service comment — not carried into the version
    if (!l.trim() && kept.length && !kept[kept.length - 1].trim()) continue; // consecutive blank lines — one
    kept.push(l);
  }
  if (!kept.some((l) => l.trim() && !l.startsWith('### '))) throw new Error('раздел «## [Unreleased]» пуст');
  lines.splice(at + 1, body.length, ...kept);
  lines[at] = `## [${version}] — ${date}`;
  lines.splice(at, 0, '## [Unreleased]', '',
    '<!-- сюда дописать изменения для следующего релиза (### Добавлено / Изменено / Исправлено) -->', '');
  return lines.join('\n');
}

function setControlVersion(text, version) {
  if (!/^Version: .*$/m.test(text)) throw new Error('в deb/control нет строки «Version:»');
  return text.replace(/^Version: .*$/m, `Version: ${version}-1`);
}

/* package.json and package-lock.json (root and packages[""]) — as npm writes:
   indent 2 and a trailing newline. */
function setJsonVersion(text, version, lock) {
  const j = JSON.parse(text);
  j.version = version;
  if (lock && j.packages && j.packages['']) j.packages[''].version = version;
  return JSON.stringify(j, null, 2) + '\n';
}

function today(d = new Date()) {
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}
/* ===== end release ===== */

// root — the repository directory (test test/release.js — a temporary repository)
function main(argv, root = __dirname) {
  const args = argv.slice(2);
  const commit = !args.includes('--no-commit');
  const di = args.indexOf('--date');
  const date = di >= 0 ? args[di + 1] : today();
  const version = args.find((a, i) => !a.startsWith('--') && (di < 0 || i !== di + 1));
  if (!/^\d{4}-\d{2}-\d{2}$/.test(String(date || ''))) throw new Error(`дата «${date}» не в формате ГГГГ-ММ-ДД`);

  const file = (n) => path.join(root, n);
  const read = (n) => fs.readFileSync(file(n), 'utf8');
  const git = (...a) => execFileSync('git', a, { cwd: root, encoding: 'utf8' }).trim();
  const current = JSON.parse(read('package.json')).version;
  checkVersion(version, current);

  if (commit) {
    const branch = git('rev-parse', '--abbrev-ref', 'HEAD');
    if (branch !== 'main') throw new Error(`релиз выпускается из main, а сейчас ветка ${branch} (только правка файлов — --no-commit)`);
    if (git('status', '--porcelain')) throw new Error('есть незакоммиченные изменения — сначала закоммитьте их (или --no-commit)');
    if (git('tag', '--list', `v${version}`)) throw new Error(`тег v${version} уже есть`);
  }

  // compute everything up front: an error in any file — we change none of them
  const out = {
    'CHANGELOG.md': releaseChangelog(read('CHANGELOG.md'), version, date),
    'CHANGELOG.en.md': releaseChangelog(read('CHANGELOG.en.md'), version, date),
    'package.json': setJsonVersion(read('package.json'), version, false),
    'package-lock.json': setJsonVersion(read('package-lock.json'), version, true),
    'deb/control': setControlVersion(read('deb/control'), version),
  };
  for (const [n, t] of Object.entries(out)) fs.writeFileSync(file(n), t);
  console.log(`${current} → ${version} (${date}): ${Object.keys(out).join(', ')}`);

  if (!commit) {
    console.log('Файлы изменены, коммита и тега нет (--no-commit).');
    return;
  }
  git('add', ...Object.keys(out));
  git('commit', '-q', '-m', `Version ${version}`);
  // The tag is NOT created: direct push to main is closed (the «Changes must
  // be made through a pull request» rule); the tag is applied after the merge,
  // to the commit in main (steps — in the header of .github/workflows/build-deb.yml).
  console.log(`Коммит «Версия ${version}» — локально, тега нет (тег — после мержа). Выпустить:\n` +
    `  git push origin main:release/${version}\n` +
    `  gh pr create --base main --head release/${version}\n` +
    `  # по зелёному build: мерж, затем:\n` +
    `  git fetch origin && git tag v${version} origin/main && git push origin v${version}`);
}

if (require.main === module) {
  try {
    main(process.argv);
  } catch (e) {
    console.error(`release: ${e.message}`);
    process.exit(1);
  }
}

module.exports = { main, checkVersion, releaseChangelog, setControlVersion, setJsonVersion, today };
