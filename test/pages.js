#!/usr/bin/env node
// Unit tests for page helpers and checks: timedCheck, groupAlive,
// pageHtml, retryButton (main.js), closingHtml.
'use strict';
const assert = require('assert');
const { spawn } = require('child_process');
const { extract } = require('./_extract');

(async () => {
  // ---- timedCheck: a network call with a deadline must not hold the window on a question ----
  const { timedCheck } = require('../src/util');
  assert.strictEqual(await timedCheck(Promise.resolve(42), 1000), 42, 'a fast value is returned');
  assert.strictEqual(await timedCheck(new Promise(() => {}), 50), null, 'timeout → null');
  assert.strictEqual(await timedCheck(Promise.reject(new Error('x')), 1000), null, 'a promise rejection → null');

  // ---- retryButton: the "Check again" button ----
  const retryButton = extract('retryButton', { tr: (ru, en) => ru });
  assert.ok(retryButton().includes('dshlauncher://retry/'), 'the retry link');
  assert.ok(retryButton().includes('Проверить снова'), 'the RU label');

  // ---- closingHtml: the "closing…" window ----
  {
    const closingHtml = extract('closingHtml', { esc: require('../src/util').esc, CSP_META: 'CSP' });
    const data = closingHtml('Закрывается <x>', 'Подождите', 'data:image/png;base64,AAA');
    assert.ok(data.startsWith('data:text/html;charset=utf-8,'), 'data: URL');
    const h = decodeURIComponent(data.slice('data:text/html;charset=utf-8,'.length));
    assert.ok(h.includes('<h1>Закрывается &lt;x&gt;</h1>') && h.includes('<title>Закрывается &lt;x&gt;</title>'), 'the heading is escaped');
    assert.ok(h.includes('<p>Подождите</p>') && h.includes('class="spin"'), 'the text and the spinner');
    assert.ok(h.includes('<img src="data:image/png;base64,AAA"'), 'the icon');
    assert.ok(!closingHtml('t', 'x', '').includes('<img'), 'no icon — no <img>');
    assert.ok(!/<script/i.test(h) && !h.includes('dshlauncher://'), 'no scripts and no routes');
  }

  // ---- pageHtml: markup of a service page ----
  const { esc, withRouteKey } = require('../src/util');
  const KEY = 'ab'.repeat(16);
  const pageHtml = extract('pageHtml', { i18n: { getLang: () => 'ru' }, esc, PAGE_CSS: 'CSS', withRouteKey, ROUTE_KEY: KEY, CSP_META: 'CSP' });
  const prefix = 'data:text/html;charset=utf-8,';
  const htmlOf = (data) => decodeURIComponent(data.slice(prefix.length));

  const ok = pageHtml('Название', '<p>тело</p>', false);
  assert.ok(ok.startsWith(prefix), 'data: URL');
  const html = htmlOf(ok);
  assert.ok(html.includes('<html lang="ru">'), 'html lang from UI_LANG');
  assert.ok(html.includes('<title>Название</title>'), 'title');
  assert.ok(html.includes('<div class="wrap">'), 'no error → no err class');
  assert.ok(html.includes(`dshlauncher://${KEY}/lang/ru/`) && html.includes('class="on"'), 'the active language is marked');
  assert.ok(!html.replaceAll(`dshlauncher://${KEY}/`, '').includes('dshlauncher://'), 'all page links carry the launch key');
  assert.ok(html.includes('<p>тело</p>'), 'the body is not escaped (trusted HTML)');

  // Dialog window mode: no language switch, dlg class on body
  const dlg = htmlOf(pageHtml('About', '<p>x</p>', false, { dialog: true }));
  assert.ok(dlg.includes('<body class="dlg">'), 'dialog: class="dlg"');
  assert.ok(!dlg.includes('/lang/'), 'dialog: no RU | EN switch');
  assert.ok(dlg.includes('<title>About</title>') && dlg.includes('<p>x</p>'), 'dialog: heading and body');
  assert.ok(html.includes('<body><div class="lang">'), 'a plain page: body without a class');

  const bad = htmlOf(pageHtml('a<b', '', true));
  assert.ok(bad.includes('<div class="wrap err">'), 'error → err class');
  assert.ok(bad.includes('a&lt;b'), 'the title is escaped');

  // ---- groupAlive: whether a process or its whole group is alive ----
  const { groupAlive } = require('../src/system');
  assert.strictEqual(groupAlive(process.pid), true, 'own pid → alive');
  assert.strictEqual(groupAlive(999999999), false, 'nonexistent → false');
  assert.strictEqual(groupAlive(999999999, true), false, 'groupOnly, nonexistent → false');
  {
    // A detached process — leader of its own group (pgid == pid).
    const child = spawn(process.execPath, ['-e', 'setTimeout(()=>{},3000)'],
      { detached: true, stdio: 'ignore' });
    child.unref();
    await new Promise(r => setTimeout(r, 100)); // give the detached child time to appear in /proc
    assert.strictEqual(groupAlive(child.pid, true), true, 'a detached group leader → the group is alive');
    child.kill('SIGKILL');
  }

  console.log('OK: pages — all checks passed');
})().catch((e) => { console.error(e); process.exit(1); });
