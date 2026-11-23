'use strict';
/*
 * test/respond-port.js — ответы dshlauncher:// (src/respond.js) и модуль
 * порта/stdout (src/port.js): чистые модули, прямой require.
 * Run: node test/respond-port.js (or npm test).
 */
process.env.DSH_PORT = '3299'; // portOpen/waitForPort смотрят именно его (env.js читает при require)
const assert = require('assert');
const nodeHttp = require('http');
const { blankResponse, noContentResponse } = require('../src/respond');
const state = require('../src/state');
const { pushErr, stderrTail, portOpen, waitForPort, waitForAuthUrl } = require('../src/port');

// ---- respond.js: blank — 200 и тёмная страница; noContent — 204 без тела ----
{
  const blank = blankResponse();
  assert.strictEqual(blank.status, 200);
  assert.strictEqual(blank.headers.get('content-type'), 'text/html');
  const no = noContentResponse();
  assert.strictEqual(no.status, 204);
  assert.strictEqual(no.body, null, 'noContent — пустое тело');
}

// ---- pushErr: trim, пустые строки мимо, хвост — не длиннее 300 ----
{
  const before = stderrTail.length;
  const origErr = console.error;
  console.error = () => {};
  pushErr('  line one  ');
  pushErr('');
  pushErr('   ');
  pushErr('line two');
  assert.deepStrictEqual(stderrTail.slice(before), ['line one', 'line two'], 'trim; пустые строки мимо');
  for (let i = 0; i < 400; i++) pushErr(`fill ${i}`);
  console.error = origErr;
  assert.strictEqual(stderrTail.length, 300, 'хвост ограничен 300');
  assert.strictEqual(stderrTail[stderrTail.length - 1], 'fill 399', 'новейшая строка — в конце');
}

async function main() {
  // 1) слушателя нет — false (connection refused, без ожидания таймаута)
  assert.strictEqual(await portOpen(), false, 'нет слушателя → false');

  // 2) процесс dsh уже завершился (или fatal) — waitForPort не ждёт до дедлайна
  state.dshProc = { exitCode: 1, dshFatal: false };
  assert.strictEqual(await waitForPort(), false, 'dsh завершился → досрочный выход');
  state.dshProc = { exitCode: null, dshFatal: true };
  assert.strictEqual(await waitForPort(), false, 'dshFatal → досрочный выход');
  state.dshProc = null;

  // 3) слушатель появился — true
  const server = nodeHttp.createServer((req, res) => res.end('ok')).listen(3299, '127.0.0.1');
  await new Promise((r) => server.once('listening', r));
  assert.strictEqual(await portOpen(), true, 'слушатель → true');
  assert.strictEqual(await waitForPort(), true, 'waitForPort — порт открыт');
  server.close();

  // 4) waitForAuthUrl: URL появляется во время поллинга; нет — state.authUrl
  state.authUrl = null;
  const t = setTimeout(() => { state.authUrl = 'http://127.0.0.1:3299/?token=t'; }, 150);
  assert.strictEqual(await waitForAuthUrl(2000), 'http://127.0.0.1:3299/?token=t', 'подождал URL');
  clearTimeout(t);
  state.authUrl = null;
  assert.strictEqual(await waitForAuthUrl(300), null, 'URL так и не было → null');

  console.log('OK: respond, port — all checks passed');
}

main().catch((e) => { console.error('FAIL:', e.message); process.exit(1); });
