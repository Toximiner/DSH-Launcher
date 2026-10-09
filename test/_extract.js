'use strict';
// Общий помощник для юнит-тестов: извлекает функцию из main.js по парным
// маркерам «/* ===== имя … ===== */» / «/* ===== end имя ===== */».
// main.js целиком не загружается (он требует electron).
const fs = require('fs');
const path = require('path');

const mainSrc = fs.readFileSync(path.join(__dirname, '..', 'main.js'), 'utf8');

function extract(name, deps = {}) {
  const s = mainSrc.indexOf('/* ===== ' + name + ' ');
  const e = mainSrc.indexOf('/* ===== end ' + name + ' ===== */');
  if (!(s >= 0 && e > s)) throw new Error('маркеры не найдены: ' + name);
  const code = mainSrc.slice(s, e);
  const names = Object.keys(deps);
  return new Function(...names, code + '\nreturn ' + name + ';')(...Object.values(deps));
}

module.exports = { extract, mainSrc };
