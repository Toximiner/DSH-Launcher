"""Инструменты страницы: контекстное меню по правому клику и поиск (Ctrl+F)."""
import pytest

from helpers.env import FAKE, NOUPD
from helpers.procs import dsh_tree_gone, wait_for

# Абзац, поле ввода и ссылка в известных координатах (CSS px = координаты клика)
FIXTURE = (
    '<p id=t style="position:absolute;left:20px;top:20px;margin:0;font-size:20px">Hello context menu</p>'
    '<input id=i value="some input text" style="position:absolute;left:20px;top:80px;width:300px;height:30px">'
    '<a id=l href="https://example.com/" style="position:absolute;left:20px;top:140px;font-size:20px">a link</a>'
)


def test_context_menu(start, gui):
    """Правый клик настоящим событием мыши → нативное меню. Кликнуть по
    пунктам driver не может; что показано — по строке в логе лаунчера."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    gui.set_body(FIXTURE)

    def menu_at(x, y):
        shown = launcher.context_menus_shown()
        gui.right_click(x, y)
        wait_for(lambda: launcher.context_menus_shown() > shown, 5, 0.25)
        return launcher.last_context_menu()

    gui.clear_selection()
    assert menu_at(600, 400) == 'Select all', 'пустое место'

    gui.select_text_of('t')
    assert menu_at(60, 30) == 'Copy, |, Select all', 'выделенный текст'

    gui.select_input('i')
    assert menu_at(100, 95) in ('Cut, Copy, Paste, |, Select all',
                                'Cut, Copy, Paste (off), |, Select all'), 'поле ввода с выделением'

    gui.clear_selection()
    assert menu_at(40, 150) == 'Copy link address, |, Select all', 'ссылка'

    assert launcher.alive, 'лаунчер жив после контекстных меню'
    assert launcher.term() is not None, 'лаунчер штатно завершился'
    assert dsh_tree_gone(), 'дерево dsh остановлено'


def test_find(start, gui):
    """Ctrl+F (настоящее нажатие) → панель поиска; счётчик «N / M»;
    «следующее» листает; нет совпадений — «none»."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    gui.set_body('<p>needle one</p><p>needle two</p><p>needle three</p>')

    find = gui.open_find()

    assert find.search('needle', lambda c: c.endswith('/ 3')), f'«needle» — 3 совпадения ({find.count()})'
    before = find.count()
    find.next()
    assert find.wait_count(lambda c: c != before), f'«следующее» сменило совпадение ({before})'
    assert find.search('nothing-like-this', lambda c: c == 'none'), f'нет совпадений ({find.count()})'
    assert launcher.alive, 'лаунчер жив'
    launcher.term()
