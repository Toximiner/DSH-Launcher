#!/usr/bin/env node
// Юнит-тесты окна «Настройки» (src/config.js): effectiveConfig — итоговые параметры
// из переменных окружения (важнее) и settings.json; settingValue — проверка
// значений полей; restartNeeded — что изменится после перезапуска.
'use strict';
const assert = require('assert');
const i18n = require('../src/i18n');
const { effectiveConfig: cfg, settingValue, restartNeeded } = require('../src/config');

i18n.setLang('ru');
const HOME = '/home/u';

// ---- effectiveConfig: по умолчанию ----
{
  const c = cfg({}, {}, HOME);
  assert.deepStrictEqual(c.args, ['--profile', 'web', '--no-open'], 'по умолчанию: профиль web, без --port');
  assert.strictEqual(c.port, 3080);
  assert.strictEqual(c.dshBin, null, 'путь к dsh — искать автоматически');
  assert.strictEqual(c.cwd, HOME, 'рабочая папка — домашняя');
  assert.deepStrictEqual([c.noGpu, c.ozone, c.checkUpdates, c.pollUpdates, c.sandbox], [false, 'auto', true, true, false], 'песочница по умолчанию выключена');
  assert.ok(Object.values(c.src).every((v) => v === 'default'), 'всё — по умолчанию');
  assert.deepStrictEqual(cfg({}, null, HOME).args, c.args, 'нет файла настроек');
  assert.deepStrictEqual(cfg({}, 'мусор', HOME).args, c.args, 'файл настроек — не объект');
}

// ---- effectiveConfig: из settings.json ----
{
  const c = cfg({}, { profile: 'work', port: 3090, dshPath: '/opt/dsh/bin/dsh', cwd: '/srv/p', gpu: 'off', ozone: 'x11', checkUpdates: false, pollUpdates: false, sandbox: true }, HOME);
  assert.deepStrictEqual(c.args, ['--profile', 'work', '--no-open', '--port', '3090'], 'профиль и порт передаются dsh');
  assert.deepStrictEqual([c.port, c.dshBin, c.cwd, c.noGpu, c.ozone, c.checkUpdates],
    [3090, '/opt/dsh/bin/dsh', '/srv/p', true, 'x11', false]);
  assert.strictEqual(c.pollUpdates, false, 'раз в 12 часов — выключено');
  assert.strictEqual(c.sandbox, true, 'песочница — по галочке');
  assert.ok(Object.values(c.src).every((v) => v === 'settings'), 'всё — из файла');
  const onlyStart = cfg({}, { checkUpdates: false }, HOME);
  assert.deepStrictEqual([onlyStart.checkUpdates, onlyStart.pollUpdates], [false, true], 'галочки независимы: без проверки при запуске');
  const onlyPoll = cfg({}, { pollUpdates: false }, HOME);
  assert.deepStrictEqual([onlyPoll.checkUpdates, onlyPoll.pollUpdates], [true, false], 'галочки независимы: без проверки по времени');
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
  const c = cfg(env, { profile: 'work', port: 3090, dshPath: '/s/dsh', cwd: '/s', gpu: 'auto', ozone: 'x11', checkUpdates: true, pollUpdates: true }, HOME);
  assert.deepStrictEqual(c.args, ['--profile', 'ci', '--no-open'], 'DSH_ARGS — как есть (без добавления --port)');
  assert.deepStrictEqual([c.port, c.dshBin, c.cwd, c.noGpu, c.ozone, c.checkUpdates, c.pollUpdates], [4000, '/e/dsh', '/e', true, 'wayland', false, false],
    'DSH_LAUNCHER_NO_UPDATE_CHECK — ни при запуске, ни по времени');
  assert.ok(Object.entries(c.src).every(([k, v]) => v === 'env' || k === 'sandbox'), 'всё — из переменных (у песочницы переменной нет)');
  assert.deepStrictEqual(cfg({ DSH_PORT: '4000' }, {}, HOME).args, ['--profile', 'web', '--no-open', '--port', '4000'],
    'DSH_PORT без DSH_ARGS — dsh слушает тот же порт');
  assert.strictEqual(cfg({ DSH_LAUNCHER_OZONE: 'bogus' }, { ozone: 'x11' }, HOME).ozone, 'x11', 'неверная переменная — берём файл');
}

// ---- settingValue: проверка полей; restartNeeded ----

assert.deepStrictEqual(settingValue('port', ' 3090 '), { value: 3090 }, 'порт');
for (const bad of ['80', '70000', 'abc', '', '30.5']) assert.ok(settingValue('port', bad).error, `порт «${bad}» — ошибка`);
assert.deepStrictEqual(settingValue('profile', 'my-profile_2.0'), { value: 'my-profile_2.0' }, 'профиль');
for (const bad of ['', 'a b', 'x;rm -rf', '../etc', '.', '..', '...']) assert.ok(settingValue('profile', bad).error, `профиль «${bad}» — ошибка`);
assert.deepStrictEqual(settingValue('profile', '.web'), { value: '.web' }, 'профиль с точкой в начале — можно');
assert.ok(settingValue('port', null).error && settingValue('port', undefined).error, 'порт: нет значения — ошибка');
for (const [k, ok, bad] of [['gpu', 'off', 'on'], ['ozone', 'x11', 'xorg'], ['lang', 'en', 'de'], ['spellcheck', 'false', 'no']]) {
  assert.ok(!settingValue(k, ok).error, `${k}: «${ok}» — можно`);
  assert.strictEqual(settingValue(k, bad).error, 'bad value', `${k}: «${bad}» — ошибка`);
}
assert.ok(/unknown setting/.test(settingValue('nope', 'x').error), 'неизвестная настройка');
assert.deepStrictEqual(settingValue('dshPath', '/usr/local/bin/dsh'), { value: '/usr/local/bin/dsh' }, 'путь');
assert.deepStrictEqual(settingValue('dshPath', ''), { value: '' }, 'пустой путь — «авто»');
assert.ok(settingValue('cwd', 'relative/dir').error, 'относительная папка — ошибка');
assert.deepStrictEqual(settingValue('checkUpdates', 'false'), { value: false }, 'галочка');
assert.deepStrictEqual(settingValue('spellcheck', 'true'), { value: true }, 'галочка');
assert.deepStrictEqual(settingValue('pollUpdates', 'false'), { value: false }, 'галочка');
assert.deepStrictEqual(settingValue('closeToTray', 'true'), { value: true }, 'галочка «в трей»');
assert.deepStrictEqual(settingValue('attention', 'false'), { value: false }, 'галочка «внимание»');
assert.deepStrictEqual(settingValue('attentionSound', 'true'), { value: true }, 'галочка «звук»');
assert.ok(settingValue('spellcheck', 'yes').error, 'галочка — только true/false');
assert.ok(settingValue('market', 'true').error, 'галочки маркета в настройках нет');
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
assert.deepStrictEqual(restartNeeded(base, cfg({}, { checkUpdates: false, pollUpdates: false }, HOME)), [], 'проверка обновлений — без перезапуска');
assert.deepStrictEqual(restartNeeded(base, cfg({}, { sandbox: true }, HOME)), ['sandbox'], 'песочница → перезапуск');
assert.ok(cfg({}, { sandbox: 'yes' }, HOME).sandbox === false, 'песочница — только true');

console.log('OK: effectiveConfig, settingValue, restartNeeded — все проверки прошли');
