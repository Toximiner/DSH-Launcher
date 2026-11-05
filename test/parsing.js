#!/usr/bin/env node
// Unit tests for the pure helpers of main.js: splitArgs, launcherRoute,
// profileFromArgs, firstErrLine, esc, envWithBinDir, withoutUnitEnv, marketInPkg,
// authUrlFromOutput.
'use strict';
const path = require('path');
const assert = require('assert');
const { extract } = require('./_extract');

// ---- splitArgs: parsing DSH_ARGS like a shell ----
const { splitArgs, launcherRoute, profileFromArgs, firstErrLine, esc } = require('../src/util');
const { envWithBinDir, withoutUnitEnv } = require('../src/system');
const { proxyUrlFromRule } = require('../src/updates-logic');
const { dpkgStatusVersion } = require('../src/versions');
assert.deepStrictEqual(splitArgs(''), [], 'empty string → []');
assert.deepStrictEqual(splitArgs('   '), [], 'only spaces → []');
assert.deepStrictEqual(splitArgs('a b'), ['a', 'b']);
assert.deepStrictEqual(splitArgs('  a   b  '), ['a', 'b'], 'many spaces');
assert.deepStrictEqual(splitArgs("a 'b c'"), ['a', 'b c'], 'single quotes preserve the space');
assert.deepStrictEqual(splitArgs('a "b  c"'), ['a', 'b  c'], 'double quotes preserve spaces');
assert.deepStrictEqual(splitArgs('a "b\\"c"'), ['a', 'b"c'], 'escaping inside "…"');
assert.deepStrictEqual(splitArgs("a 'b\\'c'"), ['a', 'b\\c'], "no escaping inside '…' — the quote closes");
assert.deepStrictEqual(splitArgs('a b\\ c'), ['a', 'b c'], '\\ escapes the space');
assert.deepStrictEqual(splitArgs('a\\\nb'), ['a\nb'], '\\ + newline → a line break');
assert.deepStrictEqual(splitArgs('a\\'), ['a\\'], 'a trailing \\ without a pair is kept');
assert.deepStrictEqual(splitArgs("''"), [''], 'an empty quoted token is present');
assert.deepStrictEqual(splitArgs('""'), [''], 'an empty quoted token (double)');
assert.deepStrictEqual(splitArgs('--profile web --no-open'), ['--profile', 'web', '--no-open']);
assert.deepStrictEqual(splitArgs("a'b'"), ['ab'], 'quotes without spaces join with the text');

// ---- route key: withRouteKey / stripRouteKey / validRouteKey ----
{
  const { withRouteKey, stripRouteKey, validRouteKey } = require('../src/util');
  const K = '0123456789abcdef'.repeat(2);
  assert.strictEqual(withRouteKey(`<a href="dshlauncher://lang/ru/">x</a><b onclick="location.href='dshlauncher://paste/' + v">`, K),
    `<a href="dshlauncher://${K}/lang/ru/">x</a><b onclick="location.href='dshlauncher://${K}/paste/' + v">`, 'the key goes into every link');
  assert.strictEqual(stripRouteKey(launcherRoute(`dshlauncher://${K}/settings/set/?k=port&v=1`), K), '/settings/set/', 'with the key — the route without the key');
  assert.strictEqual(stripRouteKey(launcherRoute('dshlauncher://settings/set/?k=dshPath&v=/tmp/x'), K), null, 'no key — rejected');
  assert.strictEqual(stripRouteKey(launcherRoute(`dshlauncher://${K.slice(1)}0/menu/about/`), K), null, 'a foreign key — rejected');
  assert.strictEqual(stripRouteKey(`/${K}`, K), null, 'key only, no route — rejected');
  assert.strictEqual(stripRouteKey(launcherRoute(`dshlauncher://${K}/paste/http%3A%2F%2Fx%2F`), K), '/paste/http%3A%2F%2Fx%2F', 'paste');
  for (const bad of ['', undefined, 'abc', 'g'.repeat(32), K.toUpperCase()]) {
    assert.strictEqual(validRouteKey(bad), false, `an invalid key: ${bad}`);
    assert.strictEqual(stripRouteKey(`/${bad}/menu/about/`, bad), null, `nothing is accepted with an invalid key: ${bad}`);
  }
  assert.strictEqual(validRouteKey(K), true, '32 hex');
}

