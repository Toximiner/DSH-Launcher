'use strict';
// GPU acceleration: detection of a real GPU (/dev/dri), EGL libraries, a Wayland
// session (ozone-platform), the "GPU driver is broken" marker and a counter of
// GPU-process crashes. Detection runs at module load (no electron —
// app is passed into apply/onChildProcessGone); the commandLine/
// disableHardwareAcceleration decisions — in apply(), which main.js calls before
// app ready.
const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const state = require('./state');
const { CONFIG, LOG_DIR } = require('./env');

// Whether the system has a real GPU (render nodes /dev/dri/renderD*).
// VMs/VNC without 3D acceleration do not have them.
const hasGpuRenderNode = (() => {
  try { return fs.readdirSync('/dev/dri').some((f) => /^renderD/.test(f)); }
  catch { return false; }
})();

// A window on Wayland is presented through EGL. If a VM's GPU has no working
// driver (ANGLE/EGL errors in the log), the window is "shown" per Electron's logs, but
// it is not visible on the screen. The X11/XWayland path degrades to software rendering —
// the window stays visible. On X11 sessions we change nothing.
// If a real GPU is present (renderD* in /dev/dri) — we do not force X11: native
// Wayland works, while an X11/XWayland window on KWin remains invisible. Manual
// switch: DSH_LAUNCHER_OZONE=x11|wayland.
const isWaylandSession = process.platform === 'linux' &&
  (String(process.env.XDG_SESSION_TYPE || '').toLowerCase() === 'wayland' ||
   Boolean(process.env.WAYLAND_DISPLAY));

// VM/VNC without 3D acceleration: no render nodes, EGL is guaranteed not to come up —
// go straight to software rendering, skipping the GPU-process crash loop (ANGLE/EGL
// errors in the log). The same if the EGL libraries are absent from the system entirely. Manual
// switch: DSH_LAUNCHER_NO_GPU=1.
const hasEglLibs = (() => {
  try {
    const out = execFileSync('ldconfig', ['-p'], { encoding: 'utf8', timeout: 2000 });
    return /libEGL\.so\.1/.test(out) && /libGLESv2\.so\.[12]/.test(out);
  } catch { return true; } // could not check — the decision is not changed
})();

// The "GPU driver is broken" marker. A broken driver (SIGSEGV in the GPU process) is not
// reliably detected at start — but the crash is visible at runtime. We remember it
// in a file: the next starts go straight to software rendering. The marker lives 30
// days (in case the driver got fixed); to return to GPU earlier — delete the
// file.
const GPU_MARKER_FILE = path.join(LOG_DIR, 'gpu-disabled.marker');
const GPU_MARKER_TTL_MS = 30 * 24 * 60 * 60 * 1000;
const gpuMarkerAgeMs = (() => {
  try { return Date.now() - fs.statSync(GPU_MARKER_FILE).mtimeMs; }
  catch { return null; }
})();
const gpuDisabledByMarker = process.platform === 'linux' &&
  gpuMarkerAgeMs !== null && gpuMarkerAgeMs < GPU_MARKER_TTL_MS;

const noGpuRequested = CONFIG.noGpu; // DSH_LAUNCHER_NO_GPU=1 or "Settings" → "Off"

// ozone-platform and disableHardwareAcceleration must be applied before app ready
// (main.js calls this at load, right after starting the log).
function apply(app) {
  if (isWaylandSession) {
    // An error in the DSH_LAUNCHER_OZONE value is not passed to Electron:
    // an unknown ozone-platform may fail to initialize at all.
    const ozoneManual = CONFIG.ozone !== 'auto'; // DSH_LAUNCHER_OZONE or "Settings"
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

/* ===== onChildProcessGone (test test/gpu.js) =====
   Two or more GPU-process deaths in one session = the driver is broken: we write the marker
   so the next starts bypass the GPU immediately. Installed at main.js load
   (before ready). state/fs/console/GPU_MARKER_FILE/gpuDisabledByMarker/LOG_DIR are
   injectable (the test replaces them). */
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
/* ===== end onChildProcessGone ===== */

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
