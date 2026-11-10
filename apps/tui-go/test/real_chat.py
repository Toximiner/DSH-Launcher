"""Verify actual dsh against a controlled local model endpoint, never a paid LLM."""
import argparse
import json
import os
from pathlib import Path
import select
import signal
import socket
import subprocess
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from terminal import Terminal

parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--dsh', required=True, help='Installed dsh 0.2.2-rc.1; Node 24.9+ must be in PATH')
parser.add_argument('--capture-tools', help='Optional terminal-cell JSON of actual tool results')
parser.add_argument('--capture-cells', help='Optional terminal-cell JSON for an actual UI screenshot')
args = parser.parse_args()
model_calls = []


class Model(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        assert self.path == '/v1/chat/completions'
        assert body['stream'] is True
        model_calls.append(body)
        number = len(model_calls)
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.end_headers()

        def send(delta, finish=None):
            chunk = {'id': 'test', 'object': 'chat.completion.chunk', 'model': 'fixture',
                     'choices': [{'index': 0, 'delta': delta, 'finish_reason': finish}]}
            self.wfile.write(('data: ' + json.dumps(chunk) + '\n\n').encode())
            self.wfile.flush()

        try:
            if number == 4:
                names = [entry['function']['name'] for entry in body.get('tools', [])]
                read_name = next((name for name in names if name == 'read' or name.endswith('__read')), None)
                assert read_name, f'Read tool not offered: {names}'
                send({'role': 'assistant', 'tool_calls': [
                    {'index': 0, 'id': 'read-notes', 'type': 'function',
                     'function': {'name': read_name, 'arguments': json.dumps({'file_path': str(project / 'notes.txt')})}},
                    {'index': 1, 'id': 'read-missing', 'type': 'function',
                     'function': {'name': read_name, 'arguments': json.dumps({'file_path': str(project / 'missing-file.txt')})}},
                ]})
                send({}, 'tool_calls')
                self.wfile.write(b'data: [DONE]\n\n')
                self.wfile.flush()
                return
            for text in [f'Ответ {number}: ', 'проверка транспорта ', 'настоящего dsh.']:
                send({'role': 'assistant', 'content': text})
                time.sleep(0.7)
            send({}, 'stop')
            self.wfile.write(b'data: [DONE]\n\n')
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass


def capture_cells(app, path):
    cells = []
    for y in range(app.screen.lines):
        for x in range(app.screen.columns):
            cell = app.screen.buffer[y][x]
            cells.append({'x': x, 'y': y, 'text': cell.data, 'fg': cell.fg,
                          'bg': cell.bg, 'bold': cell.bold, 'reverse': cell.reverse})
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps({'source': 'Actual dsh with a controlled test model endpoint',
                                 'columns': app.screen.columns, 'rows': app.screen.lines,
                                 'cells': cells}, ensure_ascii=False))


