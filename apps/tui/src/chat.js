'use strict';
const { randomUUID } = require('node:crypto');

const HELP = '/new — new session · /sessions — list · /use <id> — open\n/stop — cancel response · /approve or /reject — pending operation\n/answer <number(s) or text> — question · /dismiss — dismiss · /exit — quit';
const textBlocks = blocks => (blocks || []).filter(block => block.type === 'text').map(block => block.text).join('\n');
function streamText(records) {
  return (records || []).flatMap(record => record.type === 'text-chunks' ? record.texts : []).join('');
}
function questionAnswer(question, raw) {
  const options = question.options || [];
  const numbers = raw.split(',').map(value => value.trim());
  if (numbers.every(value => /^\d+$/.test(value))) {
    const selected = [...new Set(numbers.map(Number))];
    if ((!question.multiSelect && selected.length > 1) || selected.some(n => n < 1 || n > options.length)) throw new Error('Choose a valid option number');
    return { id: question.id, selected: selected.map(n => options[n - 1].label) };
  }
  if (question.intent?.kind === 'approval') throw new Error('This permission request requires an option number');
  if (!raw.trim()) throw new Error('An answer is required');
  return { id: question.id, selected: [], custom: raw };
}

class Chat {
  constructor(client, terminal, cwd) {
    this.client = client; this.terminal = terminal; this.cwd = cwd;
    this.pending = []; this.live = new Map();
    client.on('interaction', frame => { void this.interaction(frame).catch(error => terminal.log(error.message)); });
  }

  async open(id) {
    for (const item of this.pending.splice(0)) await this.client.answer(item.eventId, { kind: 'next' });
    this.unfollow?.(); this.live.clear(); this.attempt = null;
    this.id = id;
    this.terminal.title = `DeepSeek Harness — ${id}`;
    this.terminal.log(`Session ${id}`);
    this.unfollow = await this.client.subscribe('session/follow', {
      request: { address: { kind: 'session', sessionId: id }, activate: true, assistantStream: true, maxMessages: 30 },
    }, frame => this.follow(frame));
    this.terminal.status = 'Ready'; this.terminal.draw();
  }

  async newSession() {
    const result = await this.client.rpc('session/create', { request: { cwd: this.cwd } });
    if (typeof result?.sessionId !== 'string') throw new Error('Unsupported dsh session response');
    await this.open(result.sessionId);
  }

  follow(frame) {
    if (frame.type === 'snapshot') {
      for (const record of frame.records) if (record.type === 'event') this.event(record.event);
      const active = frame.assistantStream?.activeAttempt;
      if (active) {
        this.attempt = { id: active.attemptId, key: `${active.turn}:${active.step}` };
        this.live.set(this.attempt.key, this.terminal.log('Assistant: ' + streamText(active.stream)));
      }
    } else if (frame.type === 'event') this.event(frame.event);
    else if (frame.type === 'assistant-stream') {
      const current = frame.frame;
      if (current.type === 'start') {
        this.attempt = { id: current.attemptId, key: `${current.turn}:${current.step}` };
        this.live.set(this.attempt.key, this.terminal.log('Assistant: '));
      } else if (current.type === 'chunk' && this.attempt?.id === current.attemptId && current.chunk?.type === 'text-delta') {
        this.live.get(this.attempt.key).text += current.chunk.text;
        this.terminal.draw();
      } else if (current.type === 'end' && this.attempt?.id === current.attemptId) {
        if (current.outcome.kind === 'abandoned') this.terminal.log('[Assistant attempt interrupted]');
        this.live.delete(this.attempt.key);
        this.attempt = null;
      }
    }
  }

  event(event) {
    if (event.type === 'user/message') this.terminal.log('You: ' + textBlocks(event.data.content));
    if (event.type === 'assistant/message') {
      const text = textBlocks(event.data.message?.content) || streamText(event.data.stream);
      const key = `${event.data.turn}:${event.data.step}`;
      const live = this.live.get(key);
      if (live) { live.text = 'Assistant: ' + text; this.live.delete(key); this.terminal.draw(); }
      else if (text) this.terminal.log('Assistant: ' + text);
    }
    if (event.type === 'tool/call') this.terminal.log(`[Tool: ${event.data.name}]`);
    if (event.type === 'turn/start') this.terminal.status = 'Working…';
    if (event.type === 'turn/end') {
      this.terminal.status = 'Ready';
      if (event.data.reason?.kind === 'error') this.terminal.log('[Turn failed: ' + (event.data.reason.error?.message || 'check dsh configuration') + ']');
    }
    this.terminal.draw();
  }

