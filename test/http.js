#!/usr/bin/env node
// Юнит-тест httpGet (main.js): переходы по редиректам (ассеты релизов GitHub
// отдаются через 302), защита от петли, таймаут и запрос через прокси (агент
// от proxyAgentFor). Всё на локальных серверах, без сети.
'use strict';
const assert = require('assert');
const http = require('http');
const https = require('https');
const { extract } = require('./_extract');

const listen = (srv) => new Promise((r) => srv.listen(0, '127.0.0.1', () => r(srv.address().port)));
const text = async (res) => { let b = ''; res.setEncoding('utf8'); for await (const c of res) b += c; return b; };

(async () => {
  // Целевой сервер: /final — ответ, /r/N — цепочка редиректов, /loop — петля, /hang — молчит
  const hits = [];
  const target = http.createServer((q, s) => {
    hits.push(q.url);
    if (q.url === '/final') { s.end('ok:' + (q.headers['user-agent'] || '')); return; }
    const m = q.url.match(/^\/r\/(\d+)$/);
    if (m) { const n = Number(m[1]); s.writeHead(302, { location: n > 1 ? `/r/${n - 1}` : '/final' }); s.end(); return; }
    if (q.url === '/loop') { s.writeHead(301, { location: '/loop' }); s.end(); return; }
    if (q.url === '/hang') return; // не отвечаем
    s.writeHead(404); s.end('nope');
  });
  const tport = await listen(target);
  const base = `http://127.0.0.1:${tport}`;

  // Без прокси
  const direct = extract('httpGet', { http, https, proxyAgentFor: async () => undefined });
  let res = await direct(`${base}/final`, { headers: { 'User-Agent': 'dsh-launcher' } });
  assert.strictEqual(res.statusCode, 200, 'прямой запрос');
  assert.strictEqual(await text(res), 'ok:dsh-launcher', 'заголовки передаются');

  res = await direct(`${base}/r/3`, { headers: { 'User-Agent': 'x' } });
  assert.strictEqual(await text(res), 'ok:x', 'цепочка из 3 редиректов, заголовки сохраняются');

  res = await direct(`${base}/missing`);
  assert.strictEqual(res.statusCode, 404, 'код ошибки отдаётся вызывающему');
  await text(res);

  await assert.rejects(direct(`${base}/loop`), /слишком много перенаправлений/, 'петля редиректов → ошибка');
  await assert.rejects(direct(`${base}/hang`, { timeoutMs: 200 }), /таймаут/, 'нет ответа → таймаут');

  // Через прокси: агент Node с proxyEnv шлёт запрос на прокси (для http —
  // абсолютным URI), прокси пересылает его целевому серверу. proxyEnv — с
  // Node 24.5; в лаунчере Node из Electron 44 (24.x), а юнит-тесты CI идут
  // на Node 22 — там эту часть пропускаем.
  const [maj, min] = process.versions.node.split('.').map(Number);
  if (maj < 24 || (maj === 24 && min < 5)) {
    console.log(`  (прокси: пропущено — Node ${process.versions.node} без proxyEnv, нужен 24.5+)`);
    target.closeAllConnections(); target.close();
    console.log('OK: httpGet — все проверки прошли');
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
  assert.strictEqual(await text(res), 'ok:p', 'через прокси, с редиректом');
  assert.strictEqual(proxied, 2, 'оба запроса (редирект и цель) прошли через прокси');

  target.closeAllConnections(); target.close(); proxy.close();
  console.log('OK: httpGet — все проверки прошли');
})().catch((e) => { console.error(e); process.exit(1); });
