"""Exercise phone sizing and touch controls against the real WASM preview."""
import argparse
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
    for width, height in [(320, 568), (360, 640), (390, 844), (430, 932)]:
        context = browser.new_context(viewport={'width': width, 'height': height},
                                      is_mobile=True, has_touch=True, device_scale_factor=2)
        page = context.new_page()
        errors = []
        page.on('pageerror', lambda error: errors.append(str(error)))
        page.goto(args.url, wait_until='domcontentloaded', timeout=20000)
        page.wait_for_function('window.dshPreviewReady === true', timeout=45000)
        page.wait_for_function('previewTerminal.cols >= 44 && previewTerminal.rows >= 18')
        page.locator('[data-page="2"]').tap()
        page.wait_for_function('() => ' + SCREEN + '.includes("deepseek-flash")')
        assert 'Увеличьте терминал' not in page.evaluate(SCREEN)
        assert page.locator('[data-page="2"]').get_attribute('aria-pressed') == 'true'
        page.locator('[data-key="right"]').tap()
        page.wait_for_function('() => ' + SCREEN + '.includes("maxParallelToolCalls")')
        page.locator('[data-page="3"]').tap()
        page.wait_for_function('() => ' + SCREEN + '.includes("Мышь в интерфейсе")')
        font = page.evaluate('previewTerminal.options.fontSize')
        page.locator('#zoom-in').tap()
        assert page.evaluate('previewTerminal.options.fontSize') > font
        page.locator('#zoom-out').tap()
        page.locator('#fit').tap()
        page.wait_for_function('previewTerminal.cols >= 44 && previewTerminal.rows >= 18')
        page.wait_for_function('document.querySelector(".xterm-screen").getBoundingClientRect().width <= document.getElementById("terminal").clientWidth + 1')
        page.locator('[data-page="2"]').tap()
        if args.screenshot and width == 390:
            page.screenshot(path=args.screenshot)
        page.set_viewport_size({'width': width, 'height': 380})
        page.locator('#fit').tap()
        page.wait_for_function('previewTerminal.cols >= 44 && previewTerminal.rows >= 18')
        assert 'Увеличьте терминал' not in page.evaluate(SCREEN)
        page.set_viewport_size({'width': width, 'height': height})
        page.locator('[data-page="5"]').tap()
        page.wait_for_function('() => ' + SCREEN + '.includes("Маркет · Каталог")')
        box = page.locator('.xterm-screen').bounding_box()
        columns, rows = page.evaluate('[previewTerminal.cols,previewTerminal.rows]')
        page.touchscreen.tap(box['x'] + 5.5 * box['width'] / columns,
                             box['y'] + 6.5 * box['height'] / rows)
        page.keyboard.type('prompt')
        page.keyboard.press('Enter')
        page.wait_for_function('() => ' + SCREEN + '.includes("dsh-prompt-polish")')
        assert not errors, errors
        print(f'Phone {width}x{height}: auto fit, zoom, touch pages and settings, keyboard-sized viewport OK')
        context.close()
    browser.close()
