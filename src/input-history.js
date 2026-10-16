'use strict';
// История отправленных сообщений для поля ввода dsh: обычные ↑/↓ поднимают
// ранее отправленное, как в терминале. Чистая логика — в маркер-блоках
// (unit-тесты через test/_extract.js), впрыск в страницу dsh — buildInputHistoryScript.
// Текст живёт в main-процессе (страница dsh может перезагружаться), а через
// input-history.json в каталоге настроек — и перезапуск лаунчера. В журнал
// тексты не пишутся: только «сохранено сообщение (N символов)».
//
// Устройство dsh: поле ввода — редактор Lexical (contenteditable, маркер
// data-composer-input="true"); вставка — синтетическим beforeinput (Lexical
// обрабатывает его сам), execCommand ненадёжен. Если dsh сменит редактор,
// функция перестанет срабатывать — выключается галочкой в «Настройках».
const { loadSettings, ROUTE_KEY } = require('./env');
const { readPref, writePref } = require('./prefs');

const PREF_FILE = 'input-history.json';

/* ===== historyPush (тест test/input-history.js) ===== */
const inputHistoryLimit = 50;

// История — свежие сверху: пустое не пишем, повтор верхнего не дублируем.
function historyPush(items, text, limit = inputHistoryLimit) {
  const t = String(text == null ? '' : text).trim();
  if (!t) return items;
  if (items.length && items[0] === t) return items;
  return [t, ...items].slice(0, limit);
}
/* ===== end historyPush ===== */

/* ===== historyStep (тест test/input-history.js) ===== */
function historyReset() {
  return { active: false, index: -1, saved: '' };
}

// Шаг ↑/↓: dir −1 — вверх (к старым), 1 — вниз (к новым).
// nav: { active, index, saved } — режим истории; ctx: { canEnter — поле пустое
// или курсор в начале, current — текст поля сейчас }.
// Ответ: { nav, action }; action.type: «set» — подставить текст, «restore» —
// вернуть сохранённый черновик, «none» — не мешать редактору.
function historyStep(items, nav, dir, ctx) {
  const none = { nav, action: { type: 'none' } };
  if (!items.length) return none;
  if (!nav.active) {
    if (dir !== -1 || !ctx.canEnter) return none;
    return { nav: { active: true, index: 0, saved: ctx.current || '' }, action: { type: 'set', text: items[0] } };
  }
  if (dir === -1) {
    if (nav.index >= items.length - 1) return none; // старше нет — курсор свободен
    return { nav: { ...nav, index: nav.index + 1 }, action: { type: 'set', text: items[nav.index + 1] } };
  }
  if (nav.index > 0) return { nav: { ...nav, index: nav.index - 1 }, action: { type: 'set', text: items[nav.index - 1] } };
  return { nav: historyReset(), action: { type: 'restore', text: nav.saved || '' } }; // ниже нового — черновик
}
/* ===== end historyStep ===== */

/* ===== historyExit (тест test/input-history.js) ===== */
// Выход из режима (Esc): действие «restore» вернёт черновик.
function historyExit(nav) {
  if (!nav.active) return { nav, action: { type: 'none' } };
  return { nav: { active: false, index: -1, saved: '' }, action: { type: 'restore', text: nav.saved || '' } };
}
/* ===== end historyExit ===== */

