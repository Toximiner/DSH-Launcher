#!/bin/sh
# Запускает окно DeepSeek Harness (Electron).
DIR="$(cd "$(dirname "$0")" && pwd)"
ELECTRON="$DIR/node_modules/electron/dist/electron"
if [ ! -x "$ELECTRON" ]; then
  echo "electron не найден: выполните 'npm install' в $DIR" >&2
  exit 1
fi
# --no-sandbox: на этой системе непривилегированные user-namespace ограничены
# (политика Ubuntu), поэтому SUID-хелпер Chromium не может быть настроен без sudo.
# Если хотите включить sandbox Chromium:
#   sudo chown root:root "$DIR/node_modules/electron/dist/chrome-sandbox"
#   sudo chmod 4755 "$DIR/node_modules/electron/dist/chrome-sandbox"
# и тогда уберите --no-sandbox из этой строки.
exec "$ELECTRON" "$DIR" --no-sandbox "$@"
