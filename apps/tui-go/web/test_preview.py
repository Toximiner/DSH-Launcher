"""Smoke-test the actual WebAssembly UI in Chromium."""
import argparse
import json
import shutil

from playwright.sync_api import sync_playwright

parser = argparse.ArgumentParser()
parser.add_argument('--url', default='http://127.0.0.1:7682/')
parser.add_argument('--screenshot')
args = parser.parse_args()

SCREEN = 'Array.from({length:previewTerminal.rows},(_,i)=>previewTerminal.buffer.active.getLine(i)?.translateToString(true)||"").join("\\n")'

with sync_playwright() as playwright:
    browser = playwright.chromium.launch(
        executable_path=shutil.which('chromium') or shutil.which('google-chrome'),
        headless=True, args=['--no-sandbox', '--disable-dev-shm-usage'], timeout=20000)
    page = browser.new_page(viewport={'width': 1280, 'height': 850})
    errors = []
    page.on('pageerror', lambda error: errors.append(str(error)))
    page.goto(args.url, wait_until='domcontentloaded', timeout=20000)
    page.wait_for_function('window.dshPreviewReady === true', timeout=45000)

    def visible(text):
        page.wait_for_function('() => ' + SCREEN + '.includes(' + json.dumps(text) + ')')

    def click_cell(column, row):
        box = page.locator('.xterm-screen').bounding_box()
        columns, rows = page.evaluate('[previewTerminal.cols,previewTerminal.rows]')
        page.mouse.click(box['x'] + (column + 0.5) * box['width'] / columns,
                         box['y'] + (row + 0.5) * box['height'] / rows)

    page.locator('.xterm-helper-textarea').focus()
    page.keyboard.press('Tab')
    page.keyboard.press('ArrowDown')
    page.keyboard.press('ArrowDown')
    visible('deepseek-flash')
    if args.screenshot:
        page.screenshot(path=args.screenshot)
    click_cell(5, 10)
    visible('сведения о клиенте')
    page.keyboard.press('Escape')
    page.keyboard.type('Browser input test')
    page.keyboard.press('Enter')
    visible('Browser input test')
    page.set_viewport_size({'width': 620, 'height': 780})
    page.keyboard.press('Tab')
    page.keyboard.press('ArrowDown')
    page.keyboard.press('ArrowDown')
    visible('deepseek-flash')
    page.set_viewport_size({'width': 1280, 'height': 850})
    page.evaluate('dshTui.page(5)')
    page.locator('.xterm-helper-textarea').focus()
    page.keyboard.press('Escape')
    page.keyboard.type('/market')
    page.keyboard.press('Enter')
    visible('Маркет · Каталог')
    page.keyboard.press('/')
    page.keyboard.type('prompt')
    page.keyboard.press('Enter')
    visible('dsh-prompt-polish')
    page.keyboard.press('i')
    visible('[y Подтвердить]')
    page.keyboard.press('y')
    visible('Демо: операции')
    assert not errors, errors
    print('WASM browser: settings, keyboard, mouse, input, resize OK; page errors: 0')
    browser.close()
