"""Actual OpenCode question/Enter reference with a local model and isolated profile."""
import argparse
import json
import os
from pathlib import Path
import shutil
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from terminal import Terminal

parser = argparse.ArgumentParser()
parser.add_argument('--binary', default=shutil.which('opencode'))
parser.add_argument('--output', default='build/opencode-question-reference-v230')
parser.add_argument('--single', action='store_true', help='Offer one question instead of two')
args = parser.parse_args()
if not args.binary:
    parser.error('OpenCode was not found on PATH; supply --binary')
binary = str(Path(args.binary).resolve())
output = Path(args.output).resolve()
output.mkdir(parents=True, exist_ok=True)
requests = []
question_sent = False

class Model(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_GET(self):
        data = json.dumps({'object': 'list', 'data': [{'id': 'fixture', 'object': 'model'}]}).encode()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self):
        global question_sent
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        requests.append(body)
        (output / 'requests.json').write_text(json.dumps(requests, ensure_ascii=False, indent=2))
        names = [tool.get('function', {}).get('name', '') for tool in body.get('tools', [])]
        question = next((name for name in names if 'question' in name.lower()), None)
        print('POST', self.path, 'tools=', names, flush=True)
        if question and not question_sent:
            question_sent = True
            arguments = {'questions': [
                {'question': 'Choose the first fixture option', 'header': 'First', 'options': [
                    {'label': 'Alpha', 'description': 'First fixture choice'},
                    {'label': 'Beta', 'description': 'Second fixture choice'}]},
                {'question': 'Choose the second fixture option', 'header': 'Second', 'options': [
                    {'label': 'Gamma', 'description': 'Third fixture choice'},
                    {'label': 'Delta', 'description': 'Fourth fixture choice'}]}]}
            if args.single:
                arguments['questions'] = arguments['questions'][:1]
            delta = {'role': 'assistant', 'tool_calls': [{'index': 0, 'id': 'call_question_fixture', 'type': 'function',
                       'function': {'name': question, 'arguments': json.dumps(arguments)}}]}
            finish = 'tool_calls'
        else:
            delta = {'role': 'assistant', 'content': 'QUESTION_REFERENCE_DONE' if question_sent else 'Fixture title'}
            finish = 'stop'
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.send_header('Cache-Control', 'no-cache')
        self.end_headers()
        for data in [ {'id': 'fixture-response', 'object': 'chat.completion.chunk', 'created': 1, 'model': 'fixture',
                       'choices': [{'index': 0, 'delta': delta, 'finish_reason': None}]},
                      {'id': 'fixture-response', 'object': 'chat.completion.chunk', 'created': 1, 'model': 'fixture',
                       'choices': [{'index': 0, 'delta': {}, 'finish_reason': finish}],
                       'usage': {'prompt_tokens': 10, 'completion_tokens': 10, 'total_tokens': 20}}]:
            self.wfile.write(('data: ' + json.dumps(data) + '\n\n').encode())
        self.wfile.write(b'data: [DONE]\n\n')
        self.wfile.flush()

def settle(app, seconds=1):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        app.pump()
        time.sleep(.05)

def capture(app, name, sizes=False):
    for columns, rows in ([(160,40),(100,30),(80,24)] if sizes else [(app.screen.columns,app.screen.lines)]):
        app.resize(columns, rows)
        settle(app)
        cells = [{'x': x, 'y': y, 'text': (cell := app.screen.buffer[y][x]).data,
                  'fg': cell.fg, 'bg': cell.bg, 'bold': cell.bold, 'reverse': cell.reverse}
                 for y in range(rows) for x in range(columns)]
        stem = output / f'{name}-{columns}x{rows}'
        stem.with_suffix('.txt').write_text(app.text)
        stem.with_suffix('.json').write_text(json.dumps({'source': 'Actual installed OpenCode with local scripted model',
            'columns': columns, 'rows': rows, 'cells': cells}, ensure_ascii=False))
    print(name + ':\n' + app.text, flush=True)

with tempfile.TemporaryDirectory(prefix='opencode-question-reference-') as directory:
    os.chdir(directory)
    server = ThreadingHTTPServer(('127.0.0.1', 0), Model)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    config = {'model': 'fixture/fixture', 'update': 'disable', 'share': 'disabled',
              'providers': {'fixture': {'name': 'Isolated fixture', 'env': ['FIXTURE_API_KEY'],
              'package': '@opencode/ai/providers/openai-compatible',
              'settings': {'baseURL': f'http://127.0.0.1:{server.server_port}/v1'},
              'models': {'fixture': {'name': 'Fixture'}}}}}
    env = {'PATH': str(Path(binary).parent)+os.pathsep+os.defpath, 'HOME': directory,
           'XDG_CONFIG_HOME': directory+'/config', 'XDG_DATA_HOME': directory+'/data',
           'XDG_STATE_HOME': directory+'/state', 'XDG_CACHE_HOME': directory+'/cache',
           'TMPDIR': directory, 'TERM': 'xterm-256color', 'COLORTERM': 'truecolor', 'LANG': 'C.UTF-8',
           'FIXTURE_API_KEY': 'isolated-local-fixture', 'OPENCODE_DISABLE_MODELS_FETCH': 'true',
           'OPENCODE_DISABLE_AUTOUPDATE': 'true', 'OPENCODE_CONFIG_CONTENT': json.dumps(config)}
    app = Terminal(binary, ['--standalone'], env, columns=100, rows=30)
    try:
        settle(app, 12)
        capture(app, 'home')
        app.command('Exercise the local fixture question tool.')
        app.wait('Choose the first fixture option', timeout=30)
        capture(app, 'question-initial', True)
        for step in range(1, 2 if args.single else 4):
            app.send('\r')
            settle(app, 2)
            capture(app, f'question-after-enter-{step}', True)
            if not args.single and step < 3:
                assert not any(message.get('role') == 'tool' for body in requests for message in body.get('messages', [])), 'Answers submitted before the Submit step'
            if step == 1 and not args.single:
                assert 'Choose the second fixture option' in app.text
            if step == 2 and not args.single:
                assert 'enter submit' in app.text and 'First: Alpha' in app.text
        app.wait('QUESTION_REFERENCE_DONE', timeout=15)
        results = [message for body in requests for message in body.get('messages', []) if message.get('role') == 'tool']
        assert results and 'Alpha' in results[-1]['content'], 'No selected question result reached the local model'
        if not args.single:
            assert 'Gamma' in results[-1]['content']
        print('PASS: one Enter selected and submitted.' if args.single else 'PASS: Enter selected each answer, opened Submit, then sent the selected answers.', flush=True)
    finally:
        app.close(output/'reference.ansi')
        server.shutdown()
        for log in Path(directory).rglob('*.log'):
            (output/('isolated-'+log.name)).write_bytes(log.read_bytes())
