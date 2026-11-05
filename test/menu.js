#!/usr/bin/env node
// Unit tests for the menu (main.js): contextMenuItems — which context
// menu items to show, showContextMenu — what each item does (on a fake
// window, Menu and clipboard), appMenuTemplate — the window menu bar,
// launcherVersionRaw — launcher version (dpkg / root package.json).
// main.js is not loaded in full (electron).
'use strict';
const assert = require('assert');
const path = require('path');
const { extract } = require('./_extract');

// ---- contextMenuItems: context menu items on right click ----
const i18n = require('../src/i18n');
const ui = require('../src/ui');
// contextMenuItems in the given language (language is shared, src/i18n.js)
const inLang = (lang, fn) => (...a) => { const was = i18n.getLang(); i18n.setLang(lang); try { return fn(...a); } finally { i18n.setLang(was); } };
const ctx = inLang('ru', ui.contextMenuItems);
const acts = (items) => items.map((i) => (i.type ? '|' : i.action + (i.enabled ? '' : '-')));
const all = { canCut: true, canCopy: true, canPaste: true, canSelectAll: true };

assert.deepStrictEqual(acts(ctx({ isEditable: true, editFlags: all, selectionText: 'x' })),
  ['cut', 'copy', 'paste', '|', 'selectAll'], 'input field with a selection — everything is available');
assert.deepStrictEqual(acts(ctx({ isEditable: true, editFlags: { canPaste: true, canSelectAll: true }, selectionText: '' })),
  ['cut-', 'copy-', 'paste', '|', 'selectAll'], 'field without a selection — cut/copy are unavailable');
assert.deepStrictEqual(acts(ctx({ isEditable: false, editFlags: {}, selectionText: 'текст' })),
  ['copy', '|', 'selectAll'], 'text selected on the page — copy');
assert.deepStrictEqual(acts(ctx({ isEditable: false, editFlags: {}, selectionText: '  \n' })),
  ['selectAll'], 'only whitespace selected — no «Копировать»');
assert.deepStrictEqual(acts(ctx({ isEditable: false, editFlags: {}, selectionText: '', linkURL: 'https://x/' })),
  ['copyLink', '|', 'selectAll'], 'a link — copy its address');
assert.deepStrictEqual(acts(ctx({ isEditable: false, selectionText: 'a', linkURL: 'https://x/' })),
  ['copyLink', '|', 'copy', '|', 'selectAll'], 'a selected link; editFlags may be absent');
assert.strictEqual(ctx({ isEditable: true, editFlags: all }).find((i) => i.action === 'paste').label, 'Вставить', 'labels via tr');

// English labels
const ctxEn = inLang('en', ui.contextMenuItems);
assert.deepStrictEqual(
  ctxEn({ isEditable: true, editFlags: all, linkURL: 'https://x/' }).filter((i) => !i.type).map((i) => i.label),
  ['Copy link address', 'Cut', 'Copy', 'Paste', 'Select all'], 'EN labels');

// All combinations: separators only between groups (not at the start,
// not at the end, not two in a row), the menu is never empty, items have a label.
for (const isEditable of [true, false]) {
  for (const linkURL of ['', 'https://x/']) {
    for (const selectionText of ['', ' ', 'a']) {
      for (const editFlags of [undefined, {}, all]) {
        const items = ctx({ isEditable, linkURL, selectionText, editFlags });
        const where = JSON.stringify({ isEditable, linkURL, selectionText, editFlags });
        assert.ok(items.length > 0, 'the menu is not empty: ' + where);
        assert.ok(!items[0].type && !items[items.length - 1].type, 'no separator at the edges: ' + where);
        items.forEach((it, k) => {
          if (it.type) assert.ok(!items[k + 1].type, 'no two separators in a row: ' + where);
          else assert.ok(it.label && it.action && typeof it.enabled === 'boolean', 'the item is complete: ' + where);
        });
      }
    }
  }
}

