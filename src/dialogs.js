'use strict';
// A dialog window over the main one: opening, content, closing, «О программе», «Проверить обновления…».
const { WebContentsView, clipboard } = require('electron');
const os = require('os');
const { guardNavigation, denyExternalOpen } = require('./guard');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc, timedCheck } = require('./util');
const { updateStatus, notesBetween } = require('./versions');
const { LOG_DIR, IS_PKG_INSTALL } = require('./env');
const fileHelp = require('./file-help');
const menus = require('./menus');
const { isAltKey } = require('./ui');
const pages = require('./pages');
const updatesLive = require('./updates-live');
const updates = require('./updates');
const { noContentResponse } = require('./respond'); // a pure module — direct require, no registry

/* ---------------- dialog window: «О программе», «Проверить обновления» ---------------- */

// A dialog — a layer inside the main window (a WebContentsView over its whole
// area): a dimming and a card in the center, like the search panel and the
// toasts. There is no separate OS window — nothing extra in the taskbar and
// window list; on Wayland the compositor does not decide where to show it.
// The dsh GUI under the layer is not reloaded and does not lose state.
// One layer for all items: a repeated call changes the content. Close — by
// button or Esc (a click outside the card does not close the dialog — easy to miss it).
// Buttons — navigations to dshlauncher://dialog/… (a 204 reply: the page
// stays, the handler does its thing).
let dialogGen = 0; // generation: a late async result does not land in a closed or replaced dialog

const dialogOpen = () => Boolean(state.dialogView && !state.dialogView.webContents.isDestroyed());

function placeDialog() {
  if (!dialogOpen() || !state.win || state.win.isDestroyed()) return;
  const [w, h] = state.win.getContentSize();
  state.dialogView.setBounds({ x: 0, y: 0, width: w, height: h });
}

// Open a dialog (or reuse an open one) — the generation of the new content,
// null — there is no window / exit is in progress.
function openDialog(title) {
  if (!state.win || state.win.isDestroyed() || state.stopping) return null;
  dialogGen++;
  if (dialogOpen()) { state.dialogView.webContents.focus(); return dialogGen; }
  const v = new WebContentsView({ webPreferences: { contextIsolation: true, nodeIntegration: false, spellcheck: false } });
  v.setBackgroundColor('#00000000'); // the dimming is drawn by the layer's page
  state.dialogView = v;
  const wc = v.webContents;
  denyExternalOpen(wc);
  guardNavigation(wc); // a link without target=_blank — in the browser too, not in the layer
  wc.on('context-menu', (_e, params) => menus.showContextMenu(params, state.win, wc));
  wc.on('before-input-event', (e, input) => {
    if (input.type === 'keyDown' && input.key === 'Escape') { e.preventDefault(); closeDialog(); }
    if (isAltKey(input)) e.preventDefault(); // no focus to the menu bar (see main.js)
  });
  wc.on('did-finish-load', () => { if (!wc.isDestroyed()) wc.focus(); });
  state.win.contentView.addChildView(v); // above everything: the GUI, the search, the toasts
  placeDialog();
  if (!resizeHooked) { state.win.on('resize', placeDialog); resizeHooked = true; }
  console.log(`[launcher] диалог: ${title}`);
  return dialogGen;
}
let resizeHooked = false;

function closeDialog() {
  if (!dialogOpen()) return;
  // the resize listener is one for the whole window (resizeHooked) and lives
  // with the window — we do not remove it: recreating it per dialog is extra events
  const v = state.dialogView;
  state.dialogView = null; state.dialogRender = null; dialogGen++;
  if (state.win && !state.win.isDestroyed()) {
    state.win.contentView.removeChildView(v);
    state.win.webContents.focus();
  }
  try { v.webContents.close(); } catch { /* already closed */ }
}

// Show the content, if the dialog of the same generation is still open.
function dialogShow(gen, render) {
  if (gen !== dialogGen || !dialogOpen()) return;
  state.dialogRender = render;
  render(false);
}

function dialogPage(title, body) {
  if (!dialogOpen()) return;
  state.dialogView.webContents.loadURL(pages.pageHtml(title, body, false, { dialog: true }))
    .catch((e) => { if (e.code !== 'ERR_ABORTED') console.error('[launcher] диалог:', e.message); });
}

