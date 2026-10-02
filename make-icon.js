'use strict';

/* Генерирует icon.png (256×256) без внешних зависимостей: сырой PNG + zlib.
 * Тёмный скруглённый квадрат с буквой «D» в сине-фиолетовом градиенте,
 * сглаживание — 2×2 суперсэмплинг. */

const fs = require('fs');
const path = require('path');
const zlib = require('zlib');

const W = 256, H = 256, S = 2;
const BW = W * S, BH = H * S;

const idx = (x, y) => (y * BW + x) * 4;

function inRoundRect(x, y, x0, y0, x1, y1, r) {
  if (x < x0 || x > x1 || y < y0 || y > y1) return false;
  const cx = Math.min(Math.max(x, x0 + r), x1 - r);
  const cy = Math.min(Math.max(y, y0 + r), y1 - r);
  const dx = x - cx, dy = y - cy;
  return dx * dx + dy * dy <= r * r;
}

function inD(x, y) {
  // вертикальная перекладина
  if (x >= 82 && x <= 110 && y >= 64 && y <= 192) return true;
  // «чаша»: полукольцо справа, центр (110, 128)
  const dx = x - 110, dy = y - 128;
  if (dx < 0) return false;
  const d2 = dx * dx + dy * dy;
  return d2 >= 36 * 36 && d2 <= 64 * 64;
}

const big = Buffer.alloc(BW * BH * 4);
for (let Y = 0; Y < BH; Y++) {
  for (let X = 0; X < BW; X++) {
    const x = (X + 0.5) / S;
    const y = (Y + 0.5) / S;
    let r = 0, g = 0, b = 0, a = 0;
    if (inRoundRect(x, y, 8, 8, 248, 248, 52)) {
      const t = (x + y) / 510;
      r = 38 + (13 - 38) * t;
      g = 47 + (17 - 47) * t;
      b = 66 + (25 - 66) * t;
      a = 255;
      if (inD(x, y)) {
        const tt = Math.min(1, Math.max(0, (y - 64) / (192 - 64)));
        r = 106 + (139 - 106) * tt;
        g = 164 + (92 - 164) * tt;
        b = 255 + (246 - 255) * tt;
      }
    }
    const i = idx(X, Y);
    big[i] = r; big[i + 1] = g; big[i + 2] = b; big[i + 3] = a;
  }
}

// даунсэмплинг 2×2 (в среднем — по premultiplied-компонентам)
const out = Buffer.alloc(W * H * 4);
for (let y = 0; y < H; y++) {
  for (let x = 0; x < W; x++) {
    let sr = 0, sg = 0, sb = 0, sa = 0;
    for (let dy = 0; dy < S; dy++) {
      for (let dx = 0; dx < S; dx++) {
        const i = idx(x * S + dx, y * S + dy);
        const a = big[i + 3] / 255;
        sr += big[i] * a;
        sg += big[i + 1] * a;
        sb += big[i + 2] * a;
        sa += a;
      }
    }
    const o = (y * W + x) * 4;
    if (sa > 0) {
      out[o] = Math.round(sr / sa);
      out[o + 1] = Math.round(sg / sa);
      out[o + 2] = Math.round(sb / sa);
    }
    out[o + 3] = Math.round((sa / (S * S)) * 255);
  }
}

/* ---------- минимальный PNG-энкодер ---------- */

let CRC_TABLE;
function crc32(buf) {
  if (!CRC_TABLE) {
    CRC_TABLE = new Int32Array(256);
    for (let n = 0; n < 256; n++) {
      let c = n;
      for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
      CRC_TABLE[n] = c;
    }
  }
  let crc = -1;
  for (let i = 0; i < buf.length; i++) crc = (crc >>> 8) ^ CRC_TABLE[(crc ^ buf[i]) & 0xff];
  return (crc ^ -1) >>> 0;
}

function chunk(type, data) {
  const len = Buffer.alloc(4);
  len.writeUInt32BE(data.length);
  const t = Buffer.from(type, 'ascii');
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(Buffer.concat([t, data])));
  return Buffer.concat([len, t, data, crc]);
}

function encodePng(w, h, rgba) {
  const sig = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0);
  ihdr.writeUInt32BE(h, 4);
  ihdr[8] = 8; // bit depth
  ihdr[9] = 6; // color type: RGBA
  const raw = Buffer.alloc((w * 4 + 1) * h);
  for (let y = 0; y < h; y++) {
    raw[y * (w * 4 + 1)] = 0; // filter: none
    rgba.copy(raw, y * (w * 4 + 1) + 1, y * w * 4, (y + 1) * w * 4);
  }
  const idat = zlib.deflateSync(raw, { level: 9 });
  return Buffer.concat([sig, chunk('IHDR', ihdr), chunk('IDAT', idat), chunk('IEND', Buffer.alloc(0))]);
}

const target = path.join(__dirname, 'icon.png');
fs.writeFileSync(target, encodePng(W, H, out));
console.log('icon written:', target);
