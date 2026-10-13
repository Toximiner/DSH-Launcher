<p align="center"><img src="icon.png" width="128" alt="DeepSeek Harness Launcher"></p>

# DeepSeek Harness Launcher

[Русский](README.md) | **English**

A small Electron app: a window that opens **DeepSeek Harness**
(`dsh web`) and manages its process along the way.

## How it works

- **Clicking the “DeepSeek Harness” shortcut**:
  - if nothing is listening on port `3080`, the launcher starts
    `dsh --profile web --no-open` itself, picks up the
    `dsh web: http://127.0.0.1:3080/?token=…` line from dsh's stdout and
    opens it in the window (the server issues a session cookie, which is
    stored in the app profile and is usually not needed again);
  - if dsh is **already running** (e.g. from a terminal), the window
    attaches to it. A still-valid cookie is picked up automatically;
    otherwise the window asks you to paste the token URL from the terminal
    where dsh is running.
- **Marketplace plugin.** Before starting dsh, the launcher checks that the
  `dshmarket` plugin is installed in the profile
  (`$DSH_HOME/profiles/<profile>/package.json`, `~/.dsh` by default). If it
  is not, the window asks: “Install” (runs
  `dsh plugin --profile web add dshmarket` with a live log), “Not now” or
  “Don’t ask again”. The permanent opt-out is stored in
  `~/.config/dsh-launcher/market-prompt.json` — delete the file to get the
  question back. No question is asked when attaching to an already running dsh.
- **Closing the window** stops the whole dsh process tree (SIGTERM,
  SIGKILL after 4 s): both the one the launcher started and the one it
  “adopted” after a dsh self-restart (found by the listening port). A dsh
  started by someone else (manually in a terminal before the launcher was
  opened) is left alone. If the launcher itself receives
  SIGTERM/SIGINT/SIGHUP (pkill, session end), it quits gracefully and stops
  dsh the same way (a second signal or a hang longer than 8 s — SIGKILL at
  once); a direct `kill -9` to the launcher cannot be caught, and in that
  case dsh survives it.
- **Update checks.** At startup the launcher asks GitHub for its latest
  release (`Toximiner/DSH-Launcher`) and npm for the latest
  `@deepseek-ai/dsh`. If a newer version exists and dsh is started by the
  launcher itself, before starting dsh a window asks: “Update / Not now /
  Don’t ask again” (remembered per version — a newer release asks again). The
  launcher update prompt includes “What’s new” — the changes of every
  version newer than the installed one; after the update, the first start
  of the new version shows them once more. The release list is remembered
  with its ETag: at startup GitHub is asked “has it changed” — a 304 reply
  doesn’t use up the request limit, and a new release is seen at once; if
  GitHub is unreachable, the cached list is used when under an hour old.
  - Launcher update: downloads the release .deb (progress on screen, sha256
    checked against the asset digest from GitHub) and installs it via
    pkexec — polkit asks for the password in a system dialog; as root the
    package is copied to a root-owned directory and its sha256 re-checked,
    then `apt-get install`; on success the launcher restarts itself. No pkexec /
    cancelled password / failure — a page with the terminal command and a
    “Check again” button.
  - dsh update: `npm install -g @deepseek-ai/dsh@<version>` when the global
    npm prefix is writable without sudo, otherwise the terminal command.
    After success dsh starts with the new version immediately.
  - In attach mode (dsh already running) no prompts are shown.
  - Requests are sent right at startup, in parallel with everything else;
    before starting dsh the launcher waits for them at most ~8 s (without a
    network they fail at once and the launcher simply doesn’t ask). Disable with
    `DSH_LAUNCHER_NO_UPDATE_CHECK=1`. Requests go through the system proxy
    (GNOME/KDE settings or `HTTPS_PROXY`) when one is set.
  - While the launcher runs, the check repeats every 12 hours. A new version
    found mid-work interrupts nothing: a notice in the window corner (once per
    version) and a “Help → Update … to X…” item. Its window shows “What’s
    new” and “Update now”: the launcher restarts (dsh stops) and installs
    the update right away, without asking again. The same buttons are in
    “Help → Check for updates…”. Turned off with its own checkbox in
    “Settings”.

