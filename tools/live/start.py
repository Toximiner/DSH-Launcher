"""Supervise real dsh and a private web terminal in a Codespace or local server."""
import fcntl
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import threading
import time

ROOT = Path(__file__).resolve().parents[2]
DATA = Path(os.environ.get('DSH_LIVE_DATA', str(ROOT / '.live-data'))).resolve()
DATA.mkdir(parents=True, exist_ok=True)
if '--daemon' in sys.argv:
    with (DATA / 'service.log').open('a') as log:
        subprocess.Popen([sys.executable, str(Path(__file__).resolve())], stdin=subprocess.DEVNULL,
                         stdout=log, stderr=log, start_new_session=True)
    print('Live service starting; log:', DATA / 'service.log')
    sys.exit(0)
lock = (DATA / 'service.lock').open('w')
try:
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
except BlockingIOError:
    sys.exit(0)
(DATA / 'project').mkdir(exist_ok=True)
(DATA / 'service.pid').write_text(str(os.getpid()))
private_codespace = os.environ.get('CODESPACES') == 'true'
password = os.environ.get('DSH_LIVE_PASSWORD')
if not private_codespace and not password:
    raise SystemExit('Set DSH_LIVE_PASSWORD outside a private Codespace.')
env = {key: value for key, value in os.environ.items()
       if key not in ('GH_TOKEN', 'GITHUB_TOKEN', 'CODESPACES_GITHUB_TOKEN', 'DSH_LIVE_PASSWORD')}
env.update(DSH_HOME=str(DATA / 'dsh'), DSH_LIVE_DATA=str(DATA))
env['PATH'] = str(DATA / 'runtime/node_modules/.bin') + os.pathsep + env['PATH']
children = []
stopping = threading.Event()


def stop(*_):
    if stopping.is_set():
        return
    stopping.set()
    for process in reversed(children):
        try:
            os.killpg(process.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass


signal.signal(signal.SIGTERM, stop)
signal.signal(signal.SIGINT, stop)
signal.signal(signal.SIGHUP, stop)
dsh_port = int(os.environ.get('DSH_LIVE_BACKEND_PORT', '3080'))
port = int(os.environ.get('DSH_LIVE_PORT', '7681'))
dsh = subprocess.Popen([str(DATA / 'runtime/node_modules/.bin/dsh'), '--profile', 'web',
                        '--no-open', '--port', str(dsh_port)], cwd=DATA / 'project', env=env,
                       stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, start_new_session=True)
children.append(dsh)
ready = threading.Event()


def watch_dsh():
    with (DATA / 'backend.log').open('a') as log:
        os.chmod(DATA / 'backend.log', 0o600)
        for line in dsh.stdout:
            log.write(line)
            log.flush()
            match = re.search(r'dsh web:\s*(http://127\.0\.0\.1:' + str(dsh_port) + r'/\?token=\S+)', line)
            if match:
                path = DATA / 'login-url'
                path.write_text(match[1])
                path.chmod(0o600)
                ready.set()


threading.Thread(target=watch_dsh, daemon=True).start()
deadline = time.monotonic() + 120
while not ready.wait(0.2):
    if dsh.poll() is not None or stopping.is_set() or time.monotonic() >= deadline:
        stop()
        raise SystemExit('dsh failed to start; inspect .live-data/backend.log')
args = [str(DATA / 'bin/ttyd'), '-i', '0.0.0.0', '-p', str(port), '-W', '-O', '-m', '4',
        '-I', str(DATA / 'index.html'), '-w', str(DATA / 'project')]
if password:
    args += ['-c', 'demo:' + password]
args += [sys.executable, str(ROOT / 'tools/live/client.py')]
ttyd = subprocess.Popen(args, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
children.append(ttyd)
(DATA / 'ready.json').write_text(json.dumps({'ready': True, 'port': port, 'dsh_port': dsh_port,
                                            'real_backend': True, 'private_codespace': private_codespace}))
print('Real dsh, marketplace, and native TUI are ready on port', port, flush=True)
try:
    while not stopping.wait(1):
        if any(process.poll() is not None for process in children):
            break
finally:
    stop()
    for process in children:
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait()
    (DATA / 'ready.json').unlink(missing_ok=True)
