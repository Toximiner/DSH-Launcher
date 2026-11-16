"""Read multiple history pages from actual dsh using an isolated, local model."""
import argparse
import base64
import re
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
parser.add_argument('--capture-syntax', help='Prefix for syntax-colored history terminal-cell captures')
parser.add_argument('--capture-copy', help='Prefix for code-copy dialog terminal-cell captures')
parser.add_argument('--capture-markdown', help='Prefix for actual formatted Markdown history captures')
parser.add_argument('--capture-archive', help='Prefix for archive confirmation captures')
parser.add_argument('--capture-history', help='Prefix for actual terminal-cell JSON captures')
parser.add_argument('--capture-exact-fork', help='Prefix for actual exact-message fork captures')
parser.add_argument('--capture-fork', help='Prefix for actual fork confirmation captures')
parser.add_argument('--capture-rename', help='Prefix for actual rename editor captures')
parser.add_argument('--capture-search', help='Prefix for actual history search terminal-cell JSON captures')
args = parser.parse_args()
calls = []
release = threading.Event()
markdown_reply = '# MARKDOWN CHECK\n\n**Bold text**, *italic text*, and `inline_code`.\n\n- first item\n- second item\n\n> quoted text\n\n```go\nfmt.Println("世界")\n```\n\n```python\n\tprint("Код 🙂")\n\n```\n\n| Name | Result |\n| --- | ---: |\n| Кириллица | TABLE_32 |\n\n[Docs](https://example.test/docs)\n\nARCHIVE ANSWER 032'



class Model(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        assert self.path == '/v1/chat/completions' and body['stream']
        calls.append(body)
        number = len(calls)
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.end_headers()

        def send(text, finish=None):
            chunk = {'id': 'history-test', 'object': 'chat.completion.chunk', 'model': 'fixture',
                     'choices': [{'index': 0, 'delta': {'role': 'assistant', 'content': text},
                                  'finish_reason': finish}]}
            self.wfile.write(('data: ' + json.dumps(chunk) + '\n\n').encode())
            self.wfile.flush()

        try:
            if number == 27:
                send('ARCHIVE ACTIVE PARTIAL\n')
                assert release.wait(90), 'Active history test was not released'
            send(markdown_reply if number == 32 else f'ARCHIVE ANSWER {number:03d}', 'stop')
            self.wfile.write(b'data: [DONE]\n\n')
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass


class ConnectResponse(urllib.request.HTTPErrorProcessor):
    def http_response(self, request, response):
        return response if response.status == 302 else super().http_response(request, response)


def capture(app, path):
    def painted():
        app.pump()
        return 'PgUp/PgDn' in app.text and 'Чтение истории' not in app.text and 'Поиск по истории…' not in app.text
    try:
        wait_for(painted)
    except AssertionError:
        raise AssertionError(f'History capture did not finish painting:\n{app.text}') from None
    capture_cells(app, path)


def capture_cells(app, path):
    cells = []
    for y in range(app.screen.lines):
        for x in range(app.screen.columns):
            cell = app.screen.buffer[y][x]
            cells.append({'x': x, 'y': y, 'text': cell.data, 'fg': cell.fg,
                          'bg': cell.bg, 'bold': cell.bold, 'reverse': cell.reverse})
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps({'source': 'Actual dsh history with a controlled local model',
                                 'columns': app.screen.columns, 'rows': app.screen.lines,
                                 'cells': cells}, ensure_ascii=False))


