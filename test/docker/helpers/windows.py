"""Launcher window objects (page objects): the test says WHAT it does, while the
how — the dshlauncher://… routes, CDP, the markup — is hidden here.

Menu windows are opened via routes (the driver cannot click the native menu);
a route answers 204 — the page you came from stays in place.
"""
import json
import re
import time

from .env import GUI_URL
from .procs import wait_for


class Gui:
    """The main window with the dsh GUI."""

    def __init__(self, drv):
        self.drv = drv

    def wait(self, timeout=60):
        """Wait for the dsh GUI to load (we look for the window by address: the
        first entry in the CDP list may be a dialog window)."""
        self.drv.wait_url(GUI_URL, timeout, page=GUI_URL)
        # The address in the CDP list changes at the moment of navigation, while the
        # document may still be the old one (the helper page “Connecting…”): we wait
        # until the page already contains the dsh GUI and it is fully loaded.
        assert wait_for(lambda: self.eval("location.origin === 'http://127.0.0.1:3080' && "
                                          "document.readyState === 'complete'"), timeout, 0.2), 'dsh GUI did not finish loading'
        return self

    def eval(self, js):
        return self.drv.eval(js, page=GUI_URL)

    def go(self, route, keyed=True):
        self.drv.go(route, page=GUI_URL, keyed=keyed)

    def close(self):
        """Close the window from the page (window.close())."""
        self.drv.eval('window.close()', page=GUI_URL)

    def quit(self):
        """“File → Quit” (the same app.quit() as the menu item and the tray)."""
        self.go('dshlauncher://menu/quit/')

    def hidden(self):
        """The main window is hidden (the dsh GUI page is still alive)."""
        return self.eval('document.visibilityState') == 'hidden'

    # ---- page state ----

    def mark(self):
        """Mark the page — not_reloaded() will later show that it was not reloaded."""
        self.eval('window.__mark = 42; 1')

    def not_reloaded(self):
        return self.eval('window.__mark') == 42

    def outer_width(self):
        return self.eval('window.outerWidth')

    # ---- page content for checks ----

    def set_body(self, html):
        """Replace the page content; return once it is in place."""
        self.eval(f'document.body.innerHTML = {json.dumps(html)}; window.__body = true; 1')
        assert wait_for(lambda: self.eval('window.__body === true'), 5, 0.1), 'page content was not set'

    def select_text_of(self, element_id):
        """Select the element's text; return once the selection is really in place."""
        # A native menu closing from a previous click may reset the
        # selection — we set it again until it holds.
        js_id = json.dumps(element_id)
        select = f"""(() => {{ const s = getSelection();
            if (s.rangeCount && !s.isCollapsed && s.toString().trim() === document.getElementById({js_id}).textContent.trim()) return true;
            const r = document.createRange(); r.selectNodeContents(document.getElementById({js_id}));
            s.removeAllRanges(); s.addRange(r); return false; }})()"""
        assert wait_for(lambda: self.eval(select), 5, 0.2), 'the selection was not set'

    def select_input(self, element_id):
        """Focus the field and select all; return once the selection is in place."""
        js_id = json.dumps(element_id)
        select = f"""(() => {{ const i = document.getElementById({js_id});
            if (document.activeElement === i && i.selectionEnd - i.selectionStart === i.value.length) return true;
            i.focus(); i.select(); return false; }})()"""
        assert wait_for(lambda: self.eval(select), 5, 0.2), 'the field selection was not set'

    def clear_selection(self):
        self.eval('getSelection().removeAllRanges(); document.activeElement && document.activeElement.blur(); 1')
        assert wait_for(lambda: self.eval("getSelection().toString() === '' && document.activeElement === document.body"),
                        5, 0.1), 'the selection was not cleared'

    def right_click(self, x, y):
        self.drv.right_click(x, y, page=GUI_URL)

    def close_context_menu(self):
        """Close an open context menu (in the container it will not close by itself)."""
        self.go('dshlauncher://menu/close-context/')

    def press(self, key):
        """A key press in the main window (e.g. “Ctrl+Equal”)."""
        self.drv.key(key, page=GUI_URL)

    # ---- menu windows ----

    def open_about(self):
        self.go('dshlauncher://menu/about/')
        return AboutDialog(self.drv).wait_loaded()

    def open_update_check(self):
        self.go('dshlauncher://menu/check/')
        return UpdateCheckDialog(self.drv).wait_result()

    def open_dsh_log(self):
        self.go('dshlauncher://menu/log/')
        return LogDialog(self.drv).wait_loaded()

    def open_settings(self):
        self.go('dshlauncher://menu/settings/')
        return SettingsDialog(self.drv).wait_loaded()

    def open_update_now(self, kind):
        """“Help → Update … to X…” (the item exists when an update is found)."""
        self.go(f'dshlauncher://menu/update/?k={kind}')
        return UpdateNowDialog(self.drv, kind).wait_loaded()

    def open_rollback(self):
        self.go('dshlauncher://menu/rollback/')
        return RollbackDialog(self.drv)

    def open_find(self):
        self.drv.key('Ctrl+KeyF', page=GUI_URL)
        return FindBar(self.drv).wait_ready()

    def restart_dsh(self):
        """“File → Restart dsh” without a confirmation prompt."""
        self.go('dshlauncher://menu/restart-dsh/')


