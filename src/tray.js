'use strict';
// The tray and background operation; the "agent finished answering" signal.
const { app, Menu, Tray, Notification, nativeImage } = require('electron');
const path = require('path');
const state = require('./state');
const { agentEvent } = require('./ui');
const i18n = require('./i18n');
const { tr } = i18n;
const { loadSettings, PLAIN_URL } = require('./env');
const fileHelp = require('./file-help');

/* ---------------- the tray and background operation ---------------- */

// «Настройки → Окно» (Settings → Window): closeToTray — the close button
// hides the window to the tray, dsh keeps running (off by default: without
// tray support on the desktop — plain GNOME has none — the window would be
// lost with no icon; you can get it back by starting the launcher again).
// attention — draw attention when the agent finished answering and the
// window is not focused (on by default).

let trayIcons = null;

const closeToTrayOn = () => loadSettings().closeToTray === true;
const attentionOn = () => loadSettings().attention !== false;
const attentionSoundOn = () => loadSettings().attentionSound === true;

// The tray icon and the same one with an orange dot ("agent finished").
function makeTrayIcons() {
  let base = nativeImage.createFromPath(path.join(__dirname, '..', 'icon.png'));
  if (base.isEmpty()) base = nativeImage.createFromPath('/usr/share/icons/hicolor/256x256/apps/dsh-launcher.png');
  const normal = base.isEmpty() ? base : base.resize({ width: 24, height: 24, quality: 'best' });
  if (normal.isEmpty()) return { normal, attention: normal };
  const { width: w, height: h } = normal.getSize();
  const px = Buffer.from(normal.toBitmap()); // BGRA
  const cx = w - 6, cy = h - 6, r = 5;
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const d = Math.hypot(x - cx, y - cy);
      if (d > r + 0.5) continue;
      const i = (y * w + x) * 4;
      const ring = d > r - 1.2; // dark outline — the dot is visible on a light panel too
      px[i] = ring ? 0x15 : 0x0b; px[i + 1] = ring ? 0x11 : 0x9e; px[i + 2] = ring ? 0x0f : 0xf5; px[i + 3] = 0xff;
    }
  }
  return { normal, attention: nativeImage.createFromBitmap(px, { width: w, height: h }) };
}

function trayMenu() {
  return Menu.buildFromTemplate([
    { label: tr('Открыть DeepSeek Harness', 'Open DeepSeek Harness'), click: () => showMainWindow() },
    { label: tr('Перезапустить dsh…', 'Restart dsh…'), click: () => { showMainWindow(); void fileHelp.restartDsh(); } },
    { type: 'separator' },
    { label: tr('Выйти', 'Quit'), click: () => app.quit() },
  ]);
}

/* ===== trayStatusText (test test/tray.js) =====
   The icon tooltip about the dsh state: fatal — stopped (auto-restart
   halted), portState 'up' — running, 'down' — not running, null — not
   checked yet (only the launcher name). tr — (ru, en) → string. */
function trayStatusText(portState, fatal, tr) {
  const name = 'DeepSeek Harness';
  if (fatal) return tr(`${name} — dsh остановлен (авто-перезапуск остановлен)`, `${name} — dsh has stopped (auto-restart halted)`);
  if (portState === 'up') return tr(`${name} — dsh запущен`, `${name} — dsh is running`);
  if (portState === 'down') return tr(`${name} — dsh не запущен`, `${name} — dsh is not running`);
  return name;
}
/* ===== end trayStatusText ===== */

/* ===== trayStatusLabel (test test/tray.js) =====
   The same state for the log (the log is in Russian, tr is not needed). */
function trayStatusLabel(portState, fatal) {
  if (fatal) return 'dsh остановлен (авто-перезапуск остановлен)';
  if (portState === 'up') return 'dsh запущен';
  if (portState === 'down') return 'dsh не запущен';
  return 'состояние dsh ещё не проверено';
}
/* ===== end trayStatusLabel ===== */

// The dsh status tooltip is updated every 2 s (pollAgentState); the cache —
// so Electron is not touched on every tick while the state is unchanged.
let trayStatusCache = null;
let trayAttention = false;

/* ===== refreshTrayStatus (test test/tray.js) =====
   Updates the icon tooltip from state.portState/dshFatal. The cache
   (trayStatusCache, trayAttention) — external: we do not touch Electron
   without a state change. state — { tray, portState, dshFatal },
   tr — (ru, en) → string, console — the log, trayStatusText/
   trayStatusLabel — see above. */
