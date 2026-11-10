# Go TUI preview — checkpoint 11

[Implementation roadmap and plugin strategy](../../docs/tui-roadmap/README.en.md) ·
[План развития на русском](../../docs/tui-roadmap/README.md)

The terminal client is implemented in Go/Bubble Tea, with read-only dsh settings
and a panel layout: a workspace sidebar, conversation panel, separate
message composer, connection status, and keyboard hints. The sidebar hides below
78 columns. Input history restores drafts; scrolling, resizing, and clean shutdown
remain available. Mouse clicks select sidebar pages and the local draft, and
focus the composer. The wheel scrolls only the content under the pointer.
Mouse capture is enabled by default; `--no-mouse` disables it for native terminal
text selection. Many terminals also allow selection with Shift held down.
Dragging and positioning the text cursor by click are not implemented.
The minimum terminal size is 44 columns by 18 rows.

With a dsh session attached, ordinary text is sent to dsh and assistant text is
streamed over its authenticated WebSocket API. F8 or `/stop` requests cancellation
of the current turn. User messages are reconciled with authoritative history;
service context is not displayed as user-authored text. Without a backend, input
remains a clearly labeled local draft. The static WASM adapter remains offline.

Tool calls and results appear as correlated cards: running, done, error, stopped,
or missing result. Folded output previews three wrapped lines; F6 / Shift+F6
selects cards, Enter or a click toggles their arguments/result, and Esc returns
to the composer without sending or losing its draft. Arrow keys select cards while
they have focus; PgUp/PgDn scroll long output. Reconnection restores cards and
expansion choices without duplicating results. If a call predates the history
window, the result is explicitly labeled rather than assigned an invented name.
Arguments are bounded to 64 KiB, result text to 1 MiB, and retained chat to 4 MiB;
truncation is labeled. Non-text results remain placeholders. Incremental stdout,
nested PTC/subagent trees and question forms are not implemented.

dsh permission requests open a separate scrollable dialog containing the tool,
reason, and available call arguments. No action is selected on arrival: use
Left/Right or Tab to select Reject / Allow, then Enter, or click the button.
Permission is one-shot. Esc defers without deciding; F7 reopens the current
request; F8 cancels the agent. The original draft and other panels are preserved.
The client subscribes to `$events` and answers through `$events/result` using
exactly the receiving authenticated Peer. Disconnection disables answers;
reconnection replays outstanding requests. An uncertain answer is not resent.
Withdrawals and decisions from another client remove stale dialogs. Actual
`approval/asked` / `approval/decided` history displays the authoritative outcome.
The queue displays up to 32 requests and reloads excess pending requests after
that batch. Unsupported question interactions are delegated without inventing
an answer. No permanent allowance or permission-policy editing is implemented.

Tab focuses the sidebar, arrows select a page, and Enter focuses its content.
Esc returns to chat without losing the draft. Sessions supports refresh (`r`),
selection, and attachment; Log shows local client information. **Настройки dsh**
remains a separate read-only backend section. **Настройки TUI** opens from
**Файл → Настройки TUI…**, F4, or Alt+T. F10 or Alt+F/V/H opens File/View/Help.
Dialogs preserve page, scroll, draft, and focus. Mouse clicks work on menus and
settings controls. Help → About works offline.

Mouse and input-history preferences apply immediately and are saved atomically
in `${XDG_CONFIG_HOME:-~/.config}/dsh-launcher-tui/preferences.json`. Disabling
history clears recall entries without erasing the current draft or chat records.
Chat messages are not stored in this preferences file. `--no-mouse` overrides the
saved preference for this run; use F4 to restore mouse reporting otherwise.
A failed write leaves the old setting active. Electron graphics/window controls
are excluded. The local WASM adapter applies settings for its current session.

`/help` lists commands, `/clear` clears the local view without deleting backend
history, and `/exit` or Ctrl+D exits. Attaching the Go client needs no local Node;
starting dsh requires Node.js 24.9+.

Build with Go 1.24.2+:

```sh
cd apps/tui-go
go test -race ./...
go build -o ../../build/tui-go/dsh-launcher-tui-go .
../../build/tui-go/dsh-launcher-tui-go
```

The maintained terminal libraries are Bubble Tea, Bubbles (`textinput` and
`viewport`), and Lip Gloss. The build produces a separate executable named
`dsh-launcher-tui-go`, so it can be tried alongside the existing client.

[Static browser preview build and limitations](web/README.md).

