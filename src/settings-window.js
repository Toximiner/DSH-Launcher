'use strict';
// Окно «Настройки»: разделы, сохранение полей, перезапуск лаунчера.
const { app, dialog } = require('electron');
const { spawn } = require('child_process');
const fs = require('fs');
const os = require('os');
const path = require('path');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc, profileFromArgs } = require('./util');
const { DEFAULT_PORT, DEFAULT_PROFILE, effectiveConfig, settingValue, restartNeeded } = require('./config');
const { SETTINGS_FILE, loadSettings, CONFIG, CWD, LANG_FORCED } = require('./env');
const dialogs = require('./dialogs');
const dshProcess = require('./dsh-process');
const fileHelp = require('./file-help');
const menus = require('./menus');
const pages = require('./pages');
const tray = require('./tray');
const updatesLive = require('./updates-live');
const updates = require('./updates');
const view = require('./view');
const { noContentResponse } = require('./respond'); // чистый модуль — прямой require, без реестра
const { inputHistoryOn, inputHistoryScript } = require('./input-history'); // ↑/↓: история введённого в поле dsh

/* ---------------- окно «Настройки» ---------------- */

// «Файл → Настройки…» (Ctrl+,). Каждое поле сохраняется сразу (переход на
// dshlauncher://settings/set/?k=…&v=…, ответ 204); язык, орфография, маркет,
// «снова спрашивать» действуют сразу, проверка обновлений — со следующего
// запуска, остальное — после перезапуска лаунчера (кнопка в окне).



function saveSettings(patch) {
  const next = { ...loadSettings(), ...patch };
  for (const k of Object.keys(next)) if (next[k] === null || next[k] === '') delete next[k];
  fs.mkdirSync(path.dirname(SETTINGS_FILE), { recursive: true });
  fs.writeFileSync(SETTINGS_FILE, JSON.stringify(next, null, 2));
  return next;
}

function setLanguage(lang) {
  if (lang === 'auto') {
    try { fs.rmSync(pages.langPrefFile(), { force: true }); } catch { /* noop */ }
    i18n.setLang(/^ru/i.test(process.env.LC_ALL || process.env.LC_MESSAGES || process.env.LANG || '') ? 'ru' : 'en');
    menus.buildAppMenu();
    if (state.currentPage) state.currentPage();
  } else {
    pages.onLangSwitch(lang);
  }
}

function langSetting() {
  try { JSON.parse(fs.readFileSync(pages.langPrefFile(), 'utf8')); return i18n.getLang(); } catch { return 'auto'; }
}

function updateDismissals() {
  const p = updates.updatePrefs();
  return Object.keys(p).filter((k) => p[k]).map((k) => `${k === 'launcher' ? 'DeepSeek Harness Launcher' : 'dsh'} ${p[k]}`);
}

let settingsNote = null; // { text, error }

