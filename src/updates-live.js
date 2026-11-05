'use strict';
// Runtime updates: a check every 12 hours, a banner, a menu item, «Обновить сейчас» (Update now).
const { WebContentsView, shell } = require('electron');
const fs = require('fs');
const { guardNavigation } = require('./guard');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc, timedCheck, withRouteKey, CSP_META } = require('./util');
const { compareDebianVer, compareSemVer, tagToPackageVersion, notesBetween } = require('./versions');
const { updatePollMs, announceUpdates } = require('./updates-logic');
const { LOG_DIR, LAUNCHER_LOG_FILE, ROUTE_KEY } = require('./env');
const { probeDsh } = require('./dsh-install');
const diagnostics = require('./diagnostics');
const rollback = require('./rollback');
const dialogs = require('./dialogs');
const dshLog = require('./dsh-log');
const menus = require('./menus');
const settingsWindow = require('./settings-window');
const updates = require('./updates');
const view = require('./view');
const whatsNew = require('./whats-new');
const { noContentResponse } = require('./respond'); // pure module — direct require, no registry

/* ---------------- runtime updates ---------------- */

// While the launcher is running — the same check as at startup, every 12 hours.
// An update cannot be installed mid-work without asking (a restart would cut
// off the agent's reply): only a banner in the window and a «Обновить … до X…»
// (Update … to X…) item in «Справка» (Help); its window — «Что нового» (What's new) and «Обновить сейчас» (restart with UPDATE_NOW).
const UPDATE_POLL_MS = updatePollMs(process.env);

const notifiedUpdates = {};     // kind → version already notified about

const updateName = (kind) => (kind === 'launcher' ? 'DeepSeek Harness Launcher' : 'dsh');

// checks — the responses of the startup check (updateChecks), otherwise — request anew.
async function findUpdates(checks) {
  const [l, d] = await Promise.all(checks ? [checks.launcher, checks.dsh] : [
    timedCheck(updates.fetchLauncherLatest(), updates.UPDATE_CHECK_TIMEOUT),
    timedCheck(updates.fetchDshLatest(), updates.UPDATE_CHECK_TIMEOUT),
  ]);
  const found = {};
  // The release list was received — if «Что нового» (What's new) could not
  // show itself at startup (GitHub did not answer), show the missed changes now.
  if (l && l.releases) whatsNew.retryWhatsNew(l);
  // The local checks are independent — in parallel, not one after another.
  const lInstP = updates.launcherInstalledVersion();
  const probeP = state.dshInstalledVersion ? null : probeDsh(state.dshBin);
  const lInst = await lInstP;
  if (l && l.version && lInst && compareDebianVer(tagToPackageVersion(l.version), lInst) > 0) {
    found.launcher = { version: l.version, installed: lInst, releases: l.releases };
  }
  if (probeP) {
    const r = await probeP;
    if (r.ok) state.dshInstalledVersion = r.version;
  }
  if (d && d.version && state.dshInstalledVersion && compareSemVer(d.version, state.dshInstalledVersion) > 0) {
    found.dsh = { version: d.version, installed: state.dshInstalledVersion };
  }
  return found;
}

// Found → menu items; notify — also a banner in the window.
function setAvailableUpdates(found, notify) {
  const versions = { launcher: found.launcher && found.launcher.version, dsh: found.dsh && found.dsh.version };
  const a = announceUpdates(versions, updates.updatePrefs(), notifiedUpdates);
  const before = JSON.stringify(Object.entries(state.availableUpdates).map(([k, v]) => [k, v.version]));
  state.availableUpdates = found;
  if (JSON.stringify(Object.entries(found).map(([k, v]) => [k, v.version])) !== before) {
    for (const [kind, v] of Object.entries(a.menu)) {
      console.log(`[launcher] доступно обновление ${updateName(kind)} ${v} — пункт в меню «Справка»`);
    }
    menus.buildAppMenu();
  }
  for (const kind of a.notify) notifiedUpdates[kind] = versions[kind];
  if (notify && a.notify.length) showUpdateToast(a.notify);
}

// After the startup questions: the deferred one («Не сейчас»/Not now) — a menu item,
// without a notification (it was just asked about).
async function refreshAvailableUpdates() {
  if (updates.NO_UPDATE_CHECK) return;
  try { setAvailableUpdates(await findUpdates(state.updateChecks), false); }
  catch (e) { console.error('[launcher] проверка обновлений:', e.message); }
}

