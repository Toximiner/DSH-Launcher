"""Пути, переменные окружения и версии, общие для всех сценариев."""
import os
import re

T = '/opt/t'                                   # каталог тестов в контейнере
HOME = '/home/tester'
CONF = f'{HOME}/.config/dsh-launcher'          # userData лаунчера
UPD = f'{CONF}/updates'                        # скачанные .deb обновлений
LAUNCHER_OUT = '/tmp/launcher.out'             # stdout/stderr лаунчера
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

# Пакет лаунчера в образе собран версией 1.1.0-1 — «старее» любого релиза.
INSTALLED = '1.1.0-1'
# Последний релиз лаунчера на GitHub (передаёт run.sh): тег vX.Y.Z → пакет X.Y.Z-1.
LATEST = os.environ.get('LATEST', '')
LATEST_PKG = LATEST if '-' in LATEST else f'{LATEST}-1'
LATEST_RE = re.escape(LATEST)
LATEST_DEB = f'dsh-launcher_{LATEST_PKG}_amd64.deb'
