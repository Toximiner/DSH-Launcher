#!/usr/bin/env node
// Юнит-тесты чистых помощников main.js: splitArgs, launcherRoute,
// profileFromArgs, firstErrLine, esc, envWithBinDir, withoutUnitEnv, marketInPkg.
'use strict';
const path = require('path');
const assert = require('assert');
const { extract } = require('./_extract');

// ---- splitArgs: разбор DSH_ARGS как в shell ----
const { splitArgs, launcherRoute, profileFromArgs, firstErrLine, esc } = require('../src/util');
const { envWithBinDir, withoutUnitEnv } = require('../src/system');
const { proxyUrlFromRule } = require('../src/updates-logic');
const { dpkgStatusVersion } = require('../src/versions');
assert.deepStrictEqual(splitArgs(''), [], 'пустая строка → []');
assert.deepStrictEqual(splitArgs('   '), [], 'только пробелы → []');
assert.deepStrictEqual(splitArgs('a b'), ['a', 'b']);
assert.deepStrictEqual(splitArgs('  a   b  '), ['a', 'b'], 'много пробелов');
assert.deepStrictEqual(splitArgs("a 'b c'"), ['a', 'b c'], 'одинарные кавычки сохраняют пробел');
assert.deepStrictEqual(splitArgs('a "b  c"'), ['a', 'b  c'], 'двойные кавычки сохраняют пробелы');
assert.deepStrictEqual(splitArgs('a "b\\"c"'), ['a', 'b"c'], 'экранирование внутри "…"');
assert.deepStrictEqual(splitArgs("a 'b\\'c'"), ['a', 'b\\c'], "внутри '…' экранирования нет — кавычка закрывает");
assert.deepStrictEqual(splitArgs('a b\\ c'), ['a', 'b c'], '\\ экранирует пробел');
assert.deepStrictEqual(splitArgs('a\\\nb'), ['a\nb'], '\\ + newline → перенос строки');
assert.deepStrictEqual(splitArgs('a\\'), ['a\\'], 'хвостовой \\ без пары сохраняется');
assert.deepStrictEqual(splitArgs("''"), [''], 'пустой кавычный токен есть');
assert.deepStrictEqual(splitArgs('""'), [''], 'пустой кавычный токен (двойные)');
assert.deepStrictEqual(splitArgs('--profile web --no-open'), ['--profile', 'web', '--no-open']);
assert.deepStrictEqual(splitArgs("a'b'"), ['ab'], 'кавычки без пробелов склеиваются с текстом');

// ---- launcherRoute: dshlauncher://host/path → /host/path ----
assert.strictEqual(launcherRoute('dshlauncher://install/run/'), '/install/run/');
assert.strictEqual(launcherRoute('dshlauncher://menu/about/'), '/menu/about/');
assert.strictEqual(launcherRoute('dshlauncher://market/skip/?x=1'), '/market/skip/', 'query-строка отбрасывается');
assert.strictEqual(launcherRoute('dshlauncher://paste/http%3A%2F%2Fx%2F'), '/paste/http%3A%2F%2Fx%2F');

// ---- profileFromArgs: --profile X / --profile=X ----
assert.strictEqual(profileFromArgs(['--profile', 'web']), 'web');
assert.strictEqual(profileFromArgs(['--profile=web']), 'web');
assert.strictEqual(profileFromArgs(['--no-open', '--profile', 'web']), 'web');
assert.strictEqual(profileFromArgs([]), null);
assert.strictEqual(profileFromArgs(['--profile']), null, 'флаг без значения → null');
assert.strictEqual(profileFromArgs(['--profile=a', '--profile=b']), 'a', 'первый флаг побеждает');