function refreshTrayStatus() {
  if (!state.tray || trayAttention) return; // the "agent finished" tooltip matters more
  const key = `${state.portState}|${state.dshFatal ? 1 : 0}`;
  if (key === trayStatusCache) return;
  trayStatusCache = key;
  state.tray.setToolTip(trayStatusText(state.portState, state.dshFatal, tr));
  console.log(`[launcher] трей: ${trayStatusLabel(state.portState, state.dshFatal)}`);
}
/* ===== end refreshTrayStatus ===== */

function setTray(on) {
  if (Boolean(state.tray) === Boolean(on)) return;
  if (on) {
    if (!trayIcons) trayIcons = makeTrayIcons();
    state.tray = new Tray(trayIcons.normal);
    state.tray.setToolTip('DeepSeek Harness');
    state.tray.setContextMenu(trayMenu());
    state.tray.on('click', () => showMainWindow());
    console.log('[launcher] трей включён: крестик прячет окно, dsh работает дальше');
    trayStatusCache = null;
    refreshTrayStatus(); // show the dsh status immediately
  } else {
    state.tray.destroy();
    state.tray = null;
    trayStatusCache = null;
    console.log('[launcher] трей выключен');
    if (state.win && !state.win.isDestroyed() && !state.win.isVisible()) showMainWindow(); // do not leave the window hidden without an icon
  }
}

// ev — the agentEvent event: the tooltip next to the icon — exactly what happened.
function setTrayAttention(on, ev = { type: 'finished' }) {
  if (!state.tray || !trayIcons) return;
  trayAttention = on;
  state.tray.setImage(on ? trayIcons.attention : trayIcons.normal);
  if (on) {
    state.tray.setToolTip(`DeepSeek Harness — ${agentNotice(ev)}`);
  } else {
    trayStatusCache = null; // restore the dsh status tooltip
    refreshTrayStatus();
  }
}

function showMainWindow() {
  if (!state.win || state.win.isDestroyed()) return;
  const wasHidden = !state.win.isVisible();
  state.win.show();
  if (state.win.isMinimized()) state.win.restore();
  state.win.focus();
  state.win.flashFrame(false);
  clearAgentNotification();
  setTrayAttention(false);
  if (wasHidden) console.log('[launcher] окно показано из трея');
}

// The close button with the tray on — hide the window (not on quit and restart).
function onMainWindowClose(e) {
  if (!state.tray || !closeToTrayOn() || state.stopping || state.relaunchRequested || state.restarting) return;
  e.preventDefault();
  state.win.hide();
  console.log('[launcher] окно спрятано в трей — dsh работает дальше');
}

// The agent finished answering: while it answers, the dsh GUI chat has the
// [data-chat-running] indicator (only for the session open in the window;
// it is an internal detail of the dsh interface — if it is renamed, the
// signal simply stops firing). Checked every 2 s; the "was running →
// stopped" transition with the window not focused (or hidden) — a system
// notification (click — open the window), a dot on the tray icon, flashing
// of the taskbar button (under X11; on Wayland in GNOME there is none) and,
// if enabled, a sound.
const AGENT_POLL_MS = 2000;
let agent = { running: false, waiting: null }; // what it was on the previous check
// The last notification (the reference is kept so its click is not GCed).
// There is only one: a new one replaces the old, and when the window is
// opened it is closed. Otherwise notifications pile up in the GNOME list
// and the Ubuntu dock counts them as unread: the badge counter grows 1, 2,
// … although the window was already opened.
let agentNotification = null;

function clearAgentNotification() {
  if (!agentNotification) return;
  const n = agentNotification;
  agentNotification = null;
  try { n.close(); } catch { /* already closed */ }
  console.log('[launcher] уведомление агента закрыто');
}

function agentSessionTitle() {
  try {
    const m = /^(.*) — [^—]+$/.exec(state.win.webContents.getTitle()); // «<session> — DeepSeek Harness»
    return m ? m[1] : '';
  } catch { return ''; }
}

