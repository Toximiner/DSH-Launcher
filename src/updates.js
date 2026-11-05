'use strict';
// Updates on launch: check, ask, download and install of the launcher and dsh.
const { app, clipboard } = require('electron');
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc, timedCheck } = require('./util');
const { envWithBinDir } = require('./system');
const { runWithLiveLog, LOG_MAX_LINES, REDRAW_MS } = require('./proc-log');
const { prefFile, readPref, writePref } = require('./prefs');
const { compareDebianVer, compareSemVer, tagToPackageVersion, dpkgStatusVersion, notesBetween } = require('./versions');
const { releaseNotesLang, mdToHtml, dshNotesPart } = require('./notes');
const { GITHUB_TOKEN, UPDATE_NOW, INSTALL_VERSION, CONFIG } = require('./env');
const { probeDsh, findBin, MIN_NODE_MAJOR, npmInstallInfo, pkexecNpmArgs } = require('./dsh-install');
const { httpGet } = require('./updates-http');
const { DSH_NPM_PKG, UPDATE_GITHUB_REPO, fetchLauncherLatest, fetchDshLatest } = require('./updates-github');
const pages = require('./pages');
const settingsWindow = require('./settings-window');
const { blankResponse } = require('./respond'); // pure module — direct require, no registry

const MB = 1024 * 1024;
const DOWNLOAD_PROGRESS_MS = REDRAW_MS; // the download progress tick — like the install tick
const RESTART_AFTER_UPDATE_MS = 1500; // «Обновление установлено» → auto-restart
const RESTART_SPAWN_GRACE_MS = 200; // wait for the spawn 'error' event before closing the old instance

/* ---------------- update check (launcher and dsh) ----------------
 * On start (in onReady, in parallel with everything else) we check:
 *  - the latest dsh-launcher release (GitHub Releases Toximiner/DSH-Launcher);
 *  - the latest version of the @deepseek-ai/dsh npm package.
 * The requests go out immediately at startup, in parallel with the rest;
 * before starting dsh the launcher waits for their answer at most ~8 s
 * (with no network the request fails at once). A network error or a
 * timeout — just no question (a line in the log).
 *
 * On the self-launch path (before starting dsh) with a new version
 * available the window asks (the same pattern as the market question):
 *  - launcher: downloads the .deb from the release into
 *    userData/updates/ (the sha256 is computed on the fly and compared
 *    with the GitHub API asset digest) and installs it as root via pkexec
 *    — polkit will ask for the password in a system dialog. Under root the
 *    package is first copied to its own directory and checked against the
 *    same sha256 — a file swap in userData between the download and the
 *    install won't pass. Success → the «Обновление установлено» page and
 *    an auto-restart after ~1.5 s: the old instance releases the
 *    single-instance lock, starts the new one and closes. No pkexec / the
 *    password entry was cancelled / an error → a page with a command for
 *    the terminal (`sudo apt install -y <deb>`) and «Проверить ещё раз»;
 *  - dsh: if the global npm prefix is available without sudo —
 *    `npm install -g @deepseek-ai/dsh@<version>` with a live log; if the
 *    prefix belongs to root and npm is system — the same command via
 *    pkexec (polkit asks for the password in a system dialog, as with a
 *    launcher update); otherwise (npm from nvm, ~/.npm-global) — a page
 *    with a command for the terminal and «Проверить ещё раз». After
 *    success dsh starts immediately with the new version.
 *
 * In attach mode (dsh already runs from outside) we don't ask: someone
 * else's dsh can't be restarted, and the launcher's own update will be
 * offered on the next self-launch. «Не спрашивать больше» is remembered
 * per version (userData/update-prefs.json) — a new release asks again.
 *
 * Environment variables:
 *  DSH_LAUNCHER_NO_UPDATE_CHECK=1 — checks disabled;
 *  DSH_LAUNCHER_FAKE_LAUNCHER_LATEST / DSH_LAUNCHER_FAKE_DSH_LATEST —
 *    a «future» version for development and tests (the launcher .deb still
 *    comes from the real latest release);
 *  DSH_LAUNCHER_FAKE_INSTALLED — the «installed» launcher version (tests);
 *  DSH_LAUNCHER_GITHUB_API=http://127.0.0.1:<port> — a stand-in GitHub API
 *    (Docker tests, test/docker/fakes/github.py): the release list and .deb.
 */