class Dialog:
    """A dialog window over the main one (a separate CDP page with the TITLE title)."""
    TITLE = None  # a title regex

    def __init__(self, drv):
        self.drv = drv

    def text(self):
        """What is rendered (the DOM). The page address changes on a redraw
        before the new DOM is loaded — it cannot be used to check."""
        return ' '.join((self.drv.eval('document.body ? document.body.innerText : ""', page=self.TITLE) or '').split())

    def wait_text(self, pattern, timeout=30):
        rx = re.compile(pattern)
        last = []
        ok = wait_for(lambda: last.append(self.text()) or rx.search(last[-1]), timeout, 0.25)
        assert ok, f'window {self.TITLE}: did not see /{pattern}/ within {timeout}s; last: {(last or [""])[-1][:600]}'
        return last[-1]

    def is_open(self):
        return self.drv.page(self.TITLE) is not None

    def is_overlay(self):
        """The dialog is a layer inside the main window: a dimming over the whole area, a card in the center."""
        return bool(self.drv.eval("document.documentElement.classList.contains('ovl')"
                                  " && getComputedStyle(document.body).backgroundColor.includes('0.55')", page=self.TITLE))

    def go(self, route):
        self.drv.go(route, page=self.TITLE)

    def rows(self):
        """The window's table: {label: value}."""
        raw = self.drv.eval("JSON.stringify([...document.querySelectorAll('tr')]"
                            ".map(r => [...r.cells].map(c => c.innerText.trim())))", page=self.TITLE)
        return {r[0]: r[1] for r in json.loads(raw or '[]') if len(r) >= 2}

    def close(self):
        self.go('dshlauncher://dialog/close/')
        assert wait_for(lambda: not self.is_open(), 10), f'window {self.TITLE} did not close'

    def press(self, key):
        """A key press in the window (e.g. “Escape”)."""
        self.drv.key(key, page=self.TITLE)


class AboutDialog(Dialog):
    TITLE = '^About$'

    def wait_loaded(self):
        self.wait_text('Copy for report', 30)  # the button appears once the versions are loaded
        return self

    def value(self, label):
        return self.rows().get(label)

    def copy_for_report(self):
        self.go('dshlauncher://dialog/copy/')
        self.wait_text('Copied', 10)

    def reenable_gpu(self):
        """“Re-enable GPU” (the button exists only if the GPU was disabled after failures)."""
        self.wait_text('Re-enable GPU', 5)
        self.go('dshlauncher://dialog/gpu-enable/')


class UpdateCheckDialog(Dialog):
    TITLE = '^Update check$'

    def wait_result(self):
        self.wait_text('up to date|available|could not', 60)
        return self

    def status(self, label):
        return self.rows().get(label, '')


