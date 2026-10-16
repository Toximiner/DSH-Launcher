'use strict';
// GPU-ускорение: детект реального GPU (/dev/dri), EGL-библиотек, Wayland-
// сессии (ozone-platform), маркера «GPU-драйвер сломан» и счётчик падений
// GPU-процесса. Детект выполняется при загрузке модуля (без electron —
// app передаётся в apply/onChildProcessGone), решения в commandLine/
// disableHardwareAcceleration — в apply(), которую main.js вызывает до
// app ready.
const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const state = require('./state');
const { CONFIG, LOG_DIR } = require('./env');

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

const noGpuRequested = CONFIG.noGpu; // DSH_LAUNCHER_NO_GPU=1 или «Настройки» → «Выключено»

// ozone-platform и disableHardwareAcceleration должны применяться до app ready
// (main.js вызывает это при загрузке, сразу после старта журнала).
function apply(app) {
  if (isWaylandSession) {
    // Ошибку в значении DSH_LAUNCHER_OZONE не передаём в Electron:
    // неизвестный ozone-platform может не инициализироваться вовсе.
    const ozoneManual = CONFIG.ozone !== 'auto'; // DSH_LAUNCHER_OZONE или «Настройки»
    const ozone = ozoneManual ? CONFIG.ozone : hasGpuRenderNode ? 'wayland' : 'x11';
    app.commandLine.appendSwitch('ozone-platform', ozone);
    console.log(
      `[launcher] Wayland-сессия — ozone-platform=${ozone}` +
      (ozoneManual
        ? (CONFIG.src.ozone === 'env' ? ' (принудительно: DSH_LAUNCHER_OZONE)' : ' (выбрано в «Настройках»)')
        : hasGpuRenderNode
          ? ' (есть реальный GPU — нативный Wayland; X11/XWayland-окно на KWin невидимо)'
          : ' (render-узлов нет — окно веду через X11 (XWayland))')
    );
  }

  if (process.platform === 'linux' && (noGpuRequested || gpuDisabledByMarker || !hasGpuRenderNode || !hasEglLibs)) {
    app.disableHardwareAcceleration();
    const why = noGpuRequested ? (CONFIG.src.gpu === 'env' ? 'DSH_LAUNCHER_NO_GPU=1' : '«Настройки»: GPU-ускорение — выключено')
      : gpuDisabledByMarker ? `маркер ${GPU_MARKER_FILE} (GPU-процесс падал, маркер живёт 30 дней; удалить файл, чтобы включить GPU)`
      : !hasGpuRenderNode ? 'render-узлов в /dev/dri нет'
      : 'EGL-библиотек (libEGL/libGLESv2) в системе нет';
    console.log(`[launcher] ${why} — отключаю GPU-ускорение (программный рендер)`);
  }
}

// Две и более смерти GPU-процесса за сессию = драйвер сломан: пишем маркер,
// чтобы следующие запуски сразу обходили GPU. Ставится при загрузке main.js
// (до ready), как и раньше.
function onChildProcessGone(app, restartLauncher) {
  let gpuCrashCount = 0;
  app.on('child-process-gone', (_event, details) => {
    if (state.launcherUpdatedOnDisk) {
      console.log(`[launcher] процесс ${details.type} завершился после обновления на диске — перезапускаюсь`);
      restartLauncher();
      return;
    }
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
}

module.exports = {
  hasGpuRenderNode,
  hasEglLibs,
  GPU_MARKER_FILE,
  GPU_MARKER_TTL_MS,
  gpuDisabledByMarker,
  noGpuRequested,
  apply,
  onChildProcessGone,
};
