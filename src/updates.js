'use strict';
// Обновления при запуске: проверка, вопрос, скачивание и установка лаунчера и dsh.
const { app, session, clipboard } = require('electron');
const http = require('http');
const https = require('https');
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');
const state = require('./state');
const i18n = require('./i18n');
const { tr } = i18n;
const { esc, timedCheck } = require('./util');
const { envWithBinDir } = require('./system');
const { runWithLiveLog } = require('./proc-log');
const { prefFile, readPref, writePref } = require('./prefs');
const { compareDebianVer, compareSemVer, tagToPackageVersion, dpkgStatusVersion, notesBetween } = require('./versions');
const { releaseNotesLang, mdToHtml } = require('./notes');
const { releasesCacheMode, releasesCacheFresh, proxyUrlFromRule, parseRelease } = require('./updates-logic');
const { GITHUB_TOKEN, GITHUB_API, UPDATE_NOW, INSTALL_VERSION, CONFIG } = require('./env');
const { probeDsh, findBin, MIN_NODE_MAJOR, npmInstallInfo } = require('./dsh-install');
const pages = require('./pages');
const settingsWindow = require('./settings-window');
const launcher = require('./launcher'); // общий реестр (наполняет main.js)

/* ---------------- проверка обновлений (лаунчер и dsh) ----------------
 * При старте (в onReady, параллельно со всем остальным) проверяем:
 *  - последний релиз dsh-launcher (GitHub Releases Toximiner/DSH-Launcher);
 *  - последнюю версию npm-пакета @deepseek-ai/dsh.
 * Запросы уходят сразу при старте, параллельно остальному; перед запуском
 * dsh лаунчер ждёт их ответа не дольше ~8 с (без сети запрос падает сразу).
 * Сетевая ошибка или таймаут — просто без вопроса (в логе — строка).
 *
 * На пути само-запуска (перед стартом dsh) при наличии новой версии окно
 * спрашивает (тот же паттерн, что вопрос про маркет):
 *  - лаунчер: скачивает .deb из релиза в userData/updates/ (sha256 считается
 *    на лету и сверяется с digest ассета из GitHub API) и ставит его от root
 *    через pkexec — polkit спросит пароль системным диалогом. Под root пакет
 *    сначала копируется в свой каталог и сверяется с тем же sha256 — подмена
 *    файла в userData между скачиванием и установкой не пройдёт. Успех →
 *    страница «Обновление установлено» и авто-перезапуск через ~1.5 с: старый
 *    экземпляр отпускает single-instance лок, запускает новый и закрывается.
 *    Нет pkexec / отменили ввод пароля / ошибка → страница с командой для
 *    терминала (`sudo apt install -y <deb>`) и «Проверить ещё раз»;
 *  - dsh: если глобальный npm-prefix доступен без sudo —
 *    `npm install -g @deepseek-ai/dsh@<версия>` с живым логом; иначе —
 *    страница с командой для терминала и «Проверить ещё раз». После успеха
 *    dsh стартует сразу с новой версией.
 *
 * В режиме подключения (dsh уже работает из-вне) не спрашиваем: чужой dsh
 * перезапускать нельзя, а обновление самого лаунчера предложим при
 * следующем само-запуске. «Не спрашивать больше» запоминается на версию
 * (userData/update-prefs.json) — новый релиз спросит снова.
 *
 * Переменные окружения:
 *  DSH_LAUNCHER_NO_UPDATE_CHECK=1 — проверки отключены;
 *  DSH_LAUNCHER_FAKE_LAUNCHER_LATEST / DSH_LAUNCHER_FAKE_DSH_LATEST —
 *    «будущая» версия для разработки и тестов (у лаунчера .deb всё равно
 *    берётся из настоящего latest-релиза);
 *  DSH_LAUNCHER_FAKE_INSTALLED — «установленная» версия лаунчера (тесты);
 *  DSH_LAUNCHER_GITHUB_API=http://127.0.0.1:<порт> — подставной GitHub API
 *    (Docker-тесты, test/docker/fakes/github.py): список релизов и .deb.
 */
