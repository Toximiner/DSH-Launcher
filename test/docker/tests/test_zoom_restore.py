"""The window zoom between runs: the saved zoom-level.json is applied (log),
Ctrl+= zooms in and writes the new value. The keys are sent as real X
events (xdotool), as in test_keyboard: CDP delivery is unpredictable."""
import os
import subprocess

from helpers.env import CONF, FAKE, NOUPD
from helpers.procs import as_tester, wait_for

ZOOM = f'{CONF}/zoom-level.json'
X = {'DISPLAY': ':99'}


def zoom_file():
    _, out = as_tester(f'cat {ZOOM} 2>/dev/null')
    return out.strip()


def key(name):
    win = subprocess.run(['xdotool', 'search', '--onlyvisible', '--name', 'DeepSeek Harness'],
                         capture_output=True, text=True, env={**X, 'PATH': '/usr/bin:/bin'}).stdout.split()
    assert win, 'launcher window for X keys not found'
    subprocess.run(['xdotool', 'windowfocus', '--sync', win[0]], env=X)
    subprocess.run(['xdotool', 'key', name], env=X)


def test_zoom_restore(start, gui):
    as_tester(f'mkdir -p {CONF} && echo \'{{"zoomLevel":2}}\' > {ZOOM}')

    launcher = start(FAKE, NOUPD)
    assert launcher.wait_log('восстановлен масштаб: 2', 60), launcher.log()[-2000:]
    gui.wait()

    # Ctrl+= — zoom in: the value is written to zoom-level.json (after 400 ms).
    key('ctrl+equal')
    assert wait_for(lambda: '"zoomLevel":2.5' in zoom_file(), 10), f'new value: {zoom_file()!r}'

    gui.close()
    assert launcher.wait_exit(15) is not None, 'launcher exited'
    assert os.path.exists(ZOOM), 'zoom file in place'
    os.remove(ZOOM)
