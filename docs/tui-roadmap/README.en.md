# TUI development roadmap

The OpenCode-style shell and conversation have separate intermediate builds. Preview `opencode.8` migrates settings: compact grouped search with all 15 actions, arrow changes to local values, and flat shortcut help, dsh forms and small launcher dialogs. Stage 4 remains in progress: plugins/market, automation, files, terminals, subagents, queue/history, runtime, install/update/rollback and diagnostics still await migration. The preview does not replace the installed TUI. See the [rebuild plan](../tui-rebuild-plan.en.md).

Plugin Market and Automation tasks now follow the GUI structure: dedicated pages, cards and status indicators; tasks have status filters and creation/editing forms. Delivery history is readable, with explicit time zones. F2 in a form opens advanced JSON for complex rules. Revision checks and confirmation remain in place.

Preview.40 Plugins is a GUI-shaped page with built-in settings, experimental and installed packages, dsh metadata, More, Market and Add. Components remain a separate read-only view. Confirmed toggles recheck the observed baseline and reread their result; existing operation/editor safeguards remain in use. Actual-dsh tests cover cancellation, enablement and fixture restoration without model calls.

The preview.40 keyboard reference has seven topic groups, key/action columns, muted contexts, section filtering, search and a sticky section heading. Checks cover complete inventory, the last scrollable line, RU/EN, mouse input and 80×24 or larger terminals.

The preview.40 settings modal follows the GUI: five sections on the left, described parameters on the right, configuration and close controls above. It supports two focus panes, scrolling, mouse input and return from nested panels. Default permissions for future sessions use the revision-checked dsh editor; language, theme and input settings remain local to TUI. Actual-dsh checks verify inheritance by a new session without changing the current one.

Active conversations in preview.40 use a centered column up to 100 cells, user messages on the right, assistant replies on the left, a preset header and a Trajectory link to history. The composer, editor, permissions and model stay at the bottom. Times and statistics come from dsh; Copy and Ctrl+Y preserve the source text. Checks cover 80×24 and larger sizes, mouse targets and actual dsh with a fixture model.

Preview.40: the sidebar follows the GUI grouping: primary actions above a workspace/session tree, with settings and account in the fixed lower More menu. Snapshots come from dsh without model calls; the tree scrolls independently. New sessions honor the selected workspace.

Empty sessions now use a landing screen with workspace/preset context, actual model selection and permissions. Controls open actual panels; permission changes require confirmation and state reconciliation.

Preview.39: `--start` and `/runtime` startup reuse a running local dsh after API authentication. Unavailable login discovery gives guidance instead of spawning another process. Failed startup preserves diagnostics and the existing backend ownership record.

GUI/TUI now share local dsh client leases: closing either client preserves the backend for the other, and the last client stops a managed backend; external dsh remains external. Actual-dsh checks cover both close orders, with additional GUI Docker scenarios on CI. Ctrl+X cancels the current package request by requestId; account profile/balance opens in a scrollable view.

[Русский](README.md) | **English**

Baseline: November 16, 2026, Go client `0.2.0-preview.40`.
Terminal minimum: 80×24; recommended: 100×30.
This document tracks differences from the Electron launcher and its dsh interface,
and the order of implementation. Planned items are not shipped capabilities.


## Preview.38: remaining client work

- TUI/GUI settings now include a searchable keyboard reference with contexts.
- TUI settings persist dark/light theme, RU/EN language and sidebar visibility.
  Session search filters titles/IDs locally; clicking selects a session.
  Background session refresh no longer shifts the list or flashes a loading label.
- `/account` uses actual DeepSeek account APIs: safe state, browser sign-in,
  cancellation, profile/balance and sign-out after checking running account work.
  Live identity-provider acceptance still needs an interactive browser login.
- `/files` previews PNG/JPEG/GIF thumbnails, PDF text through `pdftotext`
  (poppler-utils), and local HTML text without scripts. Full-file reads use the
  typed binary carrier and size limits. Scanned PDFs require OCR elsewhere.
