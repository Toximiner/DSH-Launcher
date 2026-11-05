'use strict';
// Diagnostics: the run marker (the message after a crash) and «Сохранить отчёт…».
const { app, dialog } = require('electron');
const { execFile } = require('child_process');
const util = require('util');
const { promisify } = util;
const fs = require('fs');
const os = require('os');
const path = require('path');
const execFileAsync = promisify(execFile);
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc, redactLog } = require('./util');
const { reportFileName, reportInfoText, pickCrashDumps } = require('./ui');
const { loadSettings, CONFIG, DSH_ARGS, CWD, PORT, LOG_FILE, LAUNCHER_LOG_FILE } = require('./env');
const dialogs = require('./dialogs');
const dshLog = require('./dsh-log');
const fileHelp = require('./file-help');
const menus = require('./menus');
const updatesLive = require('./updates-live');
const updates = require('./updates');

/* ---------------- previous run: did it exit gracefully ---------------- */

// On start — the running.json marker (pid, time, version), on a graceful
// exit (will-quit) — removed. It is left and the process is gone — the
// previous run terminated abnormally (crash, kill -9, power loss): a
// toast with «Журнал…» and «Сохранить отчёт…».
const runMarkerFile = () => path.join(app.getPath('userData'), 'running.json');
let previousCrash = null;

// A previous instance is still running: the process exists, is not a
// "zombie" (finished, but the parent did not collect the exit code) and it
// is the launcher — the pid could have been reused by another program.
function launcherPidAlive(pid) {
  try {
    const stat = fs.readFileSync(`/proc/${pid}/stat`, 'utf8');
    const st = stat.slice(stat.lastIndexOf(')') + 2, stat.lastIndexOf(')') + 3);
    if (st === 'Z' || st === 'X') return false;
    return fs.readFileSync(`/proc/${pid}/cmdline`, 'utf8').includes(process.execPath);
  } catch { return false; } // the process is gone
}

function checkPreviousRun() {
  try {
    const m = JSON.parse(fs.readFileSync(runMarkerFile(), 'utf8'));
    if (Number.isInteger(m.pid) && m.pid !== process.pid && !launcherPidAlive(m.pid)) {
      previousCrash = m;
      console.log(`[launcher] прошлый запуск (pid ${m.pid}, ${m.version || '?'}, начат ${m.startedAt || '?'}) завершился аварийно`);
    }
  } catch { /* no marker — the previous run exited gracefully (or first run) */ }
  try {
    fs.mkdirSync(path.dirname(runMarkerFile()), { recursive: true });
    let version = null;
    try { version = require('../package.json').version; } catch { /* noop */ }
    fs.writeFileSync(runMarkerFile(), JSON.stringify({ pid: process.pid, startedAt: new Date().toISOString(), version }));
  } catch (e) { console.error('[launcher] running.json:', e.message); }
}

function clearRunMarker() {
  try { fs.unlinkSync(runMarkerFile()); } catch { /* noop */ }
}

/* ===== rollbackSuggested (pure function, test test/diagnostics.js) =====
   Whether to suggest a rollback to the previous version: the previous
   abnormal run — the updated version (update.to), started within
   ROLLBACK_WINDOW_MS after the update (update.at). m — the running.json
   marker (version, startedAt). Anything missing (no files, broken dates) — "do not suggest". */
const ROLLBACK_WINDOW_MS = 30 * 60 * 1000; // the «crash related to the update» window
function rollbackSuggested(m, update, now) {
  if (!m || !update) return false;
  if (String(m.version || '') !== String(update.to || '')) return false;
  const startedAt = Date.parse(m.startedAt);
  const at = Date.parse(update.at);
  if (Number.isNaN(startedAt) || Number.isNaN(at)) return false;
  const dt = now - at;
  return dt >= 0 && dt <= ROLLBACK_WINDOW_MS && startedAt >= at;
}
/* ===== end rollbackSuggested ===== */

