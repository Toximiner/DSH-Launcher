'use strict';

/*
 * The launcher registry (src/launcher.js): every launcher.X name used by the
 * src/ modules is registered in main.js (the Object.defineProperties block
 * between "launcher registry" and "end registry"), and every registered name
 * is used somewhere. Without this check a forgotten name would surface only
 * at runtime.
 *
 * Run: node test/registry.js   (npm test)
 */

const assert = require('assert');
const fs = require('fs');
const path = require('path');
const { mainSrc } = require('./_extract');

const start = mainSrc.indexOf('// ---- launcher registry');
const end = mainSrc.indexOf('// ---- end registry ----');
assert.ok(start >= 0 && end > start, 'main.js has the registry block');
const registered = new Set([...mainSrc.slice(start, end).matchAll(/^\s+(\w+): \{ get: \(\) => (\w+), enumerable: true \},$/gm)]
  .map((m) => { assert.strictEqual(m[1], m[2], `registry: ${m[1]} → ${m[2]}`); return m[1]; }));
assert.ok(registered.size > 0, 'the registry was parsed');

// strip comments and plain strings: they contain "launcher.log" … Template
// literals are left alone — ${…} holds code; "dsh-launcher.deb" in them is
// cut off by the (?<!-) lookbehind below.
const strip = (t) => t
  .replace(/\/\*[\s\S]*?\*\//g, '')
  .replace(/'(?:\\.|[^'\\\n])*'|"(?:\\.|[^"\\\n])*"/g, "''")
  .replace(/\/\/.*$/gm, '');

const srcDir = path.join(__dirname, '..', 'src');
const used = new Map(); // name → files
for (const f of fs.readdirSync(srcDir).filter((n) => n.endsWith('.js'))) {
  const t = fs.readFileSync(path.join(srcDir, f), 'utf8');
  if (!/const launcher = require\('\.\/launcher'\)/.test(t)) continue;
  for (const m of strip(t).matchAll(/(?<![\w.$-])launcher\.(\w+)/g)) {
    if (!used.has(m[1])) used.set(m[1], new Set());
    used.get(m[1]).add(f);
  }
}
assert.ok(used.size > 0, 'modules use the registry');

const missing = [...used.keys()].filter((n) => !registered.has(n));
assert.deepStrictEqual(missing, [], 'not registered in main.js: ' + missing.map((n) => `${n} (${[...used.get(n)].join(', ')})`).join('; '));
const unused = [...registered].filter((n) => !used.has(n));
assert.deepStrictEqual(unused, [], 'registered but not used: ' + unused.join(', '));

console.log(`OK: registry — ${registered.size} names, all in place`);
