"""Protecting the window from foreign pages: dshlauncher://… commands without the launch key
are not executed, foreign sites open in the system browser, not the window, and browser
permissions (geolocation, microphone …) are denied."""
import json
import os

from helpers.env import CONF, FAKE, NOUPD
from helpers.procs import wait_for


def test_foreign_route(start, gui):
    """A page in the window (dsh GUI, a plugin, a site) navigates to a launcher
    route without the key — the command is not executed, the page stays put;
    with the key (as from the launcher's own pages) — it is executed."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    gui.mark()

    gui.go('dshlauncher://settings/set/?k=port&v=3999', keyed=False)
    assert launcher.wait_log('запрос без ключа запуска — отклонён: /settings/set/', 10), launcher.log()[-1500:]
    assert gui.not_reloaded(), 'the page stayed in place'
    settings = f'{CONF}/settings.json'
    assert not os.path.exists(settings) or 'port' not in json.load(open(settings)), 'the setting did not change'
    assert 'настройки: port' not in launcher.log()

    gui.go('dshlauncher://settings/set/?k=port&v=3999')
    assert launcher.wait_log('настройки: port = 3999', 10), 'with the key — saved'
    launcher.term()


def test_external_navigation(start, gui):
    """A link to a foreign site in the window (without target=_blank) — to the
    system browser; the window stays in the dsh GUI."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    gui.mark()

    gui.eval("location.href = 'https://example.com/?token=secret123'; 1")
    assert launcher.wait_log('переход открыт в браузере: https://example.com/?token=***', 10), launcher.log()[-1500:]
    assert 'secret123' not in launcher.log(), 'the token is masked in the log'
    assert gui.not_reloaded() and gui.eval('location.origin') == 'http://127.0.0.1:3080', 'the window stayed in the dsh GUI'
    launcher.term()


def test_permissions(start, gui):
    """Geolocation (like the microphone, camera …) is not granted to a page in
    the window — denied, plus a line in the log."""
    launcher = start(FAKE, NOUPD)
    gui.wait()

    gui.eval("window.__geo = 'pending'; navigator.geolocation.getCurrentPosition("
             "() => { window.__geo = 'granted'; }, (e) => { window.__geo = 'error ' + e.code; }); 1")
    assert wait_for(lambda: gui.eval('window.__geo') != 'pending', 10, 0.2), 'the permission request finished'
    assert gui.eval('window.__geo') == 'error 1', f'PERMISSION_DENIED, not {gui.eval("window.__geo")}'
    assert launcher.wait_log('разрешение «geolocation» для http://127.0.0.1:3080 — отказ', 5), launcher.log()[-1500:]
    launcher.term()
