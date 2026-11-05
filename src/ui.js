'use strict';
// The window — pure logic: context menu items, hotkeys, window size, GPU
// state, the report name and text.
const { tr } = require('./i18n');

const WIN_DEFAULT = { width: 1440, height: 900 };
const WIN_MIN = { width: 760, height: 480 };

/* The right-click context menu — from the context-menu event's params:
   in an input field — Cut / Copy / Paste / Select all (availability per
   editFlags); outside a field — Copy (if there is a selection) and
   Select all; on a link — additionally "Copy link address"; on a
   misspelled word — first group: the correction suggestions and
   "Add to dictionary". Items — { action, label, enabled[, value] },
   groups separated by { type: 'separator' }. */
function contextMenuItems(params) {
  const f = params.editFlags || {};
  const groups = [];
  if (params.misspelledWord) {
    const sugg = (params.dictionarySuggestions || []).slice(0, 5);
    groups.push([
      ...(sugg.length
        ? sugg.map((w) => ({ action: 'replace', label: w, value: w, enabled: true }))
        : [{ action: 'none', label: tr('Нет вариантов', 'No suggestions'), enabled: false }]),
      { action: 'addWord', label: tr('Добавить в словарь', 'Add to dictionary'), enabled: true },
    ]);
  }
  if (params.linkURL) {
    groups.push([{ action: 'copyLink', label: tr('Копировать адрес ссылки', 'Copy link address'), enabled: true }]);
  }
  if (params.isEditable) {
    groups.push([
      { action: 'cut', label: tr('Вырезать', 'Cut'), enabled: Boolean(f.canCut) },
      { action: 'copy', label: tr('Копировать', 'Copy'), enabled: Boolean(f.canCopy) },
      { action: 'paste', label: tr('Вставить', 'Paste'), enabled: Boolean(f.canPaste) },
    ]);
    groups.push([{ action: 'selectAll', label: tr('Выделить всё', 'Select all'), enabled: Boolean(f.canSelectAll) }]);
  } else {
    if (String(params.selectionText || '').trim()) {
      groups.push([{ action: 'copy', label: tr('Копировать', 'Copy'), enabled: true }]);
    }
    groups.push([{ action: 'selectAll', label: tr('Выделить всё', 'Select all'), enabled: true }]);
  }
  const items = [];
  for (const g of groups) {
    if (items.length) items.push({ type: 'separator' });
    items.push(...g);
  }
  return items;
}

/* Key → an action of "View" (a name from viewActions), or null — the key
   belongs to the page. code — the physical key, independent of the
   layout (RU/EN). */
function keyAction(input) {
  const c = input.control;
  if (input.key === 'F5' || (c && input.code === 'KeyR')) return input.shift ? 'reloadHard' : 'reload';
  if (c && (input.code === 'Equal' || input.code === 'NumpadAdd')) return 'zoomIn';
  if (c && (input.code === 'Minus' || input.code === 'NumpadSubtract')) return 'zoomOut';
  if (c && (input.code === 'Digit0' || input.code === 'Numpad0')) return 'zoomReset';
  if (c && !input.shift && input.code === 'KeyF') return 'find';
  if (input.key === 'F3') return input.shift ? 'findPrev' : 'findNext';
  if (input.key === 'F11') return 'fullscreen';
  if (c && input.code === 'Comma') return 'settings';
  return null;
}

/* Both Alt keys (down and up), by the physical code — independent of the
   layout: a bare Alt arrives as key="Alt", Shift+Alt (layout switch) — as
   key="GroupNext", AltGr — as key="AltGraph", while code is always
   AltLeft/AltRight. On Linux Alt moves focus to the window's menu bar
   (on press — highlight, on release — focus), so such events are swallowed
   (preventDefault in the before-input-event handlers). Layout switching is
   not broken: the compositor keeps its state, and altKey is preserved on
   subsequent keys. */
function isAltKey(input) {
  return (input.type === 'keyDown' || input.type === 'keyUp') &&
    (input.code === 'AltLeft' || input.code === 'AltRight');
}

/* Window size from the saved state: not less than WIN_MIN and not more
   than the screen's work area (the monitor may have been changed);
   garbage — the default size. */
