# Go TUI preview — session tabs and explicit deletion

The experimental `0.2.0-preview.40-opencode.11` includes visited session tabs,
per-session drafts, compact home/conversation composers, optional right context
and Ctrl+P navigation. Command, session/workspace and model pickers share a flat
frame, measured hit targets, teal selection and a dimmed backdrop. Tool calls
collapse to compact summaries with expandable arguments/results. Permissions and
agent questions use flat decision dialogs. Actual reasoning supplied by dsh is
collapsed separately; Ctrl+O or `/thoughts` toggles it, as do header clicks.
Ctrl+O also works within history. Copying replies preserves the reply source.

Preview `.9` adds a close button to each visited tab. Closing is local: it preserves
that session's draft and running backend agent. The active tab switches to a
neighbor; preview `.11` prevents closing the final open tab. Reopening a
conversation within the same process restores its draft. Tab closing was
compared with actual installed OpenCode 2.3.0; our close button remains visible
without requiring mouse hover.

Preview `.11` restores the ordered open tabs and active conversation after restart.
Only session IDs are saved, separately for the backend origin, profile and DSH_HOME;
login tokens, conversation text and drafts are not saved. A fresh backend catalog
filters out deleted sessions before restoration. Explicit `--session` or `--new`
takes priority over the saved active conversation. An unavailable backend leaves
the saved list intact. `--start` creates a session only when there is no saved list.

Permanent deletion is a separate action: Delete in `/sessions-manager`, or
`/delete-session` for the current session. Cancel is selected by default. In
preview `.10`, TUI calls `launcherSessions/deleteSession` from the bundled
`packages/launcher-backend` module; Archive Manager is no longer required.
TUI rereads the target and activity; the backend serializes deletion, checks
activity throughout the subagent tree through native agents, pending input and
activity providers, and refuses active work. It does not stop agents automatically.
The backend removes session-owned artifacts and metadata recursively for
subagents; project files and separate fork sessions remain. Unsupported native
storage/lifecycle methods refuse deletion. There is no client-side filesystem
fallback or automatic retry after an unconfirmed result: check the list first.
Confirmed deletion forgets the local tab and draft. Deletion across multiple
session artifacts is not a global filesystem transaction.

GUI and Go TUI automatically attach the bundled module with `--patch` when they
start their own dsh backend. Assets are copied into private launcher-owned cache
storage; no user profile patch is edited and no npm/plugin installation is needed.
An already running older or external backend lacks this module: restart it
through the updated launcher to enable deletion. The launcher does not inject
code into a running backend or forcibly terminate an external process. GUI
startup loads the backend capability; this preview adds no native GUI deletion
button.

Repeated headings in the session picker, dsh settings, session manager and log
were removed in the preceding header cleanup; loading/error/workspace context
remains available.

Preview `.8` replaces the GUI-style settings panes with a compact searchable,
grouped list retaining all 15 settings actions and current values. Left/Right
changes local theme, language, mouse and input-history preferences; dsh settings
still open explicit revision-checked forms. The shortcut reference, schema forms
and small launcher dialogs use flat surfaces. F10 terminal selection and F7
pending-request access remain available.

Auxiliary-screen migration (stage 4) is in progress. Plugins, market, automation,
files, terminals, subagents, queue/history, runtime install/update/rollback and
diagnostics still await this visual pass. The separate preview does not replace
the installed application.

From the repository root, run the separate local build:

```sh
./build/previews/opencode-11/dsh-launcher-tui --start
```

The binary remains separate from earlier builds. Main entry points:

- Ctrl+P: actions; F4: TUI settings; F11: history; F12: models.
- Alt+S or `/sessions`: search sessions and workspaces. Enter on a workspace
  filters sessions; Ctrl+N on it explicitly creates a session in that workspace.
- F2 or `/sessions-manager`: full session management. F3 or
  `/workspaces-manager`: full workspace management from the picker.
- F2/Tab in models changes reasoning; Enter applies; Esc cancels. The shared
  default side effect and next-step behavior are disclosed in the model dialog.

