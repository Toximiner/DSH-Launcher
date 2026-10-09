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
  const viewActions = new Proxy({}, { get: (_t, k) => () => calls.push('view.' + String(k)) });
  const mk = (tr, spellcheckOn = true) => extract('appMenuTemplate', {
    tr,
    spellcheckOn,
    viewActions,
    showAboutDialog: () => calls.push('about'),
    showUpdateCheckDialog: () => calls.push('check'),
    openLogsFolder: () => calls.push('logs'),
    restartDsh: () => calls.push('restart'),
    setSpellcheck: (on) => calls.push('spell:' + on),
    showLogDialog: () => calls.push('log'),
  })();
  const ru = mk((r) => r);
  const en = mk((r, e) => e);
  assert.deepStrictEqual(ru.map((m) => m.label), ['Файл', 'Правка', 'Вид', 'Справка'], 'пункты строки меню (RU)');
  assert.deepStrictEqual(en.map((m) => m.label), ['File', 'Edit', 'View', 'Help'], 'пункты строки меню (EN)');
  const item = (menu, label) => menu.submenu.find((i) => i.label === label);
  const labels = (menu) => menu.submenu.map((i) => (i.type === 'separator' ? '|' : i.label));

  // «Файл»: перезапуск dsh и выход
  assert.deepStrictEqual(labels(ru[0]), ['Перезапустить dsh…', '|', 'Выйти'], '«Файл»');
  calls.length = 0;
  item(ru[0], 'Перезапустить dsh…').click();
  assert.deepStrictEqual(calls, ['restart'], '«Перезапустить dsh…» → restart');
  assert.strictEqual(item(ru[0], 'Выйти').role, 'quit', '«Выйти» — штатный quit');

  // «Справка»: обновления, логи, «О программе» — последним пунктом
  assert.deepStrictEqual(labels(ru[3]), ['Проверить обновления…', 'Журнал dsh…', 'Открыть папку логов', '|', 'О программе'], '«Справка»');
  assert.deepStrictEqual(labels(en[3]), ['Check for updates…', 'dsh log…', 'Open logs folder', '|', 'About'], '«Help»');
  for (const [label, call] of [['Проверить обновления…', 'check'], ['Журнал dsh…', 'log'], ['Открыть папку логов', 'logs'], ['О программе', 'about']]) {
    calls.length = 0;
    item(ru[3], label).click();
    assert.deepStrictEqual(calls, [call], `«${label}» → ${call}`);
  }

  // «Правка» — те же действия, что в контекстном меню, стандартными ролями
  const edit = (t) => t[1].submenu.map((i) => (i.type ? '|' : `${i.label}:${i.role}:${i.accelerator}`));
  assert.deepStrictEqual(edit(ru),
    ['Вырезать:cut:CmdOrCtrl+X', 'Копировать:copy:CmdOrCtrl+C', 'Вставить:paste:CmdOrCtrl+V', '|', 'Выделить всё:selectAll:CmdOrCtrl+A'],
    '«Правка» (RU)');
  assert.deepStrictEqual(edit(en),
    ['Cut:cut:CmdOrCtrl+X', 'Copy:copy:CmdOrCtrl+C', 'Paste:paste:CmdOrCtrl+V', '|', 'Select all:selectAll:CmdOrCtrl+A'],
    '«Правка» (EN)');
  const ctxLabels = ctx({ isEditable: true, editFlags: all }).filter((i) => !i.type).map((i) => i.label);
  assert.deepStrictEqual(ru[1].submenu.filter((i) => !i.type).map((i) => i.label), ctxLabels, 'подписи «Правки» совпадают с контекстным меню');

  // «Вид» — действие и подсказка-сочетание у каждого пункта
  for (const [label, act, key] of [
    ['Перезагрузить', 'reload', 'F5'], ['Перезагрузить без кэша', 'reloadHard', 'CmdOrCtrl+Shift+R'],
    ['Увеличить', 'zoomIn', 'CmdOrCtrl+='], ['Уменьшить', 'zoomOut', 'CmdOrCtrl+-'], ['Обычный масштаб', 'zoomReset', 'CmdOrCtrl+0'],
    ['Найти на странице…', 'find', 'CmdOrCtrl+F'], ['Полноэкранный режим', 'fullscreen', 'F11']]) {
    calls.length = 0;
    const it = item(ru[2], label);
    assert.ok(it, `«Вид»: есть «${label}»`);
    it.click();
    assert.deepStrictEqual(calls, ['view.' + act], `«${label}» → ${act}`);
    assert.strictEqual(it.accelerator, key, `«${label}»: подсказка ${key}`);
  }
  const spell = item(ru[2], 'Проверка орфографии');
  assert.strictEqual(spell.type, 'checkbox', 'орфография — галочка');
  assert.strictEqual(spell.checked, true, 'галочка отражает состояние (вкл)');
  assert.strictEqual(item(mk((r) => r, false)[2], 'Проверка орфографии').checked, false, 'галочка отражает состояние (выкл)');
  calls.length = 0;
  spell.click({ checked: false });
  assert.deepStrictEqual(calls, ['spell:false'], 'снять галочку → выключить');

  // все сочетания — только подсказки: не перехватывать горячие клавиши GUI dsh
  for (const m of ru) for (const i of m.submenu.filter((x) => x.accelerator)) {
    assert.strictEqual(i.registerAccelerator, false, `«${i.label}»: registerAccelerator: false`);
  }
}