// ---- launcherRoute: dshlauncher://host/path → /host/path ----
assert.strictEqual(launcherRoute('dshlauncher://install/run/'), '/install/run/');
assert.strictEqual(launcherRoute('dshlauncher://menu/about/'), '/menu/about/');
assert.strictEqual(launcherRoute('dshlauncher://market/skip/?x=1'), '/market/skip/', 'the query string is dropped');
assert.strictEqual(launcherRoute('dshlauncher://paste/http%3A%2F%2Fx%2F'), '/paste/http%3A%2F%2Fx%2F');

// ---- profileFromArgs: --profile X / --profile=X ----
assert.strictEqual(profileFromArgs(['--profile', 'web']), 'web');
assert.strictEqual(profileFromArgs(['--profile=web']), 'web');
assert.strictEqual(profileFromArgs(['--no-open', '--profile', 'web']), 'web');
assert.strictEqual(profileFromArgs([]), null);
assert.strictEqual(profileFromArgs(['--profile']), null, 'a flag without a value → null');
assert.strictEqual(profileFromArgs(['--profile=a', '--profile=b']), 'a', 'the first flag wins');

// ---- firstErrLine: the error line, not the tail of a stack trace ----
assert.strictEqual(firstErrLine('at foo (x)\nError: boom\nat bar'), 'Error: boom', 'the error line, not the at-lines');
assert.strictEqual(firstErrLine('npm ERR! code E404\nnpm ERR! notarget\nat x'), 'npm ERR! code E404 npm ERR! notarget at x', 'npm ERR! (no ERR_) → fallback: the last 3 lines');
assert.strictEqual(firstErrLine('noise\nSomething ERR_ happened\nat x'), 'Something ERR_ happened', 'a line with ERR_');
assert.strictEqual(firstErrLine('at a\nat b\nError: x\nError: y'), 'Error: x', 'the first match, not the last');
assert.strictEqual(firstErrLine('plain one\nplain two\nplain three'), 'plain one plain two plain three', 'no error — the last 3 lines');
assert.strictEqual(firstErrLine('only'), 'only');
assert.strictEqual(firstErrLine(''), '', 'empty → empty');
assert.strictEqual(firstErrLine(undefined), '', 'undefined → empty');
assert.strictEqual(firstErrLine('x'.repeat(500)), 'x'.repeat(300), 'truncated to 300 characters');
assert.strictEqual(firstErrLine('  indented error line  \n'), 'indented error line', 'trimmed at the line edges');

// ---- esc: HTML escaping ----
assert.strictEqual(esc('<b>"q" & \'a\'</b>'), '&lt;b&gt;&quot;q&quot; &amp; &#39;a&#39;&lt;/b&gt;');
assert.strictEqual(esc(42), '42', 'a number is coerced to a string');
assert.strictEqual(esc(null), 'null');
assert.strictEqual(esc(''), '');

// ---- envWithBinDir: the binary's directory first in PATH ----
{
  const oldPath = process.env.PATH;
  process.env.PATH = '/usr/bin:/bin';
  try {
    const env = envWithBinDir('/home/u/.npm-global/bin/npm', { DSH: '1' });
    assert.strictEqual(env.PATH, '/home/u/.npm-global/bin' + path.delimiter + '/usr/bin:/bin', 'the bin directory first');
    assert.strictEqual(env.DSH, '1', 'the extra variables are applied');
    assert.strictEqual(env.HOME, process.env.HOME, 'the rest of the environment is kept');
  } finally {
    process.env.PATH = oldPath;
  }
}

// ---- withoutUnitEnv: dsh is not a systemd unit (dshmarket: «self-restart is disabled») ----
{
  const src = { HOME: '/home/u', PATH: '/usr/bin', INVOCATION_ID: 'abc', JOURNAL_STREAM: '8:123', NOTIFY_SOCKET: '/run/n', SYSTEMD_EXEC_PID: '42', DSH_STARTED_BY: 'dsh-launcher' };
  const env = withoutUnitEnv(src);
  for (const k of ['INVOCATION_ID', 'JOURNAL_STREAM', 'NOTIFY_SOCKET', 'SYSTEMD_EXEC_PID']) assert.ok(!(k in env), `without ${k}`);
  assert.deepStrictEqual(env, { HOME: '/home/u', PATH: '/usr/bin', DSH_STARTED_BY: 'dsh-launcher' }, 'the rest — unchanged');
  assert.strictEqual(src.INVOCATION_ID, 'abc', 'the source object is not mutated');
}