const dialogCloseButton = () =>
  `<button onclick="location.href='dshlauncher://dialog/close/'">${tr('Закрыть', 'Close')}</button>`;

function onDialogClose() {
  closeDialog();
  return noContentResponse();
}

function onDialogCopy() {
  if (state.dialogCopyText) clipboard.writeText(state.dialogCopyText);
  if (state.dialogRender) state.dialogRender(true); // repaint: «Скопировано»
  return noContentResponse();
}

async function showAboutDialog() {
  const title = tr('О программе', 'About');
  const gen = openDialog(title);
  if (gen === null) return;
  const ver = { lver: null, dver: null, loaded: false };
  const render = (copied) => {
    const gpu = fileHelp.gpuStatusNow();
    const lLabel = !IS_PKG_INSTALL ? tr(' (сборка из исходников)', ' (built from source)') : '';
    const lDisp = ver.loaded ? (ver.lver || tr('неизвестно', 'unknown')) : tr('загружаю…', 'loading…');
    const dDisp = ver.loaded ? (ver.dver || tr('не установлен', 'not installed')) : tr('загружаю…', 'loading…');
    const rows = [
      ['DeepSeek Harness Launcher', lDisp + lLabel],
      [tr('бэкенд dsh', 'dsh backend'), dDisp],
      [tr('путь к dsh', 'path to dsh'), state.dshBin || '—'],
      ['Node.js', 'v' + process.versions.node],
      [tr('ОС', 'OS'), os.platform() + ' ' + os.release() + ' (' + os.arch() + ')'],
      [tr('GPU-ускорение', 'GPU acceleration'), fileHelp.gpuStatusText(gpu)],
      [tr('логи', 'logs'), LOG_DIR],
    ];
    state.dialogCopyText = ver.loaded ? rows.map(([k, v]) => k + ': ' + v).join('\n') : '';
    const body = `
      <table style="border-collapse:collapse; margin-bottom:4px">
        ${rows.map(([k, v]) =>
          '<tr><td style="padding:3px 14px 3px 0; color:#9aa4b2; white-space:nowrap">' + esc(k) + '</td>' +
          '<td style="padding:3px 0"><code>' + esc(v) + '</code></td></tr>').join('')}
      </table>
      <p style="margin-top:10px">
        <a href="https://github.com/Toximiner/DSH-Launcher" target="_blank">${tr('Репозиторий лаунчера (GitHub)', 'Launcher repository (GitHub)')}</a> ·
        <a href="https://www.npmjs.com/package/@deepseek-ai/dsh" target="_blank">@deepseek-ai/dsh</a>
      </p>
      <div class="actions">
        ${ver.loaded ? '<button onclick="location.href=\'dshlauncher://dialog/copy/\'">' + (copied ? tr('Скопировано', 'Copied') : tr('Скопировать для отчёта', 'Copy for report')) + '</button>' : ''}
        <button onclick="location.href='dshlauncher://dialog/logs/'">${tr('Папка логов', 'Logs folder')}</button>
        <button onclick="location.href='dshlauncher://menu/report/'">${tr('Сохранить отчёт…', 'Save report…')}</button>
        ${gpu.marker ? '<button onclick="location.href=\'dshlauncher://dialog/gpu-enable/\'">' + tr('Включить GPU снова', 'Re-enable GPU') + '</button>' : ''}
        ${dialogCloseButton()}
      </div>`;
    dialogPage(title, body);
  };
  dialogShow(gen, render);
  // The versions will be plugged in when they arrive (dsh --version can take up to 8 s)
  const [lver, dver] = await Promise.all([menus.launcherVersionRaw(), menus.dshVersionRaw()]);
  Object.assign(ver, { lver, dver, loaded: true });
  dialogShow(gen, render);
}

