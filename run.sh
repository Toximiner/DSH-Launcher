#!/bin/sh
# Launches the DeepSeek Harness window (Electron).
DIR="$(cd "$(dirname "$0")" && pwd)"
ELECTRON="$DIR/node_modules/electron/dist/electron"
if [ ! -x "$ELECTRON" ]; then
  echo "electron не найден: выполните 'npm install' в $DIR" >&2
  exit 1
fi
# --no-sandbox: on this system unprivileged user namespaces are restricted
# (Ubuntu policy), so the Chromium SUID helper cannot be set up without sudo.
# To enable the Chromium sandbox:
#   sudo chown root:root "$DIR/node_modules/electron/dist/chrome-sandbox"
#   sudo chmod 4755 "$DIR/node_modules/electron/dist/chrome-sandbox"
# and then remove --no-sandbox from this line.
exec "$ELECTRON" "$DIR" --no-sandbox "$@"
