# Go TUI 0.2.0-preview.13

Linux amd64/arm64 binaries. Verify SHA256SUMS, make the file executable, and run it.

Attach using DSH_TUI_URL with --new or --session ID. With a ready session, Enter
sends text and dsh streams assistant output. F8, the Stop button, or /stop requests
cancellation. Read subscriptions reconnect automatically; prompt submission never
retries automatically. /retry-send explicitly reuses an uncertain request's ID.

Tool cards show execution state, arguments, results/errors, and cancellation. F6
selects cards, Enter/click expands details, and Esc returns to the composer.
Actual dsh file-read success and missing-file error are covered by integration tests.

Permission dialogs show the tool, reason, and available arguments. Select with
Left/Right or Tab and press Enter, or click Allow/Reject. Grants are one-shot.
Esc defers; F7 reopens; F8 cancels. Pending requests replay after reconnecting.
Uncertain answers do not retry automatically. Actual dsh grants, rejections,
cancellation, and pending-request recovery are tested in an isolated profile.

Agent questions support single/multiple choices, multiline custom text, explicit
skip, and question batches. Tab focuses options/the editor; Enter selects an
option or adds a line in the editor. F5/Answer submits the complete batch.
Timed waits use the interaction mux; continued questions reload from session
projections. Esc defers, F7 reopens, F8 cancels. No answer uses session/prompt,
and uncertain replies never retry automatically.

While the agent runs, Enter submits the next task to the dsh queue. The chat
shows server-confirmed pending tasks and sending/accepted receipts. Admitted
items disappear from the queue without duplicating their durable chat record.
Reconnecting restores pending tasks without resending them. F8 cancels the current
turn and retains the queue. Queue cancellation/editing and steering are not yet
exposed. Actual dsh tests verify two waiting tasks and three individual FIFO turns.

Recent history is bounded to a 50-message window. Rich Markdown, attachments, and full history navigation remain future work.
Offline input and the local WASM adapter remain local drafts.

F10 opens File/View/Help; F4 or Alt+T opens persisted mouse/history preferences.
Minimum terminal: 44 columns by 18 rows. Local Go client needs no Node.js when
attaching; locally starting dsh needs Node.js 24.9+ and installed dsh.

Verified with actual dsh 0.2.2-rc.1 and a controlled local model endpoint. Actual
cloud model responses require working provider configuration in dsh; this package
contains no provider credentials or simulated backend.

Source and limitations:
https://github.com/Toximiner/DSH-Launcher/blob/feature/terminal-ui/apps/tui-go/README.md