async function showUpdateCheckDialog() {
  const title = tr('Проверка обновлений', 'Update check');
  const gen = openDialog(title);
  if (gen === null) return;
  state.dialogCopyText = '';
  dialogShow(gen, () => dialogPage(title,
    '<p>' + tr('Проверяю…', 'Checking…') + '</p>' +
    '<div class="actions">' + dialogCloseButton() + '</div>'));
  const [latestL, latestD] = await Promise.all([
    timedCheck(updates.fetchLauncherLatest({ force: true }), updates.UPDATE_CHECK_TIMEOUT),
    timedCheck(updates.fetchDshLatest(), updates.UPDATE_CHECK_TIMEOUT),
  ]);
  const [lver, dver] = await Promise.all([menus.launcherVersionRaw(), menus.dshVersionRaw()]);
  const isSrc = !IS_PKG_INSTALL;
  const lSt = updateStatus(latestL && latestL.version, lver, isSrc ? 'semver' : 'deb');
  const dSt = updateStatus(latestD && latestD.version, dver, 'semver');
  // the findings — into the «Справка» menu too (no need to notify: the window is open anyway)
  const canL = lSt === 'available' && !isSrc && Boolean(lver);
  const canD = dSt === 'available' && Boolean(dver);
  if (!updates.NO_UPDATE_CHECK || canL || canD) {
    updatesLive.setAvailableUpdates({
      ...(canL ? { launcher: { version: latestL.version, installed: lver, releases: latestL.releases } } : {}),
      ...(canD ? { dsh: { version: latestD.version, installed: dver } } : {}),
    }, false);
  }
  // The dialog was closed or another opened in it — dialogShow will show nothing.
  dialogShow(gen, () => {
    let lStatus;
    if (lSt === 'nocheck') lStatus = tr('не удалось проверить (нет ответа GitHub)', 'could not check (no reply from GitHub)');
    else if (lSt === 'uptodate') lStatus = tr('актуальная версия', 'up to date');
    // 'notinstalled' (the version was not found out) — as before, we simply name the latest one
    else lStatus = tr('доступна версия <code>' + esc(latestL.version) + '</code>', 'version <code>' + esc(latestL.version) + '</code> is available');
    let dStatus;
    if (dSt === 'nocheck') dStatus = tr('не удалось проверить (нет ответа npm)', 'could not check (no reply from npm)');
    else if (dSt === 'notinstalled') dStatus = tr('dsh не установлен', 'dsh is not installed');
    else if (dSt === 'uptodate') dStatus = tr('актуальная версия', 'up to date');
    else dStatus = tr('доступна версия <code>' + esc(latestD.version) + '</code>', 'version <code>' + esc(latestD.version) + '</code> is available');
    const row = (name, ver, status) =>
      '<tr><td style="padding:3px 14px 3px 0; color:#9aa4b2; white-space:nowrap">' + esc(name) + '</td>' +
      '<td style="padding:3px 0">' +
      (ver ? tr('установлена ', 'installed ') + '<code>' + esc(ver) + '</code>' + ' — ' + status : status) +
      '</td></tr>';
    const body = `
      <table style="border-collapse:collapse; margin-bottom:14px">
        ${row('DeepSeek Harness Launcher', lver, lStatus)}
        ${row(tr('бэкенд dsh', 'dsh backend'), dver, dStatus)}
      </table>
      ${lSt === 'available' ? updates.notesBlockHtml(notesBetween(latestL.releases, lver, latestL.version, isSrc ? 'semver' : 'deb')) : ''}
      ${canL || canD ? '<p class="hint">' + tr('«Обновить» перезапустит лаунчер, dsh будет остановлен — незаконченный ответ агента прервётся.',
        '“Update” restarts the launcher and stops dsh — an unfinished agent reply will be interrupted.') + '</p>' : ''}
      <div class="actions">
        ${canL ? `<button onclick="location.href='dshlauncher://update-now/?k=launcher'">${tr('Обновить DeepSeek Harness Launcher', 'Update DeepSeek Harness Launcher')}</button>` : ''}
        ${canD ? `<button onclick="location.href='dshlauncher://update-now/?k=dsh'">${tr('Обновить dsh', 'Update dsh')}</button>` : ''}
        ${dialogCloseButton()}
      </div>`;
    dialogPage(title, body);
  });
}

Object.assign(module.exports, { openDialog, closeDialog, dialogOpen, dialogShow, dialogPage, dialogCloseButton, onDialogClose, onDialogCopy, showAboutDialog, showUpdateCheckDialog }); // we extend, not replace: the modules include each other in cycles
