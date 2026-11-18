"""Prepare actual dsh, marketplace and native Go TUI for a private live server."""
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
DATA = Path(os.environ.get('DSH_LIVE_DATA', str(ROOT / '.live-data'))).resolve()
DATA.mkdir(parents=True, exist_ok=True)
RUNTIME = DATA / 'runtime'
RUNTIME.mkdir(exist_ok=True)
BIN = DATA / 'bin'
BIN.mkdir(exist_ok=True)
ENV = {key: value for key, value in os.environ.items()
       if key not in ('GH_TOKEN', 'GITHUB_TOKEN', 'CODESPACES_GITHUB_TOKEN')}
ENV['DSH_HOME'] = str(DATA / 'dsh')
ENV['PATH'] = str(RUNTIME / 'node_modules/.bin') + os.pathsep + ENV['PATH']


def run(*args, cwd=ROOT):
    subprocess.run(args, cwd=cwd, env=ENV, check=True)


print('Installing actual dsh and terminal dependencies…', flush=True)
run('npm', 'install', '--prefix', str(RUNTIME), '--ignore-scripts', '--no-audit', '--no-fund',
    '@deepseek-ai/dsh@0.2.2-rc.1', 'pnpm@10', '@xterm/xterm@5.5.0', '@xterm/addon-fit@0.10.0')
manifest = DATA / 'dsh/profiles/web/package.json'
installed = json.loads(manifest.read_text()).get('dependencies', {}) if manifest.exists() else {}
if 'dshmarket' not in installed:
    print('Installing marketplace into the live dsh profile…', flush=True)
    run(str(RUNTIME / 'node_modules/.bin/dsh'), 'plugin', '--profile', 'web', 'add', 'dshmarket@1.73.17')
print('Building native Go TUI…', flush=True)
subprocess.run([os.environ.get('GO', 'go'), 'build', '-trimpath', '-ldflags=-s -w',
                '-o', str(BIN / 'dsh-launcher-tui-go'), '.'], cwd=ROOT / 'apps/tui-go',
               env={**ENV, 'CGO_ENABLED': '0'}, check=True)
ttyd = BIN / 'ttyd'
if not ttyd.exists():
    architecture = {'x86_64': 'x86_64', 'aarch64': 'aarch64'}[platform.machine()]
    meta = json.load(urllib.request.urlopen('https://api.github.com/repos/tsl0922/ttyd/releases/tags/1.7.7', timeout=30))
    asset = next(item for item in meta['assets'] if item['name'] == 'ttyd.' + architecture)
    content = urllib.request.urlopen(asset['browser_download_url'], timeout=60).read()
    if (asset.get('digest') or '').startswith('sha256:'):
        assert hashlib.sha256(content).hexdigest() == asset['digest'].split(':', 1)[1]
    ttyd.write_bytes(content)
    ttyd.chmod(0o700)
page = (ROOT / 'tools/live/index.html').read_text()
for marker, path in {
    '__XTERM_CSS__': RUNTIME / 'node_modules/@xterm/xterm/css/xterm.css',
    '__XTERM_JS__': RUNTIME / 'node_modules/@xterm/xterm/lib/xterm.js',
    '__FIT_JS__': RUNTIME / 'node_modules/@xterm/addon-fit/lib/addon-fit.js',
    '__LIVE_JS__': ROOT / 'tools/live/terminal.js',
}.items():
    page = page.replace(marker, path.read_text())
(DATA / 'index.html').write_text(page)
print('Live installation complete.', flush=True)
