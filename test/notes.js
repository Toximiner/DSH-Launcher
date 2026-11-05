#!/usr/bin/env node
// Unit tests for "What's new" (src/notes.js, src/versions.js): releaseNotesLang — the
// part of the release description in the window's language, notesBetween — which
// releases to show, mdToHtml — safe markdown (descriptions come from the network).
'use strict';
const assert = require('assert');
const { notesBetween, shouldShowWhatsNew } = require('../src/versions');
const { redactLog } = require('../src/util');
const { releaseNotesLang, mdToHtml, dshNotesPart } = require('../src/notes');
const { releasesCacheMode: mode, releasesCacheFresh: fresh } = require('../src/updates-logic');


// ---- releaseNotesLang: the description format assembled by build-deb.yml ----
const body = `## Русский

### Исправлено

- **Пакет ставится на Debian 12.** Подробности
  на второй строке.

---

## English

### Fixed

- **The package installs on Debian 12.** Details
  on the second line.

**Full Changelog**: https://github.com/Toximiner/DSH-Launcher/compare/v1.5.0...v1.5.1
`;
const ru = releaseNotesLang(body, 'ru');
const en = releaseNotesLang(body, 'en');
assert.ok(ru.startsWith('### Исправлено') && ru.includes('Debian 12.** Подробности'), 'RU: the Russian part');
assert.ok(!/English|Fixed|Full Changelog|^---/m.test(ru), 'RU: no English part, separator or link');
assert.ok(en.startsWith('### Fixed') && en.includes('installs on Debian 12'), 'EN: the English part');
assert.ok(!/Русский|Исправлено|Full Changelog/.test(en), 'EN: no Russian part and no link');
assert.strictEqual(releaseNotesLang('### Fixed\n\n- one\n\n**Full Changelog**: x', 'ru'), '### Fixed\n\n- one',
  'no language markers — the whole description without Full Changelog');
assert.strictEqual(releaseNotesLang('## Русский\n\n- только русский', 'en'), '- только русский',
  'no English part — Russian (better than nothing)');
assert.strictEqual(releaseNotesLang('', 'ru'), '', 'empty description');
assert.strictEqual(releaseNotesLang(null, 'en'), '', 'no description');
assert.ok(releaseNotesLang(body.replace(/\n/g, '\r\n'), 'en').startsWith('### Fixed'), 'line endings \\r\\n');

// ---- dshNotesPart: EN part of a bilingual dsh release (GitHub, deepseek-harness) ----
const dshBody = `[中文](#cn-x) | [English](#en-x)

<h3 id="cn-x">新增功能</h3>

- Новая функция на китайском. @user

<h3 id="en-x">New Features</h3>

- New feature in English. by @user
- Second EN item.

Full Changelog: [dsh-v1...dsh-v2](https://github.com/deepseek-ai/deepseek-harness/compare/a...b)`;
const dsh = dshNotesPart(dshBody);
assert.ok(dsh.startsWith('## New Features'), 'the <h3 id="en-…"> heading becomes ##');
assert.ok(dsh.includes('- New feature in English.') && dsh.includes('- Second EN item.'), 'the EN items are in place');
assert.ok(!dsh.includes('新增功能') && !dsh.includes('китайском'), 'the CN part is dropped');
assert.ok(!/Full Changelog/.test(dsh), 'the Full Changelog line is dropped');
assert.ok(!/<h3 id=/.test(dsh), 'no HTML anchors remain');
assert.strictEqual(dshNotesPart('just a plain body'), 'just a plain body', 'no anchors — the whole description');
assert.strictEqual(dshNotesPart(null), '', 'no description');
assert.strictEqual(dshNotesPart('<h3 id="en-only">Fixes</h3>\n- one\n\nFull Changelog: x'),
  '## Fixes\n- one', 'an EN anchor at the start — a heading, then the items');

// ---- notesBetween: which releases to show ----
const rels = ['1.5.1', '1.5.0', '1.4.0', '1.3.1', '1.3.0', '1.2.0'].map((v) => ({ version: v, body: 'b' + v }));
const vers = (x) => x.map((r) => r.version);
assert.deepStrictEqual(vers(notesBetween(rels, '1.3.0-1', '1.5.1', 'deb')), ['1.5.1', '1.5.0', '1.4.0', '1.3.1'],
  'package 1.3.0-1 → 1.5.1: all skipped versions, newest on top');
