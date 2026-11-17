"""Capture installed OpenCode with a private server and an empty temporary profile.

No prompt is submitted and no personal environment, provider, or config is loaded.
"""
import argparse
import json
import os
from pathlib import Path
import tempfile
import shutil
import time

from terminal import Terminal

parser = argparse.ArgumentParser()
parser.add_argument('--binary', default=shutil.which('opencode'))
parser.add_argument('--output', default='build/opencode-reference-v230')
args = parser.parse_args()
output = Path(args.output).resolve()
output.mkdir(parents=True, exist_ok=True)
if not args.binary:
    parser.error('OpenCode was not found on PATH; supply --binary')
binary = str(Path(args.binary).resolve())

def settle(app, seconds=2):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        app.pump()
        time.sleep(0.05)

def capture(app, name):
    for columns, rows in [(160, 40), (100, 30), (80, 24)]:
        app.resize(columns, rows)
        settle(app, 1)
        cells = [{'x': x, 'y': y, 'text': (cell := app.screen.buffer[y][x]).data,
                  'fg': cell.fg, 'bg': cell.bg, 'bold': cell.bold, 'reverse': cell.reverse}
                 for y in range(rows) for x in range(columns)]
        stem = output / f'{name}-{columns}x{rows}'
        stem.with_suffix('.json').write_text(json.dumps({'source': 'Actual installed OpenCode, isolated temporary profile',
            'columns': columns, 'rows': rows, 'cells': cells}, ensure_ascii=False))
        stem.with_suffix('.txt').write_text(app.text)
    print(name + ':\n' + app.text, flush=True)

with tempfile.TemporaryDirectory(prefix='opencode-reference-') as directory:
    os.chdir(directory)
    env = {'PATH': str(Path(binary).parent) + os.pathsep + os.defpath,
           'HOME': directory, 'XDG_CONFIG_HOME': directory + '/config', 'XDG_DATA_HOME': directory + '/data',
           'XDG_STATE_HOME': directory + '/state', 'XDG_CACHE_HOME': directory + '/cache',
           'TMPDIR': directory, 'TERM': 'xterm-256color', 'COLORTERM': 'truecolor',
           'LANG': 'C.UTF-8', 'OPENCODE_DISABLE_MODELS_FETCH': 'true', 'OPENCODE_DISABLE_AUTOUPDATE': 'true',
           'OPENCODE_CONFIG_CONTENT': json.dumps({'enabled_providers': [], 'autoupdate': False})}
    app = Terminal(binary, ['--standalone'], env, columns=160, rows=40)
    try:
        settle(app, 12)
        capture(app, 'home')
        app.send('\x10')
        settle(app)
        capture(app, 'palette-ctrlp')
        app.send('\x1b')
        settle(app)
        app.send('\x18l')
        settle(app)
        capture(app, 'sessions')
        app.send('\x03')
    finally:
        app.close(output / 'reference.ansi')