function maybeShowCrashNotice() {
  if (!previousCrash) return;
  const m = previousCrash;
  previousCrash = null; // only once
  console.log('[launcher] уведомление: прошлый запуск завершился аварийно');
  let when = '';
  try { when = new Date(m.startedAt).toLocaleString(i18n.getLang() === 'ru' ? 'ru-RU' : 'en-US'); } catch { /* noop */ }
  const update = updates.readLastUpdate();
  const suggestRollback = rollbackSuggested(m, update, Date.now());
  if (suggestRollback) console.log(`[launcher] прошлый запуск — версия ${m.version}, установленная ${update.at} — предлагаю откат`);
  updatesLive.showToast(updatesLive.toastPageUrl({
    title: tr('В прошлый раз лаунчер завершился аварийно', 'The launcher closed unexpectedly last time'),
    sub: when ? tr(`запуск от ${when}`, `started ${when}`) : '',
    buttons: [
      ...(suggestRollback ? [{ label: tr('Вернуть предыдущую версию…', 'Roll back to the previous version…'), href: 'dshlauncher://toast/rollback/' }] : []),
      { label: tr('Журнал…', 'Log…'), href: 'dshlauncher://toast/crash-log/' },
      { label: tr('Сохранить отчёт…', 'Save report…'), href: 'dshlauncher://toast/report/' },
    ],
  }));
}

/* ---------------- report for support ---------------- */

// «Справка → Сохранить отчёт…»: a .tar.gz archive — system and version info, the
// launcher log, the tail of the dsh log, the settings; tokens and keys are
// hidden (redactLog). DSH_LAUNCHER_REPORT_PATH — where to save without a
// dialog (tests).


async function reportFacts() {
  const [lver, dver] = await Promise.all([menus.launcherVersionRaw(), menus.dshVersionRaw()]);
  let osName = '';
  try { osName = (/^PRETTY_NAME="?([^"\n]*)/m.exec(fs.readFileSync('/etc/os-release', 'utf8')) || [])[1] || ''; } catch { /* noop */ }
  return [
    ['DeepSeek Harness Launcher', lver],
    [tr('бэкенд dsh', 'dsh backend'), dver],
    ['Electron', process.versions.electron],
    ['Chromium', process.versions.chrome],
    ['Node.js', process.versions.node],
    [tr('ОС', 'OS'), osName],
    [tr('ядро', 'kernel'), `${os.release()} (${os.arch()})`],
    [tr('рабочий стол', 'desktop'), process.env.XDG_CURRENT_DESKTOP],
    [tr('сессия', 'session'), process.env.XDG_SESSION_TYPE],
    [tr('режим окна', 'window mode'), app.commandLine.getSwitchValue('ozone-platform') || 'x11'],
    [tr('GPU-ускорение', 'GPU acceleration'), fileHelp.gpuStatusText(fileHelp.gpuStatusNow())],
    [tr('язык окна', 'window language'), i18n.getLang()],
    [tr('путь к dsh', 'path to dsh'), state.dshBin],
    [tr('аргументы dsh', 'dsh arguments'), DSH_ARGS.join(' ')],
    [tr('рабочая папка', 'working folder'), CWD],
    [tr('порт', 'port'), PORT],
    [tr('откуда настройки', 'settings source'), Object.entries(CONFIG.src).map(([k, v]) => `${k}=${v}`).join(', ')],
    [tr('лаунчер работает', 'launcher uptime'), `${Math.round(process.uptime())} s`],
    [tr('отчёт создан', 'report created'), new Date().toISOString()],
  ];
}

// Electron crash dumps (*.dmp) in userData/Crashpad (crashReporter in main.js).
function listCrashDumps() {
  const out = [];
  const walk = (d, depth) => {
    let items;
    try { items = fs.readdirSync(d, { withFileTypes: true }); } catch { return; }
    for (const it of items) {
      const p = path.join(d, it.name);
      if (it.isDirectory() && depth < 3) walk(p, depth + 1);
      else if (it.isFile() && it.name.endsWith('.dmp')) {
        try { const st = fs.statSync(p); out.push({ path: p, mtimeMs: st.mtimeMs, size: st.size }); } catch { /* vanished */ }
      }
    }
  };
  let root = null;
  try { root = app.getPath('crashDumps'); } catch { /* noop */ }
  if (root) walk(root, 0);
  return out;
}