`actions.go` defines palette actions, exact no-argument command aliases,
base-screen keys and generated help rows. Argumentful commands and local
editor/list keys remain context-specific handlers. See the
[rebuild plan](../../docs/tui-rebuild-plan.en.md) for completed scope and next stages.
Older interface descriptions below record screens still being migrated.

[Implementation roadmap and plugin strategy](../../docs/tui-roadmap/README.en.md) ·
[План развития на русском](../../docs/tui-roadmap/README.md)

## Plugins page (preview.40)

Sidebar Plugins and `/plugins` open a page with built-in settings, experimental
bundles and installed third-party packages. Names/descriptions come from dsh's
localized package metadata, with package names as fallback. Built-in cards open
the exact served settings namespace using the existing revision-checked form.
The GUI's pinned optional-bundle order and More grouping are preserved; Ctrl+O
expands the secondary list, and search also finds those secondary packages.

Up/Down, Home/End and PgUp/PgDn select cards; Enter opens settings/details.
Space or the displayed On/Off control prepares an explicit confirmation. Before
writing, TUI rereads the bundle's observed enablement; changed/removed/read-only
targets require refresh. This check is not atomic compare-and-swap. After an
acknowledged action, a read-only refresh reconciles the displayed state. Writes
never retry automatically; application/restart outcomes remain visible.
Ctrl+N adds, F3 updates and Ctrl+X removes supported packages through the existing
confirmation, install cancellation and receipt workflows. F6/Right opens Market.

F2 opens the separate read-only Loader-component view. Internal modules are not
misrepresented as user-manageable packages. A host without profile management
still exposes its served settings and component inventory; Add is refused.
Global menus and sidebar navigation remain available from the page. Opening
Plugins through settings retains a modal return path. Checks cover 80×24 and
larger sizes, exact switch hit areas, cancellation, actual profile toggle/restore,
settings forms and unchanged model-call counts in an isolated dsh profile.

## Settings hub (preview.40)

F4 / Alt+T opens a GUI-shaped modal with General, Models, Built-in plugins,
Agent presets and Plugin Market sections on the left. Values and descriptions
appear on the right. Tab or Left/Right changes panes; Up/Down and Home/End select;
Enter/Space activates. The mouse selects sections, activates rows and scrolls
the parameter list. The selection stays visible when resizing down to 80×24.

General contains TUI language, theme, keyboard reference, mouse, input history
and sidebar preferences. These retain their private, atomic persistence. Font
size belongs to the terminal settings. The default-permission row opens the
shared dsh `permission.defaultPreset` field in the existing revision-checked form;
Enter stages a change and F5 saves it. This changes defaults for future sessions,
not the current session's permission. Models, plugins and presets open actual
panels over the hub; Escape returns to settings. Shortcuts also return to settings.
The configuration button opens the redacted dsh settings browser, and Market
opens the existing catalog page. F5 refreshes the settings snapshot explicitly.

F1 groups the keyboard reference into navigation, chat/input, sessions/history,
settings/editors, questions/approvals, files/plugins/account and background work.
Keys and actions have separate columns; contexts are muted. Tab/Left/Right or
the arrow buttons switch between All sections and individual groups. Typing
searches the selected group, including its name and each action's context;
Ctrl+U clears the query. Up/Down, PgUp/PgDn, Home/End and the wheel scroll while
keeping the current section heading visible. Descriptions wrap without losing
the original combinations. Escape returns to settings when opened from there.

## Active conversation (preview.40)

Started conversations use a centered column capped at 100 terminal cells. User
messages sit on the right; assistant Markdown stays on the left. The header shows
the session and agent preset, Chat/Trajectory tabs and a session action menu.
Trajectory opens the existing durable history (F11). The bottom composer keeps
the editor (+), permissions and actual model/reasoning controls available.

Committed messages show their event time when available; assistant reply duration
uses the matching step start. The status line uses dsh projections for turns,
steps, decoding speed, cumulative tokens, cache reads and approximate context
pressure. Missing figures are omitted; a fresh projection can lag a live reply.
Click Copy below a message, or press Ctrl+Y to copy the latest committed assistant
reply, preserving the source Markdown and composer draft. The existing clipboard
transport and 64 KiB limit apply; streaming text and tool cards are excluded.
Layouts and mouse targets are checked at 80×24, 100×30 and larger sizes.