const UPDATE_CHECK_TIMEOUT = 8000; // how long to wait for the check before not asking
const NO_UPDATE_CHECK = !CONFIG.checkUpdates; // DSH_LAUNCHER_NO_UPDATE_CHECK or «Настройки»

// The launcher package version — from the dpkg database, without running
// dpkg-query: spawning a child process while the dialog window loads
// crashed Electron 44 («О программе»: SIGSEGV / «terminate called
// without an active exception»). null — not installed via .deb (run from
// sources) → we don't check.
async function launcherInstalledVersion() {
  const fake = process.env.DSH_LAUNCHER_FAKE_INSTALLED;
  if (fake) return String(fake);
  try {
    return dpkgStatusVersion(await fs.promises.readFile('/var/lib/dpkg/status', 'utf8'), 'dsh-launcher');
  } catch {
    return null;
  }
}

// The «Что нового» block — versions from newest to oldest; empty — ''.
function notesBlockHtml(items) {
  const parts = (items || [])
    .map((r) => ({ v: r.version, html: mdToHtml(releaseNotesLang(r.body, i18n.getLang())) }))
    .filter((r) => r.html);
  if (!parts.length) return '';
  return `<p style="margin:14px 0 6px; color:#d7dce3">${tr('Что нового:', 'What’s new:')}</p>
    <div class="notes">${parts.map((r) => `<div class="ver"><code>${esc(r.v)}</code></div>${r.html}`).join('')}</div>`;
}

// On start — both requests in parallel (we don't wait for them here).
function startUpdateChecks() {
  if (GITHUB_TOKEN) console.log('[launcher] запросы к GitHub API — с токеном (DSH_LAUNCHER_GITHUB_TOKEN)');
  if (UPDATE_NOW) console.log(`[launcher] обновление ${UPDATE_NOW} — по просьбе пользователя, без вопроса`);
  if (NO_UPDATE_CHECK && !UPDATE_NOW) {
    console.log('[launcher] проверка обновлений отключена (DSH_LAUNCHER_NO_UPDATE_CHECK)');
    return;
  }
  state.updateChecks = {
    launcher: timedCheck(fetchLauncherLatest(), UPDATE_CHECK_TIMEOUT).then((v) => {
      if (v) console.log(`[launcher] последний dsh-launcher: ${v.version}`);
      return v;
    }),
    dsh: timedCheck(fetchDshLatest(), UPDATE_CHECK_TIMEOUT).then((v) => {
      if (v) console.log(`[launcher] последний ${DSH_NPM_PKG}: ${v.version}`);
      return v;
    }),
  };
}

let updateState = {
  kind: null,          // 'launcher' | 'dsh' — which question is on the page now
  mode: 'ask',         // 'ask' | 'running' | 'manual' | 'success'
  resolve: null,       // the resolver of the current question
  running: false,      // a process is running (download / install)
  log: [], timer: null, proc: null, cmdLine: null,
  latest: null,        // the new version (the question text)
  installed: null,     // the current version
  htmlUrl: null,       // the release page (for dsh — the npm package page)
  notes: [],           // «Что нового»: releases between the installed and the new version (launcher)
  debPath: null,       // the downloaded .deb
  manualCmd: null,     // the command for the terminal (manual mode)
  restartTimer: null,
};

// Launcher shutdown: the update-step process, the redraw timer and the
// auto-restart timer (before-quit → state.settleAllOperations, main.js).
function settleUpdateState() {
  if (updateState.proc) { try { updateState.proc.kill('SIGTERM'); } catch { /* noop */ } }
  clearInterval(updateState.timer);
  if (updateState.restartTimer) {
    clearTimeout(updateState.restartTimer);
    updateState.restartTimer = null;
  }
}
state.settleOperation(settleUpdateState);

const UPDATE_PREFS = 'update-prefs.json';
const updatePrefFile = () => prefFile(UPDATE_PREFS);

function updatePrefs() {
  return readPref(UPDATE_PREFS) || {};
}

// «Не спрашивать больше» is per version: dismissing the old one doesn't hide the new one.
function updateDismissed(kind, version) {
  return updatePrefs()[kind] === version;
}

function updateDismiss(kind, version) {
  const p = updatePrefs();
  p[kind] = version;
  if (writePref(UPDATE_PREFS, p)) console.log(`[launcher] ${kind}: «не спрашивать больше» до версии ${version}`);
}

