"""Actual dsh account flow with a local Platform fixture and isolated credentials."""
import argparse
import http.cookiejar
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import threading
import urllib.parse
import urllib.request
import uuid

from terminal import Terminal, wait_for

parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--dsh', required=True)
args = parser.parse_args()
attempts = []
profile_value = {'id': 'fixture-user', 'email': 'f***@example.invalid', 'id_profile': {'name': 'Fixture User'}}

class Platform(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass
    def reply(self, data):
        body = json.dumps({'code': 0, 'data': {'biz_code': 0, 'biz_data': data}}).encode()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        self.wfile.write(body)
    def do_POST(self):
        data = json.loads(self.rfile.read(int(self.headers.get('Content-Length', '0'))) or b'{}')
        origin = f'http://127.0.0.1:{self.server.server_port}'
        if self.path.endswith('/auth_init'):
            attempts.append(data)
            self.reply({'authorize_id': str(len(attempts)), 'authorize_url': origin + '/dsh/authorize', 'expires_in': 60})
        elif self.path.endswith('/auth_exchange'):
            self.reply({'token': 'fixture-account-token', 'authorized_url': origin + '/dsh/authorized', 'user': profile_value})
        else:
            self.reply({})
    def do_GET(self):
        assert self.headers.get('x-dsh-auth-token') == 'fixture-account-token'
        if self.path.endswith('/current'):
            self.reply(profile_value)
        else:
            self.reply({'normal_wallets': [{'currency': 'USD', 'balance': '12.34'}], 'bonus_wallets': []})

class ConnectResponse(urllib.request.HTTPErrorProcessor):
    def http_response(self, request, response):
        return response if response.status == 302 else super().http_response(request, response)

with tempfile.TemporaryDirectory(prefix='dsh-tui-account-') as directory:
    root = Path(directory)
    platform = ThreadingHTTPServer(('127.0.0.1', 0), Platform)
    threading.Thread(target=platform.serve_forever, daemon=True).start()
    profile = root / 'dsh/profiles/web'
    profile.mkdir(parents=True)
    (profile / 'package.json').write_text(json.dumps({'name': 'isolated-account-test', 'private': True,
        'dependencies': {}, 'dsh': {'profile': {'bundles': ['@deepseek-ai/dsh-base', '@deepseek-ai/dsh-web-app']}}}))
    (profile / 'cordis.yml').write_text('[]\n')
    (profile / 'cordis.patch.yml').write_text(json.dumps([
        {'id': 'deepseek-account', 'config': {'platformOrigin': f'http://127.0.0.1:{platform.server_port}',
          'allowLoopbackHttp': True, 'rewriteBrowserOrigin': True, 'logoutMaxRetries': 0}},
        {'id': 'session-title-llm', 'disabled': True}, {'id': 'goal-round-driver', 'disabled': True}]))
    env = {k: v for k, v in os.environ.items() if k not in ('DSH_TUI_URL', 'NO_COLOR', 'GH_TOKEN', 'GITHUB_TOKEN')}
    env.update(HOME=str(root), DSH_HOME=str(root / 'dsh'), XDG_CONFIG_HOME=str(root / 'config'),
               XDG_DATA_HOME=str(root / 'data'), TERM='xterm-256color', COLORTERM='truecolor')
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    backend = subprocess.Popen([str(Path(args.dsh).resolve()), '--profile', 'web', '--no-open', '--port', str(port)],
        cwd=root, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, start_new_session=True)
    login, logs = [], []
    def drain():
        for line in backend.stdout:
            logs.append(line)
            if 'dsh web: ' in line:
                login.append(line.split('dsh web: ')[1].strip())
    threading.Thread(target=drain, daemon=True).start()
    app = None
    try:
        wait_for(lambda: login or backend.poll() is not None, timeout=40)
        assert login, ''.join(logs)[-4000:]
        parsed = urllib.parse.urlsplit(login[0])
        origin = urllib.parse.urlunsplit((parsed.scheme, parsed.netloc, '', '', ''))
        opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()), ConnectResponse())
        opener.open(origin + '/connect?' + parsed.query).close()
        def rpc(method, payload):
            rpc_id = uuid.uuid4().hex
            data = json.dumps({'type': 'client-request', 'rpcId': rpc_id, 'method': method, 'payload': {'args': payload}}).encode()
            result = json.load(opener.open(urllib.request.Request(origin + '/api/' + method, data=data,
                headers={'Content-Type': 'application/json', 'Origin': origin})))
            assert result['rpcId'] == rpc_id and result['result']['ok'], result
            return result['result'].get('value')
        def state():
            return rpc('account/getState', {})
        app = Terminal(str(Path(args.binary).resolve()), ['--new'], {**env, 'DSH_TUI_URL': login[0]}, columns=100, rows=30)
        app.wait('Введите сообщение.', timeout=30)
        app.command('/account')
        app.wait('Вход не выполнен')
        def action(key):
            app.send(key)
            app.wait('Enter — подтвердить')
            app.send('\r')
        action('\x0e')
        wait_for(lambda: (state().get('attempt') or {}).get('phase') == 'waiting-browser', timeout=20)
        app.send('\x1b[15~')
        app.wait('waiting-browser')
        action('\x07')
        wait_for(lambda: (state().get('attempt') or {}).get('phase') == 'cancelled', timeout=20)
        app.wait('cancelled')
        action('\x0e')
        wait_for(lambda: (state().get('attempt') or {}).get('phase') == 'waiting-browser', timeout=20)
        callback = attempts[-1]['redirect_uri'] + '?' + urllib.parse.urlencode({'state': attempts[-1]['state'], 'code': 'fixture-code'})
        opener.open(callback).close()
        wait_for(lambda: state()['status'] == 'credential-stored', timeout=20)
        app.send('\x1b[15~')
        app.wait('Учётные данные сохранены')
        action('\x1bOR')  # F3: profile and balance, without a model request.
        app.wait('Fixture User')
        assert 'fixture-account-token' not in app.text
        app.send('\x1b')
        app.wait('Учётные данные сохранены')
        action('\x18')
        app.wait('Вход не выполнен')
        assert state()['status'] == 'signed-out'
        assert 'fixture-account-token' not in (root / 'dsh/.credentials.yaml').read_text()
        print('Actual dsh account: sign-in cancellation, callback exchange, private grant, profile/balance and sign-out OK')
    except BaseException:
        if app:
            print(app.text)
        print('Backend tail:', ''.join(logs)[-3000:])
        raise
    finally:
        if app:
            app.close(root / 'terminal.log')
        if backend.poll() is None:
            os.killpg(backend.pid, signal.SIGTERM)
            try:
                backend.wait(timeout=10)
            except subprocess.TimeoutExpired:
                os.killpg(backend.pid, signal.SIGKILL)
                backend.wait(timeout=10)
        platform.shutdown()
