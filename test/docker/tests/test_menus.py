"""Окна меню «Справка»: «О программе», «Проверить обновления», «Журнал dsh».

Окна — модальные поверх главного; GUI dsh под ними затемняется, но не
перезагружается. Проверка обновлений при старте включена, «последние» версии
подменены на старые — без вопросов об обновлении.
"""
import pytest

from helpers.env import FAKE, INSTALLED, NO_PROMPTS, NOUPD
from helpers.procs import wait_for


@pytest.fixture
def launched(start, gui):
    """Лаунчер с GUI dsh после проверки обновлений при старте."""
    launcher = start(FAKE, NO_PROMPTS)
    gui.wait()
    assert launcher.wait_log('последний @deepseek-ai/dsh', 30), 'проверка обновлений при старте прошла (npm)'
    gui.mark()
    yield launcher
    launcher.term()


def test_menu_about(launched, gui):
    """«О программе» — версии, GPU, логи, «Скопировать»; три открытия подряд
    (1.3.0 падал на «О программе» после проверки обновлений)."""
    for attempt in (1, 2, 3):
        about = gui.open_about()
        assert about.value('DSH Launcher') == INSTALLED, f'#{attempt}: версия лаунчера'
        assert about.value('dsh backend') == '0.2.0-rc.2', f'#{attempt}: версия dsh'
        assert gui.not_reloaded(), f'#{attempt}: GUI dsh под окном не перезагружался'
        assert gui.dimmed(), f'#{attempt}: GUI dsh под окном затемнён'
        if attempt == 1:
            assert about.value('GPU acceleration').startswith('off (no /dev/dri/renderD'), 'GPU (в контейнере выкл.)'
            assert about.value('logs') == '/home/tester/.local/state/dsh-launcher', 'путь к логам'
            about.copy_for_report()

        about.close()

        assert wait_for(lambda: not gui.dimmed(), 5), f'#{attempt}: затемнение снято'
        assert launched.alive, f'#{attempt}: лаунчер жив'


def test_update_check_dialog(launched, gui):
    """«Проверить обновления» — оба компонента актуальны."""
    check = gui.open_update_check()

    assert 'up to date' in check.status('DSH Launcher'), check.rows()
    assert 'up to date' in check.status('dsh backend'), check.rows()
    assert gui.not_reloaded(), 'GUI dsh под окном не перезагружался'
    check.close()
    assert launched.alive, 'лаунчер жив'


def test_dsh_log(start, gui):
    """«Журнал dsh» — хвост dsh.log; токен входа скрыт."""
    launcher = start(FAKE, NOUPD)
    gui.wait()

    log = gui.open_dsh_log()

    text = log.log_text()
    assert 'dsh web: http://127.0.0.1:3080/?token=***' in text, text[-500:]
    assert 'token=abc' not in text, 'настоящего токена в окне нет'
    log.close()
    assert launcher.alive, 'лаунчер жив'
    launcher.term()