function onSettingsSet(url) {
  const q = new URL(url).searchParams;
  const key = q.get('k');
  const r = settingValue(key, q.get('v'));
  if (r.error) {
    settingsNote = { text: r.error, error: true };
  } else {
    try {
      if (key === 'lang') setLanguage(r.value);
      else if (key === 'spellcheck') view.setSpellcheck(r.value);
      else if (key === 'gpu' || key === 'ozone') saveSettings({ [key]: r.value === 'auto' ? null : r.value });
      else if (key === 'port') saveSettings({ port: r.value === DEFAULT_PORT ? null : r.value });
      else if (key === 'profile') saveSettings({ profile: r.value === DEFAULT_PROFILE ? null : r.value });
      else if (key === 'checkUpdates') saveSettings({ checkUpdates: r.value ? null : false });
      else if (key === 'pollUpdates') { saveSettings({ pollUpdates: r.value ? null : false }); updatesLive.setUpdatePolling(r.value); }
      else if (key === 'closeToTray') { saveSettings({ closeToTray: r.value ? true : null }); tray.setTray(r.value); }
      else if (key === 'attention') saveSettings({ attention: r.value ? null : false });
      else if (key === 'sandbox') saveSettings({ sandbox: r.value ? true : null });
      else if (key === 'attentionSound') { saveSettings({ attentionSound: r.value ? true : null }); if (r.value && state.win) tray.playAgentSound(); } // сразу — услышать
      else if (key === 'inputHistory') { // сразу: впрыск в уже открытую страницу dsh
        saveSettings({ inputHistory: r.value ? null : false });
        try {
          if (state.win && !state.win.isDestroyed()) {
            const js = r.value ? inputHistoryScript() : 'window.__dshInputHistory && window.__dshInputHistory.setEnabled(false); 1';
            void state.win.webContents.executeJavaScript(js).catch(() => {});
          }
        } catch { /* noop */ }
      }
      else saveSettings({ [key]: r.value || null }); // dshPath, cwd: '' — по умолчанию
      console.log(`[launcher] настройки: ${key} = ${key === 'dshPath' || key === 'cwd' ? (r.value || '(по умолчанию)') : r.value}`);
      settingsNote = { text: tr('Сохранено', 'Saved'), error: false };
    } catch (e) {
      settingsNote = { text: e.message, error: true };
    }
  }
  if (state.dialogRender) state.dialogRender(false);
  return noContentResponse();
}

async function onSettingsPick(kind) {
  if (!dialogs.dialogOpen()) return noContentResponse();
  const isDir = kind === 'cwd';
  const r = await dialog.showOpenDialog(state.win, {
    title: isDir ? tr('Рабочая папка dsh', 'dsh working folder') : tr('Программа dsh', 'The dsh program'),
    defaultPath: isDir ? CWD : path.dirname(state.dshBin),
    properties: isDir ? ['openDirectory'] : ['openFile'],
  });
  if (!r.canceled && r.filePaths[0]) {
    return onSettingsSet(`dshlauncher://settings/set/?k=${kind}&v=${encodeURIComponent(r.filePaths[0])}`);
  }
  return noContentResponse();
}

function onSettingsResetUpdates() {
  try { fs.writeFileSync(updates.updatePrefFile(), '{}'); } catch { /* noop */ }
  console.log('[launcher] настройки: снова спрашивать об обновлениях');
  settingsNote = { text: tr('Об отложенных обновлениях лаунчер снова спросит при запуске.', 'The launcher will ask about postponed updates again at startup.'), error: false };
  if (state.dialogRender) state.dialogRender(false);
  return noContentResponse();
}

// Перезапуск лаунчера (применить настройки). Не app.relaunch(): Electron
// запускает новый экземпляр, пока старый ещё держит блокировку «один
// экземпляр», и новый тут же закрывается. Поэтому так: before-quit
// останавливает dsh, затем (relaunchRequested) старый отпускает блокировку
// и запускает новый экземпляр через sh, который ждёт, пока старый выйдет
// совсем (не дольше 20 с): иначе экземпляры перекрываются — новый не может
// занять то, что ещё держит старый (порт отладки, файлы Chromium).


function relaunchLauncher(why = 'чтобы применить настройки') {
  if (state.relaunchRequested || state.stopping) return;
  console.log(`[launcher] перезапуск лаунчера, ${why}`);
  state.relaunchRequested = true;
  app.quit();
}

// Новый экземпляр лаунчера — отдельно от этого, через bash: закрыть
// унаследованные дескрипторы (CLOSE_INHERITED_FDS), дождаться, пока этот
// процесс выйдет (до 20 с), и exec — тот же pid.
// $0 — pid этого процесса, "$@" — команда нового экземпляра.
function spawnLauncherAfterExit(cmd, args, stdio) {
  const script = dshProcess.CLOSE_INHERITED_FDS +
    'i=0; while kill -0 "$0" 2>/dev/null && [ $i -lt 200 ]; do sleep 0.1; i=$((i+1)); done; exec "$@"';
  return spawn('/bin/bash', ['-c', script, String(process.pid), cmd, ...args], { detached: true, stdio, env: process.env });
}

