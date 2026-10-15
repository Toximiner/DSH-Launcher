'use strict';
// Обновления — HTTP-слой: запросы через http/https из Node, а не
// electronNet.fetch. После electronNet.fetch к внешним хостам при старте
// следующий запуск дочернего процесса (dpkg-query, dsh --version в «О
// программе») ронял Electron 44 в нативном коде: «terminate called without
// an active exception» / SIGSEGV (Wayland, Ubuntu 26.04; 1.3.0).
// Системный прокси берём у Chromium (session.resolveProxy — настройки
// GNOME/KDE и переменные окружения) и передаём агенту Node (proxyEnv,
// Node 24.5+). (Проверка куки окна — plainUrlIsAuthed — остаётся на
// electronNet: ей нужна сессия окна.)
const { session } = require('electron');
const http = require('http');
const https = require('https');
const { proxyUrlFromRule } = require('./updates-logic');

// Агент Node через прокси (правило Chromium; SOCKS агент не умеет —
// см. proxyUrlFromRule).
async function proxyAgentFor(u) {
  let rule = 'DIRECT';
  try { rule = await session.defaultSession.resolveProxy(u.toString()); } catch { /* напрямую */ }
  const proxy = proxyUrlFromRule(rule);
  if (!proxy) return undefined;
  const isHttp = u.protocol === 'http:';
  return new (isHttp ? http : https).Agent({ proxyEnv: { [isHttp ? 'HTTP_PROXY' : 'HTTPS_PROXY']: proxy } });
}

/* ===== httpGet (тест test/http.js, с подставным агентом) =====
   GET с переходами по редиректам (ассеты релизов GitHub отдаются через 302)
   → поток ответа (http.IncomingMessage): statusCode, headers, данные.
   Authorization — только в пределах исходного адреса (origin). */
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
    // токен — только тому адресу, которому его давали: на другой хост, порт
    // или схему заголовок Authorization не уходит (как у curl и браузеров)
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