function showUpdatePage(note, noteIsError = false) {
  if (!state.win || state.win.isDestroyed()) return;
  state.currentPage = () => showUpdatePage(note, noteIsError);
  if (typeof note === 'function') note = note();
  const kind = updateState.kind;
  const mode = updateState.mode;
  const noteHtml = note
    ? `<p style="color:${noteIsError ? '#f85149' : '#3fb950'}">${esc(note)}</p>`
    : '';
  const link = updateState.htmlUrl
    ? `<a href="${esc(updateState.htmlUrl)}" target="_blank">${kind === 'dsh'
        ? tr('Страница пакета (npm)', 'Package page (npm)')
        : tr('Страница релиза (GitHub)', 'Release page (GitHub)')}</a>`
    : '';
  let title;
  let body;
  if (mode === 'running') {
    title = kind === 'dsh'
      ? tr('Обновление DeepSeek Harness…', 'Updating DeepSeek Harness…')
      : updateState.rollback
        ? tr(`Установка версии ${updateState.latest}…`, `Installing version ${updateState.latest}…`)
        : tr('Обновление DeepSeek Harness Launcher…', 'Updating DeepSeek Harness Launcher…');
    body = `
      ${noteHtml}
      <p>${tr('Выполняю', 'Running')} <code>${esc(updateState.cmdLine || '')}</code></p>
      <p>${tr('Последние строки:', 'Latest output lines:')}</p>
      ${pages.liveLogPre(updateState.log)}`;
  } else if (mode === 'success') {
    title = tr('Обновление установлено', 'Update installed');
    body = `
      ${noteHtml}
      <p>${tr(
        `Установлена версия <code>${esc(updateState.latest)}</code>. Приложение перезапустится через пару секунд (или нажмите кнопку).`,
        `Version <code>${esc(updateState.latest)}</code> is installed. The app will restart in a couple of seconds (or click the button).`)}</p>
      <div style="margin-top:12px; display:flex; gap:10px; flex-wrap:wrap">
        <button onclick="location.href='dshlauncher://update/restart/'">${tr('Перезапустить сейчас', 'Restart now')}</button>
      </div>`;
  } else if (mode === 'manual') {
    title = tr('Обновить вручную', 'Update manually');
    body = `
      ${noteHtml}
      ${updateState.manualCmd
        ? `<p>${tr('Лаунчер не смог обновиться сам. Выполните в терминале:', 'The launcher cannot update itself. Run in a terminal:')}</p>
           <pre>${esc(updateState.manualCmd)}</pre>`
        : ''}
      ${kind === 'launcher' && updateState.debPath
        ? `<p>${tr('Скачанный пакет:', 'Downloaded package:')} <code>${esc(updateState.debPath)}</code></p>`
        : ''}
      ${link ? `<p>${link}</p>` : ''}
      <div style="margin-top:12px; display:flex; gap:10px; flex-wrap:wrap">
        ${updateState.manualCmd ? `<button onclick="location.href='dshlauncher://update/copy/'">${tr('Скопировать команду', 'Copy command')}</button>` : ''}
        <button onclick="location.href='dshlauncher://update/recheck/'">${tr('Проверить ещё раз', 'Check again')}</button>
        <button onclick="location.href='dshlauncher://update/later/'">${tr('Не сейчас', 'Not now')}</button>
        <button onclick="location.href='dshlauncher://update/never/'">${tr('Не спрашивать больше', 'Don’t ask again')}</button>
      </div>`;
  } else {
    // ask
    title = kind === 'dsh'
      ? tr('Обновить DeepSeek Harness?', 'Update DeepSeek Harness?')
      : tr('Обновить DeepSeek Harness Launcher?', 'Update DeepSeek Harness Launcher?');
    body = `
      ${noteHtml}
      <p>${tr(
        `Доступна версия <code>${esc(updateState.latest)}</code> (установлена: <code>${esc(updateState.installed)}</code>). Обновиться?`,
        `Version <code>${esc(updateState.latest)}</code> is available (installed: <code>${esc(updateState.installed)}</code>). Update?`)}</p>
      ${notesBlockHtml(updateState.notes)}
      <div style="margin-top:12px; display:flex; gap:10px; flex-wrap:wrap">
        <button onclick="location.href='dshlauncher://update/install/'">${tr('Обновить', 'Update')}</button>
        <button onclick="location.href='dshlauncher://update/later/'">${tr('Не сейчас', 'Not now')}</button>
        <button onclick="location.href='dshlauncher://update/never/'">${tr('Не спрашивать больше', 'Don’t ask again')}</button>
        ${link}
      </div>`;
  }
  if (mode === 'running') pages.liveLogSync(updateState);
  pages.showStatus(title, body, noteIsError);
}