- [Plugin SDK](../../packages/tui-plugin-sdk/README.en.md): versioned declarative
  fields/actions/status/tables over existing commands. Two different examples
  run through the same renderer against actual dsh.
- `/packages`: direct installation, compatible version selection, explicit
  approval of named blocked build scripts, backend rollback outcome.
  `/operations` stores private receipts across client restarts. Unknown outcomes
  require inventory checks and are never replayed automatically.
- CI adds an actual-dsh/pnpm local-registry lifecycle suite (including failed
  update preservation) and clean installs on Ubuntu 24.04/26.04 and Debian 13.

Remaining external contracts: workspace file writes with revision checks,
permanent session deletion with descendants/tasks/workspace semantics, and team
mutation APIs. The pinned workspaceFiles service explicitly exposes no writes.
Do not replace these contracts with direct edits of backend files. Other providers'
OAuth and arbitrary web-only plugin workflows need their own supported adapters.
First public TUI delivery is the final release check after green CI.

## Preview.37: stages 11–13

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

## Preview.36: stage 10 client increment verified

Actual-dsh scenarios cover:

- shell output before completion, rereads, cancelled/confirmed stop and reconnect
  without rerunning the process;
- shared ownerless Host output, gap indicators and stop through the real registry
  with a dedicated test producer;
- a native continuable child: queued follow-up, child navigation/history/search,
  reconnect while running, interruption and `/parent` navigation;
- direct `--session <child-id>` reconnect and F8 through the parent API;
- explicit schedule run, cancelled confirmation, saved delivery history and
  reconnect without duplicate delivery.

Child chat/history now use the durable parent/child/mode address. Ordinary forks
retain ordinary addresses even with an inherited descriptor. Child chat is a
viewer; follow-ups use the parent's `/agents`. New panels remain manual F5
snapshots. The stage 10 client increment is complete within the available API.
Team-task writes remain an external backend-contract dependency; a generic plugin
workflow UI belongs to stages 8–9. Registry jobs are not promised to survive a dsh
process restart.

## Preview.35 progress across stages 6–10

- **6:** `/presets` lists/selects presets before the first turn and creates a
  session with one. Actual plan review verifies Keep planning and Approve.
  Goal editing/revision protection remains available from preview.34.
- **7:** `/workspaces`, `/files`, `/compose`, `/attach`, `/terminals` supply folder
  selection for new sessions, paged text reads, multiline drafts, four uploads up
  to 2 MiB through receipts, and a native interactive TTY with Ctrl+] detach.
  File writes and PDF/image/web document rendering remain unimplemented.
- **8:** `/plugins` and `/commands` provide inventories, existing schema forms,
  command discovery/confirmed execution and text results. A declarative protocol
  and SDK are not implemented; arbitrary web screens still require a server
  contract or an adapter.
- **9:** `/packages` complements `/market`: capabilities, enable/disable, allowed
  removal/exact-version updates, application/restart/rollback outcomes. No automatic
  host restart or dependency-build approval. Actual external-package updates,
  restart recovery and dedicated OAuth/account adapters remain to be verified.
- **10:** `/agents`, `/tasks`, `/jobs`, `/schedules` provide subagent/team-task
  snapshots, child-session navigation, queued messages/interrupt for continuable
  agents, bounded job output/stop, schedule create/edit/pause/delete with CAS,
  history and explicit run-now. Team-task mutation needs a backend API.

Actual isolated dsh checks cover inventories, file pages, schedule lifecycle,
shell attach/detach/close, upload/multiline send and both plan decisions. Full chat
regressions and Go race tests pass. New panels are F5 snapshots, not live feeds.
In preview.35 advanced agent operations, job output, schedule history/run and
package updates still needed dedicated actual-backend scenarios. Preview.36 adds
background-work verification; actual package updates remain outstanding. Stages 7–10 had a working
baseline in preview.35; stage 10 verification is extended in preview.36 above.
Stages 7–9 remain partial.

