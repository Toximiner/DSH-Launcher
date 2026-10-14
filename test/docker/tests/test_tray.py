"""Трей и работа в фоне: крестик прячет окно (dsh работает дальше), повторный
запуск и выключение трея возвращают окно; «агент закончил» — точка на
значке трея и системное уведомление (одно; окно открыли — закрыто)."""
import json
import os
import subprocess

import pytest

from helpers.env import FAKE, NOUPD, T
from helpers.procs import as_tester, wait_for, wait_port


def enable_tray(gui, launcher):
    settings = gui.open_settings()
    settings.set('closeToTray', 'true')
    settings.wait_note('Saved')
    assert launcher.wait_log('трей включён', 5)
    settings.close()


def test_close_to_tray(start, gui):
    launcher = start(FAKE, NOUPD)
    gui.wait()
    enable_tray(gui, launcher)
    gui.mark()

    gui.go('dshlauncher://window/close/')  # то же, что крестик
    assert launcher.wait_log('окно спрятано в трей', 5)
    assert launcher.alive and wait_port(5), 'лаунчер и dsh работают дальше'

    # повторный запуск лаунчера возвращает спрятанное окно
    as_tester('DISPLAY=:99 HOME=/home/tester /usr/bin/dsh-launcher >/dev/null 2>&1')
    assert launcher.wait_log('окно показано из трея', 15)
    assert gui.not_reloaded(), 'GUI dsh не перезагружался'
    launcher.term()


def test_tray_off_shows_window(start, gui):
    """Трей выключили, пока окно спрятано, — окно показывается (без значка его
    было бы не вернуть)."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    enable_tray(gui, launcher)
    gui.go('dshlauncher://window/close/')
    assert launcher.wait_log('окно спрятано в трей', 5)

    gui.go('dshlauncher://settings/set/?k=closeToTray&v=false')
    assert launcher.wait_log('трей выключен', 5)
    assert launcher.wait_log('окно показано из трея', 5)
    launcher.term()


def test_agent_finished(start, gui):
    """Окно в трее, агент закончил ответ (индикатор [data-chat-running] в чате
    исчез) — системное уведомление, точка на значке трея и (включён) звук; агент
    ждёт подтверждения команды или ответа на вопрос — тот же сигнал."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    enable_tray(gui, launcher)
    gui.go('dshlauncher://settings/set/?k=attentionSound&v=true')
    assert launcher.wait_log('звук: агент закончил ответ', 5), 'включили звук — сразу пробный сигнал'
    gui.go('dshlauncher://window/close/')
    assert launcher.wait_log('окно спрятано в трей', 5)

    gui.eval("document.body.insertAdjacentHTML('beforeend', '<div id=\"run\" data-chat-running></div>')")
    assert launcher.wait_log('агент отвечает', 10)
    gui.eval("document.getElementById('run').remove()")
    assert launcher.wait_log('агент закончил ответ — окно в трее', 10)
    assert launcher.wait_log('значок трея: агент закончил ответ', 5)
    assert 'уведомление: агент закончил ответ' in launcher.log(), 'системное уведомление'
    assert launcher.log().count('звук: агент закончил ответ') == 2, 'и звук'

    # агент остановился и ждёт подтверждения команды — тот же сигнал
    gui.eval("document.body.insertAdjacentHTML('beforeend', '<div id=\"ask\" data-approval-key=\"k1\"></div>')")
    assert launcher.wait_log('агент ждёт: подтверждение команды — окно в трее', 10)
    assert launcher.wait_log('уведомление: агент ждёт: подтверждение команды', 5)
    # ответили (панель занята ответом) — повторно не сообщает
    gui.eval("document.getElementById('ask').setAttribute('aria-busy', 'true')")
    gui.eval("document.body.insertAdjacentHTML('beforeend', '<div id=\"q\" data-question-key=\"k2\"></div>')")
    assert launcher.wait_log('агент ждёт: ответ на вопрос', 10), 'вопрос агента'
    assert launcher.log().count('] агент ждёт: подтверждение команды — ') == 1, 'то же ожидание — сообщено один раз'
    launcher.term()


NOTIFYD_LOG = '/tmp/notifyd.log'
NOTIFYD_OPEN = '/tmp/notifyd-open.json'


@pytest.fixture
def notifications():
    """Сессионная шина D-Bus с подставным сервером уведомлений (как у GNOME:
    уведомление лежит в списке, пока его не закроют) → окружение для лаунчера
    и open(): сколько уведомлений сейчас в списке."""
    out = subprocess.run(['runuser', '-u', 'tester', '--', 'dbus-daemon', '--session', '--address=unix:tmpdir=/tmp',
                          '--fork', '--print-address=1'], capture_output=True, text=True, check=True).stdout
    bus = out.splitlines()[0]
    subprocess.Popen(['runuser', '-u', 'tester', '--', 'env', f'DBUS_SESSION_BUS_ADDRESS={bus}', 'python3', f'{T}/fakes/notifyd.py'],
                     stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    assert wait_for(lambda: os.path.exists(NOTIFYD_LOG) and 'listening' in open(NOTIFYD_LOG).read(), 10, 0.1), 'сервер уведомлений'

    def opened():
        with open(NOTIFYD_OPEN) as f:
            return len(json.load(f))
    yield {'DBUS_SESSION_BUS_ADDRESS': bus}, opened


def test_agent_notification_cleared(start, gui, notifications):
    """Уведомление «агент закончил» не копится: следующее заменяет прежнее, а
    когда окно открыли — оно закрыто (иначе док-панель Ubuntu считает их
    непрочитанными, и счётчик на значке растёт 1, 2, …)."""
    bus, opened = notifications
    launcher = start(FAKE, NOUPD, bus)
    gui.wait()
    enable_tray(gui, launcher)
    gui.go('dshlauncher://window/close/')
    assert launcher.wait_log('окно спрятано в трей', 5)

    for n in (1, 2):
        gui.eval("document.body.insertAdjacentHTML('beforeend', '<div id=\"run\" data-chat-running></div>')")
        assert launcher.wait_log_count('агент отвечает', n, 10)
        gui.eval("document.getElementById('run').remove()")
        assert launcher.wait_log_count('уведомление: агент закончил ответ', n, 10)
        assert wait_for(lambda: open(NOTIFYD_LOG).read().count('summary=The agent has finished') == n, 5, 0.1), \
            f'уведомление {n} показано'
        assert opened() == 1, f'после уведомления {n} в списке одно, а не {opened()}'

    as_tester('DISPLAY=:99 HOME=/home/tester /usr/bin/dsh-launcher >/dev/null 2>&1')  # открыть окно
    assert launcher.wait_log('окно показано из трея', 15)
    assert wait_for(lambda: opened() == 0, 5, 0.1), f'окно открыли — уведомлений в списке нет ({opened()})'
    assert 'уведомление агента закрыто' in launcher.log()
    launcher.term()
