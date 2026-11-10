'use strict';
const { EventEmitter } = require('node:events');
const readline = require('node:readline');
const { stripVTControlCharacters } = require('node:util');

// Never interpret model, plugin, or session text as terminal escape sequences.
const clean = value => Array.from(stripVTControlCharacters(String(value))).filter(char => {
  const code = char.codePointAt(0);
  return code === 9 || code === 10 || (code >= 32 && !(code >= 127 && code <= 159));
}).join('');
function wrap(text, width) {
  return clean(text).split('\n').flatMap(line => {
    const chars = Array.from(line.replace(/\t/g, '    '));
    const result = [];
    do { result.push(chars.splice(0, width).join('')); } while (chars.length);
    return result;
  });
}

class Terminal extends EventEmitter {
  constructor(input = process.stdin, output = process.stdout) {
    super();
    this.input = input; this.output = output;
    this.lines = []; this.text = ''; this.cursor = 0; this.offset = 0;
    this.history = []; this.historyIndex = 0; this.draft = '';
    this.title = 'DeepSeek Harness — terminal'; this.status = 'Connecting…';
    this.key = (text, key) => this.onKey(text, key || {});
    this.resize = () => this.draw();
  }

  start() {
    if (!this.input.isTTY || !this.output.isTTY) throw new Error('Run this application in an interactive terminal');
    readline.emitKeypressEvents(this.input);
    this.wasRaw = !!this.input.isRaw;
    this.input.setRawMode(true);
    this.input.resume();
    this.input.on('keypress', this.key);
    this.output.on('resize', this.resize);
    this.output.write('\x1b[?1049h\x1b[?2004h\x1b[?25l');
    this.draw();
  }

  log(text) {
    const entry = { text: String(text) };
    this.lines.push(entry);
    if (this.lines.length > 1000) this.lines.shift();
    this.offset = 0;
    this.draw();
    return entry;
  }

  draw() {
    if (this.closed || this.timer) return;
    this.timer = setTimeout(() => { this.timer = null; this.render(); }, 25);
  }

  render() {
    const width = Math.max(1, (this.output.columns || 80) - 1);
    const height = Math.max(5, this.output.rows || 24);
    const bodyHeight = height - 4;
    const body = this.lines.flatMap(line => wrap(line.text, width));
    this.offset = Math.min(this.offset, Math.max(0, body.length - bodyHeight));
    const end = body.length - this.offset;
    const visible = body.slice(Math.max(0, end - bodyHeight), end);
    while (visible.length < bodyHeight) visible.push('');
    const chars = Array.from(this.text);
    const start = Math.max(0, this.cursor - Math.max(1, width - 4));
    const prompt = '> ' + chars.slice(start, start + Math.max(0, width - 2)).join('');
    const rows = [clean(this.title), ...visible, clean(this.status), '↑↓ history · PgUp/PgDn scroll · Ctrl+C stop · Ctrl+D exit · /help', prompt];
    this.output.write('\x1b[?25l\x1b[H' + rows.map(row => '\x1b[2K' + Array.from(row).slice(0, width).join('')).join('\r\n') +
      `\x1b[${height};${Math.min(width, this.cursor - start + 3)}H\x1b[?25h`);
  }

  onKey(text, key) {
    if (key.name === 'paste-start') { this.pasting = true; return; }
    if (key.name === 'paste-end') { this.pasting = false; return; }
    if (key.ctrl && key.name === 'd') return this.emit('quit');
    if (key.ctrl && key.name === 'c') return this.emit('interrupt');
    if (key.name === 'pageup') this.offset += Math.max(1, (this.output.rows || 24) - 5);
    else if (key.name === 'pagedown') this.offset = Math.max(0, this.offset - Math.max(1, (this.output.rows || 24) - 5));
    else if ((key.name === 'return' || key.name === 'enter') && !this.pasting) {
      const line = this.text.trim();
      this.text = ''; this.cursor = 0; this.offset = 0;
      if (line) {
        if (this.history.at(-1) !== line) this.history.push(line);
        if (this.history.length > 50) this.history.shift();
        this.historyIndex = this.history.length;
        this.emit('line', line);
      }
    } else if (key.name === 'up' || key.name === 'down') {
      if (this.historyIndex === this.history.length) this.draft = this.text;
      this.historyIndex = Math.max(0, Math.min(this.history.length, this.historyIndex + (key.name === 'up' ? -1 : 1)));
      this.text = this.history[this.historyIndex] ?? this.draft;
      this.cursor = Array.from(this.text).length;
    } else if (key.name === 'left') this.cursor = Math.max(0, this.cursor - 1);
    else if (key.name === 'right') this.cursor = Math.min(Array.from(this.text).length, this.cursor + 1);
    else if (key.name === 'home' || (key.ctrl && key.name === 'a')) this.cursor = 0;
    else if (key.name === 'end' || (key.ctrl && key.name === 'e')) this.cursor = Array.from(this.text).length;
    else if (key.ctrl && key.name === 'u') { this.text = ''; this.cursor = 0; }
    else if (key.name === 'backspace' || key.name === 'delete') {
      const chars = Array.from(this.text);
      if (key.name === 'delete') chars.splice(this.cursor, 1);
      else if (this.cursor > 0) chars.splice(--this.cursor, 1);
      this.text = chars.join('');
    } else if (text && !key.ctrl && !key.meta) {
      const chars = Array.from(this.text);
      const added = Array.from(clean(text).replace(/[\r\n]/g, ' '));
      if (chars.length + added.length <= 65536) {
        chars.splice(this.cursor, 0, ...added); this.cursor += added.length; this.text = chars.join('');
      }
    }
    this.draw();
  }

  close() {
    if (this.closed) return;
    this.closed = true;
    clearTimeout(this.timer);
    this.input.removeListener('keypress', this.key);
    this.output.removeListener('resize', this.resize);
    this.input.setRawMode(this.wasRaw);
    this.input.pause();
    this.output.write('\x1b[?2004l\x1b[?25h\x1b[?1049l');
  }
}

module.exports = { Terminal, clean, wrap };