const DSH_NPM_PKG = '@deepseek-ai/dsh';
const UPDATE_GITHUB_REPO = 'Toximiner/DSH-Launcher';
const UPDATE_CHECK_TIMEOUT = 8000; // сколько ждать проверки, прежде чем не спрашивать
const NO_UPDATE_CHECK = !CONFIG.checkUpdates; // DSH_LAUNCHER_NO_UPDATE_CHECK или «Настройки»





// Установленная версия лаунчера через dpkg (пакет .deb dsh-launcher).
// null — не установлен через .deb (запуск из исходников) → не проверяем.


// Версия пакета лаунчера — из базы dpkg, без запуска dpkg-query: запуск
// дочернего процесса во время загрузки окна-диалога ронял Electron 44
// («О программе»: SIGSEGV / «terminate called without an active exception»).
async function launcherInstalledVersion() {
  const fake = process.env.DSH_LAUNCHER_FAKE_INSTALLED;
  if (fake) return String(fake);
  try {
    return dpkgStatusVersion(await fs.promises.readFile('/var/lib/dpkg/status', 'utf8'), 'dsh-launcher');
  } catch {
    return null;
  }
}

// Проверка с дедлайном: по истечении ms возвращает null — медленная сеть
// не должна держать окно на вопросе, а вопрос — не держать старт.


// Внешние запросы (GitHub, npm, скачивание .deb) — через http/https из Node,
// а не electronNet.fetch. После electronNet.fetch к внешним хостам при старте
// следующий запуск дочернего процесса (dpkg-query, dsh --version в «О
// программе») ронял Electron 44 в нативном коде: «terminate called without
// an active exception» / SIGSEGV (Wayland, Ubuntu 26.04; 1.3.0). Системный
// прокси берём у Chromium (session.resolveProxy — настройки GNOME/KDE и
// переменные окружения) и передаём агенту Node (proxyEnv, Node 24.5+).
// Проверка куки окна (plainUrlIsAuthed) остаётся на electronNet — ей нужна
// сессия окна.



async function proxyAgentFor(u) {
  let rule = 'DIRECT';
  try { rule = await session.defaultSession.resolveProxy(u.toString()); } catch { /* напрямую */ }
  const proxy = proxyUrlFromRule(rule);
  if (!proxy) return undefined;
  const isHttp = u.protocol === 'http:';
  return new (isHttp ? http : https).Agent({ proxyEnv: { [isHttp ? 'HTTP_PROXY' : 'HTTPS_PROXY']: proxy } });
}

/* ===== httpGet (тест test/http.js, с подставным агентом) =====
   GET с переходами по редиректам (ассеты релизов GitHub отдаются через 302)
   → поток ответа (http.IncomingMessage): statusCode, headers, данные. */
async function httpGet(url, { headers = {}, maxRedirects = 5, timeoutMs = 30000 } = {}) {
  const u = new URL(url);
  const agent = await proxyAgentFor(u);
  const res = await new Promise((resolve, reject) => {
    const req = (u.protocol === 'http:' ? http : https).get(u, { headers, agent }, resolve);
    req.on('error', reject);
    req.setTimeout(timeoutMs, () => req.destroy(new Error(`таймаут ${timeoutMs / 1000} с: ${u.host}`)));
  });
  if ([301, 302, 303, 307, 308].includes(res.statusCode) && res.headers.location) {
    res.resume();
    if (maxRedirects <= 0) throw new Error('слишком много перенаправлений');
    return httpGet(new URL(res.headers.location, u).toString(), { headers, maxRedirects: maxRedirects - 1, timeoutMs });
  }
  return res;
}
/* ===== end httpGet ===== */

async function httpJson(url, headers, what) {
  const res = await httpGet(url, { headers });
  let body = '';
  res.setEncoding('utf8');
  for await (const c of res) body += c;
  if (res.statusCode !== 200) throw new Error(`${what}: HTTP ${res.statusCode}`);
  return JSON.parse(body);
}



