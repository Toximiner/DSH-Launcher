#!/usr/bin/python3
"""Фейковый dsh: слушает порт, печатает URL с токеном, держит потомка
(sleep 7777) в своей группе процессов. Поведение — через переменные окружения:
  FAKE_DSH_VERSION         — что печатать на --version (0.2.0-rc.2)
  FAKE_IGNORE_TERM=1       — игнорировать SIGTERM (лаунчер должен добить SIGKILL)
  FAKE_SELF_RESTART_AFTER  — через N с перезапуститься самому (как dsh после
                             обновления плагина); FAKE_REBORN_DETACHED=1 — новый
                             процесс в своей группе (иначе — в той же)
  FAKE_SPAM_LINES          — после адреса напечатать столько строк (журнал растёт)
  FAKE_CRASH_AFTER         — через N с упасть (выход с кодом 3); FAKE_CRASH_ONCE=<файл>
                             — только в первый раз (файл-метка), дальше работать
  FAKE_REQUIRE_TOKEN=1     — GUI только с ?token=<FAKE_TOKEN, abc> (тогда куки
                             и редирект на адрес без токена) или с куки этого
                             токена; иначе 401
  FAKE_DSH_GUI=1           — вместо страницы-заглушки отдавать «GUI» с полем
                             ввода, как у dsh: div data-composer-input="true"
                             (заглушка редактора Lexical: синтетический
                             beforeinput insertText вставляет текст, Enter —
                             «отправка» в #sent); хелперы страницы:
                             __type(t) — набрать текст, __field() — текст поля
  FAKE_REBORN_TOKEN, FAKE_REBORN_OUT=<файл> — у перезапустившегося процесса
                             свой токен (старое куки не годится) и вывод в файл
                             (как перезапуск кнопкой маркета)
  FAKE_LISTEN_FILE=<файл>  — открыть порт, только когда появится файл (dsh
                             запустился, но долго не поднимает сервис)
  plugin --profile P add X — «установить плагин»: X в dependencies
                             $DSH_HOME/profiles/P/package.json (по умолчанию
                             ~/.dsh); FAKE_PLUGIN_FAIL=1 — ошибка, код 1
Журнал событий — /tmp/fakedsh.log.
"""
import json
import http.server
import os
import signal
import subprocess
import sys
import threading
import time

LOG = '/tmp/fakedsh.log'
ROLE = os.environ.get('FAKE_ROLE', 'main')
TOKEN = os.environ.get('FAKE_TOKEN', 'abc')


def log(msg):
    with open(LOG, 'a') as f:
        f.write(f'{int(time.time() * 1000)} pid={os.getpid()} role={ROLE} {msg}\n')


if '--version' in sys.argv:
    print(os.environ.get('FAKE_DSH_VERSION', '0.2.0-rc.2'))
    sys.exit(0)

if sys.argv[1:2] == ['plugin']:
    profile, name = sys.argv[sys.argv.index('--profile') + 1], sys.argv[sys.argv.index('add') + 1]
    pnpm = next((os.path.join(d, 'pnpm') for d in os.environ.get('PATH', '').split(':')
                 if os.access(os.path.join(d, 'pnpm'), os.X_OK)), None)
    log(f'plugin add {name} profile={profile} pnpm={pnpm}')
    print(f'Resolving {name}…', flush=True)
    if os.environ.get('FAKE_PLUGIN_FAIL') == '1':
        print(f'ERR_PNPM_FETCH_404  {name}: not found', file=sys.stderr, flush=True)
        sys.exit(1)
    pkg_dir = os.path.join(os.environ.get('DSH_HOME') or os.path.expanduser('~/.dsh'), 'profiles', profile)
    os.makedirs(pkg_dir, exist_ok=True)
    pkg_file = os.path.join(pkg_dir, 'package.json')
    pkg = json.load(open(pkg_file)) if os.path.exists(pkg_file) else {}
    pkg.setdefault('dependencies', {})[name] = '1.0.0'
    json.dump(pkg, open(pkg_file, 'w'))
    print(f'Added {name} to profile {profile}', flush=True)
    sys.exit(0)

