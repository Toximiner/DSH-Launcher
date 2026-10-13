"""Защита окна от чужих страниц: команды dshlauncher://… без ключа запуска не
выполняются, чужие сайты открываются в системном браузере, а не в окне,
разрешения браузера (геолокация, микрофон …) — отказ."""
import json
import os

from helpers.env import CONF, FAKE, NOUPD
from helpers.procs import wait_for


def test_foreign_route(start, gui):
    """Страница в окне (GUI dsh, плагин, сайт) переходит на маршрут лаунчера
    без ключа — команда не выполняется, страница остаётся на месте; с ключом
    (как со страниц самого лаунчера) — выполняется."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    gui.mark()

    gui.go('dshlauncher://settings/set/?k=port&v=3999', keyed=False)
    assert launcher.wait_log('запрос без ключа запуска — отклонён: /settings/set/', 10), launcher.log()[-1500:]
    assert gui.not_reloaded(), 'страница осталась на месте'
    settings = f'{CONF}/settings.json'
    assert not os.path.exists(settings) or 'port' not in json.load(open(settings)), 'настройка не изменилась'
    assert 'настройки: port' not in launcher.log()

    gui.go('dshlauncher://settings/set/?k=port&v=3999')
    assert launcher.wait_log('настройки: port = 3999', 10), 'с ключом — сохранено'
    launcher.term()


def test_external_navigation(start, gui):
    """Ссылка на чужой сайт в окне (без target=_blank) — в системный браузер;
    окно остаётся в GUI dsh."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    gui.mark()

    gui.eval("location.href = 'https://example.com/?token=secret123'; 1")
    assert launcher.wait_log('переход открыт в браузере: https://example.com/?token=***', 10), launcher.log()[-1500:]
    assert 'secret123' not in launcher.log(), 'токен в журнале скрыт'
    assert gui.not_reloaded() and gui.eval('location.origin') == 'http://127.0.0.1:3080', 'окно осталось в GUI dsh'
    launcher.term()


def test_permissions(start, gui):
    """Геолокацию (как и микрофон, камеру …) странице в окне не дают — отказ и
    строка в журнале."""
    launcher = start(FAKE, NOUPD)
    gui.wait()

    gui.eval("window.__geo = 'pending'; navigator.geolocation.getCurrentPosition("
             "() => { window.__geo = 'granted'; }, (e) => { window.__geo = 'error ' + e.code; }); 1")
    assert wait_for(lambda: gui.eval('window.__geo') != 'pending', 10, 0.2), 'запрос разрешения завершился'
    assert gui.eval('window.__geo') == 'error 1', f'PERMISSION_DENIED, а не {gui.eval("window.__geo")}'
    assert launcher.wait_log('разрешение «geolocation» для http://127.0.0.1:3080 — отказ', 5), launcher.log()[-1500:]
    launcher.term()
