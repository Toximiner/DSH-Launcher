'use strict';
// Служебные страницы в главном окне: общий вид, статусы, страница вставки токена.
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc, withRouteKey, CSP_META } = require('./util');
const { prefFile, readPref, writePref } = require('./prefs');
const { PLAIN_URL, LANG_FORCED, ROUTE_KEY } = require('./env');
const { stderrTail } = require('./port'); // чистый модуль — прямой require, без реестра
const menus = require('./menus');
const { blankResponse } = require('./respond'); // чистый модуль — прямой require, без реестра

/* ---------------- страницы-статусы ---------------- */



const PAGE_CSS = `
  html,body{margin:0;height:100%;background:#0f1115;color:#d7dce3;
    font-family:system-ui,'Segoe UI',Roboto,Ubuntu,sans-serif}
  .wrap{max-width:700px;margin:0 auto;padding:12vh 24px 24px}
  h1{font-size:20px;font-weight:600;margin:0 0 14px}
  p{line-height:1.6;color:#9aa4b2;margin:0 0 12px}
  code{background:#1a2130;padding:2px 6px;border-radius:6px;font-size:0.9em;color:#c7d2fe}
  pre{background:#151a23;border:1px solid #232b3a;border-radius:10px;padding:12px 14px;
    font-size:12px;line-height:1.5;overflow:auto;max-height:34vh;white-space:pre-wrap;
    word-break:break-word;color:#c7cedb}
  input:not([type=checkbox]):not([type=radio]){width:100%;box-sizing:border-box;background:#151a23;
    border:1px solid #2a3550;border-radius:10px;color:#e6ebf4;padding:12px 14px;font-size:14px;margin-bottom:12px}
  input:focus{outline:none;border-color:#3b82f6}
  button{background:#3b82f6;border:0;border-radius:10px;color:#fff;padding:12px 22px;
    font-size:14px;cursor:pointer}
  button:hover{background:#2f6fd8}
  .dot{display:inline-block;width:10px;height:10px;border-radius:50%;background:#3fb950;margin-right:9px}
  .err .dot{background:#f85149}
  .lang{position:fixed;top:14px;right:18px;font-size:13px;color:#4b5567;user-select:none}
  .lang a{color:#6b7689;text-decoration:none;padding:2px 4px;border-radius:5px}
  .lang a:hover{color:#d7dce3}
  .lang a.on{color:#d7dce3;font-weight:600;pointer-events:none}
  ::-webkit-scrollbar{width:8px;height:8px}
  ::-webkit-scrollbar-track{background:transparent}
  ::-webkit-scrollbar-thumb{background:#2a3550;border-radius:8px;border:2px solid transparent;background-clip:padding-box}
  ::-webkit-scrollbar-thumb:hover{background:#3b4a6b;background-clip:padding-box}
  ::-webkit-scrollbar-corner{background:transparent}
  .notes{max-height:34vh;overflow:auto;background:#151a23;border:1px solid #232b3a;border-radius:10px;
    padding:4px 16px 8px;margin-bottom:14px}
  .notes .ver{margin:12px 0 2px}
  .notes h4{margin:10px 0 4px;font-size:13px;color:#9aa4b2;font-weight:600}
  .notes ul{margin:4px 0 8px;padding-left:20px}
  .notes li{line-height:1.5;color:#c7cedb;margin:3px 0}
  .notes p{margin:4px 0 8px}
  .notes a{color:#7aa2f7}
  .dlg .wrap{max-width:none;padding:20px 24px;height:100%;box-sizing:border-box;
    display:flex;flex-direction:column}
  /* диалог — слой внутри главного окна: затемнение и карточка по центру */
  html.ovl,html.ovl body{background:transparent}
  html.ovl body{margin:0;height:100vh;display:flex;align-items:center;justify-content:center;
    background:rgba(0,0,0,.55);animation:ovlin .12s ease-out}
  /* всплывающее «Сохранено» и ошибки — в углу карточки, а не всего окна */
  html.ovl .wrap{position:relative}
  html.ovl .dlg .pop,html.ovl .pop{position:absolute}
  html.ovl .wrap{width:min(760px,calc(100vw - 48px));height:min(580px,calc(100vh - 48px));
    background:#0f1115;border:1px solid #2a3550;border-radius:14px;box-shadow:0 18px 60px rgba(0,0,0,.6)}
  @keyframes ovlin{from{opacity:0}to{opacity:1}}
  .dlg .notes{flex:1 1 auto;max-height:none;min-height:80px;margin-bottom:0}
  .dlg pre.log{flex:1 1 auto;max-height:none;min-height:80px;margin:0;white-space:pre-wrap}
  .dlg .form{flex:1 1 auto;overflow:auto;min-height:0;padding-right:6px}
  .dlg .form h4{margin:14px 0 6px;font-size:13px;color:#d7dce3;font-weight:600}
  .dlg .form h4:first-child{margin-top:0}
  .dlg .row{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin:6px 0;color:#9aa4b2}
  .dlg .row > span:first-child{min-width:150px}
  .dlg .row input:not([type=checkbox]),.dlg .row select{width:auto;margin:0;padding:6px 9px;font-size:13px;
    background:#151a23;border:1px solid #2a3550;border-radius:7px;color:#e6ebf4}
  .dlg .row input:disabled,.dlg .row select:disabled{opacity:.55}
  .dlg .row button{padding:6px 12px;font-size:13px}
  .dlg .row code.val{max-width:360px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .dlg .sett{display:flex;gap:22px;align-items:flex-start}
  .dlg .sett nav{display:flex;flex-direction:column;gap:2px;min-width:150px;position:sticky;top:0}
  .dlg .sett nav a{padding:7px 12px;border-radius:8px;color:#9aa4b2;cursor:pointer;text-decoration:none;white-space:nowrap}
  .dlg .sett nav a:hover{background:#151a23;color:#d7dce3}
  .dlg .sett nav a.on{background:#1f2836;color:#e6ebf4;font-weight:600}
  .dlg .sett .panes{flex:1;min-width:0}
  .dlg .sett section[hidden]{display:none}
  .dlg .chk{display:inline-flex;align-items:center;gap:8px;cursor:pointer;line-height:1.4}
  .dlg .chk input{width:16px;height:16px;margin:0;flex:none;accent-color:#3b82f6;cursor:pointer}
  .dlg .hint{font-size:12px;color:#6b7689}
  .dlg .banner{background:#2a2410;border:1px solid #6b5a1e;color:#e5c76b;border-radius:8px;padding:8px 12px;
    margin-bottom:10px;display:flex;align-items:center;gap:12px;flex-wrap:wrap}
  .dlg .note{margin:0 0 8px;color:#3fb950}
  .dlg .note.err{color:#f85149}
  /* «Сохранено» / ошибка в окне «Настройки» — поверх, в углу над кнопками:
     не сдвигает поля. «Сохранено» гаснет само, ошибка остаётся. */
  .dlg .pop{position:fixed;right:22px;bottom:74px;margin:0;padding:8px 14px;border-radius:9px;z-index:5;
    background:#14251a;border:1px solid #2f6b3d;color:#3fb950;box-shadow:0 6px 20px rgba(0,0,0,.45);
    pointer-events:none;animation:popfade 2.6s ease forwards}
  .dlg .pop.err{background:#2a1517;border-color:#7f2a2a;color:#f85149;animation:popin .15s ease}
  @keyframes popfade{0%{opacity:0;transform:translateY(6px)}8%{opacity:1;transform:none}75%{opacity:1}100%{opacity:0}}
  @keyframes popin{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}
  .dlg .actions{margin-top:auto;padding-top:14px;display:flex;gap:10px;flex-wrap:wrap}
  .dlg h1{font-size:18px}
  .dlg button{padding:9px 18px}
`;

