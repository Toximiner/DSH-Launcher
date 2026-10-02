# Changelog

[Русский](CHANGELOG.md) | **English**

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
The `## [X.Y.Z]` section becomes the description of the `vX.Y.Z` GitHub release.

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
