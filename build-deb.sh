#!/bin/sh
# Собирает .deb для Ubuntu 26.04 (amd64) без root: dpkg-deb --root-owner-group.
# Результат: dsh-launcher_<версия>_<rev>_amd64.deb рядом со скриптом.
set -eu
cd "$(dirname "$0")"

NAME=dsh-launcher
ARCH=amd64
STAGE=build/deb

# Версия пакета — единственный источник: deb/control, формат X.Y.Z-R.
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

# --- приложение ---
cp main.js package.json "$STAGE/opt/$NAME/"
cp deb/run.sh "$STAGE/opt/$NAME/run.sh"
chmod 755 "$STAGE/opt/$NAME/run.sh"
cp -a node_modules/electron/dist "$STAGE/opt/$NAME/electron"

# --- ярлык и иконка ---
cp deb/dsh.desktop "$STAGE/usr/share/applications/$NAME.desktop"
cp icon.png "$STAGE/usr/share/icons/hicolor/256x256/apps/$NAME.png"
ln -sf /opt/$NAME/run.sh "$STAGE/usr/bin/$NAME"

# --- метаданные ---
cp deb/control "$STAGE/DEBIAN/control"
cp deb/postinst "$STAGE/DEBIAN/postinst"
chmod 755 "$STAGE/DEBIAN/postinst"

# --- права ---
# Файлы, созданные в песочнице, имеют mode 600 — в пакете (root:root)
# они были бы нечитаемы для пользователя. Делаем все не-исполняемые
# файлы читаемыми для всех (исполняемые уже имеют нужный mode).
find "$STAGE/opt" "$STAGE/usr" -type f ! -perm -u+x -exec chmod 644 {} +

INSTALLED_SIZE=$(du -sk --exclude=DEBIAN "$STAGE" | cut -f1)
sed -i "s/^Installed-Size:.*/Installed-Size: $INSTALLED_SIZE/" "$STAGE/DEBIAN/control"

echo "Собираю (xz, это может занять пару минут)..."
dpkg-deb --build --root-owner-group -Zxz "$STAGE" "$OUT"
echo "Готово: $OUT ($(du -h "$OUT" | cut -f1))"
