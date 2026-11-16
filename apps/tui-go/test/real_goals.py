"""Goal snapshots and revision-checked edits against isolated actual dsh."""
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
parser.add_argument('--capture', help='Prefix for actual goal form terminal-cell captures')
args = parser.parse_args()
requests = []

class Model(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        requests.append(body)
        self.send_response(503)
        self.end_headers()
        self.wfile.write(b'Unexpected model request in metadata-only goal test')

class ConnectResponse(urllib.request.HTTPErrorProcessor):
    def http_response(self, request, response):
        return response if response.status == 302 else super().http_response(request, response)

with tempfile.TemporaryDirectory(prefix='dsh-tui-goals-') as directory:
    root = Path(directory)
    model = ThreadingHTTPServer(('127.0.0.1', 0), Model)
    threading.Thread(target=model.serve_forever, daemon=True).start()
    endpoint = f'http://127.0.0.1:{model.server_port}/v1'
    profile = root / 'dsh/profiles/web'
    profile.mkdir(parents=True)
    dsh = Path(args.dsh).resolve()
    modules = next(path for path in dsh.parents if path.name == 'node_modules')
    (profile / 'node_modules').symlink_to(modules, target_is_directory=True)
    (profile / 'package.json').write_text(json.dumps({'name': 'isolated-goals-test', 'private': True,
        'dependencies': {}, 'dsh': {'profile': {'bundles': ['@deepseek-ai/dsh-base', '@deepseek-ai/dsh-web-app']}}}))
    (profile / 'cordis.yml').write_text('[]\n')
    (profile / 'cordis.patch.yml').write_text(json.dumps([
        {'id': 'llm-pi-ai', 'config': {'providers': {'settings-fixture': {
            'apiKeyEnv': 'SETTINGS_TEST_KEY', 'api': 'openai-completions', 'baseURL': endpoint,
            'models': [{'id': 'fixture', 'contextWindow': 131072, 'maxTokens': 256, 'reasoningEfforts': False, 'input': ['text']}],
            'retryPolicy': {'mode': 'normal', 'maxRetries': 0}}}}},
        {'id': 'agent-default-model', 'config': {'provider': 'settings-fixture', 'model': 'fixture'}},
        {'id': 'session-title-llm', 'disabled': True},
        {'id': 'goal-round-driver', 'disabled': True}]))
    project = root / 'project'
    project.mkdir()
    env = {key: value for key, value in os.environ.items() if key not in
           ('DSH_TUI_URL', 'GH_TOKEN', 'GITHUB_TOKEN', 'CODESPACES_GITHUB_TOKEN', 'NO_COLOR')}
    env.update(DSH_HOME=str(root / 'dsh'), SETTINGS_TEST_KEY='isolated-original-key',
               TERM='xterm-256color', COLORTERM='truecolor', XDG_CONFIG_HOME=str(root / 'config'))
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    backend = subprocess.Popen([str(dsh), '--profile', 'web', '--no-open', '--port', str(port)],
        cwd=project, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, start_new_session=True)
    app = None
    login = []
    def drain():
        for line in backend.stdout:
            if 'dsh web: ' in line:
                login.append(line.split('dsh web: ')[1].strip())
    reader = threading.Thread(target=drain, daemon=True)
    reader.start()
    try:
        wait_for(lambda: login, timeout=40)
        parsed = urllib.parse.urlsplit(login[0])
        origin = urllib.parse.urlunsplit((parsed.scheme, parsed.netloc, '', '', ''))
        opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()), ConnectResponse())
        opener.open(origin + '/connect?' + parsed.query).close()
        def rpc(method, args):
            rpc_id = uuid.uuid4().hex
            data = json.dumps({'type': 'client-request', 'rpcId': rpc_id, 'method': method, 'payload': {'args': args}}).encode()
            response = json.load(opener.open(urllib.request.Request(origin + '/api/' + method, data=data,
                headers={'Content-Type': 'application/json', 'Origin': origin})))
            assert response['rpcId'] == rpc_id
            return response['result']
        created = rpc('session/create', {'request': {}})
        assert created['ok']
        session_id = created['value']['sessionId']
        def goal():
            result = rpc('goals/get', {'agentId': session_id})
            assert result['ok'], result
            return result.get('value')
        assert goal() is None
        original = 'Initial goal\nUnicode: привет'
        created_goal = rpc('goals/create', {'agentId': session_id, 'request': {'objective': original, 'maxGoalRounds': 8}})
        assert created_goal['ok'], created_goal
        assert rpc('goals/pause', {'agentId': session_id, 'ref': created_goal['value']['ref']})['ok']
        def edit_remote(**fields):
            current = goal()
            result = rpc('goals/edit', {'agentId': session_id, 'ref': {'id': current['id'], 'revision': current['revision']}, 'request': fields})
            assert result['ok'], result
        def wait_view(text):
            app.wait(text)
            app.wait('Ctrl+R перечитать')
        def open_goal():
            app.command('/goal')
            wait_view('Цель сессии')
            app.wait('Раунды:')
        def finish():
            wait_for(lambda: (app.pump(), app.proc.poll() is not None and b'\x1b[?1049l' in app.raw)[1])
            app.exited()
        def edit_objective(value):
            app.send('\x1bOR')  # F3
            app.wait('Цель · редактирование')
            app.wait('Tab поле')
            current = goal()['objective']
            app.send('\x1b<' + '\x0b' * (2 * (current.count('\n') + 1) + 4))
            app.send('\x1b[200~' + value + '\x1b[201~')
            app.wait(value.split('\n')[0])
        def capture(kind, columns, rows):
            if not args.capture:
                return
            cells = [{'x': x, 'y': y, 'text': (cell := app.screen.buffer[y][x]).data,
                      'fg': cell.fg, 'bg': cell.bg, 'bold': cell.bold, 'reverse': cell.reverse}
                     for y in range(rows) for x in range(columns)]
            target = Path(args.capture + f'-{kind}-{columns}x{rows}.json')
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(json.dumps({'source': 'Actual isolated dsh goal dialog', 'columns': columns, 'rows': rows, 'cells': cells}, ensure_ascii=False))
        binary = str(Path(args.binary).resolve())
        for columns, rows in [(120, 42), (100, 30), (80, 24)]:
            app = Terminal(binary, ['--session', session_id], {**env, 'DSH_TUI_URL': login[0]}, columns=columns, rows=rows)
            app.wait('Введите сообщение.', timeout=20)
            open_goal()
            app.wait('приостановлена')
            app.wait('Автопродолжение: выключено')
            capture('snapshot', columns, rows)
            edit_objective('Discard this goal draft\nUnicode: отмена')
            capture('editor', columns, rows)
            app.send('\x1b')
            app.wait('Черновик отменён')
            assert goal()['objective'] == original
            app.send('\x04')
            finish()
            app.close(root / f'goal-{columns}x{rows}.ansi')
            app = None
        app = Terminal(binary, ['--session', session_id], {**env, 'DSH_TUI_URL': login[0]}, columns=100, rows=30)
        app.wait('Введите сообщение.', timeout=20)
        open_goal()
        edited = 'Saved goal\nUnicode: новая цель\n\nFinal line'
        edit_objective(edited)
        app.send('\t\x01\x0b10')
        app.send('\x1b[15~')
        app.wait('Изменения цели подтверждены')
        saved = goal()
        assert saved['objective'] == edited and saved['maxGoalRounds'] == 10
        assert saved['phase'] == 'paused' and saved['activation'] == 'disarmed' and saved['roundsStarted'] == 0
        edit_objective('Local goal draft\nPreserve the other field')
        edit_remote(maxGoalRounds=17)
        app.send('\x1b[15~')
        app.wait('Запись цели не подтверждена')
        app.send('\x12')
        app.wait('Цель изменилась')
        app.wait('Раунды: 0 / 17')
        app.send('\x02')
        app.wait('Свежая ревизия принята')
        app.wait('Лимит раундов: 17')
        app.send('\x1b[15~')
        app.wait('Изменения цели подтверждены')
        saved = goal()
        assert saved['objective'] == 'Local goal draft\nPreserve the other field' and saved['maxGoalRounds'] == 17
        edit_objective('Discard conflicting goal')
        edit_remote(objective='Other client objective')
        app.send('\x1b[15~')
        app.wait('Запись цели не подтверждена')
        app.send('\x12')
        app.wait('Цель изменилась')
        app.wait('Other client objective')
        app.send('\x1b')
        app.wait('Черновик отменён')
        assert goal()['objective'] == 'Other client objective'
        app.send('\x04')
        finish()
        app.close(root / 'goal-before-reconnect.ansi')
        app = Terminal(binary, ['--session', session_id], {**env, 'DSH_TUI_URL': login[0]}, columns=100, rows=30)
        app.wait('Введите сообщение.', timeout=20)
        open_goal()
        app.wait('Other client objective')
        app.wait('Раунды: 0 / 17')
        saved = goal()
        assert rpc('goals/resume', {'agentId': session_id, 'ref': {'id': saved['id'], 'revision': saved['revision']}})['ok']
        app.send('\x12')
        app.wait('Фаза: активна')
        app.wait('Автопродолжение: включено')
        saved = goal()
        assert rpc('goals/pause', {'agentId': session_id, 'ref': {'id': saved['id'], 'revision': saved['revision']}})['ok']
        app.send('\x12')
        app.wait('приостановлена')
        app.wait('Автопродолжение: выключено')
        saved = goal()
        assert rpc('goals/clear', {'agentId': session_id, 'ref': {'id': saved['id'], 'revision': saved['revision']}})['ok']
        app.send('\x12')
        app.wait('В этой сессии нет цели')
        assert not requests, 'Goal UI unexpectedly started a model turn'
        app.send('\x04')
        finish()
        print('Actual dsh goals: three sizes, snapshots, multiline objective and round-cap edit, cancellation, CAS conflict, explicit review, untouched foreign fields, reconnect, phase/activation and no model calls OK')
    except Exception:
        if app:
            print(app.text, flush=True)
        raise
    finally:
        if app:
            app.close(root / 'goal-terminal.ansi')
        if backend.poll() is None:
            os.killpg(backend.pid, signal.SIGTERM)
        backend.wait(timeout=15)
        reader.join(timeout=2)
        backend.stdout.close()
        model.shutdown()
