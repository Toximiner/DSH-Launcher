"""Golden baseline captures for real-dsh TUI states (isolated profile).

Replicates the real_chat.py fixture (isolated dsh + controlled local model)
but is capture-only: every scene is taken at both 100x30 and 80x24 into
golden/before/real-<name>-<W>x<H>.json. No PTC, no paid model calls.

Determinism: model turns 1, 10, 11 stall at their first chunk behind
threading.Event gates, so streaming and queue states do not race the
resize/pump cycle.

Usage:
  python3 capture_golden_real.py --binary <built-tui> --dsh /usr/bin/dsh
"""
import argparse
import json
import os
import re
import select
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from terminal import Terminal, wait_for  # noqa: E402
from golden import capture_cells  # noqa: E402

parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--dsh', required=True)
parser.add_argument('--out', default=str(Path(__file__).resolve().parent / 'golden' / 'before'))
args = parser.parse_args()
BINARY = str(Path(args.binary).resolve())
OUT = Path(args.out)
SIZES = [(100, 30), (80, 24)]
model_calls = []
hold = {1: threading.Event(), 10: threading.Event(), 11: threading.Event()}
project = None


class Model(BaseHTTPRequestHandler):
    """Scripted turns: 1 chat (held), 2 read tool, 3 chat, 4/6 approval
    probe, 5/7/9 chat, 8 three-question ask, 10-12 queue (10/11 held)."""

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
            if number == 8:
                names = [entry['function']['name'] for entry in body.get('tools', [])]
                tool = next((name for name in names
                             if name == 'ask_user_question' or name.endswith('__ask_user_question')), None)
                assert tool, f'Question tool not offered: {names}'
                questions = [
                    {'id': 'language', 'header': 'Language',
                     'question': 'Which language should the agent use?',
                     'options': [{'label': 'Go', 'description': 'Use the current Go implementation.'}, {'label': 'Rust'}]},
                    {'id': 'checks', 'question': 'Which checks should run?', 'multi_select': True,
                     'options': [{'label': 'Unit tests'}, {'label': 'PTY checks'}, {'label': 'CI'}]},
                    {'id': 'notes', 'question': 'Add implementation notes.'},
                ]
                send({'role': 'assistant', 'tool_calls': [
                    {'index': 0, 'id': 'question-8', 'type': 'function',
                     'function': {'name': tool, 'arguments': json.dumps({'questions': questions, 'timeout': -1})}},
                ]})
                send({}, 'tool_calls')
                self.wfile.write(b'data: [DONE]\n\n')
                self.wfile.flush()
                return
            if number in (4, 6):
                names = [entry['function']['name'] for entry in body.get('tools', [])]
                tool = next((name for name in names
                             if name == 'tui_approval_probe' or name.endswith('__tui_approval_probe')), None)
                assert tool, f'Approval test tool not offered: {names}'
                marker = 'grant' if number == 4 else 'reject'
                send({'role': 'assistant', 'tool_calls': [
                    {'index': 0, 'id': 'approval-' + marker, 'type': 'function',
                     'function': {'name': tool, 'arguments': json.dumps({'marker': marker})}},
                ]})
                send({}, 'tool_calls')
                self.wfile.write(b'data: [DONE]\n\n')
                self.wfile.flush()
                return
            if number == 2:
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
                if number in hold and index == 0:
                    assert hold[number].wait(60), f'Golden queue/stream turn {number} was not released'
                time.sleep(0.5)
            send({}, 'stop')
            self.wfile.write(b'data: [DONE]\n\n')
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass


def scene(app, name, marker):
    """Capture a named state at both baseline sizes (marker must stay visible)."""
    markers = (marker,) if isinstance(marker, str) else tuple(marker)
    for columns, rows in SIZES:
        app.resize(columns, rows)
        app.wait(markers, timeout=20)
        for _ in range(8):
            app.pump()
        path = OUT / f'real-{name}-{columns}x{rows}.json'
        capture_cells(app, path, source=f'real dsh golden: {name}')
        print(f'  {name} {columns}x{rows} -> {path.name}', flush=True)
    # Reset to the larger size WITHOUT a strict marker wait: scrolled chat
    # content can reflow off-screen after a resize, which made strict waits
    # flaky. The next step drives the UI itself and waits for its own state.
    app.resize(100, 30)
    for _ in range(6):
        app.pump()


