#!/usr/bin/env node
'use strict';
const { options } = require('../src/options');
const { DshProcess } = require('../src/process');
const { DshInstaller } = require('../src/install');
const { Terminal } = require('../src/terminal');
const { DshClient } = require('../src/client');
const { Chat, HELP } = require('../src/chat');

const USAGE = `dsh-launcher-tui [options]
  --url URL       Attach using a dsh login URL (or DSH_TUI_URL)
  --dsh PATH      dsh executable (or DSH_BIN; default: dsh from PATH)
  --cwd PATH      Workspace (or DSH_CWD; default: current directory)
  --profile NAME  dsh web profile (default: web)
  --port NUMBER   Port for a server started here (default: 3080)
  --session ID    Open an existing session
  --help          Show help

Without --url, starts its own dsh web server. Use a free port.
If dsh is missing, /install installs it for this user; /cancel exits.
Only loopback connections are accepted; use SSH forwarding for remote hosts.
Ctrl+D or /exit quits. Ctrl+C stops the current response.
The client requires Node.js 22+; installing/running dsh 0.2.2-rc.1 requires 24.9+.`;

async function main() {
  const config = options(process.argv.slice(2));
  if (config.help) { console.log(USAGE); return; }
  const terminal = new Terminal();
  const server = new DshProcess();
  const installer = new DshInstaller();
  const lifetime = new AbortController();
  let client, chat, exiting;
  const quit = () => {
    if (exiting) return exiting;
    exiting = (async () => {
      terminal.status = 'Stopping…'; terminal.draw();
      lifetime.abort();
      client?.close();
      try { await installer.stop(); await server.stop(); } finally { terminal.close(); terminal.emit('quit-complete'); }
    })();
    return exiting;
  };
  terminal.start();
  const onSignal = () => { void quit().catch(() => { process.exitCode = 1; }); };
  for (const signal of ['SIGINT', 'SIGTERM', 'SIGHUP']) process.on(signal, onSignal);
  terminal.on('quit', onSignal);
  terminal.on('interrupt', () => {
    if (chat) void chat.input('/stop').catch(error => terminal.log(error.message));
    else onSignal();
  });
  let queue = Promise.resolve();
  terminal.on('line', line => {
    if (line === '/exit' || line === '/quit' || (!chat && line === '/cancel')) { onSignal(); return; }
    if (!chat) {
      if (installer.running) terminal.log('Installation is running; use /cancel to stop.');
      return;
    }
    queue = queue.then(() => { if (!exiting) return chat.input(line); }).catch(error => terminal.log('Error: ' + error.message));
  });
  try {
    terminal.log('DSH Launcher TUI — experimental\n' + HELP);
    let url = config.url;
    if (!url) {
      const bin = await installer.ensure(config, terminal, lifetime.signal);
      if (!bin || lifetime.signal.aborted) { await quit(); return; }
      terminal.status = 'Starting dsh…'; terminal.draw();
      url = await server.start({ ...config, bin });
    }
    if (exiting) { await exiting; return; }
    client = new DshClient(url);
    client.on('fault', error => {
      client.close();
      terminal.log('Connection error: ' + error.message);
      terminal.status = 'Disconnected — exit and reconnect'; terminal.draw();
    });
    await client.connect();
    if (exiting) { await exiting; return; }
    chat = new Chat(client, terminal, config.cwd);
    if (config.session) await chat.open(config.session); else await chat.newSession();
    await new Promise(resolve => terminal.once('quit-complete', resolve));
  } catch (error) {
    if (exiting) { await exiting; return; }
    await quit();
    throw error;
  } finally {
    for (const signal of ['SIGINT', 'SIGTERM', 'SIGHUP']) process.removeListener(signal, onSignal);
  }
}

main().catch(error => { console.error('dsh-launcher-tui:', error.message); process.exitCode = 1; });
