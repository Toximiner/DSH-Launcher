#!/usr/bin/env node
// Unit test of httpGet (main.js): following redirects (GitHub release
// assets are served via 302), loop protection, a timeout and a request
// through a proxy (the agent from proxyAgentFor). Everything on local
// servers, no network.
'use strict';
const assert = require('assert');
const http = require('http');
const https = require('https');
const { extract } = require('./_extract');

const listen = (srv) => new Promise((r) => srv.listen(0, '127.0.0.1', () => r(srv.address().port)));
const text = async (res) => { let b = ''; res.setEncoding('utf8'); for await (const c of res) b += c; return b; };

(async () => {
  // The target server: /final — the answer, /r/N — a chain of redirects, /loop — a loop, /hang — silent
  const hits = [];
  const target = http.createServer((q, s) => {
    hits.push(q.url);
    if (q.url === '/final') { s.end('ok:' + (q.headers['user-agent'] || '')); return; }
    const m = q.url.match(/^\/r\/(\d+)$/);
    if (m) { const n = Number(m[1]); s.writeHead(302, { location: n > 1 ? `/r/${n - 1}` : '/final' }); s.end(); return; }
    if (q.url === '/loop') { s.writeHead(301, { location: '/loop' }); s.end(); return; }
    if (q.url === '/hang') return; // we don't answer
    if (q.url === '/auth') { s.end('auth:' + (q.headers.authorization || '-')); return; }
    if (q.url === '/to-auth') { s.writeHead(302, { location: '/auth' }); s.end(); return; }
    if (q.url === '/cross') { s.writeHead(302, { location: `${otherBase}/auth` }); s.end(); return; }
    s.writeHead(404); s.end('nope');
  });
  // Another server (another port — another origin): what arrived in Authorization
  const other = http.createServer((q, s) => { s.end('auth:' + (q.headers.authorization || '-')); });
  const otherBase = `http://127.0.0.1:${await listen(other)}`;
  const tport = await listen(target);
  const base = `http://127.0.0.1:${tport}`;

  // Without a proxy
  const direct = extract('httpGet', { http, https, proxyAgentFor: async () => undefined });
  let res = await direct(`${base}/final`, { headers: { 'User-Agent': 'dsh-launcher' } });
  assert.strictEqual(res.statusCode, 200, 'direct request');
  assert.strictEqual(await text(res), 'ok:dsh-launcher', 'headers are passed through');

  res = await direct(`${base}/r/3`, { headers: { 'User-Agent': 'x' } });
  assert.strictEqual(await text(res), 'ok:x', 'chain of 3 redirects, headers preserved');

  res = await direct(`${base}/missing`);
  assert.strictEqual(res.statusCode, 404, 'the error status is passed to the caller');
  await text(res);

  // The token — only to the original address: a redirect to the same origin — with it, to another — without it
  const auth = { 'User-Agent': 'x', Authorization: 'Bearer t0ken' };
  assert.strictEqual(await text(await direct(`${base}/to-auth`, { headers: auth })), 'auth:Bearer t0ken', 'same origin — with the token');
  assert.strictEqual(await text(await direct(`${base}/cross`, { headers: auth })), 'auth:-', 'different origin — without the token');
  assert.strictEqual(auth.Authorization, 'Bearer t0ken', 'the caller’s headers are not mutated');

  await assert.rejects(direct(`${base}/loop`), /слишком много перенаправлений/, 'redirect loop → error');
  await assert.rejects(direct(`${base}/hang`, { timeoutMs: 200 }), /таймаут/, 'no response → timeout');

  // Through a proxy: a Node agent with proxyEnv sends the request to the
  // proxy (for http — an absolute URI), the proxy forwards it to the
  // target server. proxyEnv — since Node 24.5; in the launcher Node is
  // from Electron 44 (24.x), and the CI unit tests run on Node 22 — we
  // skip this part there.
  const [maj, min] = process.versions.node.split('.').map(Number);
  if (maj < 24 || (maj === 24 && min < 5)) {
    console.log(`  (proxy: skipped — Node ${process.versions.node} has no proxyEnv, need 24.5+)`);
    other.closeAllConnections(); other.close(); target.closeAllConnections(); target.close();
    console.log('OK: httpGet — all checks passed');
    return;
  }
  let proxied = 0;
  const proxy = http.createServer((q, s) => {
    proxied++;
    const u = new URL(q.url);
    http.get(u, { headers: q.headers }, (r) => { s.writeHead(r.statusCode, r.headers); r.pipe(s); })
      .on('error', () => { s.writeHead(502); s.end(); });
  });
  const pport = await listen(proxy);
  const viaProxy = extract('httpGet', {
    http, https,
    proxyAgentFor: async (u) => new http.Agent({ proxyEnv: { HTTP_PROXY: `http://127.0.0.1:${pport}` } }),
  });
  res = await viaProxy(`${base}/r/1`, { headers: { 'User-Agent': 'p' } });
  assert.strictEqual(await text(res), 'ok:p', 'through the proxy, with a redirect');
  assert.strictEqual(proxied, 2, 'both requests (the redirect and the target) went through the proxy');

  other.closeAllConnections(); other.close(); target.closeAllConnections(); target.close(); proxy.close();
  console.log('OK: httpGet — all checks passed');
})().catch((e) => { console.error(e); process.exit(1); });
