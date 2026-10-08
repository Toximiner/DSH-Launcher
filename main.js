'use strict';

/*
 * dsh-launcher — окно, которое ведёт процесс DeepSeek Harness.
 *
 *  - если порт 3080 не слушается, запускает `dsh --profile web --no-open`;
 *  - строку `dsh web: http://127.0.0.1:3080/?token=...`, которую dsh печатает
 *    в stdout, подхватывает и открывает в окне — так выдаётся сессийный куки,
 *    который запоминается в профиле приложения;
 *  - если dsh уже запущен (например, из терминала), подключается к нему и НЕ
 *    убивает его при закрытии; если валидного куки нет, просит вставить URL
 *    с токеном из терминала, где работает dsh;
 *  - закрытие окна завершает всё дерево процессов dsh, которое запустил лаунчер
 *    (и dsh, «усыновлённый» после самовозрождения — он ищется по порту):
 *    сначала SIGTERM, через 4 с — SIGKILL;
 *  - SIGTERM/SIGINT/SIGHUP самому лаунчеру: штатное закрытие (как закрытие окна);
 *    повторный сигнал или зависание дольше 8 с — SIGKILL дереву dsh. Прямой
 *    SIGKILL лаунчеру перехватить нельзя — тот случай остаётся незакрытым.
 */

const { app, BrowserWindow, Menu, protocol, net: electronNet, shell, clipboard } = require('electron');
const { spawn, execFile, execFileSync } = require('child_process');
const { promisify } = require('util');
const nodeNet = require('net');
const fs = require('fs');
const os = require('os');
const path = require('path');

const execFileAsync = promisify(execFile);

// Разбор строки аргументов как в shell: пробелы разделяют, '…' и "…"
// группируют, \ экранирует следующий символ (внутри '…' — нет).
function splitArgs(str) {
  const out = [];
  let cur = '';
  let has = false; // был ли токен (в т.ч. пустой '' / "")
  let quote = null;
  for (let i = 0; i < str.length; i++) {
    const c = str[i];
    if (quote) {
      if (c === quote) quote = null;
      else if (c === '\\' && quote === '"' && i + 1 < str.length) cur += str[++i];
      else cur += c;
    } else if (c === "'" || c === '"') {
      quote = c; has = true;
    } else if (c === '\\' && i + 1 < str.length) {
      cur += str[++i]; has = true;
    } else if (/\s/.test(c)) {
      if (has) { out.push(cur); cur = ''; has = false; }
    } else {
      cur += c; has = true;
    }
  }
  if (has) out.push(cur);
  return out;
}

/* ========================= настройки ========================= */

const DSH_BIN_DEFAULT = '/usr/bin/dsh';
const DSH_BIN_EXPLICIT = Boolean(process.env.DSH_BIN);
// путь к бинарнику dsh; если DSH_BIN не задан и /usr/bin/dsh нет — ищется
// в PATH, npm prefix, nvm и т.п. (см. findDsh), найденный путь запоминается
let DSH_BIN = process.env.DSH_BIN || DSH_BIN_DEFAULT;
const DSH_ARGS = process.env.DSH_ARGS
  ? splitArgs(process.env.DSH_ARGS)
  : ['--profile', 'web', '--no-open'];
const CWD = process.env.DSH_CWD || os.homedir(); // рабочая директория для dsh
const HOST = '127.0.0.1';
const PORT = Number(process.env.DSH_PORT || 3080);
const PLAIN_URL = `http://${HOST}:${PORT}/`;
const START_TIMEOUT = Number(process.env.DSH_START_TIMEOUT_MS || 120000); // сколько ждать старта
// в установленной версии (run.sh ставит DSH_LOG_DIR) — в ~/.local/state,
// в dev-режиме — рядом с приложением
const LOG_DIR = process.env.DSH_LOG_DIR || path.join(__dirname, 'logs');
const LOG_FILE = path.join(LOG_DIR, 'dsh.log');

// Язык окон лаунчера: DSH_LAUNCHER_LANG=ru|en, иначе выбор переключателем
// RU | EN в окне (userData/ui-lang.json, подхватывается в onReady), иначе по
// локали системы (LC_ALL / LC_MESSAGES / LANG): ru* — русский, иначе английский.
const LANG_FORCED = (() => {
  const v = String(process.env.DSH_LAUNCHER_LANG || '').toLowerCase();
  return v === 'ru' || v === 'en' ? v : null;
})();
let UI_LANG = LANG_FORCED || (/^ru/i.test(process.env.LC_ALL || process.env.LC_MESSAGES || process.env.LANG || '') ? 'ru' : 'en');
const tr = (ru, en) => (UI_LANG === 'ru' ? ru : en);

/* ============================================================== */

app.setName('dsh-launcher');

// Диагностика сессии в лог — первое, что смотреть, если «окно не появляется».
if (process.platform === 'linux') {
  console.log(
    `[launcher] сессия: XDG_SESSION_TYPE=${process.env.XDG_SESSION_TYPE || '(пусто)'} ` +
    `WAYLAND_DISPLAY=${process.env.WAYLAND_DISPLAY || '(пусто)'} DISPLAY=${process.env.DISPLAY || '(пусто)'}`
  );
}

// Есть ли в системе реальный GPU (render-узлы /dev/dri/renderD*).
// ВМ/VNC без 3D-ускорения их не имеют.
const hasGpuRenderNode = (() => {
  try { return fs.readdirSync('/dev/dri').some((f) => /^renderD/.test(f)); }
  catch { return false; }
})();

// Окно на Wayland предъявляется через EGL. Если у GPU виртуалки нет рабочего
// драйвера (ANGLE/EGL-ошибки в логе), окно «показано» по логам Electron, но на
// экране его не видно. X11/XWayland-путь деградирует до программного рендера —
// окно остаётся видимым. На X11-сессиях ничего не меняем.
// Если реальный GPU есть (renderD* в /dev/dri) — не форсим X11: нативный
// Wayland работает, а X11/XWayland-окно на KWin остаётся невидимым. Ручной
// переключатель: DSH_LAUNCHER_OZONE=x11|wayland.
const isWaylandSession = process.platform === 'linux' &&
  (String(process.env.XDG_SESSION_TYPE || '').toLowerCase() === 'wayland' ||
   Boolean(process.env.WAYLAND_DISPLAY));
if (isWaylandSession) {
  // Ошибку в значении DSH_LAUNCHER_OZONE не передаём в Electron:
  // неизвестный ozone-platform может не инициализироваться вовсе.
  const envOzone = String(process.env.DSH_LAUNCHER_OZONE || '').toLowerCase();
  const ozoneManual = envOzone === 'x11' || envOzone === 'wayland';
  const ozone = ozoneManual ? envOzone : hasGpuRenderNode ? 'wayland' : 'x11';
  app.commandLine.appendSwitch('ozone-platform', ozone);
  console.log(
    `[launcher] Wayland-сессия — ozone-platform=${ozone}` +
    (ozoneManual
      ? ' (принудительно: DSH_LAUNCHER_OZONE)'
      : hasGpuRenderNode
        ? ' (есть реальный GPU — нативный Wayland; X11/XWayland-окно на KWin невидимо)'
        : ' (render-узлов нет — окно веду через X11 (XWayland))')
  );
}

// ВМ/VNC без 3D-ускорения: render-узлов нет, EGL гарантированно не поднимется —
// идём сразу в программный рендер, минуя цикл падений GPU-процесса (ANGLE/EGL
// ошибки в логе). То же, если библиотек EGL в системе нет вовсе. Ручной
// переключатель: DSH_LAUNCHER_NO_GPU=1.
const hasEglLibs = (() => {
  try {
    const out = execFileSync('ldconfig', ['-p'], { encoding: 'utf8', timeout: 2000 });
    return /libEGL\.so\.1/.test(out) && /libGLESv2\.so\.[12]/.test(out);
  } catch { return true; } // не удалось проверить — решение не меняем
})();

// Маркер «GPU-драйвер сломан». Сломанный драйвер (SIGSEGV в GPU-процессе) на
// старте надёжно не определяется — но падение видно в рантайме. Запоминаем его
// в файл: следующие запуски сразу идут в программный рендер. Маркер живёт 30
// дней (на случай, если драйвер починили); чтобы вернуть GPU раньше — удалить
// файл.
const GPU_MARKER_FILE = path.join(LOG_DIR, 'gpu-disabled.marker');
const GPU_MARKER_TTL_MS = 30 * 24 * 60 * 60 * 1000;
const gpuMarkerAgeMs = (() => {
  try { return Date.now() - fs.statSync(GPU_MARKER_FILE).mtimeMs; }
  catch { return null; }
})();
const gpuDisabledByMarker = process.platform === 'linux' &&
  gpuMarkerAgeMs !== null && gpuMarkerAgeMs < GPU_MARKER_TTL_MS;

const noGpuRequested = process.env.DSH_LAUNCHER_NO_GPU === '1';
if (process.platform === 'linux' && (noGpuRequested || gpuDisabledByMarker || !hasGpuRenderNode || !hasEglLibs)) {
  app.disableHardwareAcceleration();
  const why = noGpuRequested ? 'DSH_LAUNCHER_NO_GPU=1'
    : gpuDisabledByMarker ? `маркер ${GPU_MARKER_FILE} (GPU-процесс падал, маркер живёт 30 дней; удалить файл, чтобы включить GPU)`
    : !hasGpuRenderNode ? 'render-узлов в /dev/dri нет'
    : 'EGL-библиотек (libEGL/libGLESv2) в системе нет';
  console.log(`[launcher] ${why} — отключаю GPU-ускорение (программный рендер)`);
}

