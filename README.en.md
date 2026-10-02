# dsh-launcher

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
  SIGKILL after 4 s) — but **only if the launcher started that process**.
  A dsh started by someone else (manually in a terminal) is left alone.

## Running from a terminal

From the project directory:

```sh
npm install          # first time only
npx install-electron # Electron 44+ does not download its binary automatically
npm start
```

## Configuration

Constants at the very top of `main.js`, or environment variables:

| Variable                | Default                   | What it does                  |
|-------------------------|---------------------------|-------------------------------|
| `DSH_BIN`               | `/usr/bin/dsh`            | path to the dsh binary        |
| `DSH_ARGS`              | `--profile web --no-open` | launch arguments (shell-like: spaces, `"…"`, `'…'`, `\`) |
| `DSH_CWD`               | home directory            | dsh working directory         |
| `DSH_PORT`              | `3080`                    | web UI port                   |
| `DSH_START_TIMEOUT_MS`  | `120000`                  | how long to wait for startup, ms |
| `DSH_LAUNCHER_LANG`     | in-window choice / locale | launcher window language: `ru` or `en` |

The language of the launcher's own screens (startup, dsh installation,
marketplace prompt, errors) can be switched right in the window — **RU | EN**
in the top-right corner; the choice is saved to
`~/.config/dsh-launcher/ui-lang.json`. Until a choice is made, the system
locale is used (`LC_ALL` / `LC_MESSAGES` / `LANG`): `ru*` means Russian,
anything else English. `DSH_LAUNCHER_LANG=ru|en` forces the language and
overrides the saved choice.

The dsh process log is written to `logs/dsh.log`.

## Keys in the window

The window menu is disabled, so control is via browser-like shortcuts
(they work with any keyboard layout — RU/EN):

| Keys                | Action                      |
|---------------------|-----------------------------|
| `F5` / `Ctrl+R`     | reload the page             |
| `Ctrl+Shift+R`      | reload bypassing the cache  |
| `Ctrl+=` / `Ctrl+-` | zoom in / out one step      |
| `Ctrl+0`            | reset zoom                  |

The zoom level is kept between launches.

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

## Package for Ubuntu 26.04 (.deb)

Ready-made packages are published in the repository's GitHub **Releases** —
the easiest way is to download the latest `.deb` from there and install it.
What changed in each version is in [CHANGELOG.md](CHANGELOG.md) (in Russian).

### Building it yourself

Required (on the **build machine** only): Ubuntu/Debian amd64 with `dpkg-deb`
(the `dpkg-dev` package), Node.js 22.12+ (required by Electron 44) and
internet access (the ~290 MB Electron binary is downloaded). The target
machine does **not** need node/npm — the package is self-contained and
bundles Electron.

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
helper (`chown root:root` + `chmod 4755 chrome-sandbox`); in that case
`run.sh` starts **without** `--no-sandbox`, and if the helper could not be
set up it automatically falls back to `--no-sandbox`.

dsh log in the installed version: `~/.local/state/dsh-launcher/dsh.log`.

> **Note.** If you previously installed a shortcut manually
> (`~/.local/share/applications/dsh-launcher.desktop` and/or a desktop
> icon), it overrides the system one (user files take precedence). Remove
> it after installing the package:
>
> ```sh
> rm ~/.local/share/applications/dsh-launcher.desktop
> rm ~/Desktop/dsh-launcher.desktop 2>/dev/null
> ```

## About `--no-sandbox`

`run.sh` starts Electron with `--no-sandbox`: on Ubuntu systems where
unprivileged user namespaces are restricted by policy, Chromium's SUID
helper cannot be set up without `sudo`. For a local trusted UI this is
fine. If you want to enable the Chromium sandbox:

```sh
# from the project directory:
sudo chown root:root node_modules/electron/dist/chrome-sandbox
sudo chmod 4755 node_modules/electron/dist/chrome-sandbox
```

and remove `--no-sandbox` from the last line of `run.sh`.

## Troubleshooting

- If dsh is not installed (or `DSH_BIN` points to a missing file), the
  window offers to install it: the `npm install -g @deepseek-ai/dsh`
  command with a “Copy command” button and, if the global npm prefix is
  writable without sudo, an “Install” button (installs right from the
  window, with a log). “Check again” continues startup once dsh is there.
- If dsh was restarted (e.g. after a plugin update), the window may show
  “session ended”. The launcher watches the port and reloads the page by
  itself once dsh responds again (usually within 2–3 s). If you don't want
  to wait, just press `F5` in the window: the cookie, sessions and UI state
  are stored on disk (`~/.dsh/.credentials.yaml`, `~/.dsh/sessions/`), so
  the conversation is restored without loss.
- The window shows an error page with the latest lines of the dsh log.
- Full log: `logs/dsh.log`.
- Check that `dsh --profile web --no-open` starts in a terminal and
  prints the URL.
- The cookie lives in the Electron profile (`~/.config/dsh-launcher`); to
  “forget” the authorization, close the window (dsh will stop) and delete
  that directory.
