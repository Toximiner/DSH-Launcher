'use strict';
const { EventEmitter } = require('node:events');
const { randomUUID } = require('node:crypto');
const WebSocket = require('ws');

// Wire adapter for dsh 0.2.2-rc.1. Keep its pre-stable API out of terminal rendering.
class DshClient extends EventEmitter {
  constructor(url) {
    super();
    const login = new URL(url);
    this.origin = login.origin;
    this.token = login.searchParams.get('token');
    this.streams = new Map();
    this.cookie = '';
    this.closed = false;
    this.lifetime = new AbortController();
  }

  async connect() {
    if (!this.token) throw new Error('A dsh login URL with ?token= is required. Set DSH_TUI_URL or use --url.');
    const login = new URL('/connect', this.origin);
    login.searchParams.set('token', this.token);
    login.searchParams.set('interactions', 'answer');
    const response = await fetch(login, { redirect: 'manual', signal: AbortSignal.any([this.lifetime.signal, AbortSignal.timeout(10000)]) });
    this.token = null;
    if (response.status !== 302) throw new Error(`dsh login failed (HTTP ${response.status}); obtain a fresh login URL`);
    this.cookie = response.headers.getSetCookie().map(value => value.split(';')[0]).join('; ');
    if (!this.cookie) throw new Error('dsh did not return an authentication cookie');
    await new Promise((resolve, reject) => {
      const ws = this.socket = new WebSocket(this.origin.replace(/^http/, 'ws') + '/api/remote.mux', {
        headers: { Cookie: this.cookie, Origin: this.origin }, handshakeTimeout: 10000, maxPayload: 16 * 1024 * 1024,
      });
      ws.on('message', data => {
        try {
          const message = JSON.parse(String(data));
          if (!message || typeof message.streamId !== 'string') throw new Error('Malformed dsh stream');
          const stream = this.streams.get(message.streamId);
          if (!stream) return;
          if (message.type === 'item') stream.item(message.value);
          else if (message.type === 'error') stream.fail(new Error(message.error?.message || 'dsh stream failed'));
          else if (message.type === 'end') stream.fail(new Error('dsh stream ended'));
          else throw new Error('Unsupported dsh stream frame');
        } catch (error) { this.emit('fault', error); }
      });
      ws.once('open', resolve);
      ws.on('error', () => { reject(new Error('Cannot connect to dsh WebSocket')); });
      ws.once('close', () => {
        reject(new Error('dsh disconnected'));
        for (const stream of this.streams.values()) stream.fail(new Error('dsh disconnected'));
        this.streams.clear();
        if (!this.closed) this.emit('fault', new Error('dsh disconnected; restart the terminal client to reconnect'));
      });
    });
    await this.subscribe('$events', {}, value => {
      if (value.type === 'ready') this.clientId = value.clientId;
      else if (!this.emit('interaction', value) && value.type === 'waterfall') {
        // Do not hold another session's decision while the terminal is still opening.
        void this.answer(value.eventId, { kind: 'next' }).catch(error => { if (!this.closed) this.emit('fault', error); });
      }
    });
    if (!this.clientId) throw new Error('Unsupported dsh event protocol');
  }

  async rpc(endpoint, args) {
    if (this.closed) throw new Error('Connection is closed; restart the terminal client');
    const rpcId = randomUUID();
    const response = await fetch(`${this.origin}/api/${endpoint}`, {
      method: 'POST', headers: { 'Content-Type': 'application/json', Cookie: this.cookie, Origin: this.origin },
      body: JSON.stringify({ type: 'client-request', rpcId, method: endpoint, payload: { args } }),
      redirect: 'error', signal: AbortSignal.any([this.lifetime.signal, AbortSignal.timeout(30000)]),
    });
    if (!response.ok) throw new Error(`dsh request failed (HTTP ${response.status})`);
    const envelope = await response.json();
    if (envelope.type !== 'server-response' || envelope.rpcId !== rpcId || typeof envelope.result?.ok !== 'boolean') throw new Error('Unsupported dsh RPC response');
    if (!envelope.result.ok) throw new Error(envelope.result.error?.message || 'dsh rejected the request');
    return envelope.result.value;
  }

  subscribe(endpoint, args, onItem) {
    if (this.closed) return Promise.reject(new Error('Connection is closed'));
    const id = randomUUID();
    let opened = false;
    return new Promise((resolve, reject) => {
      const cancel = () => {
        clearTimeout(timer);
        this.streams.delete(id);
        if (this.socket?.readyState === WebSocket.OPEN) this.socket.send(JSON.stringify({ type: 'cancel', streamId: id }));
      };
      const timer = setTimeout(() => { cancel(); reject(new Error(`dsh ${endpoint} did not return an opening snapshot`)); }, 10000);
      this.streams.set(id, {
        item: value => {
          onItem(value);
          if (!opened) { opened = true; clearTimeout(timer); resolve(cancel); }
        },
        fail: error => {
          cancel();
          if (opened) { if (!this.closed) this.emit('fault', error); } else reject(error);
        },
      });
      this.socket.send(JSON.stringify({ type: 'open', streamId: id, endpoint, payload: { args } }));
    });
  }

  answer(eventId, outcome) { return this.rpc('$events/result', { clientId: this.clientId, eventId, outcome }); }

  close() {
    this.closed = true;
    this.lifetime.abort();
    this.socket?.terminate();
    this.cookie = '';
    this.token = null;
  }
}

module.exports = { DshClient };
