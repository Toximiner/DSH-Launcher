'use strict';
// i18n: tr/getLang/setLang/systemLang. systemLang reads the environment
// directly — for the check we set LC_ALL/LC_MESSAGES/LANG and reset them
// after.
const assert = require('node:assert');
const i18n = require('../src/i18n');

{
  // basics: the language switches, tr picks the right string
  i18n.setLang('ru');
  assert.strictEqual(i18n.getLang(), 'ru');
  assert.strictEqual(i18n.tr('привет', 'hello'), 'привет');
  i18n.setLang('en');
  assert.strictEqual(i18n.tr('привет', 'hello'), 'hello');
  i18n.setLang('de'); // not ru — to en
  assert.strictEqual(i18n.getLang(), 'en');
}

{
  // systemLang: LC_ALL is more important than LANG; ru* (case-insensitive) — ru, the rest — en
  const saved = { LC_ALL: process.env.LC_ALL, LC_MESSAGES: process.env.LC_MESSAGES, LANG: process.env.LANG };
  const run = (env) => {
    for (const k of ['LC_ALL', 'LC_MESSAGES', 'LANG']) delete process.env[k];
    Object.assign(process.env, env);
    return i18n.systemLang();
  };
  try {
    assert.strictEqual(run({ LC_ALL: 'ru_RU.UTF-8', LANG: 'C.UTF-8' }), 'ru', 'LC_ALL ru — ru');
    assert.strictEqual(run({ LANG: 'ru' }), 'ru', 'LANG ru — ru');
    assert.strictEqual(run({ LANG: 'RU_RU' }), 'ru', 'case-insensitive');
    assert.strictEqual(run({ LC_MESSAGES: 'ru_RU', LANG: 'C' }), 'ru', 'LC_MESSAGES ru — ru');
    assert.strictEqual(run({ LANG: 'C.UTF-8' }), 'en', 'C.UTF-8 — en');
    assert.strictEqual(run({ LANG: 'de_DE.UTF-8' }), 'en', 'de — en');
    assert.strictEqual(run({}), 'en', 'no locale — en');
  } finally {
    for (const k of ['LC_ALL', 'LC_MESSAGES', 'LANG']) {
      if (saved[k] === undefined) delete process.env[k]; else process.env[k] = saved[k];
    }
  }
  console.log('OK: i18n — tr, setLang, systemLang');
}