with tempfile.TemporaryDirectory(prefix='dsh-tui-golden-real-') as home:
    root = Path(home)
    model = ThreadingHTTPServer(('127.0.0.1', 0), Model)
    threading.Thread(target=model.serve_forever, daemon=True).start()
    profile = root / 'dsh/profiles/web'
    profile.mkdir(parents=True)
    (profile / 'package.json').write_text(json.dumps({
        'name': 'dsh-tui-golden-real', 'private': True, 'dependencies': {},
        'dsh': {'profile': {'bundles': ['@deepseek-ai/dsh-base', '@deepseek-ai/dsh-web-app']}}}))
    (profile / 'approval-probe.mjs').write_text(Path(__file__).with_name('approval_probe.mjs').read_text())
    (profile / 'cordis.yml').write_text('[]\n')
    dsh = Path(args.dsh).resolve()
    # System installs nest the web-app under dsh/node_modules; CI's flat npm
    # layout keeps it in the top-level node_modules. Accept whichever has it.
    preset_file = Path('@deepseek-ai/dsh-web-app/presets/standard.patch.yml')
    candidates = [dsh.parents[1] / 'node_modules',
                  next((path for path in dsh.parents if path.name == 'node_modules'), None)]
    modules = next((path for path in candidates if path and (path / preset_file).is_file()), None)
    assert modules, f'dsh-web-app presets not found near {dsh}'
    (profile / 'node_modules').symlink_to(modules, target_is_directory=True)
    (profile / 'cordis.patch.yml').write_text(json.dumps([
        {'insert': [{'id': 'tui-approval-probe', 'name': str(profile / 'approval-probe.mjs'),
                     'config': {'directory': str(root / 'project')}}]},
        {'id': 'llm-pi-ai', 'config': {'providers': {'tui-fixture': {
            'apiKeyEnv': 'TUI_TEST_API_KEY', 'api': 'openai-completions',
            'baseURL': f'http://127.0.0.1:{model.server_port}/v1',
            'models': [{'id': 'fixture', 'contextWindow': 131072, 'maxTokens': 2048,
                        'reasoningEfforts': False, 'input': ['text']},
                       {'id': 'fixture-alt', 'contextWindow': 131072, 'maxTokens': 2048,
                        'reasoningEfforts': False, 'input': ['text']}],
            'retryPolicy': {'mode': 'normal', 'maxRetries': 0}}}}},
        {'id': 'agent-default-model', 'config': {'provider': 'tui-fixture', 'model': 'fixture'}},
        {'id': 'session-title-llm', 'disabled': True},
    ]))
    # Standard preset overlay: keep the installed preset, enable timed questions.
    preset = (modules / '@deepseek-ai/dsh-web-app/presets/standard.patch.yml').read_text()
    config = preset.split('      config:\n', 1)[1]
    plugin = "            name: '@deepseek-ai/dsh-tool-ask-user'\n"
    assert config.count(plugin) == 1, 'Pinned question preset shape changed'
    config = config.replace(plugin, plugin + '            config:\n              mode: timed\n')
    patch_file = profile / 'cordis.patch.yml'
    patches = json.loads(patch_file.read_text())
    patch_file.write_text(''.join('- ' + json.dumps(patch) + '\n' for patch in patches)
                          + '- id: preset-standard\n  config:\n'
                          + ''.join(line[4:] if line.startswith('    ') else line
                                    for line in config.splitlines(keepends=True)))
    project = root / 'project'
    project.mkdir()
    (project / 'notes.txt').write_text('\n'.join(f'Tool output line {i:02d}' for i in range(1, 13)) + '\n')
    env = {key: value for key, value in os.environ.items()
           if key not in ('GH_TOKEN', 'GITHUB_TOKEN', 'CODESPACES_GITHUB_TOKEN', 'DSH_TUI_URL')}
    env.pop('NO_COLOR', None)
    env.update(DSH_HOME=str(root / 'dsh'), TUI_TEST_API_KEY='test-only-not-a-real-key',
               TERM='xterm-256color', COLORTERM='truecolor',
               XDG_CONFIG_HOME=str(root / 'config'), XDG_DATA_HOME=str(root / 'data'))
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
        deadline = time.monotonic() + 40
        while time.monotonic() < deadline and process.poll() is None:
            if select.select([process.stdout], [], [], 0.5)[0]:
                line = process.stdout.readline()
                log.write(line)
                log.flush()
                if 'dsh web: ' in line:
                    login = line.split('dsh web: ')[1].strip()
                    break
        if not login:
            raise RuntimeError('Real dsh did not start:\n' + (root / 'backend.log').read_text()[-3000:])
        reader = threading.Thread(target=lambda: [log.write(l) or log.flush() for l in process.stdout],
                                  daemon=True)
        reader.start()
        app = Terminal(BINARY, ['--new'], {**env, 'DSH_TUI_URL': login})
        app.wait('Введите сообщение.', timeout=30)
        session_id = re.search(r'Сессия · (session-[a-z0-9-]+)', app.text).group(1)
        print(f'session: {session_id}', flush=True)
        # ---- landing with a live session
        scene(app, 'landing', ('С чего начнём?', 'Сессия · session-'))
        # ---- model picker: browse at both sizes, then confirm fixture-alt
        app.send('MODEL DRAFT')
        app.wait('> MODEL DRAFT')
        app.send('\x1b[24~')
        app.wait('Модель сессии')
        app.wait('Текущая: tui-fixture/fixture')
        app.send('fixture-alt')
        app.wait('Выбор: tui-fixture/fixture-alt')
        scene(app, 'models', ('Модель сессии', 'Выбор: tui-fixture/fixture-alt'))
        app.send('\r')
        app.wait('выбор подтверждён', timeout=20)
        app.wait('Текущая: tui-fixture/fixture-alt')
        app.send('\x1b')
        app.wait('> MODEL DRAFT')
        app.send('\x15')
        # ---- first streamed reply: partial frame (held), then completed
        app.command('Reply briefly without invoking any tools.')
        app.wait('Ответ 1: ', timeout=30)
        assert 'настоящего dsh.' not in app.text, 'Stream completed before the partial capture'
        scene(app, 'chat-streaming', 'Ответ 1: ')
        hold[1].set()
        app.wait('Ответ 1: проверка транспорта настоящего dsh.', timeout=20)
        assert model_calls and model_calls[0]['model'] == 'fixture-alt', 'Confirmed TUI selection did not reach the model'
        scene(app, 'chat-final', 'Ответ 1: проверка транспорта настоящего dsh.')
        app.wait('Enter — отправить', timeout=20)
        # ---- tool cards: read success + read error
        app.command('Read notes.txt and missing-file.txt using tools.')
        app.wait('Ответ 3: проверка транспорта настоящего dsh.', timeout=30)
        app.wait('Enter — отправить', timeout=20)
        assert len(model_calls) == 3
        results = [message for message in model_calls[2]['messages'] if message.get('role') == 'tool']
        assert len(results) == 2 and any('Tool output line 12' in json.dumps(m) for m in results)
        app.send('preserved draft')
        app.wait('> preserved draft')
        app.send('\x1b[17~')
        app.wait('> ▸ read ·', timeout=10)
        if '> ▸ read · ! Ошибка' in app.text:
            app.send('\x1b[17~')
        app.wait('> ▸ read · ✓ Готово', timeout=10)
        scene(app, 'tool-cards', ('> ▸ read · ✓ Готово', 'preserved draft'))
        app.send('\x1b')
        wait_for(lambda: (app.pump(), '> preserved draft' in app.text)[1])
        app.send('\x15')
        # ---- pending permission (grant)
        app.command('Request permission to create the grant marker.')
        app.wait('Запрос разрешения dsh', timeout=30)
        app.wait('Create grant.marker', timeout=10)
        scene(app, 'approval-pending', ('Запрос разрешения dsh', 'Create grant.marker'))
        app.send('\t\x1b[C\r')
        app.wait('Ответ 5: проверка транспорта настоящего dsh.', timeout=30)
        assert (project / 'grant.marker').read_text() == 'approved\n'
        app.wait('Разрешено один раз', timeout=10)
        app.send('\x1b[5~' * 20)
        app.wait('Enter — отправить', timeout=25)
        # ---- pending permission (reject)
        app.command('Request permission to create the reject marker.')
        app.wait('Create reject.marker', timeout=30)
        scene(app, 'approval-reject', ('Запрос разрешения dsh', 'Create reject.marker'))
        app.send('\t\r')
        app.wait('Ответ 7: проверка транспорта настоящего dsh.', timeout=30)
        assert not (project / 'reject.marker').exists()
        app.wait('Отказано', timeout=10)
        app.send('\x1b[5~' * 20)
        app.wait('Enter — отправить', timeout=25)
        # ---- question form: three questions, multi-select, freeform
        app.command('Ask for language, checks, and free-form notes.')
        app.wait('Which language should the agent use?', timeout=30)
        app.send('\x1b')
        def question_deferred():
            app.pump()
            return 'Вопрос агента' not in app.text
        wait_for(question_deferred)
        app.send('retained draft')
        app.wait('> retained draft')
        app.send('\x1b[18~')
        app.wait('Вопрос агента')
        app.wait('Which language should the agent use?', timeout=10)
        app.send('\t\r\x1b[C')
        app.wait('Which checks should run?', timeout=10)
        app.send('\t\r\x1b[B ')
        app.wait('[✓] PTY checks')
        scene(app, 'question-options', ('Which checks should run?', '[✓] PTY checks'))
        app.send('\x1b[C')
        app.wait('Add implementation notes.', timeout=10)
        app.send('\tGolden notes work.\rSecond draft line.')
        app.wait('Second draft line.', timeout=10)
        scene(app, 'question-answered', ('Add implementation notes.', 'Second draft line.'))
        app.send('\x1b[15~')
        app.wait('Ответ 9: проверка транспорта настоящего dsh.', timeout=30)
        answered = [m for m in model_calls[8]['messages'] if m.get('role') == 'tool' and m.get('tool_call_id') == 'question-8']
        assert len(answered) == 1, 'Question answer was not returned as the tool result'
        app.wait('> retained draft', timeout=15)
        app.send('\x1b[5~' * 20)
        app.wait('Enter — отправить', timeout=25)
        # ---- queue: one running (held), two pending; then one pending
        app.command('QUEUE FIRST')
        app.wait('Ответ 10: ', timeout=30)
        app.wait('Enter — в очередь')
        app.command('QUEUE SECOND')
        app.wait('ОЧЕРЕДЬ DSH · 1', timeout=15)
        app.command('QUEUE THIRD')
        app.wait('ОЧЕРЕДЬ DSH · 2', timeout=15)
        assert len(model_calls) == 10, 'Queued prompt interrupted the running model turn'
        scene(app, 'queue', 'Enter — в очередь')
        hold[10].set()
        app.wait('Ответ 10: проверка транспорта настоящего dsh.', timeout=20)
        app.wait('Ответ 11: ', timeout=30)
        scene(app, 'queue-one', 'Enter — в очередь')
        hold[11].set()
        app.wait('Ответ 11: проверка транспорта настоящего dsh.', timeout=20)
        app.wait('Ответ 12: проверка транспорта настоящего dsh.', timeout=30)
        app.wait('Enter — отправить', timeout=20)
        order = [json.dumps(model_calls[n]['messages']) for n in (9, 10, 11)]
        assert 'QUEUE FIRST' in order[0] and 'QUEUE SECOND' in order[1] and 'QUEUE THIRD' in order[2]
        app.close(root / 'golden-main.ansi')
        app = None
        # ---- history: collapsed tool cards at baseline sizes (fresh terminals)
        for columns, rows in SIZES:
            history_app = Terminal(BINARY, ['--session', session_id], {**env, 'DSH_TUI_URL': login},
                                   columns=columns, rows=rows)
            try:
                history_app.wait('Ответ 12: проверка транспорта настоящего dsh.', timeout=30)
                history_app.send('HISTORY TOOL DRAFT')
                history_app.send('\x1b[23~')
                history_app.wait('PgUp/PgDn')
                history_app.send('\x1b[H')
                history_app.send('\x1b[17~')
                history_app.wait('> ▸ read ·')
                if '> ▸ read · ! Ошибка' in history_app.text:
                    history_app.send('\x1b[17~')
                history_app.wait('> ▸ read · ✓ Готово')
                assert 'Tool output line 12' not in history_app.text, 'History details start expanded'
                # Expanded tool card: capture at the top of the details block.
                history_app.send('\r')
                history_app.wait('> ▾ read · ✓ Готово')
                history_app.wait('Аргументы:')
                for _ in range(8):
                    history_app.pump()
                path = OUT / f'real-tool-expanded-{columns}x{rows}.json'
                capture_cells(history_app, path, source='real dsh golden: tool expanded (history view)')
                print(f'  tool-expanded {columns}x{rows} -> {path.name}', flush=True)
            finally:
                history_app.close(root / f'golden-history-{columns}.ansi')
    finally:
        if app:
            app.close(root / 'golden-terminal.ansi')
        if process.poll() is None:
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
        process.wait(timeout=10)
        log.close()
        model.shutdown()

print('real golden captures complete', flush=True)
for path in sorted(OUT.glob('real-*.json')):
    print(' ', path.name, flush=True)
