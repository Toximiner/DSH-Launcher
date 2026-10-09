#!/opt/node22/bin/node
// Драйвер окна лаунчера через CDP (--remote-debugging-port=9222).
//   driver.js text                     — текст текущей страницы
//   driver.js url                      — URL текущей страницы
//   driver.js wait-text <re> <sec>     — ждать текст страницы по regexp
//   driver.js wait-url <re> <sec>      — ждать URL страницы по regexp
//   driver.js eval <js>                — выполнить JS на странице
//   driver.js rclick <x> <y>           — правый клик мышью в точке окна (контекстное меню)
//   driver.js key <Ctrl+KeyF | F3 | …> — нажатие клавиши (Input.dispatchKeyEvent; код по KeyboardEvent.code)
//   driver.js --page <re> <команда …>  — в окне, чей заголовок или URL подходит под <re>
//                                        (без --page — первое окно в списке CDP)
//   driver.js pages                    — окна: заголовок | URL
const CDP = process.env.DRV_CDP || 'http://127.0.0.1:9222'; // DRV_CDP — другой порт отладки
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

let PAGE_RE = null; // --page <re>: окно по заголовку или URL

async function pages() {
  const list = await (await fetch(`${CDP}/json/list`, { signal: AbortSignal.timeout(5000) })).json();
  return list.filter((t) => t.type === 'page');
}

async function page() {
  const list = await pages();
  return (PAGE_RE ? list.find((t) => PAGE_RE.test(t.title) || PAGE_RE.test(t.url)) : list[0]) || null;
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

// Вызвать один или несколько методов CDP на странице по очереди; результат
// последнего (или null, если ответа не было за 3 с).
async function cdp(calls) {
  const p = await page();
  const ws = new WebSocket(p.webSocketDebuggerUrl);
  // Страница может как раз перезагружаться — тогда соединение не открывается
  // и не падает; не ждём его вечно.
  await new Promise((res, rej) => {
    ws.onopen = res;
    ws.onerror = rej;
    setTimeout(() => rej(new Error('CDP: WebSocket не открылся за 5 с')), 5000);
  });
  let r = null;
  for (let i = 0; i < calls.length; i++) {
    const id = i + 1;
    ws.send(JSON.stringify({ id, method: calls[i][0], params: calls[i][1] }));
    r = await new Promise((res) => {
      ws.onmessage = (m) => { const d = JSON.parse(m.data); if (d.id === id) res(d); };
      setTimeout(() => res(null), 3000);
    });
  }
  try { ws.close(); } catch { /* noop */ }
  return r;
}

async function evalJs(expr) {
  const r = await cdp([['Runtime.evaluate', { expression: expr, returnByValue: true }]]);
  return r && r.result && r.result.result ? r.result.result.value : undefined;
}

// Нажатие клавиши: «Ctrl+Shift+KeyF», «F3», «Escape». Последняя часть —
// KeyboardEvent.code; key и windowsVirtualKeyCode выводим из него.
async function pressKey(spec) {
  const parts = spec.split('+');
  const code = parts.pop();
  const mods = { Alt: 1, Ctrl: 2, Meta: 4, Shift: 8 };
  const modifiers = parts.reduce((m, p) => m | (mods[p] || 0), 0);
  let key = code; let vk = 0;
  if (/^Key[A-Z]$/.test(code)) { key = code.slice(3).toLowerCase(); vk = code.charCodeAt(3); }
  else if (/^Digit\d$/.test(code)) { key = code.slice(5); vk = code.charCodeAt(5); }
  else if (/^F\d{1,2}$/.test(code)) { vk = 111 + Number(code.slice(1)); }
  else vk = { Escape: 27, Enter: 13 }[code] || 0;
  const ev = (type) => ['Input.dispatchKeyEvent', { type, modifiers, code, key, windowsVirtualKeyCode: vk, nativeVirtualKeyCode: vk }];
  const r = await cdp([ev('rawKeyDown'), ev('keyUp')]);
  if (!r || r.error) throw new Error('CDP: нажатие не прошло' + (r && r.error ? ': ' + r.error.message : ''));
}

// Настоящее событие мыши (Input.dispatchMouseEvent): Chromium обрабатывает
// его как клик пользователя — для правой кнопки приходит context-menu.
async function rightClick(x, y) {
  const ev = (type) => ['Input.dispatchMouseEvent', { type, x, y, button: 'right', buttons: 2, clickCount: 1 }];
  const r = await cdp([['Input.dispatchMouseEvent', { type: 'mouseMoved', x, y }], ev('mousePressed'), ev('mouseReleased')]);
  if (!r || r.error) throw new Error('CDP: правый клик не прошёл' + (r && r.error ? ': ' + r.error.message : ''));
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
  let args = process.argv.slice(2);
  if (args[0] === '--page') { PAGE_RE = new RegExp(args[1]); args = args.slice(2); }
  const [cmd, a, b] = args;
  // Страховка от любого зависания: команда не живёт дольше своего таймаута + 20 с.
  const limit = (cmd === 'wait-text' || cmd === 'wait-url' ? Number(b || 30) : 10) + 20;
  setTimeout(() => { console.error(`driver: команда ${cmd} зависла (> ${limit} с)`); process.exit(1); }, limit * 1000).unref();
  if (cmd === 'text') console.log(pageText(await current()));
  else if (cmd === 'url') console.log(await current());
  else if (cmd === 'wait-text') await waitFor(async () => pageText(await current()), a, Number(b || 30));
  else if (cmd === 'wait-url') await waitFor(current, a, Number(b || 30));
  else if (cmd === 'eval') console.log(await evalJs(a));
  else if (cmd === 'rclick') await rightClick(Number(a), Number(b));
  else if (cmd === 'key') await pressKey(a);
  else if (cmd === 'pages') for (const t of await pages()) console.log(`${t.title} | ${t.url.slice(0, 80)}`);
  else { console.error('unknown command'); process.exit(2); }
})().catch((e) => { console.error('driver error:', e.message); process.exit(1); });