// ---- firstErrLine: строка ошибки, а не хвост стектрейса ----
assert.strictEqual(firstErrLine('at foo (x)\nError: boom\nat bar'), 'Error: boom', 'строка с ошибкой, а не at-строки');
assert.strictEqual(firstErrLine('npm ERR! code E404\nnpm ERR! notarget\nat x'), 'npm ERR! code E404 npm ERR! notarget at x', 'npm ERR! (без ERR_) → фолбэк: последние 3 строки');
assert.strictEqual(firstErrLine('noise\nSomething ERR_ happened\nat x'), 'Something ERR_ happened', 'строка с ERR_');
assert.strictEqual(firstErrLine('at a\nat b\nError: x\nError: y'), 'Error: x', 'первая подходящая, не последняя');
assert.strictEqual(firstErrLine('plain one\nplain two\nplain three'), 'plain one plain two plain three', 'без error — последние 3 строки');
assert.strictEqual(firstErrLine('only'), 'only');
assert.strictEqual(firstErrLine(''), '', 'пусто → пусто');
assert.strictEqual(firstErrLine(undefined), '', 'undefined → пусто');
assert.strictEqual(firstErrLine('x'.repeat(500)), 'x'.repeat(300), 'обрезка до 300 символов');
assert.strictEqual(firstErrLine('  indented error line  \n'), 'indented error line', 'обрезка по краям строк');

// ---- esc: HTML-экранирование ----
assert.strictEqual(esc('<b>"q" & \'a\'</b>'), '&lt;b&gt;&quot;q&quot; &amp; &#39;a&#39;&lt;/b&gt;');
assert.strictEqual(esc(42), '42', 'число приводится к строке');
assert.strictEqual(esc(null), 'null');
assert.strictEqual(esc(''), '');

// ---- envWithBinDir: каталог бинарника первым в PATH ----
{
  const oldPath = process.env.PATH;
  process.env.PATH = '/usr/bin:/bin';
  try {
    const env = envWithBinDir('/home/u/.npm-global/bin/npm', { DSH: '1' });
    assert.strictEqual(env.PATH, '/home/u/.npm-global/bin' + path.delimiter + '/usr/bin:/bin', 'bin-каталог первым');
    assert.strictEqual(env.DSH, '1', 'extra-переменные накладываются');
    assert.strictEqual(env.HOME, process.env.HOME, 'остальное окружение сохраняется');
  } finally {
    process.env.PATH = oldPath;
  }
}

// ---- withoutUnitEnv: dsh — не служба systemd (dshmarket: «self-restart is disabled») ----
{
  const src = { HOME: '/home/u', PATH: '/usr/bin', INVOCATION_ID: 'abc', JOURNAL_STREAM: '8:123', NOTIFY_SOCKET: '/run/n', SYSTEMD_EXEC_PID: '42', DSH_STARTED_BY: 'dsh-launcher' };
  const env = withoutUnitEnv(src);
  for (const k of ['INVOCATION_ID', 'JOURNAL_STREAM', 'NOTIFY_SOCKET', 'SYSTEMD_EXEC_PID']) assert.ok(!(k in env), `без ${k}`);
  assert.deepStrictEqual(env, { HOME: '/home/u', PATH: '/usr/bin', DSH_STARTED_BY: 'dsh-launcher' }, 'остальное — как было');
  assert.strictEqual(src.INVOCATION_ID, 'abc', 'исходный объект не меняется');
}

// ---- marketInPkg: плагин маркета в package.json профиля ----
const marketInPkg = extract('marketInPkg', { MARKET_PKG: 'dshmarket' });
assert.strictEqual(marketInPkg({ dependencies: { dshmarket: '1.66.8' } }), true, 'зависимость dshmarket');
assert.strictEqual(marketInPkg({ dsh: { profile: { bundles: ['other', 'dshmarket'] } } }), true, 'bundles dsh.profile');
assert.strictEqual(marketInPkg({ dependencies: { dshmarket: '1' }, dsh: { profile: { bundles: [] } } }), true, 'любой признак');
assert.strictEqual(marketInPkg({ dependencies: { other: '1' } }), false);
assert.strictEqual(marketInPkg({ dsh: { profile: {} } }), false, 'нет bundles → false');
assert.strictEqual(marketInPkg({}), false);

