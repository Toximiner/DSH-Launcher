#!/usr/bin/env node
// Unit tests for periodic update checks (src/updates-logic.js): updatePollMs —
// the interval; announceUpdates — menu items and notifications for found versions.
'use strict';
const assert = require('assert');
const { updatePollMs, announceUpdates } = require('../src/updates-logic');

// ---- updatePollMs ----
assert.strictEqual(updatePollMs({}), 12 * 3600 * 1000, 'default — 12 hours');
assert.strictEqual(updatePollMs({ DSH_LAUNCHER_UPDATE_POLL_SEC: '3' }), 3000, 'tests — in seconds');
for (const bad of ['0', '-5', 'abc', '', '0.5']) {
  assert.strictEqual(updatePollMs({ DSH_LAUNCHER_UPDATE_POLL_SEC: bad }), 12 * 3600 * 1000, `"${bad}" — default`);
}

// ---- announceUpdates ----
assert.deepStrictEqual(announceUpdates({}, {}, {}), { menu: {}, notify: [] }, 'nothing found');
assert.deepStrictEqual(announceUpdates({ launcher: null, dsh: undefined }, {}, {}), { menu: {}, notify: [] }, 'null — no update');
assert.deepStrictEqual(announceUpdates({ launcher: '1.8.0', dsh: '0.3.0' }, {}, {}),
  { menu: { launcher: '1.8.0', dsh: '0.3.0' }, notify: ['launcher', 'dsh'] }, 'both — menu and notifications');
assert.deepStrictEqual(announceUpdates({ launcher: '1.8.0' }, {}, { launcher: '1.8.0' }),
  { menu: { launcher: '1.8.0' }, notify: [] }, 'already notified — menu only');
assert.deepStrictEqual(announceUpdates({ launcher: '1.9.0' }, {}, { launcher: '1.8.0' }),
  { menu: { launcher: '1.9.0' }, notify: ['launcher'] }, 'an even newer one came out — notify again');
assert.deepStrictEqual(announceUpdates({ dsh: '0.3.0' }, { dsh: '0.3.0' }, {}),
  { menu: { dsh: '0.3.0' }, notify: [] }, '"do not ask again" — no notification, the menu item is present');
assert.deepStrictEqual(announceUpdates({ dsh: '0.4.0' }, { dsh: '0.3.0' }, {}),
  { menu: { dsh: '0.4.0' }, notify: ['dsh'] }, 'old one dismissed — notify about the new one');
assert.deepStrictEqual(announceUpdates({ dsh: '0.3.0' }, null, undefined),
  { menu: { dsh: '0.3.0' }, notify: ['dsh'] }, 'no files — as if empty');

console.log('OK: updatePollMs, announceUpdates — all checks passed');

// ---- parseRelease, rollbackCandidates: “Revert to a previous version…” ----
{
  const { parseRelease, rollbackCandidates } = require('../src/updates-logic');
  const sha = 'a'.repeat(64);
  const rel = (tag, deb = true, digest = 'sha256:' + sha) => parseRelease({
    tag_name: tag, published_at: '2026-10-01T10:00:00Z', html_url: 'https://x/' + tag,
    assets: deb ? [{ name: `dsh-launcher_${tag.slice(1)}-1_amd64.deb`, browser_download_url: 'https://dl/' + tag, digest }] : [{ name: 'notes.txt' }],
  });
  assert.deepStrictEqual(rel('v1.2.0'), { version: '1.2.0', date: '2026-10-01', htmlUrl: 'https://x/v1.2.0', assetUrl: 'https://dl/v1.2.0', assetSha256: sha }, 'release parsing');
  assert.strictEqual(rel('v1.0.0', true, '').assetSha256, null, 'old release without a digest');
  assert.strictEqual(rel('v1.0.0', false).assetUrl, null, 'without a .deb');
  assert.deepStrictEqual(parseRelease({ tag_name: '1.3.0' }), { version: '1.3.0', date: null, htmlUrl: null, assetUrl: null, assetSha256: null },
    'tag without "v", no date, no link, no assets');
  assert.strictEqual(parseRelease({ tag_name: 'v1.3.0', assets: [{ name: 'x.deb', browser_download_url: 'u', digest: 'sha256:' + 'A'.repeat(64) }] }).assetSha256,
    'a'.repeat(64), 'uppercase digest — lowered to lowercase');

  const all = ['v1.12.0', 'v1.11.1', 'v1.11.0', 'v1.10.0', 'v1.9.0', 'v1.8.1', 'v1.8.0', 'v1.0.0-14'].map((t) => rel(t));
  assert.deepStrictEqual(rollbackCandidates(all, '1.11.1-1').map((r) => r.version), ['1.11.0', '1.10.0', '1.9.0', '1.8.1', '1.8.0'],
    'older than installed, newest first, no more than 5');
  assert.deepStrictEqual(rollbackCandidates(all, '1.10.0-1', 10).map((r) => r.version), ['1.9.0', '1.8.1', '1.8.0', '1.0.0-14'], 'max');
  assert.deepStrictEqual(rollbackCandidates([rel('v1.9.0', false), rel('v1.8.0')], '1.10.0-1').map((r) => r.version), ['1.8.0'], 'without a .deb — not allowed');
  assert.deepStrictEqual(rollbackCandidates(all, '1.0.0-14'), [], 'nothing older');
  assert.deepStrictEqual(rollbackCandidates(null, '1.0.0-1'), [], 'no list');
  assert.deepStrictEqual(rollbackCandidates([...all].reverse(), '1.9.0-1').map((r) => r.version)[0], '1.8.1', 'order does not matter');
  console.log('OK: parseRelease, rollbackCandidates — all checks passed');
}
