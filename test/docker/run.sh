#!/bin/bash
# Сценарии установленного лаунчера в Docker на чистых Ubuntu 24.04 и 26.04:
# закрытие окна и сигналы, «усыновлённый» dsh, обновление dsh (npm) и самого
# лаунчера (настоящий последний релиз с GitHub через pkexec).
#
#   test/docker/run.sh                 — все сценарии на обеих системах
#   test/docker/run.sh close sigterm   — только указанные
#   OSES="26.04" JOBS=2 test/docker/run.sh
#
# Нужны Docker, собранный Electron (npm install && npx install-electron) и
# сеть (GitHub, npm, nodejs.org). Контейнеры идут с seccomp=unconfined —
# иначе sandbox Chromium не может создать namespace.
# Логи: test/docker/.work/results/<os>-<сценарий>.log
set -eu
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
WORK=$HERE/.work
NODE_VER=v22.23.3
OSES=${OSES:-"24.04 26.04"}
JOBS=${JOBS:-4}
ALL="close sigterm sigterm_ignored sigterm_double adopt_samegroup adopt_detached
     dsh_update_auto dsh_update_other_prefix dsh_update_root_prefix
     launcher_update launcher_update_denied launcher_update_nopkexec"
SCENARIOS=${*:-$ALL}

mkdir -p "$WORK/src" "$WORK/ctx" "$WORK/results"

echo ">> сборка .deb из текущего кода (версия 1.1.0-1)"
rm -rf "$WORK/src"/* && mkdir -p "$WORK/src"
cp -a "$ROOT/main.js" "$ROOT/package.json" "$ROOT/deb" "$ROOT/build-deb.sh" "$ROOT/icon.png" "$WORK/src/"
ln -s "$ROOT/node_modules" "$WORK/src/node_modules"
sed -i 's/^Version: .*/Version: 1.1.0-1/' "$WORK/src/deb/control"
(cd "$WORK/src" && sh build-deb.sh >/dev/null)
cp -f "$WORK/src/dsh-launcher_1.1.0-1_amd64.deb" "$WORK/ctx/"

TARBALL=node-$NODE_VER-linux-x64.tar.xz
if [ ! -f "$WORK/ctx/$TARBALL" ]; then
  echo ">> скачиваю $TARBALL"
  curl -fsSL -o "$WORK/ctx/$TARBALL" "https://nodejs.org/dist/$NODE_VER/$TARBALL"
  curl -fsSL "https://nodejs.org/dist/$NODE_VER/SHASUMS256.txt" | grep " $TARBALL\$" \
    | (cd "$WORK/ctx" && sha256sum -c --quiet -) || { rm -f "$WORK/ctx/$TARBALL"; exit 1; }
fi
cp -f "$HERE/Dockerfile" "$HERE/fakedsh.js" "$HERE/driver.js" "$HERE/scenario.sh" "$WORK/ctx/"

for os in $OSES; do
  echo ">> образ dshl-test:$os"
  docker build -q --build-arg BASE=ubuntu:$os --build-arg NODE_VER=$NODE_VER -t dshl-test:$os "$WORK/ctx" >/dev/null
done

rm -f "$WORK/results"/*
one() {
  local os=$1 sc=$2 extra=""
  [ "$sc" = launcher_update_denied ] && extra="-e POLKIT=no"
  local name="dshl-test-${os//./}-$sc-$$"
  # --init: PID 1 в контейнере — tini, и SIGTERM от timeout доходит до
  # scenario.sh (сам он как PID 1 сигналы без обработчика не получает).
  # Если и так не завершился — kill клиента и docker rm -f.
  timeout -k 30 600 docker run --rm --init --name "$name" --shm-size=1g \
    --security-opt seccomp=unconfined $extra \
    dshl-test:$os /opt/t/scenario.sh "$sc" >"$WORK/results/$os-$sc.log" 2>&1
  local rc=$?
  docker rm -f "$name" >/dev/null 2>&1 || true
  [ $rc -eq 124 ] || [ $rc -eq 137 ] && echo "  [FAIL] сценарий не уложился в 10 минут" >>"$WORK/results/$os-$sc.log"
  echo "$([ $rc -eq 0 ] && echo PASS || echo "FAIL") $os $sc"
}
export -f one; export WORK
echo ">> сценарии (параллельно: $JOBS)"
for os in $OSES; do for sc in $SCENARIOS; do echo "$os $sc"; done; done \
  | xargs -P "$JOBS" -L 1 bash -c 'one $0 $1' | tee "$WORK/results/summary.txt"

FAILED=$(grep -c '^FAIL' "$WORK/results/summary.txt" || true)
TOTAL=$(wc -l < "$WORK/results/summary.txt")
echo ">> итог: $((TOTAL - FAILED))/$TOTAL прошли"
for f in $(grep '^FAIL' "$WORK/results/summary.txt" | awk '{print $2"-"$3}'); do
  echo "---- $f"; grep -E '^\s+\[FAIL\]|TIMEOUT' "$WORK/results/$f.log" || tail -20 "$WORK/results/$f.log"
done
[ "$FAILED" -eq 0 ]