assert.deepStrictEqual(vers(notesBetween(rels, '1.5.0-1', '1.5.1', 'deb')), ['1.5.1'], 'one version');
assert.deepStrictEqual(vers(notesBetween(rels, '1.5.1-1', '1.5.1', 'deb')), [], 'already the latest — empty');
assert.deepStrictEqual(vers(notesBetween(rels, '1.3.0-1', '1.4.0', 'deb')), ['1.4.0', '1.3.1'],
  'not newer than the offered one (a spoofed "latest")');
assert.deepStrictEqual(vers(notesBetween(rels, '1.4.0', '1.5.1', 'semver')), ['1.5.1', '1.5.0'], 'a source build (semver)');
assert.deepStrictEqual(vers(notesBetween(rels, null, '1.5.1', 'deb')), ['1.5.1'], 'installed version unknown — only the new one');
assert.deepStrictEqual(vers(notesBetween([...rels].reverse(), '1.4.0-1', '1.5.1', 'deb')), ['1.5.1', '1.5.0'],
  'the order does not depend on the order in the response');
assert.deepStrictEqual(vers(notesBetween(undefined, '1.0.0-1', '1.5.1', 'deb')), [], 'no release list');
// repackaging of the same version: tag v1.5.1-2 is newer than package 1.5.1-1
assert.deepStrictEqual(vers(notesBetween([{ version: '1.5.1-2', body: '' }, ...rels], '1.5.1-1', '1.5.1-2', 'deb')), ['1.5.1-2'],
  'a repackaging (package revision)');

// ---- mdToHtml ----
assert.strictEqual(mdToHtml('### Fixed'), '<h4>Fixed</h4>', 'a heading');
assert.strictEqual(mdToHtml('- one\n- two'), '<ul><li>one</li><li>two</li></ul>', 'a list');
assert.strictEqual(mdToHtml('Intro\n- a\n- b\nafter'), '<p>Intro</p><ul><li>a</li><li>b</li></ul><p>after</p>',
  'a list right after a paragraph and a paragraph right after a list — no blank lines');
assert.strictEqual(mdToHtml('- **Bold** item\n  continued here\n- next'),
  '<ul><li><b>Bold</b> item continued here</li><li>next</li></ul>', 'a line break inside an item');
assert.strictEqual(mdToHtml('use `npm i -g x` now'), '<p>use <code>npm i -g x</code> now</p>', 'code');
assert.strictEqual(mdToHtml('see [docs](https://example.com/a?b=1)'),
  '<p>see <a href="https://example.com/a?b=1" target="_blank">docs</a></p>', 'an https link');
assert.strictEqual(mdToHtml('[x](javascript:alert(1))'), '<p>[x](javascript:alert(1))</p>', 'a javascript: link does not become a link');
assert.strictEqual(mdToHtml('para one\nstill one\n\npara two'), '<p>para one still one</p><p>para two</p>', 'paragraphs');
assert.strictEqual(mdToHtml('### A\n- x\n\n### B\n- y'), '<h4>A</h4><ul><li>x</li></ul><h4>B</h4><ul><li>y</li></ul>', 'several sections');
// HTML from the network is not executed
const evil = mdToHtml('<img src=x onerror=alert(1)> **<script>alert(2)</script>** `<b>` [<i>t</i>](https://e.com/"onmouseover=x)');
assert.ok(!/<img|<script|<i>|<b>&lt;|onerror=alert\(1\)>/.test(evil.replace(/<b>&lt;script/g, '')), 'tags are escaped: ' + evil);
assert.ok(!evil.includes('"onmouseover'), 'a quote in the link address does not break the attribute: ' + evil);
assert.ok(evil.includes('&lt;img') && evil.includes('&lt;script&gt;'), 'the escaping is visible as text');
assert.strictEqual(mdToHtml(''), '', 'empty');

// End-to-end: real description format → HTML in the window's language
const html = mdToHtml(releaseNotesLang(body, 'ru'));
assert.strictEqual(html, '<h4>Исправлено</h4><ul><li><b>Пакет ставится на Debian 12.</b> Подробности на второй строке.</li></ul>',
  'release description → HTML');