// Shows the question (ask/manual mode) and waits for the choice:
// 'install' | 'later' | 'never' | 'recheck'. 'copy' and 'restart' don't
// resolve the question (they redraw the page / close the app).
function askUpdate(note, noteIsError) {
  return new Promise((resolve) => {
    updateState.resolve = resolve;
    showUpdatePage(note, noteIsError);
  });
}

function onUpdateChoice(choice) {
  if (choice === 'copy') {
    if (updateState.manualCmd) {
      clipboard.writeText(updateState.manualCmd);
      showUpdatePage(tr('Команда скопирована в буфер обмена.', 'Command copied to the clipboard.'), false);
    }
    return blankResponse();
  }
  if (choice === 'restart') {
    restartLauncher();
    return blankResponse();
  }
  const resolve = updateState.resolve;
  if (resolve && !updateState.running) {
    updateState.resolve = null;
    resolve(choice);
  }
  return blankResponse();
}

// An install step with a live log on the page (the live log —
// pages.liveLogTick: lines are appended without reloading the page, the
// scroll stays with the user).
function runUpdateStep(cmd, args, env) {
  updateState.mode = 'running';
  updateState.cmdLine = [cmd, ...args].join(' ');
  console.log(`[launcher] шаг обновления: ${updateState.cmdLine}`);
  showUpdatePage();
  return runWithLiveLog(updateState, cmd, args, {
    env,
    // mode === 'running' — otherwise another page is already shown
    // (error/success), and an extra redraw would overwrite its note (the
    // tick may have resolved later).
    redraw: pages.liveRedraw(updateState, () => updateState.mode === 'running', showUpdatePage),
  });
}

// Downloading the .deb from the release into userData/updates/. The
// progress — in the log on the page (the same ticks as install). The
// sha256 is computed over the bytes from the network (not over the file
// on disk) and, if GitHub knows it, is compared with the digest.
async function downloadDeb(url, expectedSha256) {
  updateState.running = true;
  updateState.mode = 'running';
  updateState.log = [];
  updateState.logSeq = 0;
  updateState.cmdLine = `download ${url}`;
  console.log(`[launcher] скачиваю ${url}`);
  showUpdatePage();
  const dir = path.join(app.getPath('userData'), 'updates');
  try {
    fs.mkdirSync(dir, { recursive: true });
    // Packages from earlier attempts are no longer needed — we don't accumulate them.
    for (const f of fs.readdirSync(dir)) {
      if (f.endsWith('.deb')) try { fs.unlinkSync(path.join(dir, f)); } catch { /* noop */ }
    }
    const res = await httpGet(url, { headers: { 'User-Agent': 'dsh-launcher' } }); // not electronNet — see proxyUrlFromRule
    if (res.statusCode !== 200) { res.resume(); throw new Error(`download: HTTP ${res.statusCode}`); }
    const total = Number(res.headers['content-length'] || 0);
    const name = url.split('/').pop() || 'dsh-launcher.deb';
    const dest = path.join(dir, name);
    updateState.log.push(total ? `${name} (${(total / MB).toFixed(1)} МБ)` : name);
    updateState.logSeq++;
    pages.liveRedraw(updateState, () => updateState.mode === 'running', showUpdatePage)();
    const out = fs.createWriteStream(dest);
    const hash = crypto.createHash('sha256');
    let got = 0;
    updateState.timer = setInterval(() => {
      updateState.log.push(total
        ? `${(got / MB).toFixed(1)} / ${(total / MB).toFixed(1)} МБ (${Math.round((100 * got) / total)}%)`
        : `${(got / MB).toFixed(1)} МБ`);
      if (updateState.log.length > LOG_MAX_LINES) updateState.log.shift();
      updateState.logSeq++;
      pages.liveRedraw(updateState, () => updateState.mode === 'running', showUpdatePage)();
    }, DOWNLOAD_PROGRESS_MS);
    try {
      await new Promise((resolve, reject) => {
        const body = res;
        body.on('data', (c) => { got += c.length; hash.update(c); });
        body.on('error', reject);
        out.on('error', reject);
        out.on('finish', resolve);
        body.pipe(out);
      });
    } catch (e) {
      try { out.destroy(); } catch { /* noop */ }
      try { fs.unlinkSync(dest); } catch { /* noop */ }
      throw e;
    } finally {
      clearInterval(updateState.timer);
    }
    const sha256 = hash.digest('hex');
    if (expectedSha256 && sha256 !== expectedSha256) {
      try { fs.unlinkSync(dest); } catch { /* noop */ }
      throw new Error(`sha256 не совпадает с релизом (${sha256.slice(0, 12)}… ≠ ${expectedSha256.slice(0, 12)}…)`);
    }
    updateState.debPath = dest;
    console.log(`[launcher] скачал ${name} → ${dest} (sha256 ${sha256}${expectedSha256 ? ', совпадает с релизом' : ', digest в релизе нет'})`);
    return { ok: true, path: dest, sha256 };
  } catch (e) {
    console.error('[launcher] скачивание не удалось:', e.message);
    return { ok: false, err: e.message };
  } finally {
    updateState.running = false;
  }
}

