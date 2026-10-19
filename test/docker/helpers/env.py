"""Пути, переменные окружения и версии, общие для всех сценариев."""
import os
import re
import secrets

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

# «Последняя» версия dsh, которую видит лаунчер (DSH_LAUNCHER_FAKE_DSH_LATEST).
# Пиним: настоящий npm-реестр живёт своей жизнью — когда там появился
# 0.2.1-rc.2, сценарии, ждавшие 0.2.0-rc.2, упали без единой правки кода.
# 0.2.0-rc.2 совпадает с FAKE_DSH_VERSION фейкового dsh (он же «установлен»).
DSH_LATEST = '0.2.0-rc.2'

# Наборы переменных окружения для запуска лаунчера
FAKE = {'DSH_BIN': FAKEDSH}                                         # фейковый dsh
NOUPD = {'DSH_LAUNCHER_NO_UPDATE_CHECK': '1'}                       # без проверки обновлений
NO_PROMPTS = {'DSH_LAUNCHER_FAKE_LAUNCHER_LATEST': '1.0.0',         # проверка идёт, но «последние»
              'DSH_LAUNCHER_FAKE_DSH_LATEST': '0.0.1'}              # версии старые — без вопросов

# Токен CI для запросов лаунчера к GitHub API: убираем из окружения pytest при
# импорте — дочерние процессы теста его не наследуют; лаунчеру передаётся явно.
GITHUB_TOKEN = os.environ.pop('DSH_LAUNCHER_GITHUB_TOKEN', '')

# Ключ маршрутов dshlauncher://… (без него лаунчер команды не принимает):
# в тестах задаём сами (DSH_LAUNCHER_ROUTE_KEY), драйвер подставляет его в адрес.
ROUTE_KEY = secrets.token_hex(16)

# Покрытие кода лаунчера сценариями: main-процесс Electron пишет его сюда
# (NODE_V8_COVERAGE, при штатном выходе); conftest после сценария оставляет
# только файлы лаунчера и кладёт в /results, отчёт строит run.sh (c8).
V8_COVERAGE = '/tmp/v8-coverage'

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
