"""'Interface language' in settings: ru → ui-lang.json + the dialog sections
in Russian, en → back, auto → the file is removed and the language comes
from the system locale (C.UTF-8 → en). Also checks a fix: changing the
language does not redraw the old status page («Запуск dsh…») on top of the
already-loaded dsh GUI (main.js, did-finish-load). The dialog's CDP page
title is fixed when the window is opened (closure) and stays as it was —
the content is translated, not the title."""
import json

from helpers.env import CONF, FAKE, NOUPD
from helpers.windows import Dialog

LANG_FILE = f'{CONF}/ui-lang.json'


class SettingsDialogRu(Dialog):
    """The same settings window when the title was «Настройки» (opened in ru)."""
    TITLE = r'^Настройки$'


def lang_file():
    try:
        return json.load(open(LANG_FILE))
    except (OSError, ValueError):
        return None


def test_settings_lang(start, gui):
    launcher = start(FAKE, NOUPD)
    gui.wait()

    # Russian: the file, the log; the dialog content is redrawn in Russian
    # (the CDP page title — in the language the window was opened in).
    settings = gui.open_settings()
    settings.set('lang', 'ru')
    settings.wait_text('Общие.*Обновления.*Запуск dsh.*Графика', 15)
    assert launcher.wait_log('язык окна: ru', 10), launcher.log()[-1000:]
    assert launcher.wait_log('настройки: lang = ru', 10), launcher.log()[-1000:]
    assert lang_file() == {'lang': 'ru'}, lang_file()

    # Meanwhile the dsh GUI is not clobbered by the old status page.
    assert 'FAKE DSH' in (gui.eval('document.body.innerText') or ''), \
        'the dsh GUI was clobbered by the «Запуск dsh…» status page'

    # English: the file is updated, the content is in English again.
    settings.set('lang', 'en')
    settings.wait_text('General.*Updates.*Starting dsh.*Graphics', 15)
    assert launcher.wait_log('язык окна: en', 10), launcher.log()[-1000:]
    assert lang_file() == {'lang': 'en'}, lang_file()

    # auto: the file is removed, the language — from the system locale (C.UTF-8 → en).
    settings.set('lang', 'auto')
    assert launcher.wait_log('настройки: lang = auto', 10), launcher.log()[-1000:]
    assert lang_file() is None, 'ui-lang.json removed'
    settings.wait_text('General.*Updates.*Starting dsh.*Graphics', 15)

    launcher.term()
