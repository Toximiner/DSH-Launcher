"""The tray and background operation: the close button hides the window (dsh
keeps running), a relaunch and disabling the tray bring the window back;
«the agent finished» — a dot on the tray icon and a system notification
(one; the window is opened — it is closed)."""
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

    gui.go('dshlauncher://window/close/')  # same as the close button
    assert launcher.wait_log('окно спрятано в трей', 5)
    assert launcher.alive and wait_port(5), 'the launcher and dsh keep running'

    # relaunching the launcher brings the hidden window back
    as_tester('DISPLAY=:99 HOME=/home/tester /usr/bin/dsh-launcher >/dev/null 2>&1')
    assert launcher.wait_log('окно показано из трея', 15)
    assert gui.not_reloaded(), 'the dsh GUI was not reloaded'
    launcher.term()


def test_tray_off_shows_window(start, gui):
    """The tray is disabled while the window is hidden — the window is shown
    (without the icon it would be impossible to get back)."""
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
    """The window is in the tray and the agent has finished its reply (the
    [data-chat-running] indicator in the chat has disappeared) — a system
    notification, a dot on the tray icon and (enabled) sound; the agent waits
    for a command confirmation or an answer to a question — the same signal."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    enable_tray(gui, launcher)
    gui.go('dshlauncher://settings/set/?k=attentionSound&v=true')
    assert launcher.wait_log('звук: агент закончил ответ', 5), 'enabled sound — an immediate test signal'
    gui.go('dshlauncher://window/close/')
    assert launcher.wait_log('окно спрятано в трей', 5)

    gui.eval("document.body.insertAdjacentHTML('beforeend', '<div id=\"run\" data-chat-running></div>')")
    assert launcher.wait_log('агент отвечает', 10)
    gui.eval("document.getElementById('run').remove()")
    assert launcher.wait_log('агент закончил ответ — окно в трее', 10)
    assert launcher.wait_log('значок трея: агент закончил ответ', 5)
    assert 'уведомление: агент закончил ответ' in launcher.log(), 'a system notification'
    assert launcher.log().count('звук: агент закончил ответ') == 2, 'and sound'

    # the agent has stopped and is waiting for a command confirmation — the same signal
    gui.eval("document.body.insertAdjacentHTML('beforeend', '<div id=\"ask\" data-approval-key=\"k1\"></div>')")
    assert launcher.wait_log('агент ждёт: подтверждение команды — окно в трее', 10)
    assert launcher.wait_log('уведомление: агент ждёт: подтверждение команды', 5)
    # answered (the panel is busy with the answer) — no repeat notification
    gui.eval("document.getElementById('ask').setAttribute('aria-busy', 'true')")
    gui.eval("document.body.insertAdjacentHTML('beforeend', '<div id=\"q\" data-question-key=\"k2\"></div>')")
    assert launcher.wait_log('агент ждёт: ответ на вопрос', 10), 'a question from the agent'
    assert launcher.log().count('] агент ждёт: подтверждение команды — ') == 1, 'the same wait — notified once'
    launcher.term()


NOTIFYD_LOG = '/tmp/notifyd.log'
NOTIFYD_OPEN = '/tmp/notifyd-open.json'


@pytest.fixture
def notifications():
    """A session D-Bus bus with a fake notification server (like GNOME's:
    a notification stays in the list until it is closed) → the environment
    for the launcher and open(): how many notifications are in the list now."""
    out = subprocess.run(['runuser', '-u', 'tester', '--', 'dbus-daemon', '--session', '--address=unix:tmpdir=/tmp',
                          '--fork', '--print-address=1'], capture_output=True, text=True, check=True).stdout
    bus = out.splitlines()[0]
    subprocess.Popen(['runuser', '-u', 'tester', '--', 'env', f'DBUS_SESSION_BUS_ADDRESS={bus}', 'python3', f'{T}/fakes/notifyd.py'],
                     stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    assert wait_for(lambda: os.path.exists(NOTIFYD_LOG) and 'listening' in open(NOTIFYD_LOG).read(), 10, 0.1), 'the notification server'

    def opened():
        with open(NOTIFYD_OPEN) as f:
            return len(json.load(f))
    yield {'DBUS_SESSION_BUS_ADDRESS': bus}, opened


def test_agent_notification_cleared(start, gui, notifications):
    """The «the agent finished» notification does not accumulate: the next
    one replaces the previous, and when the window is opened it is closed
    (otherwise Ubuntu's dock counts them as unread, and the counter on the
    icon grows 1, 2, …)."""
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
            f'notification {n} was shown'
        assert opened() == 1, f'after notification {n} there is one in the list, not {opened()}'
    # the second one — more than 8 s after start: the «ready-to-show never
    # came» fallback must not bring back a window that was already shown
    # and hidden into the tray
    assert 'показываю окно вручную' not in launcher.log(), 'the hidden window stayed in the tray'

    as_tester('DISPLAY=:99 HOME=/home/tester /usr/bin/dsh-launcher >/dev/null 2>&1')  # open the window
    assert launcher.wait_log('окно показано из трея', 15)
    assert wait_for(lambda: opened() == 0, 5, 0.1), f'the window was opened — no notifications in the list ({opened()})'
    assert 'уведомление агента закрыто' in launcher.log()
    launcher.term()
