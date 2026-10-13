"""Пути, переменные окружения и версии, общие для всех сценариев."""
import os
import re

T = '/opt/t'                                   # каталог тестов в контейнере
HOME = '/home/tester'
CONF = f'{HOME}/.config/dsh-launcher'          # userData лаунчера
UPD = f'{CONF}/updates'                        # скачанные .deb обновлений
LAUNCHER_OUT = '/tmp/launcher.out'             # stdout/stderr лаунчера
STATE = f'{HOME}/.local/state/dsh-launcher'    # логи лаунчера и dsh (DSH_LOG_DIR из run.sh пакета)
FAKEDSH = f'{T}/fakes/fakedsh.py'
FAKEDSH_LOG = '/tmp/fakedsh.log'
PROXY = f'{T}/fakes/proxy.py'
PROXY_LOG = '/tmp/proxy.log'
GUI_URL = r'^http://127\.0\.0\.1:3080/'        # адрес GUI dsh (регулярное выражение)

# Наборы переменных окружения для запуска лаунчера
FAKE = {'DSH_BIN': FAKEDSH}                                         # фейковый dsh
NOUPD = {'DSH_LAUNCHER_NO_UPDATE_CHECK': '1'}                       # без проверки обновлений
NO_PROMPTS = {'DSH_LAUNCHER_FAKE_LAUNCHER_LATEST': '1.0.0',         # проверка идёт, но «последние»
              'DSH_LAUNCHER_FAKE_DSH_LATEST': '0.0.1'}              # версии старые — без вопросов

# Токен CI для запросов лаунчера к GitHub API: убираем из окружения pytest при
# импорте — дочерние процессы теста его не наследуют; лаунчеру передаётся явно.
GITHUB_TOKEN = os.environ.pop('DSH_LAUNCHER_GITHUB_TOKEN', '')

# Подставной GitHub (fakes/github.py, helpers/github.py): лаунчер берёт список
# релизов и .deb у него (кроме сценариев прокси — те ходят к настоящему).
GITHUB_API = 'http://127.0.0.1:8765'

# Пакет лаунчера в образе собран версией 1.1.0-1 — «старее» последнего релиза.
INSTALLED = '1.1.0-1'
# Последний релиз в подставном GitHub: тег v2.0.0 → пакет 2.0.0-1 (из текущего
# кода, собирает run.sh). ROLLBACK — старый релиз с .deb, на него откатываются.
LATEST = '2.0.0'
LATEST_PKG = f'{LATEST}-1'
ROLLBACK = '1.0.0-14'
LATEST_RE = re.escape(LATEST)
LATEST_DEB = f'dsh-launcher_{LATEST_PKG}_amd64.deb'