// The restart after a launcher update: we release the single-instance
// lock so the new instance takes it at once, spawn the new one detached
// (it will survive our exit) and close gracefully — via before-quit.
function restartLauncher() {
  if (state.restarting || state.stopping) return;
  state.restarting = true;
  const exe = process.env.DSH_LAUNCHER_RESTART_BIN || '/usr/bin/dsh-launcher';
  app.releaseSingleInstanceLock();
  let started = false;
  const fail = (e) => {
    state.restarting = false;
    state.launcherUpdatedOnDisk = false; // don't restart again on every process crash
    app.requestSingleInstanceLock(); // we stay the only instance
    if (state.stopping) return;
    console.error('[launcher] авто-перезапуск не удался:', e.message);
    showUpdatePage(
      tr(`Не удалось запустить новую версию: ${e.message}. Обновление уже установлено — перезапустите лаунчер вручную.`,
         `Failed to start the new version: ${e.message}. The update is already installed — restart the launcher manually.`),
      true
    );
  };
  // the spawn — through bash (spawnLauncherAfterExit): if the program is
  // missing we learn it here, not via 'error' (bash is there)
  try { fs.accessSync(exe, fs.constants.X_OK); } catch (e) { fail(e); return; }
  const p = settingsWindow.spawnLauncherAfterExit(exe, [], 'ignore');
  p.on('spawn', () => { started = true; });
  p.on('error', fail);
  p.unref();
  // We give the 'error' event a chance (missing exe — it fires at once):
  // if the process didn't start, we don't close the app — the error is shown.
  setTimeout(() => {
    if (!started) return;
    console.log(`[launcher] запущена новая версия (${exe}), закрываю старую`);
    app.quit();
  }, RESTART_SPAWN_GRACE_MS);
}

// When and to where the launcher was updated (last-update.json): the next
// start, if the updated version crashes, uses this to understand "the
// failure is related to the update" and offers a rollback
// (diagnostics.rollbackSuggested).
const readLastUpdate = () => {
  const u = readPref('last-update.json');
  return u && typeof u === 'object' ? u : null;
};
const writeLastUpdate = () =>
  writePref('last-update.json', { from: updateState.installed, to: tagToPackageVersion(updateState.latest), at: new Date().toISOString() });

// The launcher is updated on disk but the old one is still running: apt
// replaced the /opt/dsh-launcher files under us. The already-open files
// live on (old inodes), but new Chromium processes (GPU, utilities) would
// start from the new binary — so we restart quickly, and on any process
// crash — at once.
function showUpdateSuccess() {
  state.launcherUpdatedOnDisk = true;
  writeLastUpdate();
  if (updateState.debPath) {
    try { fs.unlinkSync(updateState.debPath); } catch { /* noop */ }
    updateState.debPath = null;
  }
  updateState.mode = 'success';
  showUpdatePage();
  // ~1.5 s so the page has time to appear; if the window is closed
  // earlier — before-quit clears the timer.
  updateState.restartTimer = setTimeout(() => restartLauncher(), RESTART_AFTER_UPDATE_MS);
}

// Installing the .deb as root (via pkexec). pkexec clears the
// environment, so DEBIAN_FRONTEND is set inside. The package is copied
// into root's directory and checked against the sha256 computed at
// download time: the file in userData is writable by the user, and without
// the copy it could be swapped between the check and the install.
// $1 — the .deb path, $2 — the sha256.
const ROOT_INSTALL_SH = [
  'set -e',
  'd=$(mktemp -d)',
  'trap \'rm -rf "$d"\' EXIT',
  'cp -- "$1" "$d/update.deb"',
  'echo "$2  $d/update.deb" | sha256sum -c --quiet -',
  // --allow-downgrades: «Вернуть предыдущую версию…» installs a version older than the installed one
  'DEBIAN_FRONTEND=noninteractive apt-get install -y --allow-downgrades "$d/update.deb"',
].join('; ');