## Goal and workflow

A terminal-only Linux client using Go + Bubble Tea, with functional equivalents
of launcher, dsh, and plugin features. Capability matters more than copying web
layouts. dsh keeps running JavaScript plugins; Go supplies terminal presentation.

Work proceeds in small increments: one complete capability, verification, user
review, then the next increment. Each milestone below is split accordingly.
Show actual application screenshots for UI changes. The public Pages demo is
disabled; private Codespaces supports live inspection.

## Already implemented

- File/View/Help menu, modal TUI settings, persisted mouse/input-history preferences.
- Sidebar navigation, panels, local draft input/history, mouse, and scrolling.
- dsh attachment, schema-driven revision-checked settings, provider creation, and masked API-key entry.
- dsh task queue: submission while running, server-confirmed pending items, individual removal/editing, queue-to-steering promotion, admission into history, and reconnect recovery.
- Agent questions: choices, multi-select, multiline answers, skip, batches, timed holds, and continued replies.
- Permission dialogs: reason, arguments, one-shot allow/reject, F7, cancellation, and pending request recovery.
- Tool cards: live command stdout/stderr, state, arguments, result/error, mouse or F6 → Enter expansion, history restoration, and explicit nested PTC relationships.
- Session text chat: submission, streaming, cancellation, read reconnection without automatic resubmission.
- Read-only paginated session history: F11, a fixed cut, older/newer pages, saved scroll positions and drafts; Ctrl+F/F3 message-text search across older pages.
- Session creation/list/attachment, backend-confirmed modes, and goal commands.
- Starting an installed dsh with `--start`, and offering dshmarket preinstallation.
- Marketplace catalog/search/installed packages and confirmed install/update/remove
  actions, with progress read from the API.
- Go/PTY/CI checks and a live Codespaces configuration with actual dsh.

Offline input stays local. Chat was verified against actual dsh with a controlled
local model endpoint; this establishes protocol integration, not a real LLM reply.
On November 16, the native TUI was also checked against the GUI-configured
Qwen3.8-27B: a real model reply and command output before completion were observed.
The check used an isolated temporary profile; credentials are not bundled.

## Capability inventory

| Area | Current state | Remaining work | Milestone |
|---|---|---|---|
| Application menu | Top menu and modal client settings | Basic milestone complete; further polish at milestone 14 | 1 ✓ |
| Chat | Submission, streaming, cancellation, reconnection implemented | Verified with actual Qwen3.8-27B | 2 ✓ |
| Tools and questions | Tool cards, permissions, questions, queue submission/view/removal/editing/steering, nested PTC calls, command stdout/stderr | Basic milestone complete; arbitrary plugin streams need backend APIs | 3 ✓ |
| Sessions and history | Create/list/attach, rename, fork, archive; history, message/tool search, cross-page replacements | Current increment complete; permanent deletion needs dsh API support | 4 ✓ |
| Message rendering | Markdown, syntax coloring, tables, folding, copying | Current increment complete | 4 ✓ |
| Models and accounts | Catalog, session model/reasoning selection, provider creation, API keys | OAuth, specialized editors, remaining account flows | 5, partial |
| dsh settings | Forms, multiline/JSON editor, local/backend validation, persistence, revision conflicts | Basic client increment complete; specialized editors are future improvements | 5 ✓ |
| Agent modes | Editable goals, plan review, presets | Client increment implemented | 6 ✓ |
| Workspace | Workspace selection, directory tree, paged text reads | File writes need API; PDF/HTML/thumbnails implemented | 7, partial |
| Input and attachments | Multiline editor and receipt-based uploads | Basic input/attachments implemented | 7 ✓ |
| Additional panes | Native interactive TTY | Basic previews implemented; arbitrary web workflows need adapters | 7, partial |
| Plugins | Inventory, shared schemas/commands, market/packages | Basic contract/SDK implemented; dedicated adapters depend on API | 8–9, partial |
| Marketplace | Basic operations | Client implemented; additional adapters depend on API | 9 |
| Subagents and jobs | Subagents, read-only team tasks, jobs, schedules; actual flows/reconnect | Team-task writes need API; plugin workflows belong to stages 8–9 | 10, client ✓ |
| dsh installation/process | Discovery, install progress, logs, owned processes/retry | Baseline client work complete | 11 ✓ |
| Updates and rollback | dsh/TUI check, install, backup/recovery | Public TUI delivery needs its first dedicated release | 12, client ✓ |
| Diagnostics/notifications | Owned dsh log, masked report, notices/BEL | Baseline client work complete | 13 ✓ |
| Appearance/interaction | Themes, RU/EN, search, sidebar, shortcut reference, mouse | Release delivery checks | 14 |

