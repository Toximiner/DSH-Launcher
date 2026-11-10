#!/usr/bin/python3
"""Deterministic installed-profile and session API fixture; never calls a model."""
import json
import os
from pathlib import Path
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

args = sys.argv[1:]
root = Path(os.environ['DSH_TEST_ROOT'])
profile = args[args.index('--profile') + 1]
if args[0] == 'plugin':
    assert args == ['plugin', '--profile', profile, 'add', 'dshmarket']
    manifest = Path(os.environ['DSH_HOME']) / 'profiles' / profile / 'package.json'
    manifest.parent.mkdir(parents=True, exist_ok=True)
    manifest.write_text(json.dumps({'dependencies': {'dshmarket': 'test'}}))
    print('Test marketplace installed', flush=True)
    sys.exit(0)
port = int(args[args.index('--port') + 1])
state = {'plan': False, 'goal': None, 'plugins': {'alpha-plugin': '1.0.0'}}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        if self.path.startswith('/dsh-market/'):
            values = {
                '/dsh-market/api/v1/capabilities': {'apiVersion': 1, 'profile': profile, 'marketVersion': '1.73.17'},
                '/dsh-market/installed': {'installed': state['plugins'], 'present': list(state['plugins'])},
                '/dsh-market/status': {'busy': False, 'active': False},
                '/dsh-market/registry': {'registry': {'plugins': [
                    {'name': 'Alpha', 'npm': 'alpha-plugin', 'url': 'https://github.com/test/alpha', 'description': {'en': 'Alpha plugin'}},
                    {'name': 'Beta', 'npm': 'beta-plugin', 'url': 'https://github.com/test/beta', 'description': {'en': 'Beta plugin'}}
                ]}},
            }
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps(values[self.path]).encode())
            return
        self.send_response(302)
        self.send_header('Set-Cookie', 'dsh=test; Path=/')
        self.send_header('Location', '/')
        self.end_headers()

    def do_POST(self):
        request = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        if self.path.startswith('/dsh-market/'):
            operation = self.path.rsplit('/', 1)[-1]
            with (root / 'market-writes.jsonl').open('a') as log:
                log.write(json.dumps({'operation': operation, 'body': request}) + '\n')
            if operation == 'install':
                assert request == {'url': 'https://github.com/test/beta'}
                state['plugins']['beta-plugin'] = '1.0.0'
            elif operation == 'update':
                assert request == {'name': 'beta-plugin'}
                state['plugins']['beta-plugin'] = '2.0.0'
            elif operation == 'uninstall':
                assert request == {'name': 'beta-plugin'}
                del state['plugins']['beta-plugin']
            else:
                raise AssertionError('Unexpected mutation')
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(b'{"ok":true}')
            return
        method = request['method']
        arguments = request['payload']['args']
        with (root / 'requests.jsonl').open('a') as log:
            log.write(json.dumps({'method': method, 'args': arguments}) + '\n')
        ok, value = True, None
        if method == 'settings/describe':
            value = {'namespaces': []}
        elif method == 'session/create':
            value = {'sessionId': 'session-pty'}
        elif method == 'session/list':
            value = {'items': [{'sessionId': 'session-pty'}]}
        elif method == 'session/projections':
            value = {'kind': 'sequenced', 'values': {
                'plan': {'active': state['plan'], 'pending': False}, 'goal': state['goal']}}
        elif method == 'session/state':
            value = {'running': False}
        elif method == 'commands/execute':
            assert arguments['agentId'] == 'session-pty'
            assert arguments['line'] in ('/plan', '/plan off')
            state['plan'] = arguments['line'] == '/plan'
            value = {'result': {'kind': 'success'}}
        elif method == 'goals/create':
            state['goal'] = {'goal': {'id': 'goal-pty', 'revision': 1, 'phase': 'active',
                                      'objective': arguments['request']['objective']}, 'roundsStarted': 0}
        elif method in ('goals/pause', 'goals/resume', 'goals/clear'):
            goal = state['goal']['goal']
            assert arguments['ref'] == {'id': goal['id'], 'revision': goal['revision']}
            goal['revision'] += 1
            if method == 'goals/clear':
                state['goal'] = None
            else:
                goal['phase'] = 'paused' if method == 'goals/pause' else 'active'
        else:
            ok = False
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        self.wfile.write(json.dumps({'type': 'server-response', 'rpcId': request['rpcId'],
                                    'result': {'ok': ok, 'value': value}}).encode())


server = HTTPServer(('127.0.0.1', port), Handler)
print(f'dsh web: http://127.0.0.1:{port}/?token=test-only', flush=True)
server.serve_forever()