- **Window menu** (always visible; labels follow the window language, RU|EN):
  - “File” — “Settings…” (Ctrl+,; see below), “Restart dsh…” (e.g. after
    plugin updates; asks first; a dsh not started by the launcher is left
    alone) and “Quit” — standard shutdown (stops the dsh the launcher
    started).
  - “Edit” — “Cut”, “Copy”, “Paste”, “Select all” (same as the context menu;
    Ctrl+X / C / V / A work without the menu too).
  - “View” — reload (F5, Ctrl+Shift+R — ignoring cache), zoom (Ctrl+= /
    Ctrl+− / Ctrl+0, remembered), find in page, spell check (on/off), full
    screen (F11).
  - “Help”:
    - “Check for updates…” — the latest release on GitHub and the latest
      `@deepseek-ai/dsh` on npm, per component; when there is a newer one —
      “What’s new” and an “Update” button;
    - “Roll back to a previous version…” — if a new version doesn’t suit
      you: a list of past releases (up to five) and a “Roll back” button; the
      launcher restarts, installs the chosen version the same way as an
      update and no longer offers the newer one (undo — “Settings →
      Updates”);
    - “dsh log…” — the latest lines of `dsh.log`, tokens and keys hidden;
    - “Open logs folder”, “Save report…” (see “Logs and diagnostics”);
    - “About” — the launcher and dsh backend versions, the dsh path,
      Node.js, OS, GPU acceleration, the logs path; one click copies the
      whole block to the clipboard for a bug report.

  “About”, “Check for updates”, “dsh log”, “Settings” and other dialogs open
  inside the launcher window — a card over the dimmed dsh GUI (which isn't
  reloaded). Close with “Close” or Esc.

- **Settings window** (“File → Settings…”, Ctrl+,) — sections in the menu on
  the left:
  - general — interface language (Auto / Русский / English), spell check;
  - updates — whether to check at startup and every 12 hours (two
    checkboxes), “ask again” about postponed versions;
  - starting dsh — the dsh program (automatic or a chosen file), profile,
    working folder, port (the launcher passes it to dsh: `--port`);
  - window — “The close button hides the window to the tray” (dsh keeps
    running; the icon menu: “Open”, “Restart dsh”, “Quit”; needs a system
    tray — in GNOME the AppIndicator extension, included in Ubuntu; starting
    the launcher again also brings the hidden window back), “Tell me when the
    agent finishes or is waiting for you” — a command to approve, a
    question, a plan to review (window not focused — a desktop notification with the
    session title, a dot on the tray icon, under X11 the taskbar button
    flashes; “…and with a sound” is a separate checkbox; only the session
    open in the window is seen);
  - graphics — GPU acceleration (Auto / Off, with the current state), window
    on Wayland (Auto / Wayland / X11 — if the window is invisible), the
    Chromium sandbox (off by default, see “About `--no-sandbox`”).

  Language, spell check, the 12-hour check, “ask again”, the tray and the
  “agent finished” signal apply at once; the rest — after the launcher
  restarts (a notice with a “Restart the launcher” button at the top of the
  window). “Saved” and errors pop up in the window corner. Stored in
  `~/.config/dsh-launcher/settings.json`. Environment variables (`DSH_BIN`,
  `DSH_ARGS`, `DSH_CWD`, `DSH_PORT`, `DSH_LAUNCHER_LANG`,
  `DSH_LAUNCHER_NO_UPDATE_CHECK`, `DSH_LAUNCHER_NO_GPU`,
  `DSH_LAUNCHER_OZONE`) take precedence: such a field is disabled and
  labelled.

- **Find in page (Ctrl+F)** — a bar in the top-right corner with a match
  counter; Enter / Shift+Enter (F3 / Shift+F3) — next / previous, Esc —
  close.

- **Spell check** in text fields (Russian and English): suggestions and
  “Add to dictionary” on right-click over an underlined word. Chromium
  downloads the dictionaries the first time it is on (offline it does not
  work).

- **The window size is remembered** between launches.

- **Logs and diagnostics:** “Help → Open logs folder”; “About” shows the
  logs path and the GPU acceleration state. If the GPU was turned off after
  GPU process crashes, a “Re-enable GPU” button is there too.
  - “Help → Save report…” (also a button in “About”) — an archive
    `dsh-launcher-report-<date>.tar.gz`: versions, system, session, GPU,
    settings, the launcher log and the tail of the dsh log; tokens and keys
    hidden. If the launcher crashed — also crash dumps (up to three from the
    last 30 days): Electron keeps them locally only, in
    `~/.config/dsh-launcher/Crashpad`, and never sends them anywhere. Attach it to an issue on GitHub.
  - If the launcher closed unexpectedly last time (a crash, `kill -9`, a
    power loss), the next start shows a notice in the window corner with
    “Log…” and “Save report…” buttons.

