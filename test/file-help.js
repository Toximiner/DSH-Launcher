'use strict';
/*
 * test/file-help.js — restartDsh (src/file-help.js): dsh, запущенный не
 * лаунчером (attach), не трогаем — показываем объяснение; dsh лаунчера —
 * останавливаем и запускаем заново (с подтверждением или без, например из
 * dshlauncher://menu/restart-dsh/). И gpuStatusText — строки состояния GPU.
 * Обе функции извлекаются по маркерам (test/_extract.js); dialog/pages/dshStop/
 * launch/portOpen/sleep инжектим.
 * Запуск: node test/file-help.js (или npm test).
 */
const assert = require('assert');
const { extract } = require('./_extract');

const tr = (ru) => ru; // только русские формулировки

function makeDeps({
  weStartedDsh = true,
  adoptedDsh = false,
  launching = false,
  stopping = false,
  restartingDsh = false,
  destroyed = false,
  dialogResponses = [],
  portUpTimes = 0,
} = {}) {
  const win = { isDestroyed: () => destroyed };
  const dialog = {
    calls: [],
    showMessageBox: async (w, opts) => {
      dialog.calls.push({ win: w, opts });
      return { response: dialogResponses.length ? dialogResponses.shift() : 0 };
    },
  };
  const pages = {
    shown: [],
    showPage: (render) => { pages.shown.push(render); },
    showStatus: (title, body, isError) => ({ title, body, isError }),
  };
  const dshStop = {
    stopped: [],
    stopDsh: async () => { dshStop.stopped.push('stopDsh'); },
    stopAdoptedDsh: async () => { dshStop.stopped.push('stopAdoptedDsh'); },
  };
  const launch = {
    started: 0,
    launchDshInner: async () => { launch.started += 1; },
  };
  const state = {
    win,
    stopping,
    launching,
    restartingDsh,
    weStartedDsh,
    adoptedDsh,
    adoptedOldPgid: adoptedDsh ? 4242 : null,
    dshProc: { pid: 777 },
    dshFatal: false,
  };
  let portUpsLeft = portUpTimes;
  const portOpen = async () => (portUpsLeft-- > 0);
  const sleep = async () => { /* без реального ожидания */ };
  const logs = [];
  return {
    state,
    dialog,
    pages,
    dshStop,
    launch,
    portOpen,
    sleep,
    tr,
    console: { log: (m) => logs.push(m), error: (m) => logs.push(m) },
    logs,
  };
}

