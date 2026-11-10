# Go TUI preview — checkpoint 3

This interactive migration checkpoint adds a panel layout next to the working
JavaScript client in `apps/tui`: a workspace sidebar, conversation panel, separate
message composer, connection status, and keyboard hints. The sidebar hides below
78 columns. Input history restores drafts; scrolling, resizing, and clean shutdown
remain available. Mouse clicks select sidebar pages and the local draft, and
focus the composer. The wheel scrolls only the content under the pointer.
Mouse capture is enabled by default; `--no-mouse` disables it for native terminal
text selection. Many terminals also allow selection with Shift held down.
Dragging and positioning the text cursor by click are not implemented.
The minimum terminal size is 44 columns by 18 rows.

It does not connect to dsh or send messages yet. Submitted text appears as a
local draft card; it is never represented as an assistant reply. Tab focuses the sidebar, Up/Down selects a page, Enter focuses
its content, and Esc returns to chat without losing the draft. Sessions shows
only the local draft; Settings and Log show actual local application information.
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

## Migration scope

The agreed target is terminal-only operation: launcher capabilities, the dsh
interface, and plugin functionality through dedicated TUI views and adapters.
Arbitrary HTML/JavaScript plugin screens cannot run inside Bubble Tea. Each
adapter needs its own mapping from a backend API to terminal controls and a
behavior test; a generic chat client does not establish plugin compatibility.

Work proceeds in small, reviewable checkpoints with user feedback. After this
layout checkpoint, the proposed next checkpoint is connection to an existing dsh
and a minimal streamed conversation. Subsequent checkpoints cover sessions,
decisions, local backend management, settings, updates/rollback, diagnostics,
core dsh screens, and named plugin adapters. No full-parity claim is made by
this preview. The existing JS packages on Pages remain the working distribution
during migration.
