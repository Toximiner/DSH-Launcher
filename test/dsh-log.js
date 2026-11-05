#!/usr/bin/env node
// Юнит-тест logTail (src/dsh-log.js): чтение хвоста большого файла.
// logTail — чистая fs-функция, вырезается из dsh-log.js маркером (модуль
// требует electron через dialogs), fs заменяется на фейк.
'use strict';
const assert = require('assert');
const { extract } = require('./_extract');

function makeFs(content) {
  // Файл — строка `content`; openSync/fstatSync/readSync/closeSync работают
  // над ней. fd — всегда 1 (значит «открыт»).
  return {
    openSync: (file) => 1,
    fstatSync: () => ({ size: Buffer.byteLength(content) }),
    readSync: (fd, buf, off, len, pos) => {
      const slice = content.slice(pos, pos + len);
      buf.write(slice, off, 'utf8');
      return slice.length;
    },
    closeSync: () => {},
  };
}

const logTail = extract('logTail', { fs: makeFs('line1\nline2\nline3\n') });

// ---- малый файл: всё читаем, без отброса обрывка ----
{
  const t = logTail('small', 300, 256 * 1024);
  assert.strictEqual(t, 'line1\nline2\nline3', 'малый файл → все строки, хвост без пробелов');
}

// ---- maxLines: берём последние N ----
{
  const many = Array.from({ length: 50 }, (_, i) => 'l' + i).join('\n');
  const t = extract('logTail', { fs: makeFs(many) })('many', 3, 256 * 1024);
  assert.strictEqual(t, 'l47\nl48\nl49', 'maxLines=3 → последние 3 строки');
}

// ---- maxBytes: читаем только хвост, первая (обрывочная) строка отбрасывается ----
{
  const big = Array.from({ length: 10 }, (_, i) => 'row' + i + '-xxxxxxxxxxxxxxxxxxxx\n').join('');
  const fs = makeFs(big);
  const lt = extract('logTail', { fs });
  // 40 байт = последние 26 (строка row9 целиком) + 14 байт строки row8
  // (обрывок). Обрывок должен быть отброшен → только row9.
  const t = lt('big', 300, 40);
  assert.strictEqual(t, 'row9-xxxxxxxxxxxxxxxxxxxx', 'обрывочная первая строка отброшена, хвостовые пробелы срезаны');
}

// ---- отсутствующий файл → null ----
{
  const bad = {
    openSync: () => { throw new Error('ENOENT'); },
    fstatSync: () => { throw new Error('nope'); },
    readSync: () => { throw new Error('nope'); },
    closeSync: () => {},
  };
  const lt = extract('logTail', { fs: bad });
  assert.strictEqual(lt('missing'), null, 'не открывается → null, не бросает');
}

console.log('OK: dsh-log — logTail все проверки прошли');
