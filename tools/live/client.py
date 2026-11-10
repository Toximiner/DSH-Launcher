#!/usr/bin/env python3
"""ttyd starts only the actual native TUI, never a shell."""
import os
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DATA = Path(os.environ.get('DSH_LIVE_DATA', str(ROOT / '.live-data'))).resolve()
login = (DATA / 'login-url').read_text().strip()
env = {key: value for key, value in os.environ.items()
       if key not in ('GH_TOKEN', 'GITHUB_TOKEN', 'CODESPACES_GITHUB_TOKEN')}
env.update(DSH_TUI_URL=login, COLORTERM='truecolor', TERM='xterm-256color')
env.pop('NO_COLOR', None)
os.chdir(DATA / 'project')
executable = str(DATA / 'bin/dsh-launcher-tui-go')
os.execve(executable, [executable, '--new'], env)