// ---- showContextMenu: what each item does and where the menu is shown ----
function fakeEnv({ destroyed = false } = {}) {
  const calls = [];
  const wc = {};
  for (const m of ['cut', 'copy', 'paste', 'selectAll']) wc[m] = () => calls.push('wc.' + m);
  wc.isDestroyed = () => destroyed;
  const win = { webContents: wc, isDestroyed: () => destroyed };
  const env = { calls, win, template: null, popupOpts: null };
  const Menu = {
    buildFromTemplate: (t) => { env.template = t; return { once: () => {}, popup: (o) => { env.popupOpts = o; calls.push('popup'); } }; },
  };
  const clipboard = { writeText: (t) => calls.push('clipboard:' + t) };
  env.logs = [];
  env.show = extract('showContextMenu', { state: { win }, Menu, clipboard, contextMenuItems: ctx, console: { log: (m) => env.logs.push(m) }, openContextMenu: null });
  return env;
}
const byLabel = (env, label) => env.template.find((i) => i.label === label);

{
  // Input field: each item calls its own webContents method, availability is passed through
  const env = fakeEnv();
  env.show({ isEditable: true, editFlags: { canPaste: true, canSelectAll: true }, selectionText: '' });
  assert.strictEqual(env.popupOpts.window, env.win, 'the menu is shown in the launcher window');
  assert.deepStrictEqual(env.template.map((i) => i.type || i.label),
    ['Вырезать', 'Копировать', 'Вставить', 'separator', 'Выделить всё'], 'the menu template');
  assert.strictEqual(byLabel(env, 'Вырезать').enabled, false, 'no selection — «Вырезать» is disabled');
  assert.strictEqual(byLabel(env, 'Вставить').enabled, true, '«Вставить» is enabled');
  assert.deepStrictEqual(env.logs, ['[launcher] контекстное меню: Вырезать (off), Копировать (off), Вставить, |, Выделить всё'], 'the log line');
  for (const [label, call] of [['Вырезать', 'wc.cut'], ['Копировать', 'wc.copy'], ['Вставить', 'wc.paste'], ['Выделить всё', 'wc.selectAll']]) {
    env.calls.length = 0;
    byLabel(env, label).click();
    assert.deepStrictEqual(env.calls, [call], `«${label}» → ${call}`);
  }
}
{
  // Link: the address goes to the clipboard, not through webContents
  const env = fakeEnv();
  env.show({ isEditable: false, linkURL: 'https://example.com/a?b=1', selectionText: '' });
  env.calls.length = 0;
  byLabel(env, 'Копировать адрес ссылки').click();
  assert.deepStrictEqual(env.calls, ['clipboard:https://example.com/a?b=1'], 'the link address goes to the clipboard');
}
{
  // "About" dialog — a layer of the main window: the menu is over the main window,
  // the actions — in the layer's content
  const env = fakeEnv();
  const calls2 = [];
  const dlgWc = { isDestroyed: () => false, cut() {}, copy: () => calls2.push('dlg.copy'), paste() {}, selectAll() {} };
  env.show({ isEditable: false, selectionText: 'v1.4.0' }, env.win, dlgWc);
  assert.strictEqual(env.popupOpts.window, env.win, 'the menu is shown over the main window');
  byLabel(env, 'Копировать').click();
  assert.deepStrictEqual(calls2, ['dlg.copy'], '«Копировать» — in the dialog layer');
  assert.deepStrictEqual(env.calls.filter((c) => c.startsWith('wc.')), [], 'the main window is untouched');
}
{
  // Window closed — the menu is neither built nor shown
  const env = fakeEnv({ destroyed: true });
  env.show({ isEditable: true, editFlags: all });
  assert.strictEqual(env.template, null, 'a closed window — no menu');
  assert.deepStrictEqual(env.calls, [], 'a closed window — nothing is called');
}

// The right-click handler is wired to the window
const { mainSrc } = require('./_extract');
assert.ok(/win\.webContents\.on\('context-menu',\s*\(_e, params\) => showContextMenu\(params\)\)/.test(mainSrc),
  'main.js: context-menu → showContextMenu');