// Notification texts per the agentEvent event.
function agentNotice(ev) {
  if (ev.type === 'finished') return tr('Агент закончил ответ', 'The agent has finished');
  if (ev.kind === 'approval') return tr('Агент ждёт подтверждения команды', 'The agent is waiting for you to approve a command');
  if (ev.kind === 'question') return tr('Агент задал вопрос', 'The agent has a question for you');
  return tr('Агент ждёт согласования плана', 'The agent is waiting for you to review the plan');
}

// For the log (it is in Russian regardless of the window language).
const agentWhat = (ev) => (ev.type === 'finished' ? 'агент закончил ответ'
  : `агент ждёт: ${{ approval: 'подтверждение команды', question: 'ответ на вопрос', plan: 'согласование плана' }[ev.kind]}`);

function notifyAgent(ev) {
  const title = agentNotice(ev);
  console.log(`[launcher] уведомление: ${agentWhat(ev)}`);
  if (!Notification.isSupported()) return;
  clearAgentNotification(); // do not pile up: one, the newest
  const n = new Notification({
    title,
    body: agentSessionTitle() || 'DeepSeek Harness',
    icon: path.join(__dirname, '..', 'icon.png'),
  });
  n.on('click', () => showMainWindow());
  n.show();
  agentNotification = n;
}

// A short two-note signal — WebAudio right in the window (no external programs).
function playAgentSound() {
  console.log('[launcher] звук: агент закончил ответ');
  const js = `(() => { try {
    const c = new AudioContext(); const t = c.currentTime;
    for (const [f, at] of [[660, 0], [880, 0.16]]) {
      const o = c.createOscillator(); const g = c.createGain();
      o.type = 'sine'; o.frequency.value = f;
      g.gain.setValueAtTime(0.0001, t + at); g.gain.exponentialRampToValueAtTime(0.18, t + at + 0.02);
      g.gain.exponentialRampToValueAtTime(0.0001, t + at + 0.3);
      o.connect(g).connect(c.destination); o.start(t + at); o.stop(t + at + 0.32);
    }
    setTimeout(() => c.close(), 900);
  } catch (e) {} })()`;
  state.win.webContents.executeJavaScript(js, true).catch(() => {});
}

// Markers in the dsh GUI: [data-chat-running] — the agent is answering;
// human-waiting panels — command approval ([data-approval-key], aria-busy
// after the answer), a question ([data-question-key]), plan review
// ([data-plan-review-key]). Internal details of the dsh interface: if they
// are renamed, the signal simply stops firing.
const AGENT_PROBE = `(() => ({
  running: !!document.querySelector('[data-chat-running]'),
  waiting: document.querySelector('[data-approval-key]:not([aria-busy="true"])') ? 'approval'
    : document.querySelector('[data-question-key]') ? 'question'
    : document.querySelector('[data-plan-review-key]') ? 'plan' : null,
}))()`;

async function pollAgentState() {
  refreshTrayStatus(); // the dsh status on the icon — independent of the agent
  if (!state.win || state.win.isDestroyed() || state.stopping) return;
  try { if (new URL(state.win.webContents.getURL()).origin !== new URL(PLAIN_URL).origin) { agent = { running: false, waiting: null }; return; } } catch { return; }
  let cur;
  try { cur = await state.win.webContents.executeJavaScript(AGENT_PROBE, true); } catch { return; }
  if (!cur || typeof cur !== 'object') return;
  const prev = agent;
  agent = { running: Boolean(cur.running), waiting: cur.waiting || null };
  if (agent.running && !prev.running) console.log('[launcher] агент отвечает');
  const ev = agentEvent(prev, agent);
  if (!ev) return;
  const hidden = !state.win.isVisible();
  const away = hidden || !state.win.isFocused();
  const what = agentWhat(ev);
  console.log(`[launcher] ${what}${hidden ? ' — окно в трее' : away ? ' — окно не в фокусе' : ''}`);
  if (!away || !attentionOn()) return;
  notifyAgent(ev);
  if (state.tray) { setTrayAttention(true, ev); console.log(`[launcher] значок трея: ${what}`); }
  if (!hidden) state.win.flashFrame(true);
  if (attentionSoundOn()) playAgentSound();
}

Object.assign(module.exports, { closeToTrayOn, attentionOn, attentionSoundOn, trayMenu, setTray, setTrayAttention, clearAgentNotification, showMainWindow, onMainWindowClose, AGENT_POLL_MS, playAgentSound, pollAgentState, trayStatusText, trayStatusLabel, refreshTrayStatus }); // extend rather than replace: the modules require each other in a circle
