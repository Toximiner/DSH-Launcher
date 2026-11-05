"""«Язык интерфейса» в настройках: ru → ui-lang.json + разделы диалога на
русском, en → обратно, auto → файл удалён и язык по системной (C.UTF-8 → en).
Проверяет и фикс: смена языка не перерисовывает старую статус-страницу
(«Запуск dsh…») поверх уже загруженного GUI dsh (main.js, did-finish-load).
Заголовок CDP-страницы диалога зафиксирован при открытии окна (closure) и
останется прежним — переводим контент, не заголовок."""
import json

from helpers.env import CONF, FAKE, NOUPD
from helpers.windows import Dialog

LANG_FILE = f'{CONF}/ui-lang.json'


class SettingsDialogRu(Dialog):
    """То же окно настроек, когда заголовок был «Настройки» (открыто на ru)."""
    TITLE = r'^Настройки$'


def lang_file():
    try:
        return json.load(open(LANG_FILE))
    except (OSError, ValueError):
        return None


def test_settings_lang(start, gui):
    launcher = start(FAKE, NOUPD)
    gui.wait()

    # Русский: файл, лог; контент диалога перерисован на русском
    # (заголовок CDP-страницы — в языке, при котором открыли окно).
    settings = gui.open_settings()
    settings.set('lang', 'ru')
    settings.wait_text('Общие.*Обновления.*Запуск dsh.*Графика', 15)
    assert launcher.wait_log('язык окна: ru', 10), launcher.log()[-1000:]
    assert launcher.wait_log('настройки: lang = ru', 10), launcher.log()[-1000:]
    assert lang_file() == {'lang': 'ru'}, lang_file()

    # GUI dsh при этом не затёрт старой статус-страницей.
    assert 'FAKE DSH' in (gui.eval('document.body.innerText') or ''), \
        'GUI dsh затёрт статус-страницей «Запуск dsh…»'

    # Английский: файл обновлён, контент снова по-английски.
    settings.set('lang', 'en')
    settings.wait_text('General.*Updates.*Starting dsh.*Graphics', 15)
    assert launcher.wait_log('язык окна: en', 10), launcher.log()[-1000:]
    assert lang_file() == {'lang': 'en'}, lang_file()

    # auto: файл удалён, язык — по системной (C.UTF-8 → en).
    settings.set('lang', 'auto')
    assert launcher.wait_log('настройки: lang = auto', 10), launcher.log()[-1000:]
    assert lang_file() is None, 'ui-lang.json удалён'
    settings.wait_text('General.*Updates.*Starting dsh.*Graphics', 15)

    launcher.term()