// Две и более смерти GPU-процесса за сессию = драйвер сломан: пишем маркер,
// чтобы следующие запуски сразу обходили GPU.
let gpuCrashCount = 0;
app.on('child-process-gone', (_event, details) => {
  if (details.type !== 'GPU' || (details.reason !== 'crashed' && details.reason !== 'abnormal-exit')) return;
  gpuCrashCount += 1;
  console.log(`[launcher] GPU-процесс завершился (reason=${details.reason}, code=${details.exitCode}, #${gpuCrashCount})`);
  if (gpuCrashCount === 2 && !gpuDisabledByMarker) {
    try {
      fs.mkdirSync(LOG_DIR, { recursive: true });
      fs.writeFileSync(GPU_MARKER_FILE, `GPU process crashed x2, reason=${details.reason}, ${new Date().toISOString()}\n`);
      console.log(`[launcher] GPU-драйвер, похоже, сломан — следующий запуск пойдёт в программном рендере (маркер ${GPU_MARKER_FILE})`);
    } catch (e) {
      console.log('[launcher] не удалось записать GPU-маркер:', e.message);
    }
  }
});

let win = null;
let dshProc = null;
let weStartedDsh = false;
let adoptedDsh = false; // dsh «усыновлён» после самовозрождения — тоже останавливаем при закрытии
let adoptedOldPgid = null; // группа исходного dsh до самовозрождения (могли остаться потомки)
let dshFatal = false;
let stopping = false;
let authUrl = null; // URL с токеном, напечатанный dsh
const stderrTail = [];

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function pushErr(line) {
  const t = line.trim();
  if (!t) return;
  stderrTail.push(t);
  if (stderrTail.length > 300) stderrTail.shift();
  console.error('[dsh]', t);
}

function portOpen() {
  return new Promise((resolve) => {
    const s = new nodeNet.Socket();
    const done = (ok) => {
      try { s.destroy(); } catch { /* noop */ }
      resolve(ok);
    };
    s.setTimeout(800);
    s.once('connect', () => done(true));
    s.once('timeout', () => done(false));
    s.once('error', () => done(false));
    s.connect(PORT, HOST);
  });
}

async function waitForPort() {
  const deadline = Date.now() + START_TIMEOUT;
  while (Date.now() < deadline) {
    if (await portOpen()) return true;
    if (dshProc && (dshProc.exitCode !== null || dshFatal)) return false;
    await sleep(400);
  }
  return portOpen();
}

async function waitForAuthUrl(timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  while (!authUrl && Date.now() < deadline) await sleep(100);
  return authUrl;
}

/* ---------------- проверка установки dsh ---------------- */

// Окружение для запуска dsh: каталог бинарника — первым в PATH. dsh — это
// `#!/usr/bin/env node`-скрипт; установленный через nvm, он должен получить
// node из того же каталога, а при запуске с ярлыка nvm в PATH нет.
function dshEnv(extra) {
  return envWithBinDir(DSH_BIN, extra);
}

async function probeDsh(bin) {
  if (!bin || !fs.existsSync(bin)) return { ok: false, reason: 'missing' };
  try {
    const { stdout: out } = await execFileAsync(bin, ['--version'], { encoding: 'utf8', timeout: 8000, env: envWithBinDir(bin) });
    return { ok: true, version: (out.trim().split('\n').pop() || '').trim() };
  } catch (e) {
    // Строка с самой ошибкой полезнее хвоста стектрейса («at …», «Node.js vX»).
    const lines = String(e.stderr || e.message || '').trim().split('\n').map((l) => l.trim()).filter(Boolean);
    const errLine = lines.find((l) => !l.startsWith('at ') && /error|ERR_/i.test(l));
    const detail = (errLine || lines.slice(-3).join(' ')).slice(0, 300);
    return { ok: false, reason: 'broken', detail };
  }
}

const dshBinPrefFile = () => path.join(app.getPath('userData'), 'dsh-bin.json');

// bin-каталоги всех версий node из nvm, свежие первыми.
function nvmBinDirs() {
  const nvmDir = process.env.NVM_DIR || path.join(os.homedir(), '.nvm');
  try {
    return fs.readdirSync(path.join(nvmDir, 'versions', 'node'))
      .sort((a, b) => b.localeCompare(a, undefined, { numeric: true }))
      .map((v) => path.join(nvmDir, 'versions', 'node', v, 'bin'));
  } catch { return []; } // nvm нет
}

// Окружение с каталогом бинарника первым в PATH (для `#!/usr/bin/env node`-скриптов).
function envWithBinDir(bin, extra) {
  return { ...process.env, PATH: path.dirname(bin) + path.delimiter + (process.env.PATH || ''), ...extra };
}

// Исполняемый файл: рядом с dsh, в PATH, иначе в nvm (при запуске с ярлыка
// nvm в PATH нет).
function findBin(name) {
  const dirs = [path.dirname(DSH_BIN)]
    .concat(String(process.env.PATH || '').split(path.delimiter).filter(Boolean), nvmBinDirs());
  for (const dir of dirs) {
    const bin = path.join(dir, name);
    try { fs.accessSync(bin, fs.constants.X_OK); return bin; } catch { /* дальше */ }
  }
  return null;
}
const findNpm = () => findBin('npm');

// Где искать dsh, если DSH_BIN не задан: запомненный путь, /usr/bin/dsh,
// PATH, типичные каталоги пользовательских npm-установок, все версии nvm
// (свежие первыми). npm prefix -g — последним, он медленный.
function dshCandidates() {
  const home = os.homedir();
  const list = [];
  try { list.push(JSON.parse(fs.readFileSync(dshBinPrefFile(), 'utf8')).bin); } catch { /* не запоминали */ }
  list.push(DSH_BIN_DEFAULT);
  for (const dir of String(process.env.PATH || '').split(path.delimiter)) if (dir) list.push(path.join(dir, 'dsh'));
  list.push(path.join(home, '.npm-global', 'bin', 'dsh'), path.join(home, '.local', 'bin', 'dsh'), '/usr/local/bin/dsh');
  for (const dir of nvmBinDirs()) list.push(path.join(dir, 'dsh'));
  return [...new Set(list.filter(Boolean))];
}

// Проверяем не только наличие файла, но и что бинарник реально запускается:
// битый symlink, несовместимый Node или отсутствующий npm дадут ошибку здесь,
// а не загадочным ENOENT в момент старта. dsh --version занимает ~60 мс.
// Асинхронно: при медленной системе синхронный вызов замораживал бы окно.
// Если DSH_BIN не задан явно — перебираем кандидатов (dshCandidates) и
// переключаем DSH_BIN на первый рабочий, запоминая его в userData/dsh-bin.json.
async function dshInstallCheck() {
  if (DSH_BIN_EXPLICIT) return probeDsh(DSH_BIN);
  let firstBroken = null;
  const tryBin = async (bin) => {
    const r = await probeDsh(bin);
    if (r.ok) {
      if (bin !== DSH_BIN) console.log(`[launcher] dsh найден: ${bin} (${r.version})`);
      DSH_BIN = bin;
      try {
        fs.mkdirSync(path.dirname(dshBinPrefFile()), { recursive: true });
        fs.writeFileSync(dshBinPrefFile(), JSON.stringify({ bin }));
      } catch (e) { console.error('[launcher] dsh-bin.json:', e.message); }
    } else if (r.reason === 'broken' && !firstBroken) {
      firstBroken = { ...r, bin };
    }
    return r.ok ? r : null;
  };
  for (const bin of dshCandidates()) {
    const r = await tryBin(bin);
    if (r) return r;
  }
  const npm = findNpm();
  if (npm) {
    try {
      const prefix = (await execFileAsync(npm, ['prefix', '-g'], { encoding: 'utf8', timeout: 10000, env: envWithBinDir(npm) })).stdout.trim();
      const r = await tryBin(path.join(prefix, 'bin', 'dsh'));
      if (r) return r;
    } catch { /* npm не отвечает */ }
  }
  if (firstBroken) {
    DSH_BIN = firstBroken.bin; // показываем на странице тот, что сломан
    return { ok: false, reason: 'broken', detail: firstBroken.detail };
  }
  DSH_BIN = DSH_BIN_DEFAULT;
  return { ok: false, reason: 'missing' };
}

// Есть ли npm и доступен ли глобальный prefix без sudo — от этого зависит,
// можно ли ставить dsh кнопкой прямо из окна.
const INSTALL_CMD = 'npm install -g @deepseek-ai/dsh';
// Право записи проверяем у ближайшего существующего каталога: на чистой
// системе <prefix>/lib/node_modules ещё нет (появится при первой глобальной
// установке), и ENOENT от него раньше ошибочно означал «npm нет».
function writableOrCreatable(dir) {
  while (!fs.existsSync(dir) && path.dirname(dir) !== dir) dir = path.dirname(dir);
  try { fs.accessSync(dir, fs.constants.W_OK); return true; } catch { return false; }
}

// dsh нужен Node.js 22+. npm ставит его и на более старый Node без единого
// предупреждения (Ubuntu 24.04: Node 18 из apt), а потом dsh не запускается.
const MIN_NODE_MAJOR = 22;

async function nodeVersionNear(npm) {
  try {
    const { stdout } = await execFileAsync('node', ['--version'], { encoding: 'utf8', timeout: 5000, env: envWithBinDir(npm) });
    return stdout.trim();
  } catch { return null; }
}

