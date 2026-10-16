'use strict';
// «Вид»: перезагрузка, масштаб, поиск по странице, полный экран, орфография, горячие клавиши.
const { app, WebContentsView, session } = require('electron');
const fs = require('fs');
const path = require('path');
const { guardNavigation } = require('./guard');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc, withRouteKey, CSP_META } = require('./util');
const { ROUTE_KEY } = require('./env');
const menus = require('./menus');
const settingsWindow = require('./settings-window');
const updatesLive = require('./updates-live');
const { noContentResponse } = require('./respond'); // чистый модуль — прямой require, без реестра

/* ---------------- «Вид»: перезагрузка, масштаб, поиск, полный экран ---------------- */

// Действия доступны и из меню «Вид», и с клавиатуры (keyAction). Масштаб
// сохраняем в userData, чтобы не сбрасывался между запусками. (Ctrl+колесо
// не поддержано: в свежих Electron wheel-события в main-процесс приходят
// ненадёжно, а перехват колеса в самой странице конфликтует с её
// обработчиками.)
const zoomFile = () => path.join(app.getPath('userData'), 'zoom-level.json');
let zoomSaveTimer = null;

function applyZoom(level, note) {
  if (!state.win || state.win.isDestroyed()) return;
  const z = Math.max(-5, Math.min(5, level));
  state.win.webContents.setZoomLevel(z);
  clearTimeout(zoomSaveTimer);
  zoomSaveTimer = setTimeout(() => {
    try { fs.writeFileSync(zoomFile(), JSON.stringify({ zoomLevel: z })); } catch { /* noop */ }
  }, 400);
  if (note) console.log(`[launcher] ${note}`);
}

const zoomOf = () => (state.win && !state.win.isDestroyed() ? state.win.webContents.getZoomLevel() : 0);
const viewActions = {
  reload: () => { state.win.webContents.reload(); console.log('[launcher] перезагрузка страницы'); },
  reloadHard: () => { state.win.webContents.reloadIgnoringCache(); console.log('[launcher] перезагрузка страницы без кэша'); },
  zoomIn: () => applyZoom(zoomOf() + 0.5),
  zoomOut: () => applyZoom(zoomOf() - 0.5),
  zoomReset: () => applyZoom(0, 'масштаб сброшен'),
  find: () => openFind(),
  findNext: () => findStep(true),
  findPrev: () => findStep(false),
  fullscreen: () => state.win.setFullScreen(!state.win.isFullScreen()),
  settings: () => settingsWindow.showSettingsDialog(),
};



// Поиск по странице (Ctrl+F): панель в правом верхнем углу окна — отдельный
// WebContentsView поверх страницы (не окно: на Wayland его не поставить в
// нужное место). Панель — data:-страница; ввод и кнопки — переходы на
// dshlauncher://find/… (ответ 204: панель остаётся как есть, фокус в поле).
// Счётчик «N из M» — по событию found-in-page главного окна.
const FIND_W = 430;
const FIND_H = 46;

let findText = '';

function findBarUrl() {
  const css = `html,body{margin:0;height:100%;background:#151a23;color:#d7dce3;overflow:hidden;
    font:13px system-ui,'Segoe UI',Roboto,Ubuntu,sans-serif}
    body{display:flex;align-items:center;gap:6px;padding:0 8px;box-sizing:border-box;
    border:1px solid #2a3550;border-radius:10px}
    input{flex:1;min-width:0;background:#0f1115;border:1px solid #2a3550;border-radius:7px;color:#e6ebf4;
    padding:6px 9px;font-size:13px}
    input:focus{outline:none;border-color:#3b82f6}
    #n{min-width:64px;text-align:center;color:#9aa4b2;font-variant-numeric:tabular-nums}
    button{background:#1f2836;border:0;border-radius:7px;color:#d7dce3;width:28px;height:28px;cursor:pointer;font-size:14px}
    button:hover{background:#2a3550}`;
  const go = (r) => `location.href='dshlauncher://find/${r}/'`;
  const html = `<!doctype html><html lang="${i18n.getLang()}"><head><meta charset="utf-8">${CSP_META}<style>${css}</style></head><body>
    <input id="q" placeholder="${esc(tr('Найти на странице', 'Find in page'))}" spellcheck="false" autocomplete="off">
    <span id="n"></span>
    <button title="${esc(tr('Предыдущее (Shift+Enter)', 'Previous (Shift+Enter)'))}" onclick="${go('prev')}">&#8593;</button>
    <button title="${esc(tr('Следующее (Enter)', 'Next (Enter)'))}" onclick="${go('next')}">&#8595;</button>
    <button title="${esc(tr('Закрыть (Esc)', 'Close (Esc)'))}" onclick="${go('close')}">&#10005;</button>
    <script>
      const q = document.getElementById('q');
      q.addEventListener('input', () => { location.href = 'dshlauncher://find/q/?t=' + encodeURIComponent(q.value); });
      q.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') { e.preventDefault(); ${go("' + (e.shiftKey ? 'prev' : 'next') + '")}; }
        else if (e.key === 'Escape') { e.preventDefault(); ${go('close')}; }
      });
      window.setCount = (t) => { document.getElementById('n').textContent = t; };
    </script></body></html>`;
  return 'data:text/html;charset=utf-8,' + encodeURIComponent(withRouteKey(html, ROUTE_KEY));
}