## Milestones and acceptance criteria

1. **Top menu and TUI settings — complete in preview.8.** File, View, Help menus; move TUI settings out
   of the sidebar into a dialog reached from the top. Keep dsh settings separate.
   Initial editable preferences: mouse and input history, persisted between runs;
   include client information and help. Done: keyboard/mouse and narrow terminals
   work; opening/closing preserves the page, draft, and focus.
2. **Minimal actual chat — implemented in preview.9; verified with actual Qwen3.8-27B on November 16, 2026.** Session submission, streamed assistant text,
   cancellation, errors, reconnection without resubmitting accepted messages.
   Done: an actual model answers through actual dsh; exit restores the terminal.
3. **Tools and interaction — first increment implemented in preview.10.** Tool cards, results/errors, cancellation state, folding, and history restoration; verified with actual dsh file reads. Permissions implemented in preview.11: explicit one-shot allow/reject, cancellation, reconnection, and decision history. Questions implemented in preview.12: choices, free text, batches, timed holds, and continued replies. Queue submission/view implemented in preview.13, verified with three tasks and reconnection. Individual queue removal implemented in preview.14; multiline editing in preview.15; queue-to-steering promotion in preview.16; nested PTC cards in preview.17; live command stdout/stderr in preview.18.
   Durable job registrations bind streams to calls; observation does not consume
   model output and reconnect resumes byte offsets. Bounds: 16 streams, a 128 KiB
   tail per stream, with explicit loss markers. Tools without job APIs do not get
   synthesized streams. Next milestone: history and sessions.
   Done: the agent completes a tool task; rejection/cancellation reaches dsh,
   and a question answer is not submitted as an ordinary chat message.
4. **History and sessions — first increment in preview.19.** F11 opens paginated
   history with a fixed cut, older/newer pages, saved scroll positions, and F5
   for newer messages; live chat and its draft remain separate. Actual dsh checks
   cover 27 turns, an active answer, reconnect, and narrow terminals. Page limits:
   100 displayed records, 4 MiB, 4096 events. Preview.30 supports cross-page
   replacements through a fixed-snapshot index. Preview.20 adds case-insensitive literal message-text
   search via Ctrl+F, jumps to matches, and F3 for an older result. Scans share the
   fixed cut in batches of up to ten pages; Esc cancels and F5 refreshes the cut.
   Preview.30 adds tool search. Preview.21 adds `/rename` for the current
   session and F3 for a selected list entry; confirmed titles appear in the list
   and chat header. Preview.22 adds `/fork` and Ctrl+B in the list, branching at
   the latest completed turn. The source keeps running; the child inherits history
   and opens with the preserved draft. Independent continuation is verified.
   Preview.23 selects history messages with Tab/Shift+Tab or a click; Ctrl+B forks
   the exact inclusive event prefix. Search hits are selected automatically.
   User-message and assistant-answer boundaries are verified.
   Preview.24 adds archive/restore through `/archive`, `/restore`, and Ctrl+X; a
   switches active/archive lists. The shared GUI registry reconnects; running
   work is never stopped implicitly.
   Preview.26 adds Markdown for committed replies and history: code, lists, quotes,
   links, and tables; Ctrl+R toggles source. Wide tables retain cells as labeled
   records. Search and branching preserve authoritative source event identities.
   Preview.27 adds Ctrl+Y for message source and Ctrl+K for a code-block picker:
   Linux clipboard helpers or OSC 52 with an explicit receipt status.
   Preview.28 adds fence-language syntax coloring with bounded work and plain
   fallback, preserving source and clipboard contents.
   Preview.29 adds collapsible history tool cards: F6/Shift+F6 select and
   Enter/click toggles details, with bounded preferences across snapshot pages
   independent of live chat.
   Preview.30 adds tool-name/argument/result search and all / messages / tools
   scopes via Ctrl+T. Tool matches expand without selecting a message fork boundary.
   A snapshot index folds cross-page and repeated replacements in surface order,
   keeping replaced text and nested cards out of older pages and search results.
   Index bounds: 100,000 events / 64 MiB read. The current stage-4 increment is
   complete; permanent deletion remains separate and needs dsh API support.
   Next: stage 5 — models and editable dsh settings.
   Done: reattachment restores actual history without duplicates.
