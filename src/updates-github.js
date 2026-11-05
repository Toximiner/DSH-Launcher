'use strict';
// Updates — the release list (GitHub API) and the "latest" dsh version
// (npm registry): ETag cache of releases, picking the latest published,
// and stub versions for development and tests.
const { app } = require('electron');
const fs = require('fs');
const path = require('path');
const { httpGet, httpJson } = require('./updates-http');
const { GITHUB_TOKEN, GITHUB_API } = require('./env');
const { compareDebianVer, tagToPackageVersion } = require('./versions');
const { releasesCacheMode, releasesCacheFresh, parseRelease } = require('./updates-logic');

const DSH_NPM_PKG = '@deepseek-ai/dsh';
const UPDATE_GITHUB_REPO = 'Toximiner/DSH-Launcher';

// Release list cache: without an ETag — do not ask GitHub more than once
// an hour; if GitHub did not answer — the list from the cache, if it is
// no older than an hour.
const RELEASES_MAX_AGE_MS = 60 * 60 * 1000;
const releasesCacheFile = () => path.join(app.getPath('userData'), 'releases-cache.json');

// GitHub release list (repo, cache in cacheFile) with the cache taken into
// account (see releasesCacheMode). 100 — the GitHub API maximum: «Вернуть
// предыдущую версию…» (Roll back to a previous version…) and «Что нового»
// (What's new) see old releases too (with 20 they dropped out of the list);
// the response is cached by ETag.
async function githubReleasesList(repo, cacheFile, { force = false } = {}) {
  let entry = null;
  try { entry = JSON.parse(fs.readFileSync(cacheFile(), 'utf8')); } catch { /* no cache */ }
  const mode = releasesCacheMode(entry, Date.now(), RELEASES_MAX_AGE_MS, force);
  if (mode === 'cache') return entry.list;
  const headers = { 'User-Agent': 'dsh-launcher', 'Accept': 'application/vnd.github+json' };
  if (mode === 'revalidate') headers['If-None-Match'] = entry.etag;
  if (GITHUB_TOKEN && GITHUB_API === 'https://api.github.com') headers.Authorization = `Bearer ${GITHUB_TOKEN}`; // only to the real GitHub
  let res;
  let body = '';
  try {
    res = await httpGet(`${GITHUB_API}/repos/${repo}/releases?per_page=100`, { headers });
    res.setEncoding('utf8');
    for await (const c of res) body += c;
    if (res.statusCode !== 200 && !(res.statusCode === 304 && entry)) throw new Error(`GitHub API: HTTP ${res.statusCode}`);
  } catch (e) {
    // no network, a limit, a GitHub failure — a recent list is better than none
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
    fs.mkdirSync(path.dirname(cacheFile()), { recursive: true });
    fs.writeFileSync(cacheFile(), JSON.stringify({ ...entry, fetchedAt: Date.now() }));
  } catch { /* the cache is optional */ }
  return list;
}

// The launcher's release list (see githubReleasesList).
async function githubReleases({ force = false } = {}) {
  return githubReleasesList(UPDATE_GITHUB_REPO, releasesCacheFile, { force });
}

// Release list (in one request): the latest — the highest version among
// the published ones (not a draft, not a prerelease); the descriptions of
// the rest — for «Что нового» (What's new) (the changes between the
// installed and the new version).
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
    all: rels.map(parseRelease), // for «Вернуть предыдущую версию…» (Roll back to a previous version…)
  };
  const fake = fakeLatest('DSH_LAUNCHER_FAKE_LAUNCHER_LATEST'); // development/tests
  if (fake) meta.version = fake;
  return meta;
}

// A stub "latest" version (development/tests): the value of the variable or,
// if it is a path, the contents of the file (read on every check).
function fakeLatest(name) {
  const v = process.env[name];
  if (!v || !v.startsWith('/')) return v || null;
  try { return fs.readFileSync(v, 'utf8').trim() || null; } catch { return null; }
}

// dsh releases — from the project's GitHub repository (deepseek-harness):
// the npm registry does not serve version descriptions, while the GitHub
// releases have them (tag dsh-v<version>). Its own ETag cache — separate
// from the launcher's releases: a different repository.
const DSH_GITHUB_REPO = 'deepseek-ai/deepseek-harness';
const dshReleasesCacheFile = () => path.join(app.getPath('userData'), 'dsh-releases-cache.json');

async function fetchDshReleases({ force = false } = {}) {
  return githubReleasesList(DSH_GITHUB_REPO, dshReleasesCacheFile, { force });
}

// The latest dsh version — from the npm registry (not from the local npm:
// without an install and without access to the global prefix). The release
// descriptions («Что нового»/What's new) — from GitHub; their failure does
// not break the update offer — the descriptions are simply empty.
async function fetchDshLatest() {
  const j = await httpJson(`https://registry.npmjs.org/${DSH_NPM_PKG}/latest`,
    { 'User-Agent': 'dsh-launcher' }, 'npm registry');
  const version = String(j.version || '');
  if (!version) throw new Error('нет версии в ответе npm');
  const meta = { version, htmlUrl: `https://www.npmjs.com/package/${DSH_NPM_PKG}`, releases: [] };
  try {
    const list = await fetchDshReleases();
    meta.releases = (Array.isArray(list) ? list : [])
      .filter((r) => r && !r.draft && String(r.tag_name || '').startsWith('dsh-v'))
      .map((r) => ({ version: String(r.tag_name).replace(/^dsh-v/, ''), body: String(r.body || '') }));
  } catch (e) {
    console.log(`[launcher] описания релизов dsh: ${e.message} — без «Что нового»`);
  }
  const fake = fakeLatest('DSH_LAUNCHER_FAKE_DSH_LATEST'); // development/tests
  if (fake) meta.version = fake;
  return meta;
}

// RELEASES_MAX_AGE_MS / releasesCacheFile / fakeLatest — internal to the
// module (release cache, stub dev/test versions) — not exported.
module.exports = { DSH_NPM_PKG, UPDATE_GITHUB_REPO, githubReleases, fetchLauncherLatest, fetchDshLatest };
