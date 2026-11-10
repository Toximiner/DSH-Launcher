# Go TUI preview — checkpoint 4

This interactive migration checkpoint adds read-only dsh settings to the panel
layout next to the working
JavaScript client in `apps/tui`: a workspace sidebar, conversation panel, separate
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
its content, and Esc returns to chat without losing the draft. Sessions shows
only the local draft; Log shows actual local application information. Without a login URL, Settings
shows local window information and connection instructions.
`/help` lists local commands, `/clear` clears the view, and `/exit` or Ctrl+D exits. Node.js and npm
are not needed to run this preview.

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

## Read dsh settings

Start dsh separately, then pass its login URL through the environment (avoids
putting it in the client process arguments), or use `--url URL`:

```sh
DSH_TUI_URL='http://127.0.0.1:3080/?token=YOUR_TOKEN' ./dsh-launcher-tui-go
```

Only loopback HTTP(S) URLs are accepted; remote hosts can use an SSH tunnel.
The client signs in through `/connect` and reads `/api/settings/describe`.
It never calls settings write endpoints or loads the settings document from disk.
Open Settings with the mouse or sidebar keyboard navigation. Left/Right switches
namespaces, `r` refreshes the snapshot, and the wheel or PgUp/PgDn scrolls values.
Loaded namespaces are sorted by their backend names; fields retain API names.
Schema-declared secrets show only whether they are set. Neither the schema nor
base/user layers are rendered. A failed refresh retains the previous snapshot
and marks it stale. Reads have a timeout and are cancelled on client shutdown.
This is a point-in-time view, without automatic change subscriptions, editing,
backend installation/startup, or chat transport. The existing dsh is left running.

## Migration scope

The agreed target is terminal-only operation: launcher capabilities, the dsh
interface, and plugin functionality through dedicated TUI views and adapters.
Arbitrary HTML/JavaScript plugin screens cannot run inside Bubble Tea. Each
adapter needs its own mapping from a backend API to terminal controls and a
behavior test; a generic chat client does not establish plugin compatibility.

Work proceeds in small, reviewable checkpoints with user feedback. After this
read-only settings checkpoint, the proposed next checkpoint is a minimal streamed
conversation with an existing dsh. Subsequent checkpoints cover sessions,
decisions, local backend management, settings, updates/rollback, diagnostics,
core dsh screens, and named plugin adapters. No full-parity claim is made by
this preview. The existing JS packages on Pages remain the working distribution
during migration.