// The timer of the periodic check: the checkbox in «Настройки» (Settings) takes effect immediately.
let pollTimer = null;

function setUpdatePolling(on) {
  if (Boolean(pollTimer) === Boolean(on)) return;
  if (on) {
    pollTimer = setInterval(() => { void pollUpdates(); }, UPDATE_POLL_MS);
    console.log(`[launcher] периодическая проверка обновлений: раз в ${Math.round(UPDATE_POLL_MS / 1000)} с`);
  } else {
    clearInterval(pollTimer);
    pollTimer = null;
    console.log('[launcher] периодическая проверка обновлений выключена');
  }
}

async function pollUpdates() {
  if (state.stopping || state.launching || state.restarting || state.relaunchRequested || updates.updateState.running) return;
  try {
    const found = await findUpdates();
    if (!state.stopping) setAvailableUpdates(found, true);
  } catch (e) {
    console.error('[launcher] периодическая проверка обновлений:', e.message);
  }
}

// A «Доступна … X» (… is available) banner in the top-right corner of the window,
// over the dsh page (like the search panel — WebContentsView; under it if the
// search is open). It stays until closed: the user may have stepped away.
// «Подробнее…» (Details…) — the «Обновить … до X…» (Update … to X…) window.
const TOAST_W = 520;
const TOAST_H = 58;
let toastView = null;
let toastResizeHooked = false;

// Banner: title, an explanatory line, buttons (dshlauncher://… navigations) and ×.
function toastPageUrl({ title, sub, buttons }) {
  const css = `html,body{margin:0;height:100%;background:#151a23;color:#d7dce3;overflow:hidden;
    font:13px system-ui,'Segoe UI',Roboto,Ubuntu,sans-serif}
    body{display:flex;align-items:center;gap:8px;padding:0 8px 0 14px;box-sizing:border-box;
    border:1px solid #3b82f6;border-radius:10px}
    .t{flex:1;min-width:0}
    .t b{display:block;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
    .t span{color:#9aa4b2;font-size:12px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;display:block}
    button{background:#1f2836;border:0;border-radius:7px;color:#d7dce3;height:30px;cursor:pointer;font-size:13px;padding:0 12px;white-space:nowrap}
    button:hover{background:#2a3550}
    button.x{width:28px;padding:0;font-size:14px}`;
  const html = `<!doctype html><html lang="${i18n.getLang()}"><head><meta charset="utf-8">${CSP_META}<title>launcher-toast</title><style>${css}</style></head><body>
    <div class="t"><b>${esc(title)}</b><span>${esc(sub)}</span></div>
    ${buttons.map((b) => `<button onclick="location.href='${b.href}'">${esc(b.label)}</button>`).join('')}
    <button class="x" title="${esc(tr('Закрыть', 'Close'))}" onclick="location.href='dshlauncher://toast/close/'">&#10005;</button>
    </body></html>`;
  return 'data:text/html;charset=utf-8,' + encodeURIComponent(withRouteKey(html, ROUTE_KEY));
}

function updateToastUrl(kinds) {
  const u = kinds.map((k) => state.availableUpdates[k]).filter(Boolean);
  return toastPageUrl({
    title: kinds.length === 1
      ? tr(`Доступна ${updateName(kinds[0])} ${u[0].version}`, `${updateName(kinds[0])} ${u[0].version} is available`)
      : tr('Доступны обновления', 'Updates are available'),
    sub: kinds.length === 1
      ? tr(`установлена ${u[0].installed}`, `installed: ${u[0].installed}`)
      : kinds.map((k) => `${updateName(k)} ${state.availableUpdates[k].version}`).join(', '),
    buttons: [{ label: tr('Подробнее…', 'Details…'), href: `dshlauncher://toast/open/?k=${kinds[0]}` }],
  });
}

function placeToast() {
  if (!toastView || !state.win || state.win.isDestroyed()) return;
  const [cw] = state.win.getContentSize();
  const y = 10 + (state.findView && state.findView.getVisible() ? view.FIND_H + 8 : 0);
  toastView.setBounds({ x: Math.max(0, cw - TOAST_W - 18), y, width: Math.min(TOAST_W, cw), height: TOAST_H });
}

