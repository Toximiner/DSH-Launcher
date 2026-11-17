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
# Use Electron's bundled Node: no system Node installation is required.
OZONE_PLATFORM="$(ELECTRON_RUN_AS_NODE=1 "$ELECTRON" "$DIR/src/ozone-startup.js" "$@")" || exit $?
unset DSH_LAUNCHER_AUTO_OZONE
if [ -n "$OZONE_PLATFORM" ]; then
  export DSH_LAUNCHER_AUTO_OZONE="$OZONE_PLATFORM"
  set -- "--ozone-platform=$OZONE_PLATFORM" "$@"
fi
exec "$ELECTRON" "$DIR" --no-sandbox "$@"
