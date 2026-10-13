#!/usr/bin/env node
// Юнит-тесты периодической проверки обновлений (src/updates-logic.js): updatePollMs —
// интервал; announceUpdates — пункты меню и уведомления по найденным версиям.
'use strict';
const assert = require('assert');
const { updatePollMs, announceUpdates } = require('../src/updates-logic');

// ---- updatePollMs ----
assert.strictEqual(updatePollMs({}), 12 * 3600 * 1000, 'по умолчанию — 12 часов');
assert.strictEqual(updatePollMs({ DSH_LAUNCHER_UPDATE_POLL_SEC: '3' }), 3000, 'тесты — секунды');
for (const bad of ['0', '-5', 'abc', '', '0.5']) {
  assert.strictEqual(updatePollMs({ DSH_LAUNCHER_UPDATE_POLL_SEC: bad }), 12 * 3600 * 1000, `«${bad}» — по умолчанию`);
}

// ---- announceUpdates ----
assert.deepStrictEqual(announceUpdates({}, {}, {}), { menu: {}, notify: [] }, 'ничего не найдено');
assert.deepStrictEqual(announceUpdates({ launcher: null, dsh: undefined }, {}, {}), { menu: {}, notify: [] }, 'null — нет обновления');
assert.deepStrictEqual(announceUpdates({ launcher: '1.8.0', dsh: '0.3.0' }, {}, {}),
  { menu: { launcher: '1.8.0', dsh: '0.3.0' }, notify: ['launcher', 'dsh'] }, 'оба — меню и уведомления');
assert.deepStrictEqual(announceUpdates({ launcher: '1.8.0' }, {}, { launcher: '1.8.0' }),
  { menu: { launcher: '1.8.0' }, notify: [] }, 'уже уведомляли — только меню');
assert.deepStrictEqual(announceUpdates({ launcher: '1.9.0' }, {}, { launcher: '1.8.0' }),
  { menu: { launcher: '1.9.0' }, notify: ['launcher'] }, 'вышла ещё более новая — снова уведомить');
assert.deepStrictEqual(announceUpdates({ dsh: '0.3.0' }, { dsh: '0.3.0' }, {}),
  { menu: { dsh: '0.3.0' }, notify: [] }, '«не спрашивать больше» — без уведомления, пункт меню есть');
assert.deepStrictEqual(announceUpdates({ dsh: '0.4.0' }, { dsh: '0.3.0' }, {}),
  { menu: { dsh: '0.4.0' }, notify: ['dsh'] }, 'отклонена старая — о новой уведомить');
assert.deepStrictEqual(announceUpdates({ dsh: '0.3.0' }, null, undefined),
  { menu: { dsh: '0.3.0' }, notify: ['dsh'] }, 'нет файлов — как пустые');

console.log('OK: updatePollMs, announceUpdates — все проверки прошли');

// ---- parseRelease, rollbackCandidates: «Вернуть предыдущую версию…» ----
{
  const { parseRelease, rollbackCandidates } = require('../src/updates-logic');
  const sha = 'a'.repeat(64);
  const rel = (tag, deb = true, digest = 'sha256:' + sha) => parseRelease({
    tag_name: tag, published_at: '2026-10-01T10:00:00Z', html_url: 'https://x/' + tag,
    assets: deb ? [{ name: `dsh-launcher_${tag.slice(1)}-1_amd64.deb`, browser_download_url: 'https://dl/' + tag, digest }] : [{ name: 'notes.txt' }],
  });
  assert.deepStrictEqual(rel('v1.2.0'), { version: '1.2.0', date: '2026-10-01', htmlUrl: 'https://x/v1.2.0', assetUrl: 'https://dl/v1.2.0', assetSha256: sha }, 'разбор релиза');
  assert.strictEqual(rel('v1.0.0', true, '').assetSha256, null, 'старый релиз без digest');
  assert.strictEqual(rel('v1.0.0', false).assetUrl, null, 'без .deb');
  assert.deepStrictEqual(parseRelease({ tag_name: '1.3.0' }), { version: '1.3.0', date: null, htmlUrl: null, assetUrl: null, assetSha256: null },
    'тег без «v», ни даты, ни ссылки, ни ассетов');
  assert.strictEqual(parseRelease({ tag_name: 'v1.3.0', assets: [{ name: 'x.deb', browser_download_url: 'u', digest: 'sha256:' + 'A'.repeat(64) }] }).assetSha256,
    'a'.repeat(64), 'digest в верхнем регистре — приводится к нижнему');

  const all = ['v1.12.0', 'v1.11.1', 'v1.11.0', 'v1.10.0', 'v1.9.0', 'v1.8.1', 'v1.8.0', 'v1.0.0-14'].map((t) => rel(t));
  assert.deepStrictEqual(rollbackCandidates(all, '1.11.1-1').map((r) => r.version), ['1.11.0', '1.10.0', '1.9.0', '1.8.1', '1.8.0'],
    'старше установленной, новые первыми, не больше 5');
  assert.deepStrictEqual(rollbackCandidates(all, '1.10.0-1', 10).map((r) => r.version), ['1.9.0', '1.8.1', '1.8.0', '1.0.0-14'], 'max');
  assert.deepStrictEqual(rollbackCandidates([rel('v1.9.0', false), rel('v1.8.0')], '1.10.0-1').map((r) => r.version), ['1.8.0'], 'без .deb — нельзя');
  assert.deepStrictEqual(rollbackCandidates(all, '1.0.0-14'), [], 'старее нет');
  assert.deepStrictEqual(rollbackCandidates(null, '1.0.0-1'), [], 'нет списка');
  assert.deepStrictEqual(rollbackCandidates([...all].reverse(), '1.9.0-1').map((r) => r.version)[0], '1.8.1', 'порядок не важен');
  console.log('OK: parseRelease, rollbackCandidates — все проверки прошли');
}
