"""Окно «Файл → Настройки…»: поля сохраняются в settings.json, применяются
после перезапуска лаунчера; переменные окружения важнее (поле неактивно);
неверные значения не сохраняются."""
import json
import os

from helpers.env import CONF, FAKE, NOUPD
from helpers.procs import fakedsh_events, wait_exit, wait_for, wait_port

SETTINGS = f'{CONF}/settings.json'


def settings_file():
    try:
        return json.load(open(SETTINGS))
    except (OSError, ValueError):
        return {}


def test_settings_apply(start, gui):
    """Профиль и рабочая папка → settings.json и плашка «нужен перезапуск»;
    «Перезапустить лаунчер» — новый экземпляр запускает dsh с ними."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    settings = gui.open_settings()
    assert not settings.restart_banner(), 'сначала — без плашки перезапуска'

    settings.set('profile', 'second')
    settings.wait_note('Saved')
    settings.set('cwd', '/tmp')
    settings.wait_note('Saved')

    assert settings_file() == {'profile': 'second', 'cwd': '/tmp'}, settings_file()
    assert wait_for(settings.restart_banner, 5, 0.2), 'плашка «изменения — после перезапуска»'

    settings.relaunch()

    assert launcher.wait_exit(20) is not None, 'прежний экземпляр завершился'
    new = launcher.successor()
    assert new, 'лаунчер перезапустился'
    # журнал фейкового dsh: «start pgid=… cwd=<папка> args=<аргументы>»
    assert wait_for(lambda: 'cwd=/tmp args=--profile second --no-open' in fakedsh_events(), 60), \
        f'новый dsh — в папке /tmp и с профилем second:\n{fakedsh_events()}'
    # GUI нового экземпляра через CDP не проверить: его запускает старый, и порт
    # отладки 9222 остаётся занят унаследованным сокетом (только в тестах —
    # у пользователей порта отладки нет). Проверяем по результату: dsh отвечает.
    assert wait_port(30), 'новый экземпляр поднял dsh'
    os.kill(new, 15)
    assert wait_exit(new, 15) is not None, 'новый экземпляр штатно завершился'


def test_settings_env_locked(start, gui):
    """Рабочая папка задана переменной DSH_CWD — в окне она показана, но
    менять её нельзя (подпись «set by DSH_CWD»)."""
    launcher = start(FAKE, NOUPD, DSH_CWD='/opt')
    gui.wait()
    settings = gui.open_settings()

    settings.wait_text(r'Working folder /opt set by DSH_CWD', 5)
    assert 'Choose' not in settings.text().split('Working folder', 1)[1].split('Port', 1)[0], 'нет кнопки «Выбрать…»'
    assert settings.disabled('checkUpdates'), 'проверка обновлений задана DSH_LAUNCHER_NO_UPDATE_CHECK — поле неактивно'
    assert settings.disabled('pollUpdates'), 'и проверка раз в 12 часов — тоже'
    settings.close()
    launcher.term()


def test_settings_validation(start, gui):
    """Неверный порт или профиль не сохраняются — подсказка с ошибкой."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    settings = gui.open_settings()

    settings.set('port', '80')
    settings.wait_note('The port must be a number from 1024 to 65535')
    settings.set('profile', 'a%20b')
    settings.wait_note('The profile may contain Latin letters')

    assert settings_file() == {}, f'ничего не сохранено: {settings_file()}'
    assert not settings.restart_banner(), 'без плашки перезапуска'
    settings.close()
    launcher.term()