- **Right-click context menu** — both in the dsh GUI and on the launcher's
  own pages: in a text field “Cut”, “Copy”, “Paste”, “Select all”; on
  selected text — “Copy”; on a link — “Copy link address”. Labels follow the
  window language.
- **Protection from foreign pages in the window.** The dsh GUI shows model
  output and plugin UIs, so:
  - launcher commands (`dshlauncher://…`: settings, updates, restart) are
    accepted only from the launcher's own pages — their links carry a random
    key, new on every launch; a request without the key is rejected (a line
    in the log);
  - a link to another site opens in the system browser, not in the window;
  - browser permissions — only clipboard writes and fullscreen; microphone,
    camera, geolocation, notifications are denied (the log says which
    permission and for which page).

## System requirements

Tested on **Ubuntu 26.04** (desktop, Wayland) and on clean **Ubuntu 26.04**,
**Ubuntu 24.04** and **Debian 13** (amd64) in a container: the package
installs via `apt` with all dependencies, and the dsh installation scenarios
go all the way to the UI. The package also installs on systems with the old
library names (without the `t64` suffix) — Debian 12, Ubuntu 22.04; those
are not tested regularly.

The launcher itself does not need Node.js — Electron is bundled in the
package. But **DeepSeek Harness** is an npm package, and it needs:

- **Node.js 22 or newer** and npm;
- **pnpm** — to install plugins (including the marketplace).

The launcher window tells you what is missing. How to get Node.js:

| System | Node.js from apt | What to do |
|---|---|---|
| Ubuntu 26.04 | 22 — fine | `sudo apt install nodejs npm`, then `sudo npm install -g @deepseek-ai/dsh pnpm` (system npm installs into `/usr/local`, needs sudo) |
| Ubuntu 24.04 | 18 — **too old**, dsh will not run on it | Node.js 22+ via nvm (below) or NodeSource packages |
| any | — | **nvm**, no sudo — then the launcher installs dsh, pnpm and the marketplace itself with the “Install” button |

Installing nvm and Node.js:

```sh
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh | bash
# open a new terminal
nvm install 22
```

## Running from a terminal

From the project directory:

```sh
npm install          # first time only
npx install-electron # Electron 44+ does not download its binary automatically
npm start
```

## Configuration

Defaults are in `src/env.js` and `src/config.js` (and the Settings window), or environment variables:

