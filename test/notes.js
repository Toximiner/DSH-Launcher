#!/usr/bin/env node
// Юнит-тесты «Что нового» (main.js): releaseNotesLang — часть описания релиза
// на языке окна, notesBetween — какие релизы показать, mdToHtml — безопасный
// markdown (описания приходят из сети).
'use strict';
const assert = require('assert');
const fs = require('fs');
const path = require('path');
const { extract } = require('./_extract');

// Функции сравнения версий — из блока между маркерами «сравнение версий»
const src = fs.readFileSync(path.join(__dirname, '..', 'main.js'), 'utf8');
const a = src.indexOf('сравнение версий');
const b = src.indexOf('/* ===== конец сравнения версий');
const { compareDebianVer, compareSemVer, tagToPackageVersion } = new Function(
  src.slice(src.lastIndexOf('\n', a), b) + ';return { compareDebianVer, compareSemVer, tagToPackageVersion };')();

const esc = extract('esc');
const releaseNotesLang = extract('releaseNotesLang');
const notesBetween = extract('notesBetween', { compareDebianVer, compareSemVer, tagToPackageVersion });
const mdToHtml = extract('mdToHtml', { esc });

// ---- releaseNotesLang: формат описания, который собирает build-deb.yml ----
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
assert.ok(ru.startsWith('### Исправлено') && ru.includes('Debian 12.** Подробности'), 'RU: русская часть');
assert.ok(!/English|Fixed|Full Changelog|^---/m.test(ru), 'RU: без английской части, разделителя и ссылки');
assert.ok(en.startsWith('### Fixed') && en.includes('installs on Debian 12'), 'EN: английская часть');
assert.ok(!/Русский|Исправлено|Full Changelog/.test(en), 'EN: без русской части и ссылки');
assert.strictEqual(releaseNotesLang('### Fixed\n\n- one\n\n**Full Changelog**: x', 'ru'), '### Fixed\n\n- one',
  'нет разметки языков — всё описание без Full Changelog');
assert.strictEqual(releaseNotesLang('## Русский\n\n- только русский', 'en'), '- только русский',
  'нет английской части — русская (лучше, чем ничего)');
assert.strictEqual(releaseNotesLang('', 'ru'), '', 'пустое описание');
assert.strictEqual(releaseNotesLang(null, 'en'), '', 'нет описания');
assert.ok(releaseNotesLang(body.replace(/\n/g, '\r\n'), 'en').startsWith('### Fixed'), 'переводы строк \\r\\n');

// ---- notesBetween: какие релизы показать ----
const rels = ['1.5.1', '1.5.0', '1.4.0', '1.3.1', '1.3.0', '1.2.0'].map((v) => ({ version: v, body: 'b' + v }));
const vers = (x) => x.map((r) => r.version);
assert.deepStrictEqual(vers(notesBetween(rels, '1.3.0-1', '1.5.1', 'deb')), ['1.5.1', '1.5.0', '1.4.0', '1.3.1'],
  'пакет 1.3.0-1 → 1.5.1: все пропущенные версии, новые сверху');
assert.deepStrictEqual(vers(notesBetween(rels, '1.5.0-1', '1.5.1', 'deb')), ['1.5.1'], 'одна версия');
assert.deepStrictEqual(vers(notesBetween(rels, '1.5.1-1', '1.5.1', 'deb')), [], 'уже последняя — пусто');
assert.deepStrictEqual(vers(notesBetween(rels, '1.3.0-1', '1.4.0', 'deb')), ['1.4.0', '1.3.1'],
  'не новее предлагаемой (подменённая «последняя»)');
assert.deepStrictEqual(vers(notesBetween(rels, '1.4.0', '1.5.1', 'semver')), ['1.5.1', '1.5.0'], 'сборка из исходников (semver)');
assert.deepStrictEqual(vers(notesBetween(rels, null, '1.5.1', 'deb')), ['1.5.1'], 'установленная неизвестна — только новая');
assert.deepStrictEqual(vers(notesBetween([...rels].reverse(), '1.4.0-1', '1.5.1', 'deb')), ['1.5.1', '1.5.0'],
  'порядок не зависит от порядка в ответе');
assert.deepStrictEqual(vers(notesBetween(undefined, '1.0.0-1', '1.5.1', 'deb')), [], 'нет списка релизов');
// перепаковка той же версии: тег v1.5.1-2 новее пакета 1.5.1-1
assert.deepStrictEqual(vers(notesBetween([{ version: '1.5.1-2', body: '' }, ...rels], '1.5.1-1', '1.5.1-2', 'deb')), ['1.5.1-2'],
  'перепаковка (ревизия пакета)');

// ---- mdToHtml ----
assert.strictEqual(mdToHtml('### Fixed'), '<h4>Fixed</h4>', 'заголовок');
assert.strictEqual(mdToHtml('- one\n- two'), '<ul><li>one</li><li>two</li></ul>', 'список');
assert.strictEqual(mdToHtml('- **Bold** item\n  continued here\n- next'),
  '<ul><li><b>Bold</b> item continued here</li><li>next</li></ul>', 'перенос строки внутри пункта');
assert.strictEqual(mdToHtml('use `npm i -g x` now'), '<p>use <code>npm i -g x</code> now</p>', 'код');
assert.strictEqual(mdToHtml('see [docs](https://example.com/a?b=1)'),
  '<p>see <a href="https://example.com/a?b=1" target="_blank">docs</a></p>', 'ссылка https');
