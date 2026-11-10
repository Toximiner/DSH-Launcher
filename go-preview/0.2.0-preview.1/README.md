# Go TUI preview — checkpoint 1

This is the first interactive migration checkpoint, next to the working
JavaScript client in `apps/tui`. The current preview implements terminal input,
input history with draft restoration, scrolling, resize handling, and clean
shutdown through commands, keyboard shortcuts, and process signals.

It does not connect to dsh or send messages yet. Submitted text is echoed as
`Введено: …`; it is never represented as an assistant reply. `/help` lists local
commands, `/clear` clears the view, and `/exit` or Ctrl+D exits. Node.js and npm
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
terminal shell, the proposed next checkpoint is connection to an existing dsh
and a minimal streamed conversation. Subsequent checkpoints cover sessions,
decisions, local backend management, settings, updates/rollback, diagnostics,
core dsh screens, and named plugin adapters. No full-parity claim is made by
this preview. The existing JS packages on Pages remain the working distribution
during migration.