## Empty-session landing screen (preview.40)

An empty ordinary session uses a centered composer rather than an empty history
panel. Workspace and agent-preset controls sit above it; the editor, permission
preset and actual model/reasoning selection sit below the input. After messages,
queued work or a live reply appear, the conversation uses the normal history and
bottom composer. Archived sessions, subagent viewers and active goals retain
those existing views. Background refreshes keep the landing layout stable.

Click the controls or use Alt+W (workspaces), Alt+A (agent presets), Alt+P
(permissions), Alt+E (multiline editor) and F12 (models). Opening controls preserves
the draft. The model catalog and preset labels come from read-only dsh APIs,
refreshed without model calls. Permission controls are shown only when the session
advertises a selection. The picker reads permissionPresets/catalog, confirms an
explicit /permission command, checks the observed selection again, and rereads
its result. A conflict or unconfirmed write requires manual refresh; writes never
retry automatically. The API has no atomic compare-and-swap for this command,
so a later concurrent change remains authoritative. Agent presets retain dsh's
before-first-turn restriction. No unsupported @mention behavior is advertised.

## Sidebar (preview.40)

The sidebar follows the GUI structure: New Session, Plugins, Plugin Market,
Automation tasks and More at the top, followed by actual dsh workspaces with
collapsible session lists. Tools under the upper More menu include sessions,
files, terminals, presets, commands, packages, agents, team tasks and jobs.
The lower More menu stays fixed and opens TUI/dsh settings, the DeepSeek account,
shortcuts, diagnostics and About. Existing Alt+C/S/D/L/M shortcuts still work.

Tab focuses the sidebar; Up/Down select without running actions, Enter opens,
Left/Right collapse/expand a workspace, and Home/End select the first/last entry.
F5 refreshes the workspace snapshot; Ctrl+F opens the searchable workspace panel.
Mouse clicks activate entries and the wheel scrolls the workspace tree separately.
New Session uses the selected workspace, otherwise the current session's workspace.
Snapshots refresh every ten seconds without model calls; failures keep the last
snapshot, and switching backends discards the old tree. Archive membership remains
managed by the existing live subscription. All layouts fit 80×24 and larger.

## Starting when dsh is already running (preview.39)

`dsh-launcher-tui --start` checks the configured loopback port first. If dsh is
already running, the client discovers its login URL from a regular stdout file
owned by the same user and verifies authentication and the session API before
connecting. It skips local Node/dsh installation and marketplace setup in this
case. No second backend is started and an external backend is not claimed.
If the port is occupied but login discovery fails (for example, stdout is a pipe),
the client asks for `--url` or `DSH_TUI_URL` with a fresh login URL.
Failed startup does not overwrite an existing managed backend identity, and
startup diagnostics are drained before returning an error.

## Shared GUI/TUI backend lifetime

Updated GUI and TUI clients record their local backend port and Linux process
identity in `${XDG_DATA_HOME:-~/.local/share}/dsh-launcher/backends`. Closing either
client (including SIGTERM/SIGHUP) preserves dsh while the other client is alive.
The last client stops the backend started by either launcher. Stale client PIDs
are ignored; PID start ticks prevent signalling reused PIDs. A manually started
dsh without a launcher ownership record remains external and is left running.
Both frontends must include this change and use the same user/data directory.
Explicit Stop/Restart dsh actions remain explicit process operations.

## Installation, updates and diagnostics (preview.37)

- `/runtime` (also File → dsh): Node/dsh discovery, user-local npm installation
  with live logs and retry, owned start/stop/restart, executable/profile/port/cwd.
  Installations live under `${XDG_DATA_HOME:-~/.local/share}/dsh-launcher-tui/versions`;
  global npm and the GUI are unchanged. The dsh profile remains shared.
  A filesystem lock excludes concurrent process/install owners.
- `/updates`: npm/GitHub versions and release notes, exact/latest dsh installation,
  atomic promotion of a verified slot and previous-slot rollback. Stop owned dsh
  before changing versions. Binary rollback does not roll back profile data.
