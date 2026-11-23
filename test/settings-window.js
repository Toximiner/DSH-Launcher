'use strict';
/*
 * test/settings-window.js — applySetting (src/settings-window.js): одно поле
 * «Настроек» → патч в settings.json + немедленные эффекты (tray, polling,
 * spellcheck, страница dsh). Извлечён маркером (test/_extract.js); всё
 * инжектится. Направление «default → null (поле удаляется)» у ключей
 * разнонаправленное — таблица это и проверяет.
 * Run: node test/settings-window.js (or npm test).
 */
const assert = require('assert');
const { extract } = require('./_extract');

const tr = (ru) => ru; // Russian wording only

function makeDeps({ win = null, throwOnSave = false } = {}) {
  const saved = [];
  const calls = { setLanguage: [], setSpellcheck: [], setUpdatePolling: [], setTray: [], playAgentSound: 0, js: [] };
  const state = {
    win: win ? {
      isDestroyed: () => win.destroyed || false,
      webContents: { executeJavaScript: async (js) => { calls.js.push(js); } },
    } : null,
  };
  const deps = {
    state,
    tr,
    setLanguage: (l) => { calls.setLanguage.push(l); },
    view: { setSpellcheck: (v) => { calls.setSpellcheck.push(v); } },
    saveSettings: (patch) => { if (throwOnSave) throw new Error('disk full'); saved.push(patch); },
    DEFAULT_PORT: 3080,
    DEFAULT_PROFILE: 'web',
    updatesLive: { setUpdatePolling: (v) => { calls.setUpdatePolling.push(v); } },
    tray: { setTray: (v) => { calls.setTray.push(v); }, playAgentSound: () => { calls.playAgentSound += 1; } },
    inputHistoryScript: () => 'HISTORY_SCRIPT',
    console: { log: () => { /* noop */ } },
  };
  return { deps, saved, calls };
}

async function main() {
  let m = makeDeps();
  let applySetting = extract('applySetting', m.deps);
  const last = () => m.saved[m.saved.length - 1];

  // Таблица «ключ, значение → патч». default → null (поле из файла удаляется).
  const cases = [
    ['gpu', 'off', { gpu: 'off' }],
    ['gpu', 'auto', { gpu: null }],
    ['ozone', 'wayland', { ozone: 'wayland' }],
    ['ozone', 'auto', { ozone: null }],
    ['port', 3090, { port: 3090 }],
    ['port', 3080, { port: null }],
    ['profile', 'api', { profile: 'api' }],
    ['profile', 'web', { profile: null }],
    ['checkUpdates', false, { checkUpdates: false }],
    ['checkUpdates', true, { checkUpdates: null }],
    ['closeToTray', true, { closeToTray: true }],
    ['closeToTray', false, { closeToTray: null }],
    ['attention', true, { attention: null }],
    ['attention', false, { attention: false }],
    ['sandbox', true, { sandbox: true }],
    ['sandbox', false, { sandbox: null }],
    ['attentionSound', false, { attentionSound: null }],
    ['inputHistory', true, { inputHistory: null }],
    ['inputHistory', false, { inputHistory: false }],
    ['dshPath', '/usr/bin/dsh', { dshPath: '/usr/bin/dsh' }],
    ['dshPath', '', { dshPath: null }],
    ['cwd', '/home/u', { cwd: '/home/u' }],
    ['cwd', '', { cwd: null }],
  ];
  for (const [key, value, patch] of cases) {
    const note = applySetting(key, value);
    assert.deepStrictEqual(note, { text: 'Сохранено', error: false }, `${key}=${value} — note`);
    assert.deepStrictEqual(last(), patch, `${key}=${value} — патч в settings.json`);
  }

  // Немедленные эффекты без записи в файл: lang, spellcheck.
  assert.strictEqual(m.calls.setLanguage.length, 0, 'lang ещё не трогали');
  assert.strictEqual(applySetting('lang', 'ru').error, false, 'lang — сохранено');
  assert.deepStrictEqual(m.calls.setLanguage, ['ru'], 'lang → setLanguage');
  assert.strictEqual(applySetting('spellcheck', true).error, false, 'spellcheck — сохранено');
  assert.deepStrictEqual(m.calls.setSpellcheck, [true], 'spellcheck → view.setSpellcheck');

  // pollUpdates: запись + живой переключатель поллинга.
  applySetting('pollUpdates', false);
  assert.deepStrictEqual(last(), { pollUpdates: false });
  assert.deepStrictEqual(m.calls.setUpdatePolling, [false], 'polling off включён сразу');
  applySetting('pollUpdates', true);
  assert.deepStrictEqual(last(), { pollUpdates: null });
  assert.deepStrictEqual(m.calls.setUpdatePolling, [false, true], 'polling on включён сразу');

  // closeToTray: запись + tray сразу (в таблице closeToTray уже был — смотрим на рост).
  let trayBefore = m.calls.setTray.length;
  applySetting('closeToTray', true);
  assert.deepStrictEqual(m.calls.setTray.slice(trayBefore), [true], 'tray — сразу');

  // attentionSound: звук — только если окно живо.
  let soundBefore = m.calls.playAgentSound;
  applySetting('attentionSound', true);
  assert.strictEqual(m.calls.playAgentSound, soundBefore, 'без окна — звука нет');
  m = makeDeps({ win: {} });
  applySetting = extract('applySetting', m.deps);
  applySetting('attentionSound', true);
  assert.strictEqual(m.calls.playAgentSound, 1, 'окно живо — звук сразу');
  m.saved.length = 0;
  applySetting('attentionSound', false);
  assert.strictEqual(m.calls.playAgentSound, 1, 'выключили — звука нет');

  // inputHistory: инъекция в открытую страницу dsh.
  applySetting('inputHistory', true);
  assert.deepStrictEqual(m.calls.js, ['HISTORY_SCRIPT'], 'включение — скрипт в страницу');
  applySetting('inputHistory', false);
  assert.strictEqual(m.calls.js.length, 2, 'выключение — тоже инъекция');
  assert.ok(/setEnabled\(false\)/.test(m.calls.js[1]), 'выключение — setEnabled(false)');
  m = makeDeps(); // без окна
  applySetting = extract('applySetting', m.deps);
  applySetting('inputHistory', true);
  assert.deepStrictEqual(m.saved, [{ inputHistory: null }], 'без окна — только запись');
  assert.strictEqual(m.calls.js.length, 0, 'без окна — инъекции нет');

  // Ошибка записи — заметка об ошибке, не исключение наружу.
  m = makeDeps({ throwOnSave: true });
  applySetting = extract('applySetting', m.deps);
  assert.deepStrictEqual(applySetting('port', 3090), { text: 'disk full', error: true }, 'ошибка → заметка');
}

main().catch((e) => { console.error('FAIL:', e.message); process.exit(1); });