// Кэш списка релизов: без метки ETag — не спрашивать GitHub чаще раза в час;
// GitHub не ответил — список из кэша, если он не старше часа.
const RELEASES_MAX_AGE_MS = 60 * 60 * 1000;
const releasesCacheFile = () => path.join(app.getPath('userData'), 'releases-cache.json');

// Список релизов GitHub с учётом кэша (см. releasesCacheMode).
async function githubReleases({ force = false } = {}) {
  let entry = null;
  try { entry = JSON.parse(fs.readFileSync(releasesCacheFile(), 'utf8')); } catch { /* кэша нет */ }
  const mode = releasesCacheMode(entry, Date.now(), RELEASES_MAX_AGE_MS, force);
  if (mode === 'cache') return entry.list;
  const headers = { 'User-Agent': 'dsh-launcher', 'Accept': 'application/vnd.github+json' };
  if (mode === 'revalidate') headers['If-None-Match'] = entry.etag;
  if (GITHUB_TOKEN && GITHUB_API === 'https://api.github.com') headers.Authorization = `Bearer ${GITHUB_TOKEN}`; // только настоящему GitHub
  let res;
  let body = '';
  try {
  // 100 — максимум GitHub API: «Вернуть предыдущую версию…» и «Что нового» видят
  // и старые релизы (с 20 они выпадали из списка); ответ кэшируется по ETag
    res = await httpGet(`${GITHUB_API}/repos/${UPDATE_GITHUB_REPO}/releases?per_page=100`, { headers });
    res.setEncoding('utf8');
    for await (const c of res) body += c;
    if (res.statusCode !== 200 && !(res.statusCode === 304 && entry)) throw new Error(`GitHub API: HTTP ${res.statusCode}`);
  } catch (e) {
    // нет сети, лимит, сбой GitHub — недавний список лучше никакого
    if (!force && releasesCacheFresh(entry, Date.now(), RELEASES_MAX_AGE_MS)) {
      console.log(`[launcher] релизы GitHub: ${e.message} — список из кэша`);
      return entry.list;
    }
    throw e;
  }
  let list;
  if (res.statusCode === 304) {
    list = entry.list;
    console.log('[launcher] релизы GitHub: не изменились (304, лимит не расходуется)');
  } else {
    list = JSON.parse(body);
    entry = { etag: res.headers.etag || null, list };
  }
  try {
    fs.mkdirSync(path.dirname(releasesCacheFile()), { recursive: true });
    fs.writeFileSync(releasesCacheFile(), JSON.stringify({ ...entry, fetchedAt: Date.now() }));
  } catch { /* кэш — не обязателен */ }
  return list;
}

// Список релизов (одним запросом): последний — наибольшая версия среди
// опубликованных (не черновик, не prerelease); описания остальных — для
// «Что нового» (изменения между установленной и новой версией).
async function fetchLauncherLatest({ force = false } = {}) {
  const list = await githubReleases({ force });
  const rels = (Array.isArray(list) ? list : []).filter((r) => r && !r.draft && !r.prerelease && r.tag_name);
  if (!rels.length) throw new Error('GitHub API: нет опубликованных релизов');
  const ver = (r) => String(r.tag_name).replace(/^v/, '');
  const j = rels.reduce((a, b) => (compareDebianVer(tagToPackageVersion(ver(b)), tagToPackageVersion(ver(a))) > 0 ? b : a));
  const { version, htmlUrl, assetUrl, assetSha256 } = parseRelease(j);
  const meta = {
    version,
    htmlUrl,
    assetUrl,
    assetSha256,
    releases: rels.map((r) => ({ version: ver(r), body: String(r.body || '') })),
    all: rels.map(parseRelease), // для «Вернуть предыдущую версию…»
  };
  const fake = fakeLatest('DSH_LAUNCHER_FAKE_LAUNCHER_LATEST'); // разработка/тесты
  if (fake) meta.version = fake;
  return meta;
}







