# Installed TUI Docker scenarios

The `docker` job of `.github/workflows/tui.yml` tests the current `.deb` on
Ubuntu 24.04, Ubuntu 26.04, and Debian 13 (amd64). It runs automatically for
relevant pull requests and pushes to main, and through `workflow_dispatch`.
Each OS has a separate job; a failure on one does not cancel the others.

The runner builds the Debian package, verifies an official Node archive against
`SHASUMS256.txt`, and builds the image. The image installs the package through
apt, including its Node dependency. A test-only Debian wrapper contains the
actual official Node 24.22.1 distribution. Node 24 is needed by the pinned
`@deepseek-ai/dsh@0.2.2-rc.1` (requires 24.9+); the separate TUI unit job still
checks Node 22 compatibility. NodeSource and a desktop environment are not needed.

Tests run as `tester`, with a real PTY, no display variables, no extra Linux
capabilities, and `--network none`. Only the installed `/usr/bin/dsh-launcher-tui`
is executed: the source application and its development node_modules are not
copied into the image. A deterministic Web API peer controls streaming and
decision timing. A separate smoke scenario boots real dsh, creates and lists
sessions, and verifies shutdown. No model provider or API key is used.

The 23 scenarios cover package metadata and help, five exit paths, stopping a
stubborn process group, preserving an attached server, partial output before
settlement, duplicate prevention, session creation and switching, explicit allow
and reject decisions, questions and cancellation, unrelated-session delegation,
disconnects, bracketed paste and resize, invalid login, a missing executable,
and real backend startup. Installer scenarios cover declining, explicit consent,
successful startup and reuse, npm failure and retry, duplicate requests, and
cancellation through commands or signals. npm is replaced by a stand-in for
these cases, so they do not download packages. Normal exits and errors check
terminal restoration.

CI calls `test/docker-tui/run.sh` with `OSES` set to one matrix entry. Artifacts
under `.work/results/<os>/` include the build log, pytest output, JUnit XML,
terminal transcripts, and fake-backend RPC logs. The images need network access
while building (distribution mirrors and npm); test containers have no runtime
network access beyond loopback. Docker startup/build/run failures fail the job.

Follow `CONTRIBUTING.md`: Docker scenario execution belongs in CI. Once the
workflow is available on GitHub, a manual run is:

```sh
gh workflow run tui --ref feature/terminal-ui
```

For host-side development without Docker, install pytest and pyte into a virtual
environment, extract the built `.deb`, and run the same scenarios against it:

```sh
dpkg-deb -x build/tui/dsh-launcher-tui_0.1.0-1_all.deb /tmp/tui-installed
TUI_TEST_BIN=/tmp/tui-installed/usr/lib/dsh-launcher-tui/bin/dsh-launcher-tui.js \
TUI_TEST_WS=/tmp/tui-installed/usr/lib/dsh-launcher-tui/node_modules/ws \
TUI_TEST_REAL_DSH=/absolute/path/to/dsh \
TUI_TEST_RESULTS=/tmp/tui-results \
python -m pytest -p no:cacheprovider -v test/docker-tui
```

Host execution checks the harness and extracted package, not container apt
installation or the OS matrix. Without `TUI_TEST_REAL_DSH`, only the real backend
scenario skips. The Docker image always supplies that variable, so its smoke
scenario cannot silently skip because of missing configuration.