// ---- keyAction: клавиша → действие «Вида» ----
{
  const ka = extract('keyAction');
  const k = (o) => ka({ type: 'keyDown', control: false, shift: false, key: '', code: '', ...o });
  assert.strictEqual(k({ key: 'F5' }), 'reload', 'F5');
  assert.strictEqual(k({ control: true, code: 'KeyR', key: 'к' }), 'reload', 'Ctrl+R в русской раскладке');
  assert.strictEqual(k({ control: true, shift: true, code: 'KeyR' }), 'reloadHard', 'Ctrl+Shift+R');
  assert.strictEqual(k({ control: true, code: 'Equal' }), 'zoomIn', 'Ctrl+=');
  assert.strictEqual(k({ control: true, code: 'NumpadAdd' }), 'zoomIn', 'Ctrl+Num+');
  assert.strictEqual(k({ control: true, code: 'Minus' }), 'zoomOut', 'Ctrl+-');
  assert.strictEqual(k({ control: true, code: 'Digit0' }), 'zoomReset', 'Ctrl+0');
  assert.strictEqual(k({ control: true, code: 'KeyF', key: 'а' }), 'find', 'Ctrl+F в русской раскладке');
  assert.strictEqual(k({ control: true, shift: true, code: 'KeyF' }), null, 'Ctrl+Shift+F — странице');
  assert.strictEqual(k({ key: 'F3' }), 'findNext', 'F3');
  assert.strictEqual(k({ key: 'F3', shift: true }), 'findPrev', 'Shift+F3');
  assert.strictEqual(k({ key: 'F11' }), 'fullscreen', 'F11');
  assert.strictEqual(k({ key: 'f', code: 'KeyF' }), null, 'просто F — странице');
  assert.strictEqual(k({ control: true, code: 'KeyC' }), null, 'Ctrl+C — странице (копирование)');
}