- TUI updates use an architecture-matched `tui-v…` release binary with SHA-256 or
  a local file with an explicit SHA-256. The client checks `--version`, writes a
  `.previous` backup, replaces atomically and restores automatically if the
  post-install check fails. Recover manually through the menu or run
  `./dsh-launcher-tui-go.previous --rollback-tui` if the new client cannot open.
  The executable directory must be writable; restart explicitly to use the new
  version. Tag-triggered binary publication is prepared in CI; no release has
  been published yet. Missing compatible release assets produce an explanation.
- `/diagnostics` (also Help → Diagnostics): bounded owned-dsh stdout/stderr and
  operation output (256 KiB), previous-run log and a private 0600 tar.gz report.
  URL tokens, Bearer/Basic, keys, passwords and cookies are masked, including
  writes split across chunks. Profiles, environment and conversation history are
  not collected. Review arbitrary log text before sharing. Operations, completed
  turns and user requests produce journal notifications; BEL sound is opt-in.
  Background status remains in the existing snapshot panels; this does not poll
  every job/schedule in the background.

Verified actual pinned-dsh npm installation into a new HOME, startup with empty
DSH_HOME, owned start/restart/exit and preservation of an externally owned dsh.
A copy of the actual TUI binary verifies update and `.previous` recovery. Go
checks cover failed/cancelled npm installs, ownership locks, durable slot pointers,
failed TUI post-install checks and report masking. UI checked at 100×30/80×24.
The baseline client work for stages 11–13 is complete; public TUI delivery remains
unverified until the first dedicated release.

Typical recovery:

```sh
# Start the previous binary to restore the main executable even if it cannot run.
./dsh-launcher-tui-go.previous --rollback-tui
# Restart manually after a successful update or rollback.
./dsh-launcher-tui-go --start --market skip
```

Runtime defaults are saved in the TUI data directory's `runtime.json`.
Explicit `--dsh/--profile/--port/--cwd` override saved launch defaults.
`/runtime` starts without a market-install prompt; `--start --market ask`
retains the prelaunch marketplace workflow. A managed installation takes
precedence over the fallback `--dsh` executable. The first managed installation
can roll back to that fallback only while it remains available. npm installation
uses `--ignore-scripts`; Node.js 24.9+ and npm must already be present.
The launcher does not install Node or migrate/downgrade dsh profile data.

Local TUI installation accepts `/absolute/path/to/binary SHA256` (paths may contain
spaces). The checksum is a corruption check, not independent publisher identity;
use a trusted build. Staging, `.previous` and `.update.json` are beside the executable.
Repeated installation of identical bytes leaves the existing backup intact.
A failed/unfinished npm install never promotes its slot; interrupted disk promotion
leaves either old or new complete bytes plus the backup. A successful `--version`
check is not a guarantee that every new runtime workflow is compatible.

The CI publishes architecture-specific binaries only for explicit `tui-v<version>`
tags after all checks; GUI `v<version>` releases remain separate. No tag or release
is created by this implementation change.

Preview.36 verifies the background-work increment against the pinned actual dsh.
Child sessions now use their durable parent-qualified address for chat, history
and search, including direct `--session <child-id>` reconnect. `/parent` returns
to the parent. The child chat is a viewer: send follow-ups through the parent's
`/agents` → F3, and F8 interrupts through the parent API. Ordinary forks keep
ordinary addresses even when their history contains an inherited descriptor.
Shared Host jobs use ownerless output reads, display their scope and output gaps,
and require explicit stop confirmation. F5 reads a new output snapshot without
consuming the model's output cursor. Schedule history misses are errors, preserving
the previous snapshot.

`test/real_background.py` runs an actual shell job, a native continuable subagent,
and actual schedule delivery against an isolated dsh/local model. It covers output
refresh, cancelled/confirmed stops, child continuation, child history/search,
parent navigation, direct child CLI reconnect/F8, schedule run/history, and
reconnect without repeating work. A small test producer exercises shared jobs
and gap markers through the real registry; it does not simulate a shell process.
`--capture <prefix>` saves actual 100×30 and 80×24 terminal cells.
Team-task projections remain read-only; mutation needs a backend contract.
These panels remain manual F5 snapshots; registry jobs are not promised to survive
a dsh process restart.

