'use strict';

/*
 * Реестр launcher (src/launcher.js): каждое имя launcher.X, которым
 * пользуются модули src/, зарегистрировано в main.js (Object.defineProperties
 * между «реестр launcher» и «конец реестра»), а зарегистрированное — где-то
 * используется. Без этой проверки забытое имя всплыло бы только при запуске.
 *
 * Запуск: node test/registry.js   (npm test)
 */

const assert = require('assert');
const fs = require('fs');
const path = require('path');
const { mainSrc } = require('./_extract');

const start = mainSrc.indexOf('// ---- реестр launcher');
const end = mainSrc.indexOf('// ---- конец реестра ----');
assert.ok(start >= 0 && end > start, 'в main.js есть блок реестра');
const registered = new Set([...mainSrc.slice(start, end).matchAll(/^\s+(\w+): \{ get: \(\) => (\w+), enumerable: true \},$/gm)]
  .map((m) => { assert.strictEqual(m[1], m[2], `реестр: ${m[1]} → ${m[2]}`); return m[1]; }));
assert.ok(registered.size > 0, 'реестр разобран');

// без комментариев и обычных строк: в них «launcher.log» … Шаблонные строки
// не трогаем — в ${…} код; «dsh-launcher.deb» в них отсекает (?<!-) ниже.
const strip = (t) => t
  .replace(/\/\*[\s\S]*?\*\//g, '')
  .replace(/'(?:\\.|[^'\\\n])*'|"(?:\\.|[^"\\\n])*"/g, "''")
  .replace(/\/\/.*$/gm, '');

const srcDir = path.join(__dirname, '..', 'src');
const used = new Map(); // имя → файлы
for (const f of fs.readdirSync(srcDir).filter((n) => n.endsWith('.js'))) {
  const t = fs.readFileSync(path.join(srcDir, f), 'utf8');
  if (!/const launcher = require\('\.\/launcher'\)/.test(t)) continue;
  for (const m of strip(t).matchAll(/(?<![\w.$-])launcher\.(\w+)/g)) {
    if (!used.has(m[1])) used.set(m[1], new Set());
    used.get(m[1]).add(f);
  }
}
assert.ok(used.size > 0, 'модули пользуются реестром');

const missing = [...used.keys()].filter((n) => !registered.has(n));
assert.deepStrictEqual(missing, [], 'не зарегистрированы в main.js: ' + missing.map((n) => `${n} (${[...used.get(n)].join(', ')})`).join('; '));
const unused = [...registered].filter((n) => !used.has(n));
assert.deepStrictEqual(unused, [], 'зарегистрированы, но не используются: ' + unused.join(', '));

console.log(`OK: registry — ${registered.size} имён, все на месте`);