/* ===== pageHtml (чистая функция, тест test/pages.js) ===== */
// opts.dialog — для окна-диалога: без переключателя языка, компактно.
function pageHtml(title, body, isError, opts = {}) {
  const langLink = (code, label) =>
    `<a href="dshlauncher://lang/${code}/"${i18n.getLang() === code ? ' class="on"' : ''}>${label}</a>`;
  const langSwitch = opts.dialog ? '' : `<div class="lang">${langLink('ru', 'RU')} | ${langLink('en', 'EN')}</div>`;
  const html = `<!doctype html><html lang="${i18n.getLang()}"${opts.dialog ? ' class="ovl"' : ''}><head><meta charset="utf-8">${CSP_META}<title>${esc(title)}</title><style>${PAGE_CSS}</style></head><body${opts.dialog ? ' class="dlg"' : ''}>${langSwitch}<div class="wrap${isError ? ' err' : ''}"><h1><span class="dot"></span>${esc(title)}</h1>${body}</div></body></html>`;
  return 'data:text/html;charset=utf-8,' + encodeURIComponent(withRouteKey(html, ROUTE_KEY));
}
/* ===== end pageHtml ===== */

function stderrBlock() {
  return `<pre>${esc(stderrTail.slice(-50).join('\n') || tr('нет данных журнала', 'no log data'))}</pre>`;
}

