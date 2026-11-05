#!/bin/sh
# Builds the .deb (amd64) without root: dpkg-deb --root-owner-group. The
# package installs on Ubuntu 24.04 / 26.04 and Debian 13 (Docker test matrix,
# .github/). Result: dsh-launcher_<version>_<rev>_amd64.deb next to the script.
set -eu
cd "$(dirname "$0")"

NAME=dsh-launcher
ARCH=amd64
STAGE=build/deb

# Package version — the single source: deb/control, format X.Y.Z-R.
FULL=$(sed -n 's/^Version: *//p' deb/control)
echo "$FULL" | grep -Eq '^[0-9]+(\.[0-9]+)+-[0-9]+$' || {
  echo "В deb/control 'Version:' должен быть в формате X.Y.Z-R (напр. 1.0.0-10), сейчас: '${FULL:-<пусто>}'" >&2;
  exit 1; }
VER=${FULL%-*}
REV=${FULL##*-}
OUT="${NAME}_${VER}-${REV}_${ARCH}.deb"

[ -x node_modules/electron/dist/electron ] || {
  echo "Нет бинарника Electron: npm install && npx install-electron" >&2; exit 1; }

rm -rf build
mkdir -p \
  "$STAGE/DEBIAN" \
  "$STAGE/opt/$NAME" \
  "$STAGE/usr/bin" \
  "$STAGE/usr/share/applications" \
  "$STAGE/usr/share/icons/hicolor/256x256/apps"

# --- application ---
cp main.js package.json icon.png "$STAGE/opt/$NAME/"
cp -a src "$STAGE/opt/$NAME/src"
cp deb/run.sh "$STAGE/opt/$NAME/run.sh"
chmod 755 "$STAGE/opt/$NAME/run.sh"
cp -a node_modules/electron/dist "$STAGE/opt/$NAME/electron"

# --- desktop link and icon ---
cp deb/dsh.desktop "$STAGE/usr/share/applications/$NAME.desktop"
cp icon.png "$STAGE/usr/share/icons/hicolor/256x256/apps/$NAME.png"
ln -sf /opt/$NAME/run.sh "$STAGE/usr/bin/$NAME"

# --- metadata ---
cp deb/control "$STAGE/DEBIAN/control"
cp deb/postinst "$STAGE/DEBIAN/postinst"
chmod 755 "$STAGE/DEBIAN/postinst"

# --- permissions ---
# Files created in the sandbox have mode 600 — in the package (root:root)
# they would be unreadable for the user. Make all non-executable files
# readable by everyone (executables already have the required mode).
find "$STAGE/opt" "$STAGE/usr" -type f ! -perm -u+x -exec chmod 644 {} +

INSTALLED_SIZE=$(du -sk --exclude=DEBIAN "$STAGE" | cut -f1)
sed -i "s/^Installed-Size:.*/Installed-Size: $INSTALLED_SIZE/" "$STAGE/DEBIAN/control"

echo "Собираю (xz, это может занять пару минут)..."
dpkg-deb --build --root-owner-group -Zxz "$STAGE" "$OUT"
echo "Готово: $OUT ($(du -h "$OUT" | cut -f1))"
