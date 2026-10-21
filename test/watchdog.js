'use strict';
/*
 * test/watchdog.js — waitForReady (src/watchdog.js): порт dsh открывается
 * раньше, чем приложение готово, — watchdog ждёт checkReady() до таймаута.
 * Функция извлекается по маркерам (test/_extract.js), электронные части
 * (openRunningGui, страницы) не подключаются.
 * Запуск: node test/watchdog.js (или npm test).
 */
const assert = require('assert');
const { extract } = require('./_extract');

const waitForReady = extract('waitForReady', {});

async function main() {
  // 1) уже готов — сразу true
  assert.strictEqual(await waitForReady(async () => true, 100, () => false, 5), true, 'сразу готов');

  // 2) стал готов после нескольких опросов — true
  let calls = 0;
  assert.strictEqual(await waitForReady(async () => {
    calls += 1;
    return calls >= 4;
  }, 1000, () => false, 5), true, 'готов после опросов');
  assert.strictEqual(calls, 4, 'остановился на первом true');

  // 3) так и не стал готов — таймаут, false
  const t0 = Date.now();
  assert.strictEqual(await waitForReady(async () => false, 60, () => false, 5), false, 'таймаут → false');
  assert.ok(Date.now() - t0 >= 55, 'дождался до таймаута');

  // 4) лаунчер закрывается — false без ожидания таймаута
  assert.strictEqual(await waitForReady(async () => false, 1000, () => true, 5), false, 'стоп → false');

  // 5) checkReady бросает (сеть ещё не отвечает) — «не готов», не пробрасывает
  let threw = true;
  try {
    assert.strictEqual(await waitForReady(async () => { throw new Error('ECONNREFUSED'); }, 40, () => false, 5), false);
    threw = false;
  } catch { /* не должно дойти */ }
  assert.strictEqual(threw, false, 'исключение из checkReady не пробрасывается');

  console.log('OK: watchdog — waitForReady');
}

main().catch((e) => { console.error('FAIL:', e.message); process.exit(1); });
