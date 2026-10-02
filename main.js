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
 *  - закрытие окна завершает всё дерево процессов dsh (если его запустил лаунчер):
 *    сначала SIGTERM, через 4 с — SIGKILL.
 */

const { app, BrowserWindow, Menu, protocol, net: electronNet, shell } = require('electron');
const { spawn, execFileSync } = require('child_process');
const nodeNet = require('net');
const fs = require('fs');
const os = require('os');
const path = require('path');

/* ========================= настройки ========================= */

const DSH_BIN = process.env.DSH_BIN || '/usr/bin/dsh'; // путь к бинарнику dsh
const DSH_ARGS = process.env.DSH_ARGS
  ? process.env.DSH_ARGS.split(' ').filter(Boolean)
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

/* ============================================================== */

app.setName('dsh-launcher');

// Диагностика сессии в лог — первое, что смотреть, если «окно не появляется».
if (process.platform === 'linux') {
  console.log(
    `[launcher] сессия: XDG_SESSION_TYPE=${process.env.XDG_SESSION_TYPE || '(пусто)'} ` +
    `WAYLAND_DISPLAY=${process.env.WAYLAND_DISPLAY || '(пусто)'} DISPLAY=${process.env.DISPLAY || '(пусто)'}`
  );
}

// Окно на Wayland предъявляется через EGL. Если у GPU виртуалки нет рабочего
// драйвера (ANGLE/EGL-ошибки в логе), окно «показано» по логам Electron, но на
// экране его не видно. X11/XWayland-путь деградирует до программного рендера —
// окно остаётся видимым. На X11-сессиях ничего не меняем.
const isWaylandSession = process.platform === 'linux' &&
  (String(process.env.XDG_SESSION_TYPE || '').toLowerCase() === 'wayland' ||
   Boolean(process.env.WAYLAND_DISPLAY));
if (isWaylandSession) {
  app.commandLine.appendSwitch('ozone-platform', 'x11');
  console.log('[launcher] Wayland-сессия — окно веду через X11 (XWayland): при сломанном GPU на Wayland окно невидимо');
}

// ВМ/VNC без 3D-ускорения: render-узлов нет, EGL гарантированно не поднимется —
// идём сразу в программный рендер, минуя цикл падений GPU-процесса (ANGLE/EGL
// ошибки в логе). То же, если библиотек EGL в системе нет вовсе. Ручной
// переключатель: DSH_LAUNCHER_NO_GPU=1.
const hasGpuRenderNode = (() => {
  try { return fs.readdirSync('/dev/dri').some((f) => /^renderD/.test(f)); }
  catch { return false; }
})();
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
  console.log(`[launcher] starting: ${DSH_BIN} ${DSH_ARGS.join(' ')} (cwd=${CWD}, log=${LOG_FILE})`);
  dshProc = spawn(DSH_BIN, DSH_ARGS, {
    cwd: CWD,
    env: { ...process.env, DSH_STARTED_BY: 'dsh-launcher' },
    stdio: ['ignore', 'pipe', 'pipe'],
    detached: true, // dsh становится лидером своей процесс-группы — можно убить всё дерево
  });
  weStartedDsh = true;
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
    pushErr(`ошибка запуска dsh: ${err.message}`);
  });
  dshProc.on('exit', (code, signal) => {
    try { logStream.end(); } catch { /* noop */ }
    handleDshExit(code, signal);
  });
}

/* ---------------- остановка дерева процессов ---------------- */

function killGroup(pid, signal) {
  try { process.kill(-pid, signal); } catch {
    try { process.kill(pid, signal); } catch { /* noop */ }
  }
}

function groupAlive(pid) {
  try { process.kill(-pid, 0); return true; } catch { /* noop */ }
  try { process.kill(pid, 0); return true; } catch { return false; }
}

async function stopDsh() {
  if (!weStartedDsh || !dshProc || dshProc.pid === undefined) return;
  const pid = dshProc.pid;
  if (dshProc.exitCode !== null || dshFatal) return;
  killGroup(pid, 'SIGTERM');
  for (let i = 0; i < 40 && groupAlive(pid); i++) await sleep(100); // до 4 с
  if (groupAlive(pid)) {
    killGroup(pid, 'SIGKILL');
    for (let i = 0; i < 20 && groupAlive(pid); i++) await sleep(100);
  }
  console.log('[launcher] dsh stopped by launcher');
}