class LogDialog(Dialog):
    TITLE = '^dsh log$'

    def wait_loaded(self):
        assert wait_for(lambda: self.is_open() and self.log_text() is not None, 15), 'the log window did not open'
        return self

    def log_text(self):
        return self.drv.eval("(document.querySelector('pre.log') || {}).innerText", page=self.TITLE)


class SettingsDialog(Dialog):
    TITLE = '^Settings$'

    def wait_loaded(self):
        self.wait_text('General.*Updates.*Starting dsh.*Graphics', 15)
        return self

    def set(self, key, value):
        """Change a field (the way the window itself does when the field changes)."""
        self.go(f'dshlauncher://settings/set/?k={key}&v={value}')

    def note_inside_card(self):
        """The floating “Saved”/error — inside the dialog card (not in the window corner)."""
        return bool(self.drv.eval("(() => { const p = document.querySelector('.pop'), w = document.querySelector('.wrap');"
                                  " if (!p || !w) return false; const a = p.getBoundingClientRect(), b = w.getBoundingClientRect();"
                                  " return a.left >= b.left && a.right <= b.right && a.top >= b.top && a.bottom <= b.bottom; })()",
                                  page=self.TITLE))

    def active_section(self):
        """Which section is selected in the left menu (and its content is shown)."""
        return self.drv.eval("(() => { const a = document.querySelector('.sett nav a.on');"
                             " return a && !document.getElementById('sec-' + a.dataset.s).hidden ? a.dataset.s : null; })()",
                             page=self.TITLE)

    def section(self, sid):
        """A section in the left menu: general, updates, dsh, window, graphics."""
        self.drv.eval(f"sec({json.dumps(sid)})", page=self.TITLE)
        assert wait_for(lambda: self.drv.eval(
            f"!document.getElementById('sec-' + {json.dumps(sid)}).hidden", page=self.TITLE), 5, 0.2), f'section {sid}'

    def wait_note(self, pattern, timeout=10):
        return self.wait_text(pattern, timeout)

    def restart_banner(self):
        return 'Changes take effect after the launcher restarts' in self.text()

    def disabled(self, element_id):
        return self.drv.eval(f"(document.getElementById({json.dumps(element_id)}) || {{}}).disabled === true", page=self.TITLE)

    def value(self, element_id):
        """The window field's value (input field text does not land in innerText)."""
        return self.drv.eval(f"(document.getElementById({json.dumps(element_id)}) || {{}}).value", page=self.TITLE)

    def relaunch(self):
        self.go('dshlauncher://settings/relaunch/')


class Toast:
    """A toast in the corner of the main window (its own CDP page): an update, a failure."""
    TITLE = '^launcher-toast$'

    def __init__(self, drv):
        self.drv = drv

    def text(self):
        return ' '.join((self.drv.eval('document.body ? document.body.innerText : ""', page=self.TITLE) or '').split())

    def wait_shown(self, pattern='Details', timeout=30):
        rx = re.compile(pattern)
        assert wait_for(lambda: self.drv.page(self.TITLE) is not None and rx.search(self.text()), timeout, 0.25), \
            f'toast /{pattern}/ did not appear'
        return self

    def is_shown(self):
        return self.drv.page(self.TITLE) is not None

    def details(self, kind):
        """“Details…” — the “Update … to X…” window; the toast closes."""
        self.drv.go(f'dshlauncher://toast/open/?k={kind}', page=self.TITLE)
        return UpdateNowDialog(self.drv, kind).wait_loaded()

    def close(self):
        self.drv.go('dshlauncher://toast/close/', page=self.TITLE)


UpdateToast = Toast  # the update toast — the same one


class RollbackDialog(Dialog):
    TITLE = '^Roll back to a previous version$'


class ReportDialog(Dialog):
    TITLE = '^Report$'


class UpdateNowDialog(Dialog):
    """The “Update … to X…” window: what's new, a warning, “Update now” / “Later”."""

    def __init__(self, drv, kind):
        super().__init__(drv)
        self.kind = kind
        self.TITLE = '^DeepSeek Harness Launcher update$' if kind == 'launcher' else '^dsh update$'

    def wait_loaded(self):
        self.wait_text('Update now', 15)
        return self

    def later(self):
        self.close()  # “Later” — the same as closing the window

    def update_now(self):
        self.go(f'dshlauncher://update-now/?k={self.kind}')


