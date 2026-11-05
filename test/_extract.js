'use strict';
// A common helper for the unit tests: extracts a function from main.js by
// the paired markers "/* ===== name … ===== */" / "/* ===== end name ===== */".
// main.js is not loaded whole (it requires electron).
const fs = require('fs');
const path = require('path');

const mainSrc = fs.readFileSync(path.join(__dirname, '..', 'main.js'), 'utf8');

// we look for the marked blocks in main.js and the src/ modules (the code is moving out of main.js)
const srcDir = path.join(__dirname, '..', 'src');
const sources = [mainSrc, ...fs.readdirSync(srcDir).filter((f) => f.endsWith('.js')).map((f) => fs.readFileSync(path.join(srcDir, f), 'utf8'))];

function extract(name, deps = {}) {
  const text = sources.find((t) => t.includes('/* ===== ' + name + ' ') && t.includes('/* ===== end ' + name + ' ===== */'));
  if (!text) throw new Error('markers not found: ' + name);
  const s = text.indexOf('/* ===== ' + name + ' ');
  const e = text.indexOf('/* ===== end ' + name + ' ===== */');
  const code = text.slice(s, e);
  const names = Object.keys(deps);
  return new Function(...names, code + '\nreturn ' + name + ';')(...Object.values(deps));
}

module.exports = { extract, mainSrc };