async function npmInstallInfo() {
  const npm = findNpm();
  if (!npm) return { hasNpm: false, canAuto: false, command: INSTALL_CMD };
  const nodeVersion = await nodeVersionNear(npm);
  const major = Number((/^v(\d+)/.exec(nodeVersion || '') || [])[1]);
  const nodeOld = Number.isFinite(major) && major < MIN_NODE_MAJOR;
  try {
    const prefix = (await execFileAsync(npm, ['prefix', '-g'], { encoding: 'utf8', timeout: 10000, env: envWithBinDir(npm) })).stdout.trim();
    const writable = writableOrCreatable(path.join(prefix, 'lib', 'node_modules')) &&
      writableOrCreatable(path.join(prefix, 'bin'));
    return { hasNpm: true, canAuto: writable && !nodeOld, nodeVersion, nodeOld, npm, prefix,
      command: writable ? INSTALL_CMD : 'sudo ' + INSTALL_CMD };
  } catch (e) {
    console.error('[launcher] npm prefix -g:', e.message);
    return { hasNpm: true, canAuto: false, nodeVersion, nodeOld, npm, command: 'sudo ' + INSTALL_CMD };
  }
}

/* ---------------- запуск процесса dsh ---------------- */

function startDsh() {
  // Простая ротация: если dsh.log разросся больше MAX — сдвигаем в dsh.log.1.
  // Файл пишет только startDsh (stdout dsh), поэтому ротируем перед открытием потока.
  const MAX_LOG_BYTES = 2 * 1024 * 1024; // 2 МБ
  try {
    const st = fs.statSync(LOG_FILE);
    if (st.size > MAX_LOG_BYTES) {
      fs.renameSync(LOG_FILE, LOG_FILE + '.1');
      console.log(`[launcher] лог прокручен: dsh.log (${st.size} Б) -> dsh.log.1`);
    }
  } catch { /* файла ещё нет — ротировать нечего */ }
  fs.mkdirSync(LOG_DIR, { recursive: true });
  const logStream = fs.createWriteStream(LOG_FILE, { flags: 'a' });
  authUrl = null; // токен прошлого процесса недействителен — ждём новый
  console.log(`[launcher] starting: ${DSH_BIN} ${DSH_ARGS.join(' ')} (cwd=${CWD}, log=${LOG_FILE})`);
  dshProc = spawn(DSH_BIN, DSH_ARGS, {
    cwd: CWD,
    env: dshEnv({ DSH_STARTED_BY: 'dsh-launcher' }),
    stdio: ['ignore', 'pipe', 'pipe'],
    detached: true, // dsh становится лидером своей процесс-группы — можно убить всё дерево
  });
  weStartedDsh = true;
  adoptedDsh = false; // снова собственный процесс — «усыновлённого» больше нет
  adoptedOldPgid = null;
  dshFatal = false;
  dshProc.stdout.on('data', (d) => {
    for (const line of d.toString().split('\n')) {
      if (!line.trim()) continue;
      console.log('[dsh]', line.trim());
      logStream.write(line + '\n');
      const m = /dsh web:\s*(https?:\/\/\S+)/.exec(line) ||
                /(https?:\/\/[^\s]*\?token=[^\s&]+)/.exec(line);
      if (m) {
        console.log('[launcher] captured auth URL');
        authUrl = m[1];
      }
    }
  });
  dshProc.stderr.on('data', (d) => {
    for (const line of d.toString().split('\n')) {
      pushErr(line);
      logStream.write(line + '\n');
    }
  });
  dshProc.on('error', (err) => {
    dshFatal = true;
    pushErr(tr(`ошибка запуска dsh: ${err.message}`, `failed to start dsh: ${err.message}`));
  });
  dshProc.on('exit', (code, signal) => {
    try { logStream.end(); } catch { /* noop */ }
    handleDshExit(code, signal);
  });
}

/* ---------------- остановка дерева процессов ---------------- */

// groupOnly: главный процесс уже завершился — его pid мог занять чужой
// процесс, поэтому без запасного варианта «просто pid», только группа (-pid).
function killGroup(pid, signal, groupOnly = false) {
  try { process.kill(-pid, signal); } catch {
    if (groupOnly) return;
    try { process.kill(pid, signal); } catch { /* noop */ }
  }
}

function groupAlive(pid, groupOnly = false) {
  try { process.kill(-pid, 0); return true; } catch { /* noop */ }
  if (groupOnly) return false;
  try { process.kill(pid, 0); return true; } catch { return false; }
}

// Главный процесс dsh завершился (по коду или по сигналу).
function dshLeaderExited() {
  return dshProc.exitCode !== null || dshProc.signalCode !== null;
}

async function stopDsh() {
  if (!weStartedDsh || !dshProc || dshProc.pid === undefined) return;
  const pid = dshProc.pid;
  if (dshFatal) return; // spawn не удался — процесса не было
  const groupOnly = dshLeaderExited();
  if (!groupAlive(pid, groupOnly)) return; // группа уже мёртва — нечего убивать
  killGroup(pid, 'SIGTERM', groupOnly);
  for (let i = 0; i < 40 && groupAlive(pid, groupOnly); i++) await sleep(100); // до 4 с
  if (groupAlive(pid, groupOnly)) {
    killGroup(pid, 'SIGKILL');
    for (let i = 0; i < 20 && groupAlive(pid); i++) await sleep(100);
  }
  console.log('[launcher] dsh stopped by launcher');
}

// pid процесса, слушающего порт (Linux, чистый fs — без fuser/ss):
// inode сокетов LISTEN берём из /proc/net/tcp{,6}, затем ищем fd
// socket:[inode] среди /proc/[pid]/fd/*.
/* ===== portListenerPid ===== */
function portListenerPid(port) {
  const hexPort = port.toString(16).toUpperCase().padStart(4, '0');
  const inodes = new Set();
  for (const file of ['/proc/net/tcp', '/proc/net/tcp6']) {
    let data;
    try { data = fs.readFileSync(file, 'utf8'); } catch { continue; }
    for (const line of data.split('\n').slice(1)) {
      const c = line.trim().split(/\s+/);
      // c[1]=local(IP:PORT), c[3]=state ('0A'=LISTEN), c[9]=inode
      if (c.length < 10 || c[3] !== '0A') continue;
      if (!c[1].endsWith(':' + hexPort)) continue;
      inodes.add(c[9]);
    }
  }
  if (inodes.size === 0) return null;
  let pids;
  try { pids = fs.readdirSync('/proc').filter((n) => /^\d+$/.test(n)); } catch { return null; }
  for (const pid of pids) {
    let fds;
    try { fds = fs.readdirSync(`/proc/${pid}/fd`); } catch { continue; }
    for (const fd of fds) {
      let link;
      try { link = fs.readlinkSync(`/proc/${pid}/fd/${fd}`); } catch { continue; }
      const m = /^socket:\[(\d+)\]$/.exec(link);
      if (m && inodes.has(m[1])) return Number(pid);
    }
  }
  return null;
}
/* ===== end portListenerPid ===== */

// pgid процесса: поле 5 из /proc/<pid>/stat (comm в скобках может содержать
// пробелы — считаем от последней «)»).
function pgidOf(pid) {
  try {
    const stat = fs.readFileSync(`/proc/${pid}/stat`, 'utf8');
    const pgid = Number(stat.slice(stat.lastIndexOf(')') + 2).split(' ')[2]);
    return pgid > 0 ? pgid : null;
  } catch { return null; }
}

// Что убивать у «усыновлённого» dsh (после самовозрождения). Ручки на новый
// процесс у нас нет, поэтому:
//  - процесс на порту: убиваем его группу по pgid — он может быть не лидером
//    группы, и тогда kill(-pid) не сработал бы, а потомки остались бы жить;
//  - группа исходного dsh: в ней могли остаться его потомки (лидер мёртв —
//    только группа, без запасного варианта по pid).
function adoptedTargets() {
  const targets = [];
  const pid = portListenerPid(PORT);
  if (pid) {
    const pgid = pgidOf(pid);
    if (pgid && pgid !== pgidOf(process.pid)) targets.push({ id: pgid, groupOnly: true });
    else targets.push({ id: pid, groupOnly: false }); // группа наша/неизвестна — только сам процесс
  }
  if (adoptedOldPgid && !targets.some((t) => t.id === adoptedOldPgid)) {
    targets.push({ id: adoptedOldPgid, groupOnly: true });
  }
  return targets;
}

async function stopAdoptedDsh() {
  const targets = adoptedTargets().filter((t) => groupAlive(t.id, t.groupOnly));
  if (targets.length === 0) {
    console.log('[launcher] усыновлённый dsh: живых процессов не найдено — не убиваем (порт уже закрыт?)');
    return;
  }
  console.log(`[launcher] останавливаю усыновлённый dsh (${targets.map((t) => (t.groupOnly ? 'pgid=' : 'pid=') + t.id).join(', ')})`);
  const alive = () => targets.filter((t) => groupAlive(t.id, t.groupOnly));
  for (const t of targets) killGroup(t.id, 'SIGTERM', t.groupOnly);
  for (let i = 0; i < 20 && alive().length; i++) await sleep(100); // до 2 с
  for (const t of alive()) killGroup(t.id, 'SIGKILL', t.groupOnly);
}

