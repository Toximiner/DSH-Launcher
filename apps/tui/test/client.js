'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { createServer } = require('node:http');
const { once } = require('node:events');
const { execFile } = require('node:child_process');
const { promisify } = require('node:util');
const path = require('node:path');
const { WebSocketServer } = require('ws');
const { DshClient } = require('../src/client');
const { Chat } = require('../src/chat');

async function fixture(t) {
  const requests = [];
  const mux = new WebSocketServer({ noServer: true });
  let peer, followId;
  const send = (id, value) => peer.send(JSON.stringify({ type: 'item', streamId: id, value }));
  const server = createServer(async (request, response) => {
    const url = new URL(request.url, 'http://localhost');
    if (url.pathname === '/connect') {
      if (url.searchParams.get('token') !== 'test-token' || url.searchParams.get('interactions') !== 'answer') {
        response.writeHead(401).end(); return;
      }
      response.writeHead(302, { 'Set-Cookie': 'dsh=test-cookie; HttpOnly; Path=/', Location: './api/remote.mux' }).end(); return;
    }
    if (request.headers.cookie !== 'dsh=test-cookie') { response.writeHead(401).end(); return; }
    let body = '';
    for await (const chunk of request) body += chunk;
    const envelope = JSON.parse(body);
    requests.push(envelope);
    assert.equal(envelope.type, 'client-request');
    assert.equal(url.pathname, '/api/' + envelope.method);
    let value;
    if (envelope.method === 'session/create') value = { sessionId: 'test-session' };
    else if (envelope.method === 'session/list') {
      assert.deepEqual(envelope.payload.args, { _request: {} });
      value = { items: [{ sessionId: 'test-session', cwd: '/tmp' }] };
    }
    else if (envelope.method === 'session/prompt') {
      assert.equal(envelope.payload.args.request.sessionId, 'test-session');
      assert.equal(envelope.payload.args.request.mode, 'queue');
      assert.deepEqual(envelope.payload.args.request.content, [{ type: 'text', text: 'Hello' }]);
      send(followId, { type: 'assistant-stream', frame: { type: 'start', attemptId: 'a', turn: 1, step: 0 } });
      send(followId, { type: 'assistant-stream', frame: { type: 'chunk', attemptId: 'a', chunk: { type: 'text-delta', text: 'Answer' } } });
      send(followId, { type: 'event', event: { type: 'assistant/message', seq: 1, data: { turn: 1, step: 0, message: { content: [{ type: 'text', text: 'Answer' }] } } } });
      send(followId, { type: 'assistant-stream', frame: { type: 'end', attemptId: 'a', outcome: { kind: 'committed' } } });
      value = { accepted: true };
    } else if (envelope.method === '$events/result' || envelope.method === 'session/cancel') value = { accepted: true };
    else {
      response.writeHead(200, { 'Content-Type': 'application/json' }).end(JSON.stringify({ type: 'server-response', rpcId: envelope.rpcId, result: { ok: false, error: { message: 'Unknown operation' } } })); return;
    }
    response.writeHead(200, { 'Content-Type': 'application/json' }).end(JSON.stringify({ type: 'server-response', rpcId: envelope.rpcId, result: { ok: true, value } }));
  });
  server.on('upgrade', (request, socket, head) => {
    if (request.headers.cookie !== 'dsh=test-cookie' || request.url !== '/api/remote.mux') { socket.destroy(); return; }
    mux.handleUpgrade(request, socket, head, ws => {
      peer = ws;
      ws.on('message', data => {
        const message = JSON.parse(String(data));
        if (message.type !== 'open') return;
        if (message.endpoint === '$events') send(message.streamId, { type: 'ready', clientId: 'test-client', host: { home: '/tmp' } });
        else {
          assert.equal(message.endpoint, 'session/follow');
          assert.deepEqual(message.payload.args.request.address, { kind: 'session', sessionId: 'test-session' });
          followId = message.streamId;
          send(followId, { type: 'snapshot', records: [], cursor: -1 });
        }
      });
    });
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  t.after(async () => {
    for (const socket of mux.clients) socket.terminate();
    mux.close();
    server.closeAllConnections();
    await new Promise(resolve => server.close(resolve));
  });
  return { url: `http://127.0.0.1:${server.address().port}/?token=test-token`, requests, disconnect: () => peer.terminate() };
}

test('authenticates HTTP and WebSocket, streams chat once, and grants only an explicit approval', async t => {
  const f = await fixture(t);
  const client = new DshClient(f.url);
  t.after(() => client.close());
  client.on('fault', error => assert.fail(error.message));
  await client.connect();
  const lines = [];
  const terminal = { log(text) { const line = { text }; lines.push(line); return line; }, draw() {} };
  const chat = new Chat(client, terminal, '/tmp');
  await chat.newSession();
  await chat.input('Hello');
  await new Promise(resolve => setTimeout(resolve, 30));
  assert.deepEqual(lines.filter(line => line.text.startsWith('Assistant:')).map(line => line.text), ['Assistant: Answer']);
  await chat.input('/sessions');
  assert.ok(lines.some(line => line.text.includes('test-session /tmp')));
  await chat.interaction({ type: 'waterfall', eventId: 'decision', agentId: 'test-session', event: 'approval/request', request: { toolName: 'bash', reason: 'Run tests' } });
  assert.equal(f.requests.filter(item => item.method === '$events/result').length, 0);
  await chat.input('/approve');
  assert.deepEqual(f.requests.at(-1).payload.args, { clientId: 'test-client', eventId: 'decision', outcome: { kind: 'result', value: 'allowed-once' } });
  await assert.rejects(chat.input('/approve'), /No pending/);
  await chat.input('/stop');
  assert.equal(f.requests.at(-1).method, 'session/cancel');
  await assert.rejects(client.rpc('invalid', {}), /Unknown operation/);
});

test('reports authentication failures and refuses requests after close', async t => {
  const f = await fixture(t);
  const client = new DshClient(f.url.replace('test-token', 'invalid'));
  await assert.rejects(client.connect(), /login failed/);
  client.close();
  await assert.rejects(client.rpc('session/list', {}), /Connection is closed/);
  await assert.rejects(client.subscribe('session/follow', {}, () => {}), /Connection is closed/);
});

test('reports a lost server and settles active streams', async t => {
  const f = await fixture(t);
  const client = new DshClient(f.url);
  t.after(() => client.close());
  await client.connect();
  const lost = once(client, 'fault');
  f.disconnect();
  assert.match((await lost)[0].message, /disconnected/);
});

test('terminal CLI handles pasted commands, cancellation, and exit in a real PTY', { skip: process.platform !== 'linux', timeout: 20000 }, async t => {
  const f = await fixture(t);
  const script = `
import os, pty, select, sys, time, signal
pid, fd = pty.fork()
if pid == 0:
    os.execv(sys.argv[1], [sys.argv[1], sys.argv[2], '--url', sys.argv[3]])
buffer = b''
reaped = False
def wait_for(text):
    global buffer
    deadline = time.monotonic() + 8
    while time.monotonic() < deadline:
        if text in buffer:
            return
        ready, _, _ = select.select([fd], [], [], 0.05)
        if ready:
            buffer += os.read(fd, 65536)
    raise RuntimeError('terminal did not display ' + repr(text))
try:
    wait_for(b'Ready')
    buffer = b''
    os.write(fd, b'\\x1b[200~/exit\\r\\x1b[201~')
    wait_for(b'> /exit')
    assert os.waitpid(pid, os.WNOHANG)[0] == 0
    os.write(fd, b'\\x15/approve\\r')
    wait_for(b'No pending decision')
    os.write(fd, b'\\x03')
    time.sleep(0.1)
    os.write(fd, b'\\x04')
    wait_for(b'\\x1b[?1049l')
    _, status = os.waitpid(pid, 0)
    reaped = True
    assert os.waitstatus_to_exitcode(status) == 0
finally:
    if not reaped:
        try: os.kill(pid, signal.SIGTERM)
        except ProcessLookupError: pass
    os.close(fd)
`;
  await promisify(execFile)('python3', ['-c', script, process.execPath, path.join(__dirname, '../bin/dsh-launcher-tui.js'), f.url], { timeout: 15000 });
  assert.ok(f.requests.some(request => request.method === 'session/cancel'));
});
