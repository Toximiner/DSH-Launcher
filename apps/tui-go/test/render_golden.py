"""Render golden cell dumps into labeled PNG contact sheets for review.

Usage:
  python3 render_golden.py            # all dumps in golden/before
  python3 render_golden.py offline    # only offline-* dumps
  python3 render_golden.py --scale 2  # larger font
"""
import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from golden import load, render_png, contact_sheet, screen_text  # noqa: E402

HERE = Path(__file__).resolve().parent
parser = argparse.ArgumentParser()
parser.add_argument('prefix', nargs='?', default=None, help='Only dumps starting with this prefix (e.g. offline, real)')
parser.add_argument('--scale', type=int, default=1, help='Enlarge output (1=normal)')
parser.add_argument('--font', type=int, default=15)
args = parser.parse_args()

dumps = sorted((HERE / 'golden' / 'before').glob('*.json'))
if args.prefix:
    dumps = [d for d in dumps if d.name.startswith(args.prefix)]
if not dumps:
    raise SystemExit(f'No dumps found under {HERE}/golden/before' + (f' matching {args.prefix!r}' if args.prefix else ''))

png_dir = HERE / 'golden' / 'review'
png_dir.mkdir(parents=True, exist_ok=True)

groups = {}
for dump in dumps:
    data = load(dump)
    key = (dump.name.split('-')[0], f"{data['columns']}x{data['rows']}")
    groups.setdefault(key, []).append((dump.stem, dump))

sheet_paths = []
for (group, size), items in sorted(groups.items()):
    rendered = []
    for stem, dump in items:
        label = stem.replace(group + '-', '')
        png = png_dir / f'{stem}.png'
        render_png(load(dump), png, cw=10, ch=20, pad=16, font_size=args.font, label=label)
        rendered.append((label, png))
    sheet = png_dir / f'sheet-{group}-{size}.png'
    contact_sheet(rendered, sheet, scale=max(1, args.scale))
    sheet_paths.append(sheet)
    print(f'{group} {size}: {len(items)} scenes -> {sheet.relative_to(HERE)}', flush=True)

# Also dump plain-text versions for quick grep/diff in review.
text_dir = png_dir / 'text'
text_dir.mkdir(exist_ok=True)
for dump in dumps:
    (text_dir / (dump.stem + '.txt')).write_text(screen_text(load(dump)) + '\n', encoding='utf8')
print(f'text views -> {text_dir.relative_to(HERE)}', flush=True)
for sheet in sheet_paths:
    print('SHEET', sheet)