  async interaction(frame) {
    if (frame.type === 'cancel') {
      this.pending = this.pending.filter(item => item.eventId !== frame.eventId);
      this.showPending(); return;
    }
    if (frame.type !== 'waterfall') return;
    if (frame.agentId !== this.id || !['approval/request', 'user-questions/request'].includes(frame.event) || frame.request.wait?.timed) {
      await this.client.answer(frame.eventId, { kind: 'next' }); return;
    }
    this.pending.push({ ...frame, answers: [] });
    if (this.pending.length === 1) this.showPending();
  }

  showPending() {
    const pending = this.pending[0];
    if (!pending) { this.terminal.status = 'Ready'; this.terminal.draw(); return; }
    if (pending.event === 'approval/request') {
      this.terminal.log(`Approval: ${pending.request.toolName}\n${pending.request.reason || ''}\n/approve — allow once · /reject — deny`);
    } else {
      const question = pending.request.questions[pending.answers.length];
      if (!question) return;
      this.terminal.log(`${question.question}\n${question.detail || ''}\n` +
        (question.options || []).map((option, index) => `${index + 1}. ${option.label}${option.description ? ' — ' + option.description : ''}`).join('\n') +
        '\n/answer <number(s), separated by commas, or text> · /dismiss');
    }
    this.terminal.status = 'Waiting for your decision'; this.terminal.draw();
  }

  async decision(command, raw) {
    const pending = this.pending[0];
    if (!pending) throw new Error('No pending decision');
    let outcome;
    if (pending.event === 'approval/request') {
      if (!['/approve', '/reject', '/dismiss'].includes(command)) throw new Error('Use /approve or /reject');
      outcome = { kind: 'result', value: command === '/approve' ? 'allowed-once' : 'rejected' };
    } else if (command === '/dismiss') {
      outcome = { kind: 'rejected', error: { name: 'UserQuestionError', code: 'ASK_CANCELLED', message: 'Dismissed by user' } };
    } else {
      if (command !== '/answer') throw new Error('Use /answer or /dismiss');
      const answer = questionAnswer(pending.request.questions[pending.answers.length], raw);
      const answers = [...pending.answers, answer];
      if (answers.length < pending.request.questions.length) { pending.answers = answers; this.showPending(); return; }
      outcome = { kind: 'result', value: { answers } };
    }
    await this.client.answer(pending.eventId, outcome);
    this.pending = this.pending.filter(item => item !== pending);
    this.showPending();
  }

  async input(line) {
    const [command] = line.split(/\s/);
    const raw = line.slice(command.length).trim();
    if (command === '/help') return this.terminal.log(HELP);
    if (command === '/new') return this.newSession();
    if (command === '/use') { if (!raw) throw new Error('Use /use <session-id>'); return this.open(raw); }
    if (command === '/sessions') {
      const result = await this.client.rpc('session/list', { _request: {} });
      return this.terminal.log(result.items.map(item => `${item.sessionId}${item.running ? ' [running]' : ''} ${item.cwd || ''}`).join('\n') || 'No sessions');
    }
    if (command === '/stop') return this.client.rpc('session/cancel', { request: { sessionId: this.id } });
    if (['/approve', '/reject', '/answer', '/dismiss'].includes(command)) return this.decision(command, raw);
    if (command.startsWith('/')) throw new Error('Unknown command; use /help. Send model input without a leading slash.');
    if (!this.id) throw new Error('Open a session first with /new or /use');
    return this.client.rpc('session/prompt', { request: { requestId: randomUUID(), sessionId: this.id, mode: 'queue', content: [{ type: 'text', text: line }] } });
  }
}

module.exports = { Chat, HELP, questionAnswer, streamText };
