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
parser.add_argument('--binary', default='build/previews/opencode-04/dsh-launcher-tui')
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
    sdk_examples = Path(__file__).resolve().parents[3] / 'packages/tui-plugin-sdk/examples'
    (profile / 'cordis.patch.yml').write_text(json.dumps([
        {'insert': [{'id': 'tui-greeting-example', 'name': str(sdk_examples / 'greeting.mjs')},
                    {'id': 'tui-counter-example', 'name': str(sdk_examples / 'counter.mjs')}]},
        {'id': 'llm-pi-ai', 'config': {'providers': {'settings-fixture': {
            'apiKeyEnv': 'SETTINGS_TEST_KEY', 'api': 'openai-completions', 'baseURL': endpoint,
            'models': [{'id': 'fixture', 'name': 'fixture baseline', 'contextWindow': 131072, 'maxTokens': 256, 'reasoningEfforts': False, 'input': ['text']}],
            'retryPolicy': {'mode': 'normal', 'maxRetries': 0}},
            'secondary-fixture': {
                'apiKeyEnv': 'SETTINGS_TEST_KEY', 'api': 'openai-completions', 'baseURL': endpoint,
                'models': [{'id': 'fixture-long', 'name': 'Fixture model with a very long readable name 世界 for grouped picker geometry',
                            'contextWindow': 131072, 'maxTokens': 256, 'reasoningEfforts': False, 'input': ['text']}],
                'retryPolicy': {'mode': 'normal', 'maxRetries': 0}}}}},
        {'id': 'agent-default-model', 'config': {'provider': 'settings-fixture', 'model': 'fixture'}},
        {'id': 'session-title-llm', 'disabled': True},
        {'id': 'goal-round-driver', 'disabled': True}]))
    project = root / 'project'
    project.mkdir()
    env = {key: value for key, value in os.environ.items() if key not in
           ('DSH_TUI_URL', 'GH_TOKEN', 'GITHUB_TOKEN', 'CODESPACES_GITHUB_TOKEN', 'NO_COLOR', 'DISPLAY', 'WAYLAND_DISPLAY', 'TMUX', 'SSH_CONNECTION', 'SSH_TTY')}
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
        workspace = rpc('workspace/create', {'request': {'path': str(project)}})
        assert workspace['ok'], workspace
        workspace_id = workspace['value']['workspace']['workspaceId']
        renamed = rpc('workspace/rename', {'request': {'workspaceId': workspace_id, 'title': 'Sidebar Fixture'}})
        assert renamed['ok'], renamed
        created = rpc('session/create', {'request': {'workspaceId': workspace_id}})
        assert created['ok']
        session_id = created['value']['sessionId']
        assert rpc('session/rename', {'request': {'sessionId': session_id, 'title': 'Sidebar chat'}})['ok']
        (project / 'sample.txt').write_text('\n'.join(f'line-{i:03d} привет' for i in range(1, 451)))
        (project / 'nested').mkdir()
        pdf_objects = [b'<< /Type /Catalog /Pages 2 0 R >>', b'<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
                       b'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>',
                       b'<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>']
        pdf_text = b'BT /F1 12 Tf 20 100 Td (PDF preview fixture) Tj ET'
        pdf_objects.append(b'<< /Length ' + str(len(pdf_text)).encode() + b' >>\nstream\n' + pdf_text + b'\nendstream')
        pdf = b'%PDF-1.4\n'
        offsets = [0]
        for number, obj in enumerate(pdf_objects, 1):
            offsets.append(len(pdf))
            pdf += f'{number} 0 obj\n'.encode() + obj + b'\nendobj\n'
        xref = len(pdf)
        pdf += b'xref\n0 6\n0000000000 65535 f \n' + b''.join(f'{offset:010d} 00000 n \n'.encode() for offset in offsets[1:])
        pdf += f'trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF\n'.encode()
        (project / 'sample.pdf').write_bytes(pdf)
        (project / 'page.html').write_text('<html><body><p>HTML preview fixture</p><script>hidden-script-text</script></body></html>')
        import struct
        import zlib
        def png_chunk(kind, data):
            return struct.pack('!I', len(data)) + kind + data + struct.pack('!I', zlib.crc32(kind + data))
        (project / 'pixel.png').write_bytes(b'\x89PNG\r\n\x1a\n' + png_chunk(b'IHDR', struct.pack('!IIBBBBB', 1, 1, 8, 2, 0, 0, 0)) + png_chunk(b'IDAT', zlib.compress(b'\x00\xff\x00\x00')) + png_chunk(b'IEND', b''))
        app = Terminal(str(Path(args.binary).resolve()), ['--session', session_id], {**env, 'DSH_TUI_URL': login[0]}, columns=100, rows=30)
        landing_marker = 'Что будем делать?'
        app.wait(landing_marker, timeout=20)
        def capture(kind, sizes=((100, 30), (80, 24))):
            if not args.capture:
                return
            for columns, rows in sizes:
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
        def wait_tab(title):
            def selected():
                app.pump()
                column = app.screen.display[0].find(title)
                return column >= 0 and app.screen.buffer[0][column].bold and 'DeepSeek Harness' in app.text
            wait_for(selected)
        # Open tabs represent visited sessions; browsing the palette is read-only.
        capture(landing_marker, sizes=((160, 40), (100, 30), (80, 24)))
        before_sessions = {item['sessionId'] for item in rpc('session/list', {'_request': {}})['value']['items']}
        app.send('preserved navigation draft')
        app.send('\x10')
        app.wait('Команды')
        capture('Команды', sizes=((160, 40), (100, 30), (80, 24)))
        app.send('Рабочие области\r')
        app.wait('Сессии и рабочие области')
        app.wait('Sidebar Fixture')
        capture('Сессии и рабочие области', sizes=((160, 40), (100, 30), (80, 24)))
        app.send('Sidebar Fixture\r')
        app.wait('Sidebar chat')
        assert {item['sessionId'] for item in rpc('session/list', {'_request': {}})['value']['items']} == before_sessions, 'Workspace filtering created a session'
        app.send('\x1b')
        app.wait('preserved navigation draft')
        app.send('\x15')
        assert {item['sessionId'] for item in rpc('session/list', {'_request': {}})['value']['items']} == before_sessions, 'Palette browsing created sessions'
        second = rpc('session/create', {'request': {'workspaceId': workspace_id}})
        assert second['ok'], second
        second_id = second['value']['sessionId']
        assert rpc('session/rename', {'request': {'sessionId': second_id, 'title': 'Second tab fixture'}})['ok']
        app.command('/session ' + second_id)
        app.wait('Second tab fixture')
        wait_tab('Second tab fixture')
        app.send('second tab draft')
        # Switch via the actual rendered tab and verify the destination draft.
        app.pump()
        first_column = app.screen.display[0].index('Sidebar chat')
        app.send(f'\x1b[<0;{first_column + 2};1M\x1b[<0;{first_column + 2};1m')
        wait_tab('Sidebar chat')
        app.send('first tab draft')
        app.pump()
        second_column = app.screen.display[0].index('Second tab')
        app.send(f'\x1b[<0;{second_column + 2};1M\x1b[<0;{second_column + 2};1m')
        wait_tab('Second tab fixture')
        app.wait('second tab draft')
        app.send('\x15')
        app.command('/session ' + session_id)
        wait_tab('Sidebar chat')
        app.wait('first tab draft')
        app.send('\x15')
        capture(landing_marker, sizes=((160, 40), (100, 30), (80, 24)))
        assert not requests, 'Tab or palette navigation invoked a model'
        # The shared quick picker filters sessions without creating or running one.
        app.command('/sessions')
        app.wait('Сессии и рабочие области')
        app.wait('Sidebar chat')
        capture('Сессии и рабочие области', sizes=((160, 40), (100, 30), (80, 24)))
        app.send('Second tab fixture')
        app.wait('Second tab fixture')
        app.pump()
        picker_row, picker_line = next((y, line) for y, line in enumerate(app.screen.display)
            if 'Second tab fixture' in line and 'Sidebar Fixture' in line)
        picker_column = picker_line.index('Second tab fixture')
        app.send(f'\x1b[<0;{picker_column + 2};{picker_row + 1}M\x1b[<0;{picker_column + 2};{picker_row + 1}m')
        wait_tab('Second tab fixture')
        app.send('picker preserved draft')
        app.send('\x10')
        app.wait('Команды')
        app.send('Список сессий\r')
        app.wait('Сессии и рабочие области')
        app.send('filter never submitted')
        app.send('\x1b')
        app.wait('picker preserved draft')
        app.send('\x15')
        app.command('/session ' + session_id)
        wait_tab('Sidebar chat')
        app.command('/workspaces')
        app.wait('Сессии и рабочие области')
        app.wait('Sidebar Fixture')
        app.send('Sidebar Fixture\r')
        app.wait('Sidebar chat')
        app.send('\x1bOR')  # F3 opens the existing explicit workspace manager.
        app.wait('Рабочие области')
        app.wait('Снимок dsh')
        app.send('\x1b')
        app.wait(landing_marker)
        assert {item['sessionId'] for item in rpc('session/list', {'_request': {}})['value']['items']} == before_sessions | {second_id}, 'Quick picker navigation created or removed sessions'
        assert not requests, 'Quick picker navigation invoked the model'

        def panel(command, title):
            app.command(command)
            app.wait(title)
            app.wait('Снимок dsh')
            capture(title, sizes=((160, 40), (100, 30), (80, 24)) if command in ('/plugins', '/schedules') else ((100, 30), (80, 24)))
        def back():
            app.send('\x1b')
            app.wait(landing_marker)
        capture(landing_marker)
        app.command('/presets')
        app.wait('Пресеты агента')
        app.wait('Текущий пресет:')
        back()
        def current_model():
            values = rpc('session/projections', {'request': {'sessionId': session_id}})['value']['values']
            selected = (values.get('modelSelection') or {}).get('next')
            return selected or rpc('session/modelCatalog', {})['value']['default']
        initial_model = current_model()
        app.send('model chooser draft')
        app.send('\x1b[24~')
        app.wait('Модель сессии')
        app.wait('fixture baseline')
        app.wait('secondary-fixture')
        capture('Модель сессии', sizes=((160, 40), (100, 30), (80, 24)))
        app.send('fixture-long')
        app.wait('Fixture model')
        app.pump()
        model_row, model_line = next((y, line) for y, line in enumerate(app.screen.display)
            if 'Fixture model' in line)
        model_column = model_line.index('Fixture model')
        app.send(f'\x1b[<0;{model_column + 2};{model_row + 1}M\x1b[<0;{model_column + 2};{model_row + 1}m')
        assert current_model() == initial_model, 'Model-row mouse selection performed a hidden write'
        app.send('\x1b')
        app.wait('model chooser draft')
        assert current_model() == initial_model, 'Cancelling model chooser changed the selection'
        app.send('\x15')
        app.send('\x1b[24~')
        app.wait('fixture baseline')
        app.send('fixture-long')
        app.wait('Fixture model')
        app.send('\r')
        app.wait('Сессия: выбор подтверждён.')
        assert current_model()['provider'] == 'secondary-fixture' and current_model()['model'] == 'fixture-long'
        capture('Модель сессии', sizes=((160, 40), (100, 30), (80, 24)))
        app.send('\x15baseline\r')
        app.wait('Сессия: выбор подтверждён.')
        wait_for(lambda: current_model() == initial_model)
        app.send('\x1b')
        app.wait(landing_marker)
        assert not requests, 'Model picker reads and explicit selection invoked the model'
        initial_permission = rpc('session/projections', {'request': {'sessionId': session_id}})['value']['values']['permissions']['currentValue']
        assert initial_permission == 'workspace-write', initial_permission
        def permission_panel():
            app.pump()
            row, line = next((y, line) for y, line in enumerate(app.screen.display) if 'Запись в рабочую область' in line or 'Только чтение' in line)
            label = 'Запись в рабочую область' if 'Запись в рабочую область' in line else 'Только чтение'
            column = line.index(label)
            app.send(f'\x1b[<0;{column + 2};{row + 1}M\x1b[<0;{column + 2};{row + 1}m')
            app.wait('Доступ к файлам и подтверждения')
            app.wait('Снимок dsh')
        permission_panel()
        app.send('Только чтение\r')
        app.wait('Enter — подтвердить')
        assert rpc('session/projections', {'request': {'sessionId': session_id}})['value']['values']['permissions']['currentValue'] == initial_permission
        app.send('\x1b')
        app.wait('Подтверждение отменено.')
        app.send('\r\r')
        app.wait('preset read-only')
        assert rpc('session/projections', {'request': {'sessionId': session_id}})['value']['values']['permissions']['currentValue'] == 'read-only'
        back()
        permission_panel()
        app.send('Запись в рабочую область\r')
        app.wait('Enter — подтвердить')
        app.send('\r')
        app.wait('preset workspace-write')
        assert rpc('session/projections', {'request': {'sessionId': session_id}})['value']['values']['permissions']['currentValue'] == initial_permission
        back()
        assert not requests, 'Agent controls invoked a model'
        # The GUI-shaped settings hub keeps its modal context across actual panels.
        app.send('\x1bOS')
        app.wait('Конфигурация dsh')
        def settings_ready():
            app.pump()
            return 'Загрузка настроек…' not in app.text
        wait_for(settings_ready)
        capture('Настройки TUI', sizes=((160, 40), (100, 30), (80, 24)))
        app.send('\x1b[B\r\r')
        app.wait('Модель сессии')
        app.wait('Текущая:')
        app.send('\x1b')
        app.wait('Конфигурация dsh')
        app.send('\x1b[D\x1b[B\r\r')
        app.wait('Плагины dsh')
        app.wait('Снимок dsh')
        app.send('\x1b')
        app.wait('Конфигурация dsh')
        app.send('\x1b[D\x1b[B\r\r')
        app.wait('Пресеты агента')
        app.wait('Текущий пресет:')
        app.send('\x1b')
        app.wait('Конфигурация dsh')
        app.send('\x1b[D\x1b[H\x1b[C\r')
        app.wait('Настройки · permission')
        app.wait('Значение:')
        app.send('\x15read-only\r\x1b[15~')
        app.wait('Настройки сохранены.')
        assert rpc('session/projections', {'request': {'sessionId': session_id}})['value']['values']['permissions']['currentValue'] == initial_permission, 'Default permission changed the existing session'
        future = rpc('session/create', {'request': {'workspaceId': workspace_id}})
        assert future['ok'], future
        assert rpc('session/projections', {'request': {'sessionId': future['value']['sessionId']}})['value']['values']['permissions']['currentValue'] == 'read-only', 'New session did not inherit the saved permission default'
        app.send('\x1b')
        app.wait('Конфигурация dsh')
        app.send('\r')
        app.wait('Настройки · permission')
        app.wait('Значение:')
        app.send('\x15workspace-write\r\x1b[15~')
        app.wait('Настройки сохранены.')
        app.send('\x1b')
        app.wait('Конфигурация dsh')
        app.send('\x1b')
        app.wait(landing_marker)
        assert not requests, 'Settings navigation unexpectedly invoked a model'
        for command, title in [('/workspaces-manager', 'Рабочие области'), ('/packages', 'Управление пакетами'),
                ('/plugins', 'Плагины dsh'), ('/account', 'Аккаунт DeepSeek'), ('/commands', 'Команды плагинов'),
                ('/jobs', 'Фоновые задания'), ('/agents', 'Подагенты сессии'), ('/tasks', 'Задачи команды')]:
            panel(command, title)
            if command == '/plugins':
                app.wait('Настройки встроенных плагинов')
                app.send('\r')
                app.wait('Настройки · subagent')
                app.send('\x1b')
                app.wait('Плагины dsh')
                app.send('\x1bOQ')  # F2 opens the separate technical view.
                app.wait('Компоненты dsh · только чтение')
                app.send('\x1bOQ')
                app.wait('Настройки встроенных плагинов')
                badge = '@deepseek-ai/dsh-experimental-badge-skill-bundle'
                def badge_enabled():
                    return next(row['enabled'] for row in rpc('pluginManager/listBundles', {})['value'] if row['name'] == badge)
                assert badge_enabled() is False
                app.send(badge)
                app.wait('[Выкл]')
                app.send(' ')
                app.wait('Enter — подтвердить действие')
                app.send('\x1b')
                assert badge_enabled() is False, 'Cancelling the switch changed the profile'
                app.send(' \r')
                app.wait('[Вкл]', timeout=30)
                assert badge_enabled() is True, 'The switch did not persist its exact target'
                app.send(' \r')
                app.wait('[Выкл]', timeout=30)
                assert badge_enabled() is False, 'The fixture switch was not restored'
                app.send('\x15')
                app.wait('Настройки встроенных плагинов')
            back()
        for command, title, expected in [('tui-greeting', 'Greeting example', 'Hello, Alice'), ('tui-counter', 'Counter example', 'Total │ 2')]:
            panel('/commands', 'Команды плагинов')
            app.send(command)
            app.wait('/' + command)
            app.send('\r')
            app.wait('Enter — подтвердить')
            app.send('\r')
            app.wait(title)
            if command == 'tui-greeting':
                app.send('\r\x15Alice\r')
                app.send('\x1b[B')
            else:
                app.send('\x1b[B\x1b[B')
            app.send('\r')
            app.wait('Enter — подтвердить')
            app.send('\r')
            app.wait(expected)
            app.send('\x1b')
            app.wait('Команды плагинов')
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
        for filename, expected in [('page.html', 'HTML preview fixture'), ('pixel.png', '1 × 1'), ('sample.pdf', 'PDF preview fixture')]:
            panel('/files', 'Файлы рабочей области')
            app.send(filename)
            app.wait(filename)
            app.send('\r')
            app.wait(expected)
            assert 'hidden-script-text' not in app.text
            app.send('\x1b')
            app.wait('Снимок dsh')
            back()
        panel('/files', 'Файлы рабочей области')
        app.send('sample.txt')
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
        panel('/schedules', 'Автоматизация')
        app.send('\x0e')
        app.wait('F5 — проверить')
        app.send('Workbench fixture\tNever run during this test\t\t\x01\x0b1440')
        app.send('\x1b[15~')
        app.wait('Enter — подтвердить')
        assert rpc('schedule/list', {'request': {}})['value']['total'] == 0
        app.send('\r')
        app.wait('Ответ dsh получен')
        app.send('\x1b[15~')
        app.wait('Workbench fixture')
        capture('Workbench fixture', sizes=((160, 40), (100, 30), (80, 24)))
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
        def committed_reply():
            app.pump()
            return 'WORKBENCH_REPLY_1' in app.text and '◌ Агент отвечает…' not in app.text
        wait_for(committed_reply)
        app.wait('Ctrl+P команды')
        # Per-message metadata now uses actual duration, without session-wide counters.
        capture('WORKBENCH_REPLY_1', sizes=((160, 40), (100, 30), (80, 24)))
        model_row, model_line = next((y, line) for y, line in enumerate(app.screen.display) if 'fixture' in line and 'Запись' in line)
        model_column = model_line.index('fixture')
        app.send(f'\x1b[<0;{model_column + 2};{model_row + 1}M\x1b[<0;{model_column + 2};{model_row + 1}m')
        app.wait('Модель сессии')
        app.send('\x1b')
        app.wait('Ctrl+P команды')
        # Ctrl+Y copies the committed source through PTY-only OSC 52.
        import base64
        import re
        app.send('conversation draft')
        copy_start = len(app.raw)
        app.send('\x19')
        app.wait('запрос OSC 52 отправлен')
        copied = re.findall(rb'\x1b\]52;c;([^\x07\x1b]*)', app.raw[copy_start:])
        assert copied and base64.b64decode(copied[-1]).decode() == 'WORKBENCH_REPLY_1', 'Conversation copy changed the source'
        assert 'conversation draft' in app.text, 'Conversation copy lost the composer draft'
        app.send('\x15')
        app.send('\x1b[23~')
        app.wait('История сессии')
        app.wait('WORKBENCH_REPLY_1')
        app.send('\x1b')
        app.wait('Ctrl+P команды')
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
        assert len(requests) == 5, f'Unexpected local model request count: {len(requests)}'
        assert any('approved' in str(message) for message in requests[4]['messages'] if message.get('role') == 'tool')
        app.send('\x04')
        app.exited()
        print('Actual dsh workbench: OpenCode shell landing/palette/tab mouse navigation and per-session drafts, shared session/workspace picker filtering/mouse/cancellation/F3 management, grouped long-model chooser mouse selection without hidden writes and explicit selection/readback/restore, agent landing, conversation layout/stats/source copy/history, model/preset controls, permission confirmation/cancellation/restore, GUI-shaped plugins page/settings/components/toggle cancellation/enable/restore, inventories, workspace files/pages, explicit schedule create/pause/delete, terminal bridge/detach/close, multiline draft/upload/send, schedule CAS conflict, both plan-review choices and exactly five local model requests OK')
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