function windowSize(saved, workArea) {
  const num = (v) => (Number.isFinite(v) && v > 0 ? v : null);
  const fit = (v, def, min, max) => {
    const x = num(v) || def;
    return Math.round(Math.max(min, num(max) ? Math.min(x, max) : x));
  };
  const st = saved && typeof saved === 'object' ? saved : {};
  const wa = workArea || {};
  return {
    width: fit(st.width, WIN_DEFAULT.width, WIN_MIN.width, wa.width),
    height: fit(st.height, WIN_DEFAULT.height, WIN_MIN.height, wa.height),
    maximized: st.maximized === true,
  };
}

/* The GPU acceleration state for "About". The reasons for disabling — as
   at the start of main.js; markerAgeMs — the crash marker's age now
   (null — none). A marker while GPU is on = it will be disabled from the
   next start. */
function gpuStatus(st) {
  const marker = st.markerAgeMs !== null && st.markerAgeMs < st.ttlMs;
  const daysLeft = marker ? Math.max(1, Math.ceil((st.ttlMs - st.markerAgeMs) / 86400000)) : 0;
  const reason = st.noGpuRequested ? (st.noGpuBySettings ? 'setting' : 'env')
    : st.disabledByMarker ? 'marker'
    : !st.hasRenderNode ? 'norender'
    : !st.hasEgl ? 'noegl'
    : null;
  return { on: reason === null, reason, marker, daysLeft };
}

/* The report file name (local time) and the "key: value" info text. */
function reportFileName(d) {
  const p = (n) => String(n).padStart(2, '0');
  return `dsh-launcher-report-${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}.tar.gz`;
}

function reportInfoText(facts) {
  return facts.map(([k, v]) => `${k}: ${v === undefined || v === null || v === '' ? '—' : v}`).join('\n') + '\n';
}

/* ===== agentEvent (pure function, test test/agent.js) =====
   What changed in the dsh GUI between two checks: prev/cur — { running,
   waiting } (waiting — 'approval' | 'question' | 'plan' | null). →
   { type: 'waiting', kind } — the agent has stopped and is waiting for a
   human (a new wait or of another kind); { type: 'finished' } — the answer
   is over and the agent waits for nothing; null — nothing to report. */
function agentEvent(prev, cur) {
  if (cur.waiting && cur.waiting !== prev.waiting) return { type: 'waiting', kind: cur.waiting };
  if (prev.running && !cur.running && !cur.waiting) return { type: 'finished' };
  return null;
}
/* ===== end agentEvent ===== */

/* ===== crashRestart (pure function, test test/agent.js) =====
   Whether to restart a dsh that exited on its own: only on a crash (a
   nonzero code or a signal) and no more than max times within windowMs —
   otherwise dsh keeps crashing again and again, and spinning it in a
   circle is pointless. history — the times of past auto-restarts (ms).
   → { restart, history } (history — including this one). */
function crashRestart(history, now, code, signal, max = 3, windowMs = 10 * 60 * 1000) {
  const recent = (Array.isArray(history) ? history : []).filter((t) => now - t >= 0 && now - t < windowMs);
  const crashed = code !== 0 || Boolean(signal);
  if (!crashed || recent.length >= max) return { restart: false, history: recent };
  return { restart: true, history: [...recent, now] };
}
/* ===== end crashRestart ===== */

/* ===== pickCrashDumps (pure function, test test/report.js) =====
   Which crash dumps to put into the report: files — [{ path, mtimeMs,
   size }]; no older than maxAgeDays, newest first, no more than max of
   them and maxBytes in total. */
function pickCrashDumps(files, now, maxAgeDays = 30, max = 3, maxBytes = 25 * 1024 * 1024) {
  const fresh = (Array.isArray(files) ? files : [])
    .filter((f) => f && now - f.mtimeMs >= 0 && now - f.mtimeMs <= maxAgeDays * 86400000)
    .sort((a, b) => b.mtimeMs - a.mtimeMs);
  const out = [];
  let total = 0;
  for (const f of fresh) {
    if (out.length >= max || total + f.size > maxBytes) continue;
    out.push(f);
    total += f.size;
  }
  return out;
}
/* ===== end pickCrashDumps ===== */

module.exports = { pickCrashDumps, crashRestart, agentEvent, WIN_MIN, contextMenuItems, keyAction, isAltKey, windowSize, gpuStatus, reportFileName, reportInfoText };
