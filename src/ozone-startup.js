'use strict';
// Resolve Ozone before Electron initializes its native window backend. Changing
// the switch in main.js can leave the main process and renderer on different backends.
const fs = require('fs');
const os = require('os');
const path = require('path');
const { effectiveConfig } = require('./config');

/* ===== startupOzone (test test/ozone-startup.js) ===== */
function startupOzone(argv, env, settings, platform, hasRenderNode) {
  // Preserve explicit Chromium arguments, including the space-separated form.
  if (argv.some((arg) => arg === '--ozone-platform' || arg.startsWith('--ozone-platform='))) return '';
  if (platform !== 'linux') return '';
  const config = effectiveConfig(env, settings, '');
  if (config.ozone !== 'auto') return config.ozone;
  const wayland = String(env.XDG_SESSION_TYPE || '').toLowerCase() === 'wayland' || Boolean(env.WAYLAND_DISPLAY);
  return wayland ? (hasRenderNode ? 'wayland' : 'x11') : '';
}
/* ===== end startupOzone ===== */

/* ===== restartLauncherArgs (test test/ozone-startup.js) ===== */
function restartLauncherArgs(argv, automaticOzone) {
  // argv[1] is the app directory. Re-enter run.sh so settings are resolved again;
  // discard only the switch added by run.sh, retaining explicit user overrides.
  return argv.slice(2).filter((arg) => arg !== '--no-sandbox' &&
    !(automaticOzone && arg === `--ozone-platform=${automaticOzone}`));
}
/* ===== end restartLauncherArgs ===== */

if (require.main === module) {
  const settingsFile = path.join(process.env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config'), 'dsh-launcher', 'settings.json');
  let settings = {};
  try { settings = JSON.parse(fs.readFileSync(settingsFile, 'utf8')); } catch { /* missing or invalid settings */ }
  let hasRenderNode = false;
  try { hasRenderNode = /^renderD/m.test(fs.readdirSync('/dev/dri').join('\n')); } catch { /* no GPU */ }
  process.stdout.write(startupOzone(process.argv.slice(2), process.env, settings, process.platform, hasRenderNode));
}

module.exports = { startupOzone, restartLauncherArgs };
