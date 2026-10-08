#!/opt/node22/bin/node
// Фейковый dsh: слушает порт, печатает URL с токеном, держит потомка
// (sleep 7777) в своей группе. Поведение — через переменные окружения.
const fs = require('fs');
const http = require('http');
const { spawn } = require('child_process');

const LOG = '/tmp/fakedsh.log';
const role = process.env.FAKE_ROLE || 'main';
const log = (m) => fs.appendFileSync(LOG, `${Date.now()} pid=${process.pid} role=${role} ${m}\n`);

if (process.argv.includes('--version')) {
  console.log(process.env.FAKE_DSH_VERSION || '0.2.0-rc.2');
  process.exit(0);
}

const pgid = fs.readFileSync(`/proc/${process.pid}/stat`, 'utf8').split(') ')[1].split(' ')[2];
log(`start pgid=${pgid}`);
spawn('sleep', ['7777'], { stdio: 'ignore' });

process.on('SIGTERM', () => {
  log('SIGTERM');
  if (process.env.FAKE_IGNORE_TERM === '1') return;
  process.exit(0);
});

const port = Number(process.env.DSH_PORT || 3080);
let srv;
function listen() {
  srv = http.createServer((q, s) => {
    s.setHeader('set-cookie', 'sess=1; Path=/');
    s.setHeader('content-type', 'text/html');
    s.end(`<html><body>FAKE DSH ${role}</body></html>`);
  });
  srv.on('error', () => setTimeout(listen, 200)); // порт ещё занят старым — ждём
  srv.listen(port, '127.0.0.1', () => {
    log('listening');
    console.log(`dsh web: http://127.0.0.1:${port}/?token=abc`);
  });
}
listen();

const after = Number(process.env.FAKE_SELF_RESTART_AFTER || 0);
if (role === 'main' && after) {
  setTimeout(() => {
    log('self-restart');
    const c = spawn(process.execPath, [__filename, ...process.argv.slice(2)], {
      env: { ...process.env, FAKE_ROLE: 'reborn' },
      stdio: 'ignore',
      detached: process.env.FAKE_REBORN_DETACHED === '1',
    });
    c.unref();
    srv.close();
    process.exit(0); // sleep 7777 остаётся в старой группе
  }, after * 1000);
}
