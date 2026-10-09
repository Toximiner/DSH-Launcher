# Changelog

[Русский](CHANGELOG.md) | **English**

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
The `## [X.Y.Z]` section becomes the description of the `vX.Y.Z` GitHub release.

## [Unreleased]

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
