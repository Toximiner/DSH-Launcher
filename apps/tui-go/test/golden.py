"""Golden-capture helpers for the TUI visual-rebuild baseline.

Shared by the offline (fake_dsh, no Node) and real (isolated dsh + local
model) capture drivers, and by render_golden.py which turns terminal-cell
JSON into PNG contact sheets for visual review.

A "capture" is the full terminal-cell buffer: every cell's text plus its
fg/bg/bold/reverse. JSON is the exchange format; PNG is the human-review
format. Run-varying values (random session ids, ports, temp roots, times,
token counters) are normalized so a diff means a *visual* change.

Normalization runs on reconstructed SCREEN ROWS, not on single cells: a
dump stores one character per cell, so patterns spanning several cells
(session ids, "15:04" clocks, "127.0.0.1:PORT", /tmp paths) can only be
matched after the row is joined. Every substitution is LENGTH-PRESERVING
(digit runs become 'x'-runs of the same count), so the text stays aligned
with its per-cell colors and the (x, y) grid.
"""
import json
import re
from pathlib import Path

# Random dsh session ids: the full UUID form and the truncated sidebar form
# ("session-ab12c3d…"). 'x' is excluded from the char classes so an
# already-normalized row survives a second pass untouched.
_SESSION_RE = re.compile(r'session-[a-z0-9]{8,}(-[a-z0-9]{3,}){0,4}')
_SESSION_TRUNC_RE = re.compile(r'session-[a-z0-9]{2,6}…')
_TEMP_RE = re.compile(r'/tmp/dsh-tui-[a-z0-9-]+|/tmp/tui-go-pty-[a-z0-9]+')
# IP-anchored so HH:MM clocks are never touched; only the port digits change.
_PORT_RE = re.compile(r'127\.0\.0\.1:(\d{2,5})')
_TIME_RE = re.compile(
    r'\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?'
    r'|\d{2}[./-]\d{2}[./-]\d{2,4}\s+\d{1,2}:\d{2}(?::\d{2})?'
)
# Status-bar values that vary with run timing (conversation_view.go:140-200):
#   "Ответ за 1.2 с", "42.5 ток/с", "123 ток", "кэш 87%", "контекст ≈42%",
#   "45 678 токенов". Only digit runs are masked; units stay.
_ELAPSED_RE = re.compile(r'Ответ за (\d+[\d.,]*) с')
_RATE_RE = re.compile(r'(\d+[\d.,]*) ток/с')
_TOK_RE = re.compile(r'\b(\d[\d\s.,]*?)\s*ток\b')
_COUNT_RE = re.compile(r'\b(\d[\d.,\s]{2,})\s*(?=токенов|токена|токен|tokens?\b)', re.I)
_CACHE_RE = re.compile(r'кэш (\d+[\d.]*)%')
_CTX_RE = re.compile(r'контекст ≈(\d+[\d.]*)%')
# messageClock "15:04" in message headers (HH:MM, 00-23 hours).
_CLOCK_RE = re.compile(r'\b(?:[01]?\d|2[0-3]):[0-5]\d\b')


def _xrun(match):
    return 'x' * (match.end() - match.start())


def _norm_row(row):
    """Normalize one screen row; the result always has the original length."""
    row = _SESSION_RE.sub(_xrun, row)
    row = _SESSION_TRUNC_RE.sub(_xrun, row)
    row = _TEMP_RE.sub(_xrun, row)
    row = _TIME_RE.sub(_xrun, row)
    row = _PORT_RE.sub(lambda m: '127.0.0.1:' + 'x' * len(m.group(1)), row)
    row = _ELAPSED_RE.sub(lambda m: 'Ответ за ' + 'x' * len(m.group(1)) + ' с', row)
    row = _RATE_RE.sub(lambda m: 'x' * len(m.group(1)) + ' ток/с', row)
    row = _TOK_RE.sub(lambda m: 'x' * len(m.group(1)) + ' ток', row)
    row = _COUNT_RE.sub(lambda m: 'x' * len(m.group(1)), row)
    row = _CACHE_RE.sub(lambda m: 'кэш ' + 'x' * len(m.group(1)) + '%', row)
    row = _CTX_RE.sub(lambda m: 'контекст ≈' + 'x' * len(m.group(1)) + '%', row)
    row = _CLOCK_RE.sub(_xrun, row)
    return row


def _rebuild_lines(data):
    """Join the one-char-per-cell dump into plain rows (first char only)."""
    rows, cols = data.get('rows', 0), data.get('columns', 0)
    lines = [' ' * cols for _ in range(rows)]
    for cell in data.get('cells', ()):
        text = cell.get('text') or ''
        x, y = cell.get('x', 0), cell.get('y', 0)
        if not (0 <= y < rows) or not (0 <= x < cols):
            continue
        lines[y] = lines[y][:x] + (text[0] if text else ' ') + lines[y][x + 1:]
    return lines


def normalize(data):
    """Strip run-varying values from a cell dump so diffs are visual-only."""
    lines = _rebuild_lines(data)
    norm_lines = [_norm_row(line) for line in lines]
    for cell in data.get('cells', ()):
        x, y = cell.get('x', 0), cell.get('y', 0)
        if len(cell.get('text') or '') == 1 and 0 <= y < len(norm_lines) and 0 <= x < len(norm_lines[y]):
            cell['text'] = norm_lines[y][x]
    return data


