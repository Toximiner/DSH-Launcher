'use strict';
// Обновления во время работы: проверка раз в 12 часов, плашка, пункт меню, «Обновить сейчас».
const { WebContentsView, shell } = require('electron');
const fs = require('fs');
const { guardNavigation } = require('./guard');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc, timedCheck, withRouteKey } = require('./util');
const { compareDebianVer, compareSemVer, tagToPackageVersion, notesBetween } = require('./versions');
const { updatePollMs, announceUpdates } = require('./updates-logic');
const { LOG_DIR, LAUNCHER_LOG_FILE, ROUTE_KEY } = require('./env');
const { probeDsh } = require('./dsh-install');
const diagnostics = require('./diagnostics');
const dialogs = require('./dialogs');
const dshLog = require('./dsh-log');
const menus = require('./menus');
const settingsWindow = require('./settings-window');
const updates = require('./updates');
const view = require('./view');
const launcher = require('./launcher'); // общий реестр (наполняет main.js)

/* ---------------- обновления во время работы ---------------- */

// Пока лаунчер работает — раз в 12 часов та же проверка, что при запуске.
// Ставить обновление посреди работы нельзя без спроса (перезапуск оборвёт
// ответ агента): только плашка в окне и пункт «Обновить … до X…» в «Справке»;
// его окно — «Что нового» и «Обновить сейчас» (перезапуск с UPDATE_NOW).
const UPDATE_POLL_MS = updatePollMs(process.env);

const notifiedUpdates = {};     // kind → версия, о которой уже уведомили

const updateName = (kind) => (kind === 'launcher' ? 'DeepSeek Harness Launcher' : 'dsh');

// checks — ответы проверки при запуске (updateChecks), иначе — запросить заново.
async function findUpdates(checks) {
  const [l, d] = await Promise.all(checks ? [checks.launcher, checks.dsh] : [
    timedCheck(updates.fetchLauncherLatest(), updates.UPDATE_CHECK_TIMEOUT),
    timedCheck(updates.fetchDshLatest(), updates.UPDATE_CHECK_TIMEOUT),
  ]);
  const found = {};
  const lInst = await updates.launcherInstalledVersion();
  if (l && l.version && lInst && compareDebianVer(tagToPackageVersion(l.version), lInst) > 0) {
    found.launcher = { version: l.version, installed: lInst, releases: l.releases };
  }
  if (!state.dshInstalledVersion) {
    const r = await probeDsh(state.dshBin);
    if (r.ok) state.dshInstalledVersion = r.version;
  }
  if (d && d.version && state.dshInstalledVersion && compareSemVer(d.version, state.dshInstalledVersion) > 0) {
    found.dsh = { version: d.version, installed: state.dshInstalledVersion };
  }
  return found;
}

// Найденное → пункты меню; notify — ещё и плашка в окне.
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

// После вопросов при старте: отложенное («Не сейчас») — пунктом в меню, без
// уведомления (о нём только что спросили).
async function refreshAvailableUpdates() {
  if (updates.NO_UPDATE_CHECK) return;
  try { setAvailableUpdates(await findUpdates(state.updateChecks), false); }
  catch (e) { console.error('[launcher] проверка обновлений:', e.message); }
}

// Таймер периодической проверки: галочка в «Настройках» действует сразу.
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

// Плашка «Доступна … X» в правом верхнем углу окна, поверх страницы dsh (как
// панель поиска — WebContentsView; под ней, если поиск открыт). Висит, пока
// не закроют: пользователь мог отойти. «Подробнее…» — окно «Обновить … до X…».
const TOAST_W = 520;
const TOAST_H = 58;
let toastView = null;
let toastResizeHooked = false;

// Плашка: заголовок, строка пояснения, кнопки (переходы dshlauncher://…) и ×.
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
  const html = `<!doctype html><html lang="${i18n.getLang()}"><head><meta charset="utf-8"><title>launcher-toast</title><style>${css}</style></head><body>
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
  try { v.webContents.close(); } catch { /* уже закрыта */ }
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
  }
  return launcher.noContentResponse();
}

// Окно «Обновить … до X…»: что нового и предупреждение о перезапуске.
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
  process.env.DSH_LAUNCHER_UPDATE_NOW = kind; // новый экземпляр — сразу к установке
  settingsWindow.relaunchLauncher(`чтобы обновить ${updateName(kind)} до ${state.availableUpdates[kind].version}`);
}

Object.assign(module.exports, { setAvailableUpdates, refreshAvailableUpdates, setUpdatePolling, toastPageUrl, placeToast, showToast, onToastRequest, showUpdateNowDialog, updateNow }); // дополняем, а не заменяем: модули подключают друг друга по кругу
