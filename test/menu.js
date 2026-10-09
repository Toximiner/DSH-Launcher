#!/usr/bin/env node
// Юнит-тест чистого помощника menuBackTarget (main.js, между маркерами
// «===== menuBackTarget … =====»). main.js целиком не загружается (electron).
'use strict';
const assert = require('assert');
const { extract } = require('./_extract');

const menuBackTarget = extract('menuBackTarget');
const page = function fakePage() {};

// GUI dsh (тот же origin, что PLAIN_URL) → null = вернуться в GUI
assert.strictEqual(menuBackTarget('http://127.0.0.1:3080/?token=abc', 'http://127.0.0.1:3080', page), null, 'GUI с токеном → null');
assert.strictEqual(menuBackTarget('http://127.0.0.1:3080/chat/xyz', 'http://127.0.0.1:3080', page), null, 'маршрут внутри GUI → null');
assert.strictEqual(menuBackTarget('http://127.0.0.1:3080', 'http://127.0.0.1:3080', page), null, 'корень GUI → null');

// Служебная страница (data:) → её рендер-функция
assert.strictEqual(menuBackTarget('data:text/html;charset=utf-8,hello', 'http://127.0.0.1:3080', page), page, 'data: URL → currentPage');

// about:blank / мусор → currentPage (new URL бросит или origin не совпадёт)
assert.strictEqual(menuBackTarget('about:blank', 'http://127.0.0.1:3080', page), page, 'about:blank → currentPage');
assert.strictEqual(menuBackTarget('not a url', 'http://127.0.0.1:3080', page), page, 'мусор → currentPage');

// Другой порт — это не GUI dsh
assert.strictEqual(menuBackTarget('http://127.0.0.1:3181/?token=x', 'http://127.0.0.1:3080', page), page, 'чужой порт → currentPage');

// currentPage == null (окно ещё на about:blank) → null в любом случае
assert.strictEqual(menuBackTarget('data:text/html,hi', 'http://127.0.0.1:3080', null), null, 'currentPage null → null');

// ---- contextMenuItems: пункты контекстного меню по правому клику ----
const ctx = extract('contextMenuItems', { tr: (ru) => ru });
const acts = (items) => items.map((i) => (i.type ? '|' : i.action + (i.enabled ? '' : '-')));
const all = { canCut: true, canCopy: true, canPaste: true, canSelectAll: true };

assert.deepStrictEqual(acts(ctx({ isEditable: true, editFlags: all, selectionText: 'x' })),
  ['cut', 'copy', 'paste', '|', 'selectAll'], 'поле ввода с выделением — всё доступно');
assert.deepStrictEqual(acts(ctx({ isEditable: true, editFlags: { canPaste: true, canSelectAll: true }, selectionText: '' })),
  ['cut-', 'copy-', 'paste', '|', 'selectAll'], 'поле без выделения — вырезать/копировать недоступны');
assert.deepStrictEqual(acts(ctx({ isEditable: false, editFlags: {}, selectionText: 'текст' })),
  ['copy', '|', 'selectAll'], 'выделенный текст на странице — копировать');
assert.deepStrictEqual(acts(ctx({ isEditable: false, editFlags: {}, selectionText: '  \n' })),
  ['selectAll'], 'выделены только пробелы — без «Копировать»');
assert.deepStrictEqual(acts(ctx({ isEditable: false, editFlags: {}, selectionText: '', linkURL: 'https://x/' })),
  ['copyLink', '|', 'selectAll'], 'ссылка — копировать адрес');
assert.deepStrictEqual(acts(ctx({ isEditable: false, selectionText: 'a', linkURL: 'https://x/' })),
  ['copyLink', '|', 'copy', '|', 'selectAll'], 'выделенная ссылка; editFlags может не быть');
assert.strictEqual(ctx({ isEditable: true, editFlags: all }).find((i) => i.action === 'paste').label, 'Вставить', 'подписи через tr');

// Английские подписи
const ctxEn = extract('contextMenuItems', { tr: (ru, en) => en });
assert.deepStrictEqual(
  ctxEn({ isEditable: true, editFlags: all, linkURL: 'https://x/' }).filter((i) => !i.type).map((i) => i.label),
  ['Copy link address', 'Cut', 'Copy', 'Paste', 'Select all'], 'EN-подписи');

