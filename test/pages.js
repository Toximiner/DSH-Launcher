#!/usr/bin/env node
// Юнит-тесты помощников страниц и проверок: timedCheck, groupAlive,
// pageHtml, retryButton (main.js), closingHtml.
'use strict';
const assert = require('assert');
const { spawn } = require('child_process');
const { extract } = require('./_extract');

(async () => {
  // ---- timedCheck: сеть с дедлайном не должна держать окно на вопросе ----
  const { timedCheck } = require('../src/util');
  assert.strictEqual(await timedCheck(Promise.resolve(42), 1000), 42, 'быстрое значение возвращается');
  assert.strictEqual(await timedCheck(new Promise(() => {}), 50), null, 'таймаут → null');
  assert.strictEqual(await timedCheck(Promise.reject(new Error('x')), 1000), null, 'ошибка промиса → null');

  // ---- retryButton: кнопка «Проверить снова» ----
  const retryButton = extract('retryButton', { tr: (ru, en) => ru });
  assert.ok(retryButton().includes('dshlauncher://retry/'), 'ссылка retry');
  assert.ok(retryButton().includes('Проверить снова'), 'RU-подпись');

  // ---- closingHtml: окошко «закрывается…» ----
  {
    const closingHtml = extract('closingHtml', { esc: require('../src/util').esc, CSP_META: 'CSP' });
    const data = closingHtml('Закрывается <x>', 'Подождите', 'data:image/png;base64,AAA');
    assert.ok(data.startsWith('data:text/html;charset=utf-8,'), 'data: URL');
    const h = decodeURIComponent(data.slice('data:text/html;charset=utf-8,'.length));
    assert.ok(h.includes('<h1>Закрывается &lt;x&gt;</h1>') && h.includes('<title>Закрывается &lt;x&gt;</title>'), 'заголовок экранирован');
    assert.ok(h.includes('<p>Подождите</p>') && h.includes('class="spin"'), 'текст и крутилка');
    assert.ok(h.includes('<img src="data:image/png;base64,AAA"'), 'значок');
    assert.ok(!closingHtml('t', 'x', '').includes('<img'), 'нет значка — нет <img>');
    assert.ok(!/<script/i.test(h) && !h.includes('dshlauncher://'), 'без скриптов и маршрутов');
  }

  // ---- pageHtml: разметка служебной страницы ----
  const { esc, withRouteKey } = require('../src/util');
  const KEY = 'ab'.repeat(16);
  const pageHtml = extract('pageHtml', { i18n: { getLang: () => 'ru' }, esc, PAGE_CSS: 'CSS', withRouteKey, ROUTE_KEY: KEY, CSP_META: 'CSP' });
  const prefix = 'data:text/html;charset=utf-8,';
  const htmlOf = (data) => decodeURIComponent(data.slice(prefix.length));

  const ok = pageHtml('Название', '<p>тело</p>', false);
  assert.ok(ok.startsWith(prefix), 'data: URL');
  const html = htmlOf(ok);
  assert.ok(html.includes('<html lang="ru">'), 'html lang из UI_LANG');
  assert.ok(html.includes('<title>Название</title>'), 'title');
  assert.ok(html.includes('<div class="wrap">'), 'без ошибки → без err-класса');
  assert.ok(html.includes(`dshlauncher://${KEY}/lang/ru/`) && html.includes('class="on"'), 'активный язык помечен');
  assert.ok(!html.replaceAll(`dshlauncher://${KEY}/`, '').includes('dshlauncher://'), 'все ссылки страницы — с ключом запуска');
  assert.ok(html.includes('<p>тело</p>'), 'body не экранируется (доверенный HTML)');

  // Режим окна-диалога: без переключателя языка, класс dlg на body
  const dlg = htmlOf(pageHtml('About', '<p>x</p>', false, { dialog: true }));
  assert.ok(dlg.includes('<body class="dlg">'), 'диалог: class="dlg"');
  assert.ok(!dlg.includes('/lang/'), 'диалог: без переключателя RU | EN');
  assert.ok(dlg.includes('<title>About</title>') && dlg.includes('<p>x</p>'), 'диалог: заголовок и тело');
  assert.ok(html.includes('<body><div class="lang">'), 'обычная страница: body без класса');

  const bad = htmlOf(pageHtml('a<b', '', true));
  assert.ok(bad.includes('<div class="wrap err">'), 'ошибка → err-класс');
  assert.ok(bad.includes('a&lt;b'), 'title экранируется');

  // ---- groupAlive: жив ли процесс или вся его группа ----
  const { groupAlive } = require('../src/system');
  assert.strictEqual(groupAlive(process.pid), true, 'свой pid → жив');
  assert.strictEqual(groupAlive(999999999), false, 'несуществующий → false');
  assert.strictEqual(groupAlive(999999999, true), false, 'groupOnly, несуществующий → false');
  {
    // Детаченый процесс — лидер собственной группы (pgid == pid).
    const child = spawn(process.execPath, ['-e', 'setTimeout(()=>{},3000)'],
      { detached: true, stdio: 'ignore' });
    child.unref();
    await new Promise(r => setTimeout(r, 100)); // даём детаченному ребёнку время появиться в /proc
    assert.strictEqual(groupAlive(child.pid, true), true, 'детаченый лидер группы → группа жива');
    child.kill('SIGKILL');
  }

  console.log('OK: pages — все проверки прошли');
})().catch((e) => { console.error(e); process.exit(1); });
