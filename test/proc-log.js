'use strict';

/*
 * Юнит-тесты src/proc-log.js (runWithLiveLog): живой лог установки dsh,
 * плагина маркета и шагов обновления. Процессы — настоящие (node -e).
 *
 * Запуск: node test/proc-log.js   (npm test)
 */

const assert = require('assert');
const { runWithLiveLog, LOG_MAX_LINES } = require('../src/proc-log');

const node = process.execPath;

(async () => {
  // вывод stdout и stderr построчно, пустые строки и пробелы по краям — нет
  {
    const st = { log: ['старое'] };
    let redraws = 0;
    const code = await runWithLiveLog(st, node, ['-e', 'console.log("a\\n\\n  b  "); console.error("c")'], { redraw: () => { redraws++; } });
    assert.strictEqual(code, 0, 'код выхода 0');
    assert.deepStrictEqual(st.log.slice().sort(), ['a', 'b', 'c'], 'строки stdout и stderr, лог начат заново');
    assert.ok(redraws >= 1, 'страница перерисована сразу');
    assert.strictEqual(st.running, false, 'running сброшен');
    assert.strictEqual(st.proc, null, 'proc сброшен');
    assert.strictEqual(st.timer, null, 'таймер перерисовки снят');
  }
  // на время работы — running и proc
  {
    const st = {};
    const pr = runWithLiveLog(st, node, ['-e', 'process.exit(3)']);
    assert.strictEqual(st.running, true, 'running на время работы');
    assert.ok(st.proc && st.proc.pid, 'proc на время работы');
    assert.strictEqual(await pr, 3, 'ненулевой код выхода');
  }
  // лимит строк лога
  {
    const st = {};
    await runWithLiveLog(st, node, ['-e', `for (let i = 0; i < ${LOG_MAX_LINES + 20}; i++) console.log(i)`]);
    assert.strictEqual(st.log.length, LOG_MAX_LINES, `не больше ${LOG_MAX_LINES} строк`);
    assert.strictEqual(st.log[st.log.length - 1], String(LOG_MAX_LINES + 19), 'последние строки сохранены');
  }
  // убит сигналом — -1
  {
    const st = {};
    const pr = runWithLiveLog(st, node, ['-e', 'setInterval(() => {}, 1000)']);
    st.proc.kill('SIGTERM');
    assert.strictEqual(await pr, -1, 'убит сигналом');
  }
  // не запустился — -1 и текст ошибки в логе
  {
    const st = {};
    const code = await runWithLiveLog(st, '/nonexistent/dsh-launcher-test', []);
    assert.strictEqual(code, -1, 'ENOENT');
    assert.ok(st.log.some((l) => /ENOENT/.test(l)), 'ошибка запуска — в логе');
    assert.strictEqual(st.running, false, 'running сброшен и после ошибки');
  }
  // env и cwd передаются
  {
    const st = {};
    await runWithLiveLog(st, node, ['-e', 'console.log(process.env.PL_TEST + " " + process.cwd())'], { env: { ...process.env, PL_TEST: 'x' }, cwd: '/' });
    assert.deepStrictEqual(st.log, ['x /'], 'env и cwd');
  }
  console.log('OK: proc-log — все проверки прошли');
})().catch((e) => { console.error(e); process.exit(1); });