/* ---------------- самовозрождение dsh (self-restart) ----------------
 * Менеджер плагинов dsh перезапускает сам процесс (после обновления плагина
 * уведомляет «изменения вступят в силу при следующем старте», и при
 * подтверждении дш поднимает новый процесс, а старый спокойно выходит с
 * code=0). Порт при этом может не закрываться вовсе (новый процесс
 * подхватывает его до выхода старого) — watchdog такой переход не видит.
 * Поэтому по exit нашего дочернего процесса проверяем порт: если он жив
 * или оживает — не показываем ошибку, а «усыновляем» запущенный dsh
 * (режим подключения: при закрытии окна его уже не убиваем) и обновляем
 * окно. Куки при этом живы — секрет подписи хранится в
 * ~/.dsh/.credentials.yaml и переживает перезапуск.
 */
let adoptionCheck = false;

async function handleDshExit(code, signal) {
  console.log('[launcher] dsh process exited:', { code, signal });
  if (stopping) return;
  pushErr(`dsh остановился (code=${code}, signal=${signal})`);
  if (adoptionCheck) return; // на всякий случай: один цикл проверки
  adoptionCheck = true;
  try {
    showStatus('dsh перезапускается…',
      `<p>Процесс <code>dsh</code> завершился (code=${code}).<br>
         Проверяю, не запустился ли новый процесс…</p>`, false);
    const deadline = Date.now() + 8000;
    let up = await portOpen();
    while (!up && Date.now() < deadline) { await sleep(500); up = await portOpen(); }
    if (up && !stopping && win && !win.isDestroyed()) {
      console.log('[launcher] порт снова открыт — dsh перезапустился сам, перехожу в режим подключения');
      weStartedDsh = false; // новый процесс — не наш: при закрытии окна его не убиваем
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
    showStatus('dsh остановился',
      `<p>Процесс DeepSeek Harness завершился, и за 8 секунд новый процесс
         не запустился.</p>${retryButton()}${stderrBlock()}`, true);
  }
}

function retryButton() {
  return `<button onclick="location.href='dshlauncher://retry/'" style="margin-bottom:16px">Проверить снова</button>`;
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
`;

function pageHtml(title, body, isError) {
  const html = `<!doctype html><html><head><meta charset="utf-8"><title>${esc(title)}</title><style>${PAGE_CSS}</style></head><body><div class="wrap${isError ? ' err' : ''}"><h1><span class="dot"></span>${esc(title)}</h1>${body}</div></body></html>`;
  return 'data:text/html;charset=utf-8,' + encodeURIComponent(html);
}

function stderrBlock() {
  return `<pre>${esc(stderrTail.slice(-50).join('\n') || 'нет данных журнала')}</pre>`;
}

function showStatus(title, body, isError) {
  if (win && !win.isDestroyed()) win.loadURL(pageHtml(title, body, !!isError));
}

function showPastePage() {
  const body = `
    <p>Похоже, <code>dsh</code> уже запущен, но у окна нет валидного сессионного куки
       (он мог истечь или dsh был перезапущен с другим секретом).</p>
    <p>В терминале, где работает dsh, найдите строку
       <code>dsh web: http://127.0.0.1:3080/?token=…</code> и вставьте сюда полный URL
       (или только токен):</p>
    <input id="u" type="text" placeholder="http://127.0.0.1:3080/?token=…" autocomplete="off" spellcheck="false">
    <button onclick="go()">Открыть</button>
     <p style="margin-top:16px">Токен взять неоткуда (dsh запущен сервисом, терминала нет)?
        Тогда остановите dsh (например, <code>pkill -f 'dsh --profile web'</code>), закройте
        это окно и запустите <code>dsh-launcher</code> снова — лаунчер поднимет dsh сам и
        подключится без токена.</p>
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
  showStatus('Нужен токен', body, true);
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
      await win.loadURL(PLAIN_URL);
    } else if (await plainUrlIsAuthed()) {
      await win.loadURL(PLAIN_URL); // окно на статус-странице, но куки валидны
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
    portState = await portOpen(); // базовое состояние, без срабатываний
    setInterval(async () => {
      if (stopping) return;
      const state = (await portOpen()) ? 'up' : 'down';
      if (state === portState) return;
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

function onPasteRequest(request) {
  try {
    const u = new URL(request.url);
    const pasted = decodeURIComponent(u.pathname.replace(/^\/paste\//, ''));
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
          await win.loadURL(PLAIN_URL);
        } else {
          console.log('[launcher] retry: dsh запущен, валидного куки нет — страница вставки');
          showPastePage();
        }
      } else {
        showStatus('dsh не запущен',
          `<p>Порт ${PORT} закрыт — <code>dsh</code> не запущен.</p>
             <p>Запустите его в терминале: <code>${esc(DSH_BIN)} ${esc(DSH_ARGS.join(' '))}</code>
             и нажмите «Проверить снова». Либо закройте окно и запустите
             dsh-launcher снова.</p>`, false);
      }
    }
  } catch (e) {
    console.error('[launcher] onRetryRequest:', e.message);
  }
  return new Response('<!doctype html><html><body style="background:#0f1115;margin:0"></body></html>',
    { headers: { 'content-type': 'text/html' } });
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
  Menu.setApplicationMenu(null);
  protocol.handle('dshlauncher', (request) => {
    try {
      const u = new URL(request.url);
      if (u.pathname.startsWith('/retry/')) return onRetryRequest();
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
  win.webContents.on('did-finish-load', () => {
    try { console.log('[launcher] страница загружена:', win.webContents.getURL().slice(0, 120)); } catch { /* noop */ }
  });
  win.webContents.on('did-fail-load', (_e, code, desc, url, isMainFrame) => {
    if (isMainFrame) console.error(`[launcher] ошибка загрузки: code=${code} ${desc} url=${String(url).slice(0, 120)}`);
  });
  win.webContents.on('render-process-gone', (_e, details) => {
    console.error('[launcher] рендер-процесс завершился:', details.reason, 'exitCode=' + details.exitCode);
  });

  const alreadyRunning = await portOpen();
  if (alreadyRunning) {
    console.log('[launcher] port already open — attaching to running dsh');
    showStatus('Подключение…',
      `<p><code>dsh</code> уже запущен на порту ${PORT}. Подключаюсь…</p>`);
    if (await plainUrlIsAuthed()) {
      console.log('[launcher] валидный куки есть — открываю GUI');
      await win.loadURL(PLAIN_URL);
    } else {
      console.log('[launcher] валидного куки нет — показываю страницу вставки токена');
      showPastePage();
    }
    startWatchdog();
    return;
  }

  showStatus('Запуск DeepSeek Harness…',
    `<p>Запускаю <code>${esc(DSH_BIN)} ${esc(DSH_ARGS.join(' '))}</code>.<br>Жду, пока сервис поднимется…</p>`);
  startDsh();

  const up = await waitForPort();
  if (!up) {
    showStatus('Не удалось запустить dsh',
      `<p>Процесс не поднял сервис на <code>${PLAIN_URL}</code>.<br>
         Проверьте, что команда запускается в терминале:
         <code>${esc(DSH_BIN)} ${esc(DSH_ARGS.join(' '))}</code>.<br>
         Последние строки журнала:</p>${stderrBlock()}`, true);
    startWatchdog(); // если dsh запустят вручную — окно оживёт само
    return;
  }

  const url = authUrl || (await waitForAuthUrl());
  if (url) {
    await win.loadURL(url); // обмен токена на куки, затем редирект на чистый URL
  } else if (await plainUrlIsAuthed()) {
    await win.loadURL(PLAIN_URL); // старое куки ещё валидно
  } else {
    showPastePage();
  }
  startWatchdog();
}

/* ---------------- закрытие = остановка ---------------- */

app.on('window-all-closed', () => app.quit());

app.on('before-quit', (e) => {
  if (stopping) return;
  stopping = true;
  e.preventDefault();
  Promise.resolve().then(() => stopDsh()).finally(() => app.quit());
});

// страховка: если before-quit почему-то не сработала — жёстко
app.on('will-quit', () => {
  if (weStartedDsh && dshProc && dshProc.pid !== undefined && dshProc.exitCode === null && !dshFatal) {
    killGroup(dshProc.pid, 'SIGKILL');
  }
});
