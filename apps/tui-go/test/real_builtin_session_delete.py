"""Built-in permanent deletion against actual dsh without Archive Manager in a disposable HOME.

Only fixture-owned IDs may be deleted. The launcher backend and a disposable
seed plugin are explicitly loaded; no third-party deletion plugin is installed.
"""
import argparse
import http.cookiejar
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import urllib.parse
import urllib.request
import uuid

from terminal import Terminal, wait_for

parser = argparse.ArgumentParser()
parser.add_argument('--dsh', required=True)
parser.add_argument('--backend', required=True, help='Launcher built-in backend module entrypoint')
parser.add_argument('--binary', help='TUI binary for confirmation and running-session guards')
parser.add_argument('--capture', help='Optional prefix for actual terminal cell captures')
args = parser.parse_args()

class ConnectResponse(urllib.request.HTTPErrorProcessor):
    def http_response(self, request, response):
        return response if response.status == 302 else super().http_response(request, response)

calls = []
release = threading.Event()
class Model(BaseHTTPRequestHandler):
    def log_message(self, *_): pass
    def do_POST(self):
        calls.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.end_headers()
        try:
            first = {'choices': [{'index': 0, 'delta': {'role': 'assistant', 'content': 'DELETE_RUNNING_FIXTURE'}, 'finish_reason': None}]}
            self.wfile.write(('data: ' + json.dumps(first) + '\n\n').encode())
            self.wfile.flush()
            release.wait(30)
            self.wfile.write(b'data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}\n\ndata: [DONE]\n\n')
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError): pass

def capture(app, suffix):
    if not args.capture: return
    for _ in range(4): app.pump()
    cells = []
    for y in range(app.screen.lines):
        for x in range(app.screen.columns):
            c = app.screen.buffer[y][x]
            cells.append({'x': x, 'y': y, 'text': c.data, 'fg': c.fg, 'bg': c.bg, 'bold': c.bold, 'reverse': c.reverse})
    path = Path(args.capture + '-' + suffix + '.json')
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps({'columns': app.screen.columns, 'rows': app.screen.lines, 'cells': cells}, ensure_ascii=False))

