"""Open-tab persistence with actual isolated dsh, stable origin, and no model work."""
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
import time
import urllib.parse
import urllib.request
import uuid

from terminal import Terminal, wait_for

parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--dsh', required=True)
parser.add_argument('--backend', default='packages/launcher-backend/index.mjs')
parser.add_argument('--capture', help='Optional prefix for actual PTY cell captures')
args = parser.parse_args()
model_calls = []
class TrapModel(BaseHTTPRequestHandler):
    def log_message(self, *_): pass
    def do_POST(self):
        model_calls.append(self.rfile.read(int(self.headers['Content-Length'])))
        self.send_error(500, 'Tab navigation must never call the model')
class ConnectResponse(urllib.request.HTTPErrorProcessor):
    def http_response(self, request, response):
        return response if response.status == 302 else super().http_response(request, response)

def capture(app, suffix):
    if not args.capture: return
    for _ in range(4): app.pump()
    cells = []
    for y in range(app.screen.lines):
        for x in range(app.screen.columns):
            c = app.screen.buffer[y][x]
            cells.append({'x': x, 'y': y, 'text': c.data, 'fg': c.fg, 'bg': c.bg, 'bold': c.bold, 'reverse': c.reverse})
    path = Path(args.capture + '-' + suffix + '.json')
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps({'columns': app.screen.columns, 'rows': app.screen.lines, 'cells': cells}, ensure_ascii=False))

