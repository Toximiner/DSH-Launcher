#!/bin/sh
# Build the terminal package separately; Electron is not a dependency.
set -eu
cd "$(dirname "$0")"
mkdir -p build/tui
npm pack ./apps/tui --pack-destination build/tui
