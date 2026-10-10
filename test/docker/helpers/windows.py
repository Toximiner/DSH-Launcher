"""Объекты окон лаунчера (page objects): тест говорит, ЧТО делает, а как —
маршруты dshlauncher://…, CDP, разметка — спрятано здесь.

Окна меню открываются по маршрутам (по нативному меню driver кликать не
умеет); маршрут отвечает 204 — страница, с которой перешли, остаётся на месте.
"""
import json
import time

from .env import GUI_URL
from .procs import wait_for


class Gui:
    """Главное окно с GUI dsh."""

    def __init__(self, drv):
        self.drv = drv

    def wait(self, timeout=60):
        """Дождаться загрузки GUI dsh (окно ищем по адресу: первым в списке CDP
        может оказаться окно-диалог)."""
        self.drv.wait_url(GUI_URL, timeout, page=GUI_URL)
        return self

    def eval(self, js):
        return self.drv.eval(js, page=GUI_URL)

    def go(self, route):
        self.drv.go(route, page=GUI_URL)

    def close(self):
        """Закрыть окно из страницы (window.close())."""
        self.drv.eval('window.close()', page=GUI_URL)

    # ---- состояние страницы ----

    def mark(self):
        """Пометить страницу — not_reloaded() потом покажет, что её не перезагружали."""
        self.eval('window.__mark = 42; 1')

    def not_reloaded(self):
        return self.eval('window.__mark') == 42

    def dimmed(self):
        """Затемнена ли страница слоем под окном-диалогом."""
        bg = self.eval("getComputedStyle(document.documentElement, '::after').backgroundColor") or ''
        return '0.55' in bg

    def outer_width(self):
        return self.eval('window.outerWidth')

    # ---- содержимое страницы для проверок ----

    def set_body(self, html):
        self.eval(f'document.body.innerHTML = {json.dumps(html)}; 1')

    def select_text_of(self, element_id):
        self.eval(f"""(() => {{ const r = document.createRange();
            r.selectNodeContents(document.getElementById({json.dumps(element_id)}));
            getSelection().removeAllRanges(); getSelection().addRange(r); return 1; }})()""")

    def select_input(self, element_id):
        self.eval(f"(() => {{ const i = document.getElementById({json.dumps(element_id)}); i.focus(); i.select(); return 1; }})()")

    def clear_selection(self):
        self.eval('getSelection().removeAllRanges(); document.activeElement && document.activeElement.blur(); 1')

    def right_click(self, x, y):
        self.drv.right_click(x, y, page=GUI_URL)

    def press(self, key):
        """Нажатие клавиши в главном окне (например, «Ctrl+Equal»)."""
        self.drv.key(key, page=GUI_URL)

    # ---- окна меню ----

    def open_about(self):
        self.go('dshlauncher://menu/about/')
        return AboutDialog(self.drv).wait_loaded()

    def open_update_check(self):
        self.go('dshlauncher://menu/check/')
        return UpdateCheckDialog(self.drv).wait_result()

    def open_dsh_log(self):
        self.go('dshlauncher://menu/log/')
        return LogDialog(self.drv).wait_loaded()

    def open_find(self):
        self.drv.key('Ctrl+KeyF', page=GUI_URL)
        return FindBar(self.drv).wait_ready()

    def restart_dsh(self):
        """«Файл → Перезапустить dsh» без вопроса-подтверждения."""
        self.go('dshlauncher://menu/restart-dsh/')


class Dialog:
    """Окно-диалог поверх главного (отдельная страница CDP с заголовком TITLE)."""
    TITLE = None  # регулярное выражение заголовка

    def __init__(self, drv):
        self.drv = drv

    def text(self):
        return self.drv.text(page=self.TITLE)

    def wait_text(self, pattern, timeout=30):
        return self.drv.wait_text(pattern, timeout, page=self.TITLE)

    def is_open(self):
        return self.drv.page(self.TITLE) is not None

    def go(self, route):
        self.drv.go(route, page=self.TITLE)

    def rows(self):
        """Таблица окна: {подпись: значение}."""
        raw = self.drv.eval("JSON.stringify([...document.querySelectorAll('tr')]"
                            ".map(r => [...r.cells].map(c => c.innerText.trim())))", page=self.TITLE)
        return {r[0]: r[1] for r in json.loads(raw or '[]') if len(r) >= 2}

    def close(self):
        self.go('dshlauncher://dialog/close/')
        assert wait_for(lambda: not self.is_open(), 10), f'окно {self.TITLE} не закрылось'

    def press(self, key):
        """Нажатие клавиши в окне (например, «Escape»)."""
        self.drv.key(key, page=self.TITLE)


class AboutDialog(Dialog):
    TITLE = '^About$'

    def wait_loaded(self):
        self.wait_text('Copy for report', 30)  # кнопка появляется, когда версии загружены
        return self

    def value(self, label):
        return self.rows().get(label)

    def copy_for_report(self):
        self.go('dshlauncher://dialog/copy/')
        self.wait_text('Copied', 10)

    def reenable_gpu(self):
        """«Включить GPU снова» (кнопка есть, только если GPU отключён после сбоев)."""
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
        assert wait_for(lambda: self.is_open() and self.log_text() is not None, 15), 'окно журнала не открылось'
        return self

    def log_text(self):
        return self.drv.eval("(document.querySelector('pre.log') || {}).innerText", page=self.TITLE)


class WhatsNewDialog(Dialog):
    TITLE = '^What.s new$'


class FindBar:
    """Панель поиска по странице (отдельный WebContentsView — своя страница CDP)."""
    PAGE = '%3Cinput%20id%3D%22q%22'  # узнаём по адресу (data:-страница с полем q)

    def __init__(self, drv):
        self.drv = drv

    def wait_ready(self):
        # страница панели появляется в CDP раньше, чем догружается
        assert wait_for(lambda: self.drv.page(self.PAGE) is not None and
                        self.drv.eval("!!document.getElementById('q') && typeof setCount === 'function'",
                                      page=self.PAGE), 10), 'панель поиска не открылась'
        return self

    def type(self, text):
        self.drv.eval("(() => { const q = document.getElementById('q'); q.value = %s; "
                      "q.dispatchEvent(new Event('input')); return 1; })()" % json.dumps(text), page=self.PAGE)

    def count(self):
        """Счётчик совпадений: «2 / 3», «none» или ''."""
        return self.drv.eval("document.getElementById('n').textContent", page=self.PAGE) or ''

    def wait_count(self, cond, timeout=5):
        return wait_for(lambda: cond(self.count()), timeout, 0.25)

    def search(self, text, expect, tries=3):
        """Ввести текст и дождаться счётчика, для которого expect(счётчик) истинно.
        Первый поиск сразу после открытия панели иногда отвечает «none» —
        повторяем ввод с паузой. Поле перед повтором НЕ очищаем: пустая строка
        останавливает поиск (stopFindInPage), и это асинхронно может отменить
        следующий запрос."""
        time.sleep(1)  # панель только что открылась — дать ей устояться
        for _ in range(tries):
            self.type(text)
            if self.wait_count(expect, 4):
                return True
            time.sleep(1)
        return False

    def next(self):
        self.drv.go('dshlauncher://find/next/', page=self.PAGE)


class UpdatePrompt:
    """Вопрос об обновлении (служебная страница главного окна)."""

    def __init__(self, drv):
        self.drv = drv

    def wait(self, pattern, timeout=60):
        return self.drv.wait_text(pattern, timeout)

    def text(self):
        return self.drv.text()

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
