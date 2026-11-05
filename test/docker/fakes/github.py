#!/usr/bin/python3
"""Fake GitHub for the launcher update and rollback scenarios: the launcher
goes here instead of api.github.com (DSH_LAUNCHER_GITHUB_API).
  github.py <port> <releases list file> <.deb directory> [dsh releases file]

  GET /repos/<owner>/<repo>/releases?per_page=N&page=P — the release list from
      the file (read on every request — a scenario may replace it);
      30 per page by default, like GitHub; ETag / If-None-Match → 304;
      deepseek-ai/deepseek-harness (dsh releases) — from the 4th file, if it
      exists (a scenario may create it), otherwise from the main one;
  GET /download/<file> — 302 to /files/<file> (GitHub assets are also served
      via redirect);
  GET /files/<file> — a .deb from the directory.
Every request — a line in /tmp/github.log.
"""
import hashlib
import http.server
import json
import os
import sys
import time
import urllib.parse

LOG = '/tmp/github.log'
PORT, RELEASES, DEBS = int(sys.argv[1]), sys.argv[2], sys.argv[3]
DSH_RELEASES = sys.argv[4] if len(sys.argv) > 4 else None  # dsh releases (separate repository)
DSH_REPO = ('deepseek-ai', 'deepseek-harness')


def log(msg):
    with open(LOG, 'a') as f:
        f.write(f'{int(time.time() * 1000)} {msg}\n')


class Handler(http.server.BaseHTTPRequestHandler):
    def reply(self, code, body=b'', headers=None):
        self.send_response(code)
        for k, v in (headers or {}).items():
            self.send_header(k, v)
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)
        log(f'GET {self.path} {code}')

    def do_GET(self):
        u = urllib.parse.urlparse(self.path)
        q = urllib.parse.parse_qs(u.query)
        parts = u.path.strip('/').split('/')
        if len(parts) == 4 and parts[0] == 'repos' and parts[3] == 'releases':
            src = RELEASES
            if (parts[1], parts[2]) == DSH_REPO and DSH_RELEASES and os.path.isfile(DSH_RELEASES):
                src = DSH_RELEASES
            with open(src) as f:
                releases = json.load(f)
            per_page = min(int(q.get('per_page', ['30'])[0]), 100)
            page = int(q.get('page', ['1'])[0])
            body = json.dumps(releases[(page - 1) * per_page:page * per_page]).encode()
            etag = '"' + hashlib.sha1(body).hexdigest() + '"'
            if self.headers.get('If-None-Match') == etag:
                return self.reply(304, headers={'ETag': etag})
            return self.reply(200, body, {'Content-Type': 'application/json', 'ETag': etag})
        if len(parts) == 2 and parts[0] == 'download':
            return self.reply(302, headers={'Location': f'/files/{parts[1]}'})
        if len(parts) == 2 and parts[0] == 'files' and '/' not in parts[1] and not parts[1].startswith('.'):
            path = os.path.join(DEBS, parts[1])
            if os.path.isfile(path):
                with open(path, 'rb') as f:
                    return self.reply(200, f.read(), {'Content-Type': 'application/octet-stream'})
        self.reply(404)

    def log_message(self, *args):
        pass


server = http.server.ThreadingHTTPServer(('127.0.0.1', PORT), Handler)
log('listening')
server.serve_forever()
