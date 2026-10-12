'use strict';
// Диагностика: метка запуска (сообщение после сбоя) и «Сохранить отчёт…».
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

/* ---------------- прошлый запуск: завершился ли штатно ---------------- */

// При запуске — метка running.json (pid, время, версия), при штатном выходе
// (will-quit) — удаляется. Осталась, а процесса нет — прошлый запуск
// завершился аварийно (падение, kill -9, выключение питания): плашка с
// «Журнал…» и «Сохранить отчёт…».
const runMarkerFile = () => path.join(app.getPath('userData'), 'running.json');
let previousCrash = null;

// Прежний экземпляр ещё работает: процесс есть, не «зомби» (завершился, но
// родитель не забрал код выхода) и это лаунчер — pid мог достаться другой
// программе.
function launcherPidAlive(pid) {
  try {
    const stat = fs.readFileSync(`/proc/${pid}/stat`, 'utf8');
    const st = stat.slice(stat.lastIndexOf(')') + 2, stat.lastIndexOf(')') + 3);
    if (st === 'Z' || st === 'X') return false;
    return fs.readFileSync(`/proc/${pid}/cmdline`, 'utf8').includes(process.execPath);
  } catch { return false; } // процесса нет
}

function checkPreviousRun() {
  try {
    const m = JSON.parse(fs.readFileSync(runMarkerFile(), 'utf8'));
    if (Number.isInteger(m.pid) && m.pid !== process.pid && !launcherPidAlive(m.pid)) {
      previousCrash = m;
      console.log(`[launcher] прошлый запуск (pid ${m.pid}, ${m.version || '?'}, начат ${m.startedAt || '?'}) завершился аварийно`);
    }
  } catch { /* метки нет — прошлый завершился штатно (или первый запуск) */ }
  try {
    fs.mkdirSync(path.dirname(runMarkerFile()), { recursive: true });
    let version = null;
    try { version = require('./package.json').version; } catch { /* noop */ }
    fs.writeFileSync(runMarkerFile(), JSON.stringify({ pid: process.pid, startedAt: new Date().toISOString(), version }));
  } catch (e) { console.error('[launcher] running.json:', e.message); }
}

function clearRunMarker() {
  try { fs.unlinkSync(runMarkerFile()); } catch { /* noop */ }
}

function maybeShowCrashNotice() {
  if (!previousCrash) return;
  const m = previousCrash;
  previousCrash = null; // один раз
  console.log('[launcher] уведомление: прошлый запуск завершился аварийно');
  let when = '';
  try { when = new Date(m.startedAt).toLocaleString(i18n.getLang() === 'ru' ? 'ru-RU' : 'en-US'); } catch { /* noop */ }
  updatesLive.showToast(updatesLive.toastPageUrl({
    title: tr('В прошлый раз лаунчер завершился аварийно', 'The launcher closed unexpectedly last time'),
    sub: when ? tr(`запуск от ${when}`, `started ${when}`) : '',
    buttons: [
      { label: tr('Журнал…', 'Log…'), href: 'dshlauncher://toast/crash-log/' },
      { label: tr('Сохранить отчёт…', 'Save report…'), href: 'dshlauncher://toast/report/' },
    ],
  }));
}

/* ---------------- отчёт для поддержки ---------------- */

// «Справка → Сохранить отчёт…»: архив .tar.gz — сведения о системе и
// версиях, журнал лаунчера, хвост журнала dsh, настройки; токены и ключи
// скрыты (redactLog). DSH_LAUNCHER_REPORT_PATH — куда сохранить без диалога
// (тесты).


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

// Дампы падений Electron (*.dmp) в userData/Crashpad (crashReporter в main.js).
function listCrashDumps() {
  const out = [];
  const walk = (d, depth) => {
    let items;
    try { items = fs.readdirSync(d, { withFileTypes: true }); } catch { return; }
    for (const it of items) {
      const p = path.join(d, it.name);
      if (it.isDirectory() && depth < 3) walk(p, depth + 1);
      else if (it.isFile() && it.name.endsWith('.dmp')) {
        try { const st = fs.statSync(p); out.push({ path: p, mtimeMs: st.mtimeMs, size: st.size }); } catch { /* исчез */ }
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
    const parent = state.win; // диалог — слой главного окна
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
      try { put(path.basename(f), fs.readFileSync(f, 'utf8')); } catch { /* нет файла */ }
    }
    const dshTail = dshLog.logTail(LOG_FILE, 5000, 1024 * 1024);
    if (dshTail !== null) put('dsh.log.tail', dshTail + '\n');
    put('settings.json', JSON.stringify(loadSettings() || {}, null, 2) + '\n');
    try { put('gpu.json', JSON.stringify(app.getGPUFeatureStatus(), null, 2) + '\n'); } catch { /* noop */ }
    // дампы падений (бинарные, как есть): последние за 30 дней
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

Object.assign(module.exports, { checkPreviousRun, clearRunMarker, maybeShowCrashNotice, saveReport }); // дополняем, а не заменяем: модули подключают друг друга по кругу