Preview.35 adds shared workbench dialogs for roadmap stages 6–10. All lists are
point-in-time snapshots; F5 rereads them. Search filters names, ↑/↓ selects,
PgUp/PgDn scrolls details, and Esc returns. Mutation dialogs require a separate
Enter confirmation; uncertain writes never retry automatically. Schedule edits
use `expectedUpdatedAt`; preset selection is locked by dsh after the first turn.

| Command | Available operations |
| --- | --- |
| `/presets` | Read presets, select for an unused session, Ctrl+N create a session with a preset |
| `/workspaces` | Read workspaces, Enter create a session there, F3 create one in another folder |
| `/files` | Browse workspace directories and read text in 200-line pages, including version/EOF |
| `/compose` | Multiline message; Enter inserts a newline, F5 sends/queues, Esc hides and retains the draft |
| `/attach <local path>` | Upload a regular local file up to 2 MiB; up to four receipts per draft; Ctrl+X removes the last |
| `/terminals` | Ctrl+N create a shell, Enter attach a native interactive TTY, Ctrl+] detach, Ctrl+X close |
| `/plugins` | Inventory/status, F4 shared schema settings; custom web screens need adapters |
| `/commands` | Discover backend plugin commands, edit arguments, confirm execution and read its text result |
| `/packages` | Read bundle capabilities, Space enable/disable, F3 update to an exact version, Ctrl+X remove when allowed |
| `/agents` | Read subagents, Enter connect their session, F3 queue a message, Ctrl+X request interruption for continuable agents |
| `/tasks` | Read team task projections; no generic mutation API is assumed |
| `/jobs` | Read background jobs, Enter bounded output snapshot with gap indicators, Ctrl+X request stop |
| `/schedules` | Read host-wide schedules, Ctrl+N create, F3 edit JSON, Space pause/resume, Enter recent history, Ctrl+G run now, Ctrl+X delete |

The existing questions dialog displays the actual `exit_plan_mode` plan and its
Approve/Keep planning choices. No mode change is inferred from displayed text.
Native terminal attachment passes the server's VT stream to the user's terminal,
including full-screen programs; it is Linux-only, preserves the server process
when detached, restores TTY state, serializes acknowledged input, and never
replays input after a disconnect. File browsing is read-only text; PDF/image/web
rendering and file editing are not implemented. Uploads use opaque, session-bound
receipts and the existing send reconciliation; they are not sent by merely
opening or closing a dialog. Drafts live in memory and end with the session/client.

Package actions use dsh's permission/capability checks and display restart/rollback
outcomes. They do not automatically restart dsh or approve dependency builds.
The shared components cover existing schemas/commands; an upstream declarative
plugin protocol/SDK and custom account/OAuth/web adapters remain separate work.
`test/real_workbench.py` verifies the actual pinned dsh with isolated profiles,
a controlled local model and PTYs: inventories, file pages, schedule lifecycle,
interactive shell/detach/close, multiline draft/upload/send and both plan decisions.
It does not establish compatibility with every external plugin or cloud model.

The terminal client is implemented in Go/Bubble Tea, with schema-driven dsh settings
and a panel layout: a workspace sidebar, conversation panel, separate
message composer, connection status, and keyboard hints. The sidebar remains
visible throughout the supported size range. Input history restores drafts; scrolling, resizing, and clean shutdown
remain available. Mouse clicks select sidebar actions and sessions, and focus the composer.
The wheel scrolls the workspace tree or conversation under the pointer.
Mouse capture is enabled by default; `--no-mouse` disables it for native terminal
text selection. Many terminals also allow selection with Shift held down.
F10 (or `/select` / the command palette) temporarily releases mouse reporting
and freezes the current frame, including open dialogs, for native terminal text
selection. Drag to select and use the terminal copy shortcut (usually Ctrl+Shift+C).
Esc/F10 resumes the live display and restores the previous mouse setting. Incoming
backend messages continue to be processed; selection keys and paste do not answer
hidden requests or change drafts. F7 returns to a pending request; F8 stops the
agent. A terminal resize exits selection mode. Positioning the text cursor by
click is not implemented.
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
remains a clearly labeled local draft.

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
remains a separate backend section with schema-driven editing. **Настройки TUI** opens from
**Файл → Настройки TUI…**, F4, or Alt+T. F10 or Alt+F/V/H opens File/View/Help.
Dialogs preserve page, scroll, draft, and focus. Mouse clicks work on menus and
settings controls. Help → About works offline.