/* ===== buildInputHistoryScript (тест test/input-history.js) ===== */
// Текст скрипта для страницы dsh (впрыск из main.js после загрузки). Автономен:
// зеркалит historyPush/historyStep, потому что в странице нет доступа к модулям.
// Повторный впрыск — setItems + повторное использование слушателей (installed).
function buildInputHistoryScript(routeKey, items) {
  const cfg = JSON.stringify({ key: String(routeKey || ''), items: Array.isArray(items) ? items : [] });
  return `(() => {
  const cfg = ${cfg};
  const reset = () => ({ active: false, index: -1, saved: '' });
  const S = window.__dshInputHistory = window.__dshInputHistory || {};
  S.setItems = (list) => { S.items = Array.isArray(list) ? list : []; S.nav = reset(); };
  S.setEnabled = (on) => { S.enabled = !!on; if (!on) S.nav = reset(); };
  S.setItems(cfg.items);
  if (S.enabled === undefined) S.enabled = true;
  if (S.installed) return 'history:updated';
  S.installed = true;
  const composer = () => document.querySelector('[data-composer-input="true"]');
  const textOf = (ed) => { try { return ed.innerText || ''; } catch (e) { return ed.textContent || ''; } };
  const caretAtStart = (ed) => {
    try {
      const sel = window.getSelection();
      if (!sel || !sel.rangeCount || !sel.isCollapsed) return false;
      const r = sel.getRangeAt(0);
      const pre = document.createRange();
      pre.selectNodeContents(ed);
      pre.setEnd(r.startContainer, r.startOffset);
      return pre.toString().replace(/\\u200b/g, '') === '';
    } catch (e) { return false; }
  };
  // Подставить текст: выделить всё и послать beforeinput — Lexical обрабатывает
  // событие сам (модель + DOM). execCommand ненадёжен, paste игнорируется.
  const setText = (ed, text) => {
    ed.focus();
    const r = document.createRange();
    r.selectNodeContents(ed);
    const sel = window.getSelection();
    sel.removeAllRanges();
    sel.addRange(r);
    S.self = true;
    try {
      ed.dispatchEvent(new InputEvent('beforeinput', { inputType: 'insertText', data: String(text), bubbles: true, cancelable: true }));
    } catch (e) { /* noop */ } finally { S.self = false; }
  };
  // Запись в историю: зеркало historyPush + маршрут в main-процесс. Текст уходит
  // только своему протоколу dshlauncher:// (локально), в журнал не пишется.
  const push = (text) => {
    const t = String(text || '').trim();
    if (!t) return;
    if (S.items.length && S.items[0] === t) { S.nav = reset(); return; }
    S.items = [t].concat(S.items).slice(0, 50);
    S.nav = reset();
    try { window.location.href = 'dshlauncher://' + cfg.key + '/history/push/?text=' + encodeURIComponent(t); } catch (e) { /* noop */ }
  };
  document.addEventListener('keydown', (e) => {
    try {
      if (!S.enabled) return;
      const ed = composer();
      if (!ed || !(e.target === ed || ed.contains(e.target))) return;
      if (e.ctrlKey || e.metaKey || e.altKey || e.isComposing) return; // чужие шорткаты (Alt+↑/↓ заняты dsh)
      if (e.key === 'Enter') { if (!e.shiftKey) push(textOf(ed)); return; } // Shift+Enter — перенос строки
      if (e.key === 'Escape') {
        if (!S.nav.active) return;
        e.preventDefault();
        e.stopPropagation();
        const saved = S.nav.saved || '';
        S.nav = reset();
        setText(ed, saved);
        return;
      }
      const dir = e.key === 'ArrowUp' ? -1 : e.key === 'ArrowDown' ? 1 : 0;
      if (!dir || e.shiftKey) return; // Shift+↑/↓ — выделение в редакторе
      if (ed.getAttribute('aria-disabled') === 'true') return;
      if (!S.items.length) return;
      const cur = textOf(ed);
      const canEnter = cur.trim() === '' || caretAtStart(ed);
      let nav = null;
      let text = null;
      if (!S.nav.active) {
        if (dir !== -1 || !canEnter) return;
        nav = { active: true, index: 0, saved: cur };
        text = S.items[0];
      } else if (dir === -1) {
        if (S.nav.index >= S.items.length - 1) return;
        nav = { active: true, index: S.nav.index + 1, saved: S.nav.saved };
        text = S.items[nav.index];
      } else if (S.nav.index > 0) {
        nav = { active: true, index: S.nav.index - 1, saved: S.nav.saved };
        text = S.items[nav.index];
      } else {
        nav = reset();
        text = S.nav.saved || '';
      }
      e.preventDefault();
      e.stopPropagation();
      S.nav = nav;
      setText(ed, text);
    } catch (e) { /* noop */ }
  }, true);
  // Пользователь начал печатать — режим истории выключается, текст не трогаем.
  document.addEventListener('beforeinput', (e) => {
    if (!S.enabled || S.self || !S.nav.active) return;
    S.nav = reset();
  }, true);
  return 'history:installed';
})();`;
}
/* ===== end buildInputHistoryScript ===== */

// ---- хранилище (main-процесс) ----

let inputHistoryCache = null; // ленивая загрузка из input-history.json

function inputHistoryOn() {
  return (loadSettings() || {}).inputHistory !== false; // по умолчанию — включено
}

function inputHistoryItems() {
  if (inputHistoryCache === null) {
    const data = readPref(PREF_FILE);
    const list = data && Array.isArray(data.items) ? data.items : [];
    inputHistoryCache = list.filter((t) => typeof t === 'string' && t.trim()).slice(0, inputHistoryLimit);
  }
  return inputHistoryCache;
}

// Маршрут dshlauncher://history/push/?text=… (со страницы dsh). Ответ:
// { changed, count } — для журнала (числа, без текста).
function pushInputHistory(text) {
  const prev = inputHistoryItems();
  const next = historyPush(prev, text);
  if (next !== prev) {
    inputHistoryCache = next;
    writePref(PREF_FILE, { items: next });
  }
  return { changed: next !== prev, count: next.length };
}

// Скрипт впрыска с текущей историей (main.js после загрузки страницы dsh,
// «Настройки» — при включении галочки на ходу).
function inputHistoryScript() {
  return buildInputHistoryScript(ROUTE_KEY, inputHistoryItems());
}

module.exports = { inputHistoryOn, inputHistoryItems, pushInputHistory, inputHistoryScript, buildInputHistoryScript, historyPush, historyStep, historyExit };
