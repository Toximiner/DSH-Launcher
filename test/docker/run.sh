#!/bin/bash
# Scenarios of the installed launcher in Docker on clean Ubuntu 24.04, 26.04
# and Debian 13:
# window close and signals, an «adopted» dsh, updating dsh (npm) and the
# launcher itself via pkexec (releases — from the fake GitHub, test/docker/fakes/github.py).
#
#   test/docker/run.sh                 — all scenarios on all three systems
#   test/docker/run.sh close sigterm   — only the specified ones
#   OSES="26.04" JOBS=2 test/docker/run.sh
#   OSES="debian-13" test/docker/run.sh   — Debian (debian:13)
#
# Requires Docker, a built Electron (npm install && npx install-electron) and
# a network (npm, nodejs.org; GitHub — only for the proxy scenario). The containers run with seccomp=unconfined —
# otherwise the Chromium sandbox cannot create a namespace.
# Logs: test/docker/.work/results/<os>-<scenario>.log
set -eu
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
WORK=$HERE/.work
NODE_VER=v22.23.3
OSES=${OSES:-"24.04 26.04 debian-13"}
JOBS=${JOBS:-4}
# Scenarios: the arguments or (by default) all — the list is provided by
# pytest itself (`--list-scenarios`, see conftest.py) after the image build.
SCENARIOS=${*:-}

mkdir -p "$WORK/src" "$WORK/ctx" "$WORK/results"

echo ">> сборка .deb из текущего кода (номер версии 1.1.0-1 — нарочно ниже любого релиза, для сценариев обновления)"
rm -rf "$WORK/src"/* && mkdir -p "$WORK/src"
cp -a "$ROOT/main.js" "$ROOT/src" "$ROOT/package.json" "$ROOT/deb" "$ROOT/build-deb.sh" "$ROOT/icon.png" "$WORK/src/"
ln -s "$ROOT/node_modules" "$WORK/src/node_modules"
sed -i 's/^Version: .*/Version: 1.1.0-1/' "$WORK/src/deb/control"
(cd "$WORK/src" && sh build-deb.sh >/dev/null)
cp -f "$WORK/src/dsh-launcher_1.1.0-1_amd64.deb" "$WORK/ctx/"

# .deb packages of the fake GitHub's releases (helpers/github.py): the same
# code with version numbers 2.0.0-1 (latest) and 1.0.0-14 (rollback) — from
# the build-deb.sh build directory.
# gzip -1: the packages live for a single run. Into the container — as the /opt/github directory.
rm -rf "$WORK/github" && mkdir -p "$WORK/github"
for pkg in 2.0.0-1 1.0.0-14; do
  stage=$WORK/stage-$pkg
  rm -rf "$stage" && cp -a "$WORK/src/build/deb" "$stage"
  sed -i "s/^Version: .*/Version: $pkg/" "$stage/DEBIAN/control"
  sed -i "s/^  \"version\": \"[^\"]*\"/  \"version\": \"${pkg%-*}\"/" "$stage/opt/dsh-launcher/package.json"
  grep -q "\"version\": \"${pkg%-*}\"" "$stage/opt/dsh-launcher/package.json" || { echo "версия в package.json не заменилась" >&2; exit 1; }
  dpkg-deb --build --root-owner-group -Zgzip -z1 "$stage" "$WORK/github/dsh-launcher_${pkg}_amd64.deb" >/dev/null
  rm -rf "$stage"
done

TARBALL=node-$NODE_VER-linux-x64.tar.xz
if [ ! -f "$WORK/ctx/$TARBALL" ]; then
  echo ">> скачиваю $TARBALL"
  curl -fsSL -o "$WORK/ctx/$TARBALL" "https://nodejs.org/dist/$NODE_VER/$TARBALL"
  curl -fsSL "https://nodejs.org/dist/$NODE_VER/SHASUMS256.txt" | grep " $TARBALL\$" \
    | (cd "$WORK/ctx" && sha256sum -c --quiet -) || { rm -f "$WORK/ctx/$TARBALL"; exit 1; }