Mouse and input-history preferences apply immediately and are saved atomically
in `${XDG_CONFIG_HOME:-~/.config}/dsh-launcher-tui/preferences.json`. Disabling
history clears recall entries without erasing the current draft or chat records.
Chat messages are not stored in this preferences file. `--no-mouse` overrides the
saved preference for this run; use F4 to restore mouse reporting otherwise.
A failed write leaves the old setting active. Electron graphics/window controls
are excluded.

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

The static browser/WASM preview has been removed. Use the native Linux binary.

## Connect and edit dsh settings

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
Open **Настройки dsh** with the mouse or sidebar keyboard navigation. Left/Right
switches namespaces, `r` refreshes the snapshot, and the wheel or PgUp/PgDn scrolls
values. Enter opens the selected namespace's schema form; F6 or `/providers` opens
the providers dictionary. The attached dsh profile owns these settings: GUI and TUI
connected to the same profile share them. The client does not edit profile files.
An attached dsh is left running when the client exits.

Forms support nested objects, dictionaries, arrays, scalar values, and constant
choices from dsh's Schemastery schema. ↑/↓ selects a field, Enter opens/edits it,
and Ctrl+N adds a dictionary entry or array member. Inside a scalar editor, Enter
stages the value and Esc cancels; constant choices also accept ↑/↓. Strings use
plain text; numbers, booleans, and complex scalar values use JSON. Missing fields
are listed from the schema. Unknown fields remain read-only. Ctrl+E opens a
multiline editor: plain text for strings and JSON for other fields, including
objects/arrays. Enter inserts a newline; F5 stages the value in the form, then a
second F5 saves to dsh. Existing multiline strings open this editor with Enter.
Cancellation preserves the earlier staged value and the chat draft. Input that
would be transformed or truncated is refused. Whole-container editing is blocked
when the container contains redacted or schema-declared secrets.

Portable schema checks run before staging/saving: types, required values, constant
choices, nested objects/arrays/unions, numeric ranges/steps and lengths (string
length uses JavaScript UTF-16 units). Checks are bounded to depth 32 / 10,000 nodes.
Backend validation remains authoritative for patterns, transforms, refinements,
unsupported schema types and relationships between fields.

F5 sends only staged paths through `settings/mutate`, with `expectedRevision`.
Unrelated settings and redacted secrets are preserved. Array edits materialize the
whole array to preserve inherited members; arrays containing secrets are refused.
Ctrl+U unsets a user override and previews the inherited base value. Ctrl+X drops
the selected draft edit (the whole array when its members were edited). Esc moves
up a level; leaving the root with staged edits asks whether to discard them.
Drafts are local to the dialog, bounded to 256 operations / 64 KiB / depth 32.
The backend validates the complete namespace and reports its application policy.

A conflict or unconfirmed write blocks further saves until Ctrl+R rereads the
namespace. Ordinary draft fields survive the reread. Changes to the same path are
marked as conflicts: explicitly edit/review the field or drop its draft with
Ctrl+X before saving. No automatic write retries occur. Namespace revision checks
protect settings writes atomically; API-key writes use a separate API without a
revision guard. Reads/writes have a 20-second timeout. Browser demo is read-only.

