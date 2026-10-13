'use strict';
// Параметры запуска: что пришло из окружения и settings.json (см. config.js),
// пути, порт, журналы. Модуль подключается первым: токен CI и флаг
// «Обновить сейчас» убираются из окружения до запуска любых процессов.
const fs = require('fs');
const os = require('os');
const path = require('path');
const { effectiveConfig } = require('./config');

// Только для разработки и тестов (CI): токен GitHub для запроса списка
// релизов — лимит 5000 запросов в час вместо 60 без авторизации. Читаем и
// убираем из окружения первым делом — до любого запуска дочерних процессов:
// его не унаследуют dsh, npm и плагины. Значение нигде не печатается.
const GITHUB_TOKEN = process.env.DSH_LAUNCHER_GITHUB_TOKEN || '';
delete process.env.DSH_LAUNCHER_GITHUB_TOKEN;
// Только для тестов (Docker): подставной GitHub API на этой машине
// (test/docker/fakes/github.py) — релизы и .deb без настоящего GitHub.
/* ===== githubApiBase (чистая функция, тест test/parsing.js) =====
   Принимается только http://127.0.0.1:<порт>, иначе — настоящий GitHub. */
function githubApiBase(v) {
  return /^http:\/\/127\.0\.0\.1:\d{1,5}$/.test(String(v || '')) ? v : 'https://api.github.com';
}
/* ===== end githubApiBase ===== */
const GITHUB_API = githubApiBase(process.env.DSH_LAUNCHER_GITHUB_API);
// «Обновить сейчас» (окно обновления во время работы) перезапускает лаунчер с
// этой переменной: новый экземпляр сразу ставит обновление, без вопроса.
const UPDATE_NOW = /^(launcher|dsh)$/.test(process.env.DSH_LAUNCHER_UPDATE_NOW || '') ? process.env.DSH_LAUNCHER_UPDATE_NOW : null;
delete process.env.DSH_LAUNCHER_UPDATE_NOW;
// «Вернуть предыдущую версию…»: какую версию лаунчера поставить (вместе с
// DSH_LAUNCHER_UPDATE_NOW=launcher). Только вида 1.2.3[-N].
const INSTALL_VERSION = UPDATE_NOW === 'launcher' && /^\d+\.\d+\.\d+(-\d+)?$/.test(process.env.DSH_LAUNCHER_INSTALL_VERSION || '')
  ? process.env.DSH_LAUNCHER_INSTALL_VERSION : null;
delete process.env.DSH_LAUNCHER_INSTALL_VERSION;

// Окно «Настройки» пишет ~/.config/dsh-launcher/settings.json (это же userData
// лаунчера — путь считаем сами: настройки нужны до app.setName). Переменные
// окружения важнее файла — ими пользуются тесты и те, кто настроил лаунчер
// раньше; поле в окне тогда неактивно.
const SETTINGS_FILE = path.join(process.env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config'),
  'dsh-launcher', 'settings.json');

function loadSettings() {
  try { const s = JSON.parse(fs.readFileSync(SETTINGS_FILE, 'utf8')); return s && typeof s === 'object' ? s : {}; }
  catch { return {}; }
}

const CONFIG = effectiveConfig(process.env, loadSettings(), os.homedir());

const DSH_BIN_DEFAULT = '/usr/bin/dsh';
// путь к dsh задан явно (переменная DSH_BIN или «Настройки»); иначе, если
// /usr/bin/dsh нет, — ищется в PATH, npm prefix, nvm и т.п. (см. findDsh),
// найденный путь запоминается
const DSH_BIN_EXPLICIT = Boolean(CONFIG.dshBin);
const DSH_ARGS = CONFIG.args;
const CWD = CONFIG.cwd; // рабочая директория для dsh
const HOST = '127.0.0.1';
const PORT = CONFIG.port;
const PLAIN_URL = `http://${HOST}:${PORT}/`;
const START_TIMEOUT = Number(process.env.DSH_START_TIMEOUT_MS || 120000); // сколько ждать старта
// в установленной версии (run.sh ставит DSH_LOG_DIR) — в ~/.local/state,
// в dev-режиме — рядом с приложением
const LOG_DIR = process.env.DSH_LOG_DIR || path.join(__dirname, '..', 'logs');
const LOG_FILE = path.join(LOG_DIR, 'dsh.log');
const LAUNCHER_LOG_FILE = path.join(LOG_DIR, 'launcher.log');
// 2 МБ — дальше файл сдвигается в *.1 (DSH_LAUNCHER_LOG_MAX_BYTES — для тестов).
// Старый *.1 при этом заменяется: на каждый журнал не больше двух файлов.
const MAX_LOG_BYTES = Number(process.env.DSH_LAUNCHER_LOG_MAX_BYTES) > 0
  ? Number(process.env.DSH_LAUNCHER_LOG_MAX_BYTES) : 2 * 1024 * 1024;

// Язык окон лаунчера: DSH_LAUNCHER_LANG=ru|en, иначе выбор переключателем
// RU | EN в окне (userData/ui-lang.json, подхватывается в onReady), иначе по
// локали системы (LC_ALL / LC_MESSAGES / LANG): ru* — русский, иначе английский.
const LANG_FORCED = (() => {
  const v = String(process.env.DSH_LAUNCHER_LANG || '').toLowerCase();
  return v === 'ru' || v === 'en' ? v : null;
})();

module.exports = {
  GITHUB_TOKEN, GITHUB_API, UPDATE_NOW, INSTALL_VERSION, SETTINGS_FILE, loadSettings, CONFIG, DSH_BIN_DEFAULT, DSH_BIN_EXPLICIT, DSH_ARGS, CWD,
  HOST, PORT, PLAIN_URL, START_TIMEOUT, LOG_DIR, LOG_FILE, LAUNCHER_LOG_FILE, MAX_LOG_BYTES, LANG_FORCED,
};
