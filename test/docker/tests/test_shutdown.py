"""Завершение работы: закрытие окна, сигналы лаунчеру, «усыновлённый» dsh.

Во всех случаях лаунчер должен остановить запущенный им dsh вместе с
потомками (у фейкового dsh — sleep 7777 в той же группе процессов).
"""
import time

import pytest

from helpers.env import FAKE, NOUPD
from helpers.procs import dsh_tree_alive, dsh_tree_gone, fakedsh_events, pgrep, sh, wait_port


def test_close(start, gui):
    """Закрытие окна — штатный выход: dsh получает SIGTERM."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    assert dsh_tree_alive(), 'дерево fakedsh живо (dsh + sleep)'

    gui.close()

    assert launcher.wait_exit(15) is not None, 'лаунчер завершился после закрытия окна'
    assert dsh_tree_gone(), 'дерево dsh остановлено'
    assert 'SIGTERM' in fakedsh_events(), 'dsh получил SIGTERM (штатно)'


def test_sigterm(start, gui):
    """SIGTERM лаунчеру (pkill, завершение сессии) — тоже штатный выход."""
    launcher = start(FAKE, NOUPD)
    gui.wait()

    assert launcher.term(15) is not None, 'лаунчер вышел по SIGTERM за 15 с'

    assert dsh_tree_gone(), 'дерево dsh остановлено'
    assert 'SIGTERM' in fakedsh_events(), 'dsh получил SIGTERM, а не сразу SIGKILL'
    assert 'SIGTERM — ' in launcher.log(), 'в логе лаунчера — обработчик сигнала'


def test_sigterm_ignored(start, gui):
    """dsh игнорирует SIGTERM — лаунчер через ~4 с добивает его SIGKILL."""
    launcher = start(FAKE, NOUPD, FAKE_IGNORE_TERM='1')
    gui.wait()

    assert launcher.term(15) is not None, 'лаунчер вышел за 15 с'

    assert dsh_tree_gone(), 'dsh, игнорирующий SIGTERM, добит SIGKILL'


def test_sigterm_double(start, gui):
    """Повторный SIGTERM — не ждать, выйти сразу (SIGKILL dsh)."""
    launcher = start(FAKE, NOUPD, FAKE_IGNORE_TERM='1')
    gui.wait()

    sh(f'kill -TERM {launcher.pid}')
    time.sleep(0.5)
    sh(f'kill -TERM {launcher.pid}')

    assert launcher.wait_exit(5) is not None, 'после повторного SIGTERM — выход сразу'
    time.sleep(0.5)
    assert dsh_tree_gone(), 'дерево dsh остановлено'


@pytest.mark.parametrize('detached', [
    pytest.param(False, id='adopt_samegroup'),   # новый dsh — в той же группе процессов
    pytest.param(True, id='adopt_detached'),     # новый dsh — в своей группе
])
def test_adopt(start, gui, detached):
    """dsh перезапустился сам (как после обновления плагина): лаунчер
    «усыновляет» новый процесс и при закрытии останавливает и его, и
    потомков старого."""
    extra = {'FAKE_REBORN_DETACHED': '1'} if detached else {}
    launcher = start(FAKE, NOUPD, FAKE_SELF_RESTART_AFTER='5', **extra)
    gui.wait()

    assert launcher.wait_log('перезапустился сам', 30), 'лаунчер усыновил перезапущенный dsh'
    assert wait_port(10), 'перерождённый dsh слушает порт'
    assert pgrep('^sleep 7777'), 'в старой группе остался sleep 7777'

    gui.close()

    assert launcher.wait_exit(15) is not None, 'лаунчер завершился после закрытия окна'
    assert dsh_tree_gone(), 'усыновлённый dsh и потомки старой группы остановлены'