5. **Models and dsh settings — first increment in preview.31.** F12 / `/models`:
   connected-provider catalog, search, model and reasoning-effort selection.
   Acknowledgement covers the session; dsh saves the shared default in the
   background. Preflight detects already changed selections without claiming
   atomic API protection; unconfirmed writes never retry. Preview.32 adds schema
   forms, provider/model creation and editing, F5 revision-checked persistence,
   and same-field conflict review. Ctrl+K enters API keys separately; old values
   are never read, and credential writes have no revision guard. Actual dsh checks
   verify the new key in a request and two-client conflicts in an isolated profile.
   Preview.33 completes the basic forms: Ctrl+E opens multiline/JSON editing;
   local checks cover types, required values, ranges, steps, and lengths. The first
   F5 stages; the next saves. The model/API-key/generic-settings client increment
   is complete. **External dependency:** generic provider OAuth needs dsh Remote
   operations to list/start/cancel flows and exchange prompts/notices. The pinned
   version has the internal authorization service but no generic RPC contract.
   DeepSeek's separate account API does not cover other providers' OAuth; its
   account screens remain a dedicated adapter task. The full stage stays partial
   until those sign-in methods are implemented and verified. Secrets stay out of
   transcripts/logs.
6. **Plans, goals, presets — preview.34–35.** `/goal` opens a
   card with objective, phase/revision, rounds/cap, block reason and separate
   continuation activation. F3 edits the multiline objective/cap; F5 saves through
   goals/edit with atomic identity/revision checks. Conflicts retain the draft;
   Ctrl+R shows the fresh snapshot, and Ctrl+B explicitly accepts its revision,
   preserving untouched foreign fields. Actual dsh verifies cancellation, absent
   goals, three terminal sizes and reconnect. Preview.35 adds plan inspection/review through questions,
   preset selection and separate session creation with a preset; both plan choices
   are verified against actual dsh.
   Done: reattachment restores state matching dsh.
7. **Workspace and input.** Add multiline input, files/attachments, file tree,
   document previews, and terminal pane in separate increments. Specify terminal
   browser workflows and limitations separately; arbitrary web screens are not
   considered automatically ported. Done: the agent receives a selected file,
   results are visible in the correct workspace, and pane focus/shortcuts coexist.
8. **Generic plugin support.** Capability inventory, forms from available schemas,
   commands, shared result views. Then a minimal declarative contract/SDK: start
   with a form and action, add tables/status later. Done: two different plugins
   reuse the same components without dedicated Go screens; unsupported features
   are explicitly identified.
9. **Marketplace and dedicated adapters.** Complete package management; add custom
   views only for workflows the generic contract cannot represent. Done: rejection,
   conflicts, and restart recovery are tested; each claimed adapter has a scenario
   against the actual plugin API.
