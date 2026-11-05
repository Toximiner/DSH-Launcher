"""«Проверить снова» с фатальной страницы: dsh мёртв (порт закрыт) — страница
«dsh не запущен» с командой для терминала; запустили dsh руками — «Проверить
снова» снова доводит окно до GUI (чужой dsh не убиваем при закрытии).

Все CDP-операции — с page= (по заголовку): в списке /json/list первым может
оказаться плашка-уведомление (launcher-toast — отдельная CDP-страница)."""
import subprocess

from helpers.env import FAKE, FAKEDSH, NOUPD
from helpers.procs import pgrep, wait_port
from helpers.windows import Toast

FATAL = 'dsh has stopped'
NOTRUN = 'dsh is not running'


def test_retry_dead(start, gui, drv):
    launcher = start(FAKE, NOUPD, FAKE_CRASH_AFTER='2')
    gui.wait()

    # dsh падает и снова падает — лаунчер перестаёт перезапускать и показывает
    # фатальную страницу с кнопкой «Проверить снова».
    assert launcher.wait_log('dsh падает снова и снова — авто-перезапуск остановлен', 150), launcher.log()[-2000:]
    drv.wait_text(FATAL + r'.*Check again', 30, page=FATAL)

    # Порт закрыт (dsh мёртв) → «Проверить снова» не до GUI, а к странице
    # «dsh не запущен» с командой запуска в терминале.
    drv.go('dshlauncher://retry/', page=FATAL)
    drv.wait_text(NOTRUN + r'.*Port 3080 is closed.*Start it in a terminal', 30, page=NOTRUN)

    # Запускаем dsh «из терминала» (руками) — «Проверить снова» подключается
    # к нему: GUI, и лаунчер не считает этот dsh своим.
    subprocess.Popen(['runuser', '-u', 'tester', '--', 'env', 'FAKE_ROLE=terminal', FAKEDSH],
                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, stdin=subprocess.DEVNULL)
    assert wait_port(15), 'ручной dsh поднял порт'
    drv.go('dshlauncher://retry/', page=NOTRUN)
    assert launcher.wait_log('retry: dsh запущен: куки валидны — открываю GUI', 60), launcher.log()[-2000:]
    gui.wait()
    assert 'FAKE DSH terminal' in (gui.eval('document.body.innerText') or ''), 'в окне — GUI того самого dsh'
    assert pgrep('fakedsh.py'), 'ручной dsh жив'

    # закрытие: чужой dsh не убиваем
    assert launcher.term() is not None, 'лаунчер штатно завершился'
    assert pgrep('fakedsh.py') and wait_port(1), 'dsh, запущенный не лаунчером, остался работать'
