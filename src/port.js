'use strict';
// Порт dsh и его stdout/stderr: проверка соединения, ожидание порта и
// авторизационного URL, хвост stderr (для страницы ошибки). Чистый модуль
// (без electron) — его можно подключать и из юнит-тестов.
const nodeNet = require('net');
const state = require('./state');
const { HOST, PORT, START_TIMEOUT } = require('./env');

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const stderrTail = [];

function pushErr(line) {
  const t = line.trim();
  if (!t) return;
  stderrTail.push(t);
  if (stderrTail.length > 300) stderrTail.shift();
  console.error('[dsh]', t);
}

function portOpen() {
  return new Promise((resolve) => {
    const s = new nodeNet.Socket();
    const done = (ok) => {
      try { s.destroy(); } catch { /* noop */ }
      resolve(ok);
    };
    s.setTimeout(800);
    s.once('connect', () => done(true));
    s.once('timeout', () => done(false));
    s.once('error', () => done(false));
    s.connect(PORT, HOST);
  });
}

async function waitForPort() {
  const deadline = Date.now() + START_TIMEOUT;
  while (Date.now() < deadline) {
    if (await portOpen()) return true;
    if (state.dshProc && (state.dshProc.exitCode !== null || state.dshFatal)) return false;
    await sleep(400);
  }
  return portOpen();
}

async function waitForAuthUrl(timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  while (!state.authUrl && Date.now() < deadline) await sleep(100);
  return state.authUrl;
}

module.exports = { sleep, stderrTail, pushErr, portOpen, waitForPort, waitForAuthUrl };
