"""Масштаб окна между запусками: сохранённый zoom-level.json применяется
(лог), Ctrl+= увеличивает и записывает новое значение. Клавиши — реальными
X-событиями (xdotool), как в test_keyboard: CDP-доставка непредсказуема."""
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
    assert win, 'окно лаунчера для X-клавиш не найдено'
    subprocess.run(['xdotool', 'windowfocus', '--sync', win[0]], env=X)
    subprocess.run(['xdotool', 'key', name], env=X)


def test_zoom_restore(start, gui):
    as_tester(f'mkdir -p {CONF} && echo \'{{"zoomLevel":2}}\' > {ZOOM}')

    launcher = start(FAKE, NOUPD)
    assert launcher.wait_log('восстановлен масштаб: 2', 60), launcher.log()[-2000:]
    gui.wait()

    # Ctrl+= — увеличение: значение записывается в zoom-level.json (через 400 мс).
    key('ctrl+equal')
    assert wait_for(lambda: '"zoomLevel":2.5' in zoom_file(), 10), f'новое значение: {zoom_file()!r}'

    gui.close()
    assert launcher.wait_exit(15) is not None, 'лаунчер завершился'
    assert os.path.exists(ZOOM), 'файл масштаба на месте'
    os.remove(ZOOM)