// ---- redactLog: tokens and keys in the dsh log ----
const log = [
  'dsh web: http://127.0.0.1:3080/?token=DviRWPYv5ugGXm7OJ2ag20onq_V12KvnZBKeeW858OI',
  '[dsh] dsh web: http://127.0.0.1:3080/?a=1&token=abc#x',
  'Authorization: Bearer abcdefgh12345678',
  'using key sk-proj-AbCdEfGhIjKlMnOpQrSt and gho_abcdefghijklmnopqrstuvwxyz0123',
  'config: {"api_key": "s3cr3t-value", password=hunter2, apiKey: xyz}',
  '\x1b[32mok\x1b[0m plain line stays',
].join('\n');
const r = redactLog(log);
assert.ok(!/DviRWPYv5|token=abc|abcdefgh12345678|AbCdEfGhIjKl|gho_abcdefghij|s3cr3t|hunter2|apiKey: xyz/.test(r), 'secrets are hidden:\n' + r);
assert.ok(r.includes('/?token=***') && r.includes('&token=***#x'), 'the token in the URL → ***');
assert.ok(r.includes('Bearer ***') && r.includes('sk-***') && r.includes('gho_***'), 'Bearer, sk-, gho_ → ***');
assert.ok(r.includes('"api_key": "***"') && r.includes('password=***') && r.includes('apiKey: ***'), 'keys in the config → ***');
assert.ok(r.includes('ok plain line stays') && !r.includes('\x1b'), 'ANSI colors stripped, plain text intact');
assert.strictEqual(redactLog('dsh: warning: 1 entry did not activate'), 'dsh: warning: 1 entry did not activate', 'an ordinary line unchanged');
assert.strictEqual(redactLog(null), '', 'no log');

// ---- shouldShowWhatsNew: show "What's new" after an update ----
assert.strictEqual(shouldShowWhatsNew('1.5.1-1', '1.6.0-1', 'deb'), true, 'the package was updated');
assert.strictEqual(shouldShowWhatsNew('1.6.0-1', '1.6.0-1', 'deb'), false, 'the same version');
assert.strictEqual(shouldShowWhatsNew('1.6.0-1', '1.5.1-1', 'deb'), false, 'a rollback — do not show');
assert.strictEqual(shouldShowWhatsNew('1.6.0-1', '1.6.0-2', 'deb'), true, 'a repackaging');
assert.strictEqual(shouldShowWhatsNew(null, '1.6.0-1', 'deb'), false, 'the first run — do not show');
assert.strictEqual(shouldShowWhatsNew('1.5.1', '1.6.0', 'semver'), true, 'a source build');
assert.strictEqual(shouldShowWhatsNew('1.5.1', null, 'semver'), false, 'the version is unknown');

// ---- releasesCacheMode: cache of the GitHub release list ----
const H = 3600e3; const now = 1e12;
const e = (o) => ({ list: [], etag: 'W/"x"', fetchedAt: now - 10 * 60e3, ...o });
assert.strictEqual(mode(null, now, H, false), 'fetch', 'no cache → fetch');
assert.strictEqual(mode({ etag: 'x' }, now, H, false), 'fetch', 'a broken cache → fetch');
assert.strictEqual(mode(e(), now, H, false), 'revalidate', 'an etag is present → always ask with If-None-Match (304 is free), even after 10 min');
assert.strictEqual(mode(e({ fetchedAt: now - 2 * H }), now, H, false), 'revalidate', 'stale → with If-None-Match');
assert.strictEqual(mode(e(), now, H, true), 'revalidate', 'a manual check — with the etag');
assert.strictEqual(mode(e({ etag: null }), now, H, false), 'cache', 'no etag, checked 10 min ago → no request');
assert.strictEqual(mode(e({ etag: null }), now, H, true), 'fetch', 'no etag, a manual check → fetch');
assert.strictEqual(mode(e({ etag: null, fetchedAt: now - 2 * H }), now, H, false), 'fetch', 'no etag, stale → a plain fetch');
assert.strictEqual(mode(e({ etag: null, fetchedAt: now + 5 * 60e3 }), now, H, false), 'fetch', 'time in the future (the clock moved) → fetch');
assert.strictEqual(fresh(e(), now, H), true, 'a fresh cache (a fallback on GitHub failure)');
assert.strictEqual(fresh(e({ fetchedAt: now - 2 * H }), now, H), false, 'stale — not a fallback');
assert.strictEqual(fresh(e({ fetchedAt: undefined }), now, H), false, 'no timestamp — not fresh');
assert.strictEqual(fresh(null, now, H), false, 'no cache');

console.log('OK: releaseNotesLang, dshNotesPart, notesBetween, mdToHtml, redactLog, shouldShowWhatsNew, releasesCacheMode, releasesCacheFresh — all checks passed');
