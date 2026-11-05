'use strict';
// Updates — the HTTP layer: requests via Node's http/https, not
// electronNet.fetch. After electronNet.fetch to external hosts at start,
// the next child process launch (dpkg-query, dsh --version in "About")
// crashed Electron 44 in native code: "terminate called without
// an active exception" / SIGSEGV (Wayland, Ubuntu 26.04; 1.3.0).
// We take the system proxy from Chromium (session.resolveProxy — GNOME/KDE
// settings and environment variables) and pass it to the Node agent
// (proxyEnv, Node 24.5+). (The window cookie check — plainUrlIsAuthed —
// stays on electronNet: it needs the window's session.)
const { session } = require('electron');
const http = require('http');
const https = require('https');
const { proxyUrlFromRule } = require('./updates-logic');

// The Node agent through a proxy (the Chromium rule; the agent cannot do
// SOCKS — see proxyUrlFromRule).
async function proxyAgentFor(u) {
  let rule = 'DIRECT';
  try { rule = await session.defaultSession.resolveProxy(u.toString()); } catch { /* direct */ }
  const proxy = proxyUrlFromRule(rule);
  if (!proxy) return undefined;
  const isHttp = u.protocol === 'http:';
  return new (isHttp ? http : https).Agent({ proxyEnv: { [isHttp ? 'HTTP_PROXY' : 'HTTPS_PROXY']: proxy } });
}

/* ===== httpGet (test test/http.js, with a stub agent) =====
   GET with redirects (GitHub release assets are served via 302)
   → the response stream (http.IncomingMessage): statusCode, headers, data.
   Authorization — only within the original address (origin). */
async function httpGet(url, { headers = {}, maxRedirects = 5, timeoutMs = 30000 } = {}) {
  const u = new URL(url);
  const agent = await proxyAgentFor(u);
  const res = await new Promise((resolve, reject) => {
    const req = (u.protocol === 'http:' ? http : https).get(u, { headers, agent }, resolve);
    req.on('error', reject);
    req.setTimeout(timeoutMs, () => req.destroy(new Error(`таймаут ${timeoutMs / 1000} с: ${u.host}`)));
  });
  if ([301, 302, 303, 307, 308].includes(res.statusCode) && res.headers.location) {
    res.resume();
    if (maxRedirects <= 0) throw new Error('слишком много перенаправлений');
    const next = new URL(res.headers.location, u);
    // the token — only to the address it was given to: to another host,
    // port or scheme the Authorization header does not go (as with curl
    // and browsers)
    const nextHeaders = next.origin === u.origin ? headers
      : Object.fromEntries(Object.entries(headers).filter(([k]) => k.toLowerCase() !== 'authorization'));
    return httpGet(next.toString(), { headers: nextHeaders, maxRedirects: maxRedirects - 1, timeoutMs });
  }
  return res;
}
/* ===== end httpGet ===== */

async function httpJson(url, headers, what) {
  const res = await httpGet(url, { headers });
  let body = '';
  res.setEncoding('utf8');
  for await (const c of res) body += c;
  if (res.statusCode !== 200) throw new Error(`${what}: HTTP ${res.statusCode}`);
  return JSON.parse(body);
}

module.exports = { proxyAgentFor, httpGet, httpJson };
