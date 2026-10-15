'use strict';
// Обновления — список релизов (GitHub API) и «последняя» версия dsh
// (npm-реестр): кэш релизов по ETag, выбор последней опубликованной,
// подставные версии для разработки и тестов.
const { app } = require('electron');
const fs = require('fs');
const path = require('path');
const { httpGet, httpJson } = require('./updates-http');
const { GITHUB_TOKEN, GITHUB_API } = require('./env');
const { compareDebianVer, tagToPackageVersion } = require('./versions');
const { releasesCacheMode, releasesCacheFresh, parseRelease } = require('./updates-logic');

const DSH_NPM_PKG = '@deepseek-ai/dsh';
const UPDATE_GITHUB_REPO = 'Toximiner/DSH-Launcher';

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

// Подставная «последняя» версия (разработка/тесты): значение переменной или,
// если это путь, — содержимое файла (читается при каждой проверке).
function fakeLatest(name) {
  const v = process.env[name];
  if (!v || !v.startsWith('/')) return v || null;
  try { return fs.readFileSync(v, 'utf8').trim() || null; } catch { return null; }
}

// Последняя версия dsh — из npm-реестра (не с локального npm: без установки и
// без доступа к глобальному prefix).
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

module.exports = { DSH_NPM_PKG, UPDATE_GITHUB_REPO, RELEASES_MAX_AGE_MS, releasesCacheFile, githubReleases, fetchLauncherLatest, fakeLatest, fetchDshLatest };