with tempfile.TemporaryDirectory(prefix='dsh-tab-restore-') as directory:
    root = Path(directory)
    project = root / 'project'
    project.mkdir()
    dsh = Path(args.dsh).resolve()
    modules = next(p for p in dsh.parents if p.name == 'node_modules')
    profile = root / 'dsh/profiles/web'
    profile.mkdir(parents=True)
    (profile / 'node_modules').symlink_to(modules, target_is_directory=True)
    (profile / 'package.json').write_text(json.dumps({'name': 'isolated-tab-restore', 'private': True,
        'dependencies': {}, 'dsh': {'profile': {'bundles': ['@deepseek-ai/dsh-base', '@deepseek-ai/dsh-web-app']}}}))
    trap = ThreadingHTTPServer(('127.0.0.1', 0), TrapModel)
    threading.Thread(target=trap.serve_forever, daemon=True).start()
    (profile / 'cordis.yml').write_text('[]\n')
    (profile / 'cordis.patch.yml').write_text(json.dumps([
        {'insert': [{'id': 'launcher-tab-fixture-backend', 'name': str(Path(args.backend).resolve())}]},
        {'id': 'llm-pi-ai', 'config': {'providers': {'tabs-fixture': {'apiKeyEnv': 'TABS_TEST_KEY',
            'api': 'openai-completions', 'baseURL': f'http://127.0.0.1:{trap.server_port}/v1',
            'models': [{'id': 'fixture', 'name': 'Tab fixture', 'contextWindow': 131072, 'maxTokens': 128,
                        'reasoningEfforts': False, 'input': ['text']}], 'retryPolicy': {'mode': 'normal', 'maxRetries': 0}}}}},
        {'id': 'agent-default-model', 'config': {'provider': 'tabs-fixture', 'model': 'fixture'}},
        {'id': 'session-title-llm', 'disabled': True}, {'id': 'goal-round-driver', 'disabled': True}]))
    env = {k: v for k, v in os.environ.items() if k not in ('DSH_TUI_URL', 'GH_TOKEN', 'GITHUB_TOKEN', 'NO_COLOR', 'DISPLAY', 'WAYLAND_DISPLAY')}
    env.update(HOME=str(root), DSH_HOME=str(root / 'dsh'), XDG_CONFIG_HOME=str(root / 'config'),
        XDG_DATA_HOME=str(root / 'data'), TABS_TEST_KEY='isolated-only', TERM='xterm-256color', COLORTERM='truecolor')
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0)); port = sock.getsockname()[1]
    process = None
    app = None
    log_lines = []
    login = []
    opener = None
    origin = None
    def backend_start():
        global process, opener, origin
        login.clear()
        process = subprocess.Popen([str(dsh), '--profile', 'web', '--no-open', '--port', str(port)],
            cwd=project, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, start_new_session=True)
        owner = process
        def drain():
            for line in owner.stdout:
                log_lines.append(line)
                if 'dsh web: ' in line: login.append(line.split('dsh web: ')[1].strip())
        threading.Thread(target=drain, daemon=True).start()
        wait_for(lambda: login, timeout=40)
        parsed = urllib.parse.urlsplit(login[0])
        origin = urllib.parse.urlunsplit((parsed.scheme, parsed.netloc, '', '', ''))
        opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()), ConnectResponse())
        opener.open(origin + '/connect?' + parsed.query, timeout=10).close()
    def backend_stop():
        global process
        if process is not None and process.poll() is None:
            os.killpg(process.pid, signal.SIGTERM)
            try: process.wait(timeout=8)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL); process.wait(timeout=5)
    def rpc(method, payload):
        rpc_id = uuid.uuid4().hex
        body = {'type': 'client-request', 'rpcId': rpc_id, 'method': method, 'payload': {'args': payload}}
        request = urllib.request.Request(origin + '/api/' + method, data=json.dumps(body).encode(), headers={'Content-Type': 'application/json', 'Origin': origin})
        with opener.open(request, timeout=15) as response: result = json.load(response)
        assert result['rpcId'] == rpc_id, result
        return result['result']
    def listed():
        result = rpc('session/list', {'_request': {}})
        assert result['ok'], result
        return {s['sessionId'] for s in result['value']['items']}
    def start(*options, extra_env=None):
        global app
        app = Terminal(str(Path(args.binary).resolve()), list(options), {**env, 'DSH_TUI_URL': login[0], **(extra_env or {})}, columns=120, rows=32)
        app.wait('Новая', timeout=25)
    def quit_app(suffix):
        global app
        app.send('\x04')
        app.exited()
        app.close(root / (suffix + '.ansi'))
        app = None
    def rail(names, active=None, timeout=20):
        def settled():
            app.pump()
            row = app.screen.display[0]
            if not all(name in row for name in names): return False
            positions = [row.index(name) for name in names]
            if positions != sorted(positions): return False
            if active is not None:
                return app.screen.buffer[0][row.index(active)].bold
            return True
        wait_for(settled, timeout=timeout)
    def empty_rail():
        wait_for(lambda: (app.pump() or True) and app.screen.display[0].strip() == '+ Новая', timeout=20)
    def click(name, close=False):
        rail([name])
        row = app.screen.display[0]
        x = row.index(name)
        if close: x = row.index('✕', x)
        app.send(f'\x1b[<0;{x + 1};1M\x1b[<0;{x + 1};1m')
    try:
        backend_start()
        assert not listed(), 'Disposable backend unexpectedly has sessions'
        ids = []
        for title in ['RESTORE_ONE', 'RESTORE_TWO']:
            created = rpc('session/create', {'request': {'cwd': str(project)}})
            assert created['ok'], created
            session_id = created['value']['sessionId']
            ids.append(session_id)
            assert rpc('session/rename', {'request': {'sessionId': session_id, 'title': title}})['ok']
        owned = set(ids)
        # The saved order is opening order, independently of backend list sort.
        start('--session', ids[0])
        rail(['RESTORE_ONE'], 'RESTORE_ONE')
        app.command('/session ' + ids[1])
        rail(['RESTORE_ONE', 'RESTORE_TWO'], 'RESTORE_TWO')
        click('RESTORE_ONE')
        rail(['RESTORE_ONE', 'RESTORE_TWO'], 'RESTORE_ONE')
        capture(app, 'before-restart')
        quit_app('before-restart')
        start()
        rail(['RESTORE_ONE', 'RESTORE_TWO'], 'RESTORE_ONE')
        capture(app, 'restored')
        quit_app('restored')
        # Backend identity survives authentication-token rotation at one origin.
        old_login = login[0]
        backend_stop()
        backend_start()
        assert login[0] != old_login, 'Backend restart did not rotate its login token'
        start()
        rail(['RESTORE_ONE', 'RESTORE_TWO'], 'RESTORE_ONE')
        quit_app('token-rotated')
        # Another local config root must never import the first root's rail.
        start(extra_env={'XDG_CONFIG_HOME': str(root / 'other-config')})
        empty_rail()
        quit_app('other-root')
        # Explicit launch identity wins over the saved active tab.
        start('--session', ids[1])
        rail(['RESTORE_ONE', 'RESTORE_TWO'], 'RESTORE_TWO')
        quit_app('explicit-session')
        start()
        rail(['RESTORE_ONE', 'RESTORE_TWO'], 'RESTORE_TWO')
        # Closing a tab only closes navigation; it must never delete a session.
        click('RESTORE_ONE', close=True)
        wait_for(lambda: (app.pump() or True) and 'RESTORE_ONE' not in app.screen.display[0])
        quit_app('closed-one')
        start()
        rail(['RESTORE_TWO'], 'RESTORE_TWO')
        assert 'RESTORE_ONE' not in app.screen.display[0]
        assert listed() == owned
        assert '✕' not in app.screen.display[0], 'Last-tab close control should be hidden'
        rail(['RESTORE_TWO'], 'RESTORE_TWO')
        capture(app, 'last-tab-retained')
        quit_app('last-close-refused')
        start()
        rail(['RESTORE_TWO'], 'RESTORE_TWO')
        assert listed() == owned, 'Refusing the last tab close changed backend sessions'
        quit_app('last-retained-restart')
        start('--session', ids[0])
        rail(['RESTORE_TWO', 'RESTORE_ONE'], 'RESTORE_ONE')
        # Reopen in a deliberate order after closing a background tab; the
        # saved rail must track user navigation rather than catalog ordering.
        click('RESTORE_TWO', close=True)
        rail(['RESTORE_ONE'], 'RESTORE_ONE')
        app.command('/session ' + ids[1])
        rail(['RESTORE_ONE', 'RESTORE_TWO'], 'RESTORE_TWO')
        quit_app('before-failure')
        # A failed catalog read is not evidence that all saved sessions vanished.
        backend_stop()
        start()
        for _ in range(8): app.pump()
        quit_app('backend-unavailable')
        backend_start()
        start()
        rail(['RESTORE_ONE', 'RESTORE_TWO'], 'RESTORE_TWO')
        quit_app('after-failure')
        # Missing/deleted IDs may be pruned only after a successful fresh catalog.
        removed = rpc('launcherSessions/deleteSession', {'sessionId': ids[0]})
        assert removed['ok'] and removed['value']['deleted'], removed
        owned.remove(ids[0])
        start()
        rail(['RESTORE_TWO'], 'RESTORE_TWO')
        assert 'RESTORE_ONE' not in app.screen.display[0]
        quit_app('missing-pruned')
        start()
        rail(['RESTORE_TWO'], 'RESTORE_TWO')
        assert 'RESTORE_ONE' not in app.screen.display[0]
        quit_app('pruned-restart')
        # Explicit --new adds one new tab without discarding saved open tabs.
        start('--new')
        wait_for(lambda: len(listed()) == len(owned) + 1, timeout=25)
        newly_created = listed() - owned
        assert len(newly_created) == 1
        owned |= newly_created
        rail(['RESTORE_TWO'])
        assert app.screen.display[0].count('✕') == 2, app.screen.display[0]
        new_id = next(iter(newly_created))
        # Rename through the public API so active state is visible on the next
        # startup; no prompt/model request is needed to produce a title.
        assert rpc('session/rename', {'request': {'sessionId': new_id, 'title': 'RESTORE_NEW'}})['ok']
        quit_app('explicit-new')
        start()
        rail(['RESTORE_TWO', 'RESTORE_NEW'], 'RESTORE_NEW')
        capture(app, 'new-restored')
        assert listed() == owned
        assert not model_calls, 'Tab persistence invoked the model'
        print('Actual tab restore: ordered rail/active, token rotation, explicit session/new, closed tabs/last-tab retention, config isolation, failed backend, deleted-ID pruning and zero model calls OK')
    finally:
        if app is not None: app.close(root / 'final.ansi')
        backend_stop()
        trap.shutdown(); trap.server_close()