// ---- marketInPkg: the market plugin in the profile's package.json ----
const marketInPkg = extract('marketInPkg', { MARKET_PKG: 'dshmarket' });
assert.strictEqual(marketInPkg({ dependencies: { dshmarket: '1.66.8' } }), true, 'a dshmarket dependency');
assert.strictEqual(marketInPkg({ dsh: { profile: { bundles: ['other', 'dshmarket'] } } }), true, 'bundles dsh.profile');
assert.strictEqual(marketInPkg({ dependencies: { dshmarket: '1' }, dsh: { profile: { bundles: [] } } }), true, 'any marker');
assert.strictEqual(marketInPkg({ dependencies: { other: '1' } }), false);
assert.strictEqual(marketInPkg({ dsh: { profile: {} } }), false, 'no bundles → false');
assert.strictEqual(marketInPkg({}), false);

// ---- proxyUrlFromRule: Chromium proxy rule → URL for the Node agent ----
assert.strictEqual(proxyUrlFromRule('DIRECT'), null, 'DIRECT → direct');
assert.strictEqual(proxyUrlFromRule(''), null, 'empty → direct');
assert.strictEqual(proxyUrlFromRule(undefined), null, 'no rule → direct');
assert.strictEqual(proxyUrlFromRule('PROXY 10.0.0.1:3128'), 'http://10.0.0.1:3128', 'PROXY');
assert.strictEqual(proxyUrlFromRule('PROXY proxy.corp:8080; DIRECT'), 'http://proxy.corp:8080', 'PROXY; DIRECT — the first one');
assert.strictEqual(proxyUrlFromRule('HTTPS secure.corp:443'), 'https://secure.corp:443', 'an HTTPS proxy');
assert.strictEqual(proxyUrlFromRule('SOCKS5 127.0.0.1:1080'), null, 'SOCKS is not supported → direct');
assert.strictEqual(proxyUrlFromRule('SOCKS5 s:1; PROXY p:3128'), 'http://p:3128', 'SOCKS is skipped, HTTP is taken');
assert.strictEqual(proxyUrlFromRule('proxy p:1'), 'http://p:1', 'case-insensitive');

// ---- DSH_LAUNCHER_GITHUB_TOKEN: for CI only, never leaves the process ----
{
  const { mainSrc } = require('./_extract');
  const envSrc = require('fs').readFileSync(require('path').join(__dirname, '..', 'src', 'env.js'), 'utf8');
  assert.ok(envSrc.includes('delete process.env.DSH_LAUNCHER_GITHUB_TOKEN'), 'the token variable is removed from the environment (src/env.js)');
  const del = mainSrc.indexOf("require('./src/env')");
  assert.ok(del > 0, 'main.js loads src/env.js');
  const firstChild = Math.min(...['execFileSync(', 'spawn(', 'execFileAsync(']
    .map((k) => mainSrc.indexOf(k, mainSrc.indexOf("require('child_process')") + 30)).filter((x) => x > 0));
  assert.ok(del < firstChild, 'the token is removed from the environment before the first child process starts');
  // the whole launcher code: main.js and the src/ modules
  const srcDir = require('path').join(__dirname, '..', 'src');
  const allSrc = [mainSrc, ...require('fs').readdirSync(srcDir).filter((f) => f.endsWith('.js'))
    .map((f) => require('fs').readFileSync(require('path').join(srcDir, f), 'utf8'))].join('\n');
  const uses = allSrc.match(/Bearer \$\{GITHUB_TOKEN\}/g) || [];
  assert.strictEqual(uses.length, 1, 'the token is substituted in one place');
  const fn = allSrc.slice(allSrc.indexOf('async function githubReleasesList('), allSrc.indexOf('async function githubReleases('));
  assert.ok(fn.includes('Bearer ${GITHUB_TOKEN}'), '— and it is the header of the common GitHub requests (api.github.com)');
  assert.ok(!/console\.(log|error)\([^)]*\$\{GITHUB_TOKEN\}/.test(allSrc), 'the token value is not printed');
}