with tempfile.TemporaryDirectory(prefix='dsh-tui-real-history-') as directory:
    root = Path(directory)
    model = ThreadingHTTPServer(('127.0.0.1', 0), Model)
    threading.Thread(target=model.serve_forever, daemon=True).start()
    profile = root / 'dsh/profiles/web'
    profile.mkdir(parents=True)
    (profile / 'package.json').write_text(json.dumps({
        'name': 'tui-history-isolated-test', 'private': True, 'dependencies': {},
        'dsh': {'profile': {'bundles': ['@deepseek-ai/dsh-base', '@deepseek-ai/dsh-web-app']}}}))
    (profile / 'history-probe.mjs').write_text(Path(__file__).with_name('history_probe.mjs').read_text())
    (profile / 'cordis.yml').write_text('[]\n')
    (profile / 'cordis.patch.yml').write_text(json.dumps([
        {'insert': [{'id': 'tui-history-probe', 'name': str(profile / 'history-probe.mjs'), 'config': {'directory': str(root)}}]},
        {'id': 'llm-pi-ai', 'config': {'providers': {'history-fixture': {
            'apiKeyEnv': 'TUI_TEST_API_KEY', 'api': 'openai-completions',
            'baseURL': f'http://127.0.0.1:{model.server_port}/v1',
            'models': [{'id': 'fixture', 'contextWindow': 131072, 'maxTokens': 256,
                        'reasoningEfforts': False, 'input': ['text']}],
            'retryPolicy': {'mode': 'normal', 'maxRetries': 0}}}}},
        {'id': 'agent-default-model', 'config': {'provider': 'history-fixture', 'model': 'fixture'}},
        {'id': 'session-title-llm', 'disabled': True},
    ]))
    dsh = Path(args.dsh).resolve()
    modules = next(path for path in dsh.parents if path.name == 'node_modules')
    (profile / 'node_modules').symlink_to(modules, target_is_directory=True)
    project = root / 'project'
    project.mkdir()
    env = {key: value for key, value in os.environ.items()
           if key not in ('GH_TOKEN', 'GITHUB_TOKEN', 'CODESPACES_GITHUB_TOKEN', 'DSH_TUI_URL', 'NO_COLOR',
                          'DISPLAY', 'WAYLAND_DISPLAY', 'TMUX', 'SSH_CONNECTION', 'SSH_TTY')}
    env.update(DSH_HOME=str(root / 'dsh'), TUI_TEST_API_KEY='test-only-not-a-real-key',
               TERM='xterm-256color', COLORTERM='truecolor', XDG_CONFIG_HOME=str(root / 'config'))
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    backend = subprocess.Popen([str(dsh), '--profile', 'web', '--no-open', '--port', str(port)],
                               cwd=project, env=env, stdout=subprocess.PIPE,
                               stderr=subprocess.STDOUT, text=True, start_new_session=True)
    app = None
    fork_app = None
    log = (root / 'backend.log').open('w')
    try:
        login = []

        def drain():
            for line in backend.stdout:
                log.write(line)
                log.flush()
                if 'dsh web: ' in line:
                    login.append(line.split('dsh web: ')[1].strip())

        reader = threading.Thread(target=drain, daemon=True)
        reader.start()
        wait_for(lambda: login, timeout=40)
        parsed = urllib.parse.urlsplit(login[0])
        origin = urllib.parse.urlunsplit((parsed.scheme, parsed.netloc, '', '', ''))
        cookies = http.cookiejar.CookieJar()
        opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cookies), ConnectResponse())
        connect = urllib.parse.urlunsplit((parsed.scheme, parsed.netloc, '/connect', parsed.query, ''))
        with opener.open(connect, timeout=10) as response:
            assert response.status == 302 and len(cookies), 'History reader login failed'

        def rpc(method, request):
            call = urllib.request.Request(origin + '/api/' + method, data=json.dumps({
                'type': 'client-request', 'rpcId': uuid.uuid4().hex, 'method': method,
                'payload': {'args': {'_request' if method == 'session/list' else 'request': request}},
            }).encode(), headers={'Content-Type': 'application/json', 'Origin': origin})
            with opener.open(call, timeout=15) as response:
                value = json.load(response)['result']
            assert value['ok'], f'Actual dsh refused {method}'
            return value.get('value')

        session = rpc('session/create', {'cwd': str(project), 'sessionId': 'session-' + uuid.uuid4().hex})['sessionId']

        def prompt(number):
            text=f'ARCHIVE USER {number:03d}' + (' · КИРИЛЛИЦА 🙂' if number==1 else '')
            receipt = rpc('session/prompt', {'sessionId': session, 'requestId': uuid.uuid4().hex,
                                            'mode': 'queue', 'content': [{'type': 'text', 'text': text}]})
            assert receipt['accepted']

        def completed(number):
            records = rpc('session/events', {'address': {'kind': 'session', 'sessionId': session}, 'maxMessages': 2})['records']
            answer = any(record['event']['type'] == 'assistant/message'
                         and f'ARCHIVE ANSWER {number:03d}' in json.dumps(record['event']['data']) for record in records)
            return answer and not rpc('session/state', {'sessionId': session})['running']

        for number in range(1, 27):
            prompt(number)
            wait_for(lambda: completed(number), timeout=15)

        binary = str(Path(args.binary).resolve())
        app = Terminal(binary, ['--session', session], {**env, 'DSH_TUI_URL': login[0]}, columns=80, rows=24)
        app.wait('ARCHIVE ANSWER 026', timeout=20)
        # Rename through the actual TUI and verify the durable title projection.
        app.send('/rename\r')
        app.wait('Название сессии')
        app.send('\x01\x0b  История   проверок 🙂 \r')
        app.wait('Сессия · История проверок')
        assert rpc('session/projections', {'sessionId': session})['values']['title'] == 'История проверок 🙂'
        app.send('\x1bs')
        app.send('r')
        app.wait('› История проверок')
        app.send('\x1b[13~')
        app.wait('Название сессии')
        app.send('\x01\x0bCANCELLED TITLE\x1b')
        assert rpc('session/projections', {'sessionId': session})['values']['title'] == 'История проверок 🙂'
        wait_for(lambda: (app.pump() or True) and 'Название сессии' not in app.text)
        app.send('\x1bc')
        app.wait('Сессия · История проверок')
        app.send('PRESERVED HISTORY DRAFT')
        app.send('\x1b[23~')
        app.wait('История сессии')
        app.wait('Страница 1')
        app.send('\x1b[H')
        app.wait('ARCHIVE USER 017')
        app.send('\x1b[D')
        app.wait('Страница 2')
        app.send('\x1b[H')
        app.wait('ARCHIVE USER 007')
        saved_rows = app.screen.display[5:19]
        app.send('\x1b[D')
        app.wait('Страница 3')
        app.send('\x1b[H')
        app.wait('ARCHIVE USER 001')
        app.send('\x1b[C')
        app.wait('Страница 2')
        app.wait('ARCHIVE USER 007')
        assert app.screen.display[5:19] == saved_rows, 'Returning to a history page lost its scroll position'

        def search(query):
            app.send('\x06')
            app.wait('Поиск:')
            app.send('\x01\x0b' + query + '\r')

        def marked_message(text):
            def visible():
                app.pump()
                for index,row in enumerate(app.screen.display):
                    if '▶ Совпадение' in row:
                        return any(text in value for value in app.screen.display[index+1:index+4])
                return False
            try:
                wait_for(visible, timeout=20)
            except AssertionError:
                raise AssertionError(f"Missing marked message {text}:\n{app.text}") from None

        search('кириллица 🙂')
        marked_message('КИРИЛЛИЦА 🙂')
        search('archive answer')
        marked_message('ARCHIVE ANSWER 026')
        app.send('\x1b[13~')
        marked_message('ARCHIVE ANSWER 025')
        search('THIS TEXT IS ABSENT')
        app.wait('до начала истории')
        app.send('\x06')
        app.wait('Поиск:')
        app.send('\x1b')
        app.wait('Поиск закрыт.')
        assert 'История сессии' in app.text, 'Cancelling query editing closed history'
        app.send('\x1b[15~')
        app.wait('Страница 1')
        app.send('\x1b[D')
        app.wait('Страница 2')
        app.send('\x1b[H')
        app.wait('ARCHIVE USER 007')
        saved_rows = app.screen.display[5:19]
        prompt(27)
        wait_for(lambda: len(calls) == 27)
        # The frozen page remains stable while the live subscription receives a partial answer.
        for _ in range(5):
            app.pump()
        assert app.screen.display[5:19] == saved_rows, 'Live output changed the pinned history page'
        search('ARCHIVE ACTIVE PARTIAL')
        app.wait('до начала истории')
        assert len(calls)==27, 'Searching an active snapshot submitted model work'
        # Fork the active source through a second real TUI. The first TUI keeps
        # its pinned historical view, so neither the source nor reader is disturbed.
        def session_ids():
            return {item['sessionId'] for item in rpc('session/list', {})['items']}

        before_fork = session_ids()
        fork_app = Terminal(binary, ['--session', session], {**env, 'DSH_TUI_URL': login[0]}, columns=80, rows=24)
        fork_app.wait('ARCHIVE ACTIVE PARTIAL', timeout=20)
        fork_app.send('/archive\r')
        fork_app.wait('Архивирование сессии')
        fork_app.send('\r')
        fork_app.wait('Не подтверждено.')
        assert rpc('session/state', {'sessionId': session})['running'] and len(calls) == 27, 'Refused archive stopped the active source'
        fork_app.send('\x1b')
        wait_for(lambda: (fork_app.pump() or True) and 'Архивирование сессии' not in fork_app.text)
        fork_app.send('/fork\r')
        fork_app.wait('Ветвление сессии')
        fork_app.send('\x1b')
        wait_for(lambda: (fork_app.pump() or True) and 'Ветвление сессии' not in fork_app.text)
        assert session_ids() == before_fork, 'Cancelled fork created a session'
        fork_app.send('/fork\r')
        fork_app.wait('Ветвление сессии')
        fork_app.send('\r')
        child = wait_for(lambda: next(iter(session_ids() - before_fork), None), timeout=20)
        assert len(session_ids() - before_fork) == 1, 'Fork created multiple sessions'
        child_page = rpc('session/events', {'address': {'kind': 'session', 'sessionId': child}, 'maxMessages': 4})
        child_text = json.dumps(child_page['records'])
        assert 'ARCHIVE ANSWER 026' in child_text and 'ARCHIVE ACTIVE PARTIAL' not in child_text and 'ARCHIVE USER 027' not in child_text, 'Fork did not use the last completed turn'
        assert not rpc('session/state', {'sessionId': child})['running'], 'Fork started the model automatically'
        assert rpc('session/state', {'sessionId': session})['running'], 'Fork stopped the source agent'
        rpc('session/rename', {'sessionId': child, 'title': 'FORK FROM COMPLETED TURN'})
        fork_app.wait('FORK FROM COMPLETED TURN', timeout=20)
        assert len(calls) == 27, 'Creating a fork submitted model work'
        release.set()
        wait_for(lambda: completed(27), timeout=15)
        for _ in range(20):
            app.pump()
        app.send('\x1b[15~')
        app.wait('Страница 1')
        app.wait('ARCHIVE ANSWER 027')
        search('archive answer 027')
        marked_message('ARCHIVE ANSWER 027')
        app.send('\x1b')
        app.wait('PRESERVED HISTORY DRAFT')
        assert len(calls) == 27, 'Reading history submitted a prompt or replayed a model call'

        for columns, rows in [(120, 42), (100, 30), (80, 24)]:
            previous, app = app, None
            previous.close(root / f'before-history-{columns}.ansi')
            app = Terminal(binary, ['--session', session], {**env, 'DSH_TUI_URL': login[0]}, columns=columns, rows=rows)
            app.wait('ARCHIVE ANSWER 027', timeout=20)
            app.send('\x1b[23~')
            app.wait('Страница 1')
            app.send('\x1b[D')
            app.wait('Страница 2')
            app.send('\x1b[H')
            app.wait('ARCHIVE USER 008')
            if args.capture_history:
                capture(app, args.capture_history + f'-{columns}x{rows}.json')
            search('кириллица 🙂')
            marked_message('КИРИЛЛИЦА 🙂')
            if args.capture_search:
                capture(app, args.capture_search + f'-{columns}x{rows}.json')
            app.send('\x02')
            app.wait('Ветвление сессии')
            app.wait('История — по событие #')
            app.wait('Enter — создать')
            if args.capture_exact_fork:
                app.pump()
                cells = [{'x': x, 'y': y, 'text': app.screen.buffer[y][x].data,
                          'fg': app.screen.buffer[y][x].fg, 'bg': app.screen.buffer[y][x].bg,
                          'bold': app.screen.buffer[y][x].bold, 'reverse': app.screen.buffer[y][x].reverse}
                         for y in range(rows) for x in range(columns)]
                Path(args.capture_exact_fork + f'-{columns}x{rows}.json').write_text(json.dumps(
                    {'source': 'Actual dsh exact-message fork', 'columns': columns, 'rows': rows, 'cells': cells}, ensure_ascii=False))
            app.send('\x1b')
            wait_for(lambda: (app.pump() or True) and 'Ветвление сессии' not in app.text)
            marked_message('КИРИЛЛИЦА 🙂')
            app.send('\x1b')
            wait_for(lambda: (app.pump() or True) and 'История сессии' not in app.text)
            app.send('/rename\r')
            app.wait('Название сессии')
            app.wait('Enter — сохранить')
            if args.capture_rename:
                app.pump()
                cells = [{'x': x, 'y': y, 'text': app.screen.buffer[y][x].data,
                          'fg': app.screen.buffer[y][x].fg, 'bg': app.screen.buffer[y][x].bg,
                          'bold': app.screen.buffer[y][x].bold, 'reverse': app.screen.buffer[y][x].reverse}
                         for y in range(rows) for x in range(columns)]
                Path(args.capture_rename + f'-{columns}x{rows}.json').write_text(json.dumps(
                    {'source': 'Actual dsh rename editor', 'columns': columns, 'rows': rows, 'cells': cells}, ensure_ascii=False))
            app.send('\x1b')
            wait_for(lambda: (app.pump() or True) and 'Название сессии' not in app.text)
            # Ctrl+B targets the selected list entry and keeps the chat draft.
            app.send('\x1bs')
            app.wait('Сессии dsh')
            app.send('r')
            app.wait('› FORK FROM COMPLETED TURN')
            app.send('\x02')
            app.wait('Ветвление сессии')
            app.wait('Enter — создать')
            if args.capture_fork:
                app.pump()
                cells = [{'x': x, 'y': y, 'text': app.screen.buffer[y][x].data,
                          'fg': app.screen.buffer[y][x].fg, 'bg': app.screen.buffer[y][x].bg,
                          'bold': app.screen.buffer[y][x].bold, 'reverse': app.screen.buffer[y][x].reverse}
                         for y in range(rows) for x in range(columns)]
                Path(args.capture_fork + f'-{columns}x{rows}.json').write_text(json.dumps(
                    {'source': 'Actual dsh fork confirmation', 'columns': columns, 'rows': rows, 'cells': cells}, ensure_ascii=False))
            app.send('\x1b')
            wait_for(lambda: (app.pump() or True) and 'Ветвление сессии' not in app.text)
        assert len(calls) == 27, 'History/rename/fork restarted model work'
        assert session_ids() == before_fork | {child}, 'Cancelled list fork created a session'
        fork_app.send('FORK CHILD ONLY\r')
        fork_app.wait('ARCHIVE ANSWER 028', timeout=20)
        assert len(calls) == 28, 'Child continuation did not make exactly one model call'
        model_input = json.dumps(calls[-1]['messages'])
        assert 'ARCHIVE ANSWER 026' in model_input and 'FORK CHILD ONLY' in model_input, 'Child lost inherited history or new prompt'
        assert 'ARCHIVE USER 027' not in model_input and 'ARCHIVE ACTIVE PARTIAL' not in model_input, 'Unfinished source turn leaked into child input'
        parent_page = rpc('session/events', {'address': {'kind': 'session', 'sessionId': session}, 'maxMessages': 4})
        assert 'FORK CHILD ONLY' not in json.dumps(parent_page['records']), 'Child continuation mutated source'
        assert rpc('session/projections', {'sessionId': session})['values']['title'] == 'История проверок 🙂', 'Child rename changed source title'
        fork_app.close(root / 'fork-terminal.ansi')
        fork_app = None
        # Search an older committed assistant message and fork its exact event.
        source_records = rpc('session/events', {'address': {'kind': 'session', 'sessionId': session}, 'maxMessages': 100})['records']
        exact_seq = next(record['event']['seq'] for record in source_records
                         if record['event']['type'] == 'assistant/message' and 'ARCHIVE ANSWER 001' in json.dumps(record['event']['data']))
        before_exact_fork = session_ids()
        fork_app = Terminal(binary, ['--session', session], {**env, 'DSH_TUI_URL': login[0]}, columns=80, rows=24)
        fork_app.wait('ARCHIVE ANSWER 027', timeout=20)
        fork_app.send('EXACT PRESERVED DRAFT')
        fork_app.send('\x1b[23~')
        fork_app.wait('Страница 1')
        fork_app.send('\x06')
        fork_app.wait('Поиск:')
        fork_app.send('archive answer 001\r')
        fork_app.wait('▶ Совпадение')
        fork_app.wait('ARCHIVE ANSWER 001')
        fork_app.send('\t')
        fork_app.wait('◆ Точка ветвления')
        fork_app.send('\x1b[Z')
        fork_app.wait(f'точка #{exact_seq}')
        fork_app.send('\x02')
        fork_app.wait(f'История — по событие #{exact_seq}')
        fork_app.send('\r')
        exact_child = wait_for(lambda: next(iter(session_ids() - before_exact_fork), None), timeout=20)
        assert len(session_ids() - before_exact_fork) == 1, 'Exact fork created multiple children'
        exact_records = rpc('session/events', {'address': {'kind': 'session', 'sessionId': exact_child}, 'maxMessages': 100})['records']
        exact_text = json.dumps(exact_records)
        assert 'ARCHIVE ANSWER 001' in exact_text and 'ARCHIVE USER 002' not in exact_text, 'Exact fork included messages after the selected boundary'
        assert any(record['event']['type'] == 'session/end-seed' and record['event']['seq'] == exact_seq + 1 for record in exact_records), 'Child did not inherit the exact inclusive prefix'
        assert any(record['event']['type'] == 'turn/end' and record['event']['data']['reason']['kind'] == 'forked' for record in exact_records), 'Open turn did not receive a fork closer'
        assert not rpc('session/state', {'sessionId': exact_child})['running'] and len(calls) == 28, 'Exact fork submitted model work'
        rpc('session/rename', {'sessionId': exact_child, 'title': 'EXACT MESSAGE BRANCH'})
        fork_app.wait('EXACT MESSAGE BRANCH', timeout=20)
        fork_app.wait('EXACT PRESERVED DRAFT')
        fork_app.send('\x01\x0bEXACT CHILD ONLY\r')
        fork_app.wait('ARCHIVE ANSWER 029', timeout=20)
        exact_input = json.dumps(calls[-1]['messages'])
        assert len(calls) == 29 and 'ARCHIVE ANSWER 001' in exact_input and 'EXACT CHILD ONLY' in exact_input and 'ARCHIVE USER 002' not in exact_input, 'Exact branch model input lost the selected cut'
        assert 'EXACT CHILD ONLY' not in json.dumps(rpc('session/events', {'address': {'kind': 'session', 'sessionId': session}, 'maxMessages': 4})['records']), 'Exact child changed source'
        fork_app.close(root / 'exact-fork-terminal.ansi')
        fork_app = None
        print('Actual dsh exact fork: searched historical message, Tab/Shift+Tab selection, inclusive event prefix, synthetic closers, preserved draft, independent continuation and model input OK')
        # A user-message cut must also remain idle and exclude the old answer.
        before_user_fork = session_ids()
        fork_app = Terminal(binary, ['--session', session], {**env, 'DSH_TUI_URL': login[0]}, columns=80, rows=24)
        fork_app.wait('ARCHIVE ANSWER 027', timeout=20)
        fork_app.send('\x1b[23~')
        fork_app.wait('Страница 1')
        fork_app.send('\x06')
        fork_app.wait('Поиск:')
        fork_app.send('archive user 001\r')
        fork_app.wait('▶ Совпадение')
        fork_app.wait('ARCHIVE USER 001')
        fork_app.send('\x02')
        fork_app.wait('История — по событие #')
        fork_app.send('\r')
        user_child = wait_for(lambda: next(iter(session_ids() - before_user_fork), None), timeout=20)
        user_records = rpc('session/events', {'address': {'kind': 'session', 'sessionId': user_child}, 'maxMessages': 100})['records']
        user_text = json.dumps(user_records)
        assert 'ARCHIVE USER 001' in user_text and 'ARCHIVE ANSWER 001' not in user_text, 'User cut included the old answer'
        assert not rpc('session/state', {'sessionId': user_child})['running'] and len(calls) == 29, 'User cut automatically ran the model'
        rpc('session/rename', {'sessionId': user_child, 'title': 'EXACT USER BRANCH'})
        fork_app.wait('EXACT USER BRANCH', timeout=20)
        fork_app.send('USER CUT CONTINUE\r')
        fork_app.wait('ARCHIVE ANSWER 030', timeout=20)
        user_input = json.dumps(calls[-1]['messages'])
        assert len(calls) == 30 and 'ARCHIVE USER 001' in user_input and 'USER CUT CONTINUE' in user_input and 'ARCHIVE ANSWER 001' not in user_input, 'User-cut continuation used an incorrect prefix'
        fork_app.close(root / 'user-fork-terminal.ansi')
        fork_app = None
        print('Actual dsh user-message fork: selected prompt retained, old answer excluded, idle child and independent continuation OK')
        # Archive an idle session through the TUI, then reconnect and restore it
        # from the archive filter. Neither action is a model prompt.
        for columns, rows in [(120, 42), (100, 30), (80, 24)]:
            fork_app = Terminal(binary, ['--session', user_child], {**env, 'DSH_TUI_URL': login[0]}, columns=columns, rows=rows)
            fork_app.wait('ARCHIVE ANSWER 030', timeout=20)
            fork_app.send('/archive\r')
            fork_app.wait('Архивирование сессии')
            fork_app.wait('Enter — подтвердить')
            if args.capture_archive:
                fork_app.pump()
                cells = [{'x': x, 'y': y, 'text': fork_app.screen.buffer[y][x].data,
                          'fg': fork_app.screen.buffer[y][x].fg, 'bg': fork_app.screen.buffer[y][x].bg,
                          'bold': fork_app.screen.buffer[y][x].bold, 'reverse': fork_app.screen.buffer[y][x].reverse}
                         for y in range(rows) for x in range(columns)]
                Path(args.capture_archive + f'-{columns}x{rows}.json').write_text(json.dumps(
                    {'source': 'Actual dsh archive confirmation', 'columns': columns, 'rows': rows, 'cells': cells}, ensure_ascii=False))
            fork_app.send('\x1b')
            wait_for(lambda: (fork_app.pump() or True) and 'Архивирование сессии' not in fork_app.text)
            if columns != 80:
                fork_app.close(root / f'before-archive-{columns}.ansi')
                fork_app = None
        fork_app.send('/archive\r')
        fork_app.wait('Архивирование сессии')
        fork_app.send('\r')
        fork_app.wait('/restore — восстановить')
        fork_app.close(root / 'archived-before-reconnect.ansi')
        fork_app = Terminal(binary, ['--session', user_child], {**env, 'DSH_TUI_URL': login[0]}, columns=80, rows=24)
        fork_app.wait('/restore — восстановить', timeout=20)
        fork_app.wait('ARCHIVE ANSWER 030')
        fork_app.send('ARCHIVED BLOCKED DRAFT\r')
        fork_app.wait('восстановить; текст')
        assert len(calls) == 30, 'Archived composer submitted model work'
        fork_app.send('\x1bs')
        fork_app.wait('Сессии dsh')
        fork_app.send('r')
        fork_app.wait('Активные сессии')
        wait_for(lambda: (fork_app.pump() or True) and 'Запрос dsh…' not in fork_app.text)
        assert 'EXACT USER BRANCH' not in fork_app.text, 'Archive remained in active list'
        fork_app.send('a')
        fork_app.wait('Архив сессий')
        fork_app.wait('› EXACT USER BRANCH')
        fork_app.send('\x18')
        fork_app.wait('Восстановление сессии')
        fork_app.send('\r')
        wait_for(lambda: (fork_app.pump() or True) and 'Восстановление сессии' not in fork_app.text)
        fork_app.send('a')
        fork_app.wait('Активные сессии')
        fork_app.wait('EXACT USER BRANCH')
        fork_app.send('\x1bc')
        fork_app.wait('ARCHIVED BLOCKED DRAFT')
        fork_app.wait('Enter — отправить')
        assert len(calls) == 30, 'Restoring session submitted model work'
        fork_app.send('\x01\x0bRESTORED CHILD ONLY\r')
        fork_app.wait('ARCHIVE ANSWER 031', timeout=20)
        assert len(calls) == 31 and 'RESTORED CHILD ONLY' in json.dumps(calls[-1]['messages']), 'Restored session could not continue'
        # A change from another client must reach the existing TUI subscription.
        app.send('\x1bc')
        app.wait('Сессия · История проверок')
        rpc('workspace/archiveSession', {'sessionId': session, 'stopActivity': False})
        app.wait('Сессия в архиве.')
        rpc('workspace/unarchiveSession', {'sessionId': session})
        wait_for(lambda: (app.pump() or True) and 'Сессия в архиве.' not in app.text)
        fork_app.close(root / 'archive-terminal.ansi')
        fork_app = None
        print('Actual dsh archive: active refusal, cancellation, durable archive across reconnect, active/archive filters, blocked composer, retained draft, restored continuation and external-client synchronization OK')
        # Formatted output must remain a view of the exact committed source.
        fork_app = Terminal(binary, ['--session', user_child], {**env, 'DSH_TUI_URL': login[0]}, columns=80, rows=24)
        fork_app.wait('ARCHIVE ANSWER 031', timeout=20)
        fork_app.send('Render Markdown with code and a Unicode table.\r')
        fork_app.wait('ARCHIVE ANSWER 032', timeout=20)
        assert len(calls) == 32, 'Markdown display submitted extra model work'
        def markdown_source():
            records = rpc('session/events', {'address': {'kind': 'session', 'sessionId': user_child}, 'maxMessages': 4})['records']
            return next(record['event']['data']['message']['content'][0]['text'] for record in reversed(records)
                        if record['event']['type'] == 'assistant/message')
        wait_for(lambda: markdown_source() == markdown_reply)
        fork_app.close(root / 'before-markdown-sizes.ansi')
        fork_app = None
        for columns, rows in [(120, 42), (100, 30), (80, 24)]:
            fork_app = Terminal(binary, ['--session', user_child], {**env, 'DSH_TUI_URL': login[0]}, columns=columns, rows=rows)
            fork_app.wait('ARCHIVE ANSWER 032', timeout=20)
            fork_app.send('MARKDOWN DRAFT')
            fork_app.send('\x1b[23~')
            fork_app.wait('Страница 1')
            fork_app.send('\x06')
            fork_app.wait('Поиск:')
            fork_app.send('**Bold text**\r')
            fork_app.wait('▶ Совпадение')
            assert '**Bold text**' not in '\n'.join(fork_app.screen.display[7:14]), 'Formatted history showed raw emphasis'
            fork_app.send('\x1b[H')
            def code_colors():
                fork_app.pump()
                for y, line in enumerate(fork_app.screen.display):
                    if 'fmt.Println' in line:
                        return {fork_app.screen.buffer[y][x].fg for x in range(fork_app.screen.columns)
                                if fork_app.screen.buffer[y][x].data in ('世', '界')}
                fork_app.send('\x1b[B')
                return set()
            wait_for(lambda: 'a5d6ff' in code_colors())
            if args.capture_syntax:
                capture_cells(fork_app, args.capture_syntax + f'-{columns}x{rows}.json')

            def copied(key, expected):
                offset = len(fork_app.raw)
                fork_app.send(key)
                def received():
                    fork_app.pump()
                    return re.search(rb'\x1b\]52;c;([A-Za-z0-9+/=]+)\x07', fork_app.raw[offset:])
                payload = wait_for(received)
                assert base64.b64decode(payload.group(1)).decode() == expected, 'Clipboard altered source bytes'
                fork_app.wait('запрос OSC 52 отправлен')
            copied('\x19', markdown_reply)
            fork_app.send('\x0b')
            fork_app.wait('Копирование блока кода')
            fork_app.wait('блок 1/2')
            copied('\r', 'fmt.Println("世界")\n')
            fork_app.send('\x1b[B')
            fork_app.wait('блок 2/2')
            copied('\r', '\tprint("Код 🙂")\n\n')
            if args.capture_copy:
                capture_cells(fork_app, args.capture_copy + f'-{columns}x{rows}.json')
            fork_app.send('\x1b')
            wait_for(lambda: (fork_app.pump() or True) and 'Копирование блока кода' not in fork_app.text)
            fork_app.send('\x12')
            fork_app.wait('История сессии · исходник')
            fork_app.wait('**Bold text**')
            fork_app.send('\x12')
            wait_for(lambda: (fork_app.pump() or True) and 'История сессии · исходник' not in fork_app.text)
            fork_app.send('\x1b[F')
            fork_app.wait('TABLE_32')
            fork_app.wait('Кириллица')
            fork_app.wait('https://example.test/docs')
            if args.capture_markdown:
                capture(fork_app, args.capture_markdown + f'-{columns}x{rows}.json')
            fork_app.send('\x1b')
            fork_app.wait('MARKDOWN DRAFT')
            fork_app.close(root / f'markdown-{columns}.ansi')
            fork_app = None
        assert markdown_source() == markdown_reply and len(calls) == 32, 'Formatting/search/toggling rewrote source or ran the model'
        print('Actual dsh clipboard: exact message source and two code blocks, tabs, blank lines, Unicode, honest OSC 52 receipt, retained selection/draft and no host clipboard changes OK')
        print('Actual dsh Markdown: syntax-colored code, Unicode table, visible link, source search, raw/formatted toggle, preserved draft, unchanged committed source and three sizes OK')
        empty = rpc('session/create', {'cwd': str(project), 'sessionId': 'session-' + uuid.uuid4().hex})['sessionId']
        before_empty_fork = session_ids()
        fork_app = Terminal(binary, ['--session', empty], {**env, 'DSH_TUI_URL': login[0]}, columns=80, rows=24)
        fork_app.wait('Сессия ·', timeout=20)
        fork_app.send('/fork\r')
        fork_app.wait('Ветвление сессии')
        fork_app.send('\r')
        fork_app.wait('Создание не подтверждено.')
        fork_app.send('\r')
        assert session_ids() == before_empty_fork and len(calls) == 32, 'Unavailable fork created a session or model work'
        fork_app.send('\x1b')
        wait_for(lambda: (fork_app.pump() or True) and 'Ветвление сессии' not in fork_app.text)
        fork_app.close(root / 'empty-fork-terminal.ansi')
        fork_app = None
        print('Actual dsh fork: cancelled current/list actions, active source continues, completed-turn prefix, one child, isolated continuation, inherited model input and empty-source refusal OK')
        print('Actual dsh rename: normalized Unicode title, durable projection, session list, cancelled edit and reconnect OK')
        all_records = rpc('session/events', {'address': {'kind': 'session', 'sessionId': session}, 'maxMessages': 1000})['records']
        prompts = [record['event'] for record in all_records if record['event']['type'] == 'user/message'
                   and record['event']['data'].get('source', {}).get('kind') == 'user']
        # Both references are in the live 50-message window, outside the history 20-message tail.
        old_start, old_end = prompts[12], prompts[14]
        old_text = old_start['data']['content'][0]['text']
        def rewrite(start, end, text):
            result_path = root / 'history-rewrite-result.json'
            result_path.unlink(missing_ok=True)
            (root / 'history-rewrite.json').write_text(json.dumps({'sessionId': session,
                'startSeq': start, 'endSeq': end, 'text': text}))
            wait_for(result_path.exists)
            result = json.loads(result_path.read_text())
            assert 'error' not in result, result
            return result['seq']
        first_replacement = rewrite(old_start['seq'], old_start['seq'], 'REPLACEMENT FIRST UNIQUE')
        # The start seq is numerically newer than the end, but precedes it on the surface.
        second_replacement = rewrite(first_replacement, old_end['seq'], 'REPLACEMENT FINAL UNIQUE')
        assert second_replacement > first_replacement > old_end['seq']
        for columns, rows in [(120, 42), (100, 30), (80, 24)]:
            fork_app = Terminal(binary, ['--session', session], {**env, 'DSH_TUI_URL': login[0]}, columns=columns, rows=rows)
            fork_app.wait('ARCHIVE ANSWER 027', timeout=20)
            fork_app.send('REPLACEMENT DRAFT')
            fork_app.send('\x1b[23~')
            fork_app.wait('PgUp/PgDn')
            fork_app.send('\x06')
            fork_app.wait('Поиск:')
            fork_app.send(old_text + '\r')
            fork_app.wait('Более ранних совпадений нет', timeout=20)
            fork_app.send('\x06')
            fork_app.wait('Поиск:')
            fork_app.send('\x01\x0bREPLACEMENT FIRST UNIQUE\r')
            fork_app.wait('Более ранних совпадений нет', timeout=20)
            fork_app.send('\x06')
            fork_app.wait('Поиск:')
            fork_app.send('\x01\x0bREPLACEMENT FINAL UNIQUE\r')
            fork_app.wait('▶ Совпадение')
            fork_app.wait('REPLACEMENT FINAL UNIQUE')
            fork_app.send('\x1b[D')
            fork_app.wait('Страница 3')
            assert old_text not in fork_app.text, 'Older page resurrected replaced source'
            fork_app.send('\x1b[C')
            fork_app.wait('REPLACEMENT FINAL UNIQUE')
            fork_app.send('\x1b')
            fork_app.wait('REPLACEMENT DRAFT')
            fork_app.close(root / f'replacements-{columns}.ansi')
            fork_app = None
        assert len(calls) == 32, 'Snapshot replacement checks ran model work'
        print('Actual dsh replacements: cross-page targets, replacement of a replacement, reversed numeric endpoints, no removed-text search hits, older/newer navigation and three sizes OK')
        app.send('\x04')
        app.exited()
        print('Actual dsh history: 27 turns, three pinned pages, older/newer navigation, preserved scroll/draft, live isolation, refresh and reconnect OK')
        print('Actual dsh message search: oldest Unicode text beyond the loaded window, next match, no match, query cancellation, pinned live cut and refresh OK')
        print('History captures verified at 120x42, 100x30 and 80x24; no paid model calls or user-profile changes.')
    finally:
        release.set()
        if fork_app:
            fork_app.close(root / 'fork-terminal.ansi')
        if app:
            app.close(root / 'terminal.ansi')
        if backend.poll() is None:
            os.killpg(backend.pid, signal.SIGTERM)
        backend.wait(timeout=15)
        if 'reader' in locals():
            reader.join(timeout=2)
        log.close()
        backend.stdout.close()
        model.shutdown()
