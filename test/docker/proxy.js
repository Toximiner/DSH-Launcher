#!/opt/node22/bin/node
// Мини HTTP-прокси для сценария proxy: CONNECT-туннели (HTTPS) и обычные
// запросы с абсолютным URI (HTTP). Каждый запрос — строка в /tmp/proxy.log.
//   proxy.js <порт>
const fs = require('fs');
const http = require('http');
const net = require('net');

const LOG = '/tmp/proxy.log';
const log = (m) => fs.appendFileSync(LOG, `${Date.now()} ${m}\n`);

const srv = http.createServer((q, s) => {
  log(`GET ${q.url}`);
  let u;
  try { u = new URL(q.url); } catch { s.writeHead(400); s.end(); return; }
  http.get(u, { headers: q.headers }, (r) => { s.writeHead(r.statusCode, r.headers); r.pipe(s); })
    .on('error', () => { s.writeHead(502); s.end(); });
});
srv.on('connect', (req, sock, head) => {
  log(`CONNECT ${req.url}`);
  const [host, port] = req.url.split(':');
  const up = net.connect(Number(port) || 443, host, () => {
    sock.write('HTTP/1.1 200 Connection Established\r\n\r\n');
    up.write(head);
    up.pipe(sock);
    sock.pipe(up);
  });
  up.on('error', () => sock.destroy());
  sock.on('error', () => up.destroy());
});
srv.listen(Number(process.argv[2] || 3129), '127.0.0.1', () => log('listening'));
