#!/usr/bin/python3
"""Fake dsh: listens on a port, prints the URL with the token, and keeps a
child process (sleep 7777) in its own process group. Behavior — via environment
variables:
  FAKE_DSH_VERSION         — what to print on --version (0.2.0-rc.2)
  FAKE_IGNORE_TERM=1       — ignore SIGTERM (the launcher must finish it with SIGKILL)
  FAKE_SELF_RESTART_AFTER  — restart itself after N seconds (like dsh after a
                             plugin update); FAKE_REBORN_DETACHED=1 — the new
                             process in its own group (otherwise — the same one)
  FAKE_SPAM_LINES          — print that many lines after the address (the log grows)
  FAKE_CRASH_AFTER         — crash after N seconds (exit code 3); FAKE_CRASH_ONCE=<file>
                             — only the first time (marker file), keep running after that
  FAKE_REQUIRE_TOKEN=1     — GUI only with ?token=<FAKE_TOKEN, abc> (then a cookie
                             and a redirect to the address without the token) or
                             with this token's cookie; otherwise 401
  FAKE_DSH_GUI=1           — instead of the stub page serve a "GUI" with an input
                             field, like dsh: div data-composer-input="true"
                             (a Lexical editor stub: a synthetic
                             beforeinput insertText inserts text, Enter —
                             "sends" it to #sent); page helpers:
                             __type(t) — type text, __field() — the field's text
  FAKE_REBORN_TOKEN, FAKE_REBORN_OUT=<file> — the restarted process has its
                             own token (the old cookie no longer works) and output
                             to a file (like a restart via the market button)
    FAKE_REBORN_HOLD_HTTP=<file> — the restarted process holds HTTP responses
                               (socket open, server unresponsive) until the
                               file appears ("dsh is slow to boot");
                               FAKE_HOLD_HTTP=<file> — the same, but for any process
  FAKE_LISTEN_FILE=<file>  — open the port only when the file appears (dsh
                             started, but is slow to bring up its service)
  plugin --profile P add X — "install plugin": add X to dependencies of
                             $DSH_HOME/profiles/P/package.json (default
                             ~/.dsh); FAKE_PLUGIN_FAIL=1 — error, code 1
Event log — /tmp/fakedsh.log.
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

# The "dsh GUI" page for input scenarios (FAKE_DSH_GUI=1): a composer with the
# same marker as the real dsh, and an editor stub — a synthetic beforeinput
# insertText inserts text (like Lexical), unmodified Enter "sends": the text
# goes to #sent, the field is cleared.
GUI_PAGE = """<!doctype html>
<!-- we do not set a <title>: Electron takes the window title from the document,
     and the xdotool focus (test_input_history) looks for the window 'DeepSeek Harness' -->
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
        # "dsh is slow to boot": the port is open (the TCP socket accepts), but
        # the HTTP server does not respond until the file appears. For reborn —
        # FAKE_REBORN_HOLD_HTTP (it does not apply to main, otherwise the first
        # GUI would not come up); otherwise — FAKE_HOLD_HTTP.
        hold = (os.environ.get('FAKE_REBORN_HOLD_HTTP') if ROLE == 'reborn'
                else os.environ.get('FAKE_HOLD_HTTP'))
        if hold and not os.path.exists(hold):
            log('http held (no ' + hold + ')')
            while not os.path.exists(hold):
                time.sleep(0.2)
            log('http released')
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
while True:  # the port is still held by the old process (after self-restart) — wait
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
        os._exit(0)  # sleep 7777 stays in the old group
    threading.Timer(after, restart).start()

srv.serve_forever()
