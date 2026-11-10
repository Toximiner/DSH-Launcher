#!/bin/bash
# Run installed-package scenarios on clean distributions. CI owns Docker runs;
# host-side pytest can validate the harness against an extracted package.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
WORK=$HERE/.work
NODE_VER=24.22.1
OSES=${OSES:-"24.04 26.04 debian-13"}
mkdir -p "$WORK/context" "$WORK/results"

(cd "$ROOT" && npm run build:tui:deb)
VER=$(cd "$ROOT" && node -p 'require("./apps/tui/package.json").version')
cp "$ROOT/build/tui/dsh-launcher-tui_${VER}-1_all.deb" "$WORK/context/launcher.deb"
TARBALL="node-v$NODE_VER-linux-x64.tar.xz"
if [ ! -f "$WORK/context/$TARBALL" ]; then
  curl -fSL --retry 3 "https://nodejs.org/dist/v$NODE_VER/$TARBALL" -o "$WORK/context/$TARBALL.part"
  mv "$WORK/context/$TARBALL.part" "$WORK/context/$TARBALL"
fi
# Verify cached archives too, not only fresh downloads.
curl -fsSL --retry 3 "https://nodejs.org/dist/v$NODE_VER/SHASUMS256.txt" -o "$WORK/context/SHASUMS256.txt"
if ! (cd "$WORK/context" && awk -v file="$TARBALL" '$2 == file { print; found=1 } END { if (!found) exit 1 }' SHASUMS256.txt | sha256sum --check --strict -); then
  rm -f "$WORK/context/$TARBALL"
  exit 1
fi
cp "$HERE/Dockerfile" "$WORK/context/Dockerfile"
rm -rf "$WORK/context/tests"
mkdir -p "$WORK/context/tests"
cp -a "$HERE/conftest.py" "$HERE/terminal.py" "$HERE/test_installed.py" "$HERE/fakes" "$WORK/context/tests/"
find "$WORK/context/tests" -name __pycache__ -type d -prune -exec rm -rf {} +

container_name=""
cleanup() { if [ -n "$container_name" ]; then docker rm -f "$container_name" >/dev/null 2>&1 || true; fi; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
failed=0
for os in $OSES; do
  case "$os" in
    24.04|26.04) base="ubuntu:$os" ;;
    debian-13) base="debian:13" ;;
    *) echo "Unsupported OS: $os" >&2; exit 1 ;;
  esac
  results="$WORK/results/$os"
  rm -rf "$results"
  mkdir -p "$results"
  # These scratch artifacts are written by the container's unprivileged user.
  chmod 777 "$results"
  echo ">> building TUI tests for $os"
  if ! docker build --build-arg "BASE=$base" --build-arg "NODE_VER=$NODE_VER" \
      -t "dsh-tui-tests:$os" "$WORK/context" >"$results/build.log" 2>&1; then
    cat "$results/build.log"
    failed=1
    continue
  fi
  container_name="dsh-tui-tests-${os//./}-$$"
  # No display server, elevated capabilities, or runtime network access.
  if timeout -k 15 240 docker run --rm --init --name "$container_name" \
      --network none --cap-drop ALL --security-opt no-new-privileges \
      --mount "type=bind,src=$results,dst=/results" "dsh-tui-tests:$os" \
      >"$results/tests.log" 2>&1; then
    echo "PASS $os"
  else
    echo "FAIL $os"
    failed=1
  fi
  cat "$results/tests.log"
  cleanup
  container_name=""
done
exit "$failed"
