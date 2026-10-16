'use strict';
// «Файл» / «Справка»: перезапуск dsh, папка логов, состояние GPU.
const { dialog, shell } = require('electron');
const fs = require('fs');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { gpuStatus } = require('./ui');
const { CONFIG, LOG_DIR } = require('./env');
const { portOpen, sleep } = require('./port'); // чистый модуль — прямой require, без реестра
const { GPU_MARKER_FILE, GPU_MARKER_TTL_MS, gpuDisabledByMarker, hasEglLibs, hasGpuRenderNode, noGpuRequested } = require('./gpu');
const dshStop = require('./dsh-stop');
const launch = require('./launch');
const pages = require('./pages');
const { noContentResponse } = require('./respond'); // чистый модуль — прямой require, без реестра

/* ---------------- «Файл» / «Справка»: перезапуск dsh, папка логов ---------------- */

async function openLogsFolder() {
  try { fs.mkdirSync(LOG_DIR, { recursive: true }); } catch { /* noop */ }
  const err = await shell.openPath(LOG_DIR);
  if (err) console.error('[launcher] не удалось открыть папку логов:', err);
}

// «Перезапустить dsh» (например, после обновления плагинов): останавливаем
// dsh, который запустил лаунчер, и запускаем заново — без вопросов об
// обновлениях. dsh, к которому лаунчер лишь подключился (запущен в
// терминале), не трогаем — у него свой хозяин.


async function restartDsh({ confirm = true } = {}) {
  if (!state.win || state.win.isDestroyed() || state.stopping || state.launching || state.restartingDsh) return;
  if (!state.weStartedDsh && !state.adoptedDsh) {
    await dialog.showMessageBox(state.win, {
      type: 'info',
      title: tr('Перезапуск dsh', 'Restart dsh'),
      message: tr('Этот dsh запущен не лаунчером', 'This dsh was not started by the launcher'),
      detail: tr('Лаунчер подключился к уже работавшему dsh — перезапустите его там, где запускали (например, в терминале).',
        'The launcher attached to a dsh that was already running — restart it where it was started (e.g. in a terminal).'),
    });
    return;
  }
  if (confirm) {
    const { response } = await dialog.showMessageBox(state.win, {
      type: 'question',
      buttons: [tr('Перезапустить', 'Restart'), tr('Отмена', 'Cancel')],
      defaultId: 0,
      cancelId: 1,
      title: tr('Перезапуск dsh', 'Restart dsh'),
      message: tr('Перезапустить dsh?', 'Restart dsh?'),
      detail: tr('Текущий ответ агента прервётся; сессии сохранятся.', 'The current agent reply will be interrupted; sessions are kept.'),
    });
    if (response !== 0) return;
  }
  console.log('[launcher] перезапуск dsh по запросу');
  state.restartingDsh = true;
  state.launching = true; // watchdog не трогает окно, пока порт падает и поднимается
  try {
    pages.showPage(() => pages.showStatus(tr('Перезапуск dsh…', 'Restarting dsh…'),
      tr('<p>Останавливаю <code>dsh</code> и запускаю заново…</p>', '<p>Stopping <code>dsh</code> and starting it again…</p>'), false));
    if (state.adoptedDsh) await dshStop.stopAdoptedDsh(); else await dshStop.stopDsh();
    for (let i = 0; i < 50 && await portOpen(); i++) await sleep(100); // порт освободился
    state.weStartedDsh = false; state.adoptedDsh = false; state.adoptedOldPgid = null; state.dshProc = null; state.dshFatal = false;
  } finally {
    state.restartingDsh = false;
    state.launching = false;
  }
  await launch.launchDshInner();
}



function gpuStatusNow() {
  let markerAgeMs = null;
  try { markerAgeMs = Date.now() - fs.statSync(GPU_MARKER_FILE).mtimeMs; } catch { /* маркера нет */ }
  return gpuStatus({
    noGpuRequested, noGpuBySettings: CONFIG.src.gpu === 'settings', disabledByMarker: gpuDisabledByMarker, hasRenderNode: hasGpuRenderNode, hasEgl: hasEglLibs,
    markerAgeMs, ttlMs: GPU_MARKER_TTL_MS,
  });
}

function gpuStatusText(g) {
  if (g.on) {
    return g.marker
      ? tr('включено; со следующего запуска будет отключено — GPU-процесс падал', 'on; will be off from the next start — the GPU process crashed')
      : tr('включено', 'on');
  }
  if (g.reason === 'env') return tr('отключено (DSH_LAUNCHER_NO_GPU=1)', 'off (DSH_LAUNCHER_NO_GPU=1)');
  if (g.reason === 'setting') return tr('отключено в «Настройках»', 'off in Settings');
  if (g.reason === 'marker') {
    return g.marker
      ? tr(`отключено после сбоев GPU-процесса (ещё ${g.daysLeft} дн.)`, `off after GPU process crashes (${g.daysLeft} more days)`)
      : tr('отключено до перезапуска лаунчера (включится при следующем запуске)', 'off until the launcher restarts (on from the next start)');
  }
  if (g.reason === 'norender') return tr('отключено (нет /dev/dri/renderD* — ВМ без 3D?)', 'off (no /dev/dri/renderD* — a VM without 3D?)');
  return tr('отключено (нет библиотек EGL)', 'off (no EGL libraries)');
}

function onDialogGpuEnable() {
  try { fs.unlinkSync(GPU_MARKER_FILE); console.log('[launcher] маркер сбоев GPU снят — GPU со следующего запуска'); }
  catch (e) { if (e.code !== 'ENOENT') console.error('[launcher] маркер GPU:', e.message); }
  if (state.dialogRender) state.dialogRender(false);
  return noContentResponse();
}

Object.assign(module.exports, { openLogsFolder, restartDsh, gpuStatusNow, gpuStatusText, onDialogGpuEnable }); // дополняем, а не заменяем: модули подключают друг друга по кругу
