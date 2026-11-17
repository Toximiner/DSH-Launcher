# OpenCode-style dsh TUI

[Русский](tui-rebuild-plan.md) | **English**

The reference is OpenCode TUI 2.2.5 and the approved mockups. The main layout uses
horizontal session tabs, a central conversation, a compact composer and the
Ctrl+P command palette. Permanent menus and the left tree are removed. Backend,
models and configuration remain native dsh features; the client stays on Go/Bubble Tea.

## Milestone 1: shell

Intermediate build: `0.2.0-preview.40-opencode.1`.

- Neutral theme with an amber accent and a block DSH home wordmark.
- Four contiguous composer rows, including actual model/agent/permission controls.
  Workspace and current mode remain visible below the input.
- Explicit visited-tab state, always-visible active session, shared rendering/hit
  geometry and separate drafts restored when switching sessions.
- Searchable, grouped RU/EN palette with availability states. Disabled entries
  never execute their neighbors. The palette captures mouse/keyboard input;
  it cannot open over a modal editor or a pending decision.
- Permanent menus/tree and the obsolete tree setting are removed. Compatible
  F1/F4/F11/F12 and Alt navigation remain during migration; F10 opens no menu.
- 80×24,100×30 and160×40 layouts; resizing retains the conversation tail.

Builds live under `build/previews/opencode-01/`; the installed TUI is unchanged.
From the repository root:

```sh
./build/previews/opencode-01/dsh-launcher-tui --start
```

This milestone does not finish conversation, tool or auxiliary-screen styling.
Those screens retain their existing implementations for now.

## Milestone 2: conversation and composer

Intermediate build: `0.2.0-preview.40-opencode.2`.

- “+ New” follows the visible tabs; unused space stays to its right.
- One conversation column without duplicate headings or Chat/Trajectory tabs.
  User messages use a quiet surface with a left accent; assistant Markdown has
  compact time, duration and source-copy controls. Question, approval and service
  message labels retain their meaning.
- Four contiguous composer rows stay pinned below the transcript, with the same
  agent/model/permission geometry as home. Mode, queue, stop, archive and child
  read-only guidance remain visible.
- Compact delivery, waiting, streaming and reconnect states; uncertain delivery
  retains explicit retry guidance. No automatic prompt retries are introduced.
- Code surfaces have language labels, wrapping and highlighting for both themes;
  Markdown caching accounts for language, theme and terminal color profile.
- F11/palette still opens history. Modal and tool styling belongs to later stages.

Run separately from the repository root:

```sh
./build/previews/opencode-02/dsh-launcher-tui --start
```

## Remaining approved plan

3. Shared action registry for palette, slash commands and shortcuts; common
   geometry/styling for model, session, settings and confirmation dialogs.
4. Tools/permissions: compact expandable results, prominent errors/questions
   and complete input capture by the topmost window.
5. Settings, Plugins, Plugin Market, Automation tasks, files, terminals,
   subagents, queue/history and installation/update/rollback/diagnostics:
   gradually adopt the same visual system while preserving dsh operations.
6. Final mockup comparison, Go race/PTY/actual-dsh scenarios in isolated profiles,
   user review, and only then replacement of the installed TUI.

## Verification and delivery

Each milestone gets a separate binary and actual terminal captures. Tests use
isolated HOME/DSH_HOME and a local fixture model, never personal profiles or
providers. Previous interface baselines remain under `apps/tui-go/test/golden/before/`.
