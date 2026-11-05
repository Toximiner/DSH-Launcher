'use strict';
// Updates — pure logic: the release list cache, the interval of the
// periodic check, what to show (a menu item, a toast) for the found
// versions, and the proxy.
const { compareDebianVer, tagToPackageVersion } = require('./versions');

/* How to get the GitHub release list: 'revalidate' — ask with
   If-None-Match (there is an ETag; a 304 "unchanged" response does not
   spend the 60/h limit — therefore always, and a new release is visible
   immediately), 'cache' — take the remembered one without a request
   (no tag, and the last check was less than maxAgeMs ago; a manual
   check — force — does not do that), 'fetch' — a regular request. */
function releasesCacheMode(entry, now, maxAgeMs, force) {
  const ok = entry && typeof entry === 'object' && Array.isArray(entry.list);
  if (!ok) return 'fetch';
  if (entry.etag) return 'revalidate';
  if (!force && releasesCacheFresh(entry, now, maxAgeMs)) return 'cache';
  return 'fetch';
}

// The cache was checked less than maxAgeMs ago (a time from the future — the clocks shifted — no).
function releasesCacheFresh(entry, now, maxAgeMs) {
  return Boolean(entry) && Number.isFinite(entry.fetchedAt) && now - entry.fetchedAt >= 0 && now - entry.fetchedAt < maxAgeMs;
}

/* updatePollMs — the interval of the periodic check: 12 hours
   (DSH_LAUNCHER_UPDATE_POLL_SEC — for tests). announceUpdates — what to
   show for the found updates: found — { launcher, dsh } (new versions or
   null), dismissed — update-prefs.json ("don't ask again" per version),
   notified — which versions have already been notified about in this
   session. → { menu: { kind: version } — "Update … to X…" items in
   "Help", notify: [kind] — system notifications (once per version; not
   for a dismissed one — there is still a menu item for it) }. */
function updatePollMs(env) {
  const sec = Number(env.DSH_LAUNCHER_UPDATE_POLL_SEC);
  return Number.isFinite(sec) && sec >= 1 ? Math.round(sec * 1000) : 12 * 3600 * 1000;
}

function announceUpdates(found, dismissed, notified) {
  const menu = {};
  const notify = [];
  for (const kind of ['launcher', 'dsh']) {
    const v = found && found[kind];
    if (!v) continue;
    menu[kind] = v;
    if ((dismissed || {})[kind] !== v && (notified || {})[kind] !== v) notify.push(kind);
  }
  return { menu, notify };
}

/* A Chromium rule ("PROXY h:p; DIRECT", "HTTPS h:p", "SOCKS5 h:p",
   "DIRECT") → a proxy URL for Node, or null (direct). We take the first
   HTTP(S) proxy; the Node agent cannot do SOCKS — then direct. */
function proxyUrlFromRule(rule) {
  for (const part of String(rule || '').split(';')) {
    const m = part.trim().match(/^(PROXY|HTTPS)\s+(\S+)$/i);
    if (m) return (m[1].toUpperCase() === 'HTTPS' ? 'https://' : 'http://') + m[2];
  }
  return null;
}

/* ===== rollbackCandidates (pure functions, test test/updates.js) =====
   parseRelease — a GitHub release → { version, date, htmlUrl, assetUrl, assetSha256 }
   (the .deb and its sha256 from the "sha256:<hex>" digest; older releases may not have it).
   rollbackCandidates — what you can roll back to: from the parsed releases
   (parseRelease; drafts and prereleases were filtered out earlier) — older
   than the installed package, with a .deb; newest first, no more than max. */
function parseRelease(r) {
  const asset = (r.assets || []).find((a) => String(a.name).endsWith('.deb'));
  return {
    version: String(r.tag_name).replace(/^v/, ''),
    date: r.published_at ? String(r.published_at).slice(0, 10) : null,
    htmlUrl: r.html_url || null,
    assetUrl: asset ? asset.browser_download_url : null,
    assetSha256: asset && /^sha256:[0-9a-f]{64}$/i.test(String(asset.digest || '')) ? String(asset.digest).slice(7).toLowerCase() : null,
  };
}

function rollbackCandidates(releases, installedPkg, max = 5) {
  return (Array.isArray(releases) ? releases : [])
    .filter((r) => r && r.version && r.assetUrl && compareDebianVer(tagToPackageVersion(r.version), installedPkg) < 0)
    .sort((a, b) => compareDebianVer(tagToPackageVersion(b.version), tagToPackageVersion(a.version)))
    .slice(0, max);
}
/* ===== end rollbackCandidates ===== */

module.exports = { releasesCacheMode, releasesCacheFresh, updatePollMs, announceUpdates, proxyUrlFromRule, parseRelease, rollbackCandidates };
