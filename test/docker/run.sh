#!/bin/bash
# Сценарии установленного лаунчера в Docker на чистых Ubuntu 24.04, 26.04 и
# Debian 13:
# закрытие окна и сигналы, «усыновлённый» dsh, обновление dsh (npm) и самого
# лаунчера (настоящий последний релиз с GitHub через pkexec).
#
#   test/docker/run.sh                 — все сценарии на всех трёх системах
#   test/docker/run.sh close sigterm   — только указанные
#   OSES="26.04" JOBS=2 test/docker/run.sh
#   OSES="debian-13" test/docker/run.sh   — Debian (debian:13)
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
OSES=${OSES:-"24.04 26.04 debian-13"}
JOBS=${JOBS:-4}
ALL="close sigterm sigterm_ignored sigterm_double adopt_samegroup adopt_detached
     dsh_update_auto dsh_update_other_prefix dsh_update_root_prefix
     launcher_update launcher_update_denied launcher_update_nopkexec menu_about context_menu
     find restart_dsh proxy window_state whats_new dsh_log"
SCENARIOS=${*:-$ALL}

mkdir -p "$WORK/src" "$WORK/ctx" "$WORK/results"

echo ">> сборка .deb из текущего кода (версия 1.1.0-1)"
rm -rf "$WORK/src"/* && mkdir -p "$WORK/src"
cp -a "$ROOT/main.js" "$ROOT/package.json" "$ROOT/deb" "$ROOT/build-deb.sh" "$ROOT/icon.png" "$WORK/src/"
ln -s "$ROOT/node_modules" "$WORK/src/node_modules"
sed -i 's/^Version: .*/Version: 1.1.0-1/' "$WORK/src/deb/control"
(cd "$WORK/src" && sh build-deb.sh >/dev/null)
cp -f "$WORK/src/dsh-launcher_1.1.0-1_amd64.deb" "$WORK/ctx/"

# Последний релиз лаунчера — на него сценарии launcher_update* и обновляются.
AUTH=(); [ -n "${GITHUB_TOKEN:-}" ] && AUTH=(-H "Authorization: Bearer $GITHUB_TOKEN")
LATEST=$(curl -fsSL "${AUTH[@]}" https://api.github.com/repos/Toximiner/DSH-Launcher/releases/latest \
  | sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p' | head -1)
[ -n "$LATEST" ] || { echo "не удалось узнать последний релиз (GitHub API)" >&2; exit 1; }
echo ">> последний релиз на GitHub: $LATEST"

TARBALL=node-$NODE_VER-linux-x64.tar.xz
if [ ! -f "$WORK/ctx/$TARBALL" ]; then
  echo ">> скачиваю $TARBALL"
  curl -fsSL -o "$WORK/ctx/$TARBALL" "https://nodejs.org/dist/$NODE_VER/$TARBALL"
  curl -fsSL "https://nodejs.org/dist/$NODE_VER/SHASUMS256.txt" | grep " $TARBALL\$" \
    | (cd "$WORK/ctx" && sha256sum -c --quiet -) || { rm -f "$WORK/ctx/$TARBALL"; exit 1; }
fi
cp -f "$HERE/Dockerfile" "$HERE/fakedsh.js" "$HERE/driver.js" "$HERE/proxy.js" "$HERE/scenario.sh" "$WORK/ctx/"

# На CI (GITHUB_ACTIONS) ошибки дублируются аннотациями — их видно на
# странице прогона без раскрытия логов (и через публичный API).
annotate() { # заголовок, текст (многострочный)
  [ -n "${GITHUB_ACTIONS:-}" ] || return 0
  local msg; msg=$(printf '%s' "$2" | sed ':a;N;$!ba;s/%/%25/g;s/\r//g;s/\n/%0A/g')
  echo "::error title=$1::$msg"
}

# Образ: базовый ubuntu с Docker Hub. Без авторизации Docker Hub ограничивает
# загрузки с одного IP (429 Too Many Requests), а IP раннеров общие — поэтому
# 3 попытки с паузой, затем то же с mirror.gcr.io (официальное зеркало
# Docker Hub от Google).
# OS — версия Ubuntu («26.04») или Debian («debian-13» → debian:13).
base_of() { case "$1" in debian-*) echo "debian:${1#debian-}" ;; *) echo "ubuntu:$1" ;; esac; }
build_image() {
  local os=$1 base out try img
  img=$(base_of "$os")
  for base in "$img" "mirror.gcr.io/library/$img"; do
    for try in 1 2 3; do
      if out=$(docker build -q --build-arg BASE="$base" --build-arg NODE_VER="$NODE_VER" -t "dshl-test:$os" "$WORK/ctx" 2>&1); then
        [ "$base" = "$img" ] || echo "   база с зеркала: $base"
        return 0
      fi
      echo "   сборка не удалась ($base, попытка $try): $(echo "$out" | grep -m1 -iE 'error|429' | cut -c1-200)"
      # Повторять имеет смысл только сетевые сбои (лимит, таймаут, DNS, npm
      # 404 свежей публикации); ошибка в Dockerfile или пакете повторится.
      echo "$out" | grep -qiE '429|Too Many Requests|toomanyrequests|failed to resolve source metadata|timeout|TLS handshake|connection reset|EAI_AGAIN|E404|ECONNRESET|ETIMEDOUT' || break 2
      [ "$try" -lt 3 ] && sleep $((try * 15))
    done
  done
  echo "$out" | tail -20
  annotate "Docker: образ $os" "$(echo "$out" | tail -5)"
  return 1
}

for os in $OSES; do
  echo ">> образ dshl-test:$os"
  build_image "$os" || exit 1
done

rm -f "$WORK/results"/*
one() {
  local os=$1 sc=$2 extra=""
  [ "$sc" = launcher_update_denied ] && extra="-e POLKIT=no"
  extra="$extra -e LATEST=$LATEST"
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
export -f one; export WORK LATEST
echo ">> сценарии (параллельно: $JOBS)"
for os in $OSES; do for sc in $SCENARIOS; do echo "$os $sc"; done; done \
  | xargs -P "$JOBS" -L 1 bash -c 'one $0 $1' | tee "$WORK/results/summary.txt"

FAILED=$(grep -c '^FAIL' "$WORK/results/summary.txt" || true)
TOTAL=$(wc -l < "$WORK/results/summary.txt")
echo ">> итог: $((TOTAL - FAILED))/$TOTAL прошли"
for f in $(grep '^FAIL' "$WORK/results/summary.txt" | awk '{print $2"-"$3}'); do
  fails=$(grep -E '^\s+\[FAIL\]|TIMEOUT' "$WORK/results/$f.log" || tail -20 "$WORK/results/$f.log")
  echo "---- $f"; echo "$fails"
  annotate "Docker: $f" "$fails"
done
[ "$FAILED" -eq 0 ]
