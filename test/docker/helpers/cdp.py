"""Launcher window driver over Chrome DevTools Protocol (CDP).

The launcher is started with --remote-debugging-port=9222; each window (the
main one, the dialog window, the search panel) is a separate “page” in the
/json/list listing. A page is selected by a regex on the title or URL
(page=...); without it — the first one in the list.
"""
import html
import json
import re
import time
import urllib.parse
import urllib.request

import websocket  # python3-websocket (websocket-client)

from .env import ROUTE_KEY

CDP = 'http://127.0.0.1:9222'


def _page_text(url):
    """Text of a helper page (data: URL) — for content checks;
    for a regular page — the URL itself."""
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
        """The page, after waiting for it: during a transition (e.g. token exchange for
        a cookie → the clean GUI address) a window with the needed address is briefly
        absent from the list."""
        end = time.time() + timeout
        while True:
            p = self.page(page)
            if p or time.time() >= end:
                return p
            time.sleep(0.25)

    def _call(self, calls, page=None):
        p = self.wait_page(page)
        if not p:
            raise AssertionError(f'CDP: no page {page!r}')
        # We do not send Origin: Chromium rejects DevTools WebSockets with a foreign Origin.
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
            return None  # the page closed / navigated to another address without answering (window.close())
        finally:
            ws.close()

    def eval(self, expr, page=None):
        """Run JS on the page and return the value (None — no value / an error)."""
        r = self._call([('Runtime.evaluate', {'expression': expr, 'returnByValue': True})], page)
        return ((r or {}).get('result') or {}).get('result', {}).get('value')

    def go(self, url, page=None, keyed=True):
        """Navigate the page to a URL (dshlauncher://… routes: with the launch key,
        keyed=False — without it, like on a foreign page)."""
        if keyed and url.startswith('dshlauncher://'):
            url = f'dshlauncher://{ROUTE_KEY}/' + url[len('dshlauncher://'):]
        self.eval(f'(() => {{ location.href = {json.dumps(url)}; return 1; }})()', page)

    def _wait(self, get, pattern, timeout):
        rx = re.compile(pattern)
        last = ''
        err = ''
        end = time.time() + timeout
        while time.time() < end:
            try:
                last = get()
                err = ''
            except Exception as e:  # the page is reloading right now
                last = ''
                err = f'{type(e).__name__}: {e}'
            if rx.search(last):
                return last
            time.sleep(0.5)
        tail = f' (error: {err[:200]})' if err else ''
        raise AssertionError(f'timed out waiting for /{pattern}/ within {timeout} s; last: {last[:600]}{tail}')

    def wait_text(self, pattern, timeout=30, page=None):
        return self._wait(lambda: self.text(page), pattern, timeout)

    def wait_url(self, pattern, timeout=30, page=None):
        return self._wait(lambda: self.url(page), pattern, timeout)

    def right_click(self, x, y, page=None):
        """A real mouse event — Chromium treats it as a user click
        (for the right button a context-menu comes)."""
        ev = lambda t: ('Input.dispatchMouseEvent', {'type': t, 'x': x, 'y': y, 'button': 'right', 'buttons': 2, 'clickCount': 1})
        r = self._call([('Input.dispatchMouseEvent', {'type': 'mouseMoved', 'x': x, 'y': y}),
                        ev('mousePressed'), ev('mouseReleased')], page)
        assert r is None or 'error' not in r, f'CDP: right click did not go through: {r}'

    def key(self, spec, page=None):
        """Key press: “Ctrl+Shift+KeyF”, “F3”, “Escape”; the last part is
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
            key, vk = {'Escape': ('Escape', 27), 'Enter': ('Enter', 13),
                       'ArrowUp': ('ArrowUp', 38), 'ArrowDown': ('ArrowDown', 40),
                       'Equal': ('=', 187), 'Minus': ('-', 189)}.get(code, (code, 0))
        ev = lambda t: ('Input.dispatchKeyEvent', {'type': t, 'modifiers': modifiers, 'code': code, 'key': key,
                                                   'windowsVirtualKeyCode': vk, 'nativeVirtualKeyCode': vk})
        r = self._call([ev('rawKeyDown'), ev('keyUp')], page)
        # None — the page closed without answering (Esc in the dialog window closes it
        # already on keydown); that is not an error
        assert r is None or 'error' not in r, f'CDP: key press did not go through: {r}'
