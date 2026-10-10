#!/usr/bin/env node
// Юнит-тесты окна «Настройки» (main.js): effectiveConfig — итоговые параметры
// из переменных окружения (важнее) и settings.json; settingValue — проверка
// значений полей; restartNeeded — что изменится после перезапуска.
'use strict';
const assert = require('assert');
const { extract, mainSrc } = require('./_extract');

const splitArgs = extract('splitArgs');
const cfg = extract('effectiveConfig', { splitArgs, DEFAULT_PORT: 3080, DEFAULT_PROFILE: 'web' });
const HOME = '/home/u';

// ---- effectiveConfig: по умолчанию ----
{
  const c = cfg({}, {}, HOME);
  assert.deepStrictEqual(c.args, ['--profile', 'web', '--no-open'], 'по умолчанию: профиль web, без --port');
  assert.strictEqual(c.port, 3080);
  assert.strictEqual(c.dshBin, null, 'путь к dsh — искать автоматически');
  assert.strictEqual(c.cwd, HOME, 'рабочая папка — домашняя');
  assert.deepStrictEqual([c.noGpu, c.ozone, c.checkUpdates], [false, 'auto', true]);
  assert.ok(Object.values(c.src).every((v) => v === 'default'), 'всё — по умолчанию');
  assert.deepStrictEqual(cfg({}, null, HOME).args, c.args, 'нет файла настроек');
  assert.deepStrictEqual(cfg({}, 'мусор', HOME).args, c.args, 'файл настроек — не объект');
}

// ---- effectiveConfig: из settings.json ----
{
  const c = cfg({}, { profile: 'work', port: 3090, dshPath: '/opt/dsh/bin/dsh', cwd: '/srv/p', gpu: 'off', ozone: 'x11', checkUpdates: false }, HOME);
  assert.deepStrictEqual(c.args, ['--profile', 'work', '--no-open', '--port', '3090'], 'профиль и порт передаются dsh');
  assert.deepStrictEqual([c.port, c.dshBin, c.cwd, c.noGpu, c.ozone, c.checkUpdates],
    [3090, '/opt/dsh/bin/dsh', '/srv/p', true, 'x11', false]);
  assert.ok(Object.values(c.src).every((v) => v === 'settings'), 'всё — из файла');
}

// ---- effectiveConfig: мусор в файле игнорируется ----
{
  const c = cfg({}, { profile: 'a b;rm', port: 'x', gpu: 'maybe', ozone: 'mir', dshPath: 5, checkUpdates: 'no' }, HOME);
  assert.deepStrictEqual(c.args, ['--profile', 'web', '--no-open'], 'плохой профиль/порт — по умолчанию');
  assert.deepStrictEqual([c.port, c.noGpu, c.ozone, c.dshBin, c.checkUpdates], [3080, false, 'auto', null, true]);
  assert.strictEqual(cfg({}, { port: 70000 }, HOME).port, 3080, 'порт вне диапазона');
}

// ---- effectiveConfig: переменные окружения важнее файла ----
{
  const env = { DSH_PORT: '4000', DSH_ARGS: '--profile ci --no-open', DSH_BIN: '/e/dsh', DSH_CWD: '/e',
    DSH_LAUNCHER_NO_GPU: '1', DSH_LAUNCHER_OZONE: 'wayland', DSH_LAUNCHER_NO_UPDATE_CHECK: '1' };
  const c = cfg(env, { profile: 'work', port: 3090, dshPath: '/s/dsh', cwd: '/s', gpu: 'auto', ozone: 'x11', checkUpdates: true }, HOME);
  assert.deepStrictEqual(c.args, ['--profile', 'ci', '--no-open'], 'DSH_ARGS — как есть (без добавления --port)');
  assert.deepStrictEqual([c.port, c.dshBin, c.cwd, c.noGpu, c.ozone, c.checkUpdates], [4000, '/e/dsh', '/e', true, 'wayland', false]);
  assert.ok(Object.values(c.src).every((v) => v === 'env'), 'всё — из переменных');
  assert.deepStrictEqual(cfg({ DSH_PORT: '4000' }, {}, HOME).args, ['--profile', 'web', '--no-open', '--port', '4000'],
    'DSH_PORT без DSH_ARGS — dsh слушает тот же порт');
  assert.strictEqual(cfg({ DSH_LAUNCHER_OZONE: 'bogus' }, { ozone: 'x11' }, HOME).ozone, 'x11', 'неверная переменная — берём файл');
}

// ---- settingValue: проверка полей; restartNeeded ----
const { settingValue, restartNeeded } = new Function('tr',
  mainSrc.slice(mainSrc.indexOf('/* ===== settingValue '), mainSrc.indexOf('/* ===== end settingValue ===== */')) +
  ';return { settingValue, restartNeeded };')((ru) => ru);

assert.deepStrictEqual(settingValue('port', ' 3090 '), { value: 3090 }, 'порт');
for (const bad of ['80', '70000', 'abc', '', '30.5']) assert.ok(settingValue('port', bad).error, `порт «${bad}» — ошибка`);
assert.deepStrictEqual(settingValue('profile', 'my-profile_2.0'), { value: 'my-profile_2.0' }, 'профиль');
for (const bad of ['', 'a b', 'x;rm -rf', '../etc']) assert.ok(settingValue('profile', bad).error, `профиль «${bad}» — ошибка`);
assert.deepStrictEqual(settingValue('dshPath', '/usr/local/bin/dsh'), { value: '/usr/local/bin/dsh' }, 'путь');
assert.deepStrictEqual(settingValue('dshPath', ''), { value: '' }, 'пустой путь — «авто»');
assert.ok(settingValue('cwd', 'relative/dir').error, 'относительная папка — ошибка');
assert.deepStrictEqual(settingValue('checkUpdates', 'false'), { value: false }, 'галочка');
assert.deepStrictEqual(settingValue('spellcheck', 'true'), { value: true }, 'галочка');
assert.ok(settingValue('market', 'yes').error, 'галочка — только true/false');
assert.deepStrictEqual(settingValue('lang', 'auto'), { value: 'auto' }, 'язык');
assert.ok(settingValue('lang', 'de').error, 'неизвестный язык');
assert.deepStrictEqual(settingValue('gpu', 'off'), { value: 'off' }, 'GPU');
assert.deepStrictEqual(settingValue('ozone', 'x11'), { value: 'x11' }, 'режим окна');
assert.ok(settingValue('ozone', 'mir').error, 'неизвестный режим окна');
assert.ok(settingValue('nope', '1').error, 'неизвестная настройка');

const base = cfg({}, {}, HOME);
assert.deepStrictEqual(restartNeeded(base, cfg({}, {}, HOME)), [], 'ничего не менялось');
assert.deepStrictEqual(restartNeeded(base, cfg({}, { profile: 'w2' }, HOME)), ['args'], 'профиль → перезапуск');
assert.deepStrictEqual(restartNeeded(base, cfg({}, { port: 3091 }, HOME)).sort(), ['args', 'port'], 'порт → перезапуск');
assert.deepStrictEqual(restartNeeded(base, cfg({}, { gpu: 'off', ozone: 'x11', cwd: '/x', dshPath: '/d' }, HOME)).sort(),
  ['cwd', 'dshBin', 'noGpu', 'ozone'], 'GPU, окно, папка, путь → перезапуск');
assert.deepStrictEqual(restartNeeded(base, cfg({}, { checkUpdates: false }, HOME)), [], 'проверка обновлений — без перезапуска');

console.log('OK: effectiveConfig, settingValue, restartNeeded — все проверки прошли');
