"""Драйвер окна лаунчера через Chrome DevTools Protocol (CDP).

Лаунчер запускается с --remote-debugging-port=9222; каждое окно (главное,
окно-диалог, панель поиска) — отдельная «страница» в списке /json/list.
Страницу выбираем регулярным выражением по заголовку или URL (page=...);
без него — первая в списке.
"""
import html
import json
import re
import time
import urllib.parse
import urllib.request

import websocket  # python3-websocket (websocket-client)

CDP = 'http://127.0.0.1:9222'


def _page_text(url):
    """Текст служебной страницы (data:-URL) — для проверок по содержимому;
    для обычной страницы — сам URL."""
    if not url.startswith('data:'):
        return url
    doc = urllib.parse.unquote(url[url.index(',') + 1:])
    doc = re.sub(r'<style[\s\S]*?</style>', '', doc)
    doc = re.sub(r'<script[\s\S]*?</script>', '', doc)
    doc = html.unescape(re.sub(r'<[^>]+>', ' ', doc))
    return re.sub(r'\s+', ' ', doc).strip()


class Driver:
    def __init__(self, base=CDP):
        self.base = base

    def pages(self):
        try:
            with urllib.request.urlopen(f'{self.base}/json/list', timeout=5) as r:
                return [t for t in json.load(r) if t.get('type') == 'page']
        except OSError:
            return []

    def page(self, page=None):
        lst = self.pages()
        if page is None:
            return lst[0] if lst else None
        rx = re.compile(page)
        return next((t for t in lst if rx.search(t.get('title', '')) or rx.search(t.get('url', ''))), None)

    def url(self, page=None):
        p = self.page(page)
        return p['url'] if p else ''

    def text(self, page=None):
        return _page_text(self.url(page))

    def wait_page(self, page=None, timeout=10):
        """Страница, дождавшись её: во время перехода (например, обмен токена на
        куки → чистый адрес GUI) окна с нужным адресом в списке на миг нет."""
        end = time.time() + timeout
        while True:
            p = self.page(page)
            if p or time.time() >= end:
                return p
            time.sleep(0.25)

    def _call(self, calls, page=None):
        p = self.wait_page(page)
        if not p:
            raise AssertionError(f'CDP: нет страницы {page!r}')
        # Origin не шлём: Chromium отклоняет WebSocket DevTools с чужим Origin.
        ws = websocket.create_connection(p['webSocketDebuggerUrl'], timeout=5, suppress_origin=True)
        try:
            res = None
            for i, (method, params) in enumerate(calls, 1):
                ws.send(json.dumps({'id': i, 'method': method, 'params': params}))
                while True:
                    msg = json.loads(ws.recv())
                    if msg.get('id') == i:
                        res = msg
                        break
            return res
        except (websocket.WebSocketException, OSError):
            return None  # страница закрылась / ушла на другой адрес, не ответив (window.close())
        finally:
            ws.close()

    def eval(self, expr, page=None):
        """Выполнить JS на странице, вернуть значение (None — нет значения/ошибка)."""
        r = self._call([('Runtime.evaluate', {'expression': expr, 'returnByValue': True})], page)
        return ((r or {}).get('result') or {}).get('result', {}).get('value')

    def go(self, url, page=None):
        """Переход страницы на URL (маршруты dshlauncher://…)."""
        self.eval(f'(() => {{ location.href = {json.dumps(url)}; return 1; }})()', page)

    def _wait(self, get, pattern, timeout):
        rx = re.compile(pattern)
        last = ''
        end = time.time() + timeout
        while time.time() < end:
            try:
                last = get()
            except Exception:  # страница как раз перезагружается
                last = ''
            if rx.search(last):
                return last
            time.sleep(0.5)
        raise AssertionError(f'не дождались /{pattern}/ за {timeout} с; последнее: {last[:600]}')

    def wait_text(self, pattern, timeout=30, page=None):
        return self._wait(lambda: self.text(page), pattern, timeout)

    def wait_url(self, pattern, timeout=30, page=None):
        return self._wait(lambda: self.url(page), pattern, timeout)

    def right_click(self, x, y, page=None):
        """Настоящее событие мыши — Chromium обрабатывает как клик пользователя
        (для правой кнопки приходит context-menu)."""
        ev = lambda t: ('Input.dispatchMouseEvent', {'type': t, 'x': x, 'y': y, 'button': 'right', 'buttons': 2, 'clickCount': 1})
        r = self._call([('Input.dispatchMouseEvent', {'type': 'mouseMoved', 'x': x, 'y': y}),
                        ev('mousePressed'), ev('mouseReleased')], page)
        assert r and 'error' not in r, f'CDP: правый клик не прошёл: {r}'

    def key(self, spec, page=None):
        """Нажатие: «Ctrl+Shift+KeyF», «F3», «Escape»; последняя часть —
        KeyboardEvent.code."""
        *mods, code = spec.split('+')
        modifiers = sum({'Alt': 1, 'Ctrl': 2, 'Meta': 4, 'Shift': 8}.get(m, 0) for m in mods)
        key, vk = code, 0
        if re.fullmatch(r'Key[A-Z]', code):
            key, vk = code[3].lower(), ord(code[3])
        elif re.fullmatch(r'Digit\d', code):
            key, vk = code[5], ord(code[5])
        elif re.fullmatch(r'F\d{1,2}', code):
            vk = 111 + int(code[1:])
        else:
            vk = {'Escape': 27, 'Enter': 13}.get(code, 0)
        ev = lambda t: ('Input.dispatchKeyEvent', {'type': t, 'modifiers': modifiers, 'code': code, 'key': key,
                                                   'windowsVirtualKeyCode': vk, 'nativeVirtualKeyCode': vk})
        r = self._call([ev('rawKeyDown'), ev('keyUp')], page)
        assert r and 'error' not in r, f'CDP: нажатие не прошло: {r}'
