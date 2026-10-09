#!/usr/bin/env node
// Юнит-тесты чистых помощников main.js: splitArgs, launcherRoute,
// profileFromArgs, firstErrLine, esc, envWithBinDir, marketInPkg.
'use strict';
const path = require('path');
const assert = require('assert');
const { extract } = require('./_extract');

// ---- splitArgs: разбор DSH_ARGS как в shell ----
const splitArgs = extract('splitArgs');
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
const launcherRoute = extract('launcherRoute');
assert.strictEqual(launcherRoute('dshlauncher://install/run/'), '/install/run/');
assert.strictEqual(launcherRoute('dshlauncher://menu/about/'), '/menu/about/');
assert.strictEqual(launcherRoute('dshlauncher://market/skip/?x=1'), '/market/skip/', 'query-строка отбрасывается');
assert.strictEqual(launcherRoute('dshlauncher://paste/http%3A%2F%2Fx%2F'), '/paste/http%3A%2F%2Fx%2F');

// ---- profileFromArgs: --profile X / --profile=X ----
const profileFromArgs = extract('profileFromArgs');
assert.strictEqual(profileFromArgs(['--profile', 'web']), 'web');
assert.strictEqual(profileFromArgs(['--profile=web']), 'web');
assert.strictEqual(profileFromArgs(['--no-open', '--profile', 'web']), 'web');
assert.strictEqual(profileFromArgs([]), null);
assert.strictEqual(profileFromArgs(['--profile']), null, 'флаг без значения → null');
assert.strictEqual(profileFromArgs(['--profile=a', '--profile=b']), 'a', 'первый флаг побеждает');

// ---- firstErrLine: строка ошибки, а не хвост стектрейса ----
const firstErrLine = extract('firstErrLine');
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
const esc = extract('esc');
assert.strictEqual(esc('<b>"q" & \'a\'</b>'), '&lt;b&gt;&quot;q&quot; &amp; &#39;a&#39;&lt;/b&gt;');
assert.strictEqual(esc(42), '42', 'число приводится к строке');
assert.strictEqual(esc(null), 'null');
assert.strictEqual(esc(''), '');

// ---- envWithBinDir: каталог бинарника первым в PATH ----
const envWithBinDir = extract('envWithBinDir', { path, process });
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

// ---- marketInPkg: плагин маркета в package.json профиля ----
const marketInPkg = extract('marketInPkg', { MARKET_PKG: 'dshmarket' });
assert.strictEqual(marketInPkg({ dependencies: { dshmarket: '1.66.8' } }), true, 'зависимость dshmarket');
assert.strictEqual(marketInPkg({ dsh: { profile: { bundles: ['other', 'dshmarket'] } } }), true, 'bundles dsh.profile');
assert.strictEqual(marketInPkg({ dependencies: { dshmarket: '1' }, dsh: { profile: { bundles: [] } } }), true, 'любой признак');
assert.strictEqual(marketInPkg({ dependencies: { other: '1' } }), false);
assert.strictEqual(marketInPkg({ dsh: { profile: {} } }), false, 'нет bundles → false');
assert.strictEqual(marketInPkg({}), false);

// ---- proxyUrlFromRule: правило прокси Chromium → URL для агента Node ----
const proxyUrlFromRule = extract('proxyUrlFromRule');
assert.strictEqual(proxyUrlFromRule('DIRECT'), null, 'DIRECT → напрямую');
assert.strictEqual(proxyUrlFromRule(''), null, 'пусто → напрямую');
assert.strictEqual(proxyUrlFromRule(undefined), null, 'нет правила → напрямую');
assert.strictEqual(proxyUrlFromRule('PROXY 10.0.0.1:3128'), 'http://10.0.0.1:3128', 'PROXY');
assert.strictEqual(proxyUrlFromRule('PROXY proxy.corp:8080; DIRECT'), 'http://proxy.corp:8080', 'PROXY; DIRECT — первый');
assert.strictEqual(proxyUrlFromRule('HTTPS secure.corp:443'), 'https://secure.corp:443', 'HTTPS-прокси');
assert.strictEqual(proxyUrlFromRule('SOCKS5 127.0.0.1:1080'), null, 'SOCKS не поддержан → напрямую');
assert.strictEqual(proxyUrlFromRule('SOCKS5 s:1; PROXY p:3128'), 'http://p:3128', 'SOCKS пропускаем, берём HTTP');
assert.strictEqual(proxyUrlFromRule('proxy p:1'), 'http://p:1', 'регистр не важен');

console.log('OK: parsing — все проверки прошли');
