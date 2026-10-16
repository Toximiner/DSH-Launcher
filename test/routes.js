'use strict';
// Юнит-тест matchRoute (main.js): таблица маршрутов dshlauncher:// —
// каждый префикс → своё имя, порядок (приоритеты) сохранён, чужое → null.
const assert = require('assert');
const { extract } = require('./_extract');

const matchRoute = extract('matchRoute', {});

const expected = [
  ['/retry/', 'retry'],
  ['/lang/', 'lang'],
  ['/install/copy/', 'install/copy'],
  ['/install/run/', 'install/run'],
  ['/install/recheck/', 'install/recheck'],
  ['/market/install/', 'market/install'],
  ['/market/skip/', 'market/skip'],
  ['/market/never/', 'market/never'],
  ['/update/install/', 'update/install'],
  ['/update/later/', 'update/later'],
  ['/update/never/', 'update/never'],
  ['/update/recheck/', 'update/recheck'],
  ['/update/copy/', 'update/copy'],
  ['/update/restart/', 'update/restart'],
  ['/menu/about/', 'menu/about'],
  ['/menu/check/', 'menu/check'],
  ['/menu/rollback/', 'menu/rollback'],
  ['/rollback/', 'rollback'],
  ['/menu/update/', 'menu/update'],
  ['/toast/', 'toast'],
  ['/update-now/', 'update-now'],
  ['/find/', 'find'],
  ['/dialog/logs/', 'dialog/logs'],
  ['/menu/quit/', 'menu/quit'],
  ['/window/close/', 'window/close'],
  ['/menu/report/', 'menu/report'],
  ['/dialog/report-show/', 'dialog/report-show'],
  ['/dialog/log-refresh/', 'dialog/log-refresh'],
  ['/menu/log/', 'menu/log'],
  ['/menu/settings/', 'menu/settings'],
  ['/settings/set/', 'settings/set'],
  ['/settings/section/', 'settings/section'],
  ['/settings/pick-dsh/', 'settings/pick-dsh'],
  ['/settings/pick-cwd/', 'settings/pick-cwd'],
  ['/settings/reset-updates/', 'settings/reset-updates'],
  ['/settings/relaunch/', 'settings/relaunch'],
  ['/menu/close-context/', 'menu/close-context'],
  ['/dialog/gpu-enable/', 'dialog/gpu-enable'],
  ['/menu/restart-dsh/', 'menu/restart-dsh'],
  ['/dialog/close/', 'dialog/close'],
  ['/dialog/copy/', 'dialog/copy'],
  ['/history/push/', 'history/push'],
];
for (const [prefix, name] of expected) {
  assert.strictEqual(matchRoute(prefix + 'x'), name, prefix);
}

// приоритеты: /menu/rollback/ раньше /rollback/ (иначе откат кликал бы диалог)
assert.strictEqual(matchRoute('/menu/rollback/v1.0.0'), 'menu/rollback');
assert.strictEqual(matchRoute('/rollback/v1.0.0'), 'rollback');

// чужие/неизвестные маршруты → null (дальше — onPasteRequest)
assert.strictEqual(matchRoute('/paste/https%3A%2F%2Fexample.com/'), null);
assert.strictEqual(matchRoute('/unknown/x/'), null);
assert.strictEqual(matchRoute('/'), null);

console.log('OK: routes — matchRoute');
