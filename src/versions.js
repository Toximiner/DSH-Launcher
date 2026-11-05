'use strict';
// Versions: comparison (Debian and semver), update status, package version
// from the dpkg database, "should we show What's new", changes between versions.

// Debian version (launcher): X.Y.Z[-R]. Numeric parts are compared in order;
// a missing revision counts as 0 (CI builds the vX.Y.Z release into the
// X.Y.Z-1 package, so a tag with the same upstream is newer than "bare" X.Y.Z).
function parseDebianVer(s) {
  const m = /^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-(\d+))?$/.exec(String(s).trim());
  if (!m) return null;
  return {
    upstream: [Number(m[1]), Number(m[2] || 0), Number(m[3] || 0)],
    rev: m[4] === undefined ? 0 : Number(m[4]),
  };
}
// semver (dsh): X.Y.Z[-prerelease]. A prerelease is lower than the release
// (0.2.0-rc.2 < 0.2.0); prereleases are compared segment by segment
// (0.2.0-rc.2 < 0.2.0-rc.10), a numeric segment is lower than an alphabetic
// one (0.2.0-1 < 0.2.0-alpha).
function parseSemVer(s) {
  const m = /^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?/.exec(String(s).trim());
  if (!m) return null;
  return { upstream: [Number(m[1]), Number(m[2]), Number(m[3])], pre: m[4] || null };
}
function cmpUpstream(a, b) {
  for (let i = 0; i < 3; i++) if (a[i] !== b[i]) return a[i] < b[i] ? -1 : 1;
  return 0;
}
function compareDebianVer(a, b) {
  const pa = parseDebianVer(a);
  const pb = parseDebianVer(b);
  if (!pa || !pb) return 0; // unrecognized — no update is offered
  const c = cmpUpstream(pa.upstream, pb.upstream);
  if (c !== 0) return c;
  return pa.rev === pb.rev ? 0 : pa.rev < pb.rev ? -1 : 1;
}
function compareSemVer(a, b) {
  const pa = parseSemVer(a);
  const pb = parseSemVer(b);
  if (!pa || !pb) return 0;
  const c = cmpUpstream(pa.upstream, pb.upstream);
  if (c !== 0) return c;
  if (pa.pre === pb.pre) return 0;
  if (pa.pre === null) return 1; // release > prerelease
  if (pb.pre === null) return -1;
  const sa = pa.pre.split('.');
  const sb = pb.pre.split('.');
  for (let i = 0; i < Math.max(sa.length, sb.length); i++) {
    const x = sa[i];
    const y = sb[i];
    if (x === undefined) return -1;
    if (y === undefined) return 1;
    const xn = /^\d+$/.test(x);
    const yn = /^\d+$/.test(y);
    if (xn && yn) { if (x !== y) return Number(x) < Number(y) ? -1 : 1; continue; }
    if (xn !== yn) return xn ? -1 : 1;
    if (x !== y) return x < y ? -1 : 1;
  }
  return 0;
}
// GitHub tag → package version (CI rule: vX.Y.Z → X.Y.Z-1, vX.Y.Z-R → X.Y.Z-R)
function tagToPackageVersion(tag) {
  const t = String(tag).replace(/^v/, '').trim();
  return t.includes('-') ? t : t + '-1';
}

/* Итог проверки обновлений для одного компонента: 'nocheck' (нет ответа
   сервера), 'notinstalled' (установленная версия неизвестна), 'uptodate',
   'available'. scheme: 'semver' — npm и сборка лаунчера из исходников
   (package.json, без ревизии), 'deb' — пакет лаунчера (тег vX.Y.Z → X.Y.Z-1). */
function updateStatus(latest, installed, scheme) {
  if (!latest) return 'nocheck';
  if (!installed) return 'notinstalled';
  const cmp = scheme === 'deb'
    ? compareDebianVer(tagToPackageVersion(latest), installed)
    : compareSemVer(latest, installed);
  return cmp <= 0 ? 'uptodate' : 'available';
}

/* Версия установленного пакета из базы dpkg (/var/lib/dpkg/status): запись
   «Package: <pkg>» со «Status: … installed» → её «Version:», иначе null. */
function dpkgStatusVersion(text, pkg) {
  for (const block of String(text || '').split(/\n(?=Package: )/)) {
    const name = /^Package: (\S+)/m.exec(block);
    if (!name || name[1] !== pkg) continue;
    if (!/^Status: .* installed$/m.test(block)) continue;
    const ver = /^Version: (\S+)/m.exec(block);
    if (ver) return ver[1];
  }
  return null;
}

function shouldShowWhatsNew(last, current, scheme) {
  if (!last || !current) return false;
  return (scheme === 'deb' ? compareDebianVer(current, last) : compareSemVer(current, last)) > 0;
}

/* Релизы новее установленной версии и не новее предлагаемой — новые
   сверху. scheme — как в updateStatus: 'deb' (пакет: 1.5.1-1) или
   'semver' (сборка из исходников: 1.5.1). Установленная неизвестна —
   только предлагаемая версия. */
function notesBetween(releases, installed, latest, scheme) {
  const key = (v) => (scheme === 'deb' ? tagToPackageVersion(v) : String(v).replace(/^v/, ''));
  const cmp = (a, b) => (scheme === 'deb' ? compareDebianVer(a, b) : compareSemVer(a, b));
  const top = key(latest);
  return (releases || [])
    .filter((r) => r && r.version && cmp(key(r.version), top) <= 0 &&
      (installed ? cmp(key(r.version), installed) > 0 : cmp(key(r.version), top) === 0))
    .sort((a, b) => cmp(key(b.version), key(a.version)));
}

module.exports = { parseDebianVer, parseSemVer, compareDebianVer, compareSemVer, tagToPackageVersion, updateStatus, dpkgStatusVersion, shouldShowWhatsNew, notesBetween };