/* ---------------- самовозрождение dsh (self-restart) ----------------
 * Менеджер плагинов dsh перезапускает сам процесс (после обновления плагина
 * уведомляет «изменения вступят в силу при следующем старте», и при
 * подтверждении дш поднимает новый процесс, а старый спокойно выходит с
 * code=0). Порт при этом может не закрываться вовсе (новый процесс
 * подхватывает его до выхода старого) — watchdog такой переход не видит.
 * Поэтому по exit нашего дочернего процесса проверяем порт: если он жив
 * или оживает — не показываем ошибку, а «усыновляем» запущенный dsh
 * (дальше работаем с ним как с подключением, но при закрытии окна всё
 * равно останавливаем — pid ищется по порту) и обновляем окно. Куки при
 * этом живы — секрет подписи хранится в ~/.dsh/.credentials.yaml и
 * переживает перезапуск.
 */
let adoptionCheck = false;

async function handleDshExit(code, signal) {
  console.log('[launcher] dsh process exited:', { code, signal });
  if (stopping) return;
  pushErr(tr(`dsh остановился (code=${code}, signal=${signal})`, `dsh stopped (code=${code}, signal=${signal})`));
  if (adoptionCheck) return; // на всякий случай: один цикл проверки
  adoptionCheck = true;
  try {
    showPage(() => showStatus(tr('dsh перезапускается…', 'dsh is restarting…'), tr(
      `<p>Процесс <code>dsh</code> завершился (code=${code}).<br>
         Проверяю, не запустился ли новый процесс…</p>`,
      `<p>The <code>dsh</code> process exited (code=${code}).<br>
         Checking whether a new process has started…</p>`), false));
    const deadline = Date.now() + 8000;
    let up = await portOpen();
    while (!up && Date.now() < deadline) { await sleep(500); up = await portOpen(); }
    if (up && !stopping && win && !win.isDestroyed()) {
      console.log('[launcher] порт снова открыт — dsh перезапустился сам, перехожу в режим подключения');
      weStartedDsh = false; // новый процесс — не наш дочерний (ручки на него нет)
      adoptedDsh = true;    // но это тот же dsh сессии — при закрытии окна остановим
      if (dshProc && dshProc.pid !== undefined) adoptedOldPgid = dshProc.pid;
      dshProc = null;
      dshFatal = false;
      portState = 'up';     // синхронизация со watchdog — не давать лишней перезагрузки
      await recoverWindow(true);
      return;
    }
  } finally {
    adoptionCheck = false;
  }
  if (win && !win.isDestroyed() && !stopping) {
    showPage(() => showStatus(tr('dsh остановился', 'dsh has stopped'), tr(
      `<p>Процесс DeepSeek Harness завершился, и за 8 секунд новый процесс
         не запустился.</p>`,
      `<p>The DeepSeek Harness process exited, and no new process started
         within 8 seconds.</p>`) + retryButton() + stderrBlock(), true));
  }
}

function retryButton() {
  return `<button onclick="location.href='dshlauncher://retry/'" style="margin-bottom:16px">${tr('Проверить снова', 'Check again')}</button>`;
}

/* ---------------- страницы-статусы ---------------- */

const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
}[c]));

const PAGE_CSS = `
  html,body{margin:0;height:100%;background:#0f1115;color:#d7dce3;
    font-family:system-ui,'Segoe UI',Roboto,Ubuntu,sans-serif}
  .wrap{max-width:700px;margin:0 auto;padding:12vh 24px 24px}
  h1{font-size:20px;font-weight:600;margin:0 0 14px}
  p{line-height:1.6;color:#9aa4b2;margin:0 0 12px}
  code{background:#1a2130;padding:2px 6px;border-radius:6px;font-size:0.9em;color:#c7d2fe}
  pre{background:#151a23;border:1px solid #232b3a;border-radius:10px;padding:12px 14px;
    font-size:12px;line-height:1.5;overflow:auto;max-height:34vh;white-space:pre-wrap;
    word-break:break-word;color:#c7cedb}
  input{width:100%;box-sizing:border-box;background:#151a23;border:1px solid #2a3550;
    border-radius:10px;color:#e6ebf4;padding:12px 14px;font-size:14px;margin-bottom:12px}
  input:focus{outline:none;border-color:#3b82f6}
  button{background:#3b82f6;border:0;border-radius:10px;color:#fff;padding:12px 22px;
    font-size:14px;cursor:pointer}
  button:hover{background:#2f6fd8}
  .dot{display:inline-block;width:10px;height:10px;border-radius:50%;background:#3fb950;margin-right:9px}
  .err .dot{background:#f85149}
  .lang{position:fixed;top:14px;right:18px;font-size:13px;color:#4b5567;user-select:none}
  .lang a{color:#6b7689;text-decoration:none;padding:2px 4px;border-radius:5px}
  .lang a:hover{color:#d7dce3}
  .lang a.on{color:#d7dce3;font-weight:600;pointer-events:none}
`;

function pageHtml(title, body, isError) {
  const langLink = (code, label) =>
    `<a href="dshlauncher://lang/${code}/"${UI_LANG === code ? ' class="on"' : ''}>${label}</a>`;
  const langSwitch = `<div class="lang">${langLink('ru', 'RU')} | ${langLink('en', 'EN')}</div>`;
  const html = `<!doctype html><html lang="${UI_LANG}"><head><meta charset="utf-8"><title>${esc(title)}</title><style>${PAGE_CSS}</style></head><body>${langSwitch}<div class="wrap${isError ? ' err' : ''}"><h1><span class="dot"></span>${esc(title)}</h1>${body}</div></body></html>`;
  return 'data:text/html;charset=utf-8,' + encodeURIComponent(html);
}

function stderrBlock() {
  return `<pre>${esc(stderrTail.slice(-50).join('\n') || tr('нет данных журнала', 'no log data'))}</pre>`;
}

// Текущая служебная страница как функция отрисовки: переключатель RU | EN
// перерисовывает её на новом языке. Каждая show*-страница регистрирует себя
// сама; разовые showStatus оборачиваются в showPage.
let currentPage = null;

function showPage(render) {
  currentPage = render;
  render();
}

const langPrefFile = () => path.join(app.getPath('userData'), 'ui-lang.json');

function loadLangPref() {
  if (LANG_FORCED) return;
  try {
    const v = JSON.parse(fs.readFileSync(langPrefFile(), 'utf8')).lang;
    if (v === 'ru' || v === 'en') UI_LANG = v;
  } catch { /* выбора ещё не было — остаётся язык локали */ }
}

function onLangSwitch(lang) {
  if ((lang === 'ru' || lang === 'en') && lang !== UI_LANG) {
    UI_LANG = lang;
    try {
      fs.mkdirSync(path.dirname(langPrefFile()), { recursive: true });
      fs.writeFileSync(langPrefFile(), JSON.stringify({ lang }));
    } catch (e) { console.error('[launcher] ui-lang.json:', e.message); }
    console.log(`[launcher] язык окна: ${lang}`);
  }
  if (currentPage) currentPage();
  return blankResponse();
}

// Загрузка в окно. ERR_ABORTED (-3) — не ошибка: эту навигацию сменила
// более новая (статус-страница, watchdog), окно уже показывает её.
async function loadInWin(url) {
  if (!win || win.isDestroyed()) return;
  try {
    await win.loadURL(url);
  } catch (e) {
    if (e.code === 'ERR_ABORTED' || e.errno === -3) {
      console.log(`[launcher] загрузка ${String(url).slice(0, 40)}… прервана более новой навигацией`);
    } else {
      throw e;
    }
  }
}

function showStatus(title, body, isError) {
  loadInWin(pageHtml(title, body, !!isError)).catch((e) => console.error('[launcher] showStatus:', e.message));
}

function showPastePage() {
  currentPage = showPastePage;
  const body = `
    ${tr(`<p>Похоже, <code>dsh</code> уже запущен, но у окна нет валидного сессионного куки
       (он мог истечь или dsh был перезапущен с другим секретом).</p>
    <p>В терминале, где работает dsh, найдите строку
       <code>dsh web: ${PLAIN_URL}?token=…</code> и вставьте сюда полный URL
       (или только токен):</p>`,
    `<p>It looks like <code>dsh</code> is already running, but the window has no valid
       session cookie (it may have expired, or dsh was restarted with a different secret).</p>
    <p>In the terminal where dsh is running, find the line
       <code>dsh web: ${PLAIN_URL}?token=…</code> and paste the full URL here
       (or just the token):</p>`)}
    <input id="u" type="text" placeholder="${PLAIN_URL}?token=…" autocomplete="off" spellcheck="false">
    <button onclick="go()">${tr('Открыть', 'Open')}</button>
    ${tr(`<p style="margin-top:16px">Токен взять неоткуда (dsh запущен сервисом, терминала нет)?
        Тогда остановите dsh (например, <code>pkill -f 'dsh --profile web'</code>), закройте
        это окно и запустите <code>dsh-launcher</code> снова — лаунчер поднимет dsh сам и
        подключится без токена.</p>`,
    `<p style="margin-top:16px">No way to get the token (dsh runs as a service, no terminal)?
        Then stop dsh (e.g. <code>pkill -f 'dsh --profile web'</code>), close this
        window and start <code>dsh-launcher</code> again — the launcher will start dsh itself
        and connect without a token.</p>`)}
    <script>
      const el = document.getElementById('u');
      el.focus();
      el.addEventListener('keydown', (e) => { if (e.key === 'Enter') go(); });
      function go() {
        const v = el.value.trim();
        if (!v) return;
        const url = /^https?:\\/\\//i.test(v) ? v : '${PLAIN_URL}?token=' + encodeURIComponent(v);
        location.href = 'dshlauncher://paste/' + encodeURIComponent(url);
      }
    </script>`;
  showStatus(tr('Нужен токен', 'Token required'), body, true);
}

