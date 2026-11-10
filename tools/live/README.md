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

The backend and marketplace are live. Ordinary text in the current TUI chat is
still a local draft/history; streaming agent chat is not implemented yet. Model
operations need your own provider configuration in dsh. Never commit API keys.

Outside Codespaces, set `DSH_LIVE_PASSWORD` before starting: HTTP Basic auth uses
the username `demo`. Put HTTPS in front of any non-local deployment.
