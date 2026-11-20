'use strict';
// «Горячие клавиши…» (Keyboard shortcuts…): the hotkeys table (hotkeyRows in
// the pure src/ui.js — all 9 keys, RU and EN) and the dialog itself
// (showHotkeysDialog, extracted from src/dialogs.js with stand-ins for the
// dialog infrastructure).
// Run: node test/hotkeys-dialog.js   (npm test)
const assert = require('assert');
const { extract } = require('./_extract');
const ui = require('../src/ui');
const i18n = require('../src/i18n');

// the keys from the README (section «Клавиши в окне») — every one must be in the table
const KEYS = ['F5', 'Ctrl+R', 'Ctrl+Shift+R', 'Ctrl+=', 'Ctrl+-', 'Ctrl+0',
  'Ctrl+F', 'F3', 'Shift+F3', 'F11', 'Ctrl+,', 'Esc'];

// call fn with the i18n language set and restored afterwards
const inLang = (lang, fn) => (...a) => {
  const was = i18n.getLang();
  i18n.setLang(lang);
  try { return fn(...a); } finally { i18n.setLang(was); }
};

async function main() {
  // ---- hotkeyRows: the table data (pure, src/ui.js) ----
  for (const lang of ['ru', 'en']) {
    const rows = inLang(lang, ui.hotkeyRows)();
    assert.strictEqual(rows.length, 9, lang + ': 9 rows');
    for (const r of rows) assert.ok(Array.isArray(r) && r.length === 2, lang + ': a row is [keys, action]');
    const keys = rows.map(([k]) => k).join(' ');
    for (const k of KEYS) assert.ok(keys.includes('<code>' + k + '</code>'), lang + ': the key ' + k + ' is in the table');
  }
  assert.strictEqual(inLang('ru', ui.hotkeyRows)()[0][1], 'перезагрузить страницу', 'RU: the first action');
  assert.strictEqual(inLang('ru', ui.hotkeyRows)()[7][1], '«Настройки»', 'RU: «Настройки»');
  assert.strictEqual(inLang('en', ui.hotkeyRows)()[0][1], 'reload the page', 'EN: the first action');
  assert.strictEqual(inLang('en', ui.hotkeyRows)()[5][1], 'next / previous match', 'EN: F3 / Shift+F3');
  assert.ok(inLang('en', ui.hotkeyRows)()[8][1].includes('close dialog or find'), 'EN: Esc — close dialog or find');

  // ---- showHotkeysDialog: the dialog content and the dialog calls ----
  const mk = (lang, { open = 1 } = {}) => {
    const calls = [];
    const fn = extract('showHotkeysDialog', {
      tr: (r, e) => (lang === 'ru' ? r : e),
      openDialog: (title) => { calls.push(['open', title]); return open; },
      dialogShow: (gen, render) => { calls.push(['show', gen]); calls.render = render; },
      dialogPage: (title, body) => calls.push(['page', title, body]),
      dialogCloseButton: () => '<button>close</button>',
      hotkeyRows: () => ui.hotkeyRows(),
      esc: (s) => String(s),
    });
    return { calls, fn };
  };

  for (const [lang, title, note] of [
    ['ru', 'Горячие клавиши', 'любой раскладке (RU/EN)'],
    ['en', 'Keyboard shortcuts', 'any keyboard layout (RU/EN)']]) {
    const { calls, fn } = mk(lang);
    fn();
    assert.deepStrictEqual(calls[0], ['open', title], lang + ': openDialog with the title');
    assert.deepStrictEqual(calls[1], ['show', 1], lang + ': dialogShow with the generation');
    calls.render();
    assert.strictEqual(calls[2][0], 'page', lang + ': dialogPage');
    assert.strictEqual(calls[2][1], title, lang + ': the page title');
    const body = calls[2][2];
    for (const k of KEYS) assert.ok(body.includes('<code>' + k + '</code>'), lang + ': the key ' + k + ' in the body');
    assert.ok(body.includes(note), lang + ': the note about the layouts');
    assert.ok(body.includes('меню окна') || body.includes('window menu'), lang + ': the note mentions the menu');
    assert.ok(body.includes('<table'), lang + ': the table');
    assert.ok(body.includes('<button>close</button>'), lang + ': the close button');
  }

  // The window is not ready (openDialog → null) — the dialog is not shown
  {
    const { calls, fn } = mk('en', { open: null });
    fn();
    assert.deepStrictEqual(calls, [['open', 'Keyboard shortcuts']], 'openDialog is null — nothing more is called');
  }

  console.log('OK: hotkeys dialog — hotkeyRows (9 keys, RU/EN) and showHotkeysDialog');
}

main().catch((e) => { console.error(e); process.exit(1); });
