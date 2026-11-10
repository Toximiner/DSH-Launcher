"""Build the shared Go TUI model for a static browser preview."""
import argparse
import base64
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import urllib.request

parser = argparse.ArgumentParser()
parser.add_argument('--go', default='go', help='Path to the Go compiler')
args = parser.parse_args()
source = Path(__file__).resolve().parent
app = source.parent
output = app.parents[1] / 'build/tui-go-web'
output.mkdir(parents=True, exist_ok=True)


def run(*command, **kwargs):
    return subprocess.check_output(command, cwd=app, text=True, **kwargs).strip()


run(args.go, 'mod', 'download')
tea_dir = Path(run(args.go, 'list', '-m', '-f', '{{.Dir}}', 'github.com/charmbracelet/bubbletea'))
patched = output / 'bubbletea'
if not patched.exists():
    shutil.copytree(tea_dir, patched)
patched.chmod(patched.stat().st_mode | 0o200)
shutil.copy2(source / 'tty_js.go.txt', patched / 'tty_js.go')
modfile = output / 'browser.mod'
shutil.copy2(app / 'go.mod', modfile)
shutil.copy2(app / 'go.sum', output / 'browser.sum')
run(args.go, 'mod', 'edit', '-modfile=' + str(modfile),
    '-replace=github.com/atotto/clipboard=' + str(source / 'clipboard'),
    '-replace=github.com/charmbracelet/bubbletea=' + str(patched))
run(args.go, 'build', '-trimpath', '-modfile=' + str(modfile), '-o', str(output / 'tui.wasm'), '.',
    env={**os.environ, 'GOOS': 'js', 'GOARCH': 'wasm'})
(output / 'tui.wasm.gz').write_bytes(gzip.compress((output / 'tui.wasm').read_bytes(), compresslevel=9, mtime=0))
goroot = Path(run(args.go, 'env', 'GOROOT'))
shutil.copy2(goroot / 'lib/wasm/wasm_exec.js', output / 'wasm_exec.js')
for name in ('index.html', 'preview.js', 'snapshot.json', 'market-demo.json'):
    shutil.copy2(source / name, output / name)
licenses = [tea_dir.joinpath('LICENSE').read_text()]
for package, version, files in [
    ('@xterm/xterm', '5.5.0', {'package/lib/xterm.js': 'xterm.js', 'package/css/xterm.css': 'xterm.css'}),
    ('@xterm/addon-fit', '0.10.0', {'package/lib/addon-fit.js': 'addon-fit.js'}),
]:
    with urllib.request.urlopen('https://registry.npmjs.org/' + package + '/' + version, timeout=30) as response:
        meta = json.load(response)
    with urllib.request.urlopen(meta['dist']['tarball'], timeout=30) as response:
        data = response.read()
    algorithm, digest = meta['dist']['integrity'].split('-', 1)
    if base64.b64encode(hashlib.new(algorithm, data).digest()).decode() != digest:
        raise RuntimeError('Package integrity check failed: ' + package)
    with tarfile.open(fileobj=io.BytesIO(data), mode='r:gz') as archive:
        for name, target in files.items():
            (output / target).write_bytes(archive.extractfile(name).read())
        for member in archive.getmembers():
            if member.name.lower() in ('package/license', 'package/license.md'):
                licenses.append(package + '\n' + archive.extractfile(member).read().decode())
(output / 'LICENSES.txt').write_text('\n\n'.join(licenses))
public = ('index.html', 'preview.js', 'snapshot.json', 'market-demo.json', 'tui.wasm.gz', 'wasm_exec.js',
          'xterm.js', 'xterm.css', 'addon-fit.js', 'LICENSES.txt')
(output / 'SHA256SUMS').write_text(''.join(
    hashlib.sha256((output / name).read_bytes()).hexdigest() + '  ' + name + '\n' for name in public))
print('Static preview built:', output)
print('Publish only the files listed in SHA256SUMS, plus SHA256SUMS itself.')