def capture_cells(app, path, source='dsh-launcher-tui-go'):
    """Record the current pyte screen to normalized terminal-cell JSON."""
    for _ in range(6):
        app.pump()
    cells = []
    for y in range(app.screen.lines):
        for x in range(app.screen.columns):
            cell = app.screen.buffer[y][x]
            cells.append({'x': x, 'y': y, 'text': cell.data, 'fg': cell.fg,
                          'bg': cell.bg, 'bold': cell.bold, 'reverse': cell.reverse})
    data = normalize({'source': source,
                      'columns': app.screen.columns, 'rows': app.screen.lines,
                      'cells': cells})
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(data, ensure_ascii=False), encoding='utf8')
    return target


def load(path):
    return json.loads(Path(path).read_text(encoding='utf8'))


def diff(path_a, path_b):
    """Changed cells between two normalized dumps; [] means visually equal."""
    a, b = load(path_a), load(path_b)
    if (a['columns'], a['rows']) != (b['columns'], b['rows']):
        return [{'kind': 'size', 'before': [a['columns'], a['rows']],
                 'after': [b['columns'], b['rows']]}]
    by_a = {(c['x'], c['y']): c for c in a['cells']}
    by_b = {(c['x'], c['y']): c for c in b['cells']}
    changed = []
    for pos in sorted(set(by_a) | set(by_b)):
        ca, cb = by_a.get(pos), by_b.get(pos)
        if ca is None or cb is None:
            changed.append({'kind': 'presence', 'x': pos[0], 'y': pos[1]})
            continue
        if any(ca[k] != cb[k] for k in ('text', 'fg', 'bg', 'bold', 'reverse')):
            changed.append({'x': pos[0], 'y': pos[1],
                            'before': {k: ca[k] for k in ('text', 'fg', 'bg', 'bold', 'reverse')},
                            'after': {k: cb[k] for k in ('text', 'fg', 'bg', 'bold', 'reverse')}})
    return changed


def screen_text(data):
    width = data['columns']
    lines = [' ' * width for _ in range(data['rows'])]
    for cell in data['cells']:
        lines[cell['y']] = lines[cell['y']][:cell['x']] + (cell['text'] or ' ') + lines[cell['y']][cell['x'] + 1:]
    return '\n'.join(line.rstrip() for line in lines)


# --- PNG rendering (PIL-based; render_golden.py requires Pillow) ----------

_BG = (16, 19, 24)
_FG = (230, 237, 243)
_XTERM256 = None


def _xterm256():
    global _XTERM256
    if _XTERM256 is not None:
        return _XTERM256
    base = [(0, 0, 0), (205, 0, 0), (0, 205, 0), (205, 205, 0), (205, 0, 238),
            (205, 0, 205), (0, 205, 205), (229, 229, 229), (127, 127, 127),
            (255, 0, 0), (255, 255, 0), (92, 92, 255),
            (255, 0, 255), (255, 255, 255), (255, 255, 255)]
    levels = [0, 95, 135, 175, 215, 255]
    table = base + [(r, g, b) for r in levels for g in levels for b in levels]
    table += [(8 + v * 10,) * 3 for v in range(242)]
    _XTERM256 = table
    return table


def cell_color(value, default):
    if value == 'default':
        return default
    if isinstance(value, str) and re.fullmatch(r'[0-9a-fA-F]{6}', value):
        return (int(value[0:2], 16), int(value[2:4], 16), int(value[4:6], 16))
    if isinstance(value, int) and 0 <= value < 256:
        return _xterm256()[value]
    return default


def render_png(data, out, cw=10, ch=20, pad=16, font_size=15, label=''):
    """Render one cell dump to a PNG using PIL. Returns pixel (w, h)."""
    from PIL import Image, ImageDraw, ImageFont
    width, height = data['columns'], data['rows']
    try:
        font = ImageFont.truetype('/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf', font_size)
        bold = ImageFont.truetype('/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf', font_size)
    except OSError:
        font = ImageFont.load_default()
        bold = font
    label_h = (len(label) * ch) if label else 0
    W = width * cw + 2 * pad
    H = height * ch + 2 * pad + label_h
    img = Image.new('RGB', (W, H), _BG)
    d = ImageDraw.Draw(img)
    if label:
        d.text((pad, 4), label, font=bold, fill=(139, 148, 158))
    for cell in data['cells']:
        x0 = pad + cell['x'] * cw
        y0 = pad + label_h + cell['y'] * ch
        fg = cell_color(cell['fg'], _FG)
        bg = cell_color(cell['bg'], _BG)
        if cell['reverse']:
            fg, bg = bg, fg
        d.rectangle((x0, y0, x0 + cw, y0 + ch - 1), fill=bg)
        if cell['text'] and cell['text'] != ' ':
            d.text((x0, y0 + 1), cell['text'], font=bold if cell['bold'] else font, fill=fg)
    Path(out).parent.mkdir(parents=True, exist_ok=True)
    img.save(out)
    return W, H


def contact_sheet(items, out, scale=1, max_cols=2):
    """Tile already-rendered PNGs into one sheet. items: [(label, path)]."""
    from PIL import Image
    images = [(label, Image.open(path).convert('RGB')) for label, path in items]
    tw = max(im.width for _, im in images)
    th = max(im.height for _, im in images)
    cols = min(max_cols, len(images)) if images else 1
    rows = (len(images) + cols - 1) // cols
    sheet = Image.new('RGB', (cols * (tw + 12) + 12, rows * (th + 12) + 12), (8, 10, 13))
    for i, (label, im) in enumerate(images):
        sheet.paste(im, (12 + (i % cols) * (tw + 12), 12 + (i // cols) * (th + 12)))
    sheet = sheet.resize((sheet.width // scale, sheet.height // scale)) if scale > 1 else sheet
    Path(out).parent.mkdir(parents=True, exist_ok=True)
    sheet.save(out)
    return out
