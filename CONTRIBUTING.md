# Contributing

## Language conventions

- **Code comments** — English. All `//`, `/* … */` and `#` comments in
  `main.js`, `src/`, `release.js`, `test/`, `test/docker/`, `*.sh`,
  `Dockerfile` and `.github/` workflows must be in English.
- **Commit messages** — English. Subject: imperative, one line,
  e.g. `Fix dsh install: pin @latest for npm 10.x`.
- **PR titles and descriptions** — English.
- **Test and assert messages** — English.

Russian is kept only where it is a *product* surface, not a code surface:

- `README.md` (Russian) and `CHANGELOG.md` (Russian release notes) —
  each mirrored in English as `README.en.md` / `CHANGELOG.en.md`;
- user-facing strings: UI text, launcher log lines (`console.log`),
  tray tooltips, dialogs, notifications;
- packaging strings the user sees: `deb/control`, `deb/postinst`.

If a comment explains user-facing Russian wording, quote the wording but
write the explanation in English.

## Development

```
npm test          # eslint + node --test test/*.js test/smoke/*.js
node test/markers.js   # marker-block integrity (also run by npm test)
npm run coverage  # unit coverage with the gates in package.json
```

The terminal minimum is 80×24; 100×30 is recommended. Resize tests also verify
that undersized windows retain drafts/dialogs and block hidden controls.

The terminal app lives in `apps/tui-go` and uses Go/Bubble Tea. Run
`go test -race ./...` and `go build` from that directory. PTY scenarios use
`apps/tui-go/test/terminal.py` and run through `test/terminal_preview.py` with
a built binary; install pyte in a project-local Python environment first.
The `tui-go-preview` workflow checks amd64/arm64 builds, PTY behavior without Node,
and actual dsh against a controlled local model endpoint (`apps/tui-go/test/real_chat.py`), using an isolated
profile and no paid model calls. This includes actual file reads and the dsh
approval service: grants, refusals, cancellation, and pending-request recovery.
Agent-question checks exercise actual ask_user_question, structured batches,
multiline text, timed holds, continued replies, and cancellation. Queue checks
verify two waiting follow-ups, reconnection, three individual FIFO turns, and confirmed removal of one pending
task while preserving the active turn and other work. Queue editing checks cover
discarding a draft, saving multiline text, model input, and retained FIFO order.
Steering checks promote a queued item to the next step of the same turn, verify
actual session journal events, and preserve remaining work across reconnect.
Nested PTC checks cover real file reads, errors, restart, and bash cancellation.
Command-output checks cover live stdout/stderr before completion, reconnect,
100×30/80×24 layouts, preserved model output, and cancellation.
`test/real_settings.py` verifies schema-driven provider editing/creation, model
catalog discovery, exact path writes, revision conflicts between two clients,
retained drafts, same-field review, and credential masking at three terminal sizes.
Settings editor checks cover multiline JSON model creation, exact Unicode/newlines,
staging before backend persistence, cancellation, portable schema constraints and
secret-container refusal.
An isolated dummy key is stored through credentials/set and checked in the next
request to the local model; no user profile or paid provider is touched.
`test/real_workbench.py` checks actual inventories, file pagination, explicit schedule
creation/pause/deletion, native shell execution/detach/close, multiline drafts,
upload receipts and exact message text, and both plan-review decisions. It uses
an isolated profile and a controlled local model; metadata browsing must not
trigger model calls. Advanced adapters require their own actual-backend scenarios.
`test/real_background.py` runs real shell jobs, native continuable children and
actual schedule delivery. It checks output snapshots, stop confirmation, child
follow-ups/history/search, direct child CLI reconnect, parent-address F8 and
navigation, schedule history, and no duplicate work across reconnect. A dedicated
producer checks ownerless jobs/gap markers through the actual registry. Capture
100×30/80×24 terminal cells with `--capture <prefix>`.
`test/real_runtime.py` verifies actual pinned-dsh user-local npm installation
(`--install`), owned start/restart/exit, a completely empty DSH_HOME, preservation
of an externally owned backend, report token masking, actual binary promotion and
recovery by executing `.previous --rollback-tui`. It copies the TUI into a temporary
HOME; the user's installed executable/profile and GUI remain untouched. Go tests
exercise failed/cancelled installs, stale pointers, cross-process ownership,
checksum/candidate/post-install failures, reports and modal confirmation/geometry.
The TUI workflow runs Go race checks and native builds as
independent jobs. PTY checks and seven actual-dsh suites fan out after the native
build and download the same binary artifact. Each integration suite has its own
runner/profile; PTC preparation runs only for chat. npm downloads are cached;
`fail-fast: false` lets other suites finish after one fails. The stable
`terminal-preview` aggregate rejects failed, cancelled or skipped prerequisites.
Publication waits for that aggregate, including all matrix entries.

