"""Окна меню «Справка»: «О программе», «Проверить обновления», «Журнал dsh».

Окна — модальные поверх главного; GUI dsh под ними затемняется, но не
перезагружается. Проверка обновлений при старте включена, «последние» версии
подменены на старые — без вопросов об обновлении.
"""
import json
import os

import pytest

from helpers.env import CONF, FAKE, INSTALLED, LATEST, LATEST_RE, NO_PROMPTS, NOUPD, STATE
from helpers.procs import as_tester, wait_for, wait_port


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


def test_update_available(start, gui, prompt):
    """«Проверить обновления», когда новая версия есть: статус «доступна» и
    «Что нового» в окне. (Вопрос при старте — «Не сейчас».)"""
    launcher = start(FAKE)
    prompt.wait('Update DSH Launcher')
    prompt.later()
    gui.wait()

    check = gui.open_update_check()

    assert f'version {LATEST} is available' in check.status('DSH Launcher'), check.rows()
    check.wait_text(rf'What.s new: {LATEST_RE} ', 5)
    check.close()
    launcher.term()


def test_dialog_keys(start, gui):
    """Esc закрывает окно-диалог; Ctrl+= / Ctrl+0 — масштаб (сохраняется)."""
    zoom = f'{CONF}/zoom-level.json'
    launcher = start(FAKE, NOUPD)
    gui.wait()

    about = gui.open_about()
    about.press('Escape')
    assert wait_for(lambda: not about.is_open(), 5), 'Esc закрыл «О программе»'

    gui.press('Ctrl+Equal')
    assert wait_for(lambda: os.path.exists(zoom) and json.load(open(zoom))['zoomLevel'] == 0.5, 5), 'Ctrl+= — масштаб +0.5 сохранён'
    gui.press('Ctrl+Digit0')
    assert wait_for(lambda: json.load(open(zoom))['zoomLevel'] == 0, 5), 'Ctrl+0 — масштаб сброшен'
    assert launcher.alive, 'лаунчер жив'
    launcher.term()


def test_gpu_reenable(start, gui):
    """GPU отключён после сбоев (маркер) — в «О программе» кнопка «Включить
    GPU снова» удаляет маркер; GPU — со следующего запуска."""
    marker = f'{STATE}/gpu-disabled.marker'
    as_tester(f'mkdir -p {STATE} && echo "GPU process crashed x2 (test)" > {marker}')
    launcher = start(FAKE, NOUPD)
    gui.wait()

    about = gui.open_about()
    assert about.value('GPU acceleration').startswith('off after GPU process crashes'), about.rows()
    about.reenable_gpu()

    assert wait_for(lambda: not os.path.exists(marker), 5), 'маркер сбоев удалён'
    about.wait_text('off until the launcher restarts', 5)
    about.close()
    launcher.term()
