#!/bin/sh
# Package the TUI and its installed JS dependency; the system supplies Node.js.
set -eu
cd "$(dirname "$0")"

[ -f apps/tui/node_modules/ws/package.json ] || {
  echo 'Run npm ci --prefix apps/tui before packaging.' >&2
  exit 1
}
VER=$(node -p 'require("./apps/tui/package.json").version')
STAGE=build/tui/deb
OUT="build/tui/dsh-launcher-tui_${VER}-1_all.deb"
rm -rf "$STAGE"
mkdir -p "$STAGE/DEBIAN" "$STAGE/usr/lib/dsh-launcher-tui/node_modules" "$STAGE/usr/bin"
cp -a apps/tui/src apps/tui/bin apps/tui/package.json apps/tui/README.md "$STAGE/usr/lib/dsh-launcher-tui/"
cp -a apps/tui/node_modules/ws "$STAGE/usr/lib/dsh-launcher-tui/node_modules/"
ln -s ../lib/dsh-launcher-tui/bin/dsh-launcher-tui.js "$STAGE/usr/bin/dsh-launcher-tui"
find "$STAGE" -type d -exec chmod 755 {} +
find "$STAGE/usr/lib" -type f -exec chmod 644 {} +
chmod 755 "$STAGE/usr/lib/dsh-launcher-tui/bin/dsh-launcher-tui.js"
SIZE=$(du -sk "$STAGE/usr" | cut -f1)
cat > "$STAGE/DEBIAN/control" <<EOF
Package: dsh-launcher-tui
Version: ${VER}-1
Section: utils
Priority: optional
Architecture: all
Maintainer: Toximiner <Toximiner@users.noreply.github.com>
Installed-Size: $SIZE
Depends: nodejs (>= 22)
Description: Terminal client and launcher for DeepSeek Harness
 Streaming terminal chat with session selection and operation approvals.
 Requires dsh installed separately. Electron is not required.
EOF
chmod 644 "$STAGE/DEBIAN/control"
dpkg-deb --build --root-owner-group "$STAGE" "$OUT"
