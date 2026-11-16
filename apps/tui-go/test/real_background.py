"""Background work against actual dsh: real shell jobs, native child agents and schedule delivery."""
import argparse
import http.cookiejar
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import signal
import re
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
parser.add_argument('--capture', help='Prefix for actual background-work terminal-cell captures')
args = parser.parse_args()
requests = []
seen_markers = []
holding = threading.Event()

class Model(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        requests.append(body)
        users = '\n'.join(str(message.get('content', '')) for message in body['messages'] if message.get('role') == 'user')
        markers = re.findall(r'BG_[A-Z_]+', users)
        marker = markers[-1] if markers else ''
        seen_markers.append(marker)
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.end_headers()
        def send(delta, finish=None):
            self.wfile.write(('data: ' + json.dumps({'choices': [{'index': 0, 'delta': delta, 'finish_reason': finish}]}) + '\n\n').encode())
            self.wfile.flush()
        try:
            names = [entry['function']['name'] for entry in body.get('tools', [])]
            def tool(suffix):
                return next(name for name in names if name == suffix or name.endswith('__' + suffix))
            history = json.dumps(body['messages'])
            if marker == 'BG_START_JOB' and 'background-fixture-call' not in history:
                delta = {'role': 'assistant', 'tool_calls': [{'index': 0, 'id': 'background-fixture-call', 'type': 'function',
                    'function': {'name': tool('bash'), 'arguments': json.dumps({'command': "printf 'start\\n' >> job-starts; printf 'JOB_FIRST привет\\n'; while [ ! -f job-append ]; do sleep 0.1; done; printf 'JOB_SECOND\\n'; while [ ! -f job-release ]; do sleep 0.1; done; printf 'JOB_LAST\\n'; touch job-finished", 'description': 'Background process fixture', 'run_in_background': True})}}]}
                send(delta); send({}, 'tool_calls')
            elif marker == 'BG_START_CHILD' and 'child-fixture-call' not in history:
                send({'role': 'assistant', 'tool_calls': [{'index': 0, 'id': 'child-fixture-call', 'type': 'function',
                    'function': {'name': tool('subagent'), 'arguments': json.dumps({'description': 'Child lifecycle fixture', 'prompt': 'BG_CHILD_INITIAL'})}}]})
                send({}, 'tool_calls')
            elif marker in ('BG_CHILD_INTERRUPT', 'BG_CHILD_INTERRUPT_AGAIN'):
                holding.set()
                send({'role': 'assistant', 'content': 'CHILD_RUNNING'})
                for _ in range(600):
                    time.sleep(0.1)
                    send({})
                raise AssertionError('Child was never interrupted')
            else:
                reply = {'BG_CHILD_INITIAL': 'CHILD_INITIAL_DONE', 'BG_CHILD_FOLLOWUP': 'CHILD_FOLLOWUP_DONE',
                         'BG_SCHEDULE': 'SCHEDULE_DELIVERED'}.get(marker, 'PARENT_READY')
                send({'role': 'assistant', 'content': reply}); send({}, 'stop')
            self.wfile.write(b'data: [DONE]\n\n'); self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass

class ConnectResponse(urllib.request.HTTPErrorProcessor):
    def http_response(self, request, response):
        return response if response.status == 302 else super().http_response(request, response)

with tempfile.TemporaryDirectory(prefix='dsh-tui-background-') as directory:
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
    probe = profile / 'background-probe.mjs'
    probe.write_text(Path(__file__).with_name('background_probe.mjs').read_text())
    (profile / 'cordis.yml').write_text('[]\n')
    (profile / 'cordis.patch.yml').write_text(json.dumps([
        {'insert': [{'id': 'tui-background-probe', 'name': str(probe)}]},
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
        binary = str(Path(args.binary).resolve())
        def launch(session=session_id, columns=100, rows=30):
            return Terminal(binary, ['--session', session], {**env, 'DSH_TUI_URL': login[0]}, columns=columns, rows=rows)
        def state(session):
            result = rpc('session/state', {'request': {'sessionId': session}})
            assert result['ok'], result
            return result['value']
        def projection(session):
            return rpc('session/projections', {'request': {'sessionId': session}})['value']['values']
        def capture(label, visible):
            if not args.capture:
                return
            for columns, rows in [(100, 30), (80, 24)]:
                app.resize(columns, rows)
                app.wait(visible)
                for _ in range(5):
                    app.pump()
                cells = [{'x': x, 'y': y, 'text': (cell := app.screen.buffer[y][x]).data,
                    'fg': cell.fg, 'bg': cell.bg, 'bold': cell.bold, 'reverse': cell.reverse}
                    for y in range(rows) for x in range(columns)]
                target = Path(args.capture + f'-{label}-{columns}x{rows}.json')
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(json.dumps({'source': 'Actual isolated dsh background work', 'columns': columns, 'rows': rows, 'cells': cells}, ensure_ascii=False))
            app.resize(100, 30)
            app.wait(visible)
        def panel(command, title):
            app.command(command); app.wait(title); app.wait('Снимок dsh')
        def back(title):
            app.send('\x1b')
            wait_for(lambda: (app.pump(), title not in app.text)[1])
        def confirm():
            app.wait('Enter — подтвердить'); app.send('\r'); app.wait('Ответ dsh получен')
        def refresh(text='Снимок dsh'):
            app.send('\x1b[15~'); app.wait(text)
        def refresh_until(text):
            deadline = time.monotonic() + 10
            while text not in app.text and time.monotonic() < deadline:
                app.send('\x1b[15~')
                for _ in range(6):
                    app.pump()
            app.wait(text)
        def output(text):
            app.wait('Вывод задания')
            refresh_until(text)
        def reconnect():
            global app
            app.send('\x04'); app.exited(); app.close(root / 'before-reconnect.ansi')
            app = launch(); app.wait('Enter —', timeout=20)
        def edit_message(text):
            app.send('\x1bOR'); app.wait('F5 — проверить')
            app.send('\x1b[200~' + text + '\x1b[201~')
            app.send('\x1b[15~'); confirm()

        app = launch(); app.wait('Enter —', timeout=20)
        panel('/jobs', 'Фоновые задания')
        app.send('Shared'); app.wait('Общее задание Host')
        app.send('\r'); app.wait('SHARED_HOST_OUTPUT привет'); app.wait('Часть вывода недоступна')
        capture('shared-output','SHARED_HOST_OUTPUT')
        back('Вывод задания'); app.send('\x18'); confirm(); refresh_until('killed')
        back('Фоновые задания')
        print('Actual shared job output, gap indicator and explicit stop OK', flush=True)
        app.command('BG_START_JOB')
        app.wait('PARENT_READY', timeout=30)
        panel('/jobs', 'Фоновые задания')
        app.send('job-starts'); app.wait('Тип: bash')
        app.send('\r'); output('JOB_FIRST привет')
        (project / 'job-append').touch()
        output('JOB_SECOND')
        capture('shell-output','JOB_SECOND')
        assert app.text.count('JOB_FIRST привет') == 1, 'Refresh duplicated retained output'
        back('Вывод задания')
        back('Фоновые задания')
        reconnect()
        panel('/jobs', 'Фоновые задания')
        app.send('job-starts'); app.wait('Тип: bash')
        app.send('\r'); output('JOB_FIRST привет')
        back('Вывод задания')
        app.send('\x18'); app.wait('Enter — подтвердить'); app.send('\x1b')
        app.wait('Подтверждение отменено')
        app.send('\x18'); confirm()
        refresh_until('killed')
        assert not (project / 'job-finished').exists(), 'Stopped job ran its final action'
        back('Фоновые задания')
        assert (project / 'job-starts').read_text().splitlines() == ['start'], 'Reconnect reran the shell job'

        print('Actual shell job output, cancelled stop, confirmed stop and reconnect OK', flush=True)
        app.command('BG_START_CHILD')
        app.wait('PARENT_READY', timeout=30)
        wait_for(lambda: projection(session_id).get('subagentCatalog'), timeout=30)
        child = projection(session_id)['subagentCatalog'][0]
        assert child['mode'] == 'continuable', child
        child_id = child['id']
        wait_for(lambda: not state(child_id)['running'], timeout=30)
        panel('/agents', 'Подагенты сессии')
        app.wait('Child lifecycle fixture')
        capture('agents','Child lifecycle fixture')
        edit_message('BG_CHILD_FOLLOWUP\nUnicode: привет')
        wait_for(lambda: not state(child_id)['running'], timeout=20)
        back('Подагенты сессии')
        reconnect()
        panel('/agents', 'Подагенты сессии')
        app.send('\r')
        app.wait('CHILD_FOLLOWUP_DONE', timeout=30)
        capture('child-viewer','подагент · просмотр')
        app.command('/history'); app.wait('История сессии'); app.wait('CHILD_FOLLOWUP_DONE')
        app.send('\x06'); app.wait('Поиск'); app.send('BG_CHILD_FOLLOWUP\r'); app.wait('Unicode: привет')
        app.wait('Совпадение')
        app.send('\x1b')
        wait_for(lambda: (app.pump(), 'История сессии' not in app.text)[1])
        print('Actual child continuation, child navigation, history and search OK', flush=True)
        app.command('/parent')
        app.wait('Сессия · BG_START_JOB', timeout=20)
        wait_for(lambda: (app.pump(), 'Подключение чата' not in app.text)[1])
        panel('/agents', 'Подагенты сессии')
        edit_message('BG_CHILD_INTERRUPT')
        wait_for(holding.is_set, timeout=20)
        back('Подагенты сессии')
        reconnect()
        panel('/agents', 'Подагенты сессии')
        assert state(child_id)['running'], 'TUI disconnect interrupted the child'
        app.send('\x18'); app.wait('Enter — подтвердить'); app.send('\x1b'); app.wait('Подтверждение отменено')
        assert state(child_id)['running'], 'Cancelled confirmation interrupted the child'

        app.send('\x18'); confirm()
        wait_for(lambda: not state(child_id)['running'], timeout=20)
        back('Подагенты сессии')
        reconnect()
        panel('/agents', 'Подагенты сессии'); app.wait(child_id)
        back('Подагенты сессии')

        panel('/agents', 'Подагенты сессии')
        holding.clear()
        edit_message('BG_CHILD_INTERRUPT_AGAIN')
        wait_for(holding.is_set, timeout=20)
        app.send('\x04'); app.exited(); app.close(root / 'parent-before-child-cli.ansi')
        app = launch(child_id, columns=80, rows=24)
        app.wait('подагент · просмотр', timeout=20); app.wait('CHILD_RUNNING')
        app.send('\x1b[19~')
        wait_for(lambda: not state(child_id)['running'], timeout=20)
        app.command('/parent'); app.wait('Сессия · BG_START_JOB', timeout=20)
        wait_for(lambda: (app.pump(), 'Подключение чата' not in app.text)[1])
        app.resize(100, 30)
        print('Actual child interruption, direct child CLI reconnect, F8 parent routing and return OK', flush=True)
        created = rpc('schedule/create', {'request': {'title': 'Background schedule fixture', 'prompt': 'BG_SCHEDULE',
            'timing': {'kind': 'after', 'after_seconds': 86400}, 'target': {'kind': 'session', 'sessionId': session_id}}})
        assert created['ok'], created
        schedule_id = created['value']['id']
        panel('/schedules', 'Расписания')
        app.send('\x07'); app.wait('Enter — подтвердить')
        assert not rpc('schedule/history', {'request': {'id': schedule_id, 'limit': 100}})['value']['records']
        app.send('\x1b'); app.wait('Подтверждение отменено')
        app.send('\x07'); confirm()
        wait_for(lambda: seen_markers.count('BG_SCHEDULE') == 1, timeout=20)
        wait_for(lambda: not state(session_id)['running'], timeout=20)
        refresh()
        app.send('\r'); app.wait('История расписания')
        app.wait('targetSessionId')
        capture('schedule-history','История расписания')
        back('История расписания')
        back('Расписания')
        reconnect()
        panel('/schedules', 'Расписания'); app.send('\r'); app.wait('История расписания')
        app.wait(session_id)
        history = rpc('schedule/history', {'request': {'id': schedule_id, 'limit': 100}})
        assert history['ok'] and len(history['value']['records']) == 1, history
        assert seen_markers.count('BG_CHILD_FOLLOWUP') == 1, 'Child follow-up was replayed'
        assert seen_markers.count('BG_CHILD_INTERRUPT') == 1, 'Child activation was restarted'
        assert seen_markers.count('BG_CHILD_INTERRUPT_AGAIN') == 1, 'Direct child reconnect restarted its model turn'
        assert seen_markers.count('BG_SCHEDULE') == 1, 'Schedule delivery was replayed'
        assert not (project / 'job-finished').exists(), 'Stopped process continued after reconnect'
        app.send('\x04'); app.exited()
        print('Actual dsh background work: shell output/kill, child continuation/interrupt/navigation, schedule run/history and reconnect OK')
    except Exception:
        if app:
            print(app.text, flush=True)
        raise
    finally:
        if app:
            app.close(root / 'background-terminal.ansi')
        if backend.poll() is None:
            os.killpg(backend.pid, signal.SIGTERM)
        backend.wait(timeout=15)
        reader.join(timeout=2)
        backend.stdout.close()
        model.shutdown()
