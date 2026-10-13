'use strict';

/*
 * Блоки-маркеры «/* ===== имя … ===== *\/» … «/* ===== end имя ===== *\/»
 * (функции, которые тесты извлекают из модулей с electron, test/_extract.js):
 *  - у каждого блока есть пара end, имена не повторяются (extract берёт
 *    первый найденный блок — второй с тем же именем молча не проверялся бы);
 *  - в шапке назван тест («тест test/X.js»), этот файл есть и упоминает имя.
 * Отчёт о покрытии (npm run coverage) такие функции не видит: они
 * выполняются из текста, а не из файла модуля, — эта проверка вместо него.
 *
 * Запуск: node test/markers.js   (npm test)
 */

const assert = require('assert');
const fs = require('fs');
const path = require('path');

const root = path.join(__dirname, '..');
const files = ['main.js', 'release.js', ...fs.readdirSync(path.join(root, 'src')).filter((f) => f.endsWith('.js')).map((f) => `src/${f}`)];

const seen = new Map(); // имя → файл
for (const f of files) {
  const t = fs.readFileSync(path.join(root, f), 'utf8');
  const opens = [...t.matchAll(/\/\* ===== (?!end )(\w+) (.*)$/gm)];
  const ends = [...t.matchAll(/\/\* ===== end (\w+) ===== \*\//g)].map((m) => m[1]);
  for (const [, name, rest] of opens) {
    assert.ok(!seen.has(name), `блок «${name}» — и в ${seen.get(name)}, и в ${f}`);
    seen.set(name, f);
    assert.strictEqual(ends.filter((e) => e === name).length, 1, `${f}: у блока «${name}» нет пары «end ${name}» (или их несколько)`);
    const s = t.indexOf(`/* ===== ${name} `);
    assert.ok(s < t.indexOf(`/* ===== end ${name} ===== */`), `${f}: «end ${name}» раньше начала блока`);
    const test = (rest.match(/тест (test\/[\w-]+\.js)/) || [])[1];
    assert.ok(test, `${f}: в шапке блока «${name}» не назван тест («тест test/X.js»)`);
    assert.ok(fs.existsSync(path.join(root, test)), `${f}: блок «${name}» ссылается на ${test} — такого файла нет`);
    assert.ok(fs.readFileSync(path.join(root, test), 'utf8').includes(name), `${test} не упоминает «${name}» (блок в ${f})`);
  }
  for (const e of ends) assert.ok(opens.some((m) => m[1] === e), `${f}: «end ${e}» без начала блока`);
}
assert.ok(seen.size >= 10, `блоков-маркеров найдено подозрительно мало: ${seen.size}`);

console.log(`OK: markers — ${seen.size} блоков, у каждого есть тест`);
