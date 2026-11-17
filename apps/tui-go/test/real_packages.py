"""Package lifecycle against actual dsh/pnpm and an isolated local npm registry."""
import argparse
import hashlib
import base64
import io
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tarfile
import tempfile
import threading
import time
import urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from terminal import Terminal, wait_for

parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--dsh', required=True)
parser.add_argument('--cancel-only', action='store_true')
args = parser.parse_args()

cancel_started = threading.Event()
cancel_release = threading.Event()
manifests = {}
tarballs = {}
for version in ['1.0.0', '1.1.0', '1.2.0', '2.0.0']:
    manifest = {'name': 'tui-lifecycle-fixture', 'version': version, 'dsh': {'bundle': {'patch': './cordis.patch.yml'}}}
    if version == '2.0.0':
        manifest['engines'] = {'dsh': '>=99.0.0'}
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode='w:gz') as archive:
        for name, data in [('package.json', json.dumps(manifest).encode()), ('cordis.patch.yml', b'[]\n')]:
            entry = tarfile.TarInfo('package/' + name)
            entry.size = len(data)
            archive.addfile(entry, io.BytesIO(data))
    tarballs[version] = b'corrupt tarball fixture' if version == '1.2.0' else output.getvalue()
    manifests[version] = manifest

class Registry(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass
    def do_GET(self):
        route = urllib.parse.urlsplit(self.path).path
        if route == '/cancel/1.0.0.tgz':
            cancel_started.set()
            cancel_release.wait(timeout=30)
            return
        if route == '/tui-cancel-fixture':
            manifest = {**manifests['1.0.0'], 'name': 'tui-cancel-fixture', 'dist': {
                'tarball': f'http://127.0.0.1:{self.server.server_port}/cancel/1.0.0.tgz'}}
            data = json.dumps({'name': 'tui-cancel-fixture', 'dist-tags': {'latest': '1.0.0'},
                               'versions': {'1.0.0': manifest}, 'time': {'1.0.0': '2020-01-01T00:00:00.000Z'}}).encode()
        elif route.endswith('.tgz'):
            version = route.rsplit('/', 1)[-1][:-4]
            data = tarballs.get(version)
        elif route == '/tui-lifecycle-fixture':
            versions = {}
            for version, manifest in manifests.items():
                versions[version] = {**manifest, 'dist': {
                    'tarball': f'http://127.0.0.1:{self.server.server_port}/tarballs/{version}.tgz',
                    'integrity': 'sha512-' + base64.b64encode(hashlib.sha512(tarballs[version]).digest()).decode()}}
            data = json.dumps({'name': 'tui-lifecycle-fixture', 'dist-tags': {'latest': '1.1.0'},
                               'versions': versions, 'time': {v: '2020-01-01T00:00:00.000Z' for v in versions}}).encode()
        else:
            data = None
        self.send_response(200 if data is not None else 404)
        self.send_header('Content-Type', 'application/octet-stream' if route.endswith('.tgz') else 'application/json')
        self.end_headers()
        self.wfile.write(data if data is not None else b'{"error":"not found"}')

with tempfile.TemporaryDirectory(prefix='dsh-tui-packages-') as directory:
    root = Path(directory)
    registry = ThreadingHTTPServer(('127.0.0.1', 0), Registry)
    threading.Thread(target=registry.serve_forever, daemon=True).start()
    profile = root / 'dsh/profiles/web'
    profile.mkdir(parents=True)
    (profile / 'package.json').write_text(json.dumps({'name': 'isolated-package-test', 'private': True,
        'dependencies': {}, 'dsh': {'profile': {'bundles': ['@deepseek-ai/dsh-base', '@deepseek-ai/dsh-web-app']}}}))
    (profile / 'cordis.yml').write_text('[]\n')
    (profile / '.npmrc').write_text('fetch-retries=0\n')
    (profile / 'cordis.patch.yml').write_text(json.dumps([
        {'id': 'plugin-manager', 'config': {'registry': f'http://127.0.0.1:{registry.server_port}', 'fallbackRegistries': [], 'inspectTimeoutMs': 10000}},
        {'id': 'session-title-llm', 'disabled': True}, {'id': 'goal-round-driver', 'disabled': True}]))
    env = {k: v for k, v in os.environ.items() if k not in ('DSH_TUI_URL', 'NO_COLOR', 'GH_TOKEN', 'GITHUB_TOKEN')}
    env.update(HOME=str(root), DSH_HOME=str(root / 'dsh'), XDG_CONFIG_HOME=str(root / 'config'),
               npm_config_fetch_retries='0', npm_config_fetch_retry_mintimeout='100', npm_config_fetch_retry_maxtimeout='100', XDG_DATA_HOME=str(root / 'data'), XDG_CACHE_HOME=str(root / 'cache'), TERM='xterm-256color', COLORTERM='truecolor')
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    backend = subprocess.Popen([str(Path(args.dsh).resolve()), '--profile', 'web', '--no-open', '--port', str(port)],
        cwd=root, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, start_new_session=True)
    login = []
    logs = []
    def drain():
        for line in backend.stdout:
            logs.append(line)
            if 'dsh web: ' in line:
                login.append(line.split('dsh web: ')[1].strip())
    threading.Thread(target=drain, daemon=True).start()
    app = None
    try:
        wait_for(lambda: login or backend.poll() is not None, timeout=40)
        assert login, ''.join(logs)[-5000:]
        app = Terminal(str(Path(args.binary).resolve()), ['--new'], {**env, 'DSH_TUI_URL': login[0]}, columns=100, rows=30)
        app.wait(('Что будем делать?', 'session-'), timeout=30)
        def saved_navigation():
            files = list((root / 'config/dsh-launcher-tui/tabs').glob('*.json'))
            return json.loads(files[0].read_text()) if len(files) == 1 else {}
        wait_for(lambda: bool(saved_navigation().get('activeId')), timeout=10)
        initial_session = saved_navigation()['activeId']
        app.command('/packages')
        app.wait(('Управление пакетами dsh', 'Снимок dsh'), absent=('Чтение снимка dsh…',))
        app.send('\x0e')  # Ctrl+N
        app.wait('Введите npm-пакет')
        if not args.cancel_only:
            app.send('tui-lifecycle-fixture@1.0.0')
            app.send('\x1b[15~')  # F5
            app.wait('Enter — подтвердить')
            app.send('\r')
            app.wait('Применение:', timeout=90)
            assert 'failed' not in app.text, app.text
            app.send('\x1b[15~')
            app.wait('Снимок dsh')
            app.send('tui-lifecycle-fixture')
            app.wait('tui-lifecycle-fixture · 1.0.0')
            app.send('\x1b[17~')  # F6
            app.wait('Версии пакета')
            app.wait('1.1.0')
            app.wait('2.0.0 · несовместима')
            # The list is newest first. Refuse the incompatible target, then choose 1.1.0.
            app.send('\r')
            app.wait('Выбранная версия несовместима')
            app.send('\x1b[B\x1b[B\r')
            app.wait('Enter — подтвердить')
            app.send('\r')
            app.wait('Применение:', timeout=90)
            assert 'failed' not in app.text, app.text
            app.send('\x1b')
            app.wait('Снимок dsh')
            app.send('tui-lifecycle-fixture')
            app.wait('tui-lifecycle-fixture · 1.1.0')
            app.send('\x1b[17~')
            app.wait('Версии пакета')
            app.wait('1.2.0')
            app.send('\x1b[B\r')
            app.wait('Enter — подтвердить')
            app.send('\r')
            app.wait('Применение: failed', timeout=180)
            app.send('\x1b')
            app.wait('Снимок dsh')
            app.send('tui-lifecycle-fixture')
            app.wait('tui-lifecycle-fixture · 1.1.0')
            app.send('\x1b')
            app.wait('Что будем делать?')
            app.command('/operations')
            app.wait('Журнал операций пакетов')
            app.wait('confirmed')
            receipts = list((root / 'data/dsh-launcher-tui/package-operations').glob('*.json'))
            assert len(receipts) == 3
            assert all(json.loads(p.read_text())['State'] == 'confirmed' for p in receipts)
            assert all(p.stat().st_mode & 0o777 == 0o600 for p in receipts)
            app.send('\x04')
            app.exited()
            app = Terminal(str(Path(args.binary).resolve()), [], {**env, 'DSH_TUI_URL': login[0]}, columns=100, rows=30)
            # A new process restores the visited session rather than creating one.
            app.wait(('Что будем делать?', initial_session[:16]), timeout=30)
            assert saved_navigation()['activeId'] == initial_session
            assert saved_navigation()['ids'] == [initial_session]
            # This fixture deliberately opens a second conversation to verify
            # package-operation receipts belong to the client, not one session.
            app.command('/new')
            wait_for(lambda: bool(saved_navigation().get('activeId')) and saved_navigation()['activeId'] != initial_session, timeout=15)
            app.wait(('Что будем делать?', saved_navigation()['activeId'][:16]))
            app.command('/operations')
            app.wait('Журнал операций пакетов')
            app.wait('confirmed')
            app.send('\x1b')
            app.wait('Что будем делать?')
            app.command('/packages')
            app.wait(('Управление пакетами dsh', 'Снимок dsh'), absent=('Чтение снимка dsh…',))
            app.send('\x15\x0e')
            app.wait('Введите npm-пакет')
        app.send('tui-cancel-fixture@1.0.0')
        app.send('\x1b[15~')
        app.wait('Enter — подтвердить')
        app.send('\r')
        wait_for(cancel_started.is_set, timeout=30)
        app.send('\x18')  # Ctrl+X cancels the named package request.
        app.wait('Применение: cancelled', timeout=30)
        assert 'tui-cancel-fixture' not in json.loads((profile / 'package.json').read_text()).get('dependencies', {})
        cancel_release.set()
        print('Actual dsh/pnpm: explicit cancellation OK' if args.cancel_only else
              'Actual dsh/pnpm: install, compatible update, incompatible refusal, restoration, private receipts and cancellation OK')
    except BaseException:
        print('Backend tail:', ''.join(logs)[-3000:])
        for log in profile.rglob('*.log'):
            if log.stat().st_size < 100000:
                print(log.name, log.read_text(errors='replace')[-2000:])
        if app:
            print(app.text)
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
        cancel_release.set()
        registry.shutdown()
