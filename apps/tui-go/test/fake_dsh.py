#!/usr/bin/python3
"""Deterministic installed-profile and session API fixture; never calls a model."""
import json
import os
from pathlib import Path
import sys
import base64
import hashlib
import socket
import struct
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

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
state = {'plan': False, 'goal': None, 'plugins': {'alpha-plugin': '1.0.0'},
         'history': [], 'streams': [], 'running': False, 'admitted': set()}
lock = threading.RLock()
cancel_generation = threading.Event()


def ws_write(stream, value):
    data = json.dumps(value, ensure_ascii=False).encode()
    header = bytes([0x81, len(data)]) if len(data) < 126 else bytes([0x81, 126]) + struct.pack('!H', len(data))
    try:
        with stream['lock']:
            stream['socket'].sendall(header + data)
    except OSError:
        pass


def ws_read(reader):
    header = reader.read(2)
    if len(header) != 2 or header[0] & 15 == 8:
        return None
    length = header[1] & 127
    if length == 126:
        length = struct.unpack('!H', reader.read(2))[0]
    elif length == 127:
        length = struct.unpack('!Q', reader.read(8))[0]
    assert length < 1024 * 1024
    mask = reader.read(4) if header[1] & 128 else None
    data = reader.read(length)
    if mask:
        data = bytes(value ^ mask[i % 4] for i, value in enumerate(data))
    return json.loads(data)


def publish(frame):
    with lock:
        streams = list(state['streams'])
    for stream in streams:
        ws_write(stream, {'type': 'item', 'streamId': stream['id'], 'value': frame})


def event(kind, data, surface=False):
    with lock:
        value = {'type': kind, 'seq': len(state['history']), 'time': 1, 'data': data}
        if surface:
            value['surfaceOp'] = 'append'
        state['history'].append({'type': 'event', 'event': value})
    publish({'type': 'event', 'event': value})
    return value['seq']


def generate_prompt(request):
    request_id = request['requestId']
    text = request['content'][0]['text']
    state['running'] = True
    event('turn/start', {'turn': 1})
    event('user/message', {'content': request['content'], 'source': {'kind': 'user', 'rpcId': request_id}}, True)
    publish({'type': 'assistant-stream', 'frame': {'type': 'start', 'attemptId': request_id, 'turn': 1, 'step': 0}})
    fragments = ['Потоковый ', 'ответ ', 'из тестового dsh']
    if 'cancel' in text:
        fragments = ['Частичный ответ до отмены']
    for i, fragment in enumerate(fragments):
        publish({'type': 'assistant-stream', 'frame': {'type': 'chunk', 'attemptId': request_id,
                 'index': i, 'chunk': {'type': 'text-delta', 'index': 0, 'text': fragment}}})
        if 'reconnect' in text and i == 0:
            with lock:
                streams = list(state['streams'])
            for stream in streams:
                try:
                    stream['socket'].shutdown(socket.SHUT_RDWR)
                except OSError:
                    pass
        if cancel_generation.wait(0.3):
            break
    if 'cancel' in text:
        cancel_generation.wait(8)
    if cancel_generation.is_set():
        seq = event('assistant/attempt', {'turn': 1, 'step': 0, 'stream': [
            {'type': 'text-chunks', 'index': 0, 'texts': fragments[:1]}]})
        reason = {'kind': 'aborted'}
    else:
        seq = event('assistant/message', {'turn': 1, 'step': 0, 'message': {
            'content': [{'type': 'text', 'text': ''.join(fragments)}]}}, True)
        reason = {'kind': 'completed'}
    publish({'type': 'assistant-stream', 'frame': {'type': 'end', 'attemptId': request_id,
             'index': len(fragments), 'outcome': {'kind': 'committed', 'seq': seq}}})
    state['running'] = False
    event('turn/end', {'turn': 1, 'reason': reason})


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        if self.path == '/api/remote.mux':
            assert 'dsh=test' in self.headers.get('Cookie', '')
            self.send_response(101)
            self.send_header('Upgrade', 'websocket')
            self.send_header('Connection', 'Upgrade')
            accept = base64.b64encode(hashlib.sha1((self.headers['Sec-WebSocket-Key'] +
                '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').encode()).digest()).decode()
            self.send_header('Sec-WebSocket-Accept', accept)
            self.end_headers()
            stream = None
            try:
                opening = ws_read(self.rfile)
                assert opening['endpoint'] == 'session/follow'
                assert opening['payload']['args']['request']['address']['sessionId'] == 'session-pty'
                stream = {'id': opening['streamId'], 'socket': self.connection, 'lock': threading.Lock()}
                with lock:
                    ws_write(stream, {'type': 'item', 'streamId': stream['id'], 'value': {
                        'type': 'snapshot', 'header': {'id': 'session-pty'},
                        'cursor': len(state['history']) - 1, 'records': state['history']}})
                    state['streams'].append(stream)
                while ws_read(self.rfile) is not None:
                    pass
            except (OSError, ValueError, TypeError):
                pass
            finally:
                with lock:
                    if stream in state['streams']:
                        state['streams'].remove(stream)
            return
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
            value = {'running': state['running'], 'cursor': len(state['history']) - 1}
        elif method == 'session/prompt':
            prompt = arguments['request']
            assert prompt['sessionId'] == 'session-pty' and prompt['mode'] == 'queue'
            if prompt['requestId'] not in state['admitted']:
                state['admitted'].add(prompt['requestId'])
                cancel_generation.clear()
                threading.Thread(target=generate_prompt, args=(prompt,), daemon=True).start()
            value = {'accepted': True}
        elif method == 'session/cancel':
            assert arguments['request']['sessionId'] == 'session-pty'
            cancel_generation.set()
            value = {'accepted': True}
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


server = ThreadingHTTPServer(('127.0.0.1', port), Handler)
print(f'dsh web: http://127.0.0.1:{port}/?token=test-only', flush=True)
server.serve_forever()
