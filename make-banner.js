'use strict';

/* Генерирует social-preview.png (1280×640) — картинку для Settings → General →
 * Social preview на GitHub. Запуск: npm run banner (через Electron: нужен
 * рендер текста системными шрифтами, который у make-icon.js не предусмотрен).
 * Значок — тот же, что в make-icon.js (координаты и цвета 1:1), но в SVG,
 * чтобы оставаться чётким в крупном размере. */

const { app, BrowserWindow } = require('electron');
const fs = require('fs');
const path = require('path');

const W = 1280, H = 640;
const OUT = path.join(__dirname, 'social-preview.png');

const ICON_SVG = `
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" width="300" height="300">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="rgb(38,47,66)"/><stop offset="1" stop-color="rgb(13,17,25)"/>
    </linearGradient>
    <linearGradient id="d" gradientUnits="userSpaceOnUse" x1="0" y1="64" x2="0" y2="192">
      <stop offset="0" stop-color="rgb(106,164,255)"/><stop offset="1" stop-color="rgb(139,92,246)"/>
    </linearGradient>
  </defs>
  <rect x="8" y="8" width="240" height="240" rx="52" fill="url(#bg)"/>
  <path fill="url(#d)" d="M82 64H110A64 64 0 0 1 110 192H82Z M110 92V164A36 36 0 0 0 110 92Z" fill-rule="evenodd"/>
</svg>`;

const HTML = `<!doctype html><html><head><meta charset="utf-8"><style>
  html,body{margin:0;width:${W}px;height:${H}px;overflow:hidden}
  body{background:radial-gradient(1100px 700px at 18% 30%, #1b2333 0%, #0f1115 70%);
    font-family:'Ubuntu','Noto Sans','DejaVu Sans',system-ui,sans-serif;color:#e6ebf4;
    display:flex;align-items:center;gap:72px;padding:0 110px;box-sizing:border-box}
  svg{flex:none;filter:drop-shadow(0 24px 48px rgba(0,0,0,.55))}
  h1{margin:0 0 18px;font-size:76px;font-weight:700;letter-spacing:-1px;line-height:1.05}
  h1 span{background:linear-gradient(180deg,#6aa4ff,#8b5cf6);-webkit-background-clip:text;color:transparent}
  p{margin:0 0 30px;font-size:32px;line-height:1.35;color:#9aa4b2}
  .tags{display:flex;gap:12px;flex-wrap:wrap}
  .tags b{font-weight:500;font-size:22px;color:#c7d2fe;background:#1a2130;border:1px solid #2a3550;
    border-radius:999px;padding:8px 18px}
</style></head><body>
  ${ICON_SVG}
  <div>
    <h1><span>DSH</span> Launcher</h1>
    <p>Desktop window for DeepSeek Harness —<br>starts <code style="font-family:inherit;color:#c7d2fe">dsh web</code> and keeps it in check</p>
    <div class="tags"><b>Electron</b><b>Ubuntu .deb</b><b>RU / EN</b></div>
  </div>
</body></html>`;

app.disableHardwareAcceleration();
app.whenReady().then(async () => {
  const win = new BrowserWindow({
    width: W, height: H, show: false, useContentSize: true,
    webPreferences: { offscreen: true },
  });
  win.webContents.setZoomFactor(1);
  await win.loadURL('data:text/html;charset=utf-8,' + encodeURIComponent(HTML));
  await new Promise((r) => setTimeout(r, 500)); // шрифты и раскладка
  const img = await win.webContents.capturePage({ x: 0, y: 0, width: W, height: H });
  const sized = img.getSize().width === W ? img : img.resize({ width: W, height: H, quality: 'best' });
  fs.writeFileSync(OUT, sized.toPNG());
  console.log(`Готово: ${OUT} (${sized.getSize().width}×${sized.getSize().height}, ${fs.statSync(OUT).size} Б)`);
  app.quit();
});