// Перебор всех сочетаний: разделители только между группами (не в начале,
// не в конце, не два подряд), меню никогда не пустое, у пунктов есть подпись.
for (const isEditable of [true, false]) {
  for (const linkURL of ['', 'https://x/']) {
    for (const selectionText of ['', ' ', 'a']) {
      for (const editFlags of [undefined, {}, all]) {
        const items = ctx({ isEditable, linkURL, selectionText, editFlags });
        const where = JSON.stringify({ isEditable, linkURL, selectionText, editFlags });
        assert.ok(items.length > 0, 'меню не пустое: ' + where);
        assert.ok(!items[0].type && !items[items.length - 1].type, 'нет разделителя по краям: ' + where);
        items.forEach((it, k) => {
          if (it.type) assert.ok(!items[k + 1].type, 'нет двух разделителей подряд: ' + where);
          else assert.ok(it.label && it.action && typeof it.enabled === 'boolean', 'пункт полный: ' + where);
        });
      }
    }
  }
}

// ---- showContextMenu: что делает каждый пункт и куда показывается меню ----
function fakeEnv({ destroyed = false } = {}) {
  const calls = [];
  const wc = {};
  for (const m of ['cut', 'copy', 'paste', 'selectAll']) wc[m] = () => calls.push('wc.' + m);
  const win = { webContents: wc, isDestroyed: () => destroyed };
  const env = { calls, win, template: null, popupOpts: null };
  const Menu = {
    buildFromTemplate: (t) => { env.template = t; return { popup: (o) => { env.popupOpts = o; calls.push('popup'); } }; },
  };
  const clipboard = { writeText: (t) => calls.push('clipboard:' + t) };
  env.logs = [];
  env.show = extract('showContextMenu', { win, Menu, clipboard, contextMenuItems: ctx, console: { log: (m) => env.logs.push(m) } });
  return env;
}
const byLabel = (env, label) => env.template.find((i) => i.label === label);

{
  // Поле ввода: каждый пункт вызывает свой метод webContents, доступность передаётся
  const env = fakeEnv();
  env.show({ isEditable: true, editFlags: { canPaste: true, canSelectAll: true }, selectionText: '' });
  assert.strictEqual(env.popupOpts.window, env.win, 'меню показывается в окне лаунчера');
  assert.deepStrictEqual(env.template.map((i) => i.type || i.label),
    ['Вырезать', 'Копировать', 'Вставить', 'separator', 'Выделить всё'], 'шаблон меню');
  assert.strictEqual(byLabel(env, 'Вырезать').enabled, false, 'нет выделения — «Вырезать» недоступен');
  assert.strictEqual(byLabel(env, 'Вставить').enabled, true, '«Вставить» доступен');
  assert.deepStrictEqual(env.logs, ['[launcher] контекстное меню: Вырезать (off), Копировать (off), Вставить, |, Выделить всё'], 'строка в лог');
  for (const [label, call] of [['Вырезать', 'wc.cut'], ['Копировать', 'wc.copy'], ['Вставить', 'wc.paste'], ['Выделить всё', 'wc.selectAll']]) {
    env.calls.length = 0;
    byLabel(env, label).click();
    assert.deepStrictEqual(env.calls, [call], `«${label}» → ${call}`);
  }
}
{
  // Ссылка: адрес — в буфер обмена, а не через webContents
  const env = fakeEnv();
  env.show({ isEditable: false, linkURL: 'https://example.com/a?b=1', selectionText: '' });
  env.calls.length = 0;
  byLabel(env, 'Копировать адрес ссылки').click();
  assert.deepStrictEqual(env.calls, ['clipboard:https://example.com/a?b=1'], 'адрес ссылки в буфере');
}
{
  // Окно закрыто — меню не строится и не показывается
  const env = fakeEnv({ destroyed: true });
  env.show({ isEditable: true, editFlags: all });
  assert.strictEqual(env.template, null, 'закрытое окно — без меню');
  assert.deepStrictEqual(env.calls, [], 'закрытое окно — ничего не вызвано');
}

// Обработчик правого клика подключён к окну
const { mainSrc } = require('./_extract');
assert.ok(/win\.webContents\.on\('context-menu',\s*\(_e, params\) => showContextMenu\(params\)\)/.test(mainSrc),
  'main.js: context-menu → showContextMenu');

console.log('OK: menuBackTarget, contextMenuItems, showContextMenu — все проверки прошли');
