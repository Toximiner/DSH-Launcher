"""«Что нового», если при запуске GitHub не отвечал: список релизов не получен
— окно не появляется и версия не запоминается; как только периодическая
проверка получает список релизов — пропущенные изменения показываются сразу
(а не копятся до следующего обновления)."""
import json
import re
import subprocess
import time

from helpers.env import CONF, FAKE, INSTALLED, NO_PROMPTS
from helpers.github import DEBS, PORT, RELEASES, SERVER, github_log
from helpers.procs import as_tester, pgrep, sh, wait_for
from helpers.windows import WhatsNewDialog


def test_whats_new_retry(start, gui, drv):
    # Подставной GitHub — выключен: при запуске лаунчер список релизов не
    # получит (соединение отклонено). Без оболочки: в командной строке
    # `sh -c "pkill -f …"` тоже есть шаблон — pkill убил бы саму оболочку.
    subprocess.run(['pkill', '-f', 'fakes/github.py'], capture_output=True)
    assert wait_for(lambda: not pgrep('github.py'), 5, 0.1), 'подставной GitHub не остановился'
    as_tester(f"mkdir -p {CONF} && echo '{{\"version\":\"1.0.0-1\"}}' > {CONF}/last-version.json")

    launcher = start(FAKE, NO_PROMPTS, DSH_LAUNCHER_UPDATE_POLL_SEC='2')
    gui.wait()

    # Цикл проверки без GitHub: окна нет, версия не запомнена (покажем при
    # ближайшей успешной проверке).
    whats_new = WhatsNewDialog(drv)
    time.sleep(4)
    assert not whats_new.is_open(), 'окно «Что нового» без списка релизов'
    assert 'обновлён' not in launcher.log(), 'версия запомнена без списка релизов'
    assert json.load(open(f'{CONF}/last-version.json'))['version'] == '1.0.0-1'

    # GitHub снова доступен — ближайшая успешная проверка (2 с) показывает
    # пропущенные изменения и запоминает версию.
    sh('rm -f /tmp/github.log')
    subprocess.Popen(f'runuser -u tester -- python3 {SERVER} {PORT} {RELEASES} {DEBS} >/tmp/github.out 2>&1',
                     shell=True)
    assert wait_for(lambda: 'listening' in github_log(), 5, 0.1), 'подставной GitHub не запустился'

    whats_new.wait_text(rf'updated: 1\.0\.0-1 → {re.escape(INSTALLED)} .*What.s new: 1\.1\.0 ', 30)
    assert json.load(open(f'{CONF}/last-version.json'))['version'] == INSTALLED, 'версия запомнена'
    whats_new.close()
    launcher.term()
