#!/opt/node22/bin/node
// Драйвер окна лаунчера через CDP (--remote-debugging-port=9222).
//   driver.js text                     — текст текущей страницы
//   driver.js url                      — URL текущей страницы
//   driver.js wait-text <re> <sec>     — ждать текст страницы по regexp
//   driver.js wait-url <re> <sec>      — ждать URL страницы по regexp
//   driver.js eval <js>                — выполнить JS на странице
const CDP = 'http://127.0.0.1:9222';
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function page() {
  const list = await (await fetch(`${CDP}/json/list`, { signal: AbortSignal.timeout(5000) })).json();
  return list.find((t) => t.type === 'page') || null;
}

function pageText(url) {
  if (!url.startsWith('data:')) return url;
  const html = decodeURIComponent(url.slice(url.indexOf(',') + 1));
  return html.replace(/<style[\s\S]*?<\/style>/g, '').replace(/<[^>]+>/g, ' ')
    .replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&')
    .replace(/\s+/g, ' ').trim();
}

async function current() {
  try { const p = await page(); return p ? p.url : ''; } catch { return ''; }
}

async function evalJs(expr) {
  const p = await page();
  const ws = new WebSocket(p.webSocketDebuggerUrl);
  // Страница может как раз перезагружаться — тогда соединение не открывается
  // и не падает; не ждём его вечно.
  await new Promise((res, rej) => {
    ws.onopen = res;
    ws.onerror = rej;
    setTimeout(() => rej(new Error('CDP: WebSocket не открылся за 5 с')), 5000);
  });
  ws.send(JSON.stringify({ id: 1, method: 'Runtime.evaluate', params: { expression: expr, returnByValue: true } }));
  const r = await new Promise((res) => {
    ws.onmessage = (m) => { const d = JSON.parse(m.data); if (d.id === 1) res(d); };
    setTimeout(() => res(null), 3000);
  });
  try { ws.close(); } catch { /* noop */ }
  return r && r.result && r.result.result ? r.result.result.value : undefined;
}

async function waitFor(get, re, sec) {
  const rx = new RegExp(re);
  const deadline = Date.now() + sec * 1000;
  let last = '';
  while (Date.now() < deadline) {
    last = await get();
    if (rx.test(last)) { console.log(last.slice(0, 400)); return; }
    await sleep(500);
  }
  console.log(`TIMEOUT waiting /${re}/; last: ${last.slice(0, 600)}`);
  process.exit(1);
}

(async () => {
  const [cmd, a, b] = process.argv.slice(2);
  // Страховка от любого зависания: команда не живёт дольше своего таймаута + 20 с.
  const limit = (cmd === 'wait-text' || cmd === 'wait-url' ? Number(b || 30) : 10) + 20;
  setTimeout(() => { console.error(`driver: команда ${cmd} зависла (> ${limit} с)`); process.exit(1); }, limit * 1000).unref();
  if (cmd === 'text') console.log(pageText(await current()));
  else if (cmd === 'url') console.log(await current());
  else if (cmd === 'wait-text') await waitFor(async () => pageText(await current()), a, Number(b || 30));
  else if (cmd === 'wait-url') await waitFor(current, a, Number(b || 30));
  else if (cmd === 'eval') console.log(await evalJs(a));
  else { console.error('unknown command'); process.exit(2); }
})().catch((e) => { console.error('driver error:', e.message); process.exit(1); });