// ---- appMenuTemplate: the window menu bar ----
{
  const calls = [];
  const viewActions = new Proxy({}, { get: (_t, k) => () => calls.push('view.' + String(k)) });
  const mk = (tr, spellcheckOn = true, availableUpdates = {}) => extract('appMenuTemplate', {
    tr,
    state: { availableUpdates, spellcheckOn },
    // modules the menu calls into (stubs)
    updatesLive: { showUpdateNowDialog: (kind) => calls.push('update:' + kind) },
    view: { viewActions, setSpellcheck: (on) => calls.push('spell:' + on) },
    dialogs: { showAboutDialog: () => calls.push('about'), showUpdateCheckDialog: () => calls.push('check') },
    rollback: { showRollbackDialog: () => calls.push('rollback') },
    fileHelp: { openLogsFolder: () => calls.push('logs'), restartDsh: () => calls.push('restart') },
    diagnostics: { saveReport: () => calls.push('report') },
    dshLog: { showLogDialog: () => calls.push('log') },
    settingsWindow: { showSettingsDialog: () => calls.push('settings') },
  })();
  const ru = mk((r) => r);
  const en = mk((r, e) => e);
  assert.deepStrictEqual(ru.map((m) => m.label), ['Файл', 'Правка', 'Вид', 'Справка'], 'the menu bar items (RU)');
  assert.deepStrictEqual(en.map((m) => m.label), ['File', 'Edit', 'View', 'Help'], 'the menu bar items (EN)');
  const item = (menu, label) => menu.submenu.find((i) => i.label === label);
  const labels = (menu) => menu.submenu.map((i) => (i.type === 'separator' ? '|' : i.label));

  // "File": restarting dsh and quitting
  assert.deepStrictEqual(labels(ru[0]), ['Настройки…', '|', 'Перезапустить dsh…', '|', 'Выйти'], '«Файл»');
  calls.length = 0;
  item(ru[0], 'Настройки…').click();
  assert.deepStrictEqual(calls, ['settings'], '«Настройки…» → the settings window');
  assert.strictEqual(item(ru[0], 'Настройки…').accelerator, 'CmdOrCtrl+,', '«Настройки…» — the Ctrl+, hint');
  calls.length = 0;
  item(ru[0], 'Перезапустить dsh…').click();
  assert.deepStrictEqual(calls, ['restart'], '«Перезапустить dsh…» → restart');
  assert.strictEqual(item(ru[0], 'Выйти').role, 'quit', '«Выйти» — the standard quit role');

  // "Help": updates, logs, "About" — as the last item
  assert.deepStrictEqual(labels(ru[3]), ['Проверить обновления…', 'Вернуть предыдущую версию…', 'Журнал dsh…', 'Открыть папку логов', 'Сохранить отчёт…', '|', 'О программе'], '«Справка»');
  assert.deepStrictEqual(labels(en[3]), ['Check for updates…', 'Roll back to a previous version…', 'dsh log…', 'Open logs folder', 'Save report…', '|', 'About'], '«Help»');
  for (const [label, call] of [['Проверить обновления…', 'check'], ['Вернуть предыдущую версию…', 'rollback'], ['Журнал dsh…', 'log'], ['Открыть папку логов', 'logs'], ['Сохранить отчёт…', 'report'], ['О программе', 'about']]) {
    calls.length = 0;
    item(ru[3], label).click();
    assert.deepStrictEqual(calls, [call], `«${label}» → ${call}`);
  }

  // updates found — first items of "Help"
  {
    const upd = mk((r) => r, true, { launcher: { version: '1.8.0' }, dsh: { version: '0.3.0' } });
    assert.deepStrictEqual(labels(upd[3]).slice(0, 4),
      ['Обновить DeepSeek Harness Launcher до 1.8.0…', 'Обновить dsh до 0.3.0…', '|', 'Проверить обновления…'], '«Справка» with the updates');
    calls.length = 0;
    item(upd[3], 'Обновить DeepSeek Harness Launcher до 1.8.0…').click();
    item(upd[3], 'Обновить dsh до 0.3.0…').click();
    assert.deepStrictEqual(calls, ['update:launcher', 'update:dsh'], 'the item → the «Обновить сейчас» dialog');
    const onlyDsh = mk((r, e) => e, true, { dsh: { version: '0.3.0' } });
    assert.deepStrictEqual(labels(onlyDsh[3]).slice(0, 3), ['Update dsh to 0.3.0…', '|', 'Check for updates…'], '«Help» — dsh only');
  }

  // "Edit" — the same actions as in the context menu, via standard roles
  const edit = (t) => t[1].submenu.map((i) => (i.type ? '|' : `${i.label}:${i.role}:${i.accelerator}`));
  assert.deepStrictEqual(edit(ru),
    ['Вырезать:cut:CmdOrCtrl+X', 'Копировать:copy:CmdOrCtrl+C', 'Вставить:paste:CmdOrCtrl+V', '|', 'Выделить всё:selectAll:CmdOrCtrl+A'],
    '«Правка» (RU)');
  assert.deepStrictEqual(edit(en),
    ['Cut:cut:CmdOrCtrl+X', 'Copy:copy:CmdOrCtrl+C', 'Paste:paste:CmdOrCtrl+V', '|', 'Select all:selectAll:CmdOrCtrl+A'],
    '«Правка» (EN)');
  const ctxLabels = ctx({ isEditable: true, editFlags: all }).filter((i) => !i.type).map((i) => i.label);
  assert.deepStrictEqual(ru[1].submenu.filter((i) => !i.type).map((i) => i.label), ctxLabels, 'the «Правка» labels match the context menu');

  // "View" — action and accelerator hint on each item
  for (const [label, act, key] of [
    ['Перезагрузить', 'reload', 'F5'], ['Перезагрузить без кэша', 'reloadHard', 'CmdOrCtrl+Shift+R'],
    ['Увеличить', 'zoomIn', 'CmdOrCtrl+='], ['Уменьшить', 'zoomOut', 'CmdOrCtrl+-'], ['Обычный масштаб', 'zoomReset', 'CmdOrCtrl+0'],
    ['Найти на странице…', 'find', 'CmdOrCtrl+F'], ['Полноэкранный режим', 'fullscreen', 'F11']]) {
    calls.length = 0;
    const it = item(ru[2], label);
    assert.ok(it, `«Вид»: «${label}» is present`);
    it.click();
    assert.deepStrictEqual(calls, ['view.' + act], `«${label}» → ${act}`);
    assert.strictEqual(it.accelerator, key, `«${label}»: the ${key} hint`);
  }
  const spell = item(ru[2], 'Проверка орфографии');
  assert.strictEqual(spell.type, 'checkbox', 'spellcheck — a checkbox');
  assert.strictEqual(spell.checked, true, 'the checkbox reflects the state (on)');
  assert.strictEqual(item(mk((r) => r, false)[2], 'Проверка орфографии').checked, false, 'the checkbox reflects the state (off)');
  calls.length = 0;
  spell.click({ checked: false });
  assert.deepStrictEqual(calls, ['spell:false'], 'unchecking → off');

  // all shortcuts are hints only: don't intercept dsh GUI hotkeys
  for (const m of ru) for (const i of m.submenu.filter((x) => x.accelerator)) {
    assert.strictEqual(i.registerAccelerator, false, `«${i.label}»: registerAccelerator: false`);
  }
}