// Текущая служебная страница как функция отрисовки: переключатель RU | EN
// перерисовывает её на новом языке. Каждая show*-страница регистрирует себя
// сама; разовые showStatus оборачиваются в showPage.


function showPage(render) {
  state.currentPage = render;
  render();
}

const LANG_PREFS = 'ui-lang.json';
const langPrefFile = () => prefFile(LANG_PREFS);

function loadLangPref() {
  if (LANG_FORCED) return;
  const p = readPref(LANG_PREFS); // выбора ещё не было — остаётся язык локали
  if (p && (p.lang === 'ru' || p.lang === 'en')) i18n.setLang(p.lang);
}

function onLangSwitch(lang) {
  if ((lang === 'ru' || lang === 'en') && lang !== i18n.getLang()) {
    i18n.setLang(lang);
    writePref(LANG_PREFS, { lang });
    console.log(`[launcher] язык окна: ${lang}`);
  }
  menus.buildAppMenu(); // подписи меню на новом языке
  if (state.currentPage) state.currentPage();
  return blankResponse();
}

// Загрузка в окно. ERR_ABORTED (-3) — не ошибка: эту навигацию сменила
// более новая (статус-страница, watchdog), окно уже показывает её.
async function loadInWin(url) {
  if (!state.win || state.win.isDestroyed()) return;
  try {
    await state.win.loadURL(url);
  } catch (e) {
    if (e.code === 'ERR_ABORTED' || e.errno === -3) {
      console.log(`[launcher] загрузка ${String(url).slice(0, 40)}… прервана более новой навигацией`);
    } else {
      throw e;
    }
  }
}

function showStatus(title, body, isError) {
  loadInWin(pageHtml(title, body, !!isError)).catch((e) => console.error('[launcher] showStatus:', e.message));
}

function showPastePage() {
  state.currentPage = showPastePage;
  const body = `
    ${tr(`<p>Похоже, <code>dsh</code> уже запущен, но у окна нет валидного сессионного куки
       (он мог истечь или dsh был перезапущен с другим секретом).</p>
    <p>В терминале, где работает dsh, найдите строку
       <code>dsh web: ${PLAIN_URL}?token=…</code> и вставьте сюда полный URL
       (или только токен):</p>`,
    `<p>It looks like <code>dsh</code> is already running, but the window has no valid
       session cookie (it may have expired, or dsh was restarted with a different secret).</p>
    <p>In the terminal where dsh is running, find the line
       <code>dsh web: ${PLAIN_URL}?token=…</code> and paste the full URL here
       (or just the token):</p>`)}
    <input id="u" type="text" placeholder="${PLAIN_URL}?token=…" autocomplete="off" spellcheck="false">
    <button onclick="go()">${tr('Открыть', 'Open')}</button>
    ${tr(`<p style="margin-top:16px">Токен взять неоткуда (dsh запущен сервисом, терминала нет)?
        Тогда остановите dsh (например, <code>pkill -f 'dsh --profile web'</code>), закройте
        это окно и запустите <code>dsh-launcher</code> снова — лаунчер поднимет dsh сам и
        подключится без токена.</p>`,
    `<p style="margin-top:16px">No way to get the token (dsh runs as a service, no terminal)?
        Then stop dsh (e.g. <code>pkill -f 'dsh --profile web'</code>), close this
        window and start <code>dsh-launcher</code> again — the launcher will start dsh itself
        and connect without a token.</p>`)}
    <script>
      const el = document.getElementById('u');
      el.focus();
      el.addEventListener('keydown', (e) => { if (e.key === 'Enter') go(); });
      function go() {
        const v = el.value.trim();
        if (!v) return;
        const url = /^https?:\\/\\//i.test(v) ? v : '${PLAIN_URL}?token=' + encodeURIComponent(v);
        location.href = 'dshlauncher://paste/' + encodeURIComponent(url);
      }
    </script>`;
  showStatus(tr('Нужен токен', 'Token required'), body, true);
}