assert.strictEqual(mdToHtml('[x](javascript:alert(1))'), '<p>[x](javascript:alert(1))</p>', 'javascript:-ссылка не становится ссылкой');
assert.strictEqual(mdToHtml('para one\nstill one\n\npara two'), '<p>para one still one</p><p>para two</p>', 'абзацы');
assert.strictEqual(mdToHtml('### A\n- x\n\n### B\n- y'), '<h4>A</h4><ul><li>x</li></ul><h4>B</h4><ul><li>y</li></ul>', 'несколько разделов');
// HTML из сети не исполняется
const evil = mdToHtml('<img src=x onerror=alert(1)> **<script>alert(2)</script>** `<b>` [<i>t</i>](https://e.com/"onmouseover=x)');
assert.ok(!/<img|<script|<i>|<b>&lt;|onerror=alert\(1\)>/.test(evil.replace(/<b>&lt;script/g, '')), 'теги экранированы: ' + evil);
assert.ok(!evil.includes('"onmouseover'), 'кавычка в адресе ссылки не ломает атрибут: ' + evil);
assert.ok(evil.includes('&lt;img') && evil.includes('&lt;script&gt;'), 'экранирование видно как текст');
assert.strictEqual(mdToHtml(''), '', 'пусто');

// Сквозное: настоящий формат описания → HTML на языке окна
const html = mdToHtml(releaseNotesLang(body, 'ru'));
assert.strictEqual(html, '<h4>Исправлено</h4><ul><li><b>Пакет ставится на Debian 12.</b> Подробности на второй строке.</li></ul>',
  'описание релиза → HTML');

// ---- redactLog: токены и ключи в журнале dsh ----
const redactLog = extract('redactLog');
const log = [
  'dsh web: http://127.0.0.1:3080/?token=DviRWPYv5ugGXm7OJ2ag20onq_V12KvnZBKeeW858OI',
  '[dsh] dsh web: http://127.0.0.1:3080/?a=1&token=abc#x',
  'Authorization: Bearer abcdefgh12345678',
  'using key sk-proj-AbCdEfGhIjKlMnOpQrSt and gho_abcdefghijklmnopqrstuvwxyz0123',
  'config: {"api_key": "s3cr3t-value", password=hunter2, apiKey: xyz}',
  '\x1b[32mok\x1b[0m plain line stays',
].join('\n');
const r = redactLog(log);
assert.ok(!/DviRWPYv5|token=abc|abcdefgh12345678|AbCdEfGhIjKl|gho_abcdefghij|s3cr3t|hunter2|apiKey: xyz/.test(r), 'секреты скрыты:\n' + r);
assert.ok(r.includes('/?token=***') && r.includes('&token=***#x'), 'токен в URL → ***');
assert.ok(r.includes('Bearer ***') && r.includes('sk-***') && r.includes('gho_***'), 'Bearer, sk-, gho_ → ***');
assert.ok(r.includes('"api_key": "***"') && r.includes('password=***') && r.includes('apiKey: ***'), 'ключи в конфиге → ***');
assert.ok(r.includes('ok plain line stays') && !r.includes('\x1b'), 'цвета ANSI убраны, обычный текст цел');
assert.strictEqual(redactLog('dsh: warning: 1 entry did not activate'), 'dsh: warning: 1 entry did not activate', 'обычная строка без изменений');
assert.strictEqual(redactLog(null), '', 'нет журнала');

// ---- shouldShowWhatsNew: показать «Что нового» после обновления ----
const shouldShowWhatsNew = extract('shouldShowWhatsNew', { compareDebianVer, compareSemVer });
assert.strictEqual(shouldShowWhatsNew('1.5.1-1', '1.6.0-1', 'deb'), true, 'пакет обновился');
assert.strictEqual(shouldShowWhatsNew('1.6.0-1', '1.6.0-1', 'deb'), false, 'та же версия');
assert.strictEqual(shouldShowWhatsNew('1.6.0-1', '1.5.1-1', 'deb'), false, 'откат — не показываем');
assert.strictEqual(shouldShowWhatsNew('1.6.0-1', '1.6.0-2', 'deb'), true, 'перепаковка');
assert.strictEqual(shouldShowWhatsNew(null, '1.6.0-1', 'deb'), false, 'первый запуск — не показываем');
assert.strictEqual(shouldShowWhatsNew('1.5.1', '1.6.0', 'semver'), true, 'сборка из исходников');
assert.strictEqual(shouldShowWhatsNew('1.5.1', null, 'semver'), false, 'версия неизвестна');

// ---- releasesCacheMode: кэш списка релизов GitHub ----
const mode = extract('releasesCacheMode');
const H = 3600e3; const now = 1e12;
const e = (o) => ({ list: [], etag: 'W/"x"', fetchedAt: now - 10 * 60e3, ...o });
assert.strictEqual(mode(null, now, H, false), 'fetch', 'кэша нет → запрос');
assert.strictEqual(mode({ etag: 'x' }, now, H, false), 'fetch', 'битый кэш → запрос');
assert.strictEqual(mode(e(), now, H, false), 'cache', 'проверяли 10 мин назад → без запроса');
assert.strictEqual(mode(e({ fetchedAt: now - 2 * H }), now, H, false), 'revalidate', 'давно → с If-None-Match');
assert.strictEqual(mode(e(), now, H, true), 'revalidate', 'ручная проверка — всегда спросить (с меткой)');
assert.strictEqual(mode(e({ etag: null, fetchedAt: now - 2 * H }), now, H, false), 'fetch', 'метки нет → обычный запрос');
assert.strictEqual(mode(e({ fetchedAt: now + 5 * 60e3 }), now, H, false), 'revalidate', 'время из будущего (часы сдвинули) → спросить');

console.log('OK: releaseNotesLang, notesBetween, mdToHtml, redactLog, shouldShowWhatsNew, releasesCacheMode — все проверки прошли');