fi
rm -rf "$WORK/ctx/t" && mkdir -p "$WORK/ctx/t" && cp -f "$HERE/Dockerfile" "$WORK/ctx/" \
  && cp -a "$HERE/conftest.py" "$HERE/helpers" "$HERE/fakes" "$HERE/tests" "$WORK/ctx/t/" \
  && find "$WORK/ctx/t" -name __pycache__ -prune -exec rm -rf {} +

# On CI (GITHUB_ACTIONS) errors are duplicated as annotations — they are
# visible on the run page without expanding the logs (and via the public API).
annotate() { # title, text (multiline)
  [ -n "${GITHUB_ACTIONS:-}" ] || return 0
  local msg; msg=$(printf '%s' "$2" | sed ':a;N;$!ba;s/%/%25/g;s/\r//g;s/\n/%0A/g')
  echo "::error title=$1::$msg"
}

# Image: the base ubuntu from Docker Hub. Without authentication Docker Hub
# limits pulls from one IP (429 Too Many Requests), and the runners share IPs
# — hence 3 attempts with a pause, then the same with mirror.gcr.io (Google's
# official Docker Hub mirror).
# Building the image. On CI (DOCKER_GHA_CACHE=1) — buildx with a layer cache
# in GitHub Actions: only changed layers are rebuilt (the launcher package
# and the tests — at the end of the Dockerfile), apt / Node / the old dsh
# come from the cache.
docker_build() { # <os> <docker build arguments>
  local os=$1; shift
  if [ -n "${DOCKER_GHA_CACHE:-}" ]; then
    docker buildx build -q --load --cache-from "type=gha,scope=dshl-$os" \
      --cache-to "type=gha,mode=max,scope=dshl-$os" "$@"
  else
    docker build -q "$@"
  fi
}

