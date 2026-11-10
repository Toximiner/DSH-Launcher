# Static browser preview

This preview runs the native client's shared Go model (`Update`/`View`) in
WebAssembly, using xterm.js for display and browser input. It requires no tunnel,
server terminal, login, or install. The page clearly labels its limits: local
chat drafts and a captured, redacted dsh demo settings snapshot. It does not
connect to a user's backend; refresh and quit commands do not operate a server.
Reloading clears drafts. Browser support requires WebAssembly and DecompressionStream.

Build from any working directory with Python 3 and Go:

```sh
python3 apps/tui-go/web/build.py --go /path/to/go
python3 -m http.server --directory build/tui-go-web 7682
```

The build leaves the native Go module and dependencies unchanged. It uses a
separate generated modfile and copy of Bubble Tea with stubs for native TTY
runtime functions, which are never used by the browser adapter. Clipboard input
comes through browser paste events. Browser assets are pinned and verified
against npm integrity metadata. Publish only the files named in the generated
SHA256SUMS; generated Go sources and modfiles are build inputs, not web assets.
The native terminal app retains its existing backend and terminal behavior.

With Playwright installed, run `python3 apps/tui-go/web/test_preview.py` against
the local server, or pass `--url` to verify a deployment. The smoke test exercises
settings, keyboard and mouse navigation, Unicode-capable input, and resizing.