## Read dsh settings

Start dsh separately, then pass its login URL through the environment (avoids
putting it in the client process arguments), or use `--url URL`:

```sh
DSH_TUI_URL='http://127.0.0.1:3080/?token=YOUR_TOKEN' ./dsh-launcher-tui-go
```

Only loopback HTTP(S) URLs are accepted; remote hosts can use an SSH tunnel.
The client signs in through `/connect` and reads `/api/settings/describe`.
It never calls settings write endpoints or loads the settings document from disk.
Open **Настройки dsh** with the mouse or sidebar keyboard navigation. Left/Right switches
namespaces, `r` refreshes the snapshot, and the wheel or PgUp/PgDn scrolls values.
Loaded namespaces are sorted by their backend names; fields retain API names.
Schema-declared secrets show only whether they are set. Neither the schema nor
base/user layers are rendered. A failed refresh retains the previous snapshot
and marks it stale. Reads have a timeout and are cancelled on client shutdown.
Settings remain a point-in-time read-only view without automatic change subscriptions.
An attached dsh is left running when the client exits.

## Streamed text chat

Use `--url` / `DSH_TUI_URL` with `--new` or `--session ID`, or run `/new` after
attaching. Wait for chat readiness, then press Enter to send. Text entered before
readiness, during a session switch, or while generation is running stays in the
composer. This milestone does not provide general queue editing or steering.

The client signs in through `/connect`, sends `session/prompt` with a unique
requestId, and subscribes to `session/follow` through `/api/remote.mux` with
assistantStream enabled. It restores a bounded recent history window (50 messages),
reassembles partial output after reconnect, and checks event cursors/chunk indexes.
Duplicate frames do not duplicate text; a committed answer replaces its partial
view. Missing frames trigger a fresh snapshot. Scrolling up disables following the
tail; statuses and new text stay visible when already following the tail.

Read reconnection is automatic, with bounded backoff. Prompt submission is never
automatically retried. A lost acknowledgement is labeled unknown; durable user
history can resolve it. `/retry-send` explicitly retries the original immutable
request with the same requestId, using dsh's deduplication. An explicit rejection
restores the text if the composer is empty. Pending receipt state is retained
across session switches in this client process, not across client restarts.
Changing sessions closes only the old subscription, not the remote agent.

F8, the composer Stop button, or `/stop` calls `session/cancel`; it does not remove queued backend messages.
The client displays confirmed turn completion/cancellation and safe error hints.
Provider error bodies are not copied into logs or chat. Permissions use their
own dialogs; question forms remain pending and F8 stays available. Rich Markdown,
attachments, and full paginated history remain later work. Replacements referencing history outside the loaded window may require
a new session; the client reports this instead of guessing an order.

The supported API was verified against actual dsh 0.2.2-rc.1 with a controlled
local model endpoint. This tests protocol integration, not an actual cloud LLM.
No working cloud model credential is configured in the private live deployment.

Optional integration check (Node 24.9+, installed dsh, Python environment with pyte):

```sh
python apps/tui-go/test/real_chat.py --binary /path/to/dsh-launcher-tui-go --dsh /path/to/dsh
```

It uses an isolated temporary profile and a local test endpoint, performs no paid
model calls, and does not alter the user's profile. CI also runs this check.

## Sessions and agent modes

Attach a session or explicitly create a new one:

```sh
DSH_TUI_URL='http://127.0.0.1:3080/?token=YOUR_TOKEN' ./dsh-launcher-tui-go --session SESSION_ID
# Or use --new instead of --session.
```

Inside the client, `/sessions` lists sessions, `/session ID` attaches one, and
`/new` creates one. Mode controls sit above the message input and support clicks,
`F2` (ordinary/plan), and `Alt+1/2/3` (ordinary/plan/goal). `/mode default`,
`/mode plan`, and `/mode goal` provide the same controls. Selecting Goal prepares
`/goal ` in the composer; entering an objective and pressing Enter explicitly
creates the goal. `/goal pause`, `/goal resume`, and `/goal clear` control it.
Goal mode can start autonomous work in dsh; it is not just an appearance setting.
Ordinary text is sent through the chat API when the session subscription is ready.

The client reads actual session projections/state every two seconds and after
commands. It shows the backend-confirmed mode, pending plan transitions, goal
phase, and errors. Changes are not shown optimistically. Returning to ordinary
or plan pauses an active goal, preserving it. Goal mutations use its current
identity/revision; failed multi-step changes reconcile the actual backend state.
Requests entered during a background read are queued rather than lost.

