#!/usr/bin/python3
"""Мини HTTP-прокси для сценария proxy: CONNECT-туннели (HTTPS) и обычные
запросы с абсолютным URI (HTTP). Каждый запрос — строка в /tmp/proxy.log.
  proxy.py <порт>
"""
import http.server
import select
import socket
import sys
import time
import urllib.request

LOG = '/tmp/proxy.log'


def log(msg):
    with open(LOG, 'a') as f:
        f.write(f'{int(time.time() * 1000)} {msg}\n')


class Handler(http.server.BaseHTTPRequestHandler):
    def do_CONNECT(self):
        log(f'CONNECT {self.path}')
        host, _, port = self.path.partition(':')
        try:
            up = socket.create_connection((host, int(port or 443)), timeout=30)
        except OSError:
            self.send_error(502)
            return
        self.send_response(200, 'Connection Established')
        self.end_headers()
        conns = [self.connection, up]
        try:
            while True:
                ready, _, _ = select.select(conns, [], [], 60)
                if not ready:
                    break
                for s in ready:
                    data = s.recv(65536)
                    if not data:
                        return
                    (up if s is self.connection else self.connection).sendall(data)
        except OSError:
            pass
        finally:
            up.close()

    def do_GET(self):
        log(f'GET {self.path}')
        try:
            req = urllib.request.Request(self.path, headers={k: v for k, v in self.headers.items() if k.lower() != 'host'})
            with urllib.request.urlopen(req, timeout=30) as r:
                body = r.read()
                self.send_response(r.status)
                for k, v in r.getheaders():
                    if k.lower() not in ('transfer-encoding', 'connection'):
                        self.send_header(k, v)
                self.end_headers()
                self.wfile.write(body)
        except Exception:
            self.send_error(502)

    def log_message(self, *_args):
        pass


srv = http.server.ThreadingHTTPServer(('127.0.0.1', int(sys.argv[1] if len(sys.argv) > 1 else 3129)), Handler)
log('listening')
srv.serve_forever()