/* ---------------- страница «dsh не установлен» ----------------
 * Порт закрыт и бинарника dsh нет — вместо попытки старта и ENOENT показываем
 * инструкцию: команда установки, «Скопировать», (если глобальный npm prefix
 * доступен без sudo) «Установить» с живым логом, и «Проверить ещё раз».
 * Состояние кэшируем: во время установки страница перерисовывается каждые
 * 800 мс, и запускать проверки в каждом рендере незачем.
 */
let installState = { check: null, npmInfo: null, running: false, log: [], timer: null, proc: null };

async function refreshInstallState() {
  [installState.check, installState.npmInfo] = await Promise.all([dshInstallCheck(), npmInstallInfo()]);
}

async function showInstallPage(note, noteIsError = false) {
  if (!win || win.isDestroyed()) return;
  currentPage = () => showInstallPage(note, noteIsError);
  if (typeof note === 'function') note = note();
  if (!installState.running) {
    try { await refreshInstallState(); } catch (e) { console.error('[launcher] refreshInstallState:', e.message); }
    if (!win || win.isDestroyed() || !installState.check) return;
  }
  const check = installState.check;
  const npmInfo = installState.npmInfo;
  const installing = installState.running;
  const noteHtml = note
    ? `<p style="color:${noteIsError ? '#f85149' : '#3fb950'}">${esc(note)}</p>`
    : '';
  const body = `
    ${noteHtml}
    <p>${installing
      ? tr(`Лаунчер не может запустить DeepSeek Harness: идёт установка <code>dsh</code>.`,
           `The launcher cannot start DeepSeek Harness: <code>dsh</code> is being installed.`)
      : check.reason === 'missing' && DSH_BIN_EXPLICIT
        ? tr(`Лаунчер не может запустить DeepSeek Harness: по пути <code>${esc(DSH_BIN)}</code> бинарника <code>dsh</code> нет.`,
             `The launcher cannot start DeepSeek Harness: there is no <code>dsh</code> binary at <code>${esc(DSH_BIN)}</code>.`)
      : check.reason === 'missing'
        ? tr(`Лаунчер не нашёл <code>dsh</code>: искал <code>${DSH_BIN_DEFAULT}</code>, в PATH, в каталогах npm и nvm.`,
             `The launcher could not find <code>dsh</code>: looked at <code>${DSH_BIN_DEFAULT}</code>, in PATH, and in npm and nvm directories.`)
        : tr(`Лаунчер не может запустить DeepSeek Harness: бинарник <code>${esc(DSH_BIN)}</code> не запускается.`,
             `The launcher cannot start DeepSeek Harness: the <code>${esc(DSH_BIN)}</code> binary fails to run.`)}</p>
    ${!installing && check.detail ? `<p>${tr('Детали', 'Details')}: <code>${esc(check.detail)}</code></p>` : ''}
    ${installing
      ? `<p>${tr('Последние строки установки:', 'Latest install output:')}</p>\n     <pre>` + esc(installState.log.slice(-40).join('\n') || '…') + '</pre>'
      : `<p>${tr('Установите dsh (нужен Node.js 22+ и npm):', 'Install dsh (requires Node.js 22+ and npm):')}</p>\n     <pre>` + esc(npmInfo.command) + '</pre>\n     ' + (
          npmInfo.nodeOld
            ? tr(`<p>Но сейчас найден Node.js <code>${esc(npmInfo.nodeVersion)}</code>, а dsh нужен Node.js ${MIN_NODE_MAJOR} или новее — на нём dsh не запустится. Сначала обновите Node.js (например, через nvm: <code>nvm install ${MIN_NODE_MAJOR}</code>, или пакеты NodeSource), затем установите dsh и нажмите «Проверить ещё раз».</p>`,
                 `<p>However, Node.js <code>${esc(npmInfo.nodeVersion)}</code> was found, and dsh needs Node.js ${MIN_NODE_MAJOR} or newer — dsh will not run on it. Upgrade Node.js first (e.g. with nvm: <code>nvm install ${MIN_NODE_MAJOR}</code>, or NodeSource packages), then install dsh and click “Check again”.</p>`)
          : npmInfo.canAuto
            ? tr('<p>Или нажмите «Установить» — лаунчер выполнит команду сам.</p>',
                 '<p>Or click “Install” — the launcher will run the command for you.</p>')
            : npmInfo.hasNpm
              ? tr('<p>Этой команде нужен <code>sudo</code> — выполните её в терминале и нажмите «Проверить ещё раз».</p>',
                   '<p>This command needs <code>sudo</code> — run it in a terminal and click “Check again”.</p>')
              : tr('<p>npm в системе нет: сначала установите Node.js (22+) с npm, затем команду выше, и нажмите «Проверить ещё раз».</p>',
                   '<p>npm is not installed: install Node.js (22+) with npm first, then the command above, and click “Check again”.</p>')
        )}
    <div style="margin-top:12px; display:flex; gap:10px; flex-wrap:wrap">
      ${installing ? '' : `<button onclick="location.href='dshlauncher://install/copy/'">${tr('Скопировать команду', 'Copy command')}</button>`}
      ${installing ? '' : (npmInfo.canAuto ? `<button onclick="location.href='dshlauncher://install/run/'">${tr('Установить', 'Install')}</button>` : '')}
      <button onclick="location.href='dshlauncher://install/recheck/'">${tr('Проверить ещё раз', 'Check again')}</button>
    </div>`;
  showStatus(installing ? tr('Устанавливаю dsh…', 'Installing dsh…') : tr('Нужен DeepSeek Harness', 'DeepSeek Harness is required'), body, !installing);
}

/* ---------------- плагин маркета (dshmarket) ----------------
 * Перед тем как лаунчер сам запускает dsh, проверяем, что в профиле стоит
 * плагин маркета. Если нет — спрашиваем пользователя: «Установить»
 * (dsh plugin --profile <профиль> add dshmarket, с живым логом), «Не сейчас»
 * или «Не спрашивать больше» (запоминается в userData/market-prompt.json).
 * Ставим именно до старта: плагины dsh подхватывает при запуске. К уже
 * запущенному dsh (режим подключения) вопрос не задаём.
 * Проверка — чтение $DSH_HOME/profiles/<профиль>/package.json (по умолчанию
 * ~/.dsh), без запуска dsh.
 */
const MARKET_PKG = 'dshmarket';

function profileFromArgs(args) {
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--profile' && args[i + 1]) return args[i + 1];
    if (args[i].startsWith('--profile=')) return args[i].slice('--profile='.length);
  }
  return null;
}
const DSH_PROFILE = profileFromArgs(DSH_ARGS);

function marketInstalled() {
  if (!DSH_PROFILE) return true; // профиль не задан — не знаем, где смотреть, не спрашиваем
  try {
    const dshHome = process.env.DSH_HOME || path.join(os.homedir(), '.dsh');
    const pkgFile = path.join(dshHome, 'profiles', DSH_PROFILE, 'package.json');
    const pkg = JSON.parse(fs.readFileSync(pkgFile, 'utf8'));
    const bundles = (pkg.dsh && pkg.dsh.profile && pkg.dsh.profile.bundles) || [];
    return Boolean(pkg.dependencies && pkg.dependencies[MARKET_PKG]) || bundles.includes(MARKET_PKG);
  } catch {
    return false; // профиля ещё нет — dsh plugin add создаст его сам
  }
}

const marketPrefFile = () => path.join(app.getPath('userData'), 'market-prompt.json');

function marketPromptDisabled() {
  try { return JSON.parse(fs.readFileSync(marketPrefFile(), 'utf8')).dontAsk === true; }
  catch { return false; }
}

let marketState = { resolve: null, running: false, log: [], timer: null, proc: null, cmdLine: null };

function marketCommand() {
  return `${DSH_BIN} plugin --profile ${DSH_PROFILE} add ${MARKET_PKG}`;
}

function showMarketPage(note, noteIsError = false) {
  currentPage = () => showMarketPage(note, noteIsError);
  if (typeof note === 'function') note = note();
  const installing = marketState.running;
  const noteHtml = note
    ? `<p style="color:${noteIsError ? '#f85149' : '#3fb950'}">${esc(note)}</p>`
    : '';
  const body = installing
    ? `<p>${tr('Выполняю', 'Running')} <code>${esc(marketState.cmdLine || marketCommand())}</code></p>
       <p>${tr('Последние строки установки:', 'Latest install output:')}</p>
       <pre>${esc(marketState.log.slice(-40).join('\n') || '…')}</pre>`
    : `${noteHtml}
       <p>${tr(`В профиле <code>${esc(DSH_PROFILE)}</code> не найден плагин маркета
          <code>${MARKET_PKG}</code>. Установить его перед запуском dsh?`,
          `The marketplace plugin <code>${MARKET_PKG}</code> is not installed in the
          <code>${esc(DSH_PROFILE)}</code> profile. Install it before starting dsh?`)}</p>
       <pre>${esc(marketCommand())}</pre>
       ${noteIsError && marketState.log.length ? `<pre>${esc(marketState.log.slice(-20).join('\n'))}</pre>` : ''}
       <div style="margin-top:12px; display:flex; gap:10px; flex-wrap:wrap">
         <button onclick="location.href='dshlauncher://market/install/'">${tr('Установить', 'Install')}</button>
         <button onclick="location.href='dshlauncher://market/skip/'">${tr('Не сейчас', 'Not now')}</button>
         <button onclick="location.href='dshlauncher://market/never/'">${tr('Не спрашивать больше', 'Don’t ask again')}</button>
       </div>`;
  showStatus(installing ? tr(`Устанавливаю ${MARKET_PKG}…`, `Installing ${MARKET_PKG}…`)
    : tr(`Установить ${MARKET_PKG}?`, `Install ${MARKET_PKG}?`), body, noteIsError);
}

