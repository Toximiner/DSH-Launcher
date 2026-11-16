# TUI development roadmap

[Русский](README.md) | **English**

Baseline: November 16, 2026, Go client `0.2.0-preview.31`.
Terminal minimum: 80×24; recommended: 100×30.
This document tracks differences from the Electron launcher and its dsh interface,
and the order of implementation. Planned items are not shipped capabilities.

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
- dsh attachment and read-only settings with secret masking.
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
| Models and accounts | Provider catalog and session model/reasoning selection | Provider authorization/editing and remaining parameters | 5, partial |
| dsh settings | Read-only snapshots | Forms, validation, persistence, revision conflicts | 5 |
| Agent modes | Ordinary/plan/goal and basic commands | Cards, plan review, goal editing, presets | 6 |
| Workspace | Current directory path | File tree, previews, workspace selection | 7 |
| Input and attachments | Single-line input | Multiline editor, file references, attachments | 7 |
| Additional panes | None | Terminal/documents; investigate terminal equivalents of browser workflows | 7 |
| Plugins | Dedicated marketplace view | Generic settings/commands, declarative contract, dedicated adapters | 8–9 |
| Marketplace | Basic operations | Compatibility, build approvals, restart, rollback, pending operation recovery | 9 |
| Subagents and jobs | No views | Progress, agent teams, jobs, schedules, workflows | 10 |
| dsh installation/process | Starts installed dsh | Installation progress, discovery, restart, recovery | 11 |
| Updates and rollback | No UI | Client/dsh updates, release notes, rollback | 12 |
| Diagnostics/notifications | Local TUI log | dsh log, reports, background status, terminal notifications | 13 |
| Appearance/interaction | Fixed theme/language, basic mouse | Themes, language, search, panes, shortcuts, improved mouse | 14 |

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
   atomic API protection; unconfirmed writes never retry. Next: provider
   authorization/editing, remaining parameters,
   schema-driven forms, revision-checked writes; secrets stay out of transcripts
   and logs. Clearly mark changes requiring restart. Done: a selected model applies
   to the session; conflicts preserve other writers' edits. Actual verification
   requires working provider access.
6. **Plans, goals, presets.** Extend current controls with plan inspection/review,
   goal editing, execution status, and preset selection.
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
10. **Subagents, jobs, schedules.** Port monitoring, available control actions,
    results, and links to related sessions one screen at a time.
    Done: states come from dsh and recover on reconnection.
11. **dsh installation and lifecycle.** Node/dsh discovery, offered installation
    with live progress, errors/retry, owned process management.
    Done: a fresh profile installs and starts; exiting never stops an attached
    backend owned by someone else.
12. **Updates and rollback.** Separate increments for dsh and the Go client:
    version checks, release notes, installation, restoring a previous version.
    Done: failed updates have a verified recovery path.
13. **Diagnostics and notifications.** dsh log, redacted report export, execution
    state, applicable terminal notifications. Done: a report supports reproducing
    a failure without disclosing keys.
14. **Polish and release.** Themes/language, search, panes, mouse, shortcut help,
    plus an active keyboard-shortcut list in both TUI and GUI settings, with
    each action and its scope. Reflect client differences and update the lists
    when bindings change. Include clean binary installation and supported Linux
    verification.
    Done: everyday workflows and known limitations have been reviewed.

## Shared TUI and GUI tasks

- [ ] Add a keyboard-shortcut list to both clients’ settings: binding, action,
  and scope. Reflect TUI/GUI differences and update the list with binding changes.
  Requested November 16, 2026.

## Which Electron settings apply

Applicable: input history, mouse, language/theme, working directory, owned dsh
path/profile/port, updates, logs, and notifications once their capabilities exist.
Add launch parameters at milestone 11 and updates at milestone 12. Terminal font
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