with tempfile.TemporaryDirectory(prefix='dsh-tui-real-chat-') as home:
    root = Path(home)
    model = ThreadingHTTPServer(('127.0.0.1', 0), Model)
    threading.Thread(target=model.serve_forever, daemon=True).start()
    profile = root / 'dsh/profiles/web'
    profile.mkdir(parents=True)
    (profile / 'package.json').write_text(json.dumps({
        'name': 'dsh-tui-isolated-test', 'private': True, 'dependencies': {},
        'dsh': {'profile': {'bundles': ['@deepseek-ai/dsh-base', '@deepseek-ai/dsh-web-app']}}}))
    (profile / 'cordis.yml').write_text('[]\n')
    dsh = Path(args.dsh).resolve()
    modules = next((path for path in dsh.parents if path.name == 'node_modules'), None)
    if modules:
        (profile / 'node_modules').symlink_to(modules, target_is_directory=True)
    (profile / 'cordis.patch.yml').write_text(json.dumps([
        {'id': 'llm-pi-ai', 'config': {'providers': {'tui-fixture': {
            'apiKeyEnv': 'TUI_TEST_API_KEY', 'api': 'openai-completions',
            'baseURL': f'http://127.0.0.1:{model.server_port}/v1',
            'models': [{'id': 'fixture', 'contextWindow': 131072, 'maxTokens': 2048,
                        'reasoningEfforts': False, 'input': ['text']}],
            'retryPolicy': {'mode': 'normal', 'maxRetries': 0}}}}},
        {'id': 'agent-default-model', 'config': {'provider': 'tui-fixture', 'model': 'fixture'}},
        {'id': 'session-title-llm', 'disabled': True},
    ]))
    project = root / 'project'
    project.mkdir()
    (project / 'notes.txt').write_text('\n'.join(f'Tool output line {i:02d}' for i in range(1, 13)) + '\n')
    env = {key: value for key, value in os.environ.items()
           if key not in ('GH_TOKEN', 'GITHUB_TOKEN', 'CODESPACES_GITHUB_TOKEN', 'DSH_TUI_URL')}
    env.pop('NO_COLOR', None)
    env.update(DSH_HOME=str(root / 'dsh'), TUI_TEST_API_KEY='test-only-not-a-real-key',
               TERM='xterm-256color', COLORTERM='truecolor', XDG_CONFIG_HOME=str(root / 'config'))
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    process = subprocess.Popen([str(dsh), '--profile', 'web', '--no-open', '--port', str(port)],
                               cwd=project, env=env, stdout=subprocess.PIPE,
                               stderr=subprocess.STDOUT, text=True, start_new_session=True)
    app = None
    log = (root / 'backend.log').open('w')
    try:
        login = None
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline and process.poll() is None:
            if select.select([process.stdout], [], [], 0.5)[0]:
                line = process.stdout.readline()
                log.write(line)
                log.flush()
                if 'dsh web: ' in line:
                    login = line.split('dsh web: ')[1].strip()
                    break
        if not login:
            raise RuntimeError('Actual dsh did not start in the isolated profile')

        def drain():
            for line in process.stdout:
                log.write(line)
                log.flush()

        reader = threading.Thread(target=drain, daemon=True)
        reader.start()
        app = Terminal(str(Path(args.binary).resolve()), ['--new'], {**env, 'DSH_TUI_URL': login})
        app.wait('Введите сообщение.', timeout=20)
        app.command('Reply briefly without invoking any tools.')
        app.wait('Ответ 1: ', timeout=25)
        assert 'настоящего dsh.' not in app.text, 'Intermediate streaming state was not observed'
        app.wait('Ответ 1: проверка транспорта настоящего dsh.', timeout=20)
        app.wait('Агент: не выполняется', timeout=20)
        app.command('A second turn without tools.')
        app.wait('Ответ 2: проверка транспорта настоящего dsh.', timeout=20)
        app.send('\x1b[5~')
        app.wait('Агент: не выполняется', timeout=20)
        app.command('Another brief answer, no tools.')
        app.wait('Ответ 3: ', timeout=20)
        if args.capture_cells:
            capture_cells(app, args.capture_cells)
        app.send('\x1b[19~')
        app.wait('Выполнение остановлено.', timeout=20)
        assert len(model_calls) == 3, 'Unexpected model request retry or missing generation'
        app.command('Read notes.txt and missing-file.txt using tools.')
        app.wait('Ответ 5: проверка транспорта настоящего dsh.', timeout=25)
        app.send('\x1b[5~' * 10)
        app.wait('Агент: не выполняется', timeout=20)
        assert len(model_calls) == 5, 'Unexpected tool-turn model request retry'
        results = [message for message in model_calls[4]['messages'] if message.get('role') == 'tool']
        assert len(results) == 2, 'Actual dsh did not feed both tool results back to the model'
        assert any('Tool output line 12' in json.dumps(message) for message in results), 'Actual filesystem read did not return the test file'
        app.resize(120, 42)
        app.send('preserved draft')
        app.send('\x1b[17~')
        app.wait('> ▸ read ·', timeout=10)
        if '> ▸ read · ! Ошибка' in app.text:
            app.send('\x1b[17~')
        app.wait('> ▸ read · ✓ Готово', timeout=10)
        app.send('\r')
        app.wait('Tool output line 12', timeout=10)
        if args.capture_tools:
            capture_cells(app, args.capture_tools)
        app.send('\x1b[17~')
        app.wait('> ▸ read · ! Ошибка', timeout=10)
        app.send('\x1b')
        app.wait('> preserved draft')
        print('Actual dsh + controlled model: file-read success/failure, result cards, expansion, draft preservation OK')
        print('Actual dsh + controlled model: submission, partial streaming, two turns, cancellation OK')
        print('This verifies protocol integration, not a real cloud LLM response.')
        app.send('\x04')
        app.exited()
    finally:
        if app:
            app.close(root / 'terminal.ansi')
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGTERM)
        process.wait(timeout=10)
        if 'reader' in locals():
            reader.join(timeout=2)
        process.stdout.close()
        log.close()
        model.shutdown()