// Блок «Что нового» — версии от новой к старой; пусто — ''.
function notesBlockHtml(items) {
  const parts = (items || [])
    .map((r) => ({ v: r.version, html: mdToHtml(releaseNotesLang(r.body, i18n.getLang())) }))
    .filter((r) => r.html);
  if (!parts.length) return '';
  return `<p style="margin:14px 0 6px; color:#d7dce3">${tr('Что нового:', 'What’s new:')}</p>
    <div class="notes">${parts.map((r) => `<div class="ver"><code>${esc(r.v)}</code></div>${r.html}`).join('')}</div>`;
}

async function fetchDshLatest() {
  const j = await httpJson(`https://registry.npmjs.org/${DSH_NPM_PKG}/latest`,
    { 'User-Agent': 'dsh-launcher' }, 'npm registry');
  const version = String(j.version || '');
  if (!version) throw new Error('нет версии в ответе npm');
  const meta = { version, htmlUrl: `https://www.npmjs.com/package/${DSH_NPM_PKG}` };
  const fake = fakeLatest('DSH_LAUNCHER_FAKE_DSH_LATEST'); // разработка/тесты
  if (fake) meta.version = fake;
  return meta;
}

// Подставная «последняя» версия (разработка/тесты): значение переменной или,
// если это путь, — содержимое файла (читается при каждой проверке).
function fakeLatest(name) {
  const v = process.env[name];
  if (!v || !v.startsWith('/')) return v || null;
  try { return fs.readFileSync(v, 'utf8').trim() || null; } catch { return null; }
}


// При старте — оба запроса в параллель (здесь не ждём их завершения).
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
  kind: null,          // 'launcher' | 'dsh' — какой вопрос сейчас на странице
  mode: 'ask',         // 'ask' | 'running' | 'manual' | 'success'
  resolve: null,       // resolve текущего вопроса
  running: false,      // идёт процесс (скачивание / установка)
  log: [], timer: null, proc: null, cmdLine: null,
  latest: null,        // новая версия (текст вопроса)
  installed: null,     // текущая версия
  htmlUrl: null,       // страница релиза (у dsh — страница пакета npm)
  notes: [],           // «Что нового»: релизы между установленной и новой версией (лаунчер)
  debPath: null,       // скачанный .deb
  manualCmd: null,     // команда для терминала (режим manual)
  restartTimer: null,
};

const UPDATE_PREFS = 'update-prefs.json';
const updatePrefFile = () => prefFile(UPDATE_PREFS);

function updatePrefs() {
  return readPref(UPDATE_PREFS) || {};
}

// «Не спрашивать больше» — на версию: отказ от старой не скрывает новую.
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
      <pre>${esc(updateState.log.slice(-40).join('\n') || '…')}</pre>`;
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
      ${kind === 'launcher' ? notesBlockHtml(updateState.notes) : ''}
      <div style="margin-top:12px; display:flex; gap:10px; flex-wrap:wrap">
        <button onclick="location.href='dshlauncher://update/install/'">${tr('Обновить', 'Update')}</button>
        <button onclick="location.href='dshlauncher://update/later/'">${tr('Не сейчас', 'Not now')}</button>
        <button onclick="location.href='dshlauncher://update/never/'">${tr('Не спрашивать больше', 'Don’t ask again')}</button>
        ${link}
      </div>`;
  }
  pages.showStatus(title, body, noteIsError);
}

// Показывает вопрос (режим ask/manual) и ждёт выбора:
// 'install' | 'later' | 'never' | 'recheck'. 'copy' и 'restart' вопрос не
// разрешают (перерисовывают страницу / закрывают приложение).
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
    return launcher.blankResponse();
  }
  if (choice === 'restart') {
    restartLauncher();
    return launcher.blankResponse();
  }
  const resolve = updateState.resolve;
  if (resolve && !updateState.running) {
    updateState.resolve = null;
    resolve(choice);
  }
  return launcher.blankResponse();
}

// Шаг установки с живым логом на странице.
function runUpdateStep(cmd, args, env) {
  updateState.mode = 'running';
  updateState.cmdLine = [cmd, ...args].join(' ');
  console.log(`[launcher] шаг обновления: ${updateState.cmdLine}`);
  return runWithLiveLog(updateState, cmd, args, { env, redraw: () => showUpdatePage() });
}