async function saveReport() {
  let dest = process.env.DSH_LAUNCHER_REPORT_PATH || null;
  if (!dest) {
    const parent = state.win; // the dialog — a layer of the main window
    let dir = os.homedir();
    try { dir = app.getPath('downloads'); } catch { /* noop */ }
    const r = await dialog.showSaveDialog(parent, {
      title: tr('Сохранить отчёт', 'Save report'),
      defaultPath: path.join(dir, reportFileName(new Date())),
      filters: [{ name: 'tar.gz', extensions: ['gz'] }],
    });
    if (r.canceled || !r.filePath) return;
    dest = r.filePath;
  }
  let tmp = null;
  try {
    tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'dsh-launcher-report-'));
    const dir = path.join(tmp, 'dsh-launcher-report');
    fs.mkdirSync(dir);
    const put = (name, text) => fs.writeFileSync(path.join(dir, name), redactLog(text));
    put('info.txt', reportInfoText(await reportFacts()));
    for (const f of [LAUNCHER_LOG_FILE + '.1', LAUNCHER_LOG_FILE]) {
      try { put(path.basename(f), fs.readFileSync(f, 'utf8')); } catch { /* no file */ }
    }
    const dshTail = dshLog.logTail(LOG_FILE, 5000, 1024 * 1024);
    if (dshTail !== null) put('dsh.log.tail', dshTail + '\n');
    put('settings.json', JSON.stringify(loadSettings(), null, 2) + '\n');
    try { put('gpu.json', JSON.stringify(app.getGPUFeatureStatus(), null, 2) + '\n'); } catch { /* noop */ }
    // crash dumps (binary, as-is): the last 30 days
    const dumps = pickCrashDumps(listCrashDumps(), Date.now());
    if (dumps.length) {
      fs.mkdirSync(path.join(dir, 'crash-dumps'));
      for (const d of dumps) fs.copyFileSync(d.path, path.join(dir, 'crash-dumps', path.basename(d.path)));
    }
    console.log(`[launcher] отчёт: дампов падений — ${dumps.length}`);
    await execFileAsync('tar', ['-czf', dest, '-C', tmp, 'dsh-launcher-report']);
    state.lastReportPath = dest;
    console.log(`[launcher] отчёт сохранён: ${dest}`);
    showReportDialog(null);
  } catch (e) {
    console.error('[launcher] отчёт:', e.message);
    showReportDialog(e.message);
  } finally {
    if (tmp) try { fs.rmSync(tmp, { recursive: true, force: true }); } catch { /* noop */ }
  }
}

function showReportDialog(error) {
  const title = tr('Отчёт', 'Report');
  const gen = dialogs.openDialog(title);
  if (gen === null) return;
  state.dialogCopyText = '';
  dialogs.dialogShow(gen, () => dialogs.dialogPage(title, error
    ? `<p class="note err">${esc(tr('Не удалось сохранить отчёт: ', 'Could not save the report: ') + error)}</p>
       <div class="actions">${dialogs.dialogCloseButton()}</div>`
    : `<p>${tr('Отчёт сохранён:', 'Report saved:')} <code>${esc(state.lastReportPath)}</code></p>
       <p class="hint">${tr('Токены и ключи в нём скрыты. Приложите файл к сообщению о проблеме на',
                            'Tokens and keys in it are hidden. Attach the file to an issue on')}
         <a href="https://github.com/${updates.UPDATE_GITHUB_REPO}/issues/new" target="_blank">GitHub</a>.</p>
       <div class="actions">
         <button onclick="location.href='dshlauncher://dialog/report-show/'">${tr('Показать в папке', 'Show in folder')}</button>
         ${dialogs.dialogCloseButton()}
       </div>`));
}

Object.assign(module.exports, { checkPreviousRun, clearRunMarker, maybeShowCrashNotice, saveReport, rollbackSuggested }); // we extend, not replace: the modules include each other in cycles