with tempfile.TemporaryDirectory(prefix='dsh-permanent-delete-') as directory:
    root = Path(directory)
    project = root / 'project'
    project.mkdir()
    sentinel = project / 'user-project.txt'
    sentinel.write_text('Project files must survive permanent session deletion.\n')
    modules = next(p for p in Path(args.dsh).resolve().parents if p.name == 'node_modules')
    ids = {name: 'session-' + uuid.uuid4().hex for name in ['primary', 'child', 'grandchild', 'fork', 'unrelated']}
    seed = root / 'seed.mjs'
    seed.write_text("""import {writeFile} from 'node:fs/promises';
export const inject = ['sessions', 'sessionPersistence'];
export async function apply(ctx) {
 const ids = JSON.parse(process.env.DELETE_SEED_IDS);
 const cwd = process.env.DELETE_SEED_CWD;
 const artifacts = {};
 for (const key of ['primary','child','grandchild','fork','unrelated']) {
  const meta = {cwd};
  if (key === 'child' || key === 'grandchild') {
   meta.origin = 'subagent'; meta.parentSession = ids[key === 'child' ? 'primary' : 'child'];
   meta.delegationDepth = key === 'child' ? 1 : 2;
  }
  // Historical ordinary forks can retain parentSession. It must never be
  // mistaken for ownership: origin=subagent is what authorizes cascading.
  if (key === 'fork') meta.parentSession = ids.primary;
  const session = ctx.sessions.prepare(ids[key], {meta});
  const handle = await ctx.sessionPersistence.create(session.header);
  await handle.flush();
  await handle.close();
  artifacts[key] = ctx.sessionPersistence.locate(session.header)?.path;
 }
 await writeFile(process.env.DELETE_SEED_ARTIFACTS, JSON.stringify(artifacts));
}
""")
    model = ThreadingHTTPServer(('127.0.0.1', 0), Model)
    threading.Thread(target=model.serve_forever, daemon=True).start()
    profile = root / 'dsh/profiles/web'
    profile.mkdir(parents=True)
    (profile / 'node_modules').symlink_to(modules, target_is_directory=True)
    (profile / 'package.json').write_text(json.dumps({'name': 'isolated-delete-test', 'private': True,
        'dependencies': {}, 'dsh': {'profile': {'bundles': ['@deepseek-ai/dsh-base', '@deepseek-ai/dsh-web-app']}}}))
    (profile / 'cordis.yml').write_text('[]\n')
    (profile / 'cordis.patch.yml').write_text(json.dumps([
        {'insert': [{'id': 'launcher-session-delete', 'name': str(Path(args.backend).resolve())},
                    {'id': 'delete-fixture-seed', 'name': str(seed)}]},
        {'id': 'llm-pi-ai', 'config': {'providers': {'delete-fixture': {'apiKeyEnv': 'DELETE_TEST_KEY',
            'api': 'openai-completions', 'baseURL': f'http://127.0.0.1:{model.server_port}/v1',
            'models': [{'id': 'fixture', 'name': 'Delete fixture', 'contextWindow': 131072, 'maxTokens': 128,
                        'reasoningEfforts': False, 'input': ['text']}], 'retryPolicy': {'mode': 'normal', 'maxRetries': 0}}}}},
        {'id': 'agent-default-model', 'config': {'provider': 'delete-fixture', 'model': 'fixture'}},
        {'id': 'session-title-llm', 'disabled': True}, {'id': 'goal-round-driver', 'disabled': True}]))
    env = {k: v for k, v in os.environ.items() if k not in ('DSH_TUI_URL', 'GH_TOKEN', 'GITHUB_TOKEN', 'NO_COLOR', 'DISPLAY', 'WAYLAND_DISPLAY')}
    env.update(HOME=str(root), DSH_HOME=str(root / 'dsh'), XDG_CONFIG_HOME=str(root / 'config'),
               XDG_DATA_HOME=str(root / 'data'), DELETE_TEST_KEY='isolated-only', DELETE_SEED_IDS=json.dumps(ids), DELETE_SEED_CWD=str(project), DELETE_SEED_ARTIFACTS=str(root / 'artifacts.json'), TERM='xterm-256color', COLORTERM='truecolor')
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0)); port = sock.getsockname()[1]
    log_lines = []
    login = []
    process = subprocess.Popen([str(Path(args.dsh).resolve()), '--profile', 'web', '--no-open', '--port', str(port)],
        cwd=project, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, start_new_session=True)
    def drain():
        for line in process.stdout:
            log_lines.append(line)
            if 'dsh web: ' in line: login.append(line.split('dsh web: ')[1].strip())
    threading.Thread(target=drain, daemon=True).start()
    app = None
    try:
        wait_for(lambda: login, timeout=40)
        parsed = urllib.parse.urlsplit(login[0])
        origin = urllib.parse.urlunsplit((parsed.scheme, parsed.netloc, '', '', ''))
        opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()), ConnectResponse())
        opener.open(origin + '/connect?' + parsed.query).close()
        def rpc(method, payload, transport=None):
            rpc_id = uuid.uuid4().hex
            body = {'type': 'client-request', 'rpcId': rpc_id, 'method': method, 'payload': {'args': payload}}
            request = urllib.request.Request(origin + '/api/' + method, data=json.dumps(body).encode(), headers={'Content-Type': 'application/json', 'Origin': origin})
            with (transport or opener).open(request, timeout=15) as response: result = json.load(response)
            assert result['rpcId'] == rpc_id, result
            return result['result']
        def listed():
            result = rpc('session/list', {'_request': {}})
            assert result['ok'], result
            return {s['sessionId'] for s in result['value']['items']}
        try:
            wait_for(lambda: listed() == set(ids.values()), timeout=20)
        except AssertionError:
            print('Seed diagnostics:', {'listed': sorted(listed()), 'expected': sorted(ids.values()), 'artifacts': (root / 'artifacts.json').read_text() if (root / 'artifacts.json').exists() else None, 'files': [str(p.relative_to(root)) for p in (root / 'dsh').rglob('session*.json*')]})
            raise
        wait_for(lambda: (root / 'artifacts.json').exists(), timeout=20)
        artifacts = json.loads((root / 'artifacts.json').read_text())
        for path in artifacts.values():
            assert path and Path(path).is_relative_to(root) and Path(path).exists(), artifacts
        capabilities = rpc('launcherSessions/capabilities', {})
        assert capabilities['ok'] and capabilities['value']['protocolVersion'] == 1 and capabilities['value']['supportsPermanentDelete'], capabilities
        other_client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()), ConnectResponse())
        other_client.open(origin + '/connect?' + parsed.query).close()
        # Populate a second independent connection's passive session observation.
        sampled = rpc('session/state', {'request': {'sessionId': ids['primary']}}, other_client)
        assert sampled['ok'], sampled
        baseline = {ids['fork'], ids['unrelated']}
        owned = set(ids.values())
        second = ids['primary']
        def delete(session_id):
            assert session_id in owned, 'Refusing deletion outside fixture-owned IDs'
            result = rpc('launcherSessions/deleteSession', {'sessionId': session_id})
            assert result['ok'] and result['value'].get('deleted') is True, result
            assert session_id not in listed(), 'Deletion acknowledged but session still listed'
            readback = rpc('session/events', {'request': {'address': {'kind': 'session', 'sessionId': session_id}, 'maxMessages': 5}})
            assert not readback['ok'], 'Deleted session is still readable'
        # The primary owns the subagent tree. Its ordinary fork and the
        # unrelated session are deliberately preserved as independent work.
        if args.binary:
            app = Terminal(str(Path(args.binary).resolve()), ['--session', second], {**env, 'DSH_TUI_URL': login[0]})
            app.wait('DeepSeek Harness', timeout=25)
            app.send('DELETE RETAINED DRAFT')
            app.wait('DELETE RETAINED DRAFT')
            def open_delete():
                app.send('\x10')
                app.wait('Найти команду…')
                app.send('Удалить сессию навсегда')
                app.wait('/delete-session')
                app.send('\r')
                app.wait('Удалить сессию навсегда?')
            open_delete()
            for columns, rows in [(80, 24), (100, 30), (160, 40)]:
                app.resize(columns, rows)
                app.wait('Удалить сессию навсегда?')
                capture(app, f'confirmation-{columns}x{rows}')
            # Enter defaults to cancellation. It must leave both the durable
            # session and the exact user draft available.
            app.send('\r')
            app.wait('DELETE RETAINED DRAFT', absent=('Удалить сессию навсегда?',))
            assert listed() == owned, 'Default Enter deleted a session'
            app.send('\x15')
            app.command('Hold this turn while checking permanent deletion guards.')
            app.wait('DELETE_RUNNING_FIXTURE', timeout=25)
            app.send('RUNNING RETAINED DRAFT')
            app.wait('RUNNING RETAINED DRAFT')
            open_delete()
            app.send('\t\r')
            app.wait(('Удаление не подтверждено.', 'остановите'), timeout=20)
            assert listed() == owned, 'Running-session guard failed to preserve the durable session'
            state = rpc('session/state', {'request': {'sessionId': second}})
            assert state['ok'] and state['value']['running'], 'Delete guard stopped or detached the running session'
            capture(app, 'running-refused')
            app.send('\x1b')
            app.wait('RUNNING RETAINED DRAFT')
            app.send('\x1b[19~')
            app.wait('Выполнение остановлено.', timeout=20)
            def stopped():
                state = rpc('session/state', {'request': {'sessionId': second}})
                return state['ok'] and not state['value']['running']
            wait_for(stopped, timeout=20)
            open_delete()
            app.send('\t\r')
            wait_for(lambda: second not in listed(), timeout=20)
            app.wait('Новая', absent=('Удалить сессию навсегда?',))
            capture(app, 'deleted')
            readback = rpc('session/events', {'request': {'address': {'kind': 'session', 'sessionId': second}, 'maxMessages': 5}})
            assert not readback['ok'], 'TUI-deleted session journal is still readable'
            assert len(calls) == 1, 'Deletion or guard made an extra model request'
            print('Actual TUI delete: default cancellation, retained draft, three sizes, running refusal, stop then confirmed deletion OK')
        else:
            delete(second)
        before_stale_send = len(calls)
        stale = rpc('session/prompt', {'request': {'sessionId': second,
            'requestId': 'request-' + uuid.uuid4().hex, 'mode': 'queue',
            'content': [{'type': 'text', 'text': 'MUST NOT RESURRECT DELETED SESSION'}]}}, other_client)
        assert not stale['ok'], 'Stale second client accepted a prompt for a deleted session'
        assert len(calls) == before_stale_send, 'Stale client invoked the model after deletion'
        assert listed() == baseline, 'Cascade removed an independent fork/session or retained a subagent'
        for key in ['primary', 'child', 'grandchild']:
            readback = rpc('session/events', {'request': {'address': {'kind': 'session', 'sessionId': ids[key]}, 'maxMessages': 5}})
            assert not readback['ok'], f'Deleted {key} journal is still readable'
        assert sentinel.read_text() == 'Project files must survive permanent session deletion.\n'
        for key, path in artifacts.items():
            assert Path(path).exists() == (key in ('fork', 'unrelated')), f'Wrong durable artifact lifetime: {key}: {path}'
        print('Actual built-in deletion without Archive Manager: primary + recursive subagents removed, independent fork and unrelated session preserved OK')
    finally:
        if sys.exc_info()[0] is not None:
            print('Isolated backend diagnostics:\n' + ''.join(line for line in log_lines[-70:] if 'token=' not in line))
        release.set()
        if app is not None: app.close(root / 'terminal.log')
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGTERM)
            try: process.wait(timeout=8)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL); process.wait(timeout=5)
        model.shutdown(); model.server_close()