// ---- keyAction: key → "View" action ----
{
  const ka = ui.keyAction;
  const k = (o) => ka({ type: 'keyDown', control: false, shift: false, key: '', code: '', ...o });
  assert.strictEqual(k({ key: 'F5' }), 'reload', 'F5');
  assert.strictEqual(k({ control: true, code: 'KeyR', key: 'к' }), 'reload', 'Ctrl+R on the Russian layout');
  assert.strictEqual(k({ control: true, shift: true, code: 'KeyR' }), 'reloadHard', 'Ctrl+Shift+R');
  assert.strictEqual(k({ control: true, code: 'Equal' }), 'zoomIn', 'Ctrl+=');
  assert.strictEqual(k({ control: true, code: 'NumpadAdd' }), 'zoomIn', 'Ctrl+Num+');
  assert.strictEqual(k({ control: true, code: 'Minus' }), 'zoomOut', 'Ctrl+-');
  assert.strictEqual(k({ control: true, code: 'Digit0' }), 'zoomReset', 'Ctrl+0');
  assert.strictEqual(k({ control: true, code: 'KeyF', key: 'а' }), 'find', 'Ctrl+F on the Russian layout');
  assert.strictEqual(k({ control: true, shift: true, code: 'KeyF' }), null, 'Ctrl+Shift+F — to the page');
  assert.strictEqual(k({ key: 'F3' }), 'findNext', 'F3');
  assert.strictEqual(k({ key: 'F3', shift: true }), 'findPrev', 'Shift+F3');
  assert.strictEqual(k({ key: 'F11' }), 'fullscreen', 'F11');
  assert.strictEqual(k({ control: true, code: 'Comma', key: 'б' }), 'settings', 'Ctrl+, (on the Russian layout — «б»)');
  assert.strictEqual(k({ key: 'f', code: 'KeyF' }), null, 'a plain F — to the page');
  assert.strictEqual(k({ control: true, code: 'KeyC' }), null, 'Ctrl+C — to the page (copy)');
}

