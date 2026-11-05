'use strict';
// Launch parameters: what came from the environment and settings.json (see config.js),
// paths, port, logs. This module is loaded first: the CI token and the
// "Update now" flag are removed from the environment before any processes are spawned.
const fs = require('fs');
const os = require('os');
const path = require('path');
const crypto = require('crypto');
const { effectiveConfig } = require('./config');
const { validRouteKey } = require('./util');

// Only for development and tests (CI): GitHub token for requesting the list
// of releases — the limit is 5000 requests per hour instead of 60 unauthenticated. We read it
// and remove it from the environment first thing — before spawning any child processes:
// dsh, npm and the plugins must not inherit it. The value is never printed anywhere.
const GITHUB_TOKEN = process.env.DSH_LAUNCHER_GITHUB_TOKEN || '';
delete process.env.DSH_LAUNCHER_GITHUB_TOKEN;
// Key for the dshlauncher://… routes (see withRouteKey in util.js) — random for
// each start. DSH_LAUNCHER_ROUTE_KEY — for tests only (Docker): the key
// is known in advance, and the window driver goes through the routes. A restarted
// instance inherits the variable and the same key.
const ROUTE_KEY = validRouteKey(process.env.DSH_LAUNCHER_ROUTE_KEY)
  ? process.env.DSH_LAUNCHER_ROUTE_KEY : crypto.randomBytes(16).toString('hex');
// For tests only (Docker): a stand-in GitHub API on this machine
// (test/docker/fakes/github.py) — releases and .deb without the real GitHub.
/* ===== githubApiBase (pure function, test test/parsing.js) =====
   Only http://127.0.0.1:<port> is accepted, otherwise — the real GitHub. */
function githubApiBase(v) {
  return /^http:\/\/127\.0\.0\.1:\d{1,5}$/.test(String(v || '')) ? v : 'https://api.github.com';
}
/* ===== end githubApiBase ===== */
const GITHUB_API = githubApiBase(process.env.DSH_LAUNCHER_GITHUB_API);
// "Update now" (the update window while running) restarts the launcher with
// this variable: the new instance applies the update immediately, without a prompt.
const UPDATE_NOW = /^(launcher|dsh)$/.test(process.env.DSH_LAUNCHER_UPDATE_NOW || '') ? process.env.DSH_LAUNCHER_UPDATE_NOW : null;
delete process.env.DSH_LAUNCHER_UPDATE_NOW;
// "Revert to the previous version…": which launcher version to install (together with
// DSH_LAUNCHER_UPDATE_NOW=launcher). Only of the form 1.2.3[-N].
const INSTALL_VERSION = UPDATE_NOW === 'launcher' && /^\d+\.\d+\.\d+(-\d+)?$/.test(process.env.DSH_LAUNCHER_INSTALL_VERSION || '')
  ? process.env.DSH_LAUNCHER_INSTALL_VERSION : null;
delete process.env.DSH_LAUNCHER_INSTALL_VERSION;

// The "Settings" window writes ~/.config/dsh-launcher/settings.json (this is also the
// launcher's userData — the path is computed here: the settings are needed before app.setName). Environment
// variables are stronger than the file — tests and people who configured the launcher
// earlier use them; the field in the window is then inactive.
const SETTINGS_FILE = path.join(process.env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config'),
  'dsh-launcher', 'settings.json');

// Whether this is a package install (run.sh places Electron in /opt/dsh-launcher) — one
// source of truth: the dpkg version against package.json, the version comparison scheme
// (deb against semver), the "built from source" label. execPath does not change over
// the life of the process, hence the constant.
const IS_PKG_INSTALL = process.execPath.startsWith('/opt/dsh-launcher/');

function loadSettings() {
  try { const s = JSON.parse(fs.readFileSync(SETTINGS_FILE, 'utf8')); return s && typeof s === 'object' ? s : {}; }
  catch { return {}; }
}

const CONFIG = effectiveConfig(process.env, loadSettings(), os.homedir());

const DSH_BIN_DEFAULT = '/usr/bin/dsh';
// the path to dsh is given explicitly (DSH_BIN variable or "Settings"); otherwise, if
// /usr/bin/dsh is missing, — it is searched in PATH, the npm prefix, nvm, etc. (see findDsh),
// and the found path is remembered
const DSH_BIN_EXPLICIT = Boolean(CONFIG.dshBin);
const DSH_ARGS = CONFIG.args;
const CWD = CONFIG.cwd; // working directory for dsh
const HOST = '127.0.0.1';
const PORT = CONFIG.port;
const PLAIN_URL = `http://${HOST}:${PORT}/`;
const START_TIMEOUT = Number(process.env.DSH_START_TIMEOUT_MS || 120000); // how long to wait for the start
// in the installed version (run.sh sets DSH_LOG_DIR) — in ~/.local/state,
// in dev mode — next to the application
const LOG_DIR = process.env.DSH_LOG_DIR || path.join(__dirname, '..', 'logs');
const LOG_FILE = path.join(LOG_DIR, 'dsh.log');
const LAUNCHER_LOG_FILE = path.join(LOG_DIR, 'launcher.log');
// 2 MB — beyond that the file is rolled to *.1 (DSH_LAUNCHER_LOG_MAX_BYTES — for tests).
// The old *.1 is replaced at the same time: at most two files per log.
const MAX_LOG_BYTES = Number(process.env.DSH_LAUNCHER_LOG_MAX_BYTES) > 0
  ? Number(process.env.DSH_LAUNCHER_LOG_MAX_BYTES) : 2 * 1024 * 1024;

// Language of the launcher's windows: DSH_LAUNCHER_LANG=ru|en, otherwise chosen by the
// RU | EN switch in the window (userData/ui-lang.json, picked up in onReady), otherwise by the
// system locale (LC_ALL / LC_MESSAGES / LANG): ru* — Russian, otherwise English.
const LANG_FORCED = (() => {
  const v = String(process.env.DSH_LAUNCHER_LANG || '').toLowerCase();
  return v === 'ru' || v === 'en' ? v : null;
})();

module.exports = {
  GITHUB_TOKEN, GITHUB_API, ROUTE_KEY, UPDATE_NOW, INSTALL_VERSION, SETTINGS_FILE, loadSettings, CONFIG, DSH_BIN_DEFAULT, DSH_BIN_EXPLICIT, DSH_ARGS, CWD,
  HOST, PORT, PLAIN_URL, START_TIMEOUT, LOG_DIR, LOG_FILE, LAUNCHER_LOG_FILE, MAX_LOG_BYTES, LANG_FORCED, IS_PKG_INSTALL,
};