log(f'start pgid={os.getpgid(0)} cwd={os.getcwd()} args={" ".join(sys.argv[1:])}')
subprocess.Popen(['sleep', '7777'], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def on_term(_sig, _frame):
    log('SIGTERM')
    if os.environ.get('FAKE_IGNORE_TERM') != '1':
        os._exit(0)


signal.signal(signal.SIGTERM, on_term)

# Страница «GUI dsh» для сценариев ввода (FAKE_DSH_GUI=1): композер с тем же
# маркером, что у настоящего dsh, и заглушка редактора — синтетический
# beforeinput insertText вставляет текст (как Lexical), Enter без модификаторов
# «отправляет»: текст уходит в #sent, поле очищается.
GUI_PAGE = """<!doctype html>
<!-- <title> не задаём: Electron берёт заголовок окна из документа,
     а xdotool-фокус (test_input_history) ищет окно 'DeepSeek Harness' -->
<html><head><meta charset="utf-8"></head>
<body>
<div id="c" contenteditable="true" role="textbox" aria-multiline="true" data-composer-input="true"></div>
<pre id="sent"></pre>
<script>
const c = document.getElementById('c');
c.addEventListener('beforeinput', (e) => {
  if (e.inputType !== 'insertText') return;
  e.preventDefault();
  const sel = window.getSelection();
  if (!sel.rangeCount) return;
  const r = sel.getRangeAt(0);
  r.deleteContents();
  r.insertNode(document.createTextNode(e.data || ''));
  r.collapse(false);
  sel.removeAllRanges();
  sel.addRange(r);
});
c.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
    e.preventDefault();
    document.getElementById('sent').textContent += (c.innerText || '') + '\\n';
    c.textContent = '';
  }
});
window.__type = (t) => {
  c.focus();
  const r = document.createRange();
  r.selectNodeContents(c);
  r.collapse(false);
  const sel = window.getSelection();
  sel.removeAllRanges();
  sel.addRange(r);
  c.dispatchEvent(new InputEvent('beforeinput', { inputType: 'insertText', data: t, bubbles: true, cancelable: true }));
};
window.__field = () => c.innerText;
</script>
</body></html>"""


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if os.environ.get('FAKE_REQUIRE_TOKEN') == '1':
            if f'token={TOKEN}' in self.path:
                log('token accepted')
                self.send_response(302)
                self.send_header('set-cookie', f'sess={TOKEN}; Path=/')
                self.send_header('location', self.path.split('?')[0])
                self.send_header('content-length', '0')
                self.end_headers()
                return
            if f'sess={TOKEN}' not in (self.headers.get('cookie') or ''):
                body = b'<html><body>401 token required</body></html>'
                self.send_response(401)
                self.send_header('content-type', 'text/html')
                self.send_header('content-length', str(len(body)))
                self.end_headers()
                self.wfile.write(body)
                return
        body = (GUI_PAGE if os.environ.get('FAKE_DSH_GUI') == '1'
                else f'<html><body>FAKE DSH {ROLE}</body></html>').encode()
        self.send_response(200)
        self.send_header('set-cookie', 'sess=1; Path=/')
        self.send_header('content-type', 'text/html')
        self.send_header('content-length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_args):
        pass


port = int(sys.argv[sys.argv.index('--port') + 1]) if '--port' in sys.argv else int(os.environ.get('DSH_PORT', '3080'))
listen_file = os.environ.get('FAKE_LISTEN_FILE')
if listen_file:
    log('waiting for ' + listen_file)
    while not os.path.exists(listen_file):
        time.sleep(0.2)
while True:  # порт ещё занят старым процессом (после самоперезапуска) — ждём
    try:
        srv = http.server.ThreadingHTTPServer(('127.0.0.1', port), Handler)
        break
    except OSError:
        time.sleep(0.2)
log('listening')
print(f'dsh web: http://127.0.0.1:{port}/?token={TOKEN}', flush=True)
for i in range(int(os.environ.get('FAKE_SPAM_LINES', '0'))):
    print(f'spam line {i:05d} ' + '.' * 60, flush=True)

crash_after = float(os.environ.get('FAKE_CRASH_AFTER') or 0)
crash_once = os.environ.get('FAKE_CRASH_ONCE')
if ROLE == 'main' and crash_after and not (crash_once and os.path.exists(crash_once)):
    if crash_once:
        open(crash_once, 'w').close()

    def crash():
        time.sleep(crash_after)
        log('crash')
        os._exit(3)
    threading.Thread(target=crash, daemon=True).start()

after = float(os.environ.get('FAKE_SELF_RESTART_AFTER') or 0)
if ROLE == 'main' and after:
    def restart():
        log('self-restart')
        env = {**os.environ, 'FAKE_ROLE': 'reborn'}
        if os.environ.get('FAKE_REBORN_TOKEN'):
            env['FAKE_TOKEN'] = os.environ['FAKE_REBORN_TOKEN']
        out = os.environ.get('FAKE_REBORN_OUT')
        subprocess.Popen([sys.executable, os.path.abspath(__file__), *sys.argv[1:]], env=env,
                         stdin=subprocess.DEVNULL, stdout=open(out, 'w') if out else subprocess.DEVNULL,
                         stderr=subprocess.DEVNULL,
                         start_new_session=os.environ.get('FAKE_REBORN_DETACHED') == '1')
        srv.server_close()
        os._exit(0)  # sleep 7777 остаётся в старой группе
    threading.Timer(after, restart).start()

srv.serve_forever()