// ---- isAltKey: Alt keys we swallow so the menu doesn't steal focus ----
{
  const ia = ui.isAltKey;
  assert.strictEqual(ia({ type: 'keyDown', key: 'Alt', code: 'AltLeft', alt: true }), true, 'a bare Alt (press)');
  assert.strictEqual(ia({ type: 'keyUp', key: 'Alt', code: 'AltLeft', alt: false }), true, 'a bare Alt (release, alt already false)');
  assert.strictEqual(ia({ type: 'keyDown', key: 'Alt', code: 'AltRight', alt: true }), true, 'the right Alt');
  assert.strictEqual(ia({ type: 'keyDown', key: 'GroupNext', code: 'AltLeft', shift: true, alt: true }), true, 'Shift+Alt — layout switch (key="GroupNext")');
  assert.strictEqual(ia({ type: 'keyUp', key: 'GroupNext', code: 'AltLeft', shift: true }), true, 'Shift+Alt (release)');
  assert.strictEqual(ia({ type: 'keyDown', key: 'a', code: 'KeyA', alt: true }), false, 'other keys with Alt — to the page (the altKey modifier)');
  assert.strictEqual(ia({ type: 'keyUp', key: 'Shift', code: 'ShiftLeft', shift: true }), false, 'other keys');
}

// ---- windowSize: window size from saved state ----
{
  const ws = ui.windowSize;
  const wa = { width: 1920, height: 1050 };
  assert.deepStrictEqual(ws(null, wa), { width: 1440, height: 900, maximized: false }, 'no state → the defaults');
  assert.deepStrictEqual(ws({ width: 1200, height: 800, maximized: true }, wa), { width: 1200, height: 800, maximized: true }, 'the saved state');
  assert.deepStrictEqual(ws({ width: 3000, height: 2000 }, wa), { width: 1920, height: 1050, maximized: false }, 'bigger than the screen → the screen size');
  assert.deepStrictEqual(ws({ width: 300, height: 200 }, wa), { width: 760, height: 480, maximized: false }, 'smaller than the minimum → the minimum');
  assert.deepStrictEqual(ws({ width: 'x', height: -5, maximized: 'yes' }, wa), { width: 1440, height: 900, maximized: false }, 'garbage → the defaults');
  assert.deepStrictEqual(ws({ width: 1000.6, height: 700.2 }, null), { width: 1001, height: 700, maximized: false }, 'no screen data');
  assert.deepStrictEqual(ws({ width: 1440, height: 900 }, { width: 700, height: 400 }), { width: 760, height: 480, maximized: false }, 'the screen is smaller than the minimum → the minimum');
}

