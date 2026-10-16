'use strict';
// Юнит-тест истории введённого (src/input-history.js): чистая логика ↑/↓
// (historyPush/historyStep/historyExit) и текст скрипта впрыска.
const assert = require('assert');
const { extract } = require('./_extract');

const historyPush = extract('historyPush', {});
const historyStep = extract('historyStep', {});
const historyExit = extract('historyExit', {});
const buildInputHistoryScript = extract('buildInputHistoryScript', {});

// ---- historyPush: свежие сверху, повторы подряд не дублируются ----
assert.deepStrictEqual(historyPush([], 'привет'), ['привет']);
assert.deepStrictEqual(historyPush(['c', 'b', 'a'], 'd'), ['d', 'c', 'b', 'a']);
assert.deepStrictEqual(historyPush(['c', 'b'], 'c'), ['c', 'b'], 'повтор верхнего не пишем');
assert.deepStrictEqual(historyPush(['b'], ''), ['b'], 'пустое не пишем');
assert.deepStrictEqual(historyPush(['b'], '   '), ['b'], 'пробелы не пишем');
assert.deepStrictEqual(historyPush(['b'], '  b  '), ['b'], 'пробелы обрезаем');
assert.deepStrictEqual(historyPush(['b'], null), ['b']);
assert.deepStrictEqual(historyPush(['b'], 'c', 1), ['c'], 'лимит списка');

const fifty = Array.from({ length: 50 }, (_, i) => 'm' + i);
const capped = historyPush(fifty, 'новое');
assert.strictEqual(capped.length, 50, 'больше 50 не храним');
assert.strictEqual(capped[0], 'новое');
assert.ok(!capped.includes('m49'), 'старейшее вытесняется');

// ---- historyStep: режим ↑/↓ ----
const idle = { active: false, index: -1, saved: '' };
assert.deepStrictEqual(historyStep([], idle, -1, { canEnter: true, current: '' }).action, { type: 'none' }, 'пустая история');
assert.deepStrictEqual(historyStep(['a'], idle, 1, { canEnter: true, current: '' }).action, { type: 'none' }, '↓ без режима ничего не делает');
assert.deepStrictEqual(historyStep(['a'], idle, -1, { canEnter: false, current: 'текст' }).action, { type: 'none' }, 'курсор не в начале — не перехватываем');

let r = historyStep(['a', 'b'], idle, -1, { canEnter: true, current: '' });
assert.deepStrictEqual(r.nav, { active: true, index: 0, saved: '' });
assert.deepStrictEqual(r.action, { type: 'set', text: 'a' });

r = historyStep(['a', 'b'], idle, -1, { canEnter: true, current: 'черновик' });
assert.deepStrictEqual(r.nav, { active: true, index: 0, saved: 'черновик' }, 'черновик запоминается');
assert.deepStrictEqual(r.action, { type: 'set', text: 'a' });

const at0 = { active: true, index: 0, saved: 'черновик' };
r = historyStep(['a', 'b'], at0, -1, { canEnter: false, current: 'a' });
assert.deepStrictEqual(r.nav, { active: true, index: 1, saved: 'черновик' });
assert.deepStrictEqual(r.action, { type: 'set', text: 'b' });

assert.deepStrictEqual(historyStep(['a', 'b'], r.nav, -1, { canEnter: false, current: 'b' }).action, { type: 'none' }, 'старше нет — не мешаем');
assert.deepStrictEqual(historyStep(['a', 'b'], r.nav, 1, { canEnter: false, current: 'b' }).action, { type: 'set', text: 'a' }, '↓ обратно к свежему');

r = historyStep(['a', 'b'], at0, 1, { canEnter: false, current: 'a' });
assert.deepStrictEqual(r.nav, { active: false, index: -1, saved: '' });
assert.deepStrictEqual(r.action, { type: 'restore', text: 'черновик' }, 'ниже нового — возвращаем черновик');

const noDraft = { active: true, index: 0, saved: '' };
assert.deepStrictEqual(historyStep(['a'], noDraft, 1, { canEnter: false, current: 'a' }).action, { type: 'restore', text: '' });

// ---- historyExit: Esc возвращает черновик ----
assert.deepStrictEqual(historyExit(idle).action, { type: 'none' }, 'без режима Esc свободен');
r = historyExit({ active: true, index: 1, saved: 'черновик' });
assert.deepStrictEqual(r.nav, { active: false, index: -1, saved: '' });
assert.deepStrictEqual(r.action, { type: 'restore', text: 'черновик' });
assert.deepStrictEqual(historyExit({ active: true, index: 0, saved: '' }).action, { type: 'restore', text: '' });

// ---- buildInputHistoryScript: текст впрыска в страницу dsh ----
const js = buildInputHistoryScript('abc123', ['второе', 'первое']);
assert.strictEqual(typeof js, 'string');
assert.ok(js.includes('[data-composer-input="true"]'), 'селектор композера dsh');
assert.ok(js.includes('beforeinput'), 'вставка через beforeinput (Lexical)');
assert.ok(js.includes('abc123'), 'ключ маршрута');
assert.ok(js.includes('второе'), 'текущая история передана');
assert.ok(js.includes('history/push/'), 'маршрут записи в main');
assert.ok(js.includes('__dshInputHistory'), 'точка управления для настроек');
assert.ok(buildInputHistoryScript('', null).includes('[]'), 'пустая история не ломает скрипт');
assert.ok(buildInputHistoryScript().includes('"key":""'), 'без ключа — пустая строка, не undefined');

console.log('OK: input-history — historyPush/historyStep/historyExit/buildInputHistoryScript');
