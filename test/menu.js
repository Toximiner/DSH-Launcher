#!/usr/bin/env node
// Юнит-тесты меню (main.js): contextMenuItems — какие пункты контекстного
// меню показать, showContextMenu — что делает каждый пункт (на подставных
// окне, Menu и буфере обмена), appMenuTemplate — строка меню окна.
// main.js целиком не загружается (electron).
'use strict';
const assert = require('assert');
const { extract } = require('./_extract');

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
  // Окно передано явно (диалог «О программе»): меню и действия — в нём
  const env = fakeEnv();
  const calls2 = [];
  const dlg = { isDestroyed: () => false, webContents: { cut() {}, copy: () => calls2.push('dlg.copy'), paste() {}, selectAll() {} } };
  env.show({ isEditable: false, selectionText: 'v1.4.0' }, dlg);
  assert.strictEqual(env.popupOpts.window, dlg, 'меню показывается в окне-диалоге');
  byLabel(env, 'Копировать').click();
  assert.deepStrictEqual(calls2, ['dlg.copy'], '«Копировать» — в диалоге');
  assert.deepStrictEqual(env.calls.filter((c) => c.startsWith('wc.')), [], 'главное окно не тронуто');
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

// ---- appMenuTemplate: строка меню окна ----
{
  const calls = [];
  const mk = (tr) => extract('appMenuTemplate', {
    tr,
    showAboutDialog: () => calls.push('about'),
    showUpdateCheckDialog: () => calls.push('check'),
  })();
  const ru = mk((r) => r);
  const en = mk((r, e) => e);
  assert.deepStrictEqual(ru.map((m) => m.label), ['Приложение', 'Правка'], 'пункты строки меню (RU)');
  assert.deepStrictEqual(en.map((m) => m.label), ['Application', 'Edit'], 'пункты строки меню (EN)');

  const app = ru[0].submenu;
  app.find((i) => i.label === 'О программе…').click();
  app.find((i) => i.label === 'Проверить обновления…').click();
  assert.deepStrictEqual(calls, ['about', 'check'], '«О программе» и «Проверить обновления» открывают свои окна');
  assert.strictEqual(app.find((i) => i.label === 'Выйти').role, 'quit', '«Выйти» — штатный quit');

  // «Правка» — те же действия, что в контекстном меню, стандартными ролями
  const edit = (t) => t[1].submenu.map((i) => (i.type ? '|' : `${i.label}:${i.role}:${i.accelerator}`));
  assert.deepStrictEqual(edit(ru),
    ['Вырезать:cut:CmdOrCtrl+X', 'Копировать:copy:CmdOrCtrl+C', 'Вставить:paste:CmdOrCtrl+V', '|', 'Выделить всё:selectAll:CmdOrCtrl+A'],
    '«Правка» (RU)');
  assert.deepStrictEqual(edit(en),
    ['Cut:cut:CmdOrCtrl+X', 'Copy:copy:CmdOrCtrl+C', 'Paste:paste:CmdOrCtrl+V', '|', 'Select all:selectAll:CmdOrCtrl+A'],
    '«Правка» (EN)');
  // те же подписи, что у контекстного меню
  const ctxLabels = ctx({ isEditable: true, editFlags: all }).filter((i) => !i.type).map((i) => i.label);
  assert.deepStrictEqual(ru[1].submenu.filter((i) => !i.type).map((i) => i.label), ctxLabels, 'подписи совпадают с контекстным меню');
  // сочетания — только подсказкой: не перехватывать горячие клавиши GUI dsh
  for (const i of ru[1].submenu.filter((x) => !x.type)) {
    assert.strictEqual(i.registerAccelerator, false, `«${i.label}»: registerAccelerator: false`);
  }
}

console.log('OK: contextMenuItems, showContextMenu, appMenuTemplate — все проверки прошли');