// The common beginning of the update page — the same set of fields for both.
function beginUpdate({ kind, version, installed, notes, htmlUrl, logLine }) {
  console.log(logLine);
  updateState.kind = kind;
  updateState.latest = version;
  updateState.installed = installed;
  updateState.htmlUrl = htmlUrl;
  updateState.notes = notes;
  updateState.mode = 'ask';
  updateState.debPath = null;
  updateState.manualCmd = null;
}

// "Install in the terminal": the command to copy and an error note.
const manual = (manualCmd, note) => {
  updateState.mode = 'manual';
  if (manualCmd) updateState.manualCmd = manualCmd;
  return { note, noteIsError: true };
};
// How the choice handler («recheck»/«install») is told "stop, with a result".
const done = (value) => ({ return: value });

// The update loop (common for the launcher and dsh): the question;
// "never" — don't ask about this version, "later" — exit; "check again" /
// "install" — installStep(choice). { note, noteIsError } — ask again with
// a note; done(...) — exit (the value is the function's result: true —
// the launcher is updated and will restart now, the new instance will
// raise dsh).
async function offerUpdateFlow({ kind, version, laterLog, installStep }) {
  let note = null;
  let noteIsError = false;
  let auto = UPDATE_NOW === kind; // «Обновить сейчас» — the first time without a question
  for (;;) {
    const choice = auto ? 'install' : await askUpdate(note, noteIsError);
    auto = false;
    if (state.stopping) return;
    if (choice === 'never') { updateDismiss(kind, version); return; }
    if (choice === 'later') { console.log(laterLog); return; }
    const r = await installStep(choice);
    if ('return' in r) return r.return;
    note = r.note;
    noteIsError = r.noteIsError;
  }
}

// true — the launcher is updated and will restart now: dsh is not started
// by this instance (the new one will raise it).
async function offerLauncherUpdate() {
  if (UPDATE_NOW === 'dsh') return; // asked to update dsh — about the launcher we'll ask another time
  const newest = await state.updateChecks.launcher;
  if (!newest || !newest.version) return; // no network / disabled / timeout
  const installed = await launcherInstalledVersion();
  if (!installed) return; // not installed via .deb (run from sources)
  // «Вернуть предыдущую версию…»: we install the chosen one (older than the installed one)
  const target = INSTALL_VERSION ? (newest.all || []).find((r) => r.version === INSTALL_VERSION) : null;
  if (INSTALL_VERSION && !target) { console.error(`[launcher] версии ${INSTALL_VERSION} нет среди релизов — откат отменён`); return; }
  const latest = target ? { ...newest, ...target } : newest;
  const latestPkg = tagToPackageVersion(latest.version);
  if (!target && compareDebianVer(latestPkg, installed) <= 0) return; // no new version
  updateState.rollback = Boolean(target);
  if (target) console.log(`[launcher] откат: ставлю ${target.version} (установлена ${installed})`);
  if (UPDATE_NOW !== 'launcher' && updateDismissed('launcher', latest.version)) {
    console.log(`[launcher] обновление до ${latest.version} отклонено — не спрашиваю`);
    return;
  }
  beginUpdate({
    kind: 'launcher',
    version: latest.version,
    installed,
    htmlUrl: latest.htmlUrl,
    notes: notesBetween(latest.releases, installed, latest.version, 'deb'),
    logLine: `[launcher] новый dsh-launcher: ${latest.version} (установлен ${installed})`,
  });
  return offerUpdateFlow({
    kind: 'launcher',
    version: latest.version,
    laterLog: '[launcher] обновление лаунчера отложено (не сейчас)',
    installStep: async (choice) => {
      if (choice === 'recheck') {
        // The user installed the .deb themselves, in the terminal.
        const now = await launcherInstalledVersion();
        if (now && compareDebianVer(latestPkg, now) <= 0) {
          updateState.installed = now;
          console.log(`[launcher] dsh-launcher обновлён до ${now} (вручную)`);
          showUpdateSuccess();
          return done(true);
        }
        return manual(null,
          tr(`Установлена версия ${now || 'неизвестно'}. Поставьте пакет и нажмите «Проверить ещё раз».`,
             `Installed version is ${now || 'unknown'}. Install the package and click "Check again".`));
      }
      // choice === 'install'
      if (!latest.assetUrl) {
        return manual(null,
          tr('В этом релизе нет файла .deb — скачайте его со страницы релиза.',
             'This release has no .deb file — download it from the release page.'));
      }
      const dl = await downloadDeb(latest.assetUrl, latest.assetSha256);
      if (state.stopping) return done(undefined);
      if (!dl.ok) {
        return manual(`curl -L -o dsh-launcher.deb ${latest.assetUrl} && sudo apt install -y ./dsh-launcher.deb`,
          tr(`Скачивание не удалось: ${dl.err}. Выполните команду выше в терминале и нажмите «Проверить ещё раз».`,
             `Download failed: ${dl.err}. Run the command above in a terminal and click "Check again".`));
      }
      const pkexec = findBin('pkexec');
      if (!pkexec) {
        return manual(`sudo apt install -y ${dl.path}`,
          tr('pkexec в системе не найден — поставьте скачанный пакет командой выше.',
             'pkexec was not found on the system — install the downloaded package with the command above.'));
      }
      const code = await runUpdateStep(pkexec, ['/bin/sh', '-c', ROOT_INSTALL_SH, 'sh', dl.path, dl.sha256]);
      if (state.stopping) return done(undefined);
      if (code === 0) {
        console.log(`[launcher] dsh-launcher ${latest.version} установлен`);
        // after a rollback don't offer at once to return to the newest
        if (target && newest.version !== target.version) updateDismiss('launcher', newest.version);
        showUpdateSuccess();
        return done(true);
      }
      // A polkit password entry refusal is also a non-zero code: the
      // terminal command refers to the already-downloaded package.
      console.error(`[launcher] pkexec apt-get завершился с кодом ${code} — даю команду вручную`);
      return manual(`sudo apt install -y ${dl.path}`,
        tr(`Установка не удалась (код ${code}). Если вы отменили ввод пароля — попробуйте ещё раз.`,
           `Installation failed (code ${code}). If you cancelled the password prompt — try again.`));
    },
  });
}