// ---- proxyUrlFromRule: правило прокси Chromium → URL для агента Node ----
assert.strictEqual(proxyUrlFromRule('DIRECT'), null, 'DIRECT → напрямую');
assert.strictEqual(proxyUrlFromRule(''), null, 'пусто → напрямую');
assert.strictEqual(proxyUrlFromRule(undefined), null, 'нет правила → напрямую');
assert.strictEqual(proxyUrlFromRule('PROXY 10.0.0.1:3128'), 'http://10.0.0.1:3128', 'PROXY');
assert.strictEqual(proxyUrlFromRule('PROXY proxy.corp:8080; DIRECT'), 'http://proxy.corp:8080', 'PROXY; DIRECT — первый');
assert.strictEqual(proxyUrlFromRule('HTTPS secure.corp:443'), 'https://secure.corp:443', 'HTTPS-прокси');
assert.strictEqual(proxyUrlFromRule('SOCKS5 127.0.0.1:1080'), null, 'SOCKS не поддержан → напрямую');
assert.strictEqual(proxyUrlFromRule('SOCKS5 s:1; PROXY p:3128'), 'http://p:3128', 'SOCKS пропускаем, берём HTTP');
assert.strictEqual(proxyUrlFromRule('proxy p:1'), 'http://p:1', 'регистр не важен');

// ---- DSH_LAUNCHER_GITHUB_TOKEN: только для CI, наружу не уходит ----
{
  const { mainSrc } = require('./_extract');
  const envSrc = require('fs').readFileSync(require('path').join(__dirname, '..', 'src', 'env.js'), 'utf8');
  assert.ok(envSrc.includes('delete process.env.DSH_LAUNCHER_GITHUB_TOKEN'), 'переменная токена удаляется из окружения (src/env.js)');
  const del = mainSrc.indexOf("require('./src/env')");
  assert.ok(del > 0, 'main.js подключает src/env.js');
  const firstChild = Math.min(...['execFileSync(', 'spawn(', 'execFileAsync(']
    .map((k) => mainSrc.indexOf(k, mainSrc.indexOf("require('child_process')") + 30)).filter((x) => x > 0));
  assert.ok(del < firstChild, 'токен убран из окружения до первого запуска дочернего процесса');
  // весь код лаунчера: main.js и модули src/
  const srcDir = require('path').join(__dirname, '..', 'src');
  const allSrc = [mainSrc, ...require('fs').readdirSync(srcDir).filter((f) => f.endsWith('.js'))
    .map((f) => require('fs').readFileSync(require('path').join(srcDir, f), 'utf8'))].join('\n');
  const uses = allSrc.match(/Bearer \$\{GITHUB_TOKEN\}/g) || [];
  assert.strictEqual(uses.length, 1, 'токен подставляется в одном месте');
  const fn = allSrc.slice(allSrc.indexOf('async function githubReleases('), allSrc.indexOf('async function fetchLauncherLatest('));
  assert.ok(fn.includes('Bearer ${GITHUB_TOKEN}'), '— и это запрос списка релизов (api.github.com)');
  assert.ok(!/console\.(log|error)\([^)]*\$\{GITHUB_TOKEN\}/.test(allSrc), 'значение токена не печатается');
}

// ---- dpkgStatusVersion: версия пакета из /var/lib/dpkg/status ----
{
  const v = dpkgStatusVersion;
  const db = [
    'Package: dsh-launcher-extra', 'Status: install ok installed', 'Version: 9.9.9-1', '',
    'Package: dsh-launcher', 'Status: install ok installed', 'Priority: optional', 'Version: 1.11.0-1', 'Description: x', ' multi-line', '',
    'Package: other', 'Status: deinstall ok config-files', 'Version: 2.0', '',
  ].join('\n');
  assert.strictEqual(v(db, 'dsh-launcher'), '1.11.0-1', 'установленный пакет');
  assert.strictEqual(v(db, 'other'), null, 'удалён (остались настройки) — не установлен');
  assert.strictEqual(v(db, 'absent'), null, 'нет такого пакета');
  assert.strictEqual(v('', 'dsh-launcher'), null, 'пустая база');
  assert.strictEqual(v(null, 'dsh-launcher'), null, 'нет файла');
}

console.log('OK: parsing — все проверки прошли');
