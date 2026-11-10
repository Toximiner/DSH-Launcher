"""Verify actual dsh against a controlled local model endpoint, never a paid LLM."""
import argparse
import json
import os
import re
from pathlib import Path
import select
import signal
import socket
import subprocess
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from terminal import Terminal, wait_for

parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--dsh', required=True, help='Installed dsh 0.2.2-rc.1; Node 24.9+ must be in PATH')
parser.add_argument('--capture-queue-edit', help='Optional terminal-cell JSON of an actual queue editor')
parser.add_argument('--capture-queue-remove', help='Optional terminal-cell JSON of the queue removal confirmation')
parser.add_argument('--capture-queue', help='Optional terminal-cell JSON of real queued follow-up tasks')
parser.add_argument('--capture-question', help='Optional terminal-cell JSON of an actual question form')
parser.add_argument('--capture-approval', help='Optional terminal-cell JSON of an actual pending permission')
parser.add_argument('--capture-tools', help='Optional terminal-cell JSON of actual tool results')
parser.add_argument('--capture-cells', help='Optional terminal-cell JSON for an actual UI screenshot')
args = parser.parse_args()
model_calls = []
queue_turns = {17: threading.Event(), 18: threading.Event(), 20: threading.Event(), 22: threading.Event()}


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
            if number in (11, 13, 16):
                names = [entry['function']['name'] for entry in body.get('tools', [])]
                tool = next((name for name in names if name == 'ask_user_question' or name.endswith('__ask_user_question')), None)
                assert tool, f'Question tool not offered: {names}'
                if number == 13:
                    schema = next(entry['function']['parameters'] for entry in body['tools'] if entry['function']['name'] == tool)
                    assert 'timeout' in schema.get('properties', {}), 'Timed question preset was not applied'
                if number == 11:
                    questions = [
                        {'id': 'language', 'header': 'Language', 'question': 'Which language should the agent use?',
                         'options': [{'label': 'Go', 'description': 'Use the current Go implementation.'}, {'label': 'Rust'}]},
                        {'id': 'checks', 'question': 'Which checks should run?', 'multi_select': True,
                         'options': [{'label': 'Unit tests'}, {'label': 'PTY checks'}, {'label': 'CI'}]},
                        {'id': 'notes', 'question': 'Add implementation notes.'},
                    ]
                else:
                    questions = [{'id': 'continue', 'question': 'Can we continue after reconnect?' if number == 13 else 'Cancel this unanswered question?',
                                  'options': [{'label': 'Continue'}, {'label': 'Pause'}]}]
                send({'role': 'assistant', 'tool_calls': [{'index': 0, 'id': f'question-{number}', 'type': 'function',
                      'function': {'name': tool, 'arguments': json.dumps({'questions': questions, 'timeout': 1 if number == 13 else -1})}}]})
                send({}, 'tool_calls')
                self.wfile.write(b'data: [DONE]\n\n')
                self.wfile.flush()
                return
            if number in (6, 8, 10):
                names = [entry['function']['name'] for entry in body.get('tools', [])]
                tool = next((name for name in names if name == 'tui_approval_probe' or name.endswith('__tui_approval_probe')), None)
                assert tool, f'Approval test tool not offered: {names}'
                marker = {6: 'grant', 8: 'reject', 10: 'cancel'}[number]
                send({'role': 'assistant', 'tool_calls': [
                    {'index': 0, 'id': 'approval-' + marker, 'type': 'function',
                     'function': {'name': tool, 'arguments': json.dumps({'marker': marker})}},
                ]})
                send({}, 'tool_calls')
                self.wfile.write(b'data: [DONE]\n\n')
                self.wfile.flush()
                return
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
            for index, text in enumerate([f'Ответ {number}: ', 'проверка транспорта ', 'настоящего dsh.']):
                send({'role': 'assistant', 'content': text})
                if number in queue_turns and index == 0:
                    assert queue_turns[number].wait(30), 'Queue test did not release the held model turn'
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
    probe = profile / 'approval-probe.mjs'
    probe.write_text(Path(__file__).with_name('approval_probe.mjs').read_text())
    (profile / 'cordis.yml').write_text('[]\n')
    dsh = Path(args.dsh).resolve()
    modules = next((path for path in dsh.parents if path.name == 'node_modules'), None)
    if modules:
        (profile / 'node_modules').symlink_to(modules, target_is_directory=True)
    (profile / 'cordis.patch.yml').write_text(json.dumps([
        {'insert': [{'id': 'tui-approval-probe', 'name': str(probe), 'config': {'directory': str(root / 'project')}}]},
        {'id': 'llm-pi-ai', 'config': {'providers': {'tui-fixture': {
            'apiKeyEnv': 'TUI_TEST_API_KEY', 'api': 'openai-completions',
            'baseURL': f'http://127.0.0.1:{model.server_port}/v1',
            'models': [{'id': 'fixture', 'contextWindow': 131072, 'maxTokens': 2048,
                        'reasoningEfforts': False, 'input': ['text']}],
            'retryPolicy': {'mode': 'normal', 'maxRetries': 0}}}}},
        {'id': 'agent-default-model', 'config': {'provider': 'tui-fixture', 'model': 'fixture'}},
        {'id': 'session-title-llm', 'disabled': True},
    ]))
    # Preset plugins are data inside the preset config, not top-level loader IDs.
    # Preserve the installed standard preset verbatim except for the question mode.
    preset = (modules / '@deepseek-ai/dsh-web-app/presets/standard.patch.yml').read_text()
    config = preset.split('      config:\n', 1)[1]
    plugin = "            name: '@deepseek-ai/dsh-tool-ask-user'\n"
    assert config.count(plugin) == 1, 'Pinned question preset shape changed'
    config = config.replace(plugin, plugin + '            config:\n              mode: timed\n')
    overlay = root / 'timed-questions.patch.yml'
    overlay.write_text('- id: preset-standard\n  config:\n' + ''.join(line[4:] if line.startswith('    ') else line for line in config.splitlines(keepends=True)))
    patch_file = profile / 'cordis.patch.yml'
    patches = json.loads(patch_file.read_text())
    patch_file.write_text(''.join('- ' + json.dumps(patch) + '\n' for patch in patches) + overlay.read_text())
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
            detail = '\n'.join(line for line in (root / 'backend.log').read_text().splitlines() if 'token=' not in line)[-3000:]
            raise RuntimeError('Actual dsh did not start in the isolated profile:\n' + detail)

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
        def input_has_focus():
            app.pump()
            return '> ▸ read · ! Ошибка' not in app.text and '> ▾ read · ! Ошибка' not in app.text
        wait_for(input_has_focus)
        app.send('\x15')
        app.command('Request permission to create the grant marker.')
        app.wait('Запрос разрешения dsh', timeout=20)
        app.wait('Create grant.marker', timeout=10)
        session_id = re.search(r'Сессия · (session-[a-z0-9-]+)', app.text).group(1)
        previous_app, app = app, None
        previous_app.close(root / 'before-approval-reconnect.ansi')
        app = Terminal(str(Path(args.binary).resolve()), ['--session', session_id], {**env, 'DSH_TUI_URL': login})
        app.resize(120, 42)
        app.wait('Create grant.marker', timeout=20)
        assert not (project / 'grant.marker').exists(), 'Tool ran before permission'
        app.send('\r')
        for _ in range(5):
            app.pump()
        assert not (project / 'grant.marker').exists(), 'Unselected Enter granted permission'
        if args.capture_approval:
            capture_cells(app, args.capture_approval)
        app.send('\t\x1b[C\r')
        app.wait('Ответ 7: проверка транспорта настоящего dsh.', timeout=20)
        assert (project / 'grant.marker').read_text() == 'approved\n', 'Approved action did not run'
        app.wait('Разрешено один раз', timeout=10)
        app.send('\x1b[5~' * 20)
        app.wait('Агент: не выполняется', timeout=20)
        app.command('Request permission to create the reject marker.')
        app.wait('Create reject.marker', timeout=20)
        app.send('\t\r')
        app.wait('Ответ 9: проверка транспорта настоящего dsh.', timeout=20)
        assert not (project / 'reject.marker').exists(), 'Rejected tool wrote its marker'
        app.wait('Отказано', timeout=10)
        app.send('\x1b[5~' * 20)
        app.wait('Агент: не выполняется', timeout=20)
        app.command('Request permission to create the cancel marker.')
        app.wait('Create cancel.marker', timeout=20)
        app.send('\x1b[19~')
        app.wait('Выполнение остановлено.', timeout=20)
        assert not (project / 'cancel.marker').exists(), 'Cancelled tool wrote its marker'
        assert len(model_calls) == 10, 'Unexpected approval model retry'
        app.command('Ask for language, checks, and free-form notes.')
        app.wait('Which language should the agent use?', timeout=20)
        app.send('\x1b')
        def question_deferred():
            app.pump()
            return 'Вопрос агента' not in app.text
        wait_for(question_deferred)
        app.send('retained chat draft')
        app.wait('> retained chat draft')
        app.send('\x1b[18~')
        app.wait('Вопрос агента')
        app.wait('Which language should the agent use?', timeout=10)
        app.send('\t\r\x1b[C')
        app.wait('Which checks should run?', timeout=10)
        app.send('\t\r\x1b[B ')
        app.wait('[✓] PTY checks')
        if args.capture_question:
            capture_cells(app, str(Path(args.capture_question).with_name('29-question-options-cells.json')))
        app.send('\x1b[C')
        app.wait('Add implementation notes.', timeout=10)
        app.send('\tTerminal questions work.\rKeep the chat draft.')
        app.wait('Keep the chat draft.', timeout=10)
        if args.capture_question:
            capture_cells(app, args.capture_question)
        app.send('\x1b[15~')
        app.wait('Ответ 12: проверка транспорта настоящего dsh.', timeout=20)
        app.wait('> retained chat draft', timeout=10)
        tool_results = [message for message in model_calls[11]['messages'] if message.get('role') == 'tool']
        answered = [message for message in tool_results if message.get('tool_call_id') == 'question-11']
        assert len(answered) == 1, 'Question answer was not returned as the tool result'
        text = answered[0]['content']
        if isinstance(text, list):
            text = next(block['text'] for block in text if block.get('type') == 'text')
        batch = json.loads(text)['answers']
        assert batch[0]['selected'] == ['Go'], batch
        assert batch[1]['selected'] == ['Unit tests', 'PTY checks'], batch
        assert batch[2]['custom'] == 'Terminal questions work.\nKeep the chat draft.', batch
        app.send('\x1b[5~' * 20)
        app.wait('Агент: не выполняется', timeout=20)
        app.send('\x15')
        app.command('Ask a timed question, then continue after disconnect.')
        app.wait('Can we continue after reconnect?', timeout=20)
        deadline = time.monotonic() + 2
        while time.monotonic() < deadline:
            app.pump()
        assert len(model_calls) == 13, 'Timed question expired while the TUI held its wait'
        session_id = re.search(r'Сессия · (session-[a-z0-9-]+)', app.text).group(1)
        previous_app, app = app, None
        previous_app.close(root / 'before-question-reconnect.ansi')
        wait_for(lambda: len(model_calls) >= 14, timeout=10)
        app = Terminal(str(Path(args.binary).resolve()), ['--session', session_id], {**env, 'DSH_TUI_URL': login})
        app.resize(120, 42)
        app.wait('Can we continue after reconnect?', timeout=20)
        app.send('\t\r\x1b[15~')
        app.wait('Ответ 15: проверка транспорта настоящего dsh.', timeout=20)
        assert any('answer_to_pending_question' in json.dumps(message) for message in model_calls[14]['messages']), 'Continued reply was not tagged by dsh'
        app.send('\x1b[5~' * 20)
        app.wait('Агент: не выполняется', timeout=20)
        app.command('Ask one more question for cancellation.')
        app.wait('Cancel this unanswered question?', timeout=20)
        app.send('\x1b[19~')
        app.wait('Выполнение остановлено.', timeout=20)
        assert len(model_calls) == 16, 'Unexpected question retry or duplicate reply'
        wait_for(question_deferred)
        app.command('QUEUE FIRST')
        app.wait('Ответ 17: ', timeout=20)
        app.wait('Enter — в очередь')
        app.command('QUEUE SECOND')
        app.wait('ОЧЕРЕДЬ DSH · 1', timeout=15)
        app.wait('QUEUE SECOND')
        app.command('QUEUE THIRD')
        app.wait('ОЧЕРЕДЬ DSH · 2', timeout=15)
        app.wait('QUEUE THIRD')
        assert len(model_calls) == 17, 'Queued prompt interrupted the running model turn'
        previous_app, app = app, None
        previous_app.close(root / 'before-queue-reconnect.ansi')
        app = Terminal(str(Path(args.binary).resolve()), ['--session', session_id], {**env, 'DSH_TUI_URL': login})
        app.resize(120, 42)
        app.wait('ОЧЕРЕДЬ DSH · 2', timeout=20)
        app.wait('Ответ 17: ', timeout=10)
        assert len(model_calls) == 17, 'Queue reconnection resent a prompt'
        for _ in range(3):
            app.pump()
        if args.capture_queue:
            capture_cells(app, args.capture_queue)
        queue_turns[17].set()
        app.wait('Ответ 18: ', timeout=20)
        app.wait('ОЧЕРЕДЬ DSH · 1', timeout=10)
        queue_turns[18].set()
        app.wait('Ответ 19: проверка транспорта настоящего dsh.', timeout=20)
        def queue_drained():
            app.pump()
            return 'ОЧЕРЕДЬ DSH ·' not in app.text
        wait_for(queue_drained)
        app.send('\x1b[5~' * 20)
        app.wait('Агент: не выполняется', timeout=20)
        assert len(model_calls) == 19, 'Queue execution retried or lost a model turn'
        markers = ['QUEUE FIRST', 'QUEUE SECOND', 'QUEUE THIRD']
        for number, expected in zip([17, 18, 19], markers):
            seen = [marker for message in model_calls[number - 1]['messages'] if message.get('role') == 'user'
                    for marker in markers if marker in json.dumps(message.get('content', ''))]
            assert seen[-1] == expected, f'Queued tasks executed out of order at turn {number}: {seen}'
        app.command('REMOVE TEST ACTIVE')
        app.wait('Ответ 20: ', timeout=20)
        app.command('REMOVE THIS PENDING TASK')
        app.wait('ОЧЕРЕДЬ DSH · 1', timeout=15)
        app.command('KEEP THIS PENDING TASK')
        app.wait('ОЧЕРЕДЬ DSH · 2', timeout=15)
        app.send('\x1b[20~')
        app.wait('Очередь заданий dsh')
        app.send('\r')
        app.wait('Удалить задание из очереди?')
        app.send('\r')
        app.wait('Удаление отменено.')
        app.send('\r')
        app.wait('Удалить задание из очереди?')
        if args.capture_queue_remove:
            capture_cells(app, args.capture_queue_remove)
        app.send('\x1b[C\r')
        app.wait('Задание удалено.', timeout=15)
        app.send('\x1b')
        app.wait('ОЧЕРЕДЬ DSH · 1', timeout=15)
        assert 'REMOVE THIS PENDING TASK' not in app.text, 'Removed task remained in the queue'
        assert len(model_calls) == 20, 'Queue removal interrupted or started another model turn'
        previous_app, app = app, None
        previous_app.close(root / 'after-queue-removal.ansi')
        app = Terminal(str(Path(args.binary).resolve()), ['--session', session_id], {**env, 'DSH_TUI_URL': login})
        app.resize(80, 24)
        app.wait('ОЧЕРЕДЬ DSH · 1', timeout=20)
        app.send('\x1b[20~')
        app.wait('Очередь заданий dsh')
        app.wait('KEEP THIS PENDING TASK')
        app.resize(44, 18)
        app.wait('Очередь заданий dsh')
        app.wait('[ Удалить выбранное ]')
        assert 'REMOVE THIS PENDING TASK' not in app.text, 'Reconnection restored a removed task'
        app.send('\x1b')
        app.resize(120, 42)
        queue_turns[20].set()
        app.wait('Ответ 21: проверка транспорта настоящего dsh.', timeout=20)
        wait_for(queue_drained)
        app.send('\x1b[5~' * 20)
        app.wait('Агент: не выполняется', timeout=20)
        assert len(model_calls) == 21, 'Removed task executed or a retained task was lost'
        assert not any('REMOVE THIS PENDING TASK' in json.dumps(message) for message in model_calls[20]['messages']), 'Removed prompt leaked into model history'
        assert any('KEEP THIS PENDING TASK' in json.dumps(message) for message in model_calls[20]['messages']), 'Remaining queued task was not executed'
        app.command('EDIT TEST ACTIVE')
        app.wait('Ответ 22: ', timeout=20)
        app.command('EDIT ORIGINAL TASK')
        app.wait('ОЧЕРЕДЬ DSH · 1', timeout=15)
        app.command('EDIT KEEP NEXT TASK')
        app.wait('ОЧЕРЕДЬ DSH · 2', timeout=15)
        app.send('\x1b[20~')
        app.wait('Очередь заданий dsh')
        app.send('\x1bOR')
        app.wait('Изменить задание в очереди')
        app.wait('EDIT ORIGINAL TASK')
        app.send('\x15')
        app.send('UNSAVED EDIT')
        app.wait('UNSAVED EDIT')
        app.send('\x1b')
        app.wait('Изменения не сохранены.')
        app.wait('EDIT ORIGINAL TASK')
        app.send('\x1bOR')
        app.wait('Изменить задание в очереди')
        app.send('\x15')
        app.send('EDIT CHANGED TASK\rsecond line kept')
        app.wait('second line kept')
        app.resize(44, 18)
        app.wait('Изменить задание в очереди')
        app.wait('[ Сохранить F5 ]')
        app.resize(120, 42)
        def editor_resized():
            app.pump()
            return '[ Сохранить F5 ]' in app.screen.display[30]
        wait_for(editor_resized)
        app.wait('second line kept')
        if args.capture_queue_edit:
            capture_cells(app, args.capture_queue_edit)
        app.send('\x1b[15~')
        app.wait('Задание изменено.', timeout=15)
        app.wait('EDIT CHANGED TASK', timeout=15)
        app.send('\x1b')
        app.wait('ОЧЕРЕДЬ DSH · 2')
        assert len(model_calls) == 22, 'Queue editing interrupted the active turn'
        previous_app, app = app, None
        previous_app.close(root / 'after-queue-edit.ansi')
        app = Terminal(str(Path(args.binary).resolve()), ['--session', session_id], {**env, 'DSH_TUI_URL': login})
        app.resize(120, 42)
        app.wait('ОЧЕРЕДЬ DSH · 2', timeout=20)
        app.wait('EDIT CHANGED TASK')
        app.wait('second line kept')
        assert 'EDIT ORIGINAL TASK' not in app.text and 'UNSAVED EDIT' not in app.text, 'Queue edit was lost or a discarded draft was saved'
        queue_turns[22].set()
        app.wait('Ответ 24: проверка транспорта настоящего dsh.', timeout=25)
        wait_for(queue_drained)
        app.send('\x1b[5~' * 20)
        app.wait('Агент: не выполняется', timeout=20)
        assert len(model_calls) == 24, 'Editing duplicated or lost a queued turn'
        changed = [message for message in model_calls[22]['messages'] if message.get('role') == 'user'][-1]
        retained = [message for message in model_calls[23]['messages'] if message.get('role') == 'user'][-1]
        assert 'EDIT CHANGED TASK' in json.dumps(changed) and 'second line kept' in json.dumps(changed), 'Model did not receive the saved multiline edit'
        assert 'EDIT KEEP NEXT TASK' in json.dumps(retained), 'Editing changed FIFO order or another task'
        assert not any('EDIT ORIGINAL TASK' in json.dumps(message) or 'UNSAVED EDIT' in json.dumps(message) for message in model_calls[22]['messages']), 'Original or discarded text entered model history'
        print('Actual dsh queue edit: discard, multiline save, minimum layout, reconnect, changed model input and retained FIFO order OK')
        print('Actual dsh queue removal: explicit confirmation, cancellation preserves active turn and other item, reconnect and minimum layout OK')
        print('Actual dsh queue: two follow-ups while streaming, server queue confirmation, individual FIFO turns, no duplicates OK')
        print('Actual dsh questions: single/multi-select, multiline custom answer, draft preservation, timed hold, continued reply, cancellation OK')
        print('Actual dsh approvals: explicit grant executes, rejection/cancellation do not; audit restored OK')
        print('Actual dsh + controlled model: file-read success/failure, result cards, expansion, draft preservation OK')
        print('Actual dsh + controlled model: submission, partial streaming, two turns, cancellation OK')
        print('This verifies protocol integration, not a real cloud LLM response.')
        app.send('\x04')
        app.exited()
    finally:
        for release in queue_turns.values():
            release.set()
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
