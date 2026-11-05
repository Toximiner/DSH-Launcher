'use strict';

/*
 * Unit tests for src/proc-log.js (runWithLiveLog): the live log of the dsh,
 * market plugin and update steps. The processes are real (node -e).
 *
 * Run: node test/proc-log.js   (npm test)
 */

const assert = require('assert');
const { runWithLiveLog, LOG_MAX_LINES, REDRAW_MS } = require('../src/proc-log');

const node = process.execPath;

(async () => {
  // stdout and stderr output line by line, no empty lines or edge whitespace
  {
    const st = { log: ['старое'] };
    let redraws = 0;
    const code = await runWithLiveLog(st, node, ['-e', 'console.log("a\\n\\n  b  "); console.error("c")'], { redraw: () => { redraws++; } });
    assert.strictEqual(code, 0, 'exit code 0');
    assert.deepStrictEqual(st.log.slice().sort(), ['a', 'b', 'c'], 'the stdout and stderr lines, the log was restarted');
    assert.ok(redraws >= 1, 'the page was redrawn immediately');
    assert.strictEqual(st.running, false, 'running was reset');
    assert.strictEqual(st.proc, null, 'proc was reset');
    assert.strictEqual(st.timer, null, 'the redraw timer was cleared');
  }
  // while running — running and proc are set
  {
    const st = {};
    const pr = runWithLiveLog(st, node, ['-e', 'process.exit(3)']);
    assert.strictEqual(st.running, true, 'running while it runs');
    assert.ok(st.proc && st.proc.pid, 'proc while it runs');
    assert.strictEqual(await pr, 3, 'a non-zero exit code');
  }
  // log line limit
  {
    const st = {};
    await runWithLiveLog(st, node, ['-e', `for (let i = 0; i < ${LOG_MAX_LINES + 20}; i++) console.log(i)`]);
    assert.strictEqual(st.log.length, LOG_MAX_LINES, `no more than ${LOG_MAX_LINES} lines`);
    assert.strictEqual(st.log[st.log.length - 1], String(LOG_MAX_LINES + 19), 'the last lines are kept');
  }
  // killed by a signal — -1
  {
    const st = {};
    const pr = runWithLiveLog(st, node, ['-e', 'setInterval(() => {}, 1000)']);
    st.proc.kill('SIGTERM');
    assert.strictEqual(await pr, -1, 'killed by a signal');
  }
  // failed to start — -1 and the error text in the log
  {
    const st = {};
    const code = await runWithLiveLog(st, '/nonexistent/dsh-launcher-test', []);
    assert.strictEqual(code, -1, 'ENOENT');
    assert.ok(st.log.some((l) => /ENOENT/.test(l)), 'the launch error is in the log');
    assert.strictEqual(st.running, false, 'running was reset after the error too');
  }
  // env and cwd are passed through
  {
    const st = {};
    await runWithLiveLog(st, node, ['-e', 'console.log(process.env.PL_TEST + " " + process.cwd())'], { env: { ...process.env, PL_TEST: 'x' }, cwd: '/' });
    assert.deepStrictEqual(st.log, ['x /'], 'env and cwd');
  }
  // the live redraw period — pages use it for the progress tick too
  assert.strictEqual(REDRAW_MS, 800, 'REDRAW_MS');
  console.log('OK: proc-log — all checks passed');
})().catch((e) => { console.error(e); process.exit(1); });
