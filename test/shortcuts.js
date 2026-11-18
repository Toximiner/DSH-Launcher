'use strict';
const assert = require('node:assert/strict');
const i18n = require('../src/i18n');
const { shortcuts, keyAction, shortcutReference } = require('../src/shortcuts');

for (const s of shortcuts) {
  const event = { shift: s.shift };
  if (s.key) event.key = s.key;
  else { event.control = true; event.code = s.codes[0]; }
  assert.equal(keyAction(event), s.action, `${s.keys} must use the documented action`);
}
assert.equal(keyAction({ control: false, code: 'Comma' }), null);
assert.equal(keyAction({ control: true, shift: true, code: 'KeyF' }), null);
for (const lang of ['ru', 'en']) {
  i18n.setLang(lang);
  const html = shortcutReference();
  assert.equal((html.match(/data-shortcut/g) || []).length, shortcuts.length + 4);
  assert.ok(html.includes('Ctrl+,') && html.includes('aria-label='));
  assert.ok(html.includes(lang === 'ru' ? 'Горяч' : 'Search shortcuts') || html.includes('Поиск горячих клавиш'));
}
i18n.setLang('ru');
