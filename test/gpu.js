'use strict';
/*
 * test/gpu.js — onChildProcessGone (src/gpu.js): две смерти GPU-процесса
 * за сессию = драйвер сломан — пишем маркер gpu-disabled.marker, следующие
 * запуски идут в программном рендере. Функция извлекается по маркерам
 * (test/_extract.js); fs/console/константы инжектим, чтобы не трогать
 * реальный диск и логи.
 * Запуск: node test/gpu.js (или npm test).
 */
const assert = require('assert');
const { EventEmitter } = require('events');
const { extract } = require('./_extract');

const MARKER = '/tmp/gpu-marker-test';

function makeDeps() {
  const written = [];
  const logs = [];
  return {
    written,
    logs,
    state: { launcherUpdatedOnDisk: false },
    fs: {
      mkdirSync: (dir, opts) => { written.push(['mkdir', dir, opts]); },
      writeFileSync: (file, content) => { written.push(['write', file, content]); },
    },
    console: { log: (m) => logs.push(m), error: (m) => logs.push(m) },
    GPU_MARKER_FILE: MARKER,
    gpuDisabledByMarker: false,
    LOG_DIR: '/tmp',
  };
}

async function main() {
  // 1) Два краша GPU-процесса → маркер записан, в тексте — причина и причина.
  let d = makeDeps();
  let onGone = extract('onChildProcessGone', d);
  let restarts = 0;
  const app = new EventEmitter();
  onGone(app, () => { restarts += 1; });
  app.emit('child-process-gone', {}, { type: 'GPU', reason: 'crashed', exitCode: 11 });
  assert.strictEqual(d.written.length, 0, 'один краш — маркера ещё нет');
  app.emit('child-process-gone', {}, { type: 'GPU', reason: 'abnormal-exit', exitCode: 42 });
  assert.strictEqual(d.written.length, 2, 'второй краш — mkdir + запись маркера');
  assert.ok(d.written[0][0] === 'mkdir' && d.written[0][1] === '/tmp', 'LOG_DIR создан (recursive)');
  assert.ok(d.written[0][2] && d.written[0][2].recursive === true, 'mkdir recursive');
  assert.strictEqual(d.written[1][0], 'write');
  assert.strictEqual(d.written[1][1], MARKER, 'маркер в GPU_MARKER_FILE');
  assert.ok(/GPU process crashed x2, reason=abnormal-exit/.test(d.written[1][2]), 'содержимое маркера: причина + x2');
  assert.ok(/20\d{2}-\d{2}-\d{2}T/.test(d.written[1][2]), 'в маркере ISO-дата');
  assert.ok(d.logs.some((m) => /GPU-драйвер, похоже, сломан/.test(m)), 'лог о сломанном драйвере');
  assert.strictEqual(restarts, 0, 'без обновления на диске — не перезапускаемся');

  // 2) Не-GPU-процессы и безобидные завершения не считаются.
  d = makeDeps();
  onGone = extract('onChildProcessGone', d);
  const app2 = new EventEmitter();
  onGone(app2, () => { restarts += 1; });
  app2.emit('child-process-gone', {}, { type: 'Renderer', reason: 'crashed', exitCode: 11 });
  app2.emit('child-process-gone', {}, { type: 'Utility', reason: 'killed', exitCode: 0 });
  app2.emit('child-process-gone', {}, { type: 'GPU', reason: 'clean-exit', exitCode: 0 });
  app2.emit('child-process-gone', {}, { type: 'GPU', reason: 'killed', exitCode: 15 });
  assert.strictEqual(d.written.length, 0, 'не-GPU / чистые выходы — маркер не пишется');
  assert.strictEqual(restarts, 0, 'перезапусков нет');

  // 3) Процесс завершился после обновления на диске → restartLauncher()
  //    без подсчёта GPU-крашей.
  d = makeDeps();
  d.state.launcherUpdatedOnDisk = true;
  onGone = extract('onChildProcessGone', d);
  const app3 = new EventEmitter();
  let called = 0;
  onGone(app3, () => { called += 1; });
  app3.emit('child-process-gone', {}, { type: 'GPU', reason: 'crashed', exitCode: 11 });
  assert.strictEqual(called, 1, 'обновление на диске — перезапуск лаунчера');
  assert.strictEqual(d.written.length, 0, '…и маркер не пишется');
  assert.ok(d.logs.some((m) => /перезапускаюсь/.test(m)), 'лог о перезапуске');

  // 4) Маркер уже живёт (gpuDisabledByMarker) — третий краш не дописывает его.
  d = makeDeps();
  d.gpuDisabledByMarker = true;
  onGone = extract('onChildProcessGone', d);
  const app4 = new EventEmitter();
  onGone(app4, () => { });
  app4.emit('child-process-gone', {}, { type: 'GPU', reason: 'crashed', exitCode: 11 });
  app4.emit('child-process-gone', {}, { type: 'GPU', reason: 'crashed', exitCode: 11 });
  assert.strictEqual(d.written.length, 0, 'маркер уже есть — не дублируем');

  // 5) Ошибка записи (нет прав на LOG_DIR) — не роняет хендлер.
  d = makeDeps();
  d.fs.mkdirSync = () => { throw new Error('EACCES'); };
  onGone = extract('onChildProcessGone', d);
  const app5 = new EventEmitter();
  onGone(app5, () => { });
  let threw = true;
  try {
    app5.emit('child-process-gone', {}, { type: 'GPU', reason: 'crashed', exitCode: 11 });
    app5.emit('child-process-gone', {}, { type: 'GPU', reason: 'crashed', exitCode: 11 });
    threw = false;
  } catch { /* не должно дойти */ }
  assert.strictEqual(threw, false, 'исключение из записи маркера не пробрасывается');
  assert.ok(d.logs.some((m) => /не удалось записать GPU-маркер/.test(m)), 'ошибка записи залогирована');

  console.log('OK: gpu — onChildProcessGone');
}

main().catch((e) => { console.error('FAIL:', e.message); process.exit(1); });
