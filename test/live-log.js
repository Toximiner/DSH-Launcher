#!/usr/bin/env node
// Юнит-тесты живого лога на страницах-статусах (src/pages.js): liveLogAppend
// — дописывание строк на странице (на подставном <pre>), liveLogFresh — какие
// строки дослать на страницу. Страница больше не перезагружается, поэтому
// скролл остаётся за пользователем.
'use strict';
const assert = require('assert');
const { extract } = require('./_extract');

// Подставный document с одним <pre id="live-log">.
const mkDoc = () => {
  const pre = { textContent: '…', scrollTop: 0, clientHeight: 100, scrollHeight: 100 };
  return { pre, document: { getElementById: (id) => (id === 'live-log' ? pre : null) } };
};

const append = extract('liveLogAppend', {});
const fresh = extract('liveLogFresh', {});

// На странице нет <pre> (уже ушли с неё) — false
{
  const doc = mkDoc();
  doc.document.getElementById = () => null;
  assert.strictEqual(append(doc.document, ['x']), false, 'нет pre — false');
}
// Лог пуст (плейсхолдер «…») — первые строки пишутся без плейсхолдера
{
  const { pre, document } = mkDoc();
  assert.strictEqual(append(document, ['a', 'b']), true, 'pre есть — true');
  assert.strictEqual(pre.textContent, 'a\nb', 'плейсхолдер заменён');
}
// Дописывание сохраняет старые строки
{
  const { pre, document } = mkDoc();
  append(document, ['a']);
  append(document, ['b']);
  assert.strictEqual(pre.textContent, 'a\nb', 'старое + новое');
}
// Пользователь прижат ко дну — остаётся прижатым после новых строк
{
  const { pre, document } = mkDoc();
  append(document, ['a', 'b', 'c']);
  pre.scrollTop = pre.scrollHeight - pre.clientHeight;
  assert.strictEqual(append(document, ['d']), true);
  assert.strictEqual(pre.scrollTop, pre.scrollHeight, 'скролл прижат ко дну');
}
// Пользователь читает не у дна — скролл не трогают
{
  const { pre, document } = mkDoc();
  append(document, ['a', 'b', 'c']);
  pre.scrollTop = 10;
  pre.scrollHeight = 500;
  pre.clientHeight = 100;
  assert.strictEqual(append(document, ['d']), true);
  assert.strictEqual(pre.scrollTop, 10, 'позиция скролла сохранена');
}
// Лимит 500 строк: при превышении старые уходят сверху
{
  const { pre, document } = mkDoc();
  append(document, Array.from({ length: 500 }, (_, i) => `l${i}`));
  append(document, ['last']);
  const ls = pre.textContent.split('\n');
  assert.strictEqual(ls.length, 500, 'не больше 500');
  assert.strictEqual(ls[0], 'l1', 'первая старая строка ушла');
  assert.strictEqual(ls[499], 'last', 'новые — внизу');
}
// liveLogFresh: новые строки с момента последней отрисовки
{
  const st = { log: ['a', 'b', 'c'], logSeq: 3 };
  assert.strictEqual(fresh(st, 3), null, 'новых нет — null');
  assert.deepStrictEqual(fresh(st, 0), ['a', 'b', 'c'], 'со старта — все');
  assert.deepStrictEqual(fresh(st, 2), ['c'], 'с последней shown — только новое');
}
// liveLogFresh: лог обрезан до 500 строк (logSeq > длины) — индекс от logSeq
{
  const log = Array.from({ length: 500 }, (_, i) => String(i + 101)); // строки 101…600
  const st = { log, logSeq: 600 };
  assert.strictEqual(fresh(st, 100).length, 500, 'весь остаток окна (1…100 ушли из лога, но уже показаны)');
  assert.deepStrictEqual(fresh(st, 100).slice(-1), ['600'], 'новые — в конце');
  assert.strictEqual(fresh(st, 499).length, 101, 'строки 500…600 — всего остатка после shown');
  assert.strictEqual(fresh(st, 499)[0], '500', 'первая — после shown');
  assert.deepStrictEqual(fresh(st, 599), ['600'], 'последняя строка после обрезки');
}

console.log('OK: liveLogAppend, liveLogFresh — все проверки прошли');
