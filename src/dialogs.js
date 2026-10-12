'use strict';
// Окно-диалог поверх главного: открытие, содержимое, закрытие, «О программе», «Проверить обновления…».
const { WebContentsView, shell, clipboard } = require('electron');
const os = require('os');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc, timedCheck } = require('./util');
const { updateStatus, notesBetween } = require('./versions');
const { LOG_DIR } = require('./env');
const fileHelp = require('./file-help');
const menus = require('./menus');
const { isAltKey } = require('./ui');
const pages = require('./pages');
const updatesLive = require('./updates-live');
const updates = require('./updates');
const launcher = require('./launcher'); // общий реестр (наполняет main.js)

/* ---------------- окно-диалог: «О программе», «Проверить обновления» ---------------- */

// Диалог — слой внутри главного окна (WebContentsView во всю его площадь):
// затемнение и карточка по центру, как у панели поиска и плашек. Отдельного
// окна ОС нет — в панели задач и списке окон ничего лишнего, на Wayland
// композитор не решает, где его показать. GUI dsh под слоем не
// перезагружается и не теряет состояние. Один слой на все пункты: повторный
// вызов меняет содержимое. Закрыть — кнопкой или Esc (клик мимо карточки
// диалог не закрывает — легко промахнуться).
// Кнопки — переходы на dshlauncher://dialog/… (ответ 204: страница остаётся,
// обработчик делает своё).
let dialogGen = 0; // поколение: поздний async-результат не попадёт в закрытый или сменившийся диалог

const dialogOpen = () => Boolean(state.dialogView && !state.dialogView.webContents.isDestroyed());

function placeDialog() {
  if (!dialogOpen() || !state.win || state.win.isDestroyed()) return;
  const [w, h] = state.win.getContentSize();
  state.dialogView.setBounds({ x: 0, y: 0, width: w, height: h });
}

// Открыть диалог (или переиспользовать открытый) — поколение нового содержимого,
// null — окна нет / идёт выход.
function openDialog(title) {
  if (!state.win || state.win.isDestroyed() || state.stopping) return null;
  dialogGen++;
  if (dialogOpen()) { state.dialogView.webContents.focus(); return dialogGen; }
  const v = new WebContentsView({ webPreferences: { contextIsolation: true, nodeIntegration: false, spellcheck: false } });
  v.setBackgroundColor('#00000000'); // затемнение рисует страница слоя
  state.dialogView = v;
  const wc = v.webContents;
  wc.setWindowOpenHandler(({ url }) => {
    if (/^https?:/i.test(url)) shell.openExternal(url);
    return { action: 'deny' };
  });
  wc.on('context-menu', (_e, params) => menus.showContextMenu(params, state.win, wc));
  wc.on('before-input-event', (e, input) => {
    if (input.type === 'keyDown' && input.key === 'Escape') { e.preventDefault(); closeDialog(); }
    if (isAltKey(input)) e.preventDefault(); // фокус не в строку меню (см. main.js)
  });
  wc.on('did-finish-load', () => { if (!wc.isDestroyed()) wc.focus(); });
  state.win.contentView.addChildView(v); // поверх всего: GUI, поиска, плашек
  placeDialog();
  if (!resizeHooked) { state.win.on('resize', placeDialog); resizeHooked = true; }
  console.log(`[launcher] диалог: ${title}`);
  return dialogGen;
}
let resizeHooked = false;

function closeDialog() {
  if (!dialogOpen()) return;
  const v = state.dialogView;
  state.dialogView = null; state.dialogRender = null; dialogGen++;
  if (state.win && !state.win.isDestroyed()) {
    state.win.contentView.removeChildView(v);
    state.win.webContents.focus();
  }
  try { v.webContents.close(); } catch { /* уже закрыт */ }
}

// Показать содержимое, если диалог того же поколения ещё открыт.
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
  return launcher.noContentResponse();
}

function onDialogCopy() {
  if (state.dialogCopyText) clipboard.writeText(state.dialogCopyText);
  if (state.dialogRender) state.dialogRender(true); // перерисовка: «Скопировано»
  return launcher.noContentResponse();
}

async function showAboutDialog() {
  const title = tr('О программе', 'About');
  const gen = openDialog(title);
  if (gen === null) return;
  const ver = { lver: null, dver: null, loaded: false };
  const render = (copied) => {
    const gpu = fileHelp.gpuStatusNow();
    const fromSrc = !process.execPath.startsWith('/opt/dsh-launcher/');
    const lLabel = fromSrc ? tr(' (сборка из исходников)', ' (built from source)') : '';
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
  // Версии подставим, когда придут (dsh --version может занять до 8 с)
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
  const isSrc = !process.execPath.startsWith('/opt/dsh-launcher/');
  const lSt = updateStatus(latestL && latestL.version, lver, isSrc ? 'semver' : 'deb');
  const dSt = updateStatus(latestD && latestD.version, dver, 'semver');
  // найденное — и в меню «Справка» (уведомлять не нужно: окно и так открыто)
  const canL = lSt === 'available' && !isSrc && Boolean(lver);
  const canD = dSt === 'available' && Boolean(dver);
  if (!updates.NO_UPDATE_CHECK || canL || canD) {
    updatesLive.setAvailableUpdates({
      ...(canL ? { launcher: { version: latestL.version, installed: lver, releases: latestL.releases } } : {}),
      ...(canD ? { dsh: { version: latestD.version, installed: dver } } : {}),
    }, false);
  }
  // Диалог закрыли или открыли в нём другое — dialogShow ничего не покажет.
  dialogShow(gen, () => {
    let lStatus;
    if (lSt === 'nocheck') lStatus = tr('не удалось проверить (нет ответа GitHub)', 'could not check (no reply from GitHub)');
    else if (lSt === 'uptodate') lStatus = tr('актуальная версия', 'up to date');
    // 'notinstalled' (версию не узнали) — как и раньше, просто называем последнюю
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

Object.assign(module.exports, { openDialog, closeDialog, dialogOpen, dialogShow, dialogPage, dialogCloseButton, onDialogClose, onDialogCopy, showAboutDialog, showUpdateCheckDialog }); // дополняем, а не заменяем: модули подключают друг друга по кругу
