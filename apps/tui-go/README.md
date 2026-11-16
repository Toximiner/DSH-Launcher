# Go TUI preview — 0.2.0-preview.30

[Implementation roadmap and plugin strategy](../../docs/tui-roadmap/README.en.md) ·
[План развития на русском](../../docs/tui-roadmap/README.md)

The terminal client is implemented in Go/Bubble Tea, with read-only dsh settings
and a panel layout: a workspace sidebar, conversation panel, separate
message composer, connection status, and keyboard hints. The sidebar remains
visible throughout the supported size range. Input history restores drafts; scrolling, resizing, and clean shutdown
remain available. Mouse clicks select sidebar pages and the local draft, and
focus the composer. The wheel scrolls only the content under the pointer.
Mouse capture is enabled by default; `--no-mouse` disables it for native terminal
text selection. Many terminals also allow selection with Shift held down.
Dragging and positioning the text cursor by click are not implemented.
The minimum terminal size is **80 columns by 24 rows**; **100×30** is recommended.
At 80×24 the sidebar, composer, and dialogs remain visible, with about 50 columns
for conversation text. Below either minimum dimension, a size notice replaces the
interface. Drafts and open dialogs are retained, and hidden controls ignore keys
and mouse input. Ctrl+C/Ctrl+D still exit; F8 can stop the live agent. Backend
subscriptions continue and the interface returns when the window is enlarged.

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
truncation is labeled. Non-text results remain placeholders. Subagent session trees
remain planned. Nested PTC tool cards are implemented in preview.17.

Preview.18 adds live command stdout/stderr through non-consuming `job/follow`
streams. Durable `job/registered` events bind each job to its exact tool call and
root, including PTC children; copied history never transfers execution ownership.
The folded card follows the latest output; F6 → Enter expands it. The final tool
result supersedes the preview. Reconnection resumes original UTF-8 byte offsets
without rerunning the command or consuming the model's output cursor. Missing
bytes, the 128 KiB display tail, and disconnected output are labeled. At most 16
running calls are observed concurrently. This requires a backend job registry,
job controller, and tool-emitted job association; arbitrary plugin output and PTC
console logs without a job stream are not synthesized. Streams are process-local:
a dsh restart loses their rings; completed output is restored from tool results.

Nested calls use actual `tool/ptc-dispatch-start` and `tool/ptc-dispatch`
events. The client pairs `subCallId`, `parentCallId`, and `rootCallId`; it does not
infer parents from names, ID punctuation, or timing. Descendants appear under their
parent in submission order with their own state, arguments, result/error, F6
navigation, expansion, and mouse hit areas. Folding a parent folds only that card's
details. Indentation is capped to keep narrow terminals readable. Missing parents
and invalid/cyclic chains are explicitly labeled instead of inventing a tree.
Reconnect restores the hierarchy from durable events without rerunning tools.
Replacing a retained parent removes its obsolete descendants. Unknown-turn orphan
calls remain unknown when a turn closes; cancellation is claimed only from a
structured abort or a known cancelled owning turn. Generic plugin wrappers that
emit no PTC relationship events have no inferred hierarchy. This is not the
subagent session UI planned for milestone 10.

The pinned dsh 0.2.2-rc.1 npm package omits its advertised `lib/process.js` PTC
worker. For integration checks only, `test/prepare_ptc.py` downloads the official
source commit `fcc0ad462172522aaf5c93a7b2e01f6b9f0b0159`, verifies the archive SHA-256
and package version, and preserves its MIT license/provenance. Pass its
`process-entry.ts` with `--ptc-bootstrap` to `test/real_chat.py`; the test copies
that worker and four installed dependencies into its isolated workspace so dsh's
file sandbox can load them. Node 24 runs this official TypeScript bootstrap.
The actual dsh executor, sandbox, and event pipeline remain active; no nested
events or outcomes are synthesized by the client. The live stand and distributed
TUI contain no worker workaround. Actual PTC use needs a complete dsh worker
installation; an incomplete backend reports its native failure in the root card.
Checks cover actual nested reads (success/error), a held test tool, client restart,
and cancellation of a real nested bash command. No paid model calls are made.

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
Actual-dsh checks verify cancellation, retained work, reconnect, and 80×24 sizing.

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
own dialogs; agent question forms are available and F8 stays available. Additional Markdown features,
attachments, and richer history rendering remain later work. Replacements referencing history outside the loaded window may require
a new session; the client reports this instead of guessing an order.