# OS — an Ubuntu version («26.04») or Debian («debian-13» → debian:13).
base_of() { case "$1" in debian-*) echo "debian:${1#debian-}" ;; *) echo "ubuntu:$1" ;; esac; }
build_image() {
  local os=$1 base out try img
  img=$(base_of "$os")
  for base in "$img" "mirror.gcr.io/library/$img"; do
    for try in 1 2 3; do
      if out=$(docker_build "$os" --build-arg BASE="$base" --build-arg NODE_VER="$NODE_VER" -t "dshl-test:$os" "$WORK/ctx" 2>&1); then
        [ "$base" = "$img" ] || echo "   база с зеркала: $base"
        return 0
      fi
      echo "   сборка не удалась ($base, попытка $try): $(echo "$out" | grep -m1 -iE 'error|429' | cut -c1-200)"
      # Retrying only makes sense for network failures (rate limit, timeout,
      # DNS, npm 404 of a fresh publish); a Dockerfile or package error will repeat.
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

if [ -z "$SCENARIOS" ]; then
  first=${OSES%% *}
  SCENARIOS=$(docker run --rm "dshl-test:$first" python3 -m pytest -p no:cacheprovider -q \
    --rootdir /opt/t /opt/t/tests --list-scenarios 2>/dev/null | sed -n 's/^SCENARIO //p' | tr '\n' ' ')
  [ -n "$SCENARIOS" ] || { echo "не удалось получить список сценариев (pytest --list-scenarios)" >&2; exit 1; }
fi

rm -f "$WORK/results"/*
one() {
  local os=$1 sc=$2 extra=""
  [ "$sc" = launcher_update_denied ] && extra="-e POLKIT=no"
  # the CI token for the launcher's GitHub API requests (limit 5000/h): -e NAME —
  # the value docker takes from the environment; it does not end up in the command line
  [ -n "${DSH_LAUNCHER_GITHUB_TOKEN:-}" ] && extra="$extra -e DSH_LAUNCHER_GITHUB_TOKEN"
  local name="dshl-test-${os//./}-$sc-$$"
  # --init: PID 1 in the container is tini, so the SIGTERM from timeout
  # reaches pytest (as PID 1 itself it does not receive unhandled signals).
  # If it still does not finish — kill the client and docker rm -f.
  timeout -k 30 600 docker run --rm --init --name "$name" --shm-size=1g \
    --security-opt seccomp=unconfined $extra -v "$WORK/results:/results" -v "$WORK/github:/opt/github:ro" \
    dshl-test:$os python3 -m pytest -p no:cacheprovider -q -rf --tb=short --color=no \
    --rootdir /opt/t /opt/t/tests --scenario "$sc" --junitxml "/results/$os-$sc.xml" \
    >"$WORK/results/$os-$sc.log" 2>&1
  local rc=$?
  docker rm -f "$name" >/dev/null 2>&1 || true
  [ $rc -eq 124 ] || [ $rc -eq 137 ] && echo "  [FAIL] сценарий не уложился в 10 минут" >>"$WORK/results/$os-$sc.log"
  echo "$([ $rc -eq 0 ] && echo PASS || echo "FAIL") $os $sc"
}
[ -n "${GITHUB_TOKEN:-}" ] && export DSH_LAUNCHER_GITHUB_TOKEN="$GITHUB_TOKEN"
export -f one; export WORK
echo ">> сценарии (параллельно: $JOBS)"
for os in $OSES; do for sc in $SCENARIOS; do echo "$os $sc"; done; done \
  | xargs -P "$JOBS" -L 1 bash -c 'one $0 $1' | tee "$WORK/results/summary.txt"

FAILED=$(grep -c '^FAIL' "$WORK/results/summary.txt" || true)
TOTAL=$(wc -l < "$WORK/results/summary.txt")
echo ">> итог: $((TOTAL - FAILED))/$TOTAL прошли"
for f in $(grep '^FAIL' "$WORK/results/summary.txt" | awk '{print $2"-"$3}'); do
  fails=$(grep -E '^E  |^(FAILED|ERROR) ' "$WORK/results/$f.log" | head -20 || true)
  [ -n "$fails" ] || fails=$(tail -20 "$WORK/results/$f.log")
  echo "---- $f"; echo "$fails"
  annotate "Docker: $f" "$fails"
done
# The results table — into the CI run summary (Summary)
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  python3 "$HERE/summary.py" "$WORK/results" >>"$GITHUB_STEP_SUMMARY" || true
fi

# Launcher code coverage from the scenarios (conftest.py: coverage-*.json
# from the main process). Package paths /opt/dsh-launcher/ → repository files
# (the package was built from them), the report — c8: test/docker/.work/coverage/ (HTML) and
# coverage.txt; on CI — in the Summary. Without c8 (npm install not run) — skip.
if ls "$WORK/results"/coverage-*.json >/dev/null 2>&1 && [ -x "$ROOT/node_modules/.bin/c8" ]; then
  rm -rf "$WORK/v8" "$WORK/coverage" && mkdir -p "$WORK/v8"
  for f in "$WORK/results"/coverage-*.json; do
    sed "s#file:///opt/dsh-launcher/#file://$ROOT/#g" "$f" >"$WORK/v8/$(basename "$f")"
  done
  (cd "$ROOT" && node_modules/.bin/c8 report --temp-directory "$WORK/v8" --reports-dir "$WORK/coverage" \
    --all --src . --include main.js --include 'src/**' \
    --reporter text --reporter text-summary --reporter html) >"$WORK/results/coverage.txt" 2>&1 || true
  echo ">> покрытие кода лаунчера сценариями ($(ls "$WORK/v8" | wc -l) процессов):"
  grep -A6 '^=* Coverage summary' "$WORK/results/coverage.txt" || tail -5 "$WORK/results/coverage.txt"
  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    { echo "### Покрытие кода лаунчера сценариями ($OSES)"; echo '```'; cat "$WORK/results/coverage.txt"; echo '```'; } >>"$GITHUB_STEP_SUMMARY"
  fi
fi
[ "$FAILED" -eq 0 ]