| Variable                | Default                   | What it does                  |
|-------------------------|---------------------------|-------------------------------|
| `DSH_BIN`               | auto-detected (see below) | path to the dsh binary        |
| `DSH_ARGS`              | `--profile web --no-open` | launch arguments (shell-like: spaces, `"…"`, `'…'`, `\`) |
| `DSH_CWD`               | home directory            | dsh working directory         |
| `DSH_PORT`              | `3080`                    | web UI port                   |
| `DSH_START_TIMEOUT_MS`  | `120000`                  | how long to wait for startup, ms |
| `DSH_LAUNCHER_LANG`     | in-window choice / locale | launcher window language: `ru` or `en` |
| `DSH_LAUNCHER_NO_UPDATE_CHECK` | —             | `1` — skip version checks (GitHub Releases + npm) |
| `DSH_LAUNCHER_NO_GPU`   | —                         | `1` — no GPU acceleration (software rendering) |
| `DSH_LAUNCHER_OZONE`    | auto                      | window on Wayland: `wayland` or `x11` (XWayland) |

The language of the launcher's windows (startup, dsh installation,
marketplace prompt, updates, errors, dialogs) is chosen in Settings or with
the **RU|EN** switch in the top-right corner of the launcher's pages; the
choice is saved to
`~/.config/dsh-launcher/ui-lang.json`. Until a choice is made, the system
locale is used (`LC_ALL` / `LC_MESSAGES` / `LANG`): `ru*` means Russian,
anything else English. `DSH_LAUNCHER_LANG=ru|en` forces the language and
overrides the saved choice.

If `DSH_BIN` is not set, the launcher looks for dsh itself: `/usr/bin/dsh`,
`PATH`, `~/.npm-global/bin`, `~/.local/bin`, `/usr/local/bin`, every nvm
version (`~/.nvm/versions/node/*/bin`) and `$(npm prefix -g)/bin`. The path
found is remembered in `~/.config/dsh-launcher/dsh-bin.json`. dsh is started
with its own directory first in `PATH`, so a dsh installed via nvm gets its
own node even when launched from the desktop shortcut.

The dsh process log is written to `dsh.log`, the launcher’s own log (update
checks, starting and stopping dsh, errors; timestamped, tokens hidden) to
`launcher.log`: in the installed version — in `~/.local/state/dsh-launcher/`,
when run from source — in the project’s `logs/`. Both are moved to `*.1` after 2 MB (at
startup and while running), replacing the previous one: at most four log
files, ~8 MB, on disk.

## Keys in the window

They work with any keyboard layout (RU/EN); the same actions are in the
window menu:

| Keys                | Action                      |
|---------------------|-----------------------------|
| `F5` / `Ctrl+R`     | reload the page             |
| `Ctrl+Shift+R`      | reload bypassing the cache  |
| `Ctrl+=` / `Ctrl+-` | zoom in / out one step      |
| `Ctrl+0`            | reset zoom                  |
| `Ctrl+F`            | find in page                |
| `F3` / `Shift+F3`   | next / previous match       |
| `F11`               | full screen                 |
| `Ctrl+,`            | Settings                    |
| `Esc`               | close a dialog or the find bar |

The zoom level is kept between launches. A lone `Alt` press doesn’t move
focus to the menu bar, so it can be used freely in dsh GUI shortcuts.

## Shortcut (if not installed yet)

There is **no** ready-made desktop file for a dev shortcut in the repository
(`deb/dsh.desktop` is the template for the .deb package): the file is
created locally because it contains absolute paths of your machine:

```sh
# from the project directory:
chmod +x run.sh
cat > dsh.desktop <<EOF
[Desktop Entry]
Type=Application
Version=1.0
Name=DeepSeek Harness
GenericName=AI Chat
Comment=Launcher for DeepSeek Harness (dsh --profile web)
Exec=$PWD/run.sh
Icon=$PWD/icon.png
Terminal=false
Categories=Development;Utility;
Keywords=dsh;deepseek;ai;chat;harness;
StartupWMClass=dsh-launcher
EOF
cp dsh.desktop ~/.local/share/applications/      # application menu entry
cp dsh.desktop ~/Desktop/ && chmod +x ~/Desktop/dsh.desktop   # desktop icon
# GNOME: right-click the desktop icon → “Allow Launching”
```

## Package for Ubuntu and Debian (.deb)

Ready-made packages are published in the repository's GitHub **Releases** —
the easiest way is to download the latest `.deb` from there and install it.
What changed in each version is in [CHANGELOG.en.md](CHANGELOG.en.md).

### Building it yourself

Required (on the **build machine** only): Ubuntu/Debian amd64 with `dpkg-deb`
(the `dpkg-dev` package), Node.js 22.12+ (required by Electron 44) and
internet access (the ~290 MB Electron binary is downloaded). The package
itself does **not** need node/npm on the target machine — Electron is
bundled (Node.js 22+ is needed only for dsh, see “System requirements”).

```sh
git clone https://github.com/Toximiner/DSH-Launcher.git
cd DSH-Launcher
npm install            # build machine only
npx install-electron   # Electron 44+ does not download its binary automatically
./build-deb.sh         # ~2 minutes (xz compression) → dsh-launcher_<version>-<revision>_amd64.deb

sudo dpkg -i dsh-launcher_*_amd64.deb
# or: sudo apt install ./dsh-launcher_*_amd64.deb
```

The package version is set in **one place** — `Version:` in `deb/control`
(format `X.Y.Z-R`, e.g. `1.0.0-9`); `build-deb.sh` reads it automatically.
To build a new revision locally, change only that file.

Installed files:

- `/opt/dsh-launcher/` — the app and the Electron runtime;
- `/usr/bin/dsh-launcher` — launch command;
- `/usr/share/applications/dsh-launcher.desktop` — the “DeepSeek Harness” menu entry;
- `/usr/share/icons/hicolor/256x256/apps/dsh-launcher.png` — icon.

`postinst` (run as root during installation) sets up Chromium's SUID sandbox
helper (`chown root:root` + `chmod 4755 chrome-sandbox`). The sandbox itself
is **off** by default (see “About `--no-sandbox`”) and is turned on with a
checkbox in “Settings → Graphics”.

Logs in the installed version: `~/.local/state/dsh-launcher/dsh.log` (dsh)
and `launcher.log` (the launcher) — “Help → Open logs folder”.

> **Note.** If you previously installed a shortcut manually
> (`~/.local/share/applications/dsh-launcher.desktop` and/or a desktop
> icon), it overrides the system one (user files take precedence). Remove
> it after installing the package:
>
> ```sh
> rm ~/.local/share/applications/dsh-launcher.desktop
> rm ~/Desktop/dsh-launcher.desktop 2>/dev/null
> ```

### Code layout

`main.js` only assembles the app: the single instance, the startup order
(`onReady`), the `dshlauncher://` routes, restoring the window after dsh
restarts, shutdown. Everything else is in `src/` modules:

| Modules | What they hold |
|---|---|
| `env.js`, `config.js`, `log.js` | startup parameters (environment and `settings.json`), logs and their rotation |
| `state.js`, `launcher.js`, `i18n.js` | shared state (window, dsh process, flags), the function registry for calls between modules, the window language |
| `dsh-install.js`, `dsh-process.js`, `dsh-stop.js`, `launch.js` | finding dsh, starting it and its self-restart, stopping the process tree, bringing the window to the GUI |
| `pages.js`, `install-page.js`, `market.js` | the launcher's pages, “dsh is not installed”, the marketplace plugin |
| `updates.js`, `updates-live.js`, `whats-new.js`, `updates-logic.js`, `versions.js`, `notes.js` | updates at startup and while running, “What’s new”, versions, release notes |
| `dialogs.js`, `settings-window.js`, `dsh-log.js`, `diagnostics.js` | dialog windows, Settings, the dsh log, the report and the crash notice |
| `menus.js`, `view.js`, `file-help.js`, `window-state.js`, `tray.js` | menus, “View” and find, “File/Help”, window size, tray |
| `util.js`, `system.js`, `ui.js` | pure functions: argument parsing, route key, processes and ports, menu items, shortcuts |
| `guard.js`, `proc-log.js`, `prefs.js` | window protection (navigation, permissions), commands with a live log, settings files |

The pure functions are covered by unit tests (`test/*.js`). Functions from
modules that need electron are taken by the tests from the source text by
marker blocks `/* ===== name (тест test/X.js) */` … `/* ===== end name ===== */`;
`test/markers.js` checks that each block has its pair, the name is unique and
the named test exists and checks that function. Modules require
each other as usual and call `pages.showPage()` and the like; shared state
is `state.<field>`. Stateful modules extend their `module.exports` instead of
replacing it, so cycles between them don't get in the way. The few things
that live in `main.js` reach modules through the `launcher.<name>` registry.

### Tests

```sh
npm test            # static check (ESLint) and unit tests of the src/ modules (test/*.js)
npm run coverage    # unit test coverage (also in CI — the build's Summary tab)
npm run lint        # ESLint only: unknown names, unused imports and the like
test/docker/run.sh  # scenarios of the installed .deb in Docker: Ubuntu 24.04, 26.04, Debian 13
```

The Docker scenarios are written with **pytest** (`test/docker/`):

- `tests/` — the tests by topic (shutdown, launcher and dsh updates, updates
  while running, Settings, menu windows, context menu and find, tray, logs,
  diagnostics, window size and proxy, Wayland); a scenario = a test;
- `helpers/` — window objects (`windows.py`: main window, dialogs, the
  notice, find bar, update prompt), launching the launcher, processes, the
  CDP driver;
- `fakes/` — a fake dsh, HTTP proxy and GitHub (`github.py`: the launcher's
  release list with ETag and the `.deb` files — the update and rollback
  scenarios don't depend on real releases; `run.sh` builds those `.deb` files
  from the current code as versions 2.0.0-1 and 1.0.0-14);
- `conftest.py` — fixtures (`start`, `gui`, `prompt`, `drv`, `wayland`) and
  scenario selection: `pytest --scenario <name>`, `--list-scenarios`.

`run.sh` builds a `.deb` from the current code (as version 1.1.0-1),
installs it into clean containers and runs one scenario per container under
Xvfb (the `wayland_*` scenarios — under a headless weston, on Ubuntu 26.04
only); the window is driven over the Chromium debugging protocol (CDP). Needs
Docker, a downloaded Electron and network access. Only some scenarios:
`test/docker/run.sh close find`, only one system: `OSES=26.04`. Logs and
JUnit reports go to `test/docker/.work/results/`.

In CI `npm test` runs before every package build and on every PR (a failing
test blocks the release), and the Docker scenarios are a separate `docker-tests` workflow,
run manually: Actions → docker-tests → Run workflow. Each system is a
separate job (in parallel, ~7 minutes), image layers are cached; the result
table is in the run's Summary tab. In tests the launcher's GitHub API
requests carry the run's temporary token (`DSH_LAUNCHER_GITHUB_TOKEN` — for
development only; the launcher removes it from its environment and never
prints it) so as not to hit the 60 requests/hour limit. Only the `proxy`
scenario talks to the real GitHub; the rest use the fake one
(`DSH_LAUNCHER_GITHUB_API=http://127.0.0.1:<port>`; the launcher accepts no
other address).

