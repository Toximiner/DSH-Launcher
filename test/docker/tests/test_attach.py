"""Окно и dsh, который запущен не лаунчером или долго поднимается:
страница «Нужен токен» (dsh из терминала, куки нет), вход без неё (dsh
перезапущен кнопкой маркета, ссылка — в его файле вывода) и «Не удалось
запустить dsh» (порт так и не открылся) — с переходом в GUI, когда dsh готов."""
import os
import subprocess

from helpers.env import FAKE, FAKEDSH, FAKEDSH_LOG, NOUPD
from helpers.procs import pgrep, wait_for, wait_port
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
    assert 'подключение: валидного куки и ссылки входа нет — страница вставки токена' in launcher.log()

    page.paste('abc')  # только токен — адрес лаунчер подставит сам

    gui.wait()
    assert 'token accepted' in open(FAKEDSH_LOG).read(), 'токен ушёл в dsh'
    assert 'FAKE DSH terminal' in gui.eval('document.body.innerText'), 'в окне — GUI того самого dsh'
    launcher.term()
    assert pgrep('fakedsh.py') and wait_port(1), 'чужой dsh работает дальше'


def test_market_restart_token(start, gui):
    """dsh перезапустили кнопкой маркета: новый процесс — не дочерний
    лаунчера, со своим токеном (старое куки не годится) и печатает ссылку
    входа в свой файл. Лаунчер берёт её из вывода процесса на порту — окно
    снова в GUI, без страницы «Нужен токен»."""
    out = '/tmp/fakedsh-reborn.out.log'
    launcher = start(FAKE, NOUPD, FAKE_REQUIRE_TOKEN='1', FAKE_SELF_RESTART_AFTER='5',
                     FAKE_REBORN_TOKEN='xyz', FAKE_REBORN_OUT=out)
    gui.wait()

    assert launcher.wait_log('ссылка входа — из вывода процесса dsh на порту', 40), 'ссылка взята из вывода нового dsh'

    def reborn_gui():
        try:
            return 'FAKE DSH reborn' in (gui.eval('document.body.innerText') or '')
        except Exception:  # noqa: BLE001 — окно в переходе
            return False
    assert wait_for(reborn_gui, 20), 'в окне — GUI перезапущенного dsh'
    assert 'token=xyz' not in launcher.log(), 'токен не попал в журнал'
    assert 'страница вставки токена' not in launcher.log()


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
