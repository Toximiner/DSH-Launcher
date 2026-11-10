# DSH Launcher TUI (experimental)

A standalone Linux terminal client for DeepSeek Harness. It uses the dsh Web API
over HTTP and WebSocket; it does not require Electron, a browser, X11, or Wayland.
The existing graphical launcher remains a separate application.

## Run from source

Requires Node.js 22+ and an installed `dsh`. The protocol adapter targets
`@deepseek-ai/dsh@0.2.2-rc.1`; upstream APIs are pre-stable.

```sh
npm ci --prefix apps/tui
npm run tui
```

The default command starts `dsh --profile web --no-open --port 3080` in the
current directory and opens a new session. Choose a free port if another dsh is
already running:

```sh
npm run tui -- --port 3081 --cwd /path/to/project
```

To attach, copy the login URL printed by an existing `dsh web` process into
`DSH_TUI_URL`, then run `npm run tui`. Alternatively pass `--url 'http://127.0.0.1:3080/?token=…'`;
be aware that command arguments can be recorded in shell history. Credentials
are held only in memory. Only loopback URLs are accepted; use an SSH tunnel to
reach a remote host. Exit stops only the process group started by this client.

`--dsh`, `--cwd`, `--profile`, `--port`, and `--session` select the executable,
workspace, web-enabled profile, port, and existing session. `DSH_BIN` and
`DSH_CWD` also work. GUI preferences are not read or changed.

## Interface

- Type a message and press Enter. Replies stream into the conversation.
- `/sessions` lists sessions; `/use <id>` opens one; `/new` creates one.
- `/stop` or Ctrl+C cancels the current response.
- `/approve` grants a pending operation once; `/reject` denies it. Ordinary
  chat input never grants permission.
- `/answer 1` selects an option; `/answer 1,3` selects several when allowed;
  `/answer text` supplies a custom answer. `/dismiss` dismisses a question.
- Up/Down browse input history; Left/Right, Home/End edit input; Ctrl+U clears
  it; Page Up/Down scroll the conversation.
- `/exit` or Ctrl+D exits and restores the terminal. SIGTERM/SIGHUP also clean
  up the owned server. SIGKILL cannot be intercepted.

The input editor is single-line. Bracketed paste does not submit messages until
Enter is pressed after the paste. History lives only in memory. Remote escape
sequences are stripped before display.

## Current limits

- This is an initial terminal client, not feature parity with the GUI. Updates,
  rollback, plugin installation, model selection, file/image uploads, rich tool
  output, and slash commands for the model are not implemented.
- Opening a session loads the latest 30 messages. Earlier history remains on
  the server; there is no history pagination or automatic reconnect yet.
- Indefinite questions and approvals for the selected session are supported.
  Timed questions, subagent questions, and other sessions' interactions are
  delegated to other answerers. They are never approved automatically.
- The interface is English-only for now; ordinary Russian text works. Terminal
  wrapping counts Unicode code points; wide characters and combining sequences
  may be misaligned.
- A self-restarting dsh that leaves its original process group is not adopted.
  The client never kills an unrelated process found by port.

## Test and package

From the repository root:

The Linux PTY regression test uses Python 3's standard library.

```sh
npm run lint
npm run test:tui
npm run build:tui
npm run build:tui:deb
```

The build creates `build/tui/dsh-launcher-tui-0.1.0.tgz`, installable with
`npm install -g ./build/tui/dsh-launcher-tui-0.1.0.tgz`. The package contains
only the terminal app and declares one runtime dependency, `ws`. It needs
Node.js and dsh installed separately. Building it does not install Electron.
CI tests and packages the TUI independently; it does not publish an npm release.

`build:tui:deb` also creates `build/tui/dsh-launcher-tui_0.1.0-1_all.deb`.
It bundles `ws` and depends on system `nodejs (>= 22)`; install it with
`sudo apt install ./build/tui/dsh-launcher-tui_0.1.0-1_all.deb`.
The dsh executable must be installed separately and available in PATH.
