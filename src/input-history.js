'use strict';
// History of sent messages for the dsh input field: plain ↑/↓ bring up
// previously sent text, as in a terminal. Pure logic — in the marker blocks
// (unit tests via test/_extract.js), injection into the dsh page — buildInputHistoryScript.
// The text lives in the main process (the dsh page may reload), and, via
// input-history.json in the settings folder, survives a launcher restart too. The texts
// are not written to the log: only "message saved (N characters)".
//
// How dsh works: the input field is a Lexical editor (contenteditable, marker
// data-composer-input="true"); insertion — via a synthetic beforeinput (Lexical
// handles it itself), execCommand is unreliable. If dsh changes its editor,
// the function will stop working — it is switched off by the checkbox in "Settings".
const { loadSettings, ROUTE_KEY } = require('./env');
const { readPref, writePref } = require('./prefs');

const PREF_FILE = 'input-history.json';

/* ===== historyPush (test test/input-history.js) ===== */
const inputHistoryLimit = 50;

// History — newest on top: we do not write empty text, and do not duplicate a repeat of the top item.
function historyPush(items, text, limit = inputHistoryLimit) {
  const t = String(text == null ? '' : text).trim();
  if (!t) return items;
  if (items.length && items[0] === t) return items;
  return [t, ...items].slice(0, limit);
}
/* ===== end historyPush ===== */

/* ===== historyStep (test test/input-history.js) ===== */
function historyReset() {
  return { active: false, index: -1, saved: '' };
}

// A step of ↑/↓: dir −1 — up (toward older), 1 — down (toward newer).
// nav: { active, index, saved } — history mode; ctx: { canEnter — the field is empty
// or the cursor is at the start, current — the field's text now }.
// Reply: { nav, action }; action.type: "set" — substitute the text, "restore" —
// return the saved draft, "none" — do not interfere with the editor.
function historyStep(items, nav, dir, ctx) {
  const none = { nav, action: { type: 'none' } };
  if (!items.length) return none;
  if (!nav.active) {
    if (dir !== -1 || !ctx.canEnter) return none;
    return { nav: { active: true, index: 0, saved: ctx.current || '' }, action: { type: 'set', text: items[0] } };
  }
  if (dir === -1) {
    if (nav.index >= items.length - 1) return none; // nothing older — the cursor is free
    return { nav: { ...nav, index: nav.index + 1 }, action: { type: 'set', text: items[nav.index + 1] } };
  }
  if (nav.index > 0) return { nav: { ...nav, index: nav.index - 1 }, action: { type: 'set', text: items[nav.index - 1] } };
  return { nav: historyReset(), action: { type: 'restore', text: nav.saved || '' } }; // below the newest — the draft
}
/* ===== end historyStep ===== */

/* ===== historyExit (test test/input-history.js) ===== */
// Exiting the mode (Esc): the "restore" action returns the draft.
function historyExit(nav) {
  if (!nav.active) return { nav, action: { type: 'none' } };
  return { nav: { active: false, index: -1, saved: '' }, action: { type: 'restore', text: nav.saved || '' } };
}
/* ===== end historyExit ===== */

/* ===== buildInputHistoryScript (test test/input-history.js) ===== */
// The script text for the dsh page (injected from main.js after load). Self-contained:
// it mirrors historyPush/historyStep, because the page has no access to the modules.
// A repeated injection — setItems + reusing the listeners (installed).
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
  // Insert text: select all and fire beforeinput — Lexical handles the
  // event itself (model + DOM). execCommand is unreliable, paste is ignored.
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
  // Record to history: a mirror of historyPush + a route to the main process.
  // The text goes only to our own dshlauncher:// protocol (locally), not to the log.
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
      if (e.ctrlKey || e.metaKey || e.altKey || e.isComposing) return; // other shortcuts (Alt+↑/↓ are taken by dsh)
      if (e.key === 'Enter') { if (!e.shiftKey) push(textOf(ed)); return; } // Shift+Enter — line break
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
      if (!dir || e.shiftKey) return; // Shift+↑/↓ — selection in the editor
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
  // The user started typing — history mode is disabled, the text is left alone.
  document.addEventListener('beforeinput', (e) => {
    if (!S.enabled || S.self || !S.nav.active) return;
    S.nav = reset();
  }, true);
  return 'history:installed';
})();`;
}
/* ===== end buildInputHistoryScript ===== */

// ---- storage (main process) ----

let inputHistoryCache = null; // lazy load from input-history.json

function inputHistoryOn() {
  return loadSettings().inputHistory !== false; // default — on
}

function inputHistoryItems() {
  if (inputHistoryCache === null) {
    const data = readPref(PREF_FILE);
    const list = data && Array.isArray(data.items) ? data.items : [];
    inputHistoryCache = list.filter((t) => typeof t === 'string' && t.trim()).slice(0, inputHistoryLimit);
  }
  return inputHistoryCache;
}

// Route dshlauncher://history/push/?text=… (from the dsh page). Reply:
// { changed, count } — for the log (numbers only, no text).
function pushInputHistory(text) {
  const prev = inputHistoryItems();
  const next = historyPush(prev, text);
  if (next !== prev) {
    inputHistoryCache = next;
    writePref(PREF_FILE, { items: next });
  }
  return { changed: next !== prev, count: next.length };
}

// Injection script with the current history (main.js after loading the dsh page,
// "Settings" — when the checkbox is switched on on the fly).
function inputHistoryScript() {
  return buildInputHistoryScript(ROUTE_KEY, inputHistoryItems());
}

module.exports = { inputHistoryOn, inputHistoryItems, pushInputHistory, inputHistoryScript, buildInputHistoryScript, historyPush, historyStep, historyExit };