// Показывает вопрос и ждёт нажатия: 'install' | 'skip' | 'never'.
function askMarket(note, noteIsError) {
  return new Promise((resolve) => {
    marketState.resolve = resolve;
    showMarketPage(note, noteIsError);
  });
}

function onMarketChoice(choice) {
  const resolve = marketState.resolve;
  if (resolve && !marketState.running) {
    marketState.resolve = null;
    resolve(choice);
  }
  return blankResponse();
}

// dsh plugin add вызывает pnpm — его каталог тоже должен быть в PATH.
function marketEnv() {
  const env = dshEnv();
  const pnpm = findBin('pnpm');
  if (pnpm) env.PATH = path.dirname(pnpm) + path.delimiter + env.PATH;
  return env;
}

// Запуск шага установки (pnpm или сам маркет) с живым логом на странице.
async function runMarketInstall(cmd, args, env) {
  marketState.running = true;
  marketState.log = [];
  marketState.cmdLine = [cmd, ...args].join(' ');
  console.log(`[launcher] установка: ${marketState.cmdLine}`);
  try {
    const p = spawn(cmd, args, { cwd: CWD, env, stdio: ['ignore', 'pipe', 'pipe'] });
    marketState.proc = p;
    const onData = (d) => {
      for (const line of d.toString().split('\n')) {
        const t = line.trim();
        if (!t) continue;
        marketState.log.push(t);
        if (marketState.log.length > 500) marketState.log.shift();
      }
    };
    p.stdout.on('data', onData);
    p.stderr.on('data', onData);
    showMarketPage();
    marketState.timer = setInterval(() => showMarketPage(), 800);
    return await new Promise((resolve) => {
      let settled = false;
      const done = (c) => { if (!settled) { settled = true; resolve(c); } };
      p.on('close', (c) => done(c === null ? -1 : c));
      p.on('error', (e) => { marketState.log.push(e.message); done(-1); });
    });
  } finally {
    clearInterval(marketState.timer);
    marketState.running = false;
    marketState.proc = null;
  }
}

async function offerMarketIfMissing() {
  if (marketInstalled()) return;
  if (marketPromptDisabled()) {
    console.log(`[launcher] ${MARKET_PKG} не установлен, вопрос отключён (${marketPrefFile()})`);
    return;
  }
  console.log(`[launcher] ${MARKET_PKG} не установлен в профиле ${DSH_PROFILE} — спрашиваю пользователя`);
  let note = null;
  let noteIsError = false;
  for (;;) {
    const choice = await askMarket(note, noteIsError);
    if (stopping) return;
    if (choice === 'never') {
      try {
        fs.mkdirSync(path.dirname(marketPrefFile()), { recursive: true });
        fs.writeFileSync(marketPrefFile(), JSON.stringify({ dontAsk: true }));
      } catch (e) { console.error('[launcher] market-prompt.json:', e.message); }
      console.log('[launcher] пользователь отказался от маркета навсегда');
      return;
    }
    if (choice !== 'install') {
      console.log('[launcher] установка маркета отложена (не сейчас)');
      return;
    }
    // dsh plugin add требует pnpm: на чистой системе его нет (код 127).
    if (!findBin('pnpm')) {
      const npmInfo = await npmInstallInfo();
      if (stopping) return;
      if (!npmInfo.canAuto) {
        const cmd = npmInfo.hasNpm ? 'sudo npm install -g pnpm' : 'npm install -g pnpm';
        console.log(`[launcher] pnpm нет, поставить сами не можем — прошу «${cmd}»`);
        marketState.log = [];
        note = () => tr(`Для плагинов dsh нужен pnpm, а его в системе нет. Выполните в терминале «${cmd}» и нажмите «Установить» ещё раз.`,
          `dsh plugins need pnpm, which is not installed. Run “${cmd}” in a terminal and click “Install” again.`);
        noteIsError = true;
        continue;
      }
      console.log('[launcher] pnpm нет — ставлю через npm');
      const pc = await runMarketInstall(npmInfo.npm, ['install', '-g', 'pnpm'], envWithBinDir(npmInfo.npm));
      if (stopping) return;
      if (pc !== 0 || !findBin('pnpm')) {
        console.error(`[launcher] установка pnpm не удалась (код ${pc})`);
        note = () => tr(`Не удалось установить pnpm (код ${pc}). Можно повторить или продолжить без маркета.`,
          `Failed to install pnpm (code ${pc}). You can retry or continue without the marketplace.`);
        noteIsError = true;
        continue;
      }
    }
    const code = await runMarketInstall(DSH_BIN, ['plugin', '--profile', DSH_PROFILE, 'add', MARKET_PKG], marketEnv());
    if (stopping) return;
    if (code === 0 && marketInstalled()) {
      console.log(`[launcher] ${MARKET_PKG} установлен`);
      return;
    }
    console.error(`[launcher] установка ${MARKET_PKG} не удалась (код ${code})`);
    note = () => tr(`Установка не удалась (код ${code}). Можно повторить или продолжить без маркета.`,
      `Installation failed (code ${code}). You can retry or continue without the marketplace.`);
    noteIsError = true;
  }
}

/* ---------------- запуск dsh и доведение окна до GUI ---------------- */

// Защита от повторного запуска: двойной клик «Проверить ещё раз» или
// «Установить» вызвал бы launchDsh дважды — второй spawn перезаписал бы
// dshProc, и первый процесс остался бы без присмотра (не убит при закрытии).
let launching = false;

function ourDshAlive() {
  return weStartedDsh && dshProc && dshProc.exitCode === null && !dshFatal;
}

async function launchDsh() {
  if (launching || ourDshAlive()) {
    console.log('[launcher] запуск dsh уже идёт — повторный вызов пропущен');
    return;
  }
  launching = true;
  try {
    await offerMarketIfMissing();
    if (stopping) return;
    await launchDshInner();
  } finally {
    launching = false;
  }
}

async function launchDshInner() {
  showPage(() => showStatus(tr('Запуск DeepSeek Harness…', 'Starting DeepSeek Harness…'), tr(
    `<p>Запускаю <code>${esc(DSH_BIN)} ${esc(DSH_ARGS.join(' '))}</code>.<br>Жду, пока сервис поднимется…</p>`,
    `<p>Running <code>${esc(DSH_BIN)} ${esc(DSH_ARGS.join(' '))}</code>.<br>Waiting for the service to come up…</p>`)));
  startDsh();

  const up = await waitForPort();
  if (!up) {
    showPage(() => showStatus(tr('Не удалось запустить dsh', 'Failed to start dsh'), tr(
      `<p>Процесс не поднял сервис на <code>${PLAIN_URL}</code>.<br>
         Проверьте, что команда запускается в терминале:
         <code>${esc(DSH_BIN)} ${esc(DSH_ARGS.join(' '))}</code>.<br>
         Последние строки журнала:</p>`,
      `<p>The process did not bring up the service at <code>${PLAIN_URL}</code>.<br>
         Check that the command runs in a terminal:
         <code>${esc(DSH_BIN)} ${esc(DSH_ARGS.join(' '))}</code>.<br>
         Latest log lines:</p>`) + stderrBlock(), true));
    startWatchdog(); // если dsh запустят вручную — окно оживёт само
    return;
  }

  // Порт уже поднят нами: проверка watchdog, начатая во время запуска и
  // завершившаяся после, не должна принять это за перезапуск dsh.
  portState = 'up';
  const url = authUrl || (await waitForAuthUrl());
  if (url) {
    await loadInWin(url); // обмен токена на куки, затем редирект на чистый URL
  } else if (await plainUrlIsAuthed()) {
    await loadInWin(PLAIN_URL); // старое куки ещё валидно
  } else {
    showPastePage();
  }
  startWatchdog();
}

/* ---------------- основной поток ---------------- */

protocol.registerSchemesAsPrivileged([
  { scheme: 'dshlauncher', privileges: { standard: false, secure: true } },
]);

async function plainUrlIsAuthed() {
  try {
    const res = await electronNet.fetch(PLAIN_URL, { redirect: 'manual' });
    return res.status === 200;
  } catch {
    return false;
  }
}

/* ---------------- авто-восстановление после перезапуска dsh ----------------
 * GUI сам не возобновляет текущую сессию, когда процесс dsh перезапускается:
 * окно застревает на «сессия завершена». Но всё нужное для возобновления
 * сохраняется на диске: сессионный куки (секрет подписи живёт в
 * ~/.dsh/.credentials.yaml), сами сессии (~/.dsh/sessions) и состояние
 * интерфейса (localStorage). Поэтому при переходе порта down→up достаточно
 * перезагрузить страницу — приложение восстановится само.
 */
let watchdogStarted = false;
let portState = null; // null | 'up' | 'down'
let lastAutoReloadAt = 0;

async function recoverWindow(force = false) {
  if (!win || win.isDestroyed() || stopping) return;
  const now = Date.now();
  if (!force && now - lastAutoReloadAt < 3000) return; // защита от флаттера порта
  lastAutoReloadAt = now;
  try {
    let cur = 'about:blank';
    try { cur = win.webContents.getURL(); } catch { return; }
    const dshOrigin = new URL(PLAIN_URL).origin;
    let onDshPage = false;
    try { onDshPage = new URL(cur).origin === dshOrigin; } catch { /* data: и т.п. */ }
    if (onDshPage) {
      await loadInWin(PLAIN_URL);
    } else if (await plainUrlIsAuthed()) {
      await loadInWin(PLAIN_URL); // окно на статус-странице, но куки валидны
    } else {
      showPastePage(); // куки не пережили перезапуск — нужен токен нового процесса
    }
  } catch (e) {
    console.error('[launcher] recover:', e.message);
  }
}

