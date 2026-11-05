"""«Проверить снова» с фатальной страницы: dsh мёртв (порт закрыт) — страница
«dsh не запущен» с командой для терминала; запустили dsh руками — «Проверить
снова» снова доводит окно до GUI (чужой dsh не убиваем при закрытии)."""
import subprocess

from helpers.env import FAKE, FAKEDSH, NOUPD
from helpers.procs import pgrep, wait_for, wait_port


def test_retry_dead(start, gui, drv):
    launcher = start(FAKE, NOUPD, FAKE_CRASH_AFTER='2')
    gui.wait()

    # dsh падает и снова падает — лаунчер перестаёт перезапускать и показывает
    # фатальную страницу с кнопкой «Проверить снова».
    assert launcher.wait_log('dsh падает снова и снова — авто-перезапуск остановлен', 150), launcher.log()[-2000:]
    drv.wait_text('dsh has stopped.*Check again', 30)

    # Порт закрыт (dsh мёртв) → «Проверить снова» не до GUI, а к странице
    # «dsh не запущен» с командой запуска в терминале.
    drv.go('dshlauncher://retry/')
    drv.wait_text('dsh is not running.*Port 3080 is closed.*Start it in a terminal', 30)

    # Запускаем dsh «из терминала» (руками) — «Проверить снова» подключается
    # к нему: GUI, и лаунчер не считает этот dsh своим.
    subprocess.Popen(['runuser', '-u', 'tester', '--', 'env', 'FAKE_ROLE=terminal', FAKEDSH],
                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, stdin=subprocess.DEVNULL)
    assert wait_port(15), 'ручной dsh поднял порт'
    drv.go('dshlauncher://retry/')
    assert launcher.wait_log('retry: dsh запущен: куки валидны — открываю GUI', 60), launcher.log()[-2000:]
    assert wait_for(lambda: 'FAKE DSH terminal' in (gui.eval('document.body.innerText') or ''), 30), \
        'в окне — GUI того самого dsh'
    assert pgrep('fakedsh.py'), 'ручной dsh жив'

    # закрытие: чужой dsh не убиваем
    assert launcher.term() is not None, 'лаунчер штатно завершился'
    assert pgrep('fakedsh.py') and wait_port(1), 'dsh, запущенный не лаунчером, остался работать'
