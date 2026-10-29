# Changelog

[Русский](CHANGELOG.md) | **English**

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
The `## [X.Y.Z]` section becomes the description of the `vX.Y.Z` GitHub release.

## [Unreleased]

<!-- сюда дописать изменения для следующего релиза (### Добавлено / Изменено / Исправлено) -->

## [1.17.3] — 2026-10-29

### Changed

- **Electron 45.0.0** (was 44.9.0) — major version: Chromium 156,
  Node 24.22, V8 15.6. No launcher code changes were needed: there are
  no preload scripts (the sandboxed-preload restrictions do not apply),
  screen/window capture now comes in as `display-capture` and is
  denied as before, and Electron dropped 32-bit builds (ia32, armv7l)
  while the launcher ships x64 only.

## [1.17.2] — 2026-10-22

### Fixed

- The `v1.17.1` GitHub release notes (and the launcher’s “What’s new”):
  a service comment from `## [Unreleased]` (“add changes for the next
  release here”) had leaked into them. The comment is no longer carried
  into a version section: `release.js` assembles the section without
  single-line HTML comments, and the release-notes builder (awk in CI)
  strips them. The offending line has been removed from the published
  `v1.17.1` release.

### Changed

- **Electron 44.9.0** (was 44.7.0) — Chromium/V8 fixes, including
  `net.fetch` after a Network-process restart. Dependabot proposes
  dependency updates weekly; PRs are checked by the tests and the build.

## [1.17.1] — 2026-10-21

### Fixed

- Auto-recovery of the window after a dsh restart (plugin update,
  marketplace self-restart): the window used to reload as soon as the TCP
  port opened — but the port opens before the dsh app is actually ready
  (plugin boot, bili attach). Reloading “blind” against a still-booting
  server did not restore the session and left a gray (empty) window, so the
  user had to restart dsh by hand. Now the watchdog waits until dsh really
  responds (valid session cookie) and only then connects the window through
  the same path as “dsh started manually”; if dsh is not ready within
  ~15 s, it shows a “dsh is not ready” page with “Check again” and
  “Restart dsh” buttons instead of the gray window.

## [1.17.0] — 2026-10-19

### Changed

- Updating dsh with a system-wide npm (prefix `/usr`, `/usr/local`) now runs
  through `pkexec`: polkit asks for the password in a system dialog, exactly
  like the launcher update itself. Previously the launcher always sent the
  user to run `sudo npm install -g` in a terminal. User prefixes (`nvm`,
  `~/.npm-global`) are still updated without root: `sudo` would leave files
  there that cannot be updated later. The same path covers the “Install”
  button on the “dsh not found” page and installing `pnpm` for the
  marketplace.

## [1.16.0] — 2026-10-16

### Added

- Input history: `↑`/`↓` in the dsh input field recall previously sent messages
  (up to 50, like a terminal), `Esc` restores an unsent draft. Stored in
  `~/.config/dsh-launcher/input-history.json`, never written to the log; can be
  turned off in “Settings” → “General”.

## [1.15.14] — 2026-10-16

### Added

- A smoke test that loads `main.js` in the unit suite (`npm test`): the entry
  point and the launcher registry are validated without Electron.

### Changed

- Launcher registry 5 → 3: `blankResponse`/`noContentResponse` moved to a pure
  module `src/respond.js` (direct require, no registry).
- Service `data:` pages (status, find bar, update toast, closing window) now
  set a Content-Security-Policy: no external resources.
- CDP driver (Docker tests): wait timeout errors now show the last exception
  from the page.

## [1.15.13] — 2026-10-16

### Changed

- The `launcher` registry in `main.js` was trimmed from 17 names to 5:
  pure modules (`src/port.js`, `src/gpu.js`) are required directly by
  their consumers; the registry is left for names from electron modules.
- Docker test driver: "paste the token" is retried up to 3 times if the
  synthetic Enter was lost under load (Ubuntu 26.04 flake).
- `SHA256SUMS` is now included in the artifact and the GitHub Release —
  for verifying the checksum during manual installation.
- `release.js` now creates a new empty `## [Unreleased]` section (with a
  hint comment) after a release, so it no longer needs to be written by hand.
- New unit tests for `src/system.js`: kill-target logic (`pickKillTargets`),
  port → listener pid, token URL parsing, process environment.

## [1.15.12] — 2026-10-16

### Changed

- **main.js structure**: code moved into dedicated modules — `src/gpu.js`
  (GPU/EGL/ozone, the “broken GPU” marker), `src/port.js` (dsh port, stderr
  tail), `src/routes.js` (the dshlauncher:// scheme and handlers),
  `src/watchdog.js` (window auto-recovery), `src/signals.js` (signals and
  hard dsh stop). `main.js` now holds only the window and lifecycle.
- Installation-step cleanup on exit (dsh, marketplace, launcher update) is
  centralized: modules register cleanup via `state.settleOperation`,
  before-quit runs them all at once.
- `npm start` runs through `run.sh` (with `--no-sandbox`, same as
  `./run.sh`); `engines: node >=22` is now declared.
- CI: unit-test coverage thresholds (lines ≥95%, branches ≥95%, functions
  100%) — the build fails if coverage drops below them. The report covers
  `src/` and `release.js` only (the test files themselves are excluded) —
  otherwise their coverage would differ between Node versions.

## [1.15.11] — 2026-10-15

### Fixed

- “What’s new”: if the release list couldn’t be fetched at startup (slow
  network), the skipped changes are shown within a minute of startup — or at
  the next successful update check — instead of piling up until the next
  update.

## [1.15.10] — 2026-10-15

### Changed

- **Live log on the “running” pages** (dsh install, marketplace install,
  updates and .deb download). New lines are now appended to the page without
  reloading it: scrolling stays with the user — if you scrolled the log up to
  read it, new lines no longer yank the view back to the first line; if you
  are at the bottom, the log stays pinned there as before.

## [1.15.9] — 2026-10-15

### Fixed

- **Launcher version in source builds.** "About", the diagnostics report,
  and "What's new" read the version from a `package.json` in the adjacent
  `src/` directory, which does not exist — the version showed as "unknown"
  and "What's new" never appeared. Package (.deb) installs were unaffected:
  their version always comes from dpkg.

## [1.15.8] — 2026-10-15

### Added

- **A "DeepSeek Harness is closing…" window on exit.** While the launcher stops
  dsh (up to a few seconds), the main window is already hidden, and the app
  looked hung or not fully closed. Now, if stopping takes a while, a small
  window with the icon and a spinner appears in the middle of the screen; a
  quick exit goes without it.

### Fixed

- **The window no longer asks for a token after dsh is restarted from the
  market.** The market starts the new dsh itself, and that process prints its
  login link to its own file (`/tmp/dsh-market-restart-*.out.log`), not to the
  launcher, so the window showed "Token required". The launcher now takes the
  link from the output of the process listening on the port and, while dsh is
  coming up, re-checks the login for a few seconds before showing the
  paste-token page. The same applies when attaching to an already running dsh
  and to "Check again".

## [1.15.7] — 2026-10-14

### Fixed

- **The counter on the dock icon no longer keeps growing.** The “The agent has
  finished” notification stayed in the GNOME notification list even after the
  window was opened — the Ubuntu dock counted it as unread, and the next one
  showed “2”, “3”… Now there is only one notification (a new one replaces the
  old one), and it is closed as soon as the window is opened.
- **A window hidden to the tray in the first seconds after launch no longer
  pops up by itself.** After 8 s the “the window never appeared” safety net
  (for Wayland) showed it again.

### Changed

- The package recommends `libnotify4`: without it system notifications are
  not shown (a regular desktop has it anyway).

## [1.15.6] — 2026-10-13

### Fixed

- The settings no longer accept `.` or `..` as the profile: the profile
  directory would point at `~/.dsh` itself or above it.

### Changed (internal)

- The Docker scenarios measure coverage of the launcher's code
  (`NODE_V8_COVERAGE` of the main process, c8 report) — in the Summary tab of
  the `docker-tests` run.
- A test of the whole `npm run release` (`main()` on a temporary git
  repository: every refusal, `--no-commit`, the commit and tag); new unit test
  cases — prerelease version order, settings validation, “What's new”, why
  the GPU is disabled.
- New Docker scenarios: the marketplace plugin prompt (install together with
  pnpm, a failed install, “Don't ask again”), the “Token required” page with
  pasting a token, “Failed to start dsh” and switching to the GUI once dsh
  comes up after all.

## [1.15.5] — 2026-10-13

### Security

- **Launcher commands are accepted only from its own pages.** Before, any page
  in the window — the dsh GUI with model output, a plugin UI or a site opened
  from a link — could navigate to `dshlauncher://…` and, for example, change
  the dsh path in the settings and restart the launcher. Now the launcher's
  page links carry a random key, new on every launch; a request without the
  key is rejected and logged.
- **A link to another site opens in the system browser**, not in the window
  (before, only links opening a new tab did).
- **Only the browser permissions that are needed:** clipboard writes and
  fullscreen. Microphone, camera, geolocation and notifications are denied
  (before, Electron silently granted them to any page).
- A token in the `Authorization` header is not sent to another address on a
  redirect.

### Changed (internal)

- `npm run coverage` — unit test coverage report (in CI — the build's Summary
  tab); `test/markers.js` checks the marker blocks of functions the tests take
  from module source.

## [1.15.4] — 2026-10-13

### Fixed

- The run marker (how the launcher learns that the previous run crashed)
  records the launcher version again: since the code moved to `src/`, the log
  showed “?” instead of the crashed run's version.

### Changed (internal)

- **The Docker update and rollback scenarios no longer depend on real GitHub
  releases:** the release list and the `.deb` files come from a fake GitHub
  (`test/docker/fakes/github.py`, with ETag and paging like GitHub's). New
  scenario: a `.deb` with a wrong sha256 is not installed. The test address is
  `DSH_LAUNCHER_GITHUB_API`; the launcher only accepts
  `http://127.0.0.1:<port>`.
- The latest release is parsed by the same function as the rollback list.
- `actions/checkout` v7 (Dependabot, PR #4).

## [1.15.3] — 2026-10-13

### Fixed

- **If installing dsh from the window fails to start** (npm not found, etc.),
  the error text now shows up in the log on the install page — before, the
  log stayed empty.

### Changed (internal)

- Running a command with a live log (installing dsh, the market plugin,
  update steps) is one shared module instead of three copies; so are the
  settings files (`update-prefs.json`, `market-prompt.json`, `ui-lang.json`).
- New test: every `launcher` registry name the modules use is registered in
  `main.js`.

## [1.15.2] — 2026-10-13

### Fixed

- **The restart button in the plugin market (dshmarket) works a second time
  too.** After the first restart from the market, dsh is adopted by systemd,
  and the systemd service variables (`INVOCATION_ID`, `JOURNAL_STREAM`) the
  launcher inherits from the GNOME session were passed on to dsh — so the
  market took it for a systemd service: “Restart failed: self-restart is
  disabled for this host”. The launcher no longer passes them to dsh.

## [1.15.1] — 2026-10-12

### Fixed

- **Alt no longer moves focus to the menu bar, including the `Shift+Alt`
  layout switch** (on KWin it doesn't arrive as “Alt”): both Alt keys are
  suppressed by physical code regardless of the layout; AltGr still types
  characters. Thanks @MrGnUm (#3).
- **“Roll back to a previous version…” sees older releases too** — the
  release list is requested in full (100, not the latest 20).

## [1.15.0] — 2026-10-12

### Added

- **Crash dumps in the report.** If the launcher crashed, “Help → Save
  report…” adds crash dumps (up to three from the last 30 days) to the
  archive — they help find the cause. Electron keeps them locally only
  (`~/.config/dsh-launcher/Crashpad`) and never sends them anywhere.

### Changed

- **Electron 44.7.0** (was 44.5.1) — Chromium fixes. Dependabot proposes
  dependency updates weekly; PRs are checked by the tests and the build.

## [1.14.0] — 2026-10-12

### Added

- **“The agent is waiting for you” signal:** the agent asks to approve a
  command, has a question or waits for a plan review while the window is not
  focused — a notification, a dot on the tray icon and (if on) a sound, like
  for “the agent has finished”.
- **Auto-restart of dsh after a crash:** if dsh crashes by itself, the
  launcher restarts it (at most three times in 10 minutes) and shows a “dsh
  was restarted after a crash” notice with a “dsh log…” button; if it keeps
  crashing — a page with the log, no endless restarts.

### Changed

- **The Chromium sandbox is off by default** (`--no-sandbox`): on some
  desktops (Electron 44, Ubuntu 26.04) the launcher crashed with it on
  “Restart dsh”. It can be turned on in “Settings → Graphics”.

- **Static code check (ESLint)** — in `npm test` and in CI before every
  build: unknown names, unused imports, unreachable code. The leftovers of
  the refactoring it found (unused imports) are removed.
- **Modules call each other directly** (`pages.showPage()` etc.) instead of
  the shared registry; the registry is left for what lives in `main.js`.

### Fixed

- **“Saved” in Settings** popped up in the corner of the whole window,
  outside the dialog card; now it's in the card's corner.

## [1.13.0] — 2026-10-12

### Added

- **“Help → Roll back to a previous version…”** — if a new version doesn’t
  suit you: a list of past releases (up to five) and a “Roll back” button.
  The launcher restarts and installs the chosen version the same way as an
  update (download, sha256 check, pkexec); it won’t offer the newer version
  again — this can be undone in “Settings → Updates”.

### Changed

- **Dialogs live inside the main window** instead of separate windows:
  “About”, “Settings”, “Check for updates…”, “dsh log”, “What’s new”, the
  report. The dsh page is dimmed, the card is centred; no extra windows in
  the taskbar or Alt+Tab. Close with the button or Esc. File pickers stay
  native.

### Fixed

- **Alt no longer moves focus to the menu bar.** A lone Alt press highlighted
  “File” and took focus from the dsh input; Alt shortcuts work as before.

## [1.12.0] — 2026-10-12

### Added

- **Tray and background mode.** “Settings → Window → The close button hides
  the window to the tray”: the window closes, dsh keeps running; the icon
  menu has “Open”, “Restart dsh”, “Quit”. Off by default: not every desktop
  has a tray (in GNOME — the AppIndicator extension, included in Ubuntu).
  Starting the launcher again also brings the hidden window back.
- **“The agent has finished” signal.** Window not focused or hidden — a
  desktop notification with the session title (a click opens the window), a
  dot on the tray icon, under X11 the taskbar button flashes; optionally a
  sound. Only the session open in the window is seen. Set up in “Settings →
  Window”.

### Changed

- **The code is split into modules** (`src/`): `main.js` went from ~4500 to
  ~800 lines — it only assembles the app. Behaviour is unchanged.
- **The Settings window has sections on the left:** “General”, “Updates”,
  “Starting dsh”, “Window”, “Graphics”. “Saved” and errors pop up in the
  window corner and don’t shift the fields.

## [1.11.1] — 2026-10-12

### Fixed

- **“About” crashed the launcher** on some systems (the window stopped
  responding, then the launcher closed). The cause is an Electron fault:
  running `dpkg-query` and `dsh --version` while the dialog window loads.
  The versions are now read without starting processes — from the dpkg
  database and the dsh package’s `package.json`; the window also opens
  faster.

## [1.11.0] — 2026-10-11

### Added

- **“Help → Save report…”** (also a button in “About”): an archive with
  versions, system, session and GPU details, settings, the launcher log and
  the tail of the dsh log — tokens and keys hidden. Handy to attach to an
  issue.
- **A notice after a crash.** If the launcher closed unexpectedly last time,
  the next start shows a notice in the window corner with “Log…” and “Save
  report…” buttons.

### Fixed

- **“Restart the launcher”** (Settings window, “Update now”): the new
  instance starts once the previous one has fully exited and no longer
  inherits its open files and sockets (Chromium left them inheritable); the
  same after an update.
- **dsh no longer gets the launcher’s open files and sockets.** It used to
  inherit dozens of Chromium descriptors (shared memory, sockets) and kept
  them even if the launcher crashed.

## [1.10.0] — 2026-10-11

### Changed

- **Full name — DeepSeek Harness Launcher** instead of “DSH Launcher” in
  windows, menus, the update notice and README. The `dsh-launcher` package,
  command and settings/log folders stay the same: updates and settings are
  unaffected.
- **A new release is seen at once.** Within an hour of a check the launcher
  used to take the release list from its cache and miss a just-published
  version. Now every check asks GitHub “has it changed” (by ETag): a 304
  reply doesn’t use up the request limit. If GitHub is unreachable (no
  network, rate limit), the cached list is used when under an hour old —
  such a check used to just fail.

## [1.9.0] — 2026-10-11

### Added

- **Launcher log** `launcher.log` next to `dsh.log`
  (`~/.local/state/dsh-launcher/`): what the launcher itself did — update
  checks, starting and stopping dsh, errors; timestamped, tokens hidden,
  moved to `launcher.log.1` after 2 MB. These messages used to be lost when
  started from the desktop shortcut.

### Fixed

- **Logs no longer grow without limit.** `dsh.log` was moved to `dsh.log.1`
  only when dsh started; with the launcher open for weeks the file could grow
  indefinitely. Now this also happens while running; at most four log files
  (~8 MB) on disk.

## [1.8.1] — 2026-10-11

### Fixed

- **“What’s new” without notes.** When the cached release list (up to an
  hour old) predated the installed version, the window showed only a GitHub
  link. The list is now fetched again.
- A run with a fake version (`DSH_LAUNCHER_FAKE_INSTALLED`, for development)
  no longer records it as the previous version for “What’s new”.

## [1.8.0] — 2026-10-10

### Added

- **Update check while running** — every 12 hours while the launcher is open
  (previously only at startup). A new launcher or dsh version interrupts
  nothing: a notice in the window corner (once per version) and a “Help → Update …
  to X…” item with “What’s new” and an “Update now” button — the launcher
  restarts and installs the update right away. “Update” buttons are also in
  “Help → Check for updates…”. “Settings” has separate checkboxes: the
  check at startup and every 12 hours (the second applies at once).

## [1.7.0] — 2026-10-10

### Added

- **Settings window** (“File → Settings…”, Ctrl+,): interface language,
  spell check; update check at startup, “ask again” about postponed
  versions, the marketplace plugin offer; the dsh program, profile, working
  folder, port; GPU acceleration and the window mode on Wayland. What used
  to be set only with environment variables can now be chosen in the
  window; the variables still take precedence (the field is then
  disabled). Stored in `~/.config/dsh-launcher/settings.json`.

### Changed

- **Scrollbars** in the launcher's windows and pages are thin and match the
  window colours.

## [1.6.0] — 2026-10-10

### Added

- **“What’s new” in the launcher update prompt** and in “Help → Check for
  updates…”: changes of every version newer than the installed one, newest
  first, in the window language.
- **“What’s new” after an update:** on the first start of a new version — a
  window with the changes between the previous and the new version (once).
- **“Help → dsh log”:** the latest lines of `dsh.log` in a window — with
  “Refresh” and “Copy”; the login token and key-like strings are hidden
  (`***`).

### Changed

- **The “About” / “Check for updates” window is bigger** (760×580) and
  resizable; buttons are at the bottom.
- **Gentler on the GitHub API limit** (60 requests per hour per IP without
  auth): the release list is remembered; at startup it is checked at most
  once an hour, and repeated requests carry an ETag (a “not modified” reply
  does not count against the limit).

## [1.5.1] — 2026-10-10

### Fixed

- **The package installs on Debian 12 and Ubuntu 22.04.** The dependencies
  used only the new library names (with the `t64` suffix), which those
  systems don't have, so `apt` refused to install the package. Each now
  lists the old name too. Tested on Debian 13 (all scenarios) and Debian 12.

## [1.5.0] — 2026-10-09

### Added

- **“Edit” menu** in the window menu bar: “Cut”, “Copy”, “Paste”, “Select
  all” — the same actions as the context menu.
- **“View” menu:** reload (F5), reload ignoring cache (Ctrl+Shift+R), zoom
  (Ctrl+= / Ctrl+− / Ctrl+0), find in page, spell check, full screen (F11).
- **Find in page (Ctrl+F):** a bar in the top-right corner with a match
  counter; Enter / Shift+Enter (or F3 / Shift+F3) — next / previous, Esc —
  close.
- **Spell check** in text fields (Russian and English): suggestions and
  “Add to dictionary” in the context menu. Toggle it in “View”. Chromium
  downloads the dictionaries the first time it is on.
- **The window size is remembered** between launches (and “maximized”).
- **“File → Restart dsh”** (asks first; a dsh not started by the launcher
  is left alone) and **“Help → Open logs folder”**.
- **“About”** shows the logs path, a “Logs folder” button and the GPU
  acceleration state; if the GPU was turned off after crashes — a
  “Re-enable GPU” button (instead of deleting the marker by hand).

### Changed

- **The menu follows the usual layout: “File” (restart dsh, quit),
  “Edit”, “View”, “Help”** (check for updates, logs folder, “About”)
  instead of a single “Application” menu.
- **“About” and “Check for updates” open in a small window over the
  launcher** instead of a separate page. The dsh GUI underneath is dimmed
  but stays as it was — no reload, typed text kept. Close with “Close”,
  Esc or the window's close button.
- **The update check and the package download use the system proxy
  again** (GNOME/KDE settings or `HTTPS_PROXY`), as before 1.3.1 — without
  the Electron network module that made the launcher crash.

## [1.4.0] — 2026-10-09

### Added

- **Right-click context menu** — in the dsh GUI and on the launcher's own
  pages: “Cut” / “Copy” / “Paste” / “Select all” in text fields, “Copy” on
  selected text, “Copy link address” on links. Labels follow the window
  language (RU|EN).

## [1.3.1] — 2026-10-09

### Fixed

- **The launcher quit when opening “About”** (and could quit on “Check for
  updates”). Cause: an Electron crash — after the startup update check, the
  first external command (`dpkg-query`, `dsh --version`) took the process
  down. The update check and the package download now use the Node.js HTTP
  client. Side effect: they don't use the system proxy — on a network with
  internet access only through a proxy, the launcher won't learn about a new
  version (as when offline), but it keeps working.

## [1.3.0] — 2026-10-09

### Added

- **Window menu “Application”.** The menu bar is now always visible, with a
  single “Application” item:
  - “About” — the launcher version (dpkg for packaged installs, package.json
    for source builds — marked as such) and the dsh backend version, the dsh
    path, Node.js and OS; a “Copy for report” button puts the whole block on
    the clipboard.
  - “Check for updates” — on-demand re-check of the latest release on GitHub
    and the latest `@deepseek-ai/dsh` on npm, shown per component (no
    auto-install — updating still happens through the startup prompt).
  - “Quit” — standard shutdown (stops the dsh the launcher started).
  Menu labels follow the window language (RU|EN) and are rebuilt on switch.

## [1.2.0] — 2026-10-08

### Added

- **Update checks at startup.** The launcher asks GitHub for its latest
  release and npm for the latest `@deepseek-ai/dsh`; if a newer version
  exists and dsh is started by the launcher itself, a window asks before dsh
  starts: “Update / Not now / Don’t ask again” (remembered per version — a
  newer release asks again).
  - Launcher update: downloads the release .deb (progress on screen, sha256
    checked against the asset digest from GitHub) and installs it via
    pkexec — polkit asks for the password in a system dialog; as root the
    package is copied to a root-owned directory and its sha256 re-checked,
    then `apt-get install`; on success — an “Update installed” screen and an
    automatic launcher restart. No pkexec / cancelled password / failure — a
    page with the terminal command (`sudo apt install -y <deb>`) and a
    “Check again” button.
  - dsh update: `npm install -g @deepseek-ai/dsh@<version>` with a live log
    when the global npm prefix is writable without sudo; otherwise the
    terminal command. After success dsh starts with the new version
    immediately (if the dsh being run is still the old one — npm installed
    into another prefix — the launcher says so).
  - In attach mode (dsh already running) no prompts are shown.
  - Requests are sent right at startup, in parallel with everything else;
    before starting dsh the launcher waits for them at most ~8 s (without a
    network they fail at once and the launcher simply doesn’t ask). Disable with
    `DSH_LAUNCHER_NO_UPDATE_CHECK=1`.
  - For development/testing: `DSH_LAUNCHER_FAKE_LAUNCHER_LATEST`,
    `DSH_LAUNCHER_FAKE_DSH_LATEST`, `DSH_LAUNCHER_FAKE_INSTALLED`.
- **Tests.**
  - `npm test` — unit tests `test/version.js` and `test/port.js`: version
    comparison (Debian and semver formats, CI tag rules) and looking up a
    process pid by its listening port via /proc. They run in CI before every
    build — a failing test blocks the package build and the release.
  - `test/docker/run.sh` — scenarios of the installed .deb in Docker on
    clean Ubuntu 24.04 and 26.04 (Xvfb, window driven via CDP): closing the
    window and signals, the “adopted” dsh, updating dsh via npm and the
    launcher itself via pkexec. In CI — a separate `docker-tests` workflow,
    run manually.

### Fixed

- Closing the window no longer leaves `dsh` running when it **restarted
  itself** mid-session (after a plugin update): the launcher “adopted” the
  new process and silently switched to attach mode, so nothing stopped it on
  close. Such a process is now stopped — its pid is looked up by the
  listening port.
- Closing the window no longer leaves `dsh` processes behind when the tree's
  main process had already exited but live children remained in the group:
  liveness is now checked by process group, not by the main process's exit
  code.
- A **second SIGTERM/SIGINT/SIGHUP** to the launcher (pkill twice, session
  end) killed it instantly: Chromium handles only the first signal (graceful
  quit) and then restores the default handler — and a `dsh` that had not
  exited by then was left orphaned. The launcher now handles the signals
  itself: the first one — graceful quit (dsh gets SIGTERM, SIGKILL after
  4 s), a second one or a hang longer than 8 s — SIGKILL to the tree and
  exit. (A direct `kill -9` to the launcher cannot be caught — in that
  case `dsh` survives it.)

## [1.1.1] — 2026-10-03

### Fixed

- The launcher did not find dsh installed outside `/usr/bin` (via nvm,
  `~/.npm-global`, etc.), including one installed with the “Install”
  button: the next launch offered installation again. dsh is now looked up
  in `PATH`, npm directories and every nvm version, the path found is
  remembered, and dsh runs with its own node even from the desktop shortcut.
- On a clean Ubuntu 26.04 with Node.js from apt the window said “npm is not
  installed”: the global packages directory `/usr/local/lib/node_modules`
  does not exist yet, which was mistaken for missing npm. npm is now also
  looked up in nvm (nvm is not in `PATH` when launched from the shortcut).
- Installing the marketplace on a clean system failed with “code 127”: dsh
  plugins need pnpm. The launcher now installs pnpm itself when npm allows
  it without sudo, and otherwise explains which command to run.
- After installing dsh from the window, an “ERR_ABORTED” error could be
  shown instead of the UI: two page loads interrupted each other.
- Node.js too old (e.g. 18 from apt on Ubuntu 24.04): npm installed dsh
  without any warning, then dsh did not run and the window was confusing
  (“not found” plus a stack trace tail). The window now says up front that
  Node.js vX was found while 22+ is required, and does not offer the
  “Install” button; for a dsh that fails to run, the actual error is shown.

## [1.1.0] — 2026-10-03

### Added

- **Marketplace plugin.** Before starting dsh, the launcher checks whether
  `dshmarket` is installed in the profile and, if not, offers to install it
  right from the window (“Install” / “Not now” / “Don’t ask again”).
- **Russian and English UI.** The launcher's own screens (startup, dsh
  installation, marketplace, token paste, errors) are available in two
  languages. An **RU | EN** switch in the top-right corner remembers the
  choice; the default is the system language, `DSH_LAUNCHER_LANG=ru|en`
  forces it.
- English README (`README.en.md`).
- `DSH_ARGS` is parsed like a shell command line: `"…"`, `'…'` quotes and
  `\` escapes are supported.

### Fixed

- Buttons on the launcher's screens did not work: “Check again”, “Copy
  command”, “Install” for dsh and “Open” on the token paste screen.
- Double-clicking “Check again” / “Install” could start a second dsh
  process that was then not stopped when the window closed.
- After dsh was started again, the window could open a URL with a stale
  token.
- The window reloaded needlessly right after dsh started.
- The window froze while checking the dsh and npm installation.
- The “dsh is not running” screen lacked the “Check again” button its
  text referred to.

## [1.0.0-14] — 2026-10-02

### Added

- If dsh is not installed, the window shows the install command with a
  “Copy command” button and, when npm works without sudo, an “Install”
  button with a live log; “Check again” continues startup.

## [1.0.0-12] — 2026-10-02

### Added

- Browser-like keys: `F5` / `Ctrl+R` — reload, `Ctrl+Shift+R` — reload
  bypassing the cache, `Ctrl+=` / `Ctrl+-` / `Ctrl+0` — zoom (kept between
  launches). They work with any keyboard layout.

## [1.0.0-10] — 2026-10-02

### Fixed

- Wayland: with a real GPU the window uses native Wayland, without one —
  X11 (XWayland); if the window has not appeared within 8 s, it is shown
  forcibly.

## [1.0.0-9] — 2026-10-02

- First release: the window starts `dsh --profile web`, picks up the token
  and opens the web UI, and stops the dsh it started when closed. `.deb`
  package for Ubuntu 26.04.