The supported API is covered by actual dsh 0.2.2-rc.1 with a controlled local
model endpoint in CI. On November 16, 2026, the native client was also checked
with the GUI-configured Qwen3.8-27B provider in an isolated temporary profile:
a committed model reply and live command output before completion were observed.
Credentials are not included in the client, tests, or private deployment.

Optional integration check (Node 24.9+, installed dsh, Python environment with pyte):

```sh
python apps/tui-go/test/real_chat.py --binary /path/to/dsh-launcher-tui-go --dsh /path/to/dsh
```

It uses an isolated temporary profile and a local test endpoint, performs no paid
model calls, and does not alter the user's profile. CI also runs this check.

## Paginated session history

F11 or `/history` opens a separate, read-only history window. Left reads an older
page; Right returns to a newer visited page at its saved scroll position. Up/Down,
PgUp/PgDn, Home/End, and the mouse wheel scroll within a page. The toolbar buttons
also support clicks. Esc or F11 closes the window and restores the composer focus
and draft. F8 remains available to stop the live agent.

Each page requests 20 user/assistant messages through `session/events`, with one
fixed `throughSeq` cut across older/newer navigation. Live streaming continues
behind the window without moving its history. F5 (New) explicitly reads the newest
page at the live chat's current cut and clears the navigation trail. Switching
sessions or closing the window cancels outstanding reads; late responses cannot
reopen it. Historical replay uses isolated state and never acknowledges a pending
prompt, changes the live agent mode, or starts a tool/output subscription.

The client retains one rendered page and the last 128 return locations. Search
also retains its most recent matching API page (bounded to 8 MiB). A page
is limited to 100 displayed records, 4 MiB of displayed content, and 4096 events;
the API response limit is 8 MiB. Oversized pages are reported explicitly while
retaining the previous page.

Preview.30 builds an immutable metadata index by reading the complete fixed log
prefix when history opens or F5 refreshes. It retains surface positions and tool
call context, rather than all message/result bodies. Limits are 100,000 events,
64 MiB of cumulative event data, and the existing 20-second request deadline.
Failure preserves the previous page/index; closing cancels the read. Page reads
and searches reuse this index until refresh. Replacement endpoints are resolved
in surface order, including numerically reversed sequence numbers and repeated
replacements. Older raw-log pages filter out replaced messages and obsolete tool
versions/descendants. Remaining surface messages within each page use surface
order; page boundaries still follow raw log ranges, so a page can become empty.
Arguments/names are restored for result cards whose call is outside the page.
Unfinished calls at the cut have an unknown result.
History does not yet persist navigation across client restarts.

Preview.21 adds **`/rename`** for the connected session and **F3** on the Sessions
page for the selected entry. The editor starts with the current title, Enter
saves through `session/rename`, and Esc discards the edit. Chat drafts remain
intact. Names in the list and chat header come from dsh's `title` projection;
untitled sessions use their IDs. The backend normalizes titles, including spaces
and its configured byte limit. Only its confirmed title is shown as saved.
A failed or uncertain request retains the editor and never retries automatically;
check the refreshed list before resubmitting. Browser demo mode cannot rename.
This operation sends no model prompt and persists in the shared dsh profile.

Preview.22 adds **`/fork`** for the connected session and **Ctrl+B** on the
Sessions page for the selected entry. A confirmation describes the boundary:
Enter creates through `session/fork`; Esc cancels. dsh chooses the latest
completed-turn prefix at request time. An active, unfinished turn stays in the
source session, which continues running. A session with no completed turn cannot
use this action. Preview.23 adds exact committed-message boundaries from history.

The confirmed child opens automatically and retains the composer draft. Its
inherited messages become context when you submit a new prompt; creating the
branch itself sends no model request. dsh resolves the child's workspace, preset,
and default model. The request uses `allowMigration: false` so a cold source that
needs migration must first be opened in dsh. Failed or uncertain creation does
not retry, including repeated Enter: close the dialog and refresh the list before
trying again. If creation succeeds but connecting fails, the acknowledged child
ID and `/session <id>` recovery command remain visible. Browser demo mode cannot
fork. Actual-dsh tests cover an active source, excluded unfinished output,
independent child continuation, inherited model input, and a source without a
completed turn.

Preview.23 adds **Tab/Shift+Tab** in history to select committed user/assistant
messages, or click the message body. **Ctrl+B** opens an exact-boundary fork
confirmation with the immutable event seq and a short excerpt. Search hits that
are committed conversation messages are selected automatically. The request adds
`atSeq` with the selected event's inclusive seq, independent of later live output.
Only conversation messages are selectable: tool/approval cards, attempt previews,
and hidden service context are excluded. Paging resets selection; resize retains
it. Selection stays within the current page; use older/newer pages or search to
reach other messages. Search editing consumes its own keys, and an outstanding
history read blocks selection and branching.

