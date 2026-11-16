"""Revision-checked forms and write-only credentials against isolated actual dsh."""
import argparse
import http.cookiejar
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import re
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
parser.add_argument('--capture', help='Prefix for actual credential form terminal-cell captures')
args = parser.parse_args()
requests = []
new_key = 'isolated-fake-key-from-tui'

class Model(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        assert self.headers.get('Authorization') == 'Bearer ' + new_key
        requests.append(body)
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.end_headers()
        chunk = {'id': 'settings-test', 'object': 'chat.completion.chunk', 'model': 'fixture',
                 'choices': [{'index': 0, 'delta': {'role': 'assistant', 'content': 'SETTINGS KEY ACCEPTED'}, 'finish_reason': 'stop'}]}
        self.wfile.write(('data: ' + json.dumps(chunk) + '\n\ndata: [DONE]\n\n').encode())

class ConnectResponse(urllib.request.HTTPErrorProcessor):
    def http_response(self, request, response):
        return response if response.status == 302 else super().http_response(request, response)

with tempfile.TemporaryDirectory(prefix='dsh-tui-settings-') as directory:
    root = Path(directory)
    model = ThreadingHTTPServer(('127.0.0.1', 0), Model)
    threading.Thread(target=model.serve_forever, daemon=True).start()
    endpoint = f'http://127.0.0.1:{model.server_port}/v1'
    profile = root / 'dsh/profiles/web'
    profile.mkdir(parents=True)
    dsh = Path(args.dsh).resolve()
    modules = next(path for path in dsh.parents if path.name == 'node_modules')
    (profile / 'node_modules').symlink_to(modules, target_is_directory=True)
    (profile / 'package.json').write_text(json.dumps({'name': 'isolated-settings-test', 'private': True,
        'dependencies': {}, 'dsh': {'profile': {'bundles': ['@deepseek-ai/dsh-base', '@deepseek-ai/dsh-web-app']}}}))
    (profile / 'cordis.yml').write_text('[]\n')
    (profile / 'cordis.patch.yml').write_text(json.dumps([
        {'id': 'llm-pi-ai', 'config': {'providers': {'settings-fixture': {
            'apiKeyEnv': 'SETTINGS_TEST_KEY', 'api': 'openai-completions', 'baseURL': endpoint,
            'models': [{'id': 'fixture', 'contextWindow': 131072, 'maxTokens': 256, 'reasoningEfforts': False, 'input': ['text']}],
            'retryPolicy': {'mode': 'normal', 'maxRetries': 0}}}}},
        {'id': 'agent-default-model', 'config': {'provider': 'settings-fixture', 'model': 'fixture'}},
        {'id': 'session-title-llm', 'disabled': True}]))
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
        def namespace():
            return next(ns for ns in rpc('settings/describe', {})['value']['namespaces'] if ns['ns'] == 'llm-pi-ai')
        def choose(name):
            def current():
                app.pump()
                match = re.search(r'› ([^=\n]+?) =', app.text)
                return match.group(1).strip() if match else None
            for _ in range(100):
                previous = current()
                if previous == name:
                    return
                app.send('\x1b[B')
                wait_for(lambda: current() != previous)
            raise AssertionError('Field not found: ' + name)
        def finish():
            # Drain the final PTY frames before checking terminal restoration.
            wait_for(lambda: (app.pump(), app.proc.poll() is not None and b'\x1b[?1049l' in app.raw)[1])
            app.exited()
        def edit(name, value):
            choose(name)
            app.send('\r')
            app.wait('Поле:')
            app.send('\x01\x0b' + value + '\r')
            wait_for(lambda: (app.pump(), 'Поле:' not in app.text and 'Изменений:' in app.text)[1])
        def enter_providers():
            app.command('/providers')
            app.wait('Настройки · llm-pi-ai')
            app.wait('settings-fixture =')
            app.send('\r')
            app.wait('/providers/settings-fixture')
        binary = str(Path(args.binary).resolve())
        app = Terminal(binary, ['--new'], {**env, 'DSH_TUI_URL': login[0]}, columns=100, rows=30)
        app.wait('Введите сообщение.', timeout=20)
        enter_providers()
        choose('timeoutMs')
        app.send('\r')
        app.wait('Поле:')
        app.send('\x01\x0b-1\r')
        app.wait('Минимум по схеме')
        assert 'timeoutMs' not in namespace()['value']['providers']['settings-fixture']
        app.send('\x1b')
        wait_for(lambda: (app.pump(), 'Поле:' not in app.text)[1])
        edit('displayName', 'Saved through TUI')
        app.send('\x1b[15~')
        app.wait('Настройки сохранены', timeout=20)
        saved = namespace()
        assert saved['value']['providers']['settings-fixture']['displayName'] == 'Saved through TUI'
        assert saved['value']['providers']['settings-fixture']['baseURL'] == endpoint
        edit('timeoutMs', '12000')
        external = rpc('settings/mutate', {'ns': saved['ns'], 'expectedRevision': saved['revision'],
            'ops': [{'op': 'set', 'path': ['providers', 'settings-fixture', 'displayName'], 'value': 'Other client'}]})
        assert external['ok']
        app.send('\x1b[15~')
        app.wait('Раздел изменён другим клиентом', timeout=20)
        app.send('\x12')
        app.wait('Перечитана свежая ревизия')
        app.send('\x1b[15~')
        app.wait('Настройки сохранены')
        saved = namespace()
        assert saved['value']['providers']['settings-fixture']['displayName'] == 'Other client'
        assert saved['value']['providers']['settings-fixture'].get('timeoutMs') == 12000, (saved['value']['providers']['settings-fixture'], app.text)
        edit('displayName', 'Local draft')
        assert rpc('settings/mutate', {'ns': saved['ns'], 'expectedRevision': saved['revision'],
            'ops': [{'op': 'set', 'path': ['providers', 'settings-fixture', 'displayName'], 'value': 'Concurrent value'}]})['ok']
        app.send('\x1b[15~')
        app.wait('Раздел изменён другим клиентом')
        app.send('\x12')
        app.wait('Конфликт полей')
        app.send('\x1b[15~')
        assert namespace()['value']['providers']['settings-fixture']['displayName'] == 'Concurrent value'
        app.send('\x18')
        app.wait('Правка отменена')
        choose('apiKeyEnv')
        app.send('\x0b')
        app.wait('Существующий ключ скрыт')
        app.wait('Состояние: задан')
        primary = app
        try:
            for columns, rows in [(120, 42), (100, 30), (80, 24)]:
                app = Terminal(binary, ['--new'], {**env, 'DSH_TUI_URL': login[0]}, columns=columns, rows=rows)
                try:
                    app.wait('Введите сообщение.', timeout=20)
                    enter_providers()
                    choose('apiKeyEnv')
                    app.send('\x0b')
                    app.wait('Существующий ключ скрыт')
                    app.wait('Состояние: задан')
                    app.send('mask-check-value')
                    def masked():
                        app.pump()
                        return '•' in app.text
                    wait_for(masked)
                    assert 'mask-check-value' not in app.text and 'isolated-original-key' not in app.text
                    if args.capture:
                        cells = [{'x': x, 'y': y, 'text': (cell := app.screen.buffer[y][x]).data,
                                  'fg': cell.fg, 'bg': cell.bg, 'bold': cell.bold, 'reverse': cell.reverse}
                                 for y in range(rows) for x in range(columns)]
                        target = Path(args.capture + f'-{columns}x{rows}.json')
                        target.parent.mkdir(parents=True, exist_ok=True)
                        target.write_text(json.dumps({'source': 'Actual isolated dsh credentials form', 'columns': columns, 'rows': rows, 'cells': cells}, ensure_ascii=False))
                    app.send('\x1b')
                    app.wait('Настройки · llm-pi-ai', absent=('API-ключ · SETTINGS_TEST_KEY',))
                    choose('models')
                    app.send('\x05')
                    app.wait('Принять в форму; затем F5')
                    if args.capture:
                        cells = [{'x': x, 'y': y, 'text': (cell := app.screen.buffer[y][x]).data,
                                  'fg': cell.fg, 'bg': cell.bg, 'bold': cell.bold, 'reverse': cell.reverse}
                                 for y in range(rows) for x in range(columns)]
                        Path(args.capture + f'-json-{columns}x{rows}.json').write_text(json.dumps(
                            {'source': 'Actual isolated dsh JSON form', 'columns': columns, 'rows': rows, 'cells': cells}, ensure_ascii=False))
                    app.send('\x1b')
                    wait_for(lambda: (app.pump(), 'многострочный редактор' not in app.text)[1])
                    app.send('\x04')
                    finish()
                except Exception:
                    print(f'Credential scenario failed at {columns}x{rows}:\n{app.text}', flush=True)
                    raise
                finally:
                    app.close(root / f'settings-{columns}x{rows}.ansi')
        finally:
            app = primary
        app.send(new_key + '\r')
        app.wait('Новый ключ записан', timeout=20)
        info = rpc('credentials/describe', {'refs': ['SETTINGS_TEST_KEY']})['value']['SETTINGS_TEST_KEY']
        assert info['configured'] and info['source'] == 'file' and new_key not in json.dumps(info)
        for path in ['providers', '']:
            app.send('\x1b')
            wait_for(lambda: (app.pump(), re.search(r'/'+path+r'\s+│', app.text))[1])
        app.send('\x1b')
        app.wait('Введите сообщение.')
        app.command('Verify the configured credential with the isolated model.')
        app.wait('SETTINGS KEY ACCEPTED', timeout=25)
        assert len(requests) == 1
        app.command('/providers')
        app.wait('settings-fixture =')
        app.send('\x0e')
        app.wait('Имя:')
        app.send('tui-added\r')
        app.wait('/providers/tui-added')
        edit('api', '"openai-completions"')
        edit('apiKeyEnv', 'SETTINGS_TEST_KEY')
        edit('baseURL', endpoint)
        choose('models')
        app.send('\r')
        app.wait('/providers/tui-added/models')
        app.send('\x0e')
        app.wait('/providers/tui-added/models/0')
        edit('id', 'fixture-added')
        edit('reasoningEfforts', 'false')
        app.send('\x1b[15~')
        app.wait('Настройки сохранены', timeout=20)
        added = namespace()['value']['providers']['tui-added']
        assert added['api'] == 'openai-completions' and added['baseURL'] == endpoint
        assert added['models'][0]['id'] == 'fixture-added' and added['models'][0]['reasoningEfforts'] is False
        app.send('\x1b')
        wait_for(lambda: (app.pump(), re.search(r'/providers/tui-added/models\s+│', app.text))[1])
        app.send('\x1b')
        wait_for(lambda: (app.pump(), re.search(r'/providers/tui-added\s+│', app.text))[1])
        choose('models')
        app.send('\x05')
        app.wait('Принять в форму; затем F5')
        # Delete the known JSON source line by line, preserving the editor mode.
        app.send('\x1b<' + '\x0b' * (2 * (json.dumps(added['models'], indent=2).count('\n') + 1) + 4))
        updated_models = [*added['models'], {'id': 'fixture-json', 'name': 'First line\nUnicode: привет', 'reasoningEfforts': False}]
        app.send('\x1b[200~' + json.dumps(updated_models, indent=2, ensure_ascii=False) + '\x1b[201~')
        app.send('\x1b[15~')
        wait_for(lambda: (app.pump(), 'многострочный редактор' not in app.text and 'Изменений:' in app.text)[1])
        # The first F5 only stages; the backend still has the original list.
        assert len(namespace()['value']['providers']['tui-added']['models']) == 1
        app.send('\x1b[15~')
        app.wait('Настройки сохранены', timeout=20)
        persisted = namespace()['value']['providers']['tui-added']['models']
        assert len(persisted) == 2 and persisted[0]['id'] == 'fixture-added'
        assert persisted[1]['id'] == 'fixture-json' and persisted[1]['name'] == 'First line\nUnicode: привет'
        app.send('\r')
        app.wait('/providers/tui-added/models')
        choose('1')
        app.send('\r')
        app.wait('/providers/tui-added/models/1')
        choose('name')
        app.send('\r')
        app.wait('Принять в форму; затем F5')
        app.wait('Unicode: привет')
        app.send('\x1b')
        wait_for(lambda: (app.pump(), 'многострочный редактор' not in app.text)[1])
        for path in ['providers/tui-added/models', 'providers/tui-added', 'providers', '']:
            app.send('\x1b')
            wait_for(lambda: (app.pump(), re.search(r'/'+path+r'\s+│', app.text))[1])
        app.send('\x1b')
        wait_for(lambda: (app.pump(), 'Настройки · llm-pi-ai' not in app.text)[1])
        app.send('\x1b[24~')
        app.wait('Модель сессии')
        app.send('tui-added')
        app.wait('fixture-added')
        app.send('\x1b')
        wait_for(lambda: (app.pump(), 'Модель сессии' not in app.text)[1])
        app.send('\x04')
        finish()
        print('Actual dsh settings: schema form, live provider edit and creation, new model catalog entry, multiline JSON/string editing, local validation, exact path writes, revision conflict, retained draft, same-field review, other-client preservation and three sizes OK')
        print('Actual dsh credentials: existing key never returned, masked input, explicit write, stored status and next model request using the new key OK; no paid calls or user-profile edits')
    except Exception:
        if app:
            print(app.text, flush=True)
        raise
    finally:
        if app:
            app.close(root / 'settings-terminal.ansi')
        if backend.poll() is None:
            os.killpg(backend.pid, signal.SIGTERM)
        backend.wait(timeout=15)
        reader.join(timeout=2)
        backend.stdout.close()
        model.shutdown()
