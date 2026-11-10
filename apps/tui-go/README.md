# Go TUI preview — checkpoint 13

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
nested PTC/subagent trees are not implemented.

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
that batch. Question interactions use the same receiving Peer and request queue. No permanent allowance or permission-policy editing is implemented.

Agent questions open a separate form with one question per page. Tab / Up/Down
focuses an option or the custom field; Enter/Space selects an option. A single
choice clears custom text; multiple choices can accompany custom text. The
custom field supports multiple lines, Unicode, and paste; Enter inserts a line.
Left/Right outside the editor, or the mouse pager, changes questions. Explicit
Skip sends an empty selection for that item. F5 or Answer submits the complete
batch only after every question is answered or explicitly skipped. No initial
selection is sent automatically. Esc defers, F7 reopens, and F8 cancels the agent.
The chat draft is preserved and question answers never use session/prompt.

The form supports up to 32 questions and 64 options each within a 1 MiB request
budget; each custom answer is limited to 65,536 characters. Plan/approval question
intents retain the supplied detail and options without implicit approval. While
connected, the client holds timed waits through userQuestions/attachWait. After
a wait continues on the backend, session userQuestions projections restore its
form and replies use userQuestions/answer. The dsh-tagged late reply appears as
an answer in history. No uncertain reply is retried automatically. Draft progress
survives deferral and read reconnects within this client process, including a timed
question becoming continued. Restarting reloads pending questions; draft answers
are not persisted to disk. Continued reply receipts suppress duplicate forms until
dsh admits the answer. Arbitrary future interaction kinds remain delegated.

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
Chat displays server-confirmed queued tasks, preserving dsh order, with up to
three preview lines per item. Internal context entries are hidden; steering
entries created by other clients are labeled separately. An RPC acknowledgement
is shown as accepted while waiting for queue/history evidence. One unconfirmed
submission per session is allowed; further input remains in the composer until
that receipt is reconciled. A state refresh follows each receipt, and passive
refreshes run every two seconds. A matching queue rpcId or durable user message
confirms receipt; queue absence alone never resolves an uncertain submission.
A later queue sample and caught-up history can retire an accepted receipt that
left the queue outside the recent-history window, without claiming execution.

As each follow-up enters history, its queue item is hidden even if a passive
queue sample arrives late. Reconnecting reads the backend inbox and does not
resend tasks. An unavailable queue is distinct from a known empty queue; cached
entries are labeled if connection/state refresh fails. F8 cancels the current
turn and retains pending inbox work; it does not clear the queue. Actual dsh
integration verifies two waiting follow-ups, client reconnection, and three
individual FIFO turns with no duplicate model requests.

F9, `/queue`, or clicking the inline queue heading opens the queue dialog.
Up/Down selects an item; Enter opens removal confirmation, defaulting to Back.
Right/Tab selects Remove and Enter confirms. Mouse selection and buttons work;
Escape backs out without changing the draft. Removal uses `session/updateQueue`
with the original immutable item ID, leaving the active turn and other items
alone. If an item starts before confirmation, dsh may reject removal; the client
refreshes instead of targeting another item. Stale/disconnected state disables
mutations. An uncertain result never retries automatically; reopen the refreshed
queue and explicitly choose again if necessary. Acknowledgement does not remove
items optimistically from cached server state. Session changes discard the dialog,
and late responses from old stream epochs cannot change the new dialog.
Actual-dsh checks verify cancellation, retained work, reconnect, and 44×18 sizing.

F3 or the Edit button opens a multiline editor for the selected queue item.
Enter inserts a newline; F5 or Save sends `session/updateQueue` with action=edit;
Escape returns to the list without saving. F9 hides/reopens the draft in the same
client session. The private browser toolbar exposes Edit F3 and Apply F5.
The chat composer draft stays intact. Only plain text up to 64 KiB is supported;
attachments, controls, or source text that the editor would transform/truncate
are refused. Empty text cannot be saved. Saving retains the original item ID,
placement, and queue order; no new prompt is submitted.

A task admitted into execution cannot be edited. If a refreshed queue sample
shows another edit, saving is blocked and the local draft remains available.
The dsh API offers no conditional revision write: this catches observed conflicts,
but cannot guarantee atomic conflict protection against concurrent clients.
Rejected/uncertain saves retain the editor, refresh state, and never retry
automatically. Explicit F5 recovery requires a sample begun after the failed
receipt; if it already contains the desired text, the editor closes without a
second mutation. Drafts are local to this client process/session and are discarded
when switching sessions or exiting. Actual-dsh tests verify discarded edits,
multiline model input, reconnect after saving, and retained FIFO work.

While an agent is running, F9 → Ctrl+G (or the Steer button) opens a
confirmation to promote the selected ordinary queued item into steering.
Back is selected initially; Right/Tab and Enter explicitly confirm. This calls
`session/updateQueue` with action=steer and the original item ID; no new prompt
is created. The backend consumes steering on the next step of the same turn,
without interrupting an in-flight response. Remaining ordinary tasks stay queued.
Already-steered items are labeled separately; reconnect restores their placement.

Steering is disabled for idle/disconnected/stale sessions, an open editor, and
items already admitted or moved to steering. A changed sampled text cancels the
confirmation instead of sending an unreviewed version; the API has no conditional
revision guard against a concurrent change after that sample. If the turn closes
before dsh handles the mutation, dsh rejects it; the client refreshes and does not
fall back to a new task. Uncertain results never retry automatically. The original
item ID prevents a second promotion after it has left the next-turn inbox.
Actual-dsh tests verify confirmation, same-turn journal events, next-step model
input, preserved ordinary work, reconnect, and minimum terminal size.

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
readiness or during a session switch stays in the composer. While generation is
running, Enter queues a follow-up through session/prompt with mode=queue. The
composer hint changes to explain this. Steering is an explicit action in the queue
dialog; the composer continues to submit ordinary queued follow-ups.

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
own dialogs; agent question forms are available and F8 stays available. Rich Markdown,
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
model remains pending working provider access. Tool-result cards are implemented in preview.10. Agent questions are implemented in preview.12, including timed waits and continued
answers. Queue submission and observation are implemented in preview.13; individual
queue removal is implemented in preview.14 and multiline editing in preview.15.
Queue-to-steering promotion is implemented in preview.16. The next checkpoints
cover nested tool calls and incremental stdout. Permission dialogs
are implemented in preview.11 and tested through the actual dsh approval service. Settings editing, workspace features,
additional plugin support, and launcher lifecycle remain planned. No
full-parity claim is made by this preview. The legacy JS client and its packaging
have been removed; its dsh installation flow remains pending in Go.
