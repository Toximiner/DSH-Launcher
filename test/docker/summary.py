#!/usr/bin/env python3
"""Таблица результатов Docker-сценариев (Markdown) из JUnit XML pytest.

  summary.py <каталог results>   — печатает таблицу; run.sh на CI дописывает
                                   её в $GITHUB_STEP_SUMMARY (сводка прогона).
"""
import glob
import os
import sys
import xml.etree.ElementTree as ET


def results(folder):
    rows = []
    for path in sorted(glob.glob(os.path.join(folder, '*.xml'))):
        name = os.path.basename(path)[:-4]           # <os>-<сценарий>
        try:
            cases = list(ET.parse(path).getroot().iter('testcase'))
        except ET.ParseError:
            cases = []
        if not cases:
            rows.append((name, '❌', '', 'pytest не отчитался (см. лог)'))
            continue
        for case in cases:
            fail = case.find('failure') if case.find('failure') is not None else case.find('error')
            skipped = case.find('skipped') is not None
            msg = ''
            if fail is not None:
                msg = (fail.get('message') or fail.text or '').strip().splitlines()[0][:200]
            mark = '❌' if fail is not None else ('⏭' if skipped else '✅')
            rows.append((name, mark, f'{float(case.get("time", 0)):.0f} с', msg))
    return rows


def main(folder):
    rows = results(folder)
    passed = sum(1 for r in rows if r[1] == '✅')
    print(f'### Docker-сценарии: {passed}/{len(rows)} прошли\n')
    print('| Сценарий | | Время | Ошибка |')
    print('|---|---|---|---|')
    for name, mark, took, msg in rows:
        cell = msg.replace('|', '\\|')  # «|» внутри ячейки таблицы Markdown
        print(f'| `{name}` | {mark} | {took} | {cell} |')


if __name__ == '__main__':
    main(sys.argv[1] if len(sys.argv) > 1 else '.')
