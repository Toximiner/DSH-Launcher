#!/usr/bin/env node
// Unit test of logTail (src/dsh-log.js): reading the tail of a big file.
// logTail — a pure fs function, cut out of dsh-log.js by the marker (the
// module requires electron via dialogs), fs is replaced with a fake.
'use strict';
const assert = require('assert');
const { extract } = require('./_extract');

function makeFs(content) {
  // The file — a string `content`; openSync/fstatSync/readSync/closeSync
  // work over it. fd — always 1 (means "open").
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

// ---- small file: we read it all, no dropping a fragment ----
{
  const t = logTail('small', 300, 256 * 1024);
  assert.strictEqual(t, 'line1\nline2\nline3', 'small file → all lines, tail without padding');
}

// ---- maxLines: we take the last N ----
{
  const many = Array.from({ length: 50 }, (_, i) => 'l' + i).join('\n');
  const t = extract('logTail', { fs: makeFs(many) })('many', 3, 256 * 1024);
  assert.strictEqual(t, 'l47\nl48\nl49', 'maxLines=3 → last 3 lines');
}

// ---- maxBytes: we read only the tail, the first (fragmented) line is dropped ----
{
  const big = Array.from({ length: 10 }, (_, i) => 'row' + i + '-xxxxxxxxxxxxxxxxxxxx\n').join('');
  const fs = makeFs(big);
  const lt = extract('logTail', { fs });
  // 40 bytes = the last 26 (the row9 line in full) + 14 bytes of the row8
  // line (a fragment). The fragment must be dropped → only row9.
  const t = lt('big', 300, 40);
  assert.strictEqual(t, 'row9-xxxxxxxxxxxxxxxxxxxx', 'fragmented first line dropped, trailing padding trimmed');
}

// ---- a missing file → null ----
{
  const bad = {
    openSync: () => { throw new Error('ENOENT'); },
    fstatSync: () => { throw new Error('nope'); },
    readSync: () => { throw new Error('nope'); },
    closeSync: () => {},
  };
  const lt = extract('logTail', { fs: bad });
  assert.strictEqual(lt('missing'), null, 'cannot be opened → null, no throw');
}

console.log('OK: dsh-log — all logTail checks passed');
