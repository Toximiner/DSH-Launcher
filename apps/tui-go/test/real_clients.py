"""Shared GUI/Go lease protocol against actual dsh, with isolated frontend processes."""
import argparse
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import threading
import time

from terminal import Terminal, wait_for

parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--dsh', required=True)
args = parser.parse_args()
binary = str(Path(args.binary).resolve())
dsh = str(Path(args.dsh).resolve())
module = str(Path(__file__).resolve().parents[3] / 'src/backend-clients.js')
node_script = r'''
const fs=require('fs'),cp=require('child_process'),m=require(process.argv[1]);
const port=Number(process.argv[2]),client=m.joinBackend(port); let backend;
if(process.argv[3]==='owner') {
 const file=process.argv[5],fd=fs.openSync(file,'w',0o600);
 backend=cp.spawn(process.argv[4],['--profile','web','--no-open','--port',String(port)],{stdio:['ignore',fd,fd],detached:true});
 backend.unref(); backend.once('spawn',()=>m.rememberBackend(port,backend.pid));
 const timer=setInterval(()=>{const match=/dsh web: (http[^\s]+)/.exec(fs.readFileSync(file,'utf8'));if(match){clearInterval(timer);console.log(match[1]);}},50);
} else console.log('joined');
process.stdin.once('data',()=>{const last=client.release(),pid=client.target();if(last&&pid)process.kill(-pid,'SIGTERM');process.exit(0);});
'''

for node_owns in [False, True]:
    with tempfile.TemporaryDirectory(prefix='dsh-shared-clients-') as directory:
        root = Path(directory)
        profile = root / 'dsh/profiles/web'
        profile.mkdir(parents=True)
        modules = next(p for p in Path(dsh).parents if p.name == 'node_modules')
        (profile / 'node_modules').symlink_to(modules, target_is_directory=True)
        (profile / 'package.json').write_text(json.dumps({'name': 'isolated-shared-client-test', 'private': True,
            'dependencies': {}, 'dsh': {'profile': {'bundles': ['@deepseek-ai/dsh-base', '@deepseek-ai/dsh-web-app']}}}))
        (profile / 'cordis.yml').write_text('[]\n')
        (profile / 'cordis.patch.yml').write_text(json.dumps([
            {'id': 'session-title-llm', 'disabled': True}, {'id': 'goal-round-driver', 'disabled': True}]))
        env = {k: v for k, v in os.environ.items() if k not in ('DSH_TUI_URL', 'NO_COLOR', 'GH_TOKEN', 'GITHUB_TOKEN')}
        env.update(HOME=str(root), DSH_HOME=str(root / 'dsh'), XDG_CONFIG_HOME=str(root / 'config'),
                   XDG_DATA_HOME=str(root / 'data'), TERM='xterm-256color', COLORTERM='truecolor')
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
        def port_open():
            with socket.socket() as sock:
                sock.settimeout(.2)
                return sock.connect_ex(('127.0.0.1', port)) == 0
        peer, app = None, None
        backend_identity = None
        def start_peer(owner):
            process = subprocess.Popen(['node', '-e', node_script, module, str(port),
                'owner' if owner else 'attached', dsh, str(root / 'backend.log')],
                cwd=root, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            lines = []
            threading.Thread(target=lambda: lines.append(process.stdout.readline().strip()), daemon=True).start()
            wait_for(lambda: lines or process.poll() is not None, timeout=40)
            assert lines and lines[0], process.stderr.read() if process.poll() is not None else 'Missing frontend ready signal'
            return process, lines[0]
        def close_peer():
            peer.stdin.write('close\n')
            peer.stdin.flush()
            assert peer.wait(timeout=10) == 0
        try:
            if node_owns:
                peer, login = start_peer(True)
                app = Terminal(binary, ['--url', login, '--new'], env)
            else:
                app = Terminal(binary, ['--start', '--market', 'skip', '--dsh', dsh, '--cwd', str(root), '--port', str(port)], env)
            app.wait('Введите сообщение.', timeout=60)
            backend_identity = json.loads((root / f'data/dsh-launcher/backends/{port}/managed.json').read_text())
            if node_owns:
                close_peer()
                assert port_open(), 'Closing GUI participant stopped actual dsh used by TUI'
                app.command('/sessions')
                app.wait('Сессии dsh')
                app.send('\x1b')
                app.wait('Введите сообщение.')
                app.send('\x04')
                app.exited()
            else:
                peer, _ = start_peer(False)
                app.send('\x04')
                app.exited()
                assert port_open(), 'Closing TUI owner stopped actual dsh used by GUI'
                # Give the backend a chance to write after its original frontend exits.
                time.sleep(.3)
                assert port_open(), 'Backend lost its stdout after TUI owner exited'
                close_peer()
            wait_for(lambda: not port_open(), timeout=10)
            print(f'Actual dsh shared clients: {"GUI" if node_owns else "TUI"} owner exits first; peer survives; last client stops backend OK')
        except BaseException:
            if app:
                print(app.text)
            raise
        finally:
            if app:
                app.close(root / 'terminal.log')
            if peer and peer.poll() is None:
                peer.kill()
                peer.wait()
            if backend_identity:
                try:
                    stat = Path(f"/proc/{backend_identity['pid']}/stat").read_text().rsplit(')', 1)[1].split()
                    if stat[19] == backend_identity['start']:
                        os.killpg(backend_identity['group'], signal.SIGKILL)
                except (ProcessLookupError, FileNotFoundError):
                    pass