The TUI workflow publishes binaries only for explicit `tui-v<version>` tags;
ordinary branch/PR checks produce artifacts without publishing a release.
`test/real_goals.py` checks actual goal snapshots, phase versus activation,
multiline objective/round-cap editing, cancellation, stale-revision refusal,
explicit conflict review, preservation of untouched foreign fields, empty goals,
reconnect and three terminal sizes. The isolated profile disables the automatic
goal-round driver and counts local model requests (must stay zero); this verifies
goal UI/protocol behavior, not autonomous round execution.
`test/real_history.py` uses another isolated actual-dsh profile to check paginated
history, fixed read cuts, page scroll positions, draft preservation, and reads
while the live model runs. Search checks cover oldest Unicode text beyond the
loaded window, next/no matches, query cancellation, pinned cuts, and refresh.
Session rename checks cover normalized Unicode titles, durable projections, list
display, cancellation, and reconnect at three terminal sizes. Fork checks cover
an active source, latest completed-turn history, cancelled actions, no automatic
model calls, independent child continuation, inherited model input, and refusal
when the source has no completed turn. Exact-message fork checks cover older
search hits, keyboard selection, inclusive user/assistant boundaries, synthetic
turn closers, retained drafts, and independent model input. Archive checks cover
active-session refusal, cancellation, durable membership across reconnect,
active/archive filters, a blocked composer, restored continuation, and changes
from another client. Markdown checks cover code and Unicode tables, source search,
raw/formatted toggling, preserved drafts, and unchanged committed text at three
terminal sizes. Clipboard checks decode PTY-only OSC 52 requests for exact source,
code, tabs, Unicode, and blank lines; graphical clipboard helpers are disabled
in this integration test so the host clipboard is untouched. Go tests cover
helper stdin, errors, size limits, cancelled/stale results, and picker geometry.
Syntax checks cover declared language aliases, exact code bytes and line wrapping,
Unicode, monochrome terminals, lexer failures and budgets. The dsh PTY scenario
requires actual string-token colors while copying the unchanged committed source.
History tool checks exercise collapsed/expanded filesystem results at three sizes,
Shift+F6, nested PTC card order, independent expansion under a collapsed parent,
and no additional model requests. Go tests additionally cover mouse hit regions,
page preference bounds, refresh reset, hidden/stale inputs and message boundaries.
Preview.30 checks include all/messages/tools search scopes, argument/result matches
scrolled into view, cross-page tool context and obsolete nested-card filtering.
The isolated history probe invokes real Session.append to test cross-page
replacement, replacement-of-replacement and reversed numeric endpoint order; no
user profiles or paid models are involved. The model-picker scenario reads the
actual dsh catalog, cancels at three sizes, applies an alternate configured model,
reopens its persisted selection, and asserts the next model request uses it. Go
tests cover reasoning effort IDs, omission of default effort, stale/cancelled
results, detected competing choices and blocked retries after unconfirmed writes. Go checks also cover invalid transitions,
cancellation, and no resurrection during older-page navigation/search.
Go WebSocket checks cover fresh archive baselines, increments,
disconnection, replacement on reconnect, and cancellation.
It uses a local controlled provider and no paid calls.
Run `python3 apps/tui-go/test/prepare_ptc.py build/ptc-bootstrap` first, then pass
`--ptc-bootstrap "$PWD/build/ptc-bootstrap/process-entry.ts"` to `real_chat.py`.
This verified official source worker is needed only because the pinned npm
package omits its advertised PTC bootstrap; it is not shipped with the client.
The test-only approval probe is never enabled by the live deployment.
The static WebAssembly/browser preview has been removed; use the native binary.
The legacy JavaScript terminal app and its npm/Debian packaging are removed.

## CI

- **build-deb** — runs on every PR and tag: unit tests, ESLint, coverage,
  package build, and (by tag) the release.
- **docker-tests** — scenarios for the installed .deb on clean
  Ubuntu 24.04 / 26.04 and Debian 13. It is manual only; trigger it with

  ```
  gh workflow run docker-tests --ref <branch>
  ```

  and wait for it to go green before merging. Do not run these scenarios
  locally in Docker.

## Merging

- Direct pushes to `main` are closed; everything lands through a PR.
- Own PRs are merged by the maintainer with `gh pr merge <n> --admin`
  after build-deb and (for anything touching runtime behavior) docker-tests
  are green.
- Releases: fill `## [Unreleased]` in both changelogs,
  `npm run release X.Y.Z`, PR, merge, then tag `vX.Y.Z` on `origin/main`
  (see the header of `.github/workflows/build-deb.yml`).