/* ---------------- живой лог на странице-статусе ----------------
 * Страницы «выполняется» раньше перерисовывались целиком каждые 800 мс
 * (новый data: URL) — скролл сбрасывался наверх, и лог нельзя было читать.
 * Теперь первая отрисовка — обычная страница с <pre id="live-log">, а
 * последующие — через executeJavaScript: строки только дописываются, и если
 * пользователь прижат ко дну, остаётся прижатым, иначе скролл не трогают.
 * Если страница уже не живая (ушли по переходу, перегрузили) — false:
 * вызывающий делает полную отрисовку, которая сбрасывает счётчик.
 */
const livePushed = new Map(); // st → logSeq последней строки, уже на странице

/* ===== liveLogAppend (чистая функция, тест test/live-log.js) ===== */
// Страницная функция: дописать строки в <pre id="live-log">. Лимит 500 —
// как LOG_MAX_LINES в proc-log.js: при превышении старые строки уходят
// сверху. false — на странице нет такого <pre> (уже не наша страница).
function liveLogAppend(document, lines) {
  const pre = document.getElementById('live-log');
  if (!pre) return false;
  const pinned = pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 8;
  if (lines.length) {
    let t = pre.textContent;
    if (t.trim() === '…') t = '';
    t = t ? t + '\n' + lines.join('\n') : lines.join('\n');
    const ls = t.split('\n');
    if (ls.length > 500) t = ls.slice(ls.length - 500).join('\n');
    pre.textContent = t;
  }
  if (pinned) pre.scrollTop = pre.scrollHeight;
  return true;
}
/* ===== end liveLogAppend ===== */

/* ===== liveLogFresh (чистая функция, тест test/live-log.js) ===== */
// Строки, которых ещё нет на странице: с logSeq = shown+1 по st.logSeq.
// null — ничего нового. st.log хранит только последние 500 строк
// (proc-log), поэтому индекс считается от logSeq; строки, ушедшие из окна
// раньше, чем их успели показать, не восстанавливаются (редко: лог > 500).
function liveLogFresh(st, shown) {
  const seq = st.logSeq || 0;
  if (seq === shown) return null;
  return st.log.slice(Math.max(0, shown - seq + st.log.length));
}
/* ===== end liveLogFresh ===== */

// Для первой отрисовки страницы «выполняется»: <pre> с текущими строками
// и скриптом дописывания (liveLogAppend — сама функция, целиком в страницу).
function liveLogPre(lines) {
  return `<pre id="live-log">${esc(lines.join('\n') || '…')}</pre>
  <script>window.__liveLog = (lines) => (${liveLogAppend.toString()})(document, lines);</script>`;
}

// После полной отрисовки «выполняется»: на странице все текущие строки.
function liveLogSync(st) {
  livePushed.set(st, st.logSeq || 0);
}

// Дописать новые строки на открытую страницу. true — дописано (или новых
// нет); false — страница уже не живая, вызывающий делает полную отрисовку.
async function liveLogTick(st) {
  if (!state.win || state.win.isDestroyed()) return false;
  const fresh = liveLogFresh(st, livePushed.get(st) || 0);
  if (fresh === null) return true;
  try {
    const ok = await state.win.webContents.executeJavaScript(`window.__liveLog(${JSON.stringify(fresh)})`);
    if (!ok) return false;
    livePushed.set(st, st.logSeq || 0);
    return true;
  } catch {
    return false;
  }
}

Object.assign(module.exports, { pageHtml, stderrBlock, showPage, langPrefFile, loadLangPref, onLangSwitch, loadInWin, showStatus, showPastePage, liveLogPre, liveLogSync, liveLogTick }); // дополняем, а не заменяем: модули подключают друг друга по кругу
