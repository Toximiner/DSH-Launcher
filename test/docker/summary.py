#!/usr/bin/env python3
"""Markdown table of Docker scenario results, built from pytest JUnit XML.

  summary.py <results dir>   — prints the table; run.sh on CI appends it to
                                $GITHUB_STEP_SUMMARY (the run summary).
"""
import glob
import os
import sys
import xml.etree.ElementTree as ET


def results(folder):
    rows = []
    for path in sorted(glob.glob(os.path.join(folder, '*.xml'))):
        name = os.path.basename(path)[:-4]           # <os>-<scenario>
        try:
            cases = list(ET.parse(path).getroot().iter('testcase'))
        except ET.ParseError:
            cases = []
        if not cases:
            rows.append((name, '❌', '', 'pytest did not report (see the log)'))
            continue
        for case in cases:
            fail = case.find('failure') if case.find('failure') is not None else case.find('error')
            skipped = case.find('skipped') is not None
            msg = ''
            if fail is not None:
                msg = (fail.get('message') or fail.text or '').strip().splitlines()[0][:200]
            mark = '❌' if fail is not None else ('⏭' if skipped else '✅')
            rows.append((name, mark, f'{float(case.get("time", 0)):.0f} s', msg))
    return rows


def main(folder):
    rows = results(folder)
    passed = sum(1 for r in rows if r[1] == '✅')
    print(f'### Docker scenarios: {passed}/{len(rows)} passed\n')
    print('| Scenario | | Time | Error |')
    print('|---|---|---|---|')
    for name, mark, took, msg in rows:
        cell = msg.replace('|', '\\|')  # a “|” inside a Markdown table cell
        print(f'| `{name}` | {mark} | {took} | {cell} |')


if __name__ == '__main__':
    main(sys.argv[1] if len(sys.argv) > 1 else '.')
