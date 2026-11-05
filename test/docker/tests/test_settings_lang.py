"""«Язык интерфейса» в настройках: ru → ui-lang.json + диалог на русском,
en → обратно, auto → файл удалён и язык по системной (C.UTF-8 → en).
Проверяет и фикс: смена языка не перерисовывает старую статус-страницу
(«Запуск dsh…») поверх уже загруженного GUI dsh (main.js, did-finish-load)."""
import json

from helpers.env import CONF, FAKE, NOUPD
from helpers.procs import wait_for
from helpers.windows import Dialog

LANG_FILE = f'{CONF}/ui-lang.json'


class SettingsRu(Dialog):
    """То же окно настроек после перевода на русский — заголовок изменился."""
    TITLE = r'^Настройки$'


def lang_file():
    try:
        return json.load(open(LANG_FILE))
    except (OSError, ValueError):
        return None


def test_settings_lang(start, gui):
    launcher = start(FAKE, NOUPD)
    gui.wait()

    # Русский: файл, лог; диалог перерисован на русском (заголовок сменился).
    settings = gui.open_settings()
    settings.set('lang', 'ru')
    assert wait_for(lambda: settings.drv.page(r'^Настройки$'), 15), 'окно настроек перешло на русский'
    ru = SettingsRu(settings.drv)
    ru.wait_text('Общие.*Обновления.*Запуск dsh.*Графика', 15)
    assert launcher.wait_log('язык окна: ru', 10), launcher.log()[-1000:]
    assert launcher.wait_log('настройки: lang = ru', 10), launcher.log()[-1000:]
    assert lang_file() == {'lang': 'ru'}, lang_file()

    # GUI dsh при этом не затёрт старой статус-страницей.
    assert 'FAKE DSH' in (gui.eval('document.body.innerText') or ''), \
        'GUI dsh затёрт статус-страницей «Запуск dsh…»'

    # Английский: файл обновлён, заголовок обратно «Settings».
    ru.set('lang', 'en')
    assert wait_for(lambda: settings.drv.page('^Settings$'), 15), 'окно настроек вернулось на английский'
    settings.drv.wait_text('General.*Updates.*Starting dsh.*Graphics', 15)
    assert launcher.wait_log('язык окна: en', 10), launcher.log()[-1000:]
    assert lang_file() == {'lang': 'en'}, lang_file()

    # auto: файл удалён, язык — по системной (C.UTF-8 → en).
    settings.set('lang', 'auto')
    assert launcher.wait_log('настройки: lang = auto', 10), launcher.log()[-1000:]
    assert lang_file() is None, 'ui-lang.json удалён'
    settings.drv.wait_text('General.*Updates.*Starting dsh.*Graphics', 15)

    launcher.term()