10. **Subagents, jobs, schedules — client increment preview.35–36.**
    Views, available controls, output, delivery history and navigation are verified
    against actual dsh, including reconnect without duplicate execution.
    Team tasks are read from projections; mutation needs a backend API.
    New views refresh with F5.
11. **dsh installation and lifecycle — preview.37.** Node/dsh discovery, offered installation
    with live progress, errors/retry, owned process management.
    Done: a fresh profile installs and starts; exiting never stops an attached
    backend owned by someone else.
12. **Updates and rollback — preview.37 client baseline.** Separate increments for dsh and the Go client:
    version checks, release notes, installation, restoring a previous version.
    Done: failed updates have a verified recovery path.
13. **Diagnostics and notifications — preview.37.** dsh log, redacted report export, execution
    state, applicable terminal notifications. Done: a report supports reproducing
    a failure without disclosing keys.
14. **Polish and release.** Themes/language, search, panes, mouse, shortcut help,
    plus an active keyboard-shortcut list in both TUI and GUI settings, with
    each action and its scope. Reflect client differences and update the lists
    when bindings change. Include clean binary installation and supported Linux
    verification.
    Done: everyday workflows and known limitations have been reviewed.

## Shared TUI and GUI tasks

- [x] Add a keyboard-shortcut list to both clients’ settings: binding, action,
  and scope. Reflect TUI/GUI differences and update the list with binding changes.
  Requested November 16, 2026.

## Which Electron settings apply

Applicable: input history, mouse, language/theme, working directory, owned dsh
path/profile/port, updates, logs, and notifications once their capabilities exist.
Launch parameters, updates and diagnostics were added in preview.37 (11–13). Terminal font
and zoom normally belong to the terminal; browser zoom belongs to the live-server
wrapper. Do not present unimplemented preferences as functional controls.

Do not port Wayland/X11, Ozone, Electron GPU, Chromium sandbox, window geometry,
close-to-tray, Electron fullscreen, or Chromium spellcheck preferences.

## Plugins: four support levels

1. **No dedicated screen.** Background features and agent tools run in dsh.
   Shared chat/tool views expose their results after milestones 2–3.
2. **Generic UI from existing data.** Build settings from dsh schemas and commands
   from available server descriptors. This covers only capabilities actually
   exposed by the API. Web actions and custom dialogs do not automatically become
   server commands.
3. **Declarative plugin UI.** A proposed, unimplemented contract: plugins describe
   forms, tables, actions, and statuses; TUI renders them. Requires versioning,
   capability discovery, and compatibility checks. JavaScript plugins can provide
   these descriptions without rewriting their backend in Go.
4. **Dedicated adapter.** Complex workflows need an explicit terminal view over
   an API, like the current marketplace, reusing shared components. If there is
   no suitable API, the plugin backend needs extending first.

Combine all four levels. Track backend operation, settings, commands, and full
screen support separately for each plugin. One screen or an installed package
does not establish complete plugin compatibility. Bubble Tea cannot automatically
execute arbitrary plugin HTML/React screens.

## Verification per increment

- Protocol/error checks against a controlled HTTP server; PTY checks for keyboard,
  mouse, resize, and terminal restoration.
- Actual wide/narrow terminal screenshots for UI changes, including 80×24 and
  minimum 80×24, with draft/focus preservation checks.
- Actual dsh/plugin checks for connected adapters. Fixtures and simulated LLMs
  are not evidence of actual agent operation.
- Verify the shipped Linux binary; add TUI installation Docker scenarios when
  installation exists. Existing GUI Docker scenarios run only in CI, following
  CONTRIBUTING.md.
- Small commits on `feature/terminal-ui`, CI checks, and a reviewable result;
  merging and releasing remain separate actions.

## Session deletion

- [ ] Add permanent session deletion in TUI with a dedicated confirmation. The pinned dsh API exposes no deletion operation; a backend contract must cover persisted history, workspace links, descendants, and active work. Archive retains history and supports restoration.