// ---- dpkgStatusVersion: package version from /var/lib/dpkg/status ----
{
  const v = dpkgStatusVersion;
  const db = [
    'Package: dsh-launcher-extra', 'Status: install ok installed', 'Version: 9.9.9-1', '',
    'Package: dsh-launcher', 'Status: install ok installed', 'Priority: optional', 'Version: 1.11.0-1', 'Description: x', ' multi-line', '',
    'Package: other', 'Status: deinstall ok config-files', 'Version: 2.0', '',
  ].join('\n');
  assert.strictEqual(v(db, 'dsh-launcher'), '1.11.0-1', 'the installed package');
  assert.strictEqual(v(db, 'other'), null, 'removed (config-files remain) — not installed');
  assert.strictEqual(v(db, 'absent'), null, 'no such package');
  assert.strictEqual(v('', 'dsh-launcher'), null, 'an empty database');
  assert.strictEqual(v(null, 'dsh-launcher'), null, 'no file');
}

// ---- githubApiBase: a substitute GitHub API — only http://127.0.0.1:<port> ----
{
  const g = extract('githubApiBase');
  const real = 'https://api.github.com';
  assert.strictEqual(g('http://127.0.0.1:8765'), 'http://127.0.0.1:8765', 'a local substitute');
  for (const v of [undefined, '', 'http://127.0.0.1', 'http://127.0.0.1:8765/', 'https://127.0.0.1:8765',
    'http://localhost:8765', 'http://10.0.0.1:8765', 'http://127.0.0.1:8765@evil.example', 'http://127.0.0.1.evil.example:80']) {
    assert.strictEqual(g(v), real, `rejected: ${v}`);
  }
}

// ---- navAction: where in-window navigation is allowed ----
{
  const nav = extract('navAction');
  const GUI = 'http://127.0.0.1:3080';
  assert.strictEqual(nav('http://127.0.0.1:3080/chat/1', 'http://127.0.0.1:3080/', GUI), 'allow', 'inside the dsh GUI');
  assert.strictEqual(nav('http://127.0.0.1:3080/', 'data:text/html,x', GUI), 'allow', 'from a launcher page — into the GUI');
  assert.strictEqual(nav('dshlauncher://k/menu/about/', 'http://127.0.0.1:3080/', GUI), 'allow', 'a launcher route (the handler checks the key)');
  assert.strictEqual(nav('https://example.com/', 'http://127.0.0.1:3080/', GUI), 'external', 'a foreign site — into the browser');
  assert.strictEqual(nav('http://127.0.0.1:3081/', 'http://127.0.0.1:3080/', GUI), 'external', 'a different port — foreign');
  assert.strictEqual(nav('https://example.com/b', 'https://example.com/a', GUI), 'allow', 'inside a page opened via «Вставить ссылку»');
  assert.strictEqual(nav('https://example.com/', 'data:text/html,x', null), 'external', 'the launcher window: http — into the browser');
  assert.strictEqual(nav('https://example.com/b', 'https://example.com/a', null), 'external', 'the launcher window: there is no "own" site');
  for (const u of ['file:///etc/passwd', 'javascript:alert(1)', 'data:text/html,x', 'chrome://gpu', 'не url']) {
    assert.strictEqual(nav(u, 'http://127.0.0.1:3080/', GUI), 'deny', `denied: ${u}`);
  }
}

// ---- authUrlFromOutput: the auth link from dsh's output ----
{
  const { authUrlFromOutput: auth } = require('../src/system');
  const PLAIN = 'http://127.0.0.1:3080/';
  assert.strictEqual(auth('', PLAIN), null, 'empty → null');
  assert.strictEqual(auth('dsh web: http://127.0.0.1:3080/?token=a\n', PLAIN), 'http://127.0.0.1:3080/?token=a');
  assert.strictEqual(auth('dsh web: http://127.0.0.1:3080/?token=a\nx\ndsh web: http://127.0.0.1:3080/?token=b', PLAIN),
    'http://127.0.0.1:3080/?token=b', 'several — the last one (a restart into the same file)');
  assert.strictEqual(auth('dsh web: http://127.0.0.1:3081/?token=a', PLAIN), null, 'a different port — not ours');
  assert.strictEqual(auth('dsh web: http://evil.example/?token=a', PLAIN), null, 'a different host — not ours');
  assert.strictEqual(auth('dsh web: http://127.0.0.1:3080/', PLAIN), null, 'no token — null');
  assert.strictEqual(auth('see http://127.0.0.1:3080/?token=a', PLAIN), null, 'not a «dsh web:» line — null');
  assert.strictEqual(auth('dsh web: http://127.0.0.1:3080/?token=a\ndsh web: http://[', PLAIN),
    'http://127.0.0.1:3080/?token=a', 'a fragment at the end of the tail — skipped');
}

console.log('OK: parsing — all checks passed');
