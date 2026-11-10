'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const { Chat, questionAnswer, streamText } = require('../src/chat');
const { options } = require('../src/options');
const { Terminal, clean, wrap } = require('../src/terminal');

test('keeps model and plugin control sequences out of the terminal', () => {
  assert.equal(clean('A\x1b[2JB\x1b]52;c;Y2xpcGJvYXJk\x07\x00C'), 'ABC');
  assert.deepEqual(wrap('Привет\nabc', 3), ['При', 'вет', 'abc']);
});

test('edits Unicode input, restores history, and does not submit pasted returns', () => {
  const terminal = new Terminal();
  const lines = [];
  terminal.draw = () => {};
  terminal.on('line', line => lines.push(line));
  terminal.onKey('Привет', {});
  terminal.onKey('', { name: 'left' });
  terminal.onKey('', { name: 'backspace' });
  terminal.onKey('', { name: 'return' });
  assert.deepEqual(lines, ['Привт']);
  terminal.onKey('', { name: 'up' });
  assert.equal(terminal.text, 'Привт');
  terminal.onKey('', { ctrl: true, name: 'u' });
  terminal.onKey('', { name: 'paste-start' });
  terminal.onKey('x', {});
  terminal.onKey('\r', { name: 'return' });
  assert.equal(lines.length, 1);
  terminal.onKey('', { name: 'paste-end' });
  terminal.onKey('', { name: 'return' });
  assert.equal(lines.at(-1), 'x');
});

test('validates command options and loopback connections', () => {
  assert.equal(options(['--port', '4010'], {}).port, 4010);
  assert.equal(options(['--url', 'http://localhost:3080/?token=x'], {}).url, 'http://localhost:3080/?token=x');
  for (const argv of [['--port', '0'], ['--profile', '..'], ['--url', 'https://example.com/'], ['--url', 'file:///tmp/x'], ['--missing'], ['--cwd']]) {
    assert.throws(() => options(argv, {}));
  }
});

test('encodes question choices and never treats free text as a permission grant', () => {
  const question = { id: 'q', options: [{ label: 'Allow' }, { label: 'Deny' }], intent: { kind: 'approval' } };
  assert.deepEqual(questionAnswer(question, '2'), { id: 'q', selected: ['Deny'] });
  assert.throws(() => questionAnswer(question, 'yes'), /option number/);
  assert.throws(() => questionAnswer(question, '1,2'), /valid option/);
  assert.deepEqual(questionAnswer({ ...question, multiSelect: true }, '1,2'), { id: 'q', selected: ['Allow', 'Deny'] });
  assert.deepEqual(questionAnswer({ id: 'free' }, 'answer'), { id: 'free', selected: [], custom: 'answer' });
  assert.equal(streamText([{ type: 'text-chunks', texts: ['A', 'B'] }, { type: 'reasoning-chunks', texts: ['hidden'] }]), 'AB');
});

test('delegates unrelated interactions and handles cancellation and multi-question answers', async () => {
  const client = new EventEmitter();
  const answers = [];
  client.answer = async (...args) => answers.push(args);
  const terminal = { log(text) { return { text }; }, draw() {} };
  const chat = new Chat(client, terminal, '/tmp');
  chat.id = 'session';
  const frame = { type: 'waterfall', eventId: 'question', agentId: 'session', event: 'user-questions/request', request: { questions: [{ id: 'a', question: 'First' }, { id: 'b', question: 'Second' }] } };
  await chat.interaction({ ...frame, agentId: 'other' });
  assert.deepEqual(answers.pop(), ['question', { kind: 'next' }]);
  await chat.interaction(frame);
  await chat.input('/answer first');
  assert.equal(answers.length, 0);
  await chat.input('/answer second');
  assert.deepEqual(answers.pop(), ['question', { kind: 'result', value: { answers: [{ id: 'a', selected: [], custom: 'first' }, { id: 'b', selected: [], custom: 'second' }] } }]);
  await chat.interaction(frame);
  await chat.interaction({ type: 'cancel', eventId: 'question' });
  await assert.rejects(chat.input('/answer stale'), /No pending/);
  await chat.interaction(frame);
  await chat.input('/dismiss');
  assert.equal(answers.pop()[1].error.code, 'ASK_CANCELLED');
});
