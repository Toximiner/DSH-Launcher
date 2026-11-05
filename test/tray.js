#!/usr/bin/env node
// Юнит-тесты src/tray.js: статус dsh у значка трея — trayStatusText
// (подсказка), trayStatusLabel (строка для лога) и refreshTrayStatus (кэш).
'use strict';
const assert = require('assert');
const { extract } = require('./_extract');

const ru = (r, e) => r;
const en = (r, e) => e;

const trayStatusText = extract('trayStatusText', {});

// подсказка (ru)
assert.strictEqual(trayStatusText('up', false, ru), 'DeepSeek Harness — dsh запущен', 'port up');
assert.strictEqual(trayStatusText('down', false, ru), 'DeepSeek Harness — dsh не запущен', 'port down');
assert.strictEqual(trayStatusText('down', true, ru), 'DeepSeek Harness — dsh остановлен (авто-перезапуск остановлен)', 'fatal');
assert.strictEqual(trayStatusText('up', true, ru), 'DeepSeek Harness — dsh остановлен (авто-перезапуск остановлен)', 'fatal важнее port');
assert.strictEqual(trayStatusText(null, false, ru), 'DeepSeek Harness', 'ещё не проверено — только имя');
// подсказка (en)
assert.strictEqual(trayStatusText('up', false, en), 'DeepSeek Harness — dsh is running', 'port up (en)');
assert.strictEqual(trayStatusText('down', true, en), 'DeepSeek Harness — dsh has stopped (auto-restart halted)', 'fatal (en)');

const trayStatusLabel = extract('trayStatusLabel', {});
assert.strictEqual(trayStatusLabel('up', false), 'dsh запущен', 'label up');
assert.strictEqual(trayStatusLabel('down', false), 'dsh не запущен', 'label down');
assert.strictEqual(trayStatusLabel(null, true), 'dsh остановлен (авто-перезапуск остановлен)', 'label fatal');
assert.strictEqual(trayStatusLabel(null, false), 'состояние dsh ещё не проверено', 'label null');

// refreshTrayStatus: в Electron лезём только при смене состояния (кэш
// trayStatusCache — параметр замыкания), при attention — вообще не лезем.
function mk(tray, attention) {
  const tools = {
    tooltips: [],
    logs: [],
    state: { tray, portState: null, dshFatal: false },
  };
  if (tray) tray.setToolTip = (x) => tools.tooltips.push(x);
  tools.run = extract('refreshTrayStatus', {
    state: tools.state,
    trayStatusCache: null,
    trayAttention: attention,
    tr: ru,
    console: { log: (l) => tools.logs.push(l) },
    trayStatusText,
    trayStatusLabel,
  });
  return tools;
}
{
  const t = mk(null, false);
  t.run();
  assert.strictEqual(t.tooltips.length, 0, 'трея нет — ничего не делаем');
}
{
  const t = mk({}, false);
  t.run();
  assert.strictEqual(t.tooltips.length, 1, 'включили трей — показали статус');
  assert.strictEqual(t.tooltips[0], 'DeepSeek Harness');
  t.run();
  assert.strictEqual(t.tooltips.length, 1, 'повторно без изменения — не трогаем (кэш)');
  t.state.portState = 'up';
  t.run();
  assert.strictEqual(t.tooltips.length, 2, 'port up — подсказка обновлена');
  assert.strictEqual(t.tooltips[1], 'DeepSeek Harness — dsh запущен');
  assert.ok(t.logs.some((l) => l.includes('трей: dsh запущен')), 'статус в лог');
  t.state.dshFatal = true;
  t.run();
  assert.strictEqual(t.tooltips.length, 3, 'fatal — подсказка обновлена');
  assert.strictEqual(t.tooltips[2], 'DeepSeek Harness — dsh остановлен (авто-перезапуск остановлен)');
  t.state.portState = 'down';
  t.state.dshFatal = false;
  t.run();
  assert.strictEqual(t.tooltips.length, 4, 'порт закрылся — подсказка обновлена');
  assert.strictEqual(t.tooltips[3], 'DeepSeek Harness — dsh не запущен');
}
{
  const t = mk({}, true);
  t.state.portState = 'up';
  t.run();
  assert.strictEqual(t.tooltips.length, 0, 'attention — статус не перетираем');
  assert.strictEqual(t.logs.length, 0, 'attention — в лог тоже не пишем');
}

console.log('OK: tray — статус dsh у значка');