Dependabot (`.github/dependabot.yml`) proposes PRs with new versions of
Electron, ESLint and GitHub actions weekly; such a PR is checked by the unit
tests and the package build, the Docker scenarios are run manually.

Release: fill in the `## [Unreleased]` section of `CHANGELOG.md` and
`CHANGELOG.en.md`, then `npm run release 1.16.0` — the section becomes
`## [1.16.0] — <date>`, the version goes into `package.json`,
`package-lock.json` and `deb/control`, and a “Версия 1.16.0” commit and a
`v1.16.0` tag are created (locally). The script refuses if the section is
empty, the branch isn't `main`, there are uncommitted changes or the version
isn't higher than the current one. To publish — `git push origin main
v1.16.0`: on the tag CI builds the package and publishes the release with
the description from both CHANGELOGs. Only edit the files, no commit or tag:
`npm run release -- 1.16.0 --no-commit`.

## About `--no-sandbox`

The launcher starts Electron with `--no-sandbox`. On some desktops (Electron
44, Ubuntu 26.04) it crashed with the Chromium sandbox when dsh exits and
starts again (“Restart dsh”, a restart after a crash). The window only shows
the local dsh GUI (127.0.0.1) and the launcher's own pages, external links
open in the browser — that is fine for such an interface.

To turn the sandbox on: “Settings → Graphics → Chromium sandbox” (from the
next start; needs the SUID helper — installing the package sets it up). When
running from source:

```sh
# from the project directory:
sudo chown root:root node_modules/electron/dist/chrome-sandbox
sudo chmod 4755 node_modules/electron/dist/chrome-sandbox
```

and remove `--no-sandbox` from the last line of `run.sh`.

## Troubleshooting

- If dsh is not found (or `DSH_BIN` points to a missing file), the
  window offers to install it: the `npm install -g @deepseek-ai/dsh`
  command with a “Copy command” button and, if the global npm prefix is
  writable without sudo, an “Install” button (installs right from the
  window, with a log). “Check again” continues startup once dsh is there.
- If dsh crashes, the launcher restarts it by itself (at most three times
  in 10 minutes) and shows a “dsh was restarted after a crash” notice with a
  “dsh log…” button. If dsh keeps crashing, a page with the log stays.
- If dsh was restarted (e.g. after a plugin update), the window may show
  “session ended”. The launcher watches the port and reloads the page by
  itself once dsh responds again (usually within 2–3 s). If you don't want
  to wait, just press `F5` in the window: the cookie, sessions and UI state
  are stored on disk (`~/.dsh/.credentials.yaml`, `~/.dsh/sessions/`), so
  the conversation is restored without loss.
- The window shows an error page with the latest lines of the dsh log.
- Full logs — “Help → Open logs folder” (`dsh.log` — dsh, `launcher.log` —
  the launcher); for a bug report, “Help → Save report…” is handier — one
  archive, tokens and keys hidden.
- Check that `dsh --profile web --no-open` starts in a terminal and
  prints the URL.
- The cookie lives in the Electron profile (`~/.config/dsh-launcher`); to
  “forget” the authorization, close the window (dsh will stop) and delete
  that directory.