An exact child includes the selected message and excludes later source events.
dsh appends its seed marker and synthetic closers for open steps/turns. Creating
it stays idle until a new prompt is submitted. Esc returns to the same reader,
selection, scroll position, and composer draft. A successful creation closes the
reader before connecting to the child, including when connection must be retried.
Event numbers are source log positions, rather than message indexes. Selecting
arbitrary service/tool events is not exposed by this UI.

Preview.24 adds **`/archive`** and **`/restore`** for the connected session.
On the Sessions page, **a** switches active/archive lists, and **Ctrl+X** archives
or restores the selected entry with confirmation. Both filters share the complete
session list; actions use the visible entry's identity. History remains readable,
including search and branching. Archived sessions keep unsent composer text;
sending and mode changes are blocked until restoration. Restoring itself submits
no model prompt. Selection scrolls into view in longer lists, including at 80×24.

Archive membership comes from `workspace/follow` baselines and increments, shared
with GUI clients on the same dsh profile. Reconnect replaces membership from a
fresh baseline; disconnected state is labeled while the last observed archive
set continues to guard sending. Archive/restore call `workspace/archiveSession`
and `workspace/unarchiveSession`. Archiving explicitly sets `stopActivity: false`:
the backend refuses if turns, subagents, jobs, or schedules still run. Stop their
work first and retry explicitly. API-confirmed membership replaces local state;
a new stream supersedes updates captured before that confirmation. Uncertain
results preserve the dialog and never retry automatically. Esc closes it so the
archive list can be checked. Browser demo mode cannot mutate archive membership.

**Permanent deletion is planned.** The pinned dsh API has no session-deletion
verb. Implementation needs an authoritative backend operation covering persisted
history, workspace membership, descendants, and active work, plus a dedicated
confirmation in TUI. Archiving remains reversible and retains saved history.

Preview.26 formats **committed assistant messages** in live chat and history:
headings, emphasis/strikethrough, nested lists, quotes, visible link destinations,
fenced/indented code, inline code, and tables. Code keeps indentation and blank
lines; tabs use four spaces. Preview.28 adds syntax coloring for declared fence
languages through Chroma v2.23.1 (compatible with this module’s Go minimum).
Language names and aliases are case-insensitive; unknown/unlabeled blocks remain
plain, with no content or filename guessing. Colors follow terminal capabilities,
including monochrome output. No code is executed. Tables
use terminal cell widths for Unicode alignment; a table wider than its container
becomes labeled records with all cell content retained. Images show their alt text
and destination; HTML remains literal text. Rendering opens no links or files.
Streaming attempts, user messages, and tool cards keep their plain display.

**Ctrl+R in history** switches between formatted assistant text and its source.
The title and footer show the current mode. Source search remains literal and
selects the same event even if markup is hidden by formatting. Message selection,
mouse hit regions, and fork boundaries follow rendered row positions and retain
source event identities. Toggling preserves the selected message and draft; while
a query is being edited, its input handles keys independently.

The parser is pinned to Goldmark v1.7.8. Source text is never rewritten. Renderer
input over 256 KiB, nesting over 64 nodes deep, more than 20,000 AST nodes, oversized
tables (over 32 columns or 2,048 cells), or output over 2 MiB falls
back to plain text. Cached renders retain at most 128 entries / 8 MiB, including
source bytes. ANSI/control sequences are removed from source and decoded entities;
only renderer-generated text styling reaches the terminal. Syntax coloring accepts
at most 16 KiB per block, 64 KiB per rendered message, and 8,192 tokens per block.
A 150 ms message budget is checked between tokens, rather than interrupting an
individual library match. Errors, panics, budget exhaustion, and any token text
mismatch fall back to intact plain code. Colored output is capped at 512 KiB per
block and uses the existing bounded message cache. The code-copy picker preview
stays plain; copying and source mode contain no generated colors.

Preview.29 starts history tool cards **collapsed**, showing an argument summary
and up to three result rows. **F6 / Shift+F6** select the next/previous card in
display order; **Enter** or a click toggles full arguments and the bounded stored
result. Arrow keys and PgUp/PgDn scroll long details; Tab/Shift+Tab return to
message selection. Each nested card expands independently, and collapsing a
parent leaves its children visible. Tool selection clears the message target,
so Ctrl+B/Ctrl+Y/Ctrl+K require selecting a conversation message again.

