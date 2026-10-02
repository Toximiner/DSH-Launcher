#!/bin/sh
# Установленный запускатель DeepSeek Harness (часть пакета dsh-launcher).
# $0 может быть ссылкой (/usr/bin/dsh-launcher -> /opt/dsh-launcher/run.sh),
# поэтому разрешаем цепочку до реального пути.
SELF="$(readlink -f "$0")"
DIR="$(cd "$(dirname "$SELF")" && pwd)"
ELECTRON="$DIR/electron/electron"
SB="$DIR/electron/chrome-sandbox"

# Sandbox Chromium включается, если SUID-хелпер настроен (postinst пакета
# делает chown root:root + chmod 4755 при установке через dpkg/apt от root).
# Иначе — работаем с --no-sandbox.
if [ "$(stat -c '%U:%a' "$SB" 2>/dev/null)" = "root:4755" ]; then
  SANDBOX_ARGS=""
else
  SANDBOX_ARGS="--no-sandbox"
fi

LOGDIR="${DSH_LOG_DIR:-$HOME/.local/state/dsh-launcher}"
mkdir -p "$LOGDIR" 2>/dev/null || true
export DSH_LOG_DIR="$LOGDIR"

exec "$ELECTRON" "$DIR" $SANDBOX_ARGS "$@"