class WhatsNewDialog(Dialog):
    TITLE = '^What.s new$'


class FindBar:
    """The in-page search bar (a separate WebContentsView — its own CDP page)."""
    PAGE = '%3Cinput%20id%3D%22q%22'  # recognized by address (a data: page with the q field)

    def __init__(self, drv):
        self.drv = drv

    def wait_ready(self):
        # the bar's page appears in CDP before it is fully loaded
        assert wait_for(lambda: self.drv.page(self.PAGE) is not None and
                        self.drv.eval("!!document.getElementById('q') && typeof setCount === 'function'",
                                      page=self.PAGE), 10), 'the find bar did not open'
        return self

    def type(self, text):
        self.drv.eval("(() => { const q = document.getElementById('q'); q.value = %s; "
                      "q.dispatchEvent(new Event('input')); return 1; })()" % json.dumps(text), page=self.PAGE)

    def count(self):
        """The match counter: “2 / 3”, “none”, or ''."""
        return self.drv.eval("document.getElementById('n').textContent", page=self.PAGE) or ''

    def wait_count(self, cond, timeout=5):
        return wait_for(lambda: cond(self.count()), timeout, 0.25)

    def search(self, text, expect, tries=3):
        """Type the text and wait for the counter for which expect(counter) is true.
        The first search right after opening the bar sometimes answers “none” —
        we repeat the input with a pause. The field is NOT cleared before a retry:
        an empty string stops the search (stopFindInPage), and that can
        asynchronously cancel the next request."""
        # the bar was just opened: wait for the focus to be in the search field
        assert wait_for(lambda: self.drv.eval("document.activeElement && document.activeElement.id === 'q'",
                                              page=self.PAGE), 5, 0.1), 'focus is not in the search field'
        for _ in range(tries):
            self.type(text)
            if self.wait_count(expect, 4):
                return True
        return False

    def next(self):
        self.drv.go('dshlauncher://find/next/', page=self.PAGE)

    def prev(self):
        self.drv.go('dshlauncher://find/prev/', page=self.PAGE)

    def close(self):
        self.drv.go('dshlauncher://find/close/', page=self.PAGE)


class UpdatePrompt:
    """The update prompt (a helper page of the main window)."""

    def __init__(self, drv):
        self.drv = drv

    def wait(self, pattern, timeout=60):
        return self.drv.wait_text(pattern, timeout)

    def text(self):
        return self.drv.text()

    def url(self):
        return self.drv.url()

    def mark(self):
        self.drv.eval('window.__mark = 42; 1')

    def not_reloaded(self):
        return self.drv.eval('window.__mark') == 42

    def click_release_link(self):
        self.drv.eval("document.querySelector('a[target=_blank]').click()")

    def install(self):
        self.drv.go('dshlauncher://update/install/')

    def later(self):
        self.drv.go('dshlauncher://update/later/')

    def never(self):
        self.drv.go('dshlauncher://update/never/')

    def recheck(self):
        self.drv.go('dshlauncher://update/recheck/')


class MarketPrompt(UpdatePrompt):
    """The “Install dshmarket?” prompt (a helper page of the main window)."""

    def install(self):
        self.drv.go('dshlauncher://market/install/')

    def skip(self):
        self.drv.go('dshlauncher://market/skip/')

    def never(self):
        self.drv.go('dshlauncher://market/never/')


class PastePage(UpdatePrompt):
    """“A token is needed”: dsh is already running, but the window has no session cookie."""

    def paste(self, value):
        """Paste a link or token into the field and press Enter — like a user.
        Enter is a synthetic event; under CI load the navigation after it
        could be lost (a 26.04 flake): we check that the window left the paste page,
        — if it did not, we repeat."""
        for _ in range(3):
            start = self.drv.url()
            self.drv.eval(f"(() => {{ const el = document.getElementById('u'); el.value = {json.dumps(value)};"
                          " el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' })); return 1; })()")
            time.sleep(1.5)
            if self.drv.url() != start:
                return
