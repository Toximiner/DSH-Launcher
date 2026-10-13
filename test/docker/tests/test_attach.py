"""Окно и dsh, который запущен не лаунчером или долго поднимается:
страница «Нужен токен» (dsh из терминала, куки нет) и «Не удалось запустить
dsh» (порт так и не открылся) — с переходом в GUI, когда dsh готов."""
import os
import subprocess

from helpers.env import FAKE, FAKEDSH, FAKEDSH_LOG, NOUPD
from helpers.procs import pgrep, wait_port
from helpers.windows import PastePage


def test_paste_token(start, gui, drv):
    """dsh уже запущен (в терминале) и требует токен, куки у окна нет —
    страница вставки; пользователь вставляет токен — окно в GUI dsh. Закрытие
    лаунчера чужой dsh не останавливает."""
    subprocess.Popen(['runuser', '-u', 'tester', '--', 'env', 'FAKE_REQUIRE_TOKEN=1', 'FAKE_ROLE=terminal', FAKEDSH,
                      '--profile', 'web'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, stdin=subprocess.DEVNULL)
    assert wait_port(15), 'dsh из «терминала» запущен'
    launcher = start(FAKE, NOUPD)
    page = PastePage(drv)
    page.wait(r'Token required.*paste the full URL here', 30)
    assert 'валидного куки нет — показываю страницу вставки токена' in launcher.log()

    page.paste('abc')  # только токен — адрес лаунчер подставит сам

    gui.wait()
    assert 'token accepted' in open(FAKEDSH_LOG).read(), 'токен ушёл в dsh'
    assert 'FAKE DSH terminal' in gui.eval('document.body.innerText'), 'в окне — GUI того самого dsh'
    launcher.term()
    assert pgrep('fakedsh.py') and wait_port(1), 'чужой dsh работает дальше'


def test_dsh_no_port(start, gui):
    """dsh запущен, но порт не поднял за отведённое время — страница «Не
    удалось запустить dsh» с журналом; поднял позже — окно само переходит в GUI."""
    listen = '/tmp/fakedsh-listen'
    if os.path.exists(listen):
        os.remove(listen)
    launcher = start(FAKE, NOUPD, FAKE_LISTEN_FILE=listen, DSH_START_TIMEOUT_MS='3000')
    page = PastePage(gui.drv)
    page.wait(r'Failed to start dsh.*did not bring up the service', 30)

    open(listen, 'w').close()  # dsh наконец поднял сервис

    assert wait_port(15), 'порт открылся'
    gui.wait(30)
    launcher.term()
