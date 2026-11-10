"""Per-test isolated backends and terminals; tests execute only the installed bin."""
import json
import os
from pathlib import Path
import re
import signal
import socket
import subprocess
import urllib.request

import pytest

from terminal import Terminal, wait_for


def free_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


def local_get(url):
    with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(url, timeout=2) as response:
        return response.read().decode()


class Lab:
    def __init__(self, root, name):
        self.root = root
        self.name = name
        self.apps = []
        self.backends = []
        self.executable = os.environ.get('TUI_TEST_BIN', '/usr/bin/dsh-launcher-tui')
        self.fake = str(Path(__file__).parent / 'fakes' / 'dsh.js')
        self.port = free_port()
        self.state = root / 'backend'
        self.state.mkdir()
        self.env = {**os.environ, 'HOME': str(root), 'DSH_HOME': str(root / 'dsh-home'),
                    'TUI_FAKE_STATE': str(self.state), 'TERM': 'xterm-256color'}
        for key in ('DISPLAY', 'WAYLAND_DISPLAY', 'DSH_TUI_URL', 'DSH_BIN', 'DSH_ARGS',
                    'DSH_CWD', 'NODE_OPTIONS', 'NODE_V8_COVERAGE', 'DSH_LAUNCHER_GITHUB_TOKEN'):
            self.env.pop(key, None)

    def ready(self):
        path = self.state / 'ready.json'
        if not path.exists():
            return None
        try:
            return json.loads(path.read_text())
        except json.JSONDecodeError:
            return None

    def requests(self, method=None):
        path = self.state / 'requests.jsonl'
        values = [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []
        return [value for value in values if method is None or value.get('method') == method]

    def request(self, method, count=1):
        return wait_for(lambda: self.requests(method) if len(self.requests(method)) >= count else None)

    def backend(self):
        output = open(self.root / 'backend.log', 'wb')
        proc = subprocess.Popen([self.fake, '--port', str(self.port)], env=self.env,
                                stdout=output, stderr=subprocess.STDOUT, start_new_session=True)
        self.backends.append((proc, output))
        return wait_for(self.ready)

    def app(self, *, attach=False, args=None, env=None):
        arguments = ['--cwd', str(self.root)]
        if attach:
            self.backend()
            app_env = {**self.env, 'DSH_TUI_URL': self.ready()['url']}
        else:
            arguments += ['--dsh', self.fake, '--port', str(self.port)]
            app_env = self.env
        app = Terminal(self.executable, arguments + (args or []), {**app_env, **(env or {})})
        self.apps.append(app)
        return app

    def close(self):
        results = Path(os.environ.get('TUI_TEST_RESULTS', str(self.root / 'results')))
        results.mkdir(parents=True, exist_ok=True)
        try:
            for index, app in enumerate(reversed(self.apps)):
                app.close(results / f'{self.name}-{index}.terminal.log')
        finally:
            for proc, output in self.backends:
                try:
                    os.killpg(proc.pid, signal.SIGTERM)
                    proc.wait(timeout=3)
                except (ProcessLookupError, subprocess.TimeoutExpired):
                    pass
                finally:
                    try:
                        os.killpg(proc.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                    proc.wait(timeout=3)
                    output.close()
            log = self.state / 'requests.jsonl'
            if log.exists():
                (results / f'{self.name}.requests.jsonl').write_bytes(log.read_bytes())


@pytest.fixture
def lab(tmp_path, request):
    fixture = Lab(tmp_path, re.sub(r'[^\w.-]', '_', request.node.name))
    try:
        yield fixture
    finally:
        fixture.close()
