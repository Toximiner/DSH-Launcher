'use strict';
// Settings: the final parameters (environment variables take precedence over settings.json),
// validation of the «Настройки» window field values, what requires a restart.
const { tr } = require('./i18n');
const { splitArgs } = require('./util');

const DEFAULT_PORT = 3080;
const DEFAULT_PROFILE = 'web';

/* The final parameters from the environment (takes precedence) and settings.json; src[key] —
   where it came from: 'env' | 'settings' | 'default'. */
function effectiveConfig(env, settings, home) {
  const s = settings && typeof settings === 'object' ? settings : {};
  const src = {};
  const from = (key, where) => { src[key] = where; };

  let port = DEFAULT_PORT;
  if (env.DSH_PORT) { port = Number(env.DSH_PORT); from('port', 'env'); }
  else if (Number.isInteger(s.port) && s.port >= 1 && s.port <= 65535) { port = s.port; from('port', 'settings'); }
  else from('port', 'default');

  let args;
  let profile = DEFAULT_PROFILE;
  if (env.DSH_ARGS) {
    args = splitArgs(env.DSH_ARGS);
    from('profile', 'env');
  } else {
    if (typeof s.profile === 'string' && /^[\w.-]+$/.test(s.profile)) { profile = s.profile; from('profile', 'settings'); }
    else from('profile', 'default');
    args = ['--profile', profile, '--no-open'];
    if (port !== DEFAULT_PORT) args.push('--port', String(port)); // dsh listens on the same port the launcher looks for
  }

  let dshBin = null; // null — look up automatically
  if (env.DSH_BIN) { dshBin = env.DSH_BIN; from('dshPath', 'env'); }
  else if (typeof s.dshPath === 'string' && s.dshPath) { dshBin = s.dshPath; from('dshPath', 'settings'); }
  else from('dshPath', 'default');

  let cwd = home;
  if (env.DSH_CWD) { cwd = env.DSH_CWD; from('cwd', 'env'); }
  else if (typeof s.cwd === 'string' && s.cwd) { cwd = s.cwd; from('cwd', 'settings'); }
  else from('cwd', 'default');

  let noGpu = false;
  if (env.DSH_LAUNCHER_NO_GPU === '1') { noGpu = true; from('gpu', 'env'); }
  else if (s.gpu === 'off') { noGpu = true; from('gpu', 'settings'); }
  else from('gpu', 'default');

  let ozone = 'auto';
  const envOzone = String(env.DSH_LAUNCHER_OZONE || '').toLowerCase();
  if (envOzone === 'x11' || envOzone === 'wayland') { ozone = envOzone; from('ozone', 'env'); }
  else if (s.ozone === 'x11' || s.ozone === 'wayland') { ozone = s.ozone; from('ozone', 'settings'); }
  else from('ozone', 'default');

  let checkUpdates = true;
  if (/^(1|true|yes)$/i.test(env.DSH_LAUNCHER_NO_UPDATE_CHECK || '')) { checkUpdates = false; from('checkUpdates', 'env'); }
  else if (s.checkUpdates === false) { checkUpdates = false; from('checkUpdates', 'settings'); }
  else from('checkUpdates', 'default');

  // a check every 12 hours while the launcher is open — a separate checkbox;
  // DSH_LAUNCHER_NO_UPDATE_CHECK forbids any checks
  let pollUpdates = true;
  if (src.checkUpdates === 'env') { pollUpdates = false; from('pollUpdates', 'env'); }
  else if (s.pollUpdates === false) { pollUpdates = false; from('pollUpdates', 'settings'); }
  else from('pollUpdates', 'default');

  // the Chromium sandbox — only via the checkbox (run.sh reads the same settings.json)
  const sandbox = s.sandbox === true;
  from('sandbox', sandbox ? 'settings' : 'default');

  return { port, args, profile, dshBin, cwd, noGpu, ozone, checkUpdates, pollUpdates, sandbox, src };
}

/* A window field value → { value } or { error } (a message for the window). */
function settingValue(key, raw) {
  const v = String(raw === undefined || raw === null ? '' : raw).trim();
  const abs = (x) => x === '' || x.startsWith('/');
  switch (key) {
    case 'port': {
      const n = Number(v);
      if (!/^\d+$/.test(v) || n < 1024 || n > 65535) return { error: tr('Порт — число от 1024 до 65535.', 'The port must be a number from 1024 to 65535.') };
      return { value: n };
    }
    case 'profile':
      if (!/^[\w.-]+$/.test(v) || /^\.+$/.test(v)) return { error: tr('Профиль — латинские буквы, цифры, «.», «_», «-».', 'The profile may contain Latin letters, digits, “.”, “_”, “-”.') };
      return { value: v };
    case 'dshPath':
    case 'cwd':
      if (!abs(v)) return { error: tr('Нужен полный путь (начинается с /).', 'A full path is needed (starting with /).') };
      return { value: v };
    case 'checkUpdates':
    case 'pollUpdates':
    case 'closeToTray':
    case 'attention':
    case 'attentionSound':
    case 'sandbox':
    case 'spellcheck':
    case 'inputHistory':
      if (v !== 'true' && v !== 'false') return { error: 'bad value' };
      return { value: v === 'true' };
    case 'lang':
      return ['auto', 'ru', 'en'].includes(v) ? { value: v } : { error: 'bad value' };
    case 'gpu':
      return ['auto', 'off'].includes(v) ? { value: v } : { error: 'bad value' };
    case 'ozone':
      return ['auto', 'wayland', 'x11'].includes(v) ? { value: v } : { error: 'bad value' };
    default:
      return { error: `unknown setting ${key}` };
  }
}

/* Which of the effective parameters will change after a launcher restart. */
function restartNeeded(running, next) {
  const changed = ['port', 'dshBin', 'cwd', 'noGpu', 'ozone', 'sandbox'].filter((k) => running[k] !== next[k]);
  if (JSON.stringify(running.args) !== JSON.stringify(next.args)) changed.push('args');
  return changed;
}

module.exports = { DEFAULT_PORT, DEFAULT_PROFILE, effectiveConfig, settingValue, restartNeeded };
