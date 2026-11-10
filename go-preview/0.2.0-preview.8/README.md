# Go TUI 0.2.0-preview.8

Linux binaries: `dsh-launcher-tui-go_linux_amd64` and `dsh-launcher-tui-go_linux_arm64`.
Verify the downloaded file with SHA256SUMS, make it executable, and run it in a terminal.

F10 opens File/View/Help. File → TUI settings, F4, or Alt+T opens client preferences.
Mouse and input-history preferences apply immediately and persist between runs.
Use F4 to restore mouse control after disabling it. `--no-mouse` overrides saved
mouse settings for that run. Dialogs preserve the page, scroll, draft, and focus.
About shows client information. dsh settings remain separate and read-only.

Preferences: `${XDG_CONFIG_HOME:-~/.config}/dsh-launcher-tui/preferences.json`.
The history preference controls recall; chat history is not stored in this file.

Supports sessions, backend-confirmed agent modes, and marketplace operations.
Ordinary text chat remains local. Starting dsh requires Node.js 24.9+ and an
installed dsh; attaching this Go client does not require Node.js locally.
Minimum terminal: 44 columns × 18 rows.

Source and limitations:
https://github.com/Toximiner/DSH-Launcher/blob/feature/terminal-ui/apps/tui-go/README.md