async function main() {
  // 1) Чужой dsh (attach): не останавливаем и не перезапускаем — объясняем,
  //    где его перезапускать.
  let d = makeDeps({ weStartedDsh: false, adoptedDsh: false });
  let restartDsh = extract('restartDsh', d);
  await restartDsh({ confirm: true });
  assert.strictEqual(d.dialog.calls.length, 1, 'показан диалог о чужом dsh');
  assert.strictEqual(d.dialog.calls[0].opts.type, 'info');
  assert.strictEqual(d.dialog.calls[0].opts.message, 'Этот dsh запущен не лаунчером');
  assert.ok(/перезапустите его там, где запускали/.test(d.dialog.calls[0].opts.detail), 'подсказка — перезапустить в терминале');
  assert.strictEqual(d.dshStop.stopped.length, 0, 'чужой dsh НЕ остановлен');
  assert.strictEqual(d.launch.started, 0, 'dsh НЕ запущен заново');
  assert.strictEqual(d.state.weStartedDsh, false, 'флаги не менялись');
  assert.strictEqual(d.pages.shown.length, 0, 'страница «Перезапуск…» не показана');

  // 2) Свой dsh, confirm, «Отмена» — ничего не делаем.
  d = makeDeps({ weStartedDsh: true, dialogResponses: [1] });
  restartDsh = extract('restartDsh', d);
  await restartDsh({ confirm: true });
  assert.strictEqual(d.dialog.calls.length, 1, 'запрос подтверждения');
  assert.strictEqual(d.dialog.calls[0].opts.type, 'question');
  assert.deepStrictEqual(d.dialog.calls[0].opts.buttons, ['Перезапустить', 'Отмена']);
  assert.strictEqual(d.dialog.calls[0].opts.cancelId, 1, '«Отмена» — cancelId');
  assert.strictEqual(d.dshStop.stopped.length, 0, '«Отмена» — dsh не остановлен');
  assert.strictEqual(d.launch.started, 0, '«Отмена» — dsh не запущен');

  // 3) Свой dsh, confirm, «Перезапустить» — стоп, ожидание порта, запуск.
  d = makeDeps({ weStartedDsh: true, dialogResponses: [0], portUpTimes: 2 });
  restartDsh = extract('restartDsh', d);
  await restartDsh({ confirm: true });
  assert.strictEqual(d.dshStop.stopped.length, 1, 'свой dsh остановлен');
  assert.strictEqual(d.dshStop.stopped[0], 'stopDsh');
  assert.strictEqual(d.launch.started, 1, 'dsh запущен заново');
  assert.strictEqual(d.state.weStartedDsh, false, 'after restart: не наш (пока новый не поднимется)');
  assert.strictEqual(d.state.adoptedDsh, false, 'adopted сброшен');
  assert.strictEqual(d.state.adoptedOldPgid, null, 'старый pgid сброшен');
  assert.strictEqual(d.state.dshProc, null, 'процесс сброшен');
  assert.strictEqual(d.state.restartingDsh, false, 'режест: restartingDsh снят');
  assert.strictEqual(d.state.launching, false, 'режест: launching снят');
  assert.ok(d.logs.some((m) => /перезапуск dsh по запросу/.test(m)), 'лог о перезапуске');
  assert.ok(typeof d.pages.shown[0] === 'function', 'показана страница «Перезапуск dsh…»');
  const rendered = d.pages.shown[0]();
  assert.strictEqual(rendered.title, 'Перезапуск dsh…', 'заголовок страницы');
  assert.strictEqual(rendered.isError, false, 'страница — не ошибка');

  // 4) «Усыновлённый» dsh, confirm:false (dshlauncher://menu/restart-dsh/) —
  //    без диалога, через stopAdoptedDsh.
  d = makeDeps({ weStartedDsh: false, adoptedDsh: true });
  restartDsh = extract('restartDsh', d);
  await restartDsh({ confirm: false });
  assert.strictEqual(d.dialog.calls.length, 0, 'confirm:false — без диалога');
  assert.deepStrictEqual(d.dshStop.stopped, ['stopAdoptedDsh'], 'остановлен через stopAdoptedDsh');
  assert.strictEqual(d.launch.started, 1, 'запущен заново');

  // 5) Лаунчер уже что-то запускает — запрос игнорируется.
  d = makeDeps({ weStartedDsh: true, launching: true });
  restartDsh = extract('restartDsh', d);
  await restartDsh({ confirm: true });
  assert.strictEqual(d.dialog.calls.length, 0, 'launching — диалога нет');
  assert.strictEqual(d.dshStop.stopped.length, 0, 'launching — стоп не вызван');
  assert.strictEqual(d.launch.started, 0, 'launching — запуск не вызван');

  // 6) Окно уже уничтожено — тихо выходим.
  d = makeDeps({ weStartedDsh: true, destroyed: true });
  restartDsh = extract('restartDsh', d);
  await restartDsh({ confirm: true });
  assert.strictEqual(d.dshStop.stopped.length, 0, 'destroyed — ничего не делаем');
  assert.strictEqual(d.launch.started, 0, 'destroyed — ничего не запускаем');

  // 7) gpuStatusText — все ветки формулировок.
  const gpuStatusText = extract('gpuStatusText', { tr });
  assert.strictEqual(gpuStatusText({ on: true, marker: false }), 'включено');
  assert.strictEqual(gpuStatusText({ on: true, marker: true }),
    'включено; со следующего запуска будет отключено — GPU-процесс падал',
    'on + маркер уже создан — предупредим');
  assert.strictEqual(gpuStatusText({ on: false, reason: 'env' }), 'отключено (DSH_LAUNCHER_NO_GPU=1)');
  assert.strictEqual(gpuStatusText({ on: false, reason: 'setting' }), 'отключено в «Настройках»');
  assert.strictEqual(gpuStatusText({ on: false, reason: 'marker', marker: true, daysLeft: 12 }),
    'отключено после сбоев GPU-процесса (ещё 12 дн.)');
  assert.strictEqual(gpuStatusText({ on: false, reason: 'marker', marker: false }),
    'отключено до перезапуска лаунчера (включится при следующем запуске)',
    'маркер создан только что — в этой сессии ещё работает');
  assert.strictEqual(gpuStatusText({ on: false, reason: 'norender' }), 'отключено (нет /dev/dri/renderD* — ВМ без 3D?)');
  assert.strictEqual(gpuStatusText({ on: false, reason: 'egl' }), 'отключено (нет библиотек EGL)');

  console.log('OK: file-help — restartDsh, gpuStatusText');
}

main().catch((e) => { console.error('FAIL:', e.message); process.exit(1); });
