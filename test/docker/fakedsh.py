#!/usr/bin/python3
"""Фейковый dsh: слушает порт, печатает URL с токеном, держит потомка
(sleep 7777) в своей группе процессов. Поведение — через переменные окружения:
  FAKE_DSH_VERSION         — что печатать на --version (0.2.0-rc.2)
  FAKE_IGNORE_TERM=1       — игнорировать SIGTERM (лаунчер должен добить SIGKILL)
  FAKE_SELF_RESTART_AFTER  — через N с перезапуститься самому (как dsh после
                             обновления плагина); FAKE_REBORN_DETACHED=1 — новый
                             процесс в своей группе (иначе — в той же)
Журнал событий — /tmp/fakedsh.log.
"""
import http.server
import os
import signal
import subprocess
import sys
import threading
import time

LOG = '/tmp/fakedsh.log'
ROLE = os.environ.get('FAKE_ROLE', 'main')


def log(msg):
    with open(LOG, 'a') as f:
        f.write(f'{int(time.time() * 1000)} pid={os.getpid()} role={ROLE} {msg}\n')


if '--version' in sys.argv:
    print(os.environ.get('FAKE_DSH_VERSION', '0.2.0-rc.2'))
    sys.exit(0)

log(f'start pgid={os.getpgid(0)}')
subprocess.Popen(['sleep', '7777'], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def on_term(_sig, _frame):
    log('SIGTERM')
    if os.environ.get('FAKE_IGNORE_TERM') != '1':
        os._exit(0)


signal.signal(signal.SIGTERM, on_term)


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = f'<html><body>FAKE DSH {ROLE}</body></html>'.encode()
        self.send_response(200)
        self.send_header('set-cookie', 'sess=1; Path=/')
        self.send_header('content-type', 'text/html')
        self.send_header('content-length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_args):
        pass


port = int(os.environ.get('DSH_PORT', '3080'))
while True:  # порт ещё занят старым процессом (после самоперезапуска) — ждём
    try:
        srv = http.server.ThreadingHTTPServer(('127.0.0.1', port), Handler)
        break
    except OSError:
        time.sleep(0.2)
log('listening')
print(f'dsh web: http://127.0.0.1:{port}/?token=abc', flush=True)

after = float(os.environ.get('FAKE_SELF_RESTART_AFTER') or 0)
if ROLE == 'main' and after:
    def restart():
        log('self-restart')
        subprocess.Popen([sys.executable, os.path.abspath(__file__), *sys.argv[1:]],
                         env={**os.environ, 'FAKE_ROLE': 'reborn'},
                         stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                         start_new_session=os.environ.get('FAKE_REBORN_DETACHED') == '1')
        srv.server_close()
        os._exit(0)  # sleep 7777 остаётся в старой группе
    threading.Timer(after, restart).start()

srv.serve_forever()
