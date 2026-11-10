# Private live TUI in Codespaces

Create a Codespace on `feature/terminal-ui` using `.devcontainer/devcontainer.json`.
The container installs Node 24, Go, actual dsh 0.2.2-rc.1 and dshmarket 1.73.17,
then builds the native terminal client. Port 7681 serves that executable through
ttyd; this deployment contains no demo API or fixture catalog.

Keep forwarded port 7681 **private**. Open its HTTPS URL while signed into the
GitHub account that owns the Codespace. The browser uses a real PTY and WebSocket;
the touch toolbar and automatic font fitting also support phones.

Installation and service logs live under `.live-data/`. To restart the service,
run `kill "$(cat .live-data/service.pid)"`, then
`python3 tools/live/start.py --daemon`. To stop billing for compute, stop the
Codespace from https://github.com/codespaces; closing the browser alone does not
stop it. Choose a 20-minute idle timeout when creating the Codespace. Stored
Codespaces also consume storage quota until deleted.

The backend, marketplace, and native chat are live. Enter sends to the attached
session; F8 or the touch toolbar Stop button requests cancellation. Tool cards
show arguments, state, and results. F6 or the touch toolbar Tools button selects
cards; Enter/click expands details and Esc restores composer focus. Model replies need your own provider
configuration in dsh; this deployment bundles no provider key. Never commit API keys.

Outside Codespaces, set `DSH_LIVE_PASSWORD` before starting: HTTP Basic auth uses
the username `demo`. Put HTTPS in front of any non-local deployment.
