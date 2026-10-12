'use strict';
// Подготовка релиза: npm run release 1.16.0
//   - раздел `## [Unreleased]` в CHANGELOG.md и CHANGELOG.en.md становится
//     `## [1.16.0] — <сегодня>` (оба раздела должны быть и быть непустыми);
//   - версия в package.json, package-lock.json и deb/control (1.16.0-1);
//   - коммит «Версия 1.16.0» и тег v1.16.0 (локально; отправка — вручную,
//     команду скрипт печатает: тег запускает сборку и публикацию релиза).
// Только правка файлов, без коммита и тега: npm run release -- 1.16.0 --no-commit
// Другая дата: --date 2026-10-13.
const fs = require('fs');
const path = require('path');
const { execFileSync } = require('child_process');
const { compareDebianVer } = require('./src/versions');

/* ===== release (чистые функции, тест test/release.js) ===== */
function checkVersion(v, current) {
  if (!/^\d+\.\d+\.\d+$/.test(String(v || ''))) throw new Error(`версия «${v}» не в формате X.Y.Z (напр. 1.16.0)`);
  if (current && compareDebianVer(`${v}-1`, `${current}-1`) <= 0) {
    throw new Error(`версия ${v} не выше текущей ${current}`);
  }
}

/* `## [Unreleased]` → `## [version] — date`; раздел должен быть непустым, а
   раздела этой версии ещё не должно быть. */
function releaseChangelog(text, version, date) {
  const lines = text.split('\n');
  const at = lines.findIndex((l) => /^## \[Unreleased\]\s*$/i.test(l));
  if (at < 0) throw new Error('нет раздела «## [Unreleased]»');
  if (lines.some((l) => l.startsWith(`## [${version}]`))) throw new Error(`раздел [${version}] уже есть`);
  const next = lines.findIndex((l, i) => i > at && l.startsWith('## ['));
  const body = lines.slice(at + 1, next < 0 ? lines.length : next);
  if (!body.some((l) => l.trim() && !l.startsWith('### '))) throw new Error('раздел «## [Unreleased]» пуст');
  lines[at] = `## [${version}] — ${date}`;
  return lines.join('\n');
}

function setControlVersion(text, version) {
  if (!/^Version: .*$/m.test(text)) throw new Error('в deb/control нет строки «Version:»');
  return text.replace(/^Version: .*$/m, `Version: ${version}-1`);
}

/* package.json и package-lock.json (корень и packages[""]) — как пишет npm:
   отступ 2 и перевод строки в конце. */
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

function main(argv) {
  const args = argv.slice(2);
  const commit = !args.includes('--no-commit');
  const di = args.indexOf('--date');
  const date = di >= 0 ? args[di + 1] : today();
  const version = args.find((a, i) => !a.startsWith('--') && (di < 0 || i !== di + 1));
  if (!/^\d{4}-\d{2}-\d{2}$/.test(String(date || ''))) throw new Error(`дата «${date}» не в формате ГГГГ-ММ-ДД`);

  const root = __dirname;
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

  // всё считаем заранее: ошибка в любом файле — не меняем ни одного
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
  git('commit', '-q', '-m', `Версия ${version}`);
  git('tag', `v${version}`);
  console.log(`Коммит «Версия ${version}» и тег v${version} — локально. Выпустить:\n  git push origin main v${version}`);
}

if (require.main === module) {
  try {
    main(process.argv);
  } catch (e) {
    console.error(`release: ${e.message}`);
    process.exit(1);
  }
}

module.exports = { checkVersion, releaseChangelog, setControlVersion, setJsonVersion, today };
