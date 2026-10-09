#!/usr/bin/env node
// Юнит-тесты помощников страниц и проверок: timedCheck, groupAlive,
// pageHtml, retryButton (main.js).
'use strict';
const assert = require('assert');
const fs = require('fs');
const { spawn } = require('child_process');
const { extract } = require('./_extract');

(async () => {
  // ---- timedCheck: сеть с дедлайном не должна держать окно на вопросе ----
  const timedCheck = extract('timedCheck');
  assert.strictEqual(await timedCheck(Promise.resolve(42), 1000), 42, 'быстрое значение возвращается');
  assert.strictEqual(await timedCheck(new Promise(() => {}), 50), null, 'таймаут → null');
  assert.strictEqual(await timedCheck(Promise.reject(new Error('x')), 1000), null, 'ошибка промиса → null');

  // ---- retryButton: кнопка «Проверить снова» ----
  const retryButton = extract('retryButton', { tr: (ru, en) => ru });
  assert.ok(retryButton().includes('dshlauncher://retry/'), 'ссылка retry');
  assert.ok(retryButton().includes('Проверить снова'), 'RU-подпись');

  // ---- pageHtml: разметка служебной страницы ----
  const esc = extract('esc');
  const pageHtml = extract('pageHtml', { UI_LANG: 'ru', esc, PAGE_CSS: 'CSS' });
  const prefix = 'data:text/html;charset=utf-8,';
  const htmlOf = (data) => decodeURIComponent(data.slice(prefix.length));

  const ok = pageHtml('Название', '<p>тело</p>', false);
  assert.ok(ok.startsWith(prefix), 'data: URL');
  const html = htmlOf(ok);
  assert.ok(html.includes('<html lang="ru">'), 'html lang из UI_LANG');
  assert.ok(html.includes('<title>Название</title>'), 'title');
  assert.ok(html.includes('<div class="wrap">'), 'без ошибки → без err-класса');
  assert.ok(html.includes('dshlauncher://lang/ru/') && html.includes('class="on"'), 'активный язык помечен');
  assert.ok(html.includes('<p>тело</p>'), 'body не экранируется (доверенный HTML)');

  const bad = htmlOf(pageHtml('a<b', '', true));
  assert.ok(bad.includes('<div class="wrap err">'), 'ошибка → err-класс');
  assert.ok(bad.includes('a&lt;b'), 'title экранируется');

  // ---- groupAlive: жив ли процесс или вся его группа ----
  const groupAlive = extract('groupAlive', { process });
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
