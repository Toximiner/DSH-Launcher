"""Инструменты страницы: контекстное меню по правому клику и поиск (Ctrl+F)."""
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

    def menu_at(x, y, prepare):
        """prepare() — выделение и т. п. прямо перед кликом (предыдущее нативное
        меню ещё может быть открыто и сбить подготовленное раньше); prepare
        возвращается, когда выделение действительно стоит."""
        shown = launcher.context_menus_shown()
        prepare()
        gui.right_click(x, y)
        wait_for(lambda: launcher.context_menus_shown() > shown, 5, 0.25)
        return launcher.last_context_menu()

    assert menu_at(600, 400, gui.clear_selection) == 'Select all', 'пустое место'
    assert menu_at(60, 30, lambda: gui.select_text_of('t')) == 'Copy, |, Select all', 'выделенный текст'
    assert menu_at(100, 95, lambda: gui.select_input('i')) in (
        'Cut, Copy, Paste, |, Select all', 'Cut, Copy, Paste (off), |, Select all'), 'поле ввода с выделением'
    assert menu_at(40, 150, gui.clear_selection) == 'Copy link address, |, Select all', 'ссылка'

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