// ---- gpuStatus: GPU acceleration for "About" ----
{
  const g = ui.gpuStatus;
  const DAY = 86400000;
  const base = { noGpuRequested: false, disabledByMarker: false, hasRenderNode: true, hasEgl: true, markerAgeMs: null, ttlMs: 30 * DAY };
  assert.deepStrictEqual(g(base), { on: true, reason: null, marker: false, daysLeft: 0 }, 'on');
  assert.deepStrictEqual(g({ ...base, disabledByMarker: true, markerAgeMs: 2.5 * DAY }), { on: false, reason: 'marker', marker: true, daysLeft: 28 }, 'marker: 28 days left');
  assert.deepStrictEqual(g({ ...base, disabledByMarker: true, markerAgeMs: null }), { on: false, reason: 'marker', marker: false, daysLeft: 0 }, 'the marker was removed — until the restart');
  assert.deepStrictEqual(g({ ...base, markerAgeMs: 1000 }), { on: true, reason: null, marker: true, daysLeft: 30 }, 'it crashed in this session — it will be off from the next launch');
  assert.strictEqual(g({ ...base, markerAgeMs: 31 * DAY }).marker, false, 'a marker older than 30 days does not apply');
  assert.strictEqual(g({ ...base, noGpuRequested: true, disabledByMarker: true }).reason, 'env', 'the environment variable outranks the marker');
  assert.strictEqual(g({ ...base, noGpuRequested: true, noGpuBySettings: true }).reason, 'setting', 'disabled in «Настройки»');
  assert.strictEqual(g({ ...base, hasRenderNode: false }).reason, 'norender', 'no render nodes');
  assert.strictEqual(g({ ...base, hasEgl: false }).reason, 'noegl', 'no EGL');
}

// ---- spellcheck in the context menu ----
{
  const word = { isEditable: true, editFlags: all, misspelledWord: 'превет', dictionarySuggestions: ['привет', 'прелесть', 'a', 'b', 'c', 'd'] };
  assert.deepStrictEqual(acts(ctx(word)),
    ['replace', 'replace', 'replace', 'replace', 'replace', 'addWord', '|', 'cut', 'copy', 'paste', '|', 'selectAll'],
    'a misspelled word: up to 5 suggestions and «Добавить в словарь» as the first group');
  assert.deepStrictEqual(acts(ctx({ ...word, dictionarySuggestions: [] })).slice(0, 2), ['none-', 'addWord'], 'no suggestions — an inactive item');
  assert.deepStrictEqual(acts(ctx({ ...word, dictionarySuggestions: undefined })).slice(0, 2), ['none-', 'addWord'], 'no suggestion list at all — the same');
  const env = fakeEnv();
  const replaced = []; const added = [];
  env.win.webContents.replaceMisspelling = (w) => replaced.push(w);
  env.win.webContents.session = { addWordToSpellCheckerDictionary: (w) => added.push(w) };
  env.show(word);
  byLabel(env, 'привет').click();
  byLabel(env, 'Добавить в словарь').click();
  assert.deepStrictEqual([replaced, added], [['привет'], ['превет']], 'the replacement and the dictionary addition');
}

// ---- launcherVersionRaw: launcher version (regression test: the source
// package.json is the root one, not src/ — otherwise "About" shows "unknown") ----
// updates/require/IS_PKG_INSTALL are fakes; require resolves relative
// paths from src/ (the module's real directory).
{
  const mk = (pkg, requireImpl) => extract('launcherVersionRaw', {
    updates: { launcherInstalledVersion: async () => '9.9.9' },
    IS_PKG_INSTALL: pkg,
    require: requireImpl,
  })();
  const realRequire = (spec) => require(path.resolve(__dirname, '..', 'src', spec));
  (async () => {
    assert.strictEqual(await mk(true, () => { throw new Error('не должно открываться'); }),
      '9.9.9', 'deb: the version from dpkg, package.json is not opened');
    assert.strictEqual(await mk(false, realRequire),
      require('../package.json').version, 'the sources: the root package.json');
    assert.strictEqual(await mk(false, () => { throw new Error('MODULE_NOT_FOUND'); }),
      null, 'package.json is unavailable — null');
    console.log('OK: contextMenuItems, showContextMenu, appMenuTemplate, keyAction, windowSize, gpuStatus, launcherVersionRaw — all checks passed');
  })();
}