// Скачивание .deb из релиза в userData/updates/. Прогресс — в логе на
// странице (те же 800 мс, что и установки). sha256 считается по байтам из
// сети (а не по файлу на диске) и, если GitHub его знает, сверяется с digest.
async function downloadDeb(url, expectedSha256) {
  updateState.running = true;
  updateState.mode = 'running';
  updateState.log = [];
  updateState.cmdLine = `download ${url}`;
  console.log(`[launcher] скачиваю ${url}`);
  const dir = path.join(app.getPath('userData'), 'updates');
  try {
    fs.mkdirSync(dir, { recursive: true });
    // Пакеты прошлых попыток больше не нужны — не копим их.
    for (const f of fs.readdirSync(dir)) {
      if (f.endsWith('.deb')) try { fs.unlinkSync(path.join(dir, f)); } catch { /* noop */ }
    }
    const res = await httpGet(url, { headers: { 'User-Agent': 'dsh-launcher' } }); // не electronNet — см. proxyUrlFromRule
    if (res.statusCode !== 200) { res.resume(); throw new Error(`download: HTTP ${res.statusCode}`); }
    const total = Number(res.headers['content-length'] || 0);
    const name = url.split('/').pop() || 'dsh-launcher.deb';
    const dest = path.join(dir, name);
    updateState.log.push(total ? `${name} (${(total / 1048576).toFixed(1)} МБ)` : name);
    const out = fs.createWriteStream(dest);
    const hash = crypto.createHash('sha256');
    let got = 0;
    updateState.timer = setInterval(() => {
      updateState.log.push(total
        ? `${(got / 1048576).toFixed(1)} / ${(total / 1048576).toFixed(1)} МБ (${Math.round((100 * got) / total)}%)`
        : `${(got / 1048576).toFixed(1)} МБ`);
      if (updateState.log.length > 500) updateState.log.shift();
      showUpdatePage();
    }, 800);
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

// Перезапуск после обновления лаунчера: отпускаем single-instance лок, чтобы
// новый экземпляр сразу его взял, запускаем новый отсоединённым (он
// переживёт наш выход) и закрываемся штатно — через before-quit.


function restartLauncher() {
  if (state.restarting || state.stopping) return;
  state.restarting = true;
  const exe = process.env.DSH_LAUNCHER_RESTART_BIN || '/usr/bin/dsh-launcher';
  app.releaseSingleInstanceLock();
  let started = false;
  const fail = (e) => {
    state.restarting = false;
    state.launcherUpdatedOnDisk = false; // не перезапускаться заново на каждом падении процесса
    app.requestSingleInstanceLock(); // остаёмся единственным экземпляром
    if (state.stopping) return;
    console.error('[launcher] авто-перезапуск не удался:', e.message);
    showUpdatePage(
      tr(`Не удалось запустить новую версию: ${e.message}. Обновление уже установлено — перезапустите лаунчер вручную.`,
         `Failed to start the new version: ${e.message}. The update is already installed — restart the launcher manually.`),
      true
    );
  };
  // запуск — через bash (spawnLauncherAfterExit): нет программы — узнаём здесь,
  // а не по 'error' (bash-то есть)
  try { fs.accessSync(exe, fs.constants.X_OK); } catch (e) { fail(e); return; }
  const p = settingsWindow.spawnLauncherAfterExit(exe, [], 'ignore');
  p.on('spawn', () => { started = true; });
  p.on('error', fail);
  p.unref();
  // Даём событию 'error' chance (нет exe — сработает сразу): если процесс не
  // стартовал, приложение не закрываем — показана ошибка.
  setTimeout(() => {
    if (!started) return;
    console.log(`[launcher] запущена новая версия (${exe}), закрываю старую`);
    app.quit();
  }, 200);
}

// Лаунчер обновлён на диске, а работает ещё старый: apt заменил файлы
// /opt/dsh-launcher под нами. Уже открытые файлы живут (старые inode), но
// новые процессы Chromium (GPU, утилиты) запустились бы из нового бинарника —
// поэтому перезапускаемся быстро, а при падении любого процесса — сразу.


function showUpdateSuccess() {
  state.launcherUpdatedOnDisk = true;
  if (updateState.debPath) {
    try { fs.unlinkSync(updateState.debPath); } catch { /* noop */ }
    updateState.debPath = null;
  }
  updateState.mode = 'success';
  showUpdatePage();
  // ~1.5 с, чтобы страница успела показаться; если окно закроют раньше —
  // before-quit очистит таймер.
  updateState.restartTimer = setTimeout(() => restartLauncher(), 1500);
}

// Установка .deb от root (через pkexec). pkexec чистит окружение, поэтому
// DEBIAN_FRONTEND задаётся внутри. Пакет копируется в каталог root и
// сверяется с sha256, посчитанным при скачивании: файл в userData доступен
// пользователю на запись, и без копии его можно было бы подменить между
// проверкой и установкой. $1 — путь к .deb, $2 — sha256.
const ROOT_INSTALL_SH = [
  'set -e',
  'd=$(mktemp -d)',
  'trap \'rm -rf "$d"\' EXIT',
  'cp -- "$1" "$d/update.deb"',
  'echo "$2  $d/update.deb" | sha256sum -c --quiet -',
  // --allow-downgrades: «Вернуть предыдущую версию…» ставит версию старше установленной
  'DEBIAN_FRONTEND=noninteractive apt-get install -y --allow-downgrades "$d/update.deb"',
].join('; ');

// true — лаунчер обновлён и сейчас перезапустится: dsh этим экземпляром не
// запускаем (его поднимет новый).
async function offerLauncherUpdate() {
  if (UPDATE_NOW === 'dsh') return; // просили обновить dsh — о лаунчере спросим в другой раз
  const newest = await state.updateChecks.launcher;
  if (!newest || !newest.version) return; // нет сети / отключено / таймаут
  const installed = await launcherInstalledVersion();
  if (!installed) return; // не установлен через .deb (запуск из исходников)
  // «Вернуть предыдущую версию…»: ставим выбранную (старше установленной)
  const target = INSTALL_VERSION ? (newest.all || []).find((r) => r.version === INSTALL_VERSION) : null;
  if (INSTALL_VERSION && !target) { console.error(`[launcher] версии ${INSTALL_VERSION} нет среди релизов — откат отменён`); return; }
  const latest = target ? { ...newest, ...target } : newest;
  const latestPkg = tagToPackageVersion(latest.version);
  if (!target && compareDebianVer(latestPkg, installed) <= 0) return; // новой версии нет
  updateState.rollback = Boolean(target);
  if (target) console.log(`[launcher] откат: ставлю ${target.version} (установлена ${installed})`);
  if (UPDATE_NOW !== 'launcher' && updateDismissed('launcher', latest.version)) {
    console.log(`[launcher] обновление до ${latest.version} отклонено — не спрашиваю`);
    return;
  }
  console.log(`[launcher] новый dsh-launcher: ${latest.version} (установлен ${installed})`);
  updateState.kind = 'launcher';
  updateState.latest = latest.version;
  updateState.installed = installed;
  updateState.htmlUrl = latest.htmlUrl;
  updateState.notes = notesBetween(latest.releases, installed, latest.version, 'deb');
  updateState.mode = 'ask';
  updateState.debPath = null;
  updateState.manualCmd = null;
  let note = null;
  let noteIsError = false;
  let auto = UPDATE_NOW === 'launcher'; // «Обновить сейчас» — первый раз без вопроса
  for (;;) {
    const choice = auto ? 'install' : await askUpdate(note, noteIsError);
    auto = false;
    if (state.stopping) return;
    if (choice === 'never') {
      updateDismiss('launcher', latest.version);
      return;
    }
    if (choice === 'later') {
      console.log('[launcher] обновление лаунчера отложено (не сейчас)');
      return;
    }
    if (choice === 'recheck') {
      // Пользователь поставил .deb сам, в терминале.
      const now = await launcherInstalledVersion();
      if (now && compareDebianVer(latestPkg, now) <= 0) {
        updateState.installed = now;
        console.log(`[launcher] dsh-launcher обновлён до ${now} (вручную)`);
        showUpdateSuccess();
        return true;
      }
      updateState.mode = 'manual';
      note = tr(`Установлена версия ${now || 'неизвестно'}. Поставьте пакет и нажмите «Проверить ещё раз».`,
                `Installed version is ${now || 'unknown'}. Install the package and click "Check again".`);
      noteIsError = true;
      continue;
    }
    // choice === 'install'
    if (!latest.assetUrl) {
      updateState.mode = 'manual';
      note = tr('В этом релизе нет файла .deb — скачайте его со страницы релиза.',
                'This release has no .deb file — download it from the release page.');
      noteIsError = true;
      continue;
    }
    const dl = await downloadDeb(latest.assetUrl, latest.assetSha256);
    if (state.stopping) return;
    if (!dl.ok) {
      updateState.mode = 'manual';
      updateState.manualCmd = `curl -L -o dsh-launcher.deb ${latest.assetUrl} && sudo apt install -y ./dsh-launcher.deb`;
      note = tr(`Скачивание не удалось: ${dl.err}. Выполните команду выше в терминале и нажмите «Проверить ещё раз».`,
                `Download failed: ${dl.err}. Run the command above in a terminal and click "Check again".`);
      noteIsError = true;
      continue;
    }
    const pkexec = findBin('pkexec');
    if (!pkexec) {
      updateState.mode = 'manual';
      updateState.manualCmd = `sudo apt install -y ${dl.path}`;
      note = tr('pkexec в системе не найден — поставьте скачанный пакет командой выше.',
                'pkexec was not found on the system — install the downloaded package with the command above.');
      noteIsError = true;
      continue;
    }
    const code = await runUpdateStep(pkexec, ['/bin/sh', '-c', ROOT_INSTALL_SH, 'sh', dl.path, dl.sha256]);
    if (state.stopping) return;
    if (code === 0) {
      console.log(`[launcher] dsh-launcher ${latest.version} установлен`);
      // после отката не предлагать сразу вернуть самую новую
      if (target && newest.version !== target.version) updateDismiss('launcher', newest.version);
      showUpdateSuccess();
      return true;
    }
    // Отказ от ввода пароля polkit — тоже ненулевой код: команда для
    // терминала ссылается на уже скачанный пакет.
    console.error(`[launcher] pkexec apt-get завершился с кодом ${code} — даю команду вручную`);
    updateState.mode = 'manual';
    updateState.manualCmd = `sudo apt install -y ${dl.path}`;
    note = tr(`Установка не удалась (код ${code}). Если вы отменили ввод пароля — попробуйте ещё раз.`,
              `Installation failed (code ${code}). If you cancelled the password prompt — try again.`);
    noteIsError = true;
  }
}

// Текущая версия dsh: из dshInstallCheck при старте (onReady); если почему-
// то нет — пробинуем саму команду.


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
  console.log(`[launcher] новый ${DSH_NPM_PKG}: ${latest.version} (установлен ${state.dshInstalledVersion})`);
  updateState.kind = 'dsh';
  updateState.notes = []; // у npm нет описаний версий — только ссылка на пакет
  updateState.latest = latest.version;
  updateState.installed = state.dshInstalledVersion;
  updateState.htmlUrl = latest.htmlUrl;
  updateState.mode = 'ask';
  updateState.debPath = null;
  updateState.manualCmd = null;
  const cmdAuto = `npm install -g ${DSH_NPM_PKG}@${latest.version}`;
  let note = null;
  let noteIsError = false;
  let auto = UPDATE_NOW === 'dsh'; // «Обновить сейчас» — первый раз без вопроса
  for (;;) {
    const choice = auto ? 'install' : await askUpdate(note, noteIsError);
    auto = false;
    if (state.stopping) return;
    if (choice === 'never') {
      updateDismiss('dsh', latest.version);
      return;
    }
    if (choice === 'later') {
      console.log('[launcher] обновление dsh отложено (не сейчас)');
      return;
    }
    if (choice === 'recheck') {
      const r = await probeDsh(state.dshBin);
      if (r.ok && compareSemVer(r.version, latest.version) >= 0) {
        state.dshInstalledVersion = r.version;
        console.log(`[launcher] dsh обновлён до ${r.version} (в терминале)`);
        return;
      }
      updateState.mode = 'manual';
      updateState.manualCmd = cmdAuto;
      note = tr(`Всё ещё версия ${r.ok ? r.version : 'неизвестно'}. Поставьте новую и нажмите «Проверить ещё раз».`,
                `Still version ${r.ok ? r.version : 'unknown'}. Install the new one and click "Check again".`);
      noteIsError = true;
      continue;
    }
    // choice === 'install'
    const npmInfo = await npmInstallInfo();
    if (state.stopping) return;
    if (!npmInfo.hasNpm || !npmInfo.canAuto) {
      // Глобальный prefix недоступен без sudo (или npm нет) — команда для
      // терминала.
      updateState.mode = 'manual';
      updateState.manualCmd = npmInfo.hasNpm ? `sudo ${cmdAuto}` : cmdAuto;
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
      noteIsError = true;
      continue;
    }
    const code = await runUpdateStep(npmInfo.npm, ['install', '-g', `${DSH_NPM_PKG}@${latest.version}`], envWithBinDir(npmInfo.npm));
    if (state.stopping) return;
    if (code === 0) {
      const r = await probeDsh(state.dshBin);
      if (r.ok) state.dshInstalledVersion = r.version;
      if (r.ok && compareSemVer(r.version, latest.version) >= 0) {
        console.log(`[launcher] ${DSH_NPM_PKG} обновлён → ${r.version}; dsh стартует с новой версией`);
        return;
      }
      // npm отработал, но запускаемый dsh — прежний: npm ставит в другой
      // prefix (например, dsh из nvm, а npm — системный).
      console.error(`[launcher] npm install прошёл, но ${state.dshBin} — ${r.ok ? r.version : 'не запускается'} (prefix npm: ${npmInfo.prefix})`);
      updateState.mode = 'manual';
      updateState.manualCmd = cmdAuto;
      note = tr(`npm поставил пакет в ${npmInfo.prefix || 'свой prefix'}, но лаунчер запускает ${state.dshBin} (версия ${r.ok ? r.version : 'неизвестна'}). Обновите dsh тем npm, которым он был установлен, и нажмите «Проверить ещё раз».`,
                `npm installed the package into ${npmInfo.prefix || 'its prefix'}, but the launcher runs ${state.dshBin} (version ${r.ok ? r.version : 'unknown'}). Update dsh with the npm it was installed with and click "Check again".`);
      noteIsError = true;
      continue;
    }
    console.error(`[launcher] обновление dsh не удалось (код ${code})`);
    updateState.mode = 'manual';
    updateState.manualCmd = cmdAuto; // prefix доступен без sudo (canAuto) — sudo с nvm только навредит
    note = tr(`Установка не удалась (код ${code}). Повторите, либо выполните команду выше в терминале и нажмите «Проверить ещё раз».`,
              `Installation failed (code ${code}). Retry, or run the command above in a terminal and click "Check again".`);
    noteIsError = true;
  }
}

Object.assign(module.exports, { DSH_NPM_PKG, UPDATE_GITHUB_REPO, UPDATE_CHECK_TIMEOUT, NO_UPDATE_CHECK, launcherInstalledVersion, fetchLauncherLatest, notesBlockHtml, fetchDshLatest, startUpdateChecks, updateState, updatePrefFile, updatePrefs, onUpdateChoice, restartLauncher, offerLauncherUpdate, offerDshUpdate }); // дополняем, а не заменяем: модули подключают друг друга по кругу