Expansion preferences are local to this reader and keyed by call ID/start event.
At most 256 expanded cards are retained across pinned page/search navigation,
evicting the oldest preference when full. F5 starts a fresh snapshot with
collapsed cards; closing discards preferences. History folding invokes no RPC
and changes no live tool selection, expansion, observer, agent state, or draft.
In-progress calls at the pinned cut remain explicitly unresolved; viewing an
old card does not start a live output subscription. Shift+F6 uses Bubble Tea’s
F18 decoding in history and live chat.

Preview.27 adds **Ctrl+Y** to copy the selected history message and **Ctrl+K**
to choose a fenced or indented code block. Select a message with Tab/Shift+Tab,
a click, or source search. In the block picker, use ↑/↓ or click to select, Enter
to copy, and Esc to return. Whole messages use exact source including Markdown;
code excludes fences and surrounding Markdown indentation. Tabs, Unicode, and
blank lines are preserved, independently of displayed wrapping. Empty fragments
and fragments over 64 KiB leave the clipboard unchanged. Code extraction accepts
at most 256 KiB, 64 blocks, 20,000 AST nodes, and depth 64. No automatic retries.

On local Linux, available wl-copy, xclip, or xsel helpers receive text on stdin,
without a shell. A successful helper exit is reported as a system clipboard copy.
Without a working helper, and over SSH, the client sends an OSC 52 request to the
terminal: its receipt explicitly says storage is unconfirmed. Terminal clipboard
permissions apply; tmux uses passthrough (allow-passthrough must be enabled), and
screen uses its transport wrapper. Clipboard contents are never read. Closing
history cancels pending helper work; late results cannot overwrite a newer view.
The browser demo uses ordinary browser text selection/copying.

Preview.20 adds **Ctrl+F / Search** inside history. Preview.30 extends the default
scope to conversation text and tool names, arguments, results, and error codes.
In the query editor, **Ctrl+T** cycles **all / messages / tools**; Enter commits
both the literal, case-insensitive query (up to 256 characters) and scope. Esc
leaves the previous committed query/scope intact. Permission records and hidden
service context remain excluded. Search uses the bounded text retained by TUI.

The reader scans raw-log pages from newest to oldest through the fixed cut,
including pages beyond the live chat’s 50-message window, filtering each against
the same snapshot index. A tool matches once at its retained result/start event,
including arguments from an earlier page; obsolete call/result versions and
replaced text cannot return as matches. A tool match selects and expands its card
and scrolls toward the matching displayed text; it clears the message boundary,
so Ctrl+B/Ctrl+Y/Ctrl+K require selecting a conversation message again. Message
matches retain source identities. F3 advances through matches in the validated
page before reading older pages.

A search batch reads at most ten pages with a 20-second timeout. If more history
remains, F3 continues; exhaustion is reported only after reaching the start. Esc
cancels a running search, exits query editing, or closes history, in that order.
Ctrl+F cancels any pending search before editing another query. Late results are
ignored. F5 loads a fresh snapshot/index and resets search continuation. Errors
preserve the visible page instead of claiming no matches. Queries, scopes,
results, and navigation are local to this history window.

Actual-dsh check with a local controlled model, no paid calls:

```sh
python apps/tui-go/test/real_history.py --binary /path/to/dsh-launcher-tui-go --dsh /path/to/dsh
```

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
while dsh settings remain separate. Streamed text chat is implemented in preview.9;
actual-model acceptance was checked with Qwen3.8-27B on November 16, 2026. Tool-result cards are implemented in preview.10. Agent questions are implemented in preview.12, including timed waits and continued
answers. Queue submission and observation are implemented in preview.13; individual
queue removal is implemented in preview.14 and multiline editing in preview.15.
Queue-to-steering promotion is implemented in preview.16; nested PTC calls in
preview.17 and live command stdout/stderr in preview.18. Preview.19 adds read-only
paginated history; preview.20 adds message search and preview.21 adds renaming.
Preview.22 branches at the latest completed turn; preview.23 branches at selected
messages. Preview.24 adds archive/restore, preview.25 raises the minimum to 80×24,
preview.26 formats Markdown, preview.27 adds message/code copying, and preview.28
adds syntax coloring. Preview.29 adds collapsible history tool details; preview.30
adds tool search and cross-page replacement support. Next: models and editable
dsh settings. Permission dialogs are implemented in preview.11 and tested through the
actual dsh approval service. Settings editing, workspace features,
additional plugin support, and launcher lifecycle remain planned. No
full-parity claim is made by this preview. The legacy JS client and its packaging
have been removed; its dsh installation flow remains pending in Go.
