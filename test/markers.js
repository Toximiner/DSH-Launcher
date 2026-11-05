'use strict';

/*
 * Marker blocks «/* ===== name … ===== *\/» … «/* ===== end name ===== *\/»
 * (functions that tests extract from electron-requiring modules,
 * test/_extract.js):
 *  - every block has exactly one matching end, names are unique (extract
 *    takes the first block found — a second one with the same name would
 *    silently go untested);
 *  - the header names the test file («test test/X.js»), that file exists
 *    and mentions the block name.
 * The coverage report (npm run coverage) does not see these functions: they
 * run from text, not from the module file — this check stands in for it.
 *
 * Run: node test/markers.js   (npm test)
 */

const assert = require('assert');
const fs = require('fs');
const path = require('path');

const root = path.join(__dirname, '..');
const files = ['main.js', 'release.js', ...fs.readdirSync(path.join(root, 'src')).filter((f) => f.endsWith('.js')).map((f) => `src/${f}`)];

const seen = new Map(); // name → file
for (const f of files) {
  const t = fs.readFileSync(path.join(root, f), 'utf8');
  const opens = [...t.matchAll(/\/\* ===== (?!end )(\w+) (.*)$/gm)];
  const ends = [...t.matchAll(/\/\* ===== end (\w+) ===== \*\//g)].map((m) => m[1]);
  for (const [, name, rest] of opens) {
    assert.ok(!seen.has(name), `block "${name}" — in both ${seen.get(name)} and ${f}`);
    seen.set(name, f);
    assert.strictEqual(ends.filter((e) => e === name).length, 1, `${f}: block "${name}" has no "end ${name}" pair (or several)`);
    const s = t.indexOf(`/* ===== ${name} `);
    assert.ok(s < t.indexOf(`/* ===== end ${name} ===== */`), `${f}: "end ${name}" comes before the block start`);
    const test = (rest.match(/test (test\/[\w-]+\.js)/) || [])[1];
    assert.ok(test, `${f}: block "${name}" header must name the test file ("test test/X.js")`);
    assert.ok(fs.existsSync(path.join(root, test)), `${f}: block "${name}" references ${test} — no such file`);
    assert.ok(fs.readFileSync(path.join(root, test), 'utf8').includes(name), `${test} does not mention "${name}" (block in ${f})`);
  }
  for (const e of ends) assert.ok(opens.some((m) => m[1] === e), `${f}: "end ${e}" without a block start`);
}
assert.ok(seen.size >= 10, `suspiciously few marker blocks found: ${seen.size}`);

console.log(`OK: markers — ${seen.size} blocks, each has a test`);
