'use strict';
// dsh port and its stdout/stderr: connection check, waiting for the port and
// the auth URL, stderr tail (for the error page). Pure module
// (no electron) — can be imported from unit tests as well.
const nodeNet = require('net');
const state = require('./state');
const { HOST, PORT, START_TIMEOUT } = require('./env');

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// The stderr tail for the error pages
const STDERR_TAIL_LINES = 300;
const stderrTail = [];

function pushErr(line) {
  const t = line.trim();
  if (!t) return;
  stderrTail.push(t);
  if (stderrTail.length > STDERR_TAIL_LINES) stderrTail.shift();
  console.error('[dsh]', t);
}

// One connection check: dsh answers fast or it does not
const PORT_OPEN_TIMEOUT_MS = 800;

function portOpen() {
  return new Promise((resolve) => {
    const s = new nodeNet.Socket();
    const done = (ok) => {
      try { s.destroy(); } catch { /* noop */ }
      resolve(ok);
    };
    s.setTimeout(PORT_OPEN_TIMEOUT_MS);
    s.once('connect', () => done(true));
    s.once('timeout', () => done(false));
    s.once('error', () => done(false));
    s.connect(PORT, HOST);
  });
}

// The waitForPort poll period
const PORT_POLL_MS = 400;

async function waitForPort() {
  const deadline = Date.now() + START_TIMEOUT;
  while (Date.now() < deadline) {
    if (await portOpen()) return true;
    if (state.dshProc && (state.dshProc.exitCode !== null || state.dshFatal)) return false;
    await sleep(PORT_POLL_MS);
  }
  return portOpen();
}

// The auth URL: poll often, wait up to AUTH_URL_TIMEOUT_MS by default
const AUTH_URL_POLL_MS = 100;
const AUTH_URL_TIMEOUT_MS = 20000;

async function waitForAuthUrl(timeoutMs = AUTH_URL_TIMEOUT_MS) {
  const deadline = Date.now() + timeoutMs;
  while (!state.authUrl && Date.now() < deadline) await sleep(AUTH_URL_POLL_MS);
  return state.authUrl;
}

module.exports = { sleep, stderrTail, pushErr, portOpen, waitForPort, waitForAuthUrl,
  STDERR_TAIL_LINES, PORT_OPEN_TIMEOUT_MS, PORT_POLL_MS, AUTH_URL_POLL_MS, AUTH_URL_TIMEOUT_MS };