On a credential-reference field such as `apiKeyEnv`, Ctrl+K opens a masked API-key
editor after pending settings have been saved. The form reads only
`credentials/describe` status (configured/source/writable), never the old key.
Enter explicitly writes the new value through `credentials/set`. Input is cleared
before sending and after failures; keys stay out of chat and input history.
A file credential can override the same environment reference for this dsh
profile. Successful storage does not prove provider access; the next model request
checks it. Limits: one line / 16 KiB. Credential deletion remains pending.
Generic provider OAuth/browser login needs a dsh Remote authorization contract:
the pinned backend has internal authorization flows but no generic API for
listing/starting/cancelling them and exchanging prompts/notices. The separate
DeepSeek account API is not a substitute for other providers' OAuth; its account
screens remain a dedicated adapter task. The TUI does not run its own OAuth flow
or modify the shared credential-record format. Schema-declared secrets are masked in the settings view and form;
private backend errors are replaced with generic diagnostics.

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

Preview.31 adds **F12 / `/models`** for the connected session. F12 also works
from the dsh settings page. The picker reads `session/modelCatalog` and the
session’s `modelSelection.next` projection, falling back to the host default for
an unconfigured session. Search matches provider/model IDs and names; ↑/↓ or a
click selects a row, Tab/Shift+Tab cycles adapter-declared reasoning efforts
(including default), Enter applies, F5 rereads, and Esc closes while idle/loading.
Unsupported current selections remain visible separately and are never invented
as catalog entries. The draft and underlying scroll remain intact.

