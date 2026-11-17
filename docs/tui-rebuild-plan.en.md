# OpenCode-style dsh TUI

[Русский](tui-rebuild-plan.md) | **English**

The visual reference is the user's four OpenCode 2.3.0 screenshots (home,
conversation with right context panel, command palette, session/project picker).
They supersede approximate mockups and the earlier 2.2.5 reference where they differ.
The implementation remains Go/Bubble Tea with actual dsh behavior and configuration.

Existing `opencode.1` and `.2` builds are technical checkpoints, not a claim of
visual parity. In `.2`, the width cap, repeated footer hints, permanent copy labels
and global amber accent still differ from the supplied reference.

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

## Milestone 3: correction from the user's screenshots

Intermediate build: `0.2.0-preview.40-opencode.3`.

- Home input has 5 rows with top/bottom padding; active conversation retains
  4 rows. Height and text placement were checked against installed OpenCode2.3.0
  running in a separate temporary profile.
- Removed the 100-column conversation cap. At 120+ columns a 30-column right
  panel shows the title, known context and workspace; the palette includes
  “Show/hide context”. Narrow terminals hide the panel automatically; home keeps
  its centered composer. The first message reflows without a resize event.
- One bottom line shows workspace, known context and Ctrl+P. Running/queue state,
  plan/goal mode, delivery failures, archive and child read-only guidance replace
  secondary details when needed.
- Purple agent accent, teal palette groups/selection with dark selected text.
  Opening the palette dims the backdrop without moving its text.
- Removed permanent message clocks/copy buttons. Ctrl+Y, palette and history
  selection still copy original source. Assistant metadata shows known role/duration;
  semantic question and approval labels are preserved.
- MCP inventory and historical model identity are omitted because current projected
  records do not provide reliable data. Unified session/model pickers are still ahead.

```sh
./build/previews/opencode-03/dsh-launcher-tui --start
```

## Milestone 4: actions and pickers

Intermediate build: `0.2.0-preview.40-opencode.4`.

- Palette, exact no-argument commands and base shortcuts share a registry of stable
  IDs, labels and availability checks. Their keyboard reference is generated from
  it. Argumentful `/session`, `/mode`, `/attach`, `/goal` and `/plan off`, plus local
  editor/list keys, deliberately retain their existing handlers.
- Commands, sessions/workspaces and models share a flat frame: search, groups,
  teal selection, top-right Esc, dim backdrop and measured mouse geometry.
  Position/height are compared with running OpenCode; frames use about half the
  terminal height, with room for model disclosures at 80×24.
- `/sessions` and Alt+S open a session/workspace picker. Enter on a workspace
  filters its sessions without creating one; Ctrl+N explicitly creates there.
  Search includes title, ID and path; Tab changes groups. Partial catalog failures
  retain usable sections; canceled/stale results cannot reopen a modal or cross profiles.
- `/sessions-manager`/F2 and `/workspaces-manager`/F3 retain complete session and
  workspace management, including archive, deletion, rename and fork.
- Models are grouped by provider, with the actual current model marked. The provider
  heading stays visible when scrolling a short window. F2/Tab changes reasoning;
  Enter applies. Cancel and row clicks do not write. Shared-default, next-step and
  unconfirmed-write disclosures and protections are preserved.
- Paste and the topmost modal capture input; drafts, visible rows and mouse targets
  are checked together. Navigation from plugin pages preserves the first typed key.

```sh
./build/previews/opencode-04/dsh-launcher-tui --start
```

## Checkpoint 5: tools, thoughts and decisions

Intermediate build: `0.2.0-preview.40-opencode.5`.

- Collapsed tools use at most two rows: name, actual state, a short command/path
  and result preview. Errors have a distinct color; missing results, cancellation,
  pending decisions and incomplete output are never presented as success.
  F6/Shift+F6 selects a tool; Enter/click expands arguments and output.
- Thoughts come only from explicit dsh reasoning blocks, separately from replies:
  a collapsed header, click, Ctrl+O or `/thoughts` reveals them. Streaming,
  restored attempts and durable history are supported; Ctrl+O works in history.
  Reply copying retains its source without adding thoughts.
- Permissions and questions use flat dimmed dialogs, top-right Esc, clear buttons
  and scrolling for complete reasons, arguments and questions. Decisions remain
  explicit; deferred requests and drafts are preserved. F7 from the palette or
  session picker reveals a pending request without answering it.
- Text limits, stale-result and uncertain-answer guards, disconnection states
  and input capture by the topmost modal remain in place.

```sh
./build/previews/opencode-05/dsh-launcher-tui --start
```

## Revised implementation order

1. **Screenshot correction, preview `opencode.3` (implemented; user review pending).** Active conversation
   uses the available width, with an optional right context panel on wide terminals;
   home keeps a compact centered composer. One bottom status line shows workspace,
   known context usage and commands. Agent accent is purple; list selection teal.
   Ordinary messages have no permanent copy button or clock; source copying remains
   through Ctrl+Y/palette/history. Preserve semantic question/approval/service labels.
   Context shows actual dsh data only; absent historical model/MCP data is omitted.
2. **Shared actions and pickers (implemented in preview4; user review pending).** One action registry for palette, slash and shortcuts;
   flat searchable grouped command/model/session/project pickers, Esc at top right,
   teal selection and dimmed backdrop. Core pickers are unified in preview4;
   editors and confirmations continue in subsequent stages.
3. **Tools and permissions (implemented in preview5; user review pending).** Compact expandable results/thoughts, clear failures and
   decisions, complete input capture by the topmost modal. Show only information dsh provides.
4. **Auxiliary pages.** Settings, plugins/market, automation, files, terminals,
   subagents, queue/history, install/update/rollback and diagnostics use the same
   visual system while retaining operations and revision/confirmation safeguards.
   Queue work also covers classifying and clearing the transient accepted-send
   notice, which can currently remain after a normal reply.
5. **Visual acceptance.** Compare actual captures of the four supplied scenes at a
   wide size and at 100×30/80×24; test Go race/PTY/isolated real dsh. Ask for a specific
   additional screenshot only when the current references leave a necessary state unclear.
   User reviews intermediate builds before any replacement of an installed application.

## Verification and delivery

The installed OpenCode 2.3.0 can serve as an executable reference: run it with
`--standalone` in temporary HOME/XDG directories and a temporary workspace. Capture
actual terminal cells at 80×24, 100×30 and 160×40, then compare with dsh at the same
sizes. Personal configuration, credentials and model providers are excluded;
conversation reference scenarios use only a local deterministic model fixture.

Each milestone gets a separate binary and actual terminal captures. Tests use
isolated HOME/DSH_HOME and a local fixture model, never personal profiles or
providers. Previous interface baselines remain under `apps/tui-go/test/golden/before/`.