function showUpdateToast(kinds) {
  console.log(`[launcher] уведомление: доступна ${kinds.map((k) => `${updateName(k)} ${state.availableUpdates[k].version}`).join(', ')}`);
  showToast(updateToastUrl(kinds));
}

function showToast(url) {
  if (!state.win || state.win.isDestroyed() || state.stopping) return;
  closeUpdateToast();
  toastView = new WebContentsView({ webPreferences: { contextIsolation: true, nodeIntegration: false, spellcheck: false } });
  toastView.setBackgroundColor('#00000000');
  guardNavigation(toastView.webContents);
  toastView.webContents.loadURL(url).catch(() => {});
  state.win.contentView.addChildView(toastView);
  if (!toastResizeHooked) { state.win.on('resize', placeToast); toastResizeHooked = true; }
  placeToast();
}

function closeUpdateToast() {
  if (!toastView) return;
  const v = toastView;
  toastView = null;
  if (state.win && !state.win.isDestroyed()) state.win.contentView.removeChildView(v);
  try { v.webContents.close(); } catch { /* already closed */ }
}

function onToastRequest(route, url) {
  if (route.startsWith('/toast/open/')) {
    closeUpdateToast();
    showUpdateNowDialog(new URL(url).searchParams.get('k'));
  } else if (route.startsWith('/toast/close/')) {
    console.log('[launcher] уведомление закрыто');
    closeUpdateToast();
  } else if (route.startsWith('/toast/crash-log/')) {
    closeUpdateToast();
    void shell.openPath(fs.existsSync(LAUNCHER_LOG_FILE) ? LAUNCHER_LOG_FILE : LOG_DIR);
  } else if (route.startsWith('/toast/dsh-log/')) {
    closeUpdateToast();
    dshLog.showLogDialog();
  } else if (route.startsWith('/toast/report/')) {
    closeUpdateToast();
    void diagnostics.saveReport();
  } else if (route.startsWith('/toast/rollback/')) {
    closeUpdateToast();
    void rollback.showRollbackDialog(); // toast «прошлый запуск упал после обновления» (the previous run crashed after an update)
  }
  return noContentResponse();
}

// «Обновить … до X…» (Update … to X…) window: what's new and a warning about the restart.
function showUpdateNowDialog(kind) {
  const u = state.availableUpdates[kind];
  if (!u) return;
  const name = updateName(kind);
  const title = tr(`Обновление ${name}`, `${name} update`);
  const gen = dialogs.openDialog(title);
  if (gen === null) return;
  state.dialogCopyText = '';
  dialogs.dialogShow(gen, () => dialogs.dialogPage(title, `
    <p>${tr(`Доступна версия <code>${esc(u.version)}</code> (установлена <code>${esc(u.installed)}</code>).`,
            `Version <code>${esc(u.version)}</code> is available (installed: <code>${esc(u.installed)}</code>).`)}</p>
    ${kind === 'launcher' ? updates.notesBlockHtml(notesBetween(u.releases, u.installed, u.version, 'deb')) : ''}
    <p class="hint">${tr('Лаунчер перезапустится, dsh будет остановлен — незаконченный ответ агента прервётся.',
                         'The launcher will restart and dsh will be stopped — an unfinished agent reply will be interrupted.')}</p>
    <div class="actions">
      <button onclick="location.href='dshlauncher://update-now/?k=${kind}'">${tr('Обновить сейчас', 'Update now')}</button>
      <button onclick="location.href='dshlauncher://dialog/close/'">${tr('Позже', 'Later')}</button>
    </div>`));
}

function updateNow(kind) {
  if (!state.availableUpdates[kind]) return;
  process.env.DSH_LAUNCHER_UPDATE_NOW = kind; // a new instance — straight to installation
  settingsWindow.relaunchLauncher(`чтобы обновить ${updateName(kind)} до ${state.availableUpdates[kind].version}`);
}

Object.assign(module.exports, { setAvailableUpdates, refreshAvailableUpdates, setUpdatePolling, toastPageUrl, placeToast, showToast, onToastRequest, showUpdateNowDialog, updateNow }); // we extend, not replace: the modules require each other in a cycle
