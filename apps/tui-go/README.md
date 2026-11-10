# Go TUI preview — checkpoint 6

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

It can read settings from an already running dsh 0.2.2-rc.1. Chat does not connect
to dsh or send messages yet. Submitted text appears as a
local draft card; it is never represented as an assistant reply. Tab focuses the sidebar, Up/Down selects a page, Enter focuses
its content, and Esc returns to chat without losing the draft. Sessions lists backend sessions (refresh with `r`, select with arrows and Enter); Log shows actual local application information. The sidebar
separates **Настройки dsh** (read-only backend snapshots and connection instructions)
from **Настройки TUI** (local client version, interface, terminal size, and workspace).
Local client information stays available with or without a backend login URL.
`/help` lists local commands, `/clear` clears the view, and `/exit` or Ctrl+D exits. Node.js is not needed for attaching the Go client to an existing backend.
Starting dsh locally requires Node.js 24.9+.

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
The text chat itself is still local and does not send ordinary messages.

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
The marketplace host exposes HTTP routes suitable for a future TUI adapter.
Its installation does not automatically translate its HTML/React screens.
Schema/command-based features can use generic terminal controls; custom screens
need explicit adapters. This checkpoint adds preinstallation, not marketplace
search/install/update/remove pages.

## Migration scope

The agreed target is terminal-only operation: launcher capabilities, the dsh
interface, and plugin functionality through dedicated TUI views and adapters.
Arbitrary HTML/JavaScript plugin screens cannot run inside Bubble Tea. Each
adapter needs its own mapping from a backend API to terminal controls and a
behavior test; a generic chat client does not establish plugin compatibility.

Work proceeds in small, reviewable checkpoints with user feedback. After this
session/mode checkpoint, the proposed next checkpoint is a minimal streamed
conversation with an existing dsh. Subsequent checkpoints cover
decisions, settings editing, dsh installation, updates/rollback, diagnostics,
core dsh screens, and named plugin adapters. No full-parity claim is made by
this preview. The legacy JS client and its packaging have been removed; its
streamed chat and dsh installation remain pending in Go.
