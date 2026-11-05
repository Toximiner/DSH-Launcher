#!/bin/sh
# Installed DeepSeek Harness launcher (part of the dsh-launcher package).
# $0 may be a symlink (/usr/bin/dsh-launcher -> /opt/dsh-launcher/run.sh),
# so resolve the chain to the real path.
SELF="$(readlink -f "$0")"
DIR="$(cd "$(dirname "$SELF")" && pwd)"
ELECTRON="$DIR/electron/electron"
SB="$DIR/electron/chrome-sandbox"

# Chromium sandbox — only if it was enabled in «Настройки → Графика»
# ("sandbox": true in settings.json) and the SUID helper is configured
# (package postinst: chown root:root + chmod 4755 on a root install).
# Default — without it: on some desktops (Electron 44, Ubuntu 26.04) the
# launcher crashes with the sandbox on when a child process finishes and is
# restarted («Перезапустить dsh», restart after a crash). The window shows
# only the local dsh GUI and its own pages; external links open in a browser.
SETTINGS="${XDG_CONFIG_HOME:-$HOME/.config}/dsh-launcher/settings.json"
if [ "$(stat -c '%U:%a' "$SB" 2>/dev/null)" = "root:4755" ] && grep -q '"sandbox": *true' "$SETTINGS" 2>/dev/null; then
  SANDBOX_ARGS=""
else
  SANDBOX_ARGS="--no-sandbox"
fi

LOGDIR="${DSH_LOG_DIR:-$HOME/.local/state/dsh-launcher}"
mkdir -p "$LOGDIR" 2>/dev/null || true
export DSH_LOG_DIR="$LOGDIR"

exec "$ELECTRON" "$DIR" $SANDBOX_ARGS "$@"