## Local startup and marketplace preinstallation

With dsh already installed and Node.js 24.9+ available:

```sh
./dsh-launcher-tui-go --start --profile web
# Optional: --dsh /path/to/dsh --cwd /project --port 3081
```

Before starting its own backend, the client checks
`$DSH_HOME/profiles/PROFILE/package.json` (default `~/.dsh`). If `dshmarket` is
absent, it offers Install, Not now, or Don't ask again. The latter is saved in
`${XDG_CONFIG_HOME:-~/.config}/dsh-launcher-tui/market-prompt.json`.
Installation uses `dsh plugin --profile PROFILE add dshmarket`, the same command
as the GUI. Missing pnpm is installed with npm into the current user's
`${XDG_DATA_HOME:-~/.local/share}/dsh-launcher-tui/tools`; no global install or
sudo is used. The resulting PATH is supplied to dsh. Command output is shown
live before the fullscreen client opens; failures allow retry or skip.

`--market install` explicitly requests installation; `--market skip` skips it.
These apply to `--start`. Attach mode never alters another running backend's
profile. Locally started dsh gets a new session automatically and is stopped
with the client. Existing attached backends are not stopped.
The dsh package itself must already be installed; its installer is not ported yet.

JS plugins run inside Node.js/dsh independently of this client's Go code.
The marketplace host exposes HTTP routes used by the marketplace TUI adapter.
Its installation does not automatically translate its HTML/React screens.
Schema/command-based features can share terminal controls where the backend
exposes sufficient metadata. A declarative plugin UI contract is planned; complex
workflows need dedicated adapters. The marketplace page is already available.

## Marketplace TUI

Open **Маркет** in the sidebar or enter `/market`. The adapter is validated
against dshmarket 1.73.17 on dsh 0.2.2-rc.1 and requires its authenticated API v1.
The client reads the catalog, profile declarations, physically present packages,
and progress from dshmarket. `/` edits the search, Enter returns to the list,
Up/Down or the wheel selects a plugin, `t` switches to installed packages,
and `r` refreshes. Mouse clicks work on the search, rows, tabs, and actions.

`i` installs the selected catalog entry, `u` updates its package, and `d` removes
it. Each operation shows the exact package/profile and requires `y` or Enter;
`n` or Esc cancels the confirmation. Pasted text cannot confirm an operation.
Writes use the market's existing endpoints with no force, drain, build-script
approval, or automatic restart flags. Package state is reread after completion
or refusal. The profile is checked again before writing. A busy market or
running-agent guard is respected; operations are never retried automatically.
Progress uses the server's phase/counts without an invented overall percentage.

Catalog failure retains access to installed profile data. Declarations whose
files are absent are labeled separately. The market and base/web host bundles
are not modified through this page. Git-only/aliased packages, compatibility
resolution dialogs, build approvals, restart, rollback, and operation cancellation
remain later work. If dsh restarts, obtain a fresh login URL. Closing an attached
client does not cancel a remote operation; reopen the market to check its state.

The browser has an explicitly labeled sample catalog: search, installed filtering,
and confirmation UI work, while writes explain that the native client is required.
The demo never changes any backend packages.

## Migration scope

The agreed target is terminal-only operation: launcher capabilities, the dsh
interface, and plugin functionality through dedicated TUI views and adapters.
Arbitrary HTML/JavaScript plugin screens cannot run inside Bubble Tea. Plugin
support combines background operation, shared schema/command views, a proposed
declarative UI contract, and dedicated adapters for complex workflows. The shared
contract is not implemented yet. Track backend, settings, commands, and complete
screen support separately; a generic chat client does not establish full plugin
compatibility.

Work proceeds in small, reviewable checkpoints with user feedback. The
[roadmap](../../docs/tui-roadmap/README.en.md) is the maintained capability inventory
and milestone order. Milestone 1 is complete: TUI preferences live in the top menu,
while dsh settings remain separate. Streamed text chat is implemented in preview.9; validation with an actual cloud
model remains pending working provider access. Tool-result cards are implemented in preview.10. The next functional checkpoint is
user questions, followed by queue controls. Permission dialogs are implemented
in preview.11 and tested through the actual dsh approval service. Settings editing, workspace features,
additional plugin support, and launcher lifecycle remain planned. No
full-parity claim is made by this preview. The legacy JS client and its packaging
have been removed; its dsh installation flow remains pending in Go.
