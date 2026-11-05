"""Page tools: the right-click context menu and search (Ctrl+F)."""
from helpers.env import FAKE, NOUPD
from helpers.procs import dsh_tree_gone, wait_for

# A paragraph, an input field and a link at known coordinates (CSS px = click coordinates)
FIXTURE = (
    '<p id=t style="position:absolute;left:20px;top:20px;margin:0;font-size:20px">Hello context menu</p>'
    '<input id=i value="some input text" style="position:absolute;left:20px;top:80px;width:300px;height:30px">'
    '<a id=l href="https://example.com/" style="position:absolute;left:20px;top:140px;font-size:20px">a link</a>'
)


def test_context_menu(start, gui):
    """A right click with a real mouse event → the native menu. The driver
    cannot click the items; what is shown — from a line in the launcher log."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    gui.set_body(FIXTURE)

    def menu_at(x, y, prepare):
        """The menu shown by a right click. A native menu from a previous click
        in the container does not close on its own and gets in the way of the
        selection — close it first and wait for the close event; prepare()
        (the selection) returns once the selection is really in place."""
        shown = launcher.context_menus_shown()
        if shown > launcher.context_menus_closed():
            closed = launcher.context_menus_closed()
            gui.close_context_menu()
            assert wait_for(lambda: launcher.context_menus_closed() > closed, 5, 0.1), 'the previous menu closed'
        prepare()
        gui.right_click(x, y)
        wait_for(lambda: launcher.context_menus_shown() > shown, 5, 0.25)
        return launcher.last_context_menu()

    assert menu_at(600, 400, gui.clear_selection) == 'Select all', 'an empty spot'
    assert menu_at(60, 30, lambda: gui.select_text_of('t')) == 'Copy, |, Select all', 'selected text'
    assert menu_at(100, 95, lambda: gui.select_input('i')) in (
        'Cut, Copy, Paste, |, Select all', 'Cut, Copy, Paste (off), |, Select all'), 'an input field with a selection'
    assert menu_at(40, 150, gui.clear_selection) == 'Copy link address, |, Select all', 'a link'

    assert launcher.alive, 'the launcher is alive after the context menus'
    assert launcher.term() is not None, 'the launcher exited normally'
    assert dsh_tree_gone(), 'the dsh tree was stopped'


def test_find(start, gui):
    """Ctrl+F (a real key press) → the find panel; the «N / M» counter;
    «next» steps through; no matches — «none»."""
    launcher = start(FAKE, NOUPD)
    gui.wait()
    gui.set_body('<p>needle one</p><p>needle two</p><p>needle three</p>')

    find = gui.open_find()

    assert find.search('needle', lambda c: c.endswith('/ 3')), f'"needle" — 3 matches ({find.count()})'
    before = find.count()
    find.next()
    assert find.wait_count(lambda c: c != before), f'"next" moved to another match ({before})'
    # "previous" — back to the first match
    find.prev()
    assert find.wait_count(lambda c: c == before), f'"previous" returned to the first match ({find.count()})'
    assert find.search('nothing-like-this', lambda c: c == 'none'), f'no matches ({find.count()})'
    # closing (×) and reopening with Ctrl+F
    find.close()
    find2 = gui.open_find()
    assert find2.search('needle', lambda c: c.endswith('/ 3')), 'the panel reopened, the matches were found again'
    find2.close()
    assert launcher.alive, 'the launcher is alive'
    launcher.term()