Applying uses `session/selectModel`, without editing provider credentials. The
picker states that dsh also saves the **shared default** in the background; the
response confirms only the session choice. Current generation continues and the
choice applies at the next prompt-assembly boundary. A preflight reread refuses
an already changed session choice; this is not an atomic compare-and-swap because
the API has no revision argument. A competing write can still race that boundary.
Unconfirmed/rejected writes never retry; Enter stays blocked until F5 rereads the
state. A read-only/no-change choice issues no mutation. Cancelled/late results and
session changes cannot update a new picker. Provider failure messages and keys
are not displayed. Catalog bounds are 256 groups / 2,000 models / 32 efforts per
model; read/write operations have a 20-second timeout. Browser demo does not
perform model changes. Preview.32 adds API-key authorization and
schema/revision-based settings forms. Preview.33 adds multiline/JSON editing and
portable schema validation. Generic OAuth needs a backend Remote contract;
specialized account screens remain separate work.

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
python apps/tui-go/test/real_settings.py --binary /path/to/dsh-launcher-tui-go --dsh /path/to/dsh
python apps/tui-go/test/real_goals.py --binary /path/to/dsh-launcher-tui-go --dsh /path/to/dsh
```

## Sessions and agent modes

Preview.34 adds **`/goal`**: a snapshot of the current goal, its phase/revision,
round count/cap, block reason, and process-local automatic-continuation state.
Active phase and armed continuation are shown separately. F5/Ctrl+R rereads;
↑/↓, PgUp/PgDn, and the wheel scroll the description. Empty goals and unavailable
reads are distinct; a failed read retains the last snapshot marked unconfirmed.

F3 opens the objective/round-cap editor. Tab switches fields, Enter inserts a
newline in the description, F5 saves through `goals/edit`, and Esc discards the
draft. F3 switches between the snapshot and the retained editor. Only changed
fields are sent, with the exact goal ID/revision. The backend atomically rejects
a stale ref. Edits preserve phase/activation; an already armed goal may continue
under its revised objective. Opening or viewing the dialog never submits a task.

A rejected/unconfirmed write retains the draft and blocks saves until Ctrl+R.
If the revision changed, the fresh snapshot is shown for review. Ctrl+B explicitly
accepts that revision and returns to the retained draft; untouched fields follow
the new snapshot. A draft cannot be rebased onto a different goal identity.
Esc cancels the draft; a second Esc closes the dialog. No write retries occur
automatically. Changing sessions cancels the dialog and ignores late responses.
Archived sessions cannot edit. Browser demo does not access goals.

Limits: objective 64 KiB, positive integral round cap up to JavaScript's safe
integer maximum (9007199254740991), 20-second read/write timeout. Input that would
be truncated or transformed is refused; pasted tabs/control characters are not
accepted. Creating/pausing/resuming/clearing goals still uses the existing
`/goal <description>`, `/goal pause`, `/goal resume`, and `/goal clear` commands.
Plan review and preset selection remain the next stage-6 increments.

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
If dsh is missing, `--start` offers installation into the TUI data directory.
Alternatively, open `/runtime` without `--start` to install with live progress.

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

## Migration scope

The agreed target is terminal-only operation: launcher capabilities, the dsh
interface, and plugin functionality through dedicated TUI views and adapters.
Arbitrary HTML/JavaScript plugin screens cannot run inside Bubble Tea. Plugin
support combines background operation, shared schema/command views, a versioned
declarative UI contract, and dedicated adapters for complex workflows. The initial shared contract is implemented in [the plugin SDK](../../packages/tui-plugin-sdk/README.en.md). Track backend, settings, commands, and complete
screen support separately; a generic chat client does not establish full plugin
compatibility.

Work proceeds in small, reviewable checkpoints with user feedback. The
[roadmap](../../docs/tui-roadmap/README.en.md) is the maintained capability inventory
and milestone order. The client includes streamed chat, tool output, questions,
permissions, queues, history/search/forks/archive, models/settings, goals/presets,
workbench adapters and owned runtime installation/update/rollback/diagnostics.
Actual-model acceptance was checked with Qwen3.8-27B on November 16, 2026.
Workspace writes, permanent session deletion and team mutations require backend contracts. Additional provider/plugin adapters remain API-dependent. Public TUI
update delivery still needs its first dedicated release. Full GUI/plugin parity
is not claimed.

F4 → Keyboard shortcuts (also F1) opens the searchable, scrollable key reference. It includes the context of modal/editor shortcuts. Type to filter, Ctrl+U to clear, PgUp/PgDn or the mouse wheel to scroll. Closing restores the composer draft and focus.


## Preview.38

F4 settings persist dark/light theme, Russian/English and sidebar visibility.
F1 opens the keyboard reference; GUI settings have their own searchable reference.
Ctrl+F or `/` on Sessions filters by title/ID. Clicking a session selects it.

`/account` shows DeepSeek account state, browser sign-in/cancel, profile/balance
and guarded sign-out. Open the displayed HTTPS login URL in a browser; over SSH,
forward the dsh port for its callback. F5 refreshes the account snapshot. Account
credentials stay in dsh. Production sign-in requires an interactive browser login.

`/files` reads PNG/JPEG/GIF as bounded terminal thumbnails, PDF as its text layer
(requires `poppler-utils` / `pdftotext`), and local HTML as passive text. Full-file
previews are capped at 6 MiB, images at 20 million pixels, PDF output at 256 KiB/10s.
Text files remain paginated. File writing needs a backend mutation/CAS contract.

`/packages`: Ctrl+N installs a package spec/path; F6 lists compatible versions;
F3 accepts an exact version. If an install reports pending build scripts, F2
names them for explicit approval before retry. `/runtime` restarts only an owned
dsh, after confirmation. `/operations` lists private local receipts for the same
backend origin; an interrupted request has an unknown outcome until checked.
Receipts do not establish success merely because the client restarted.

The [declarative SDK](../../packages/tui-plugin-sdk/README.en.md) supports shared
forms/actions/status/tables over existing human commands. Arbitrary plugin HTML
still requires a terminal adapter. CI covers two SDK examples, actual package
install/update/failure preservation and fresh installs on three distributions.

### Marketplace and automation pages

Plugin Market uses a centered catalog with description cards, installed/catalog tabs,
package versions, and explicit confirmations for installation, update, and removal.
The adapter continues to use authenticated dshmarket API v1.

Automation tasks uses the Host-wide `schedule/catalog` and status filters with counts.
F2 cycles All/Enabled/Paused/Ended; typing searches names, instructions, and IDs.
Ctrl+N opens a form for the title, instruction, and a one-shot delay or repeating
interval in minutes. F3 edits title and instruction while keeping the timing rule.
Tab/Shift+Tab selects a field; F5 reviews the request; Enter confirms it. F2 inside
a form opens the advanced JSON editor for other native timing/target options.
Space pauses/resumes, Ctrl+G runs now, Ctrl+X deletes; writes retain confirmation
and revision checks. Enter opens the timing rule and the latest 100 saved delivery
records, including retention/pruning notices. Times show the terminal's local zone.
F5 refreshes; Esc returns to the list and then to the previous page.
