#!/usr/bin/env node
'use strict';
// A deterministic Web API peer. The real-dsh scenario checks upstream startup
// separately; this peer controls partial output and human decision timing.
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const { spawn } = require('node:child_process');
const { randomUUID } = require('node:crypto');
const { WebSocketServer } = require(process.env.TUI_TEST_WS || '/usr/lib/dsh-launcher-tui/node_modules/ws');
const dir = process.env.TUI_FAKE_STATE;
const portArg = process.argv.indexOf('--port');
const port = Number(process.argv[portArg + 1]);
const token = randomUUID();
const log = value => fs.appendFileSync(path.join(dir, 'requests.jsonl'), JSON.stringify(value) + '\n');
const worker = spawn(process.execPath, ['-e', process.env.TUI_FAKE_IGNORE_TERM === '1'
  ? 'process.on("SIGTERM",()=>{});setInterval(()=>{},1000)' : 'setInterval(()=>{},1000)'], { stdio: 'ignore' });
const sessions = new Map();
const clients = new Map();
const mux = new WebSocketServer({ noServer: true });
const pendingStreams = [];
let sequence = 0;
const item = (peer, id, value) => { if (peer.readyState === 1) peer.send(JSON.stringify({ type: 'item', streamId: id, value })); };
const follow = (session, value) => {
  for (const [peer, client] of clients) for (const [id, target] of client.follows) if (target === session) item(peer, id, value);
};
const event = (session, type, data) => {
  const value = { type, seq: sequence++, time: Date.now(), data };
  sessions.get(session).records.push({ type: 'event', event: value });
  follow(session, { type: 'event', event: value });
};
function interaction(session, kind) {
  for (const [peer, client] of clients) {
    const id = randomUUID();
    client.pending.set(id, session);
    item(peer, client.events, { type: 'waterfall', eventId: id, agentId: kind === 'foreign' ? 'unrelated-session' : session,
      event: kind === 'approve' || kind === 'foreign' ? 'approval/request' : 'user-questions/request',
      request: kind === 'approve' || kind === 'foreign' ? { toolName: 'bash', reason: 'Docker test decision' }
        : { questions: [{ id: 'choice', question: 'Choose a test option', options: [{ label: 'First' }, { label: 'Second' }] }] } });
  }
}
const server = http.createServer(async (request, response) => {
  try {
    const url = new URL(request.url, 'http://localhost');
    if (url.pathname === '/health') { response.end('alive'); return; }
    if (url.pathname === '/release') { pendingStreams.shift()?.(); response.end('released'); return; }
    if (url.pathname === '/connect') {
      if (url.searchParams.get('token') !== token || url.searchParams.get('interactions') !== 'answer') { response.writeHead(401).end(); return; }
      response.writeHead(302, { Location: './api/remote.mux', 'Set-Cookie': 'tui=fixture; HttpOnly; Path=/' }).end(); return;
    }
    if (request.headers.cookie !== 'tui=fixture') { response.writeHead(401).end(); return; }
    let body = '';
    for await (const chunk of request) body += chunk;
    const envelope = JSON.parse(body);
    const args = envelope.payload.args;
    log({ method: envelope.method, args });
    const rpc = (value, error) => response.writeHead(200, { 'Content-Type': 'application/json' }).end(JSON.stringify({
      type: 'server-response', rpcId: envelope.rpcId, result: error ? { ok: false, error: { message: error } } : { ok: true, value },
    }));
    if (envelope.type !== 'client-request' || url.pathname !== '/api/' + envelope.method) { rpc(null, 'Invalid envelope'); return; }
    const session = args.request?.sessionId;
    switch (envelope.method) {
      case 'session/create': {
        const sessionId = `session-${sessions.size + 1}`;
        sessions.set(sessionId, { records: [], cwd: args.request.cwd });
        rpc({ sessionId }); break;
      }
      case 'session/list':
        if (!args._request || Object.keys(args).length !== 1) { rpc(null, 'Expected _request'); break; }
        rpc({ items: [...sessions].map(([sessionId, value]) => ({ sessionId, cwd: value.cwd })) }); break;
      case 'session/prompt': {
        if (!sessions.has(session)) { rpc(null, 'Unknown session'); break; }
        const text = args.request.content[0].text;
        event(session, 'user/message', { content: [{ type: 'text', text }] });
        rpc({ accepted: true });
        if (['approve', 'question', 'foreign'].includes(text)) { interaction(session, text); break; }
        if (text === 'disconnect') { for (const peer of mux.clients) peer.close(); break; }
        if (text === 'stream') {
          const attemptId = randomUUID();
          const frame = value => follow(session, { type: 'assistant-stream', frame: { attemptId, ...value } });
          const turn = sequence;
          event(session, 'turn/start', { turn });
          frame({ type: 'start', turn, step: 0 });
          frame({ type: 'chunk', chunk: { type: 'text-delta', text: 'FIRST_FRAGMENT' } });
          pendingStreams.push(() => {
            frame({ type: 'chunk', chunk: { type: 'text-delta', text: ' SECOND_FRAGMENT' } });
            event(session, 'assistant/message', { turn, step: 0, message: { content: [{ type: 'text', text: 'FIRST_FRAGMENT SECOND_FRAGMENT' }] } });
            frame({ type: 'end', outcome: { kind: 'committed' } });
            event(session, 'turn/end', { turn, reason: { kind: 'completed' } });
          });
        }
        break;
      }
      case 'session/cancel':
        for (const [peer, client] of clients) for (const [id, target] of client.pending) if (target === session) {
          item(peer, client.events, { type: 'cancel', eventId: id }); client.pending.delete(id);
        }
        rpc({ accepted: true }); break;
      case '$events/result': {
        const client = [...clients.values()].find(value => value.id === args.clientId);
        if (!client?.pending.has(args.eventId)) { rpc(null, 'No pending event'); break; }
        client.pending.delete(args.eventId); rpc({ accepted: true }); break;
      }
      default: rpc(null, 'Unknown method');
    }
  } catch (error) { log({ error: error.message }); response.writeHead(500).end(); }
});
server.on('upgrade', (request, socket, head) => {
  if (request.headers.cookie !== 'tui=fixture' || request.url !== '/api/remote.mux') { socket.destroy(); return; }
  mux.handleUpgrade(request, socket, head, peer => {
    const client = { id: randomUUID(), events: null, follows: new Map(), pending: new Map() };
    clients.set(peer, client);
    peer.on('close', () => clients.delete(peer));
    peer.on('error', () => {});
    peer.on('message', data => {
      const message = JSON.parse(String(data));
      if (message.type === 'cancel') { client.follows.delete(message.streamId); return; }
      if (message.type !== 'open') return;
      if (message.endpoint === '$events') {
        client.events = message.streamId;
        item(peer, message.streamId, { type: 'ready', clientId: client.id, host: { home: dir } });
      } else if (message.endpoint === 'session/follow') {
        const request = message.payload.args.request;
        const session = request.address.sessionId;
        if (!sessions.has(session)) { peer.send(JSON.stringify({ type: 'error', streamId: message.streamId, error: { message: 'Unknown session' } })); return; }
        client.follows.set(message.streamId, session);
        item(peer, message.streamId, { type: 'snapshot', cursor: sequence - 1, records: sessions.get(session).records });
      }
    });
  });
});
server.listen(port, '127.0.0.1', () => {
  fs.writeFileSync(path.join(dir, 'ready.json'), JSON.stringify({ pid: process.pid, worker: worker.pid, port, url: `http://127.0.0.1:${port}/?token=${token}` }));
  // Split the login line across writes to exercise stdout buffering.
  process.stdout.write('dsh we');
  setTimeout(() => process.stdout.write(`b: http://127.0.0.1:${port}/?token=${token}\n`), 15);
});
if (process.env.TUI_FAKE_IGNORE_TERM === '1') process.on('SIGTERM', () => {});
else process.on('SIGTERM', () => {
  if (worker.exitCode !== null || worker.signalCode !== null) process.exit(0);
  worker.once('exit', () => process.exit(0));
  worker.kill();
});