// ---- windowSize: размер окна из сохранённого состояния ----
{
  const deps = { WIN_DEFAULT: { width: 1440, height: 900 }, WIN_MIN: { width: 760, height: 480 } };
  const ws = extract('windowSize', deps);
  const wa = { width: 1920, height: 1050 };
  assert.deepStrictEqual(ws(null, wa), { width: 1440, height: 900, maximized: false }, 'нет состояния → по умолчанию');
  assert.deepStrictEqual(ws({ width: 1200, height: 800, maximized: true }, wa), { width: 1200, height: 800, maximized: true }, 'сохранённое');
  assert.deepStrictEqual(ws({ width: 3000, height: 2000 }, wa), { width: 1920, height: 1050, maximized: false }, 'больше экрана → по экрану');
  assert.deepStrictEqual(ws({ width: 300, height: 200 }, wa), { width: 760, height: 480, maximized: false }, 'меньше минимума → минимум');
  assert.deepStrictEqual(ws({ width: 'x', height: -5, maximized: 'yes' }, wa), { width: 1440, height: 900, maximized: false }, 'мусор → по умолчанию');
  assert.deepStrictEqual(ws({ width: 1000.6, height: 700.2 }, null), { width: 1001, height: 700, maximized: false }, 'нет данных об экране');
  assert.deepStrictEqual(ws({ width: 1440, height: 900 }, { width: 700, height: 400 }), { width: 760, height: 480, maximized: false }, 'экран меньше минимума → минимум');
}

// ---- gpuStatus: GPU-ускорение для «О программе» ----
{
  const g = extract('gpuStatus');
  const DAY = 86400000;
  const base = { noGpuRequested: false, disabledByMarker: false, hasRenderNode: true, hasEgl: true, markerAgeMs: null, ttlMs: 30 * DAY };
  assert.deepStrictEqual(g(base), { on: true, reason: null, marker: false, daysLeft: 0 }, 'включено');
  assert.deepStrictEqual(g({ ...base, disabledByMarker: true, markerAgeMs: 2.5 * DAY }), { on: false, reason: 'marker', marker: true, daysLeft: 28 }, 'маркер: ещё 28 дн.');
  assert.deepStrictEqual(g({ ...base, disabledByMarker: true, markerAgeMs: null }), { on: false, reason: 'marker', marker: false, daysLeft: 0 }, 'маркер сняли — до перезапуска');
  assert.deepStrictEqual(g({ ...base, markerAgeMs: 1000 }), { on: true, reason: null, marker: true, daysLeft: 30 }, 'падал в этой сессии — отключится со следующего');
  assert.strictEqual(g({ ...base, markerAgeMs: 31 * DAY }).marker, false, 'маркер старше 30 дней не действует');
  assert.strictEqual(g({ ...base, noGpuRequested: true, disabledByMarker: true }).reason, 'env', 'переменная окружения важнее маркера');
  assert.strictEqual(g({ ...base, hasRenderNode: false }).reason, 'norender', 'нет render-узлов');
  assert.strictEqual(g({ ...base, hasEgl: false }).reason, 'noegl', 'нет EGL');
}

// ---- орфография в контекстном меню ----
{
  const word = { isEditable: true, editFlags: all, misspelledWord: 'превет', dictionarySuggestions: ['привет', 'прелесть', 'a', 'b', 'c', 'd'] };
  assert.deepStrictEqual(acts(ctx(word)),
    ['replace', 'replace', 'replace', 'replace', 'replace', 'addWord', '|', 'cut', 'copy', 'paste', '|', 'selectAll'],
    'слово с ошибкой: до 5 вариантов и «Добавить в словарь» первой группой');
  assert.deepStrictEqual(acts(ctx({ ...word, dictionarySuggestions: [] })).slice(0, 2), ['none-', 'addWord'], 'вариантов нет — неактивный пункт');
  const env = fakeEnv();
  const replaced = []; const added = [];
  env.win.webContents.replaceMisspelling = (w) => replaced.push(w);
  env.win.webContents.session = { addWordToSpellCheckerDictionary: (w) => added.push(w) };
  env.show(word);
  byLabel(env, 'привет').click();
  byLabel(env, 'Добавить в словарь').click();
  assert.deepStrictEqual([replaced, added], [['привет'], ['превет']], 'замена и добавление в словарь');
}

console.log('OK: contextMenuItems, showContextMenu, appMenuTemplate, keyAction, windowSize, gpuStatus — все проверки прошли');