function startWatchdog() {
  if (watchdogStarted) return;
  watchdogStarted = true;
  (async () => {
    portState = (await portOpen()) ? 'up' : 'down'; // базовое состояние, без срабатываний
    setInterval(async () => {
      if (stopping) return;
      const state = (await portOpen()) ? 'up' : 'down';
      if (state === portState) return;
      if (state === 'up' && launching) {
        portState = state; // окно доведёт до GUI сам launchDsh — не мешаем ему второй навигацией
        return;
      }
      if (state === 'up') {
        console.log(`[launcher] dsh снова отвечает на порту ${PORT} — обновляю окно`);
        portState = state;
        recoverWindow();
      } else {
        console.log(`[launcher] порт ${PORT} закрыт — если dsh перезапустят, окно восстановится само`);
        portState = state;
      }
    }, 1500);
  })();
}

// Маршрут запроса dshlauncher://…: у нестандартной схемы первый сегмент
// (retry, install, paste, market) URL-парсер кладёт в host, а не в pathname —
// склеиваем обратно: dshlauncher://install/run/ -> '/install/run/'.
function launcherRoute(url) {
  const u = new URL(url);
  return '/' + u.host + u.pathname;
}

function onPasteRequest(request) {
  try {
    const pasted = decodeURIComponent(launcherRoute(request.url).replace(/^\/paste\//, ''));
    if (/^https?:\/\//i.test(pasted) && win && !win.isDestroyed()) {
      console.log('[launcher] loading pasted URL');
      win.loadURL(pasted).catch((e) => console.error('[launcher] loadURL:', e.message));
    }
  } catch (e) {
    console.error('[launcher] paste handler:', e.message);
  }
  return new Response('<!doctype html><html><body style="background:#0f1115;margin:0"></body></html>',
    { headers: { 'content-type': 'text/html' } });
}

// «Проверить снова» на фатальной странице: если dsh запустили вручную
// (например, в терминале), а окно оставили открытым — возвращаемся в GUI.
async function onRetryRequest() {
  try {
    if (win && !win.isDestroyed() && !stopping) {
      const up = await portOpen();
      if (up) {
        weStartedDsh = false; // чужой dsh — при закрытии окна его не убиваем
        dshProc = null;
        dshFatal = false;
        portState = 'up';
        if (await plainUrlIsAuthed()) {
          console.log('[launcher] retry: dsh запущен, куки валидны — открываю GUI');
          await loadInWin(PLAIN_URL);
        } else {
          console.log('[launcher] retry: dsh запущен, валидного куки нет — страница вставки');
          showPastePage();
        }
      } else {
        // Бинарник мог исчезнуть (разустановка, другой DSH_BIN) — тогда
        // совет запускать его в терминале бессмысленен.
        if (!(await dshInstallCheck()).ok) {
          showInstallPage(() => tr('dsh не найден — установите его (команда на странице).',
            'dsh not found — install it (the command is on this page).'));
        } else {
          showPage(() => showStatus(tr('dsh не запущен', 'dsh is not running'), tr(
            `<p>Порт ${PORT} закрыт — <code>dsh</code> не запущен.</p>
               <p>Запустите его в терминале: <code>${esc(DSH_BIN)} ${esc(DSH_ARGS.join(' '))}</code>
               и нажмите «Проверить снова». Либо закройте окно и запустите
               dsh-launcher снова.</p>`,
            `<p>Port ${PORT} is closed — <code>dsh</code> is not running.</p>
               <p>Start it in a terminal: <code>${esc(DSH_BIN)} ${esc(DSH_ARGS.join(' '))}</code>
               and click “Check again”. Or close this window and start
               dsh-launcher again.</p>`) + retryButton(), false));
        }
      }
    }
  } catch (e) {
    console.error('[launcher] onRetryRequest:', e.message);
  }
  return new Response('<!doctype html><html><body style="background:#0f1115;margin:0"></body></html>',
    { headers: { 'content-type': 'text/html' } });
}

function blankResponse() {
  return new Response('<!doctype html><html><body style="background:#0f1115;margin:0"></body></html>',
    { headers: { 'content-type': 'text/html' } });
}

// «Скопировать команду» на странице установки dsh.
async function onInstallCopy() {
  try {
    if (!installState.npmInfo) await refreshInstallState();
    clipboard.writeText(installState.npmInfo.command);
    console.log('[launcher] команда установки скопирована в буфер обмена');
    await showInstallPage(() => tr('Команда скопирована в буфер обмена — вставьте её в терминал.', 'Command copied to the clipboard — paste it into a terminal.'));
  } catch (e) {
    console.error('[launcher] onInstallCopy:', e.message);
    showInstallPage(() => tr('Не удалось скопировать команду: ' + e.message, 'Failed to copy the command: ' + e.message), true);
  }
  return blankResponse();
}

// «Проверить ещё раз»: dsh появился (поставили вручную или кнопкой) — продолжаем запуск.
async function onInstallRecheck() {
  try {
    if (installState.running) return blankResponse(); // установка идёт — страница обновится сама
    await refreshInstallState();
    if (installState.check.ok) {
      console.log(`[launcher] dsh найден (${installState.check.version}) — начинаю запуск`);
      await launchDsh();
      return blankResponse();
    }
    console.log(installState.check.reason === 'broken'
      ? `[launcher] повторная проверка: dsh найден, но не запускается (${installState.check.detail || ''})`
      : '[launcher] повторная проверка: dsh всё ещё не найден');
    showInstallPage(() => installState.check && installState.check.reason === 'broken'
      ? tr('dsh найден, но не запускается — причина ниже.', 'dsh was found but does not run — see the reason below.')
      : tr('dsh всё ещё не найден — установите его командой выше, затем проверьте снова.', 'dsh is still not found — install it with the command above, then check again.'), true);
  } catch (e) {
    console.error('[launcher] onInstallRecheck:', e.message);
    showInstallPage(() => tr('Ошибка проверки: ' + e.message, 'Check failed: ' + e.message), true);
  }
  return blankResponse();
}

// «Установить»: npm install -g прямо из окна, с живым логом.
async function onInstallRun() {
  try {
    if (installState.running || !win || win.isDestroyed() || stopping) return blankResponse();
    await refreshInstallState();
    if (installState.running) return blankResponse(); // пока проверяли, установку уже запустили
    const npmInfo = installState.npmInfo;
    if (!npmInfo.canAuto) {
      showInstallPage(() => tr('Кнопка «Установить» недоступна: глобальный npm prefix требует sudo — выполните команду в терминале.', '“Install” is unavailable: the global npm prefix requires sudo — run the command in a terminal.'), true);
      return blankResponse();
    }
    installState.running = true;
    installState.log = [];
    console.log(`[launcher] установка dsh из окна: ${npmInfo.command}`);
    installState.proc = spawn(npmInfo.npm, ['install', '-g', '@deepseek-ai/dsh'],
      { env: envWithBinDir(npmInfo.npm), stdio: ['ignore', 'pipe', 'pipe'] });
    const p = installState.proc;
    const onData = (d) => {
      for (const line of d.toString().split('\n')) {
        const t = line.trim();
        if (!t) continue;
        installState.log.push(t);
        if (installState.log.length > 500) installState.log.shift();
      }
    };
    p.stdout.on('data', onData);
    p.stderr.on('data', onData);
    installState.timer = setInterval(() => showInstallPage(), 800);
    const code = await new Promise((resolve) => {
      let settled = false;
      const done = (c) => { if (!settled) { settled = true; resolve(c); } };
      p.on('close', (c) => done(c === null ? -1 : c));
      p.on('error', () => done(-1)); // npm не запустился — close может не прийти
    });
    clearInterval(installState.timer);
    installState.running = false;
    installState.proc = null;
    if (code === 0) {
      console.log('[launcher] dsh установлен — продолжаю запуск');
      // Если prefix не /usr, бинарник лёг не в /usr/bin — находим и запоминаем фактический путь.
      const check = await dshInstallCheck();
      if (!check.ok) {
        showInstallPage(() => tr('npm сообщил об успехе, но dsh так и не запускается — см. детали ниже.',
          'npm reported success, but dsh still does not run — see details below.'), true);
        return blankResponse();
      }
      await launchDsh();
    } else {
      console.error(`[launcher] установка dsh завершилась с кодом ${code}`);
      showInstallPage(() => tr(`Установка завершилась с кодом ${code} (строки выше) — повторите в терминале.`, `Installation exited with code ${code} (see output above) — retry in a terminal.`), true);
    }
  } catch (e) {
    console.error('[launcher] onInstallRun:', e.message);
    installState.running = false;
    try { clearInterval(installState.timer); } catch { /* noop */ }
    showInstallPage(() => tr('Не удалось запустить установку: ' + e.message, 'Failed to start the installation: ' + e.message), true);
  }
  return blankResponse();
}

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on('second-instance', () => {
    if (win) {
      if (win.isMinimized()) win.restore();
      win.focus();
    }
  });
  app.whenReady().then(onReady);
}

async function onReady() {
  installSignalHandlers();
  Menu.setApplicationMenu(null);
  loadLangPref();
  protocol.handle('dshlauncher', (request) => {
    try {
      const route = launcherRoute(request.url);
      if (route.startsWith('/retry/')) return onRetryRequest();
      if (route.startsWith('/lang/')) return onLangSwitch(route.split('/')[2]);
      if (route.startsWith('/install/copy/')) return onInstallCopy();
      if (route.startsWith('/install/run/')) return onInstallRun();
      if (route.startsWith('/install/recheck/')) return onInstallRecheck();
      if (route.startsWith('/market/install/')) return onMarketChoice('install');
      if (route.startsWith('/market/skip/')) return onMarketChoice('skip');
      if (route.startsWith('/market/never/')) return onMarketChoice('never');
    } catch { /* noop */ }
    return onPasteRequest(request);
  });

  win = new BrowserWindow({
    width: 1440,
    height: 900,
    minWidth: 760,
    minHeight: 480,
    title: 'DeepSeek Harness',
    backgroundColor: '#0f1115',
    autoHideMenuBar: true,
    show: false,
    webPreferences: { contextIsolation: true, nodeIntegration: false, spellcheck: false },
  });
  win.webContents.setWindowOpenHandler(({ url }) => {
    if (/^https?:/i.test(url)) shell.openExternal(url);
    return { action: 'deny' };
  });
  // диагностика: из лога должно быть видно, что окно показало и что загрузилось
  win.once('ready-to-show', () => { win.show(); console.log('[launcher] окно показано'); });
  // Страховка: на нативном Wayland ready-to-show может прийти с большим
  // опозданием (или не прийти вовсе) — окно с show:false остаётся невидимым.
  // Если через 8 с оно всё ещё не показано — показываем вручную.
  setTimeout(() => {
    // isDestroyed: окно могли закрыть (и app.quit()) раньше таймера —
    // методы на уничтоженном окне бросают исключение.
    if (win && !win.isDestroyed() && !win.isVisible()) {
      console.log('[launcher] ready-to-show не пришло за 8 с — показываю окно вручную');
      win.show();
    }
  }, 8000);
  win.webContents.on('did-finish-load', () => {
    try { console.log('[launcher] страница загружена:', win.webContents.getURL().slice(0, 120)); } catch { /* noop */ }
  });
  win.webContents.on('did-fail-load', (_e, code, desc, url, isMainFrame) => {
    if (isMainFrame) console.error(`[launcher] ошибка загрузки: code=${code} ${desc} url=${String(url).slice(0, 120)}`);
  });
  win.webContents.on('render-process-gone', (_e, details) => {
    console.error('[launcher] рендер-процесс завершился:', details.reason, 'exitCode=' + details.exitCode);
  });

  // Управление, как в браузере: меню отключено (Menu.setApplicationMenu(null)),
  // поэтому стандартные ускорители (Reload и т.п.) не работают — привязываем свои.
  // F5 / Ctrl+R — перезагрузка страницы; Ctrl+Shift+R — без кэша.
  // Ctrl+= / Ctrl+- — шаг масштаба; Ctrl+0 — сброс.
  // Масштаб сохраняем в userData, чтобы не сбрасывался между запусками.
  // (Ctrl+колесо не поддержано: в свежих Electron wheel-события в main-процесс
  //  приходят ненадёжно, а перехват колес в самой странице конфликтует с
  //  обработчиками загруженной страницы.)
  const wc = win.webContents;
  const zoomFile = path.join(app.getPath('userData'), 'zoom-level.json');
  let zoomSaveTimer = null;
  const saveZoom = (level) => {
    clearTimeout(zoomSaveTimer);
    zoomSaveTimer = setTimeout(() => {
      try { fs.writeFileSync(zoomFile, JSON.stringify({ zoomLevel: level })); }
      catch { /* noop */ }
    }, 400);
  };
  const applyZoom = (level, note) => {
    const z = Math.max(-5, Math.min(5, level));
    wc.setZoomLevel(z);
    saveZoom(z);
    if (note) console.log(`[launcher] ${note}`);
  };
  try {
    const savedZoom = JSON.parse(fs.readFileSync(zoomFile, 'utf8'));
    if (Number.isFinite(savedZoom.zoomLevel)) {
      wc.setZoomLevel(Math.max(-5, Math.min(5, savedZoom.zoomLevel)));
      console.log(`[launcher] восстановлен масштаб: ${savedZoom.zoomLevel}`);
    }
  } catch { /* noop */ }
  win.webContents.on('before-input-event', (event, input) => {
    try {
      if (input.type !== 'keyDown') return;
      // code — физическая клавиша (не зависит от раскладки: RU/EN/другие)
      if (input.key === 'F5' || (input.control && input.code === 'KeyR')) {
        event.preventDefault();
        if (input.shift) wc.reloadIgnoringCache(); else wc.reload();
        console.log(`[launcher] перезагрузка страницы (${input.key === 'F5' ? 'F5' : 'Ctrl+R'})`);
      } else if (input.control && (input.code === 'Equal' || input.code === 'NumpadAdd')) {
        event.preventDefault();
        applyZoom(wc.getZoomLevel() + 0.5);
      } else if (input.control && (input.code === 'Minus' || input.code === 'NumpadSubtract')) {
        event.preventDefault();
        applyZoom(wc.getZoomLevel() - 0.5);
      } else if (input.control && (input.code === 'Digit0' || input.code === 'Numpad0')) {
        event.preventDefault();
        applyZoom(0, 'масштаб сброшен (Ctrl+0)');
      }
    } catch (e) { console.error('[launcher] ошибка в обработчике клавиш:', e); }
  });

  const alreadyRunning = await portOpen();
  if (alreadyRunning) {
    console.log('[launcher] port already open — attaching to running dsh');
    showPage(() => showStatus(tr('Подключение…', 'Connecting…'), tr(
      `<p><code>dsh</code> уже запущен на порту ${PORT}. Подключаюсь…</p>`,
      `<p><code>dsh</code> is already running on port ${PORT}. Connecting…</p>`)));
    if (await plainUrlIsAuthed()) {
      console.log('[launcher] валидный куки есть — открываю GUI');
      await loadInWin(PLAIN_URL);
    } else {
      console.log('[launcher] валидного куки нет — показываю страницу вставки токена');
      showPastePage();
    }
    startWatchdog();
    return;
  }

  // Порт закрыт — будем запускать dsh сами. Сначала проверяем, что он вообще
  // установлен: иначе вместо понятной страницы — ENOENT после попытки старта.
  const installCheck = await dshInstallCheck();
  if (!installCheck.ok) {
    console.log(
      `[launcher] dsh не найден (${installCheck.reason}` +
      (installCheck.detail ? ': ' + installCheck.detail : '') +
      ') — показываю страницу установки'
    );
    showInstallPage();
    startWatchdog(); // если dsh поставят и запустят вручную — окно оживёт само
    return;
  }
  await launchDsh();
}

/* ---------------- закрытие = остановка ---------------- */

app.on('window-all-closed', () => app.quit());

app.on('before-quit', (e) => {
  if (stopping) return;
  stopping = true;
  e.preventDefault();
  if (installState.running && installState.proc) {
    try { installState.proc.kill('SIGTERM'); } catch { /* noop */ }
  }
  if (marketState.proc) {
    try { marketState.proc.kill('SIGTERM'); } catch { /* noop */ }
  }
  Promise.resolve().then(async () => {
    await stopDsh();
    if (adoptedDsh) await stopAdoptedDsh();
  }).finally(() => app.quit());
});

// страховка: если before-quit почему-то не сработала — жёстко
app.on('will-quit', () => killDshNow());

// страховка на «жёсткое» завершение самого лаунчера: SIGTERM/SIGINT/SIGHUP (pkill,
// завершение сессии) и exit процесса. Поймать SIGKILL, отправленный самому
// лаунчеру, нельзя — в том случае dsh переживёт его (это ограничение
// отделимого процесса, лечится только системно — cgroup/supervisor).
function killDshNow() {
  if (weStartedDsh && dshProc && dshProc.pid !== undefined && !dshFatal) {
    killGroup(dshProc.pid, 'SIGKILL', dshLeaderExited());
  }
  if (adoptedDsh) {
    for (const t of adoptedTargets()) killGroup(t.id, 'SIGKILL', t.groupOnly);
  }
}
// По сигналу — штатное закрытие через before-quit (dsh получает SIGTERM и
// успевает сохраниться, через 4 с — SIGKILL). Жёстко (SIGKILL дереву и
// выход) — по повторному сигналу, если приложение ещё не готово или если
// штатное закрытие не уложилось в 8 с.
let termSignalSeen = false;
function onTermSignal(name, exitCode) {
  console.log(`[launcher] ${name} — останавливаю dsh`);
  if (termSignalSeen || !app.isReady()) {
    killDshNow();
    process.exit(exitCode);
  }
  termSignalSeen = true;
  setTimeout(() => {
    console.error('[launcher] штатное закрытие зависло — останавливаю dsh жёстко');
    killDshNow();
    process.exit(exitCode);
  }, 8000);
  app.quit();
}
// Ставить — после ready: Chromium при старте ставит свой обработчик
// SIGTERM/SIGINT/SIGHUP поверх раннего process.on (тот так и не вызывается),
// а после первого сигнала возвращает обработчик по умолчанию — повторный
// сигнал убивал лаунчер мгновенно, и медленно завершающийся dsh оставался
// сиротой. Поставленный позже, наш обработчик заменяет chromium-овский.
function installSignalHandlers() {
  process.on('SIGINT', () => onTermSignal('SIGINT', 130));
  process.on('SIGTERM', () => onTermSignal('SIGTERM', 143));
  process.on('SIGHUP', () => onTermSignal('SIGHUP', 129));
}
process.on('exit', killDshNow);