async function offerDshUpdate() {
  const latest = await state.updateChecks.dsh;
  if (!latest || !latest.version) return;
  if (!state.dshInstalledVersion) {
    const r = await probeDsh(state.dshBin);
    if (!r.ok) return;
    state.dshInstalledVersion = r.version;
  }
  if (compareSemVer(latest.version, state.dshInstalledVersion) <= 0) return;
  if (UPDATE_NOW !== 'dsh' && updateDismissed('dsh', latest.version)) {
    console.log(`[launcher] обновление dsh до ${latest.version} отклонено — не спрашиваю`);
    return;
  }
  beginUpdate({
    kind: 'dsh',
    version: latest.version,
    installed: state.dshInstalledVersion,
    htmlUrl: latest.htmlUrl,
    notes: (latest.releases || []).length
      ? notesBetween(latest.releases, state.dshInstalledVersion, latest.version, 'semver')
          .map((r) => ({ version: r.version, body: dshNotesPart(r.body) }))
      : [], // the dsh release descriptions — from GitHub (fetchDshLatest); none — we don't show the block
    logLine: `[launcher] новый ${DSH_NPM_PKG}: ${latest.version} (установлен ${state.dshInstalledVersion})`,
  });
  const cmdAuto = `npm install -g ${DSH_NPM_PKG}@${latest.version}`;
  // the prefix is available without sudo (canAuto) — sudo with nvm would
  // only hurt; a system prefix — the command with sudo (auto-install there
  // goes through pkexec).
  const manualCmd = (npmInfo) => (npmInfo && npmInfo.canAuto ? cmdAuto : `sudo ${cmdAuto}`);
  return offerUpdateFlow({
    kind: 'dsh',
    version: latest.version,
    laterLog: '[launcher] обновление dsh отложено (не сейчас)',
    installStep: async (choice) => {
      if (choice === 'recheck') {
        const r = await probeDsh(state.dshBin);
        if (r.ok && compareSemVer(r.version, latest.version) >= 0) {
          state.dshInstalledVersion = r.version;
          console.log(`[launcher] dsh обновлён до ${r.version} (в терминале)`);
          return done(undefined);
        }
        // The command — by the same rules as the install: a system prefix
        // needs sudo, a user one (nvm) — no.
        const info = await npmInstallInfo();
        return manual(info.hasNpm && !info.canAuto ? `sudo ${cmdAuto}` : cmdAuto,
          tr(`Всё ещё версия ${r.ok ? r.version : 'неизвестно'}. Поставьте новую и нажмите «Проверить ещё раз».`,
             `Still version ${r.ok ? r.version : 'unknown'}. Install the new one and click "Check again".`));
      }
      // choice === 'install'
      const npmInfo = await npmInstallInfo();
      if (state.stopping) return done(undefined);
      if (!npmInfo.hasNpm || (!npmInfo.canAuto && npmInfo.rootMode !== 'pkexec')) {
        // The global prefix is not available without sudo (or there's no
        // npm) — a command for the terminal. If the prefix is system
        // (root) but there's no pkexec — also here.
        let note;
        if (npmInfo.nodeOld) {
          note = tr(`Node.js ${npmInfo.nodeVersion} слишком стар для dsh (нужен ${MIN_NODE_MAJOR}+). Сначала обновите Node.js, затем поставьте dsh командой выше.`,
                    `Node.js ${npmInfo.nodeVersion} is too old for dsh (${MIN_NODE_MAJOR}+ required). Upgrade Node.js first, then install dsh with the command above.`);
        } else if (!npmInfo.hasNpm) {
          note = tr('npm не найден — сначала установите Node.js (22+) с npm.',
                    'npm not found — install Node.js (22+) with npm first.');
        } else {
          note = tr('Эта команда требует sudo — выполните её в терминале и нажмите «Проверить ещё раз».',
                    'This command needs sudo — run it in a terminal and click "Check again".');
        }
        return manual(npmInfo.hasNpm ? `sudo ${cmdAuto}` : cmdAuto, note);
      }
      const code = npmInfo.canAuto
        ? await runUpdateStep(npmInfo.npm, ['install', '-g', `${DSH_NPM_PKG}@${latest.version}`], envWithBinDir(npmInfo.npm))
        // A system prefix: npm under pkexec — polkit asks for the password
        // in a system dialog, as with a launcher update. We don't touch
        // our own user prefix (nvm, ~/.npm-global) that way — there root
        // would leave files the user can't update later (rootInstallMode).
        : await runUpdateStep(npmInfo.pkexec, pkexecNpmArgs(npmInfo.npm, ['install', '-g', `${DSH_NPM_PKG}@${latest.version}`]));
      if (state.stopping) return done(undefined);
      if (code === 0) {
        const r = await probeDsh(state.dshBin);
        if (r.ok) state.dshInstalledVersion = r.version;
        if (r.ok && compareSemVer(r.version, latest.version) >= 0) {
          console.log(`[launcher] ${DSH_NPM_PKG} обновлён → ${r.version}; dsh стартует с новой версией`);
          return done(undefined);
        }
        // npm worked, but the dsh being started is the old one: npm
        // installs into another prefix (e.g., dsh from nvm, and npm is
        // system).
        console.error(`[launcher] npm install прошёл, но ${state.dshBin} — ${r.ok ? r.version : 'не запускается'} (prefix npm: ${npmInfo.prefix})`);
        return manual(manualCmd(npmInfo),
          tr(`npm поставил пакет в ${npmInfo.prefix || 'свой prefix'}, но лаунчер запускает ${state.dshBin} (версия ${r.ok ? r.version : 'неизвестна'}). Обновите dsh тем npm, которым он был установлен, и нажмите «Проверить ещё раз».`,
             `npm installed the package into ${npmInfo.prefix || 'its prefix'}, but the launcher runs ${state.dshBin} (version ${r.ok ? r.version : 'unknown'}). Update dsh with the npm it was installed with and click "Check again".`));
      }
      console.error(`[launcher] обновление dsh не удалось (код ${code})`);
      return manual(manualCmd(npmInfo), code === 126 || code === 127
        ? tr('Ввод пароля отменён. Попробуйте ещё раз или выполните команду выше в терминале.',
             'The password prompt was cancelled. Try again or run the command above in a terminal.')
        : tr(`Установка не удалась (код ${code}). Повторите, либо выполните команду выше в терминале и нажмите «Проверить ещё раз».`,
             `Installation failed (code ${code}). Retry, or run the command above in a terminal and click "Check again".`));
    },
  });
}

Object.assign(module.exports, { DSH_NPM_PKG, UPDATE_GITHUB_REPO, UPDATE_CHECK_TIMEOUT, NO_UPDATE_CHECK, launcherInstalledVersion, fetchLauncherLatest, notesBlockHtml, fetchDshLatest, startUpdateChecks, updateState, updatePrefFile, updatePrefs, onUpdateChoice, restartLauncher, offerLauncherUpdate, offerDshUpdate, readLastUpdate }); // extend, don't replace: the modules require each other in a circle
