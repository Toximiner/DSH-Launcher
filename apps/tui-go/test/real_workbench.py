"""Workbench adapters against isolated actual dsh; never touches a personal profile."""
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
import time
import threading
import urllib.parse
import urllib.request
import uuid

from terminal import Terminal, wait_for

parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--dsh', required=True)
parser.add_argument('--capture', help='Prefix for actual workbench terminal-cell captures')
args = parser.parse_args()
requests = []

class Model(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        requests.append(body)
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.end_headers()
        number = len(requests)
        if number in (2, 4):
            names = [entry['function']['name'] for entry in body.get('tools', [])]
            tool = next(name for name in names if name == 'exit_plan_mode' or name.endswith('__exit_plan_mode'))
            delta = {'role': 'assistant', 'tool_calls': [{'index': 0, 'id': f'plan-{number}', 'type': 'function',
                'function': {'name': tool, 'arguments': json.dumps({'plan': '# Workbench plan\nReview the isolated fixture.'})}}]}
            finish = 'tool_calls'
        else:
            delta = {'role': 'assistant', 'content': f'WORKBENCH_REPLY_{number}'}
            finish = 'stop'
        for chunk in [{'choices': [{'index': 0, 'delta': delta, 'finish_reason': None}]},
                      {'choices': [{'index': 0, 'delta': {}, 'finish_reason': finish}]}]:
            self.wfile.write(('data: ' + json.dumps(chunk) + '\n\n').encode())
        self.wfile.write(b'data: [DONE]\n\n')
        self.wfile.flush()

class ConnectResponse(urllib.request.HTTPErrorProcessor):
    def http_response(self, request, response):
        return response if response.status == 302 else super().http_response(request, response)

with tempfile.TemporaryDirectory(prefix='dsh-tui-workbench-') as directory:
    root = Path(directory)
    model = ThreadingHTTPServer(('127.0.0.1', 0), Model)
    threading.Thread(target=model.serve_forever, daemon=True).start()
    endpoint = f'http://127.0.0.1:{model.server_port}/v1'
    profile = root / 'dsh/profiles/web'
    profile.mkdir(parents=True)
    dsh = Path(args.dsh).resolve()
    modules = next(path for path in dsh.parents if path.name == 'node_modules')
    (profile / 'node_modules').symlink_to(modules, target_is_directory=True)
    (profile / 'package.json').write_text(json.dumps({'name': 'isolated-workbench-test', 'private': True,
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
               TERM='xterm-256color', COLORTERM='truecolor', XDG_CONFIG_HOME=str(root / 'config'), XDG_DATA_HOME=str(root / 'data'))
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
        (project / 'sample.txt').write_text('\n'.join(f'line-{i:03d} привет' for i in range(1, 451)))
        (project / 'nested').mkdir()
        app = Terminal(str(Path(args.binary).resolve()), ['--session', session_id], {**env, 'DSH_TUI_URL': login[0]}, columns=100, rows=30)
        app.wait('Введите сообщение.', timeout=20)
        def capture(kind):
            if not args.capture:
                return
            for columns, rows in [(100, 30), (80, 24)]:
                app.resize(columns, rows)
                app.wait(kind)
                # Drain the resize repaint before recording actual terminal cells.
                for _ in range(5):
                    app.pump()
                cells = [{'x': x, 'y': y, 'text': (cell := app.screen.buffer[y][x]).data,
                    'fg': cell.fg, 'bg': cell.bg, 'bold': cell.bold, 'reverse': cell.reverse}
                    for y in range(rows) for x in range(columns)]
                target = Path(args.capture + f'-{columns}x{rows}-' + str(capture.index) + '.json')
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(json.dumps({'source': 'Actual isolated dsh workbench', 'columns': columns, 'rows': rows, 'cells': cells}, ensure_ascii=False))
            capture.index += 1
            app.resize(100, 30)
            app.wait(kind)
        capture.index = 0
        def panel(command, title):
            app.command(command)
            app.wait(title)
            app.wait('Снимок dsh')
            capture(title)
        def back():
            app.send('\x1b')
            app.wait('Введите сообщение.')
        for command, title in [('/workspaces', 'Рабочие области'), ('/packages', 'Управление пакетами'),
                ('/plugins', 'Плагины dsh'), ('/commands', 'Команды плагинов'),
                ('/jobs', 'Фоновые задания'), ('/agents', 'Подагенты сессии'), ('/tasks', 'Задачи команды')]:
            panel(command, title)
            back()
        app.command('/presets')
        app.wait('Текущий пресет:')
        preset = rpc('session/projections', {'request': {'sessionId': session_id}})['value']['values']['agentPreset']
        app.send(preset)
        app.send('\r')
        app.wait('Enter — подтвердить')
        app.send('\r')
        app.wait('Ответ dsh получен')
        assert rpc('session/projections', {'request': {'sessionId': session_id}})['value']['values']['agentPreset'] == preset
        back()
        panel('/files', 'Файлы рабочей области')
        app.send('sample')
        app.wait('sample.txt')
        app.send('\r')
        app.wait('line-001')
        def page_to(key, text):
            for _ in range(60):
                app.send(key)
                deadline = time.monotonic() + 0.2
                while time.monotonic() < deadline:
                    app.pump()
                if text in app.text:
                    return
            raise AssertionError('File scrolling skipped expected text: ' + text)
        page_to('\x1b[6~', 'line-201')
        page_to('\x1b[6~', 'line-401')
        page_to('\x1b[5~', 'line-201')
        app.send('\x1b')
        app.wait('Тип:')
        back()
        panel('/schedules', 'Расписания')
        app.send('\x0e')
        app.wait('F5 — проверить')
        app.send('\x1b<' + '\x0b' * 30)
        schedule = {'title': 'Workbench fixture', 'prompt': 'Never run during this test', 'timing': {'kind': 'after', 'after_seconds': 86400}}
        app.send('\x1b[200~' + json.dumps(schedule) + '\x1b[201~')
        app.send('\x1b[15~')
        app.wait('Enter — подтвердить')
        assert rpc('schedule/list', {'request': {}})['value']['total'] == 0
        app.send('\r')
        app.wait('Ответ dsh получен')
        app.send('\x1b[15~')
        app.wait('Workbench fixture')
        saved = rpc('schedule/list', {'request': {}})['value']['items'][0]
        app.send(' ')
        app.wait('Enter — подтвердить')
        app.send('\r')
        app.wait('Ответ dsh получен')
        assert rpc('schedule/list', {'request': {}})['value']['items'][0]['status'] != 'active'
        app.send('\x1b[15~')
        app.wait('Снимок dsh')
        current = rpc('schedule/list', {'request': {}})['value']['items'][0]
        changed = rpc('schedule/edit', {'request': {'id': current['id'], 'action': 'update',
            'expectedUpdatedAt': current['updatedAt'], 'patch': {'title': 'Foreign title'}}})
        assert changed['ok'], changed
        app.send('\x18')
        app.wait('Enter — подтвердить')
        app.send('\r')
        app.wait('Действие не подтверждено')
        assert rpc('schedule/list', {'request': {}})['value']['items'][0]['title'] == 'Foreign title'
        app.send('\x1b')
        app.wait('Подтверждение отменено')
        app.send('\x1b[15~')
        app.wait('Foreign title')
        app.send('\x18')
        app.wait('Enter — подтвердить')
        app.send('\r')
        app.wait('Ответ dsh получен')
        assert rpc('schedule/list', {'request': {}})['value']['total'] == 0
        back()
        panel('/terminals', 'Терминалы сессии')
        app.send('\x0e')
        app.wait('Enter — подтвердить')
        app.send('\r')
        app.wait('Ответ dsh получен')
        app.send('\x1b[15~')
        app.wait('running')
        terminals = rpc('terminal/list', {'sessionId': session_id})['value']
        assert len(terminals) == 1, terminals
        app.send('\r')
        app.wait('DSH terminal')
        app.send("printf 'BRIDGE_OK_123\\n'; touch bridge-marker\r")
        app.wait('BRIDGE_OK_123')
        wait_for(lambda: (project / 'bridge-marker').exists())
        app.send('\x1d')
        app.wait('Терминалы сессии')
        assert rpc('terminal/list', {'sessionId': session_id})['value'][0]['state'] == 'running'
        app.send('\x18')
        app.wait('Enter — подтвердить')
        app.send('\r')
        app.wait('Ответ dsh получен')
        back()
        def hide_compose():
            app.send('\x1b')
            wait_for(lambda: (app.pump(), 'Сообщение · многострочный' not in app.text)[1])
        app.command('/compose')
        app.wait('Enter — новая строка')
        app.send('\x1b[200~first line\nsecond line привет\x1b[201~')
        app.wait('second line привет')
        capture('Сообщение ·')
        hide_compose()
        app.command('/compose')
        app.wait('second line привет')
        hide_compose()
        app.command('/attach ' + str(project / 'sample.txt'))
        app.wait('sample.txt')
        app.wait('sample.txt (')
        assert not requests, 'Metadata adapters unexpectedly invoked a model'
        app.send('\x1b[15~')
        app.wait('WORKBENCH_REPLY_1', timeout=20)
        user = [message for message in requests[0]['messages'] if message.get('role') == 'user']
        assert 'first line\nsecond line привет' in json.dumps(user, ensure_ascii=False).replace('\\n', '\n'), user
        assert 'sample.txt' in json.dumps(requests[0]['messages'], ensure_ascii=False), 'Uploaded file reference did not reach the model'
        app.command('/plan')
        app.wait('план')
        app.command('Review the plan fixture')
        app.wait('Approve this plan', timeout=20)
        app.wait('Workbench plan')
        app.send('\t\x1b[B\r\x1b[15~')
        app.wait('WORKBENCH_REPLY_3', timeout=20)
        app.command('Review once more')
        app.wait('Approve this plan', timeout=20)
        app.send('\t\r\x1b[15~')
        app.wait('WORKBENCH_REPLY_5', timeout=20)
        assert any('approved' in str(message) for message in requests[4]['messages'] if message.get('role') == 'tool')
        app.send('\x04')
        app.exited()
        print('Actual dsh workbench: inventories, workspace files/pages, explicit schedule create/pause/delete, terminal bridge/detach/close, multiline draft/upload/send, schedule CAS conflict and both plan-review choices OK')
    except Exception:
        if app:
            print(app.text, flush=True)
        raise
    finally:
        if app:
            app.close(root / 'workbench-terminal.ansi')
        if backend.poll() is None:
            os.killpg(backend.pid, signal.SIGTERM)
        backend.wait(timeout=15)
        reader.join(timeout=2)
        backend.stdout.close()
        model.shutdown()
