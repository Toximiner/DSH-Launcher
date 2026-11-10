# TUI development roadmap

[Русский](README.md) | **English**

Baseline: November 10, 2026, Go client `0.2.0-preview.7`.
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

- Sidebar navigation, panels, local draft input/history, mouse, and scrolling.
- dsh attachment and read-only settings with secret masking.
- Session creation/list/attachment, backend-confirmed modes, and goal commands.
- Starting an installed dsh with `--start`, and offering dshmarket preinstallation.
- Marketplace catalog/search/installed packages and confirmed install/update/remove
  actions, with progress read from the API.
- Go/PTY/CI checks and a live Codespaces configuration with actual dsh.

Ordinary chat messages remain local; agent chat is not implemented.
No cloud LLM is connected to the live deployment. Registration attempts produced
no API key; actual model responses and agent tool execution remain unverified.

## Capability inventory

| Area | Current state | Remaining work | Milestone |
|---|---|---|---|
| Application menu | TUI settings in sidebar | Top menu and separate client settings dialog | 1 |
| Chat | Local drafts | Submission, streaming, cancellation, reconnection | 2 |
| Tools and questions | No dedicated views | Execution/results, permissions, user answers | 3 |
| Sessions and history | Create/list/attach | History, search, rename, fork, supported archive operations | 4 |
| Message rendering | Plain local records | Markdown, code, tables, folding, copying | 4 |
| Models and accounts | No configuration UI | Providers, authorization, model and parameter selection | 5 |
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

1. **Top menu and TUI settings.** File, View, Help menus; move TUI settings out
   of the sidebar into a dialog reached from the top. Keep dsh settings separate.
   Initial editable preferences: mouse and input history, persisted between runs;
   include client information and help. Done: keyboard/mouse and narrow terminals
   work; opening/closing preserves the page, draft, and focus.
2. **Minimal actual chat.** Session submission, streamed assistant text,
   cancellation, errors, reconnection without resubmitting accepted messages.
   Done: an actual model answers through actual dsh; exit restores the terminal.
3. **Tools and interaction.** Tool progress/results, permission decisions,
   structured and free-text questions, basic queue/steering controls.
   Done: the agent completes a tool task; rejection/cancellation reaches dsh,
   and a question answer is not submitted as an ordinary chat message.
4. **History and sessions.** Transcript loading, search, rename, fork, supported
   archive operations, Markdown, and collapsible details.
   Done: reattachment restores actual history without duplicates.
5. **Models and dsh settings.** Provider authorization, model selection,
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
    clean binary installation, supported Linux verification.
    Done: everyday workflows and known limitations have been reviewed.

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
  minimum 44×18, with draft/focus preservation checks.
- Actual dsh/plugin checks for connected adapters. Fixtures and simulated LLMs
  are not evidence of actual agent operation.
- Verify the shipped Linux binary; add TUI installation Docker scenarios when
  installation exists. Existing GUI Docker scenarios run only in CI, following
  CONTRIBUTING.md.
- Small commits on `feature/terminal-ui`, CI checks, and a reviewable result;
  merging and releasing remain separate actions.
