# Go TUI 0.2.0-preview.9

Linux amd64/arm64 binaries. Verify SHA256SUMS, make the file executable, and run it.

Attach using DSH_TUI_URL with --new or --session ID. With a ready session, Enter
sends text and dsh streams assistant output. F8, the Stop button, or /stop requests
cancellation. Read subscriptions reconnect automatically; prompt submission never
retries automatically. /retry-send explicitly reuses an uncertain request's ID.

Recent history is bounded to a 50-message window. Tool question/approval dialogs,
rich Markdown, attachments, and full history navigation remain future work.
Offline input and the local WASM adapter remain local drafts.

F10 opens File/View/Help; F4 or Alt+T opens persisted mouse/history preferences.
Minimum terminal: 44 columns by 18 rows. Local Go client needs no Node.js when
attaching; locally starting dsh needs Node.js 24.9+ and installed dsh.

Verified with actual dsh 0.2.2-rc.1 and a controlled local model endpoint. Actual
cloud model responses require working provider configuration in dsh; this package
contains no provider credentials or simulated backend.

Source and limitations:
https://github.com/Toximiner/DSH-Launcher/blob/feature/terminal-ui/apps/tui-go/README.md
