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

The terminal app lives in `apps/tui-go` and uses Go/Bubble Tea. Run
`go test -race ./...` and `go build` from that directory. PTY scenarios use
`apps/tui-go/test/terminal.py` and run through `test/terminal_preview.py` with
a built binary; install pyte in a project-local Python environment first.
The `tui-go-preview` workflow checks amd64/arm64 builds, PTY behavior without Node,
and the static WebAssembly browser preview. It also runs actual dsh against a
controlled local model endpoint (`apps/tui-go/test/real_chat.py`), using an isolated
profile and no paid model calls. This includes actual file reads and the dsh
approval service: grants, refusals, cancellation, and pending-request recovery.
The test-only approval probe is never enabled by the live deployment. See `apps/tui-go/web/README.md`.
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