function spawnNewInstance() {
  app.releaseSingleInstanceLock();
  // песочница — по настройке (как в run.sh): только с галочкой и настроенным SUID-хелпером
  const sb = path.join(path.dirname(process.execPath), 'chrome-sandbox');
  let helperOk = false;
  try { const st = fs.statSync(sb); helperOk = st.uid === 0 && (st.mode & 0o4000) !== 0; } catch { /* нет хелпера */ }
  const args = process.argv.slice(1).filter((a) => a !== '--no-sandbox');
  if (!(helperOk && effectiveConfig(process.env, loadSettings(), os.homedir()).sandbox)) args.push('--no-sandbox');
  // stdout/stderr — те же: вывод нового экземпляра идёт туда же, что и прежнего
  const p = spawnLauncherAfterExit(process.execPath, args, ['ignore', 'inherit', 'inherit']);
  p.unref();
  console.log(`[launcher] новый экземпляр (pid ${p.pid}) запустится, когда этот завершится`);
}

// Раздел окна «Настройки» (меню слева) — запоминается, пока лаунчер открыт.
const SETTINGS_SECTIONS = ['general', 'updates', 'dsh', 'window', 'graphics'];


function showSettingsDialog() {
  const title = tr('Настройки', 'Settings');
  const gen = dialogs.openDialog(title);
  if (gen === null) return;
  settingsNote = null;
  state.dialogCopyText = '';
  const render = () => {
    const next = effectiveConfig(process.env, loadSettings(), os.homedir());
    const pending = restartNeeded(CONFIG, next);
    const envNote = (key, name) => (next.src[key] === 'env'
      ? `<span class="hint">${tr('задано переменной', 'set by')} <code>${name}</code></span>` : '');
    const locked = (key) => (next.src[key] === 'env' ? ' disabled' : '');
    const sel = (key, value, options) => `<select id="${key}" onchange="set('${key}', this.value)"${locked(key)}>${
      options.map(([v, label]) => `<option value="${v}"${v === value ? ' selected' : ''}>${esc(label)}</option>`).join('')}</select>`;
    const check = (key, on, label, disabled = false) =>
      `<label class="chk"><input type="checkbox" id="${key}"${on ? ' checked' : ''}${disabled ? ' disabled' : ''}
        onchange="set('${key}', this.checked)"> ${esc(label)}</label>`;
    const dismissed = updateDismissals();
    const gpu = fileHelp.gpuStatusNow();
    const dshShown = next.dshBin || tr(`автоматически (сейчас: ${state.dshBin})`, `automatic (now: ${state.dshBin})`);
    const body = `
      ${pending.length ? `<div class="banner">${tr('Изменения вступят в силу после перезапуска лаунчера.', 'Changes take effect after the launcher restarts.')}
        <button onclick="go('relaunch')">${tr('Перезапустить лаунчер', 'Restart the launcher')}</button></div>` : ''}
      ${settingsNote ? `<p class="${settingsNote.error ? 'pop err' : 'pop'}" role="status">${esc(settingsNote.text)}</p>` : ''}
      <div class="sett">
        <nav>
          <a data-s="general" class="${state.settingsSection === 'general' ? 'on' : ''}" onclick="sec('general')">${tr('Общие', 'General')}</a>
          <a data-s="updates" class="${state.settingsSection === 'updates' ? 'on' : ''}" onclick="sec('updates')">${tr('Обновления', 'Updates')}</a>
          <a data-s="dsh" class="${state.settingsSection === 'dsh' ? 'on' : ''}" onclick="sec('dsh')">${tr('Запуск dsh', 'Starting dsh')}</a>
          <a data-s="window" class="${state.settingsSection === 'window' ? 'on' : ''}" onclick="sec('window')">${tr('Окно', 'Window')}</a>
          <a data-s="graphics" class="${state.settingsSection === 'graphics' ? 'on' : ''}" onclick="sec('graphics')">${tr('Графика', 'Graphics')}</a>
        </nav>
        <div class="panes">
          <section id="sec-general" class="form"${state.settingsSection === 'general' ? '' : ' hidden'}>
        <div class="row"><span>${tr('Язык интерфейса', 'Interface language')}</span>
          ${LANG_FORCED ? `<code>${i18n.getLang()}</code> <span class="hint">${tr('задано переменной', 'set by')} <code>DSH_LAUNCHER_LANG</code></span>`
            : sel('lang', langSetting(), [['auto', tr('Авто (как в системе)', 'Auto (system)')], ['ru', 'Русский'], ['en', 'English']])}</div>
        <div class="row">${check('spellcheck', state.spellcheckOn, tr('Проверка орфографии в полях ввода', 'Spell check in text fields'))}</div>
        <div class="row">${check('inputHistory', inputHistoryOn(), tr('Поднимать историю сообщений клавишами ↑/↓ (как в терминале)', 'Recall sent messages with ↑/↓ (like a terminal)'))}</div>
          </section>
          <section id="sec-updates" class="form"${state.settingsSection === 'updates' ? '' : ' hidden'}>
        <div class="row">${check('checkUpdates', next.checkUpdates, tr('Проверять обновления при запуске (со следующего запуска)', 'Check for updates at startup (from the next start)'), next.src.checkUpdates === 'env')}
          ${envNote('checkUpdates', 'DSH_LAUNCHER_NO_UPDATE_CHECK')}</div>
        <div class="row">${check('pollUpdates', next.pollUpdates, tr('Проверять обновления раз в 12 часов, пока лаунчер открыт', 'Check for updates every 12 hours while the launcher is open'), next.src.pollUpdates === 'env')}
          ${envNote('pollUpdates', 'DSH_LAUNCHER_NO_UPDATE_CHECK')}</div>
        <div class="row"><span>${dismissed.length ? tr('Не спрашивать: ', 'Don’t ask: ') + esc(dismissed.join(', ')) : tr('Отложенных обновлений нет', 'No postponed updates')}</span>
          ${dismissed.length ? `<button onclick="go('reset-updates')">${tr('Снова спрашивать', 'Ask again')}</button>` : ''}</div>
          </section>
          <section id="sec-dsh" class="form"${state.settingsSection === 'dsh' ? '' : ' hidden'}>
        <div class="row"><span>${tr('Программа dsh', 'dsh program')}</span> <code class="val">${esc(dshShown)}</code>
          ${next.src.dshPath === 'env' ? envNote('dshPath', 'DSH_BIN')
            : `<button onclick="go('pick-dsh')">${tr('Выбрать…', 'Choose…')}</button>${next.dshBin ? ` <button onclick="set('dshPath', '')">${tr('Авто', 'Auto')}</button>` : ''}`}</div>
        <div class="row"><span>${tr('Профиль', 'Profile')}</span>
          <input id="profile" value="${esc(next.src.profile === 'env' ? (profileFromArgs(next.args) || '') : next.profile)}" onchange="set('profile', this.value)"${locked('profile')}>
          ${envNote('profile', 'DSH_ARGS')}</div>
        <div class="row"><span>${tr('Рабочая папка', 'Working folder')}</span> <code class="val">${esc(next.cwd)}</code>
          ${next.src.cwd === 'env' ? envNote('cwd', 'DSH_CWD')
            : `<button onclick="go('pick-cwd')">${tr('Выбрать…', 'Choose…')}</button>${next.src.cwd === 'settings' ? ` <button onclick="set('cwd', '')">${tr('Домашняя', 'Home')}</button>` : ''}`}</div>
        <div class="row"><span>${tr('Порт', 'Port')}</span>
          <input id="port" type="number" min="1024" max="65535" value="${next.port}" onchange="set('port', this.value)"${locked('port')}>
          ${envNote('port', 'DSH_PORT')}</div>
          </section>
          <section id="sec-window" class="form"${state.settingsSection === 'window' ? '' : ' hidden'}>
        <div class="row">${check('closeToTray', tray.closeToTrayOn(), tr('Крестик прячет окно в трей — dsh работает дальше', 'The close button hides the window to the tray — dsh keeps running'))}</div>
        <p class="hint">${tr('Нужен трей на рабочем столе (в GNOME — расширение AppIndicator, в Ubuntu оно есть). Спрятанное окно вернёт и повторный запуск лаунчера.',
                             'Needs a system tray on the desktop (in GNOME — the AppIndicator extension, included in Ubuntu). Starting the launcher again also brings the hidden window back.')}</p>
        <div class="row">${check('attention', tray.attentionOn(), tr('Сообщать, когда агент закончил ответ или ждёт вас (подтверждение команды, вопрос, план), а окно не в фокусе — уведомление, точка на значке трея', 'Tell me when the agent finishes or is waiting for you (a command to approve, a question, a plan) and the window is not focused — a notification, a dot on the tray icon'))}</div>
        <div class="row">${check('attentionSound', tray.attentionSoundOn(), tr('…и звуком', '…and with a sound'))}</div>
          </section>
          <section id="sec-graphics" class="form"${state.settingsSection === 'graphics' ? '' : ' hidden'}>
        <div class="row"><span>${tr('GPU-ускорение', 'GPU acceleration')}</span>
          ${sel('gpu', next.noGpu ? 'off' : 'auto', [['auto', tr('Авто', 'Auto')], ['off', tr('Выключено', 'Off')]])}
          ${envNote('gpu', 'DSH_LAUNCHER_NO_GPU')}</div>
        <div class="row"><span class="hint">${tr('Сейчас:', 'Now:')} ${esc(fileHelp.gpuStatusText(gpu))}</span>
          ${gpu.marker ? `<button onclick="location.href='dshlauncher://dialog/gpu-enable/'">${tr('Включить GPU снова', 'Re-enable GPU')}</button>` : ''}</div>
        <div class="row"><span>${tr('Окно на Wayland', 'Window on Wayland')}</span>
          ${sel('ozone', next.ozone, [['auto', tr('Авто', 'Auto')], ['wayland', 'Wayland'], ['x11', tr('X11 (XWayland) — если окна не видно', 'X11 (XWayland) — if the window is invisible')]])}
          ${envNote('ozone', 'DSH_LAUNCHER_OZONE')}</div>
        <div class="row">${check('sandbox', next.sandbox, tr('Песочница Chromium (со следующего запуска)', 'Chromium sandbox (from the next start)'))}</div>
        <p class="hint">${tr('Дополнительная изоляция окна. На части систем с ней лаунчер падает при перезапуске dsh, поэтому по умолчанию выключена; окно показывает только локальный GUI dsh, внешние ссылки открываются в браузере.',
                             'Extra isolation of the window. On some systems the launcher crashes with it when dsh restarts, so it is off by default; the window only shows the local dsh GUI, external links open in the browser.')}</p>
          </section>
        </div>
      </div>
      <div class="actions">${dialogs.dialogCloseButton()}</div>
      <script>
        function set(k, v) { location.href = 'dshlauncher://settings/set/?k=' + encodeURIComponent(k) + '&v=' + encodeURIComponent(v); }
        function go(r) { location.href = 'dshlauncher://settings/' + r + '/'; }
        // раздел — сразу на странице; лаунчер запоминает его для перерисовок
        function sec(id) {
          for (const el of document.querySelectorAll('.sett section')) el.hidden = el.id !== 'sec-' + id;
          for (const a of document.querySelectorAll('.sett nav a')) a.className = a.dataset.s === id ? 'on' : '';
          location.href = 'dshlauncher://settings/section/?s=' + id;
        }
      </script>`;
    dialogs.dialogPage(title, body);
  };
  dialogs.dialogShow(gen, render);
}

Object.assign(module.exports, { onSettingsSet, onSettingsPick, onSettingsResetUpdates, relaunchLauncher, spawnLauncherAfterExit, spawnNewInstance, SETTINGS_SECTIONS, showSettingsDialog }); // дополняем, а не заменяем: модули подключают друг друга по кругу