function placeFind() {
  if (!state.findView || !state.win || state.win.isDestroyed()) return;
  const [cw] = state.win.getContentSize();
  state.findView.setBounds({ x: Math.max(0, cw - FIND_W - 18), y: 10, width: Math.min(FIND_W, cw), height: FIND_H });
}

function setFindCount(text) {
  if (!state.findView) return;
  state.findView.webContents.executeJavaScript(`window.setCount && setCount(${JSON.stringify(text)})`).catch(() => {});
}

function openFind() {
  if (!state.win || state.win.isDestroyed() || state.stopping) return;
  if (!state.findView) {
    state.findView = new WebContentsView({ webPreferences: { contextIsolation: true, nodeIntegration: false, spellcheck: false } });
    state.findView.setBackgroundColor('#00000000');
    guardNavigation(state.findView.webContents);
    state.findView.webContents.loadURL(findBarUrl()).catch(() => {});
    state.win.contentView.addChildView(state.findView);
    state.win.on('resize', placeFind);
    state.win.webContents.on('found-in-page', (_e, r) => {
      if (r.finalUpdate) setFindCount(r.matches ? `${r.activeMatchOrdinal} / ${r.matches}` : tr('нет', 'none'));
    });
    // Ушли на другую страницу — старые совпадения неактуальны.
    state.win.webContents.on('did-navigate', () => { if (findText) { findText = ''; closeFind(); } });
  }
  placeFind();
  state.findView.setVisible(true);
  updatesLive.placeToast();
  state.findView.webContents.focus();
  state.findView.webContents.executeJavaScript("document.getElementById('q') && document.getElementById('q').select()").catch(() => {});
}

function findQuery(text) {
  if (!state.win || state.win.isDestroyed()) return;
  findText = text;
  if (!text) { state.win.webContents.stopFindInPage('clearSelection'); setFindCount(''); return; }
  state.win.webContents.findInPage(text, { findNext: true });
}

function findStep(forward) {
  if (!state.findView || !state.findView.getVisible()) return openFind();
  if (findText) state.win.webContents.findInPage(findText, { forward, findNext: false });
}

function closeFind() {
  if (!state.findView || !state.win || state.win.isDestroyed()) return;
  state.win.webContents.stopFindInPage('keepSelection');
  state.findView.setVisible(false);
  updatesLive.placeToast();
  state.win.webContents.focus();
}

function onFindRequest(route, url) {
  if (route.startsWith('/find/q/')) findQuery(new URL(url).searchParams.get('t') || '');
  else if (route.startsWith('/find/next/')) findStep(true);
  else if (route.startsWith('/find/prev/')) findStep(false);
  else if (route.startsWith('/find/close/')) closeFind();
  return noContentResponse();
}

// Проверка орфографии в полях ввода (русский и английский). Словари
// Chromium на Linux скачивает при первом включении (с серверов Google); без
// сети проверка просто не работает. Вкл/выкл — меню «Вид», запоминается.
const spellFile = () => path.join(app.getPath('userData'), 'spellcheck.json');


function initSpellcheck() {
  try { state.spellcheckOn = JSON.parse(fs.readFileSync(spellFile(), 'utf8')).enabled !== false; } catch { /* по умолчанию вкл */ }
  const ses = session.defaultSession;
  const langs = ['ru', 'en-US'].filter((l) => ses.availableSpellCheckerLanguages.includes(l));
  if (langs.length) ses.setSpellCheckerLanguages(langs);
  ses.setSpellCheckerEnabled(state.spellcheckOn);
}

function setSpellcheck(on) {
  state.spellcheckOn = on;
  session.defaultSession.setSpellCheckerEnabled(on);
  try { fs.writeFileSync(spellFile(), JSON.stringify({ enabled: on })); } catch { /* noop */ }
  console.log(`[launcher] проверка орфографии: ${on ? 'вкл' : 'выкл'}`);
  menus.buildAppMenu(); // галочка в «Виде»
}

Object.assign(module.exports, { zoomFile, viewActions, FIND_H, onFindRequest, initSpellcheck, setSpellcheck }); // дополняем, а не заменяем: модули подключают друг друга по кругу
