#!/bin/bash
# Один сценарий в свежем контейнере (запуск от root): scenario.sh <имя>.
# Запускается из test/docker/run.sh; внутри контейнера — system dbus, polkitd
# (правило «tester — разрешить/запретить» по POLKIT=yes|no) и Xvfb, лаунчер —
# от пользователя tester с --remote-debugging-port=9222 (окном управляет
# driver.js через CDP).
set -u
NAME="$1"
T=/opt/t
DRV="/opt/node22/bin/node $T/driver.js"
OUT=/tmp/launcher.out
FAILS=0
NVM_BIN=$(ls -d /home/tester/.nvm/versions/node/*/bin | head -1)
UPD=/home/tester/.config/dsh-launcher/updates

ok()   { echo "  [ OK ] $*"; }
bad()  { echo "  [FAIL] $*"; FAILS=$((FAILS + 1)); }
check() { local d="$1"; shift; if "$@" >/dev/null 2>&1; then ok "$d"; else bad "$d"; fi; }
drv()  { runuser -u tester -- $DRV "$@"; }
step() { local d="$1"; shift; local o; if o=$("$@" 2>&1); then ok "$d"; else bad "$d"; echo "$o" | tail -5 | sed 's/^/         /'; fi; }

# --- окружение: system dbus, polkitd, Xvfb ---
mkdir -p /run/dbus && dbus-daemon --system --fork
POLKITD=$(ls /usr/lib/polkit-1/polkitd /usr/libexec/polkitd 2>/dev/null | head -1)
mkdir -p /etc/polkit-1/rules.d
if [ "${POLKIT:-yes}" = yes ]; then
  echo 'polkit.addRule(function(a, s) { if (s.user == "tester") return polkit.Result.YES; });' > /etc/polkit-1/rules.d/00-test.rules
else
  echo 'polkit.addRule(function(a, s) { if (s.user == "tester") return polkit.Result.NO; });' > /etc/polkit-1/rules.d/00-test.rules
fi
"$POLKITD" --no-debug >/tmp/polkitd.log 2>&1 &
Xvfb :99 -screen 0 1280x800x24 -nolisten tcp >/dev/null 2>&1 &
sleep 1.5
# Вопрос про маркет в тестах не нужен.
runuser -u tester -- sh -c 'mkdir -p ~/.config/dsh-launcher && echo "{\"dontAsk\":true}" > ~/.config/dsh-launcher/market-prompt.json'
rm -f /tmp/fakedsh.log

launcher_pids() { # главные процессы лаунчера (без --type=)
  for p in $(pgrep -u tester -f 'electron/electron /opt/dsh-launcher'); do
    tr '\0' ' ' < /proc/$p/cmdline 2>/dev/null | grep -q -- '--type=' || echo "$p"
  done
}
start_launcher() { # переменные окружения сценария — аргументами
  runuser -u tester -- env -i HOME=/home/tester USER=tester LANG=C.UTF-8 DISPLAY=:99 \
    PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin "$@" \
    /usr/bin/dsh-launcher --remote-debugging-port=9222 >"$OUT" 2>&1 &
  for _ in $(seq 1 60); do
    LPID=$(launcher_pids | head -1); [ -n "$LPID" ] && curl -s -o /dev/null http://127.0.0.1:9222/json/list && return 0
    sleep 0.5
  done
  echo "лаунчер не стартовал:"; tail -20 "$OUT"; return 1
}
wait_exit() { # pid, секунд → печатает время выхода в секундах
  local t0=$(date +%s.%N)
  for _ in $(seq 1 $(( $2 * 10 ))); do kill -0 "$1" 2>/dev/null || { awk "BEGIN{printf \"%.1f\", $(date +%s.%N) - $t0}"; return 0; }; sleep 0.1; done
  return 1
}
tree_gone()  { ! pgrep -f fakedsh.js >/dev/null && ! pgrep -f '^sleep 7777' >/dev/null; }
tree_alive() { pgrep -f fakedsh.js >/dev/null && pgrep -f '^sleep 7777' >/dev/null; }
port_up()    { curl -s -o /dev/null http://127.0.0.1:3080/; }
wait_port()  { for _ in $(seq 1 $(( $1 * 2 ))); do port_up && return 0; sleep 0.5; done; return 1; }
wait_log()   { for _ in $(seq 1 $(( $2 * 2 ))); do grep -q -- "$1" "$OUT" && return 0; sleep 0.5; done; return 1; }
real_dsh_gone() { ! pgrep -u tester -f '@deepseek-ai/dsh|bin/dsh' >/dev/null; }

FAKE="DSH_BIN=$T/fakedsh.js"
NOUPD="DSH_LAUNCHER_NO_UPDATE_CHECK=1"

echo "=== $NAME ($(. /etc/os-release; echo "$PRETTY_NAME")) ==="
case "$NAME" in

close)
  start_launcher $FAKE $NOUPD || exit 1
  step "окно дошло до GUI dsh" drv wait-url '^http://127.0.0.1:3080/' 60
  check "дерево fakedsh живо (dsh + sleep)" tree_alive
  drv eval 'window.close()' >/dev/null
  step "лаунчер завершился после закрытия окна" wait_exit "$LPID" 15
  check "дерево dsh остановлено" tree_gone
  check "dsh получил SIGTERM (штатно)" grep -q SIGTERM /tmp/fakedsh.log
  ;;

sigterm)
  start_launcher $FAKE $NOUPD || exit 1
  step "окно дошло до GUI dsh" drv wait-url '^http://127.0.0.1:3080/' 60
  kill -TERM "$LPID"
  T_EXIT=$(wait_exit "$LPID" 15) && ok "лаунчер вышел по SIGTERM за ${T_EXIT} с" || bad "лаунчер не вышел по SIGTERM за 15 с"
  check "дерево dsh остановлено" tree_gone
  check "dsh получил SIGTERM (штатное закрытие, а не сразу SIGKILL)" grep -q SIGTERM /tmp/fakedsh.log
  check "в логе лаунчера — обработчик сигнала" grep -q 'SIGTERM — ' "$OUT"
  ;;

sigterm_ignored)
  start_launcher $FAKE $NOUPD FAKE_IGNORE_TERM=1 || exit 1
  step "окно дошло до GUI dsh" drv wait-url '^http://127.0.0.1:3080/' 60
  kill -TERM "$LPID"
  T_EXIT=$(wait_exit "$LPID" 15) && ok "лаунчер вышел за ${T_EXIT} с (ожидаем ~4 с: SIGTERM → SIGKILL)" || bad "лаунчер не вышел за 15 с"
  check "dsh, игнорирующий SIGTERM, добит SIGKILL" tree_gone
  ;;

sigterm_double)
  start_launcher $FAKE $NOUPD FAKE_IGNORE_TERM=1 || exit 1
  step "окно дошло до GUI dsh" drv wait-url '^http://127.0.0.1:3080/' 60
  kill -TERM "$LPID"; sleep 0.5; kill -TERM "$LPID"
  T_EXIT=$(wait_exit "$LPID" 5) && ok "повторный SIGTERM — выход сразу (${T_EXIT} с от второго+0.5)" || bad "лаунчер не вышел за 5 с после повторного SIGTERM"
  sleep 0.5
  check "дерево dsh остановлено" tree_gone
  ;;

adopt_samegroup|adopt_detached)
  EXTRA=""; [ "$NAME" = adopt_detached ] && EXTRA="FAKE_REBORN_DETACHED=1"
  start_launcher $FAKE $NOUPD FAKE_SELF_RESTART_AFTER=5 $EXTRA || exit 1
  step "окно дошло до GUI dsh" drv wait-url '^http://127.0.0.1:3080/' 60
  step "лаунчер усыновил перезапущенный dsh" wait_log 'перезапустился сам' 30
  check "перерождённый dsh слушает порт" wait_port 10
  check "в старой группе остался sleep 7777" pgrep -f '^sleep 7777'
  grep -E 'start|self' /tmp/fakedsh.log | sed 's/^/         /'
  drv eval 'window.close()' >/dev/null
  step "лаунчер завершился после закрытия окна" wait_exit "$LPID" 15
  grep -E 'усыновл' "$OUT" | sed 's/^/         /'
  check "усыновлённый dsh и потомки старой группы остановлены" tree_gone
  ;;

dsh_update_auto)
  start_launcher DSH_LAUNCHER_FAKE_INSTALLED=9.9.9-1 || exit 1
  step "вопрос об обновлении dsh (0.1.7-rc.2 → 0.2.0-rc.2)" drv wait-text 'Update DeepSeek Harness.*0\.2\.0-rc\.2.*installed: 0\.1\.7-rc\.2' 60
  drv eval "location.href='dshlauncher://update/install/'" >/dev/null
  step "npm install -g прошёл, dsh обновлён" wait_log 'обновлён → 0.2.0-rc.2' 300
  check "в nvm теперь dsh 0.2.0-rc.2" sh -c "runuser -u tester -- env PATH=$NVM_BIN:/usr/bin:/bin $NVM_BIN/dsh --version | grep -qx 0.2.0-rc.2"
  step "лаунчер перешёл к запуску dsh (порт поднят)" wait_port 120
  kill -TERM "$LPID"
  step "лаунчер вышел по SIGTERM" wait_exit "$LPID" 15
  sleep 0.5
  check "настоящий dsh остановлен" real_dsh_gone
  ;;

dsh_update_other_prefix)
  start_launcher DSH_LAUNCHER_FAKE_INSTALLED=9.9.9-1 DSH_BIN=/home/tester/.local/bin/dsh \
    PATH=$NVM_BIN:/usr/local/bin:/usr/bin:/bin || exit 1
  step "вопрос об обновлении dsh" drv wait-text 'Update DeepSeek Harness' 60
  drv eval "location.href='dshlauncher://update/install/'" >/dev/null
  step "страница: npm поставил в другой prefix" drv wait-text 'npm installed the package into /home/tester/.nvm.*runs /home/tester/.local/bin/dsh \(version 0\.1\.7-rc\.2\)' 300
  step "в команде нет sudo" drv wait-text 'Run in a terminal: npm install -g @deepseek-ai/dsh@0\.2\.0-rc\.2' 5
  drv eval "location.href='dshlauncher://update/later/'" >/dev/null
  step "«Не сейчас» — dsh запускается" wait_port 120
  kill -TERM "$LPID"; wait_exit "$LPID" 15 >/dev/null
  ;;

dsh_update_root_prefix)
  start_launcher DSH_LAUNCHER_FAKE_INSTALLED=9.9.9-1 DSH_BIN=/opt/noderoot/bin/dsh || exit 1
  step "вопрос об обновлении dsh" drv wait-text 'Update DeepSeek Harness' 60
  drv eval "location.href='dshlauncher://update/install/'" >/dev/null
  step "prefix root — страница с sudo-командой" drv wait-text 'needs sudo.*sudo npm install -g @deepseek-ai/dsh@0\.2\.0-rc\.2' 60
  drv eval "location.href='dshlauncher://update/never/'" >/dev/null
  step "«Не спрашивать больше» — dsh запускается" wait_port 120
  check "отказ запомнен на версию" grep -q '"dsh":"0.2.0-rc.2"' /home/tester/.config/dsh-launcher/update-prefs.json
  kill -TERM "$LPID"; wait_exit "$LPID" 15 >/dev/null
  ;;

launcher_update)
  start_launcher $FAKE || exit 1
  step "вопрос об обновлении лаунчера (1.1.0-1 → 1.1.1)" drv wait-text 'Update DSH Launcher.*Version 1\.1\.1 is available \(installed: 1\.1\.0-1\s*\)' 60
  drv eval "document.querySelector('a[target=_blank]').click()" >/dev/null; sleep 1.5
  step "клик по ссылке релиза не уводит окно со страницы вопроса" drv wait-text 'Update DSH Launcher' 3
  drv eval "location.href='dshlauncher://update/install/'" >/dev/null
  step "скачано и установлено через pkexec" wait_log 'dsh-launcher 1.1.1 установлен' 400
  grep -E 'скачал|шаг обновления' "$OUT" | sed 's/^/         /'
  check "sha256 совпал с digest релиза" grep -q 'совпадает с релизом' "$OUT"
  check "dpkg: установлен 1.1.1-1" sh -c "dpkg-query -Wf '\${Version}' dsh-launcher | grep -qx 1.1.1-1"
  step "старый экземпляр завершился" wait_exit "$LPID" 20
  sleep 3
  NEW=$(launcher_pids | head -1)
  [ -n "$NEW" ] && [ "$NEW" != "$LPID" ] && ok "новый экземпляр запущен (pid $NEW)" || bad "новый экземпляр не запущен"
  check "новый экземпляр поднял dsh" wait_port 60
  check "старый экземпляр после обновления dsh не запускал (один старт)" sh -c '[ "$(grep -c "start pgid" /tmp/fakedsh.log)" -eq 1 ]'
  check "скачанный .deb удалён" sh -c "! ls $UPD/*.deb"
  [ -n "$NEW" ] && { kill -TERM "$NEW"; wait_exit "$NEW" 15 >/dev/null; }
  ;;

launcher_update_denied)
  start_launcher $FAKE || exit 1
  step "вопрос об обновлении лаунчера" drv wait-text 'Update DSH Launcher' 60
  drv eval "location.href='dshlauncher://update/install/'" >/dev/null
  step "polkit отказал — страница с командой для терминала" drv wait-text "Update manually.*Installation failed \(code 127\).*sudo apt install -y $UPD/dsh-launcher_1\.1\.1-1_amd64\.deb" 400
  check "dpkg: всё ещё 1.1.0-1" sh -c "dpkg-query -Wf '\${Version}' dsh-launcher | grep -qx 1.1.0-1"
  step "пользователь ставит пакет в терминале" apt-get install -y "$UPD/dsh-launcher_1.1.1-1_amd64.deb"
  drv eval "location.href='dshlauncher://update/recheck/'" >/dev/null
  step "«Проверить ещё раз» — «Update installed»" wait_log 'обновлён до 1.1.1-1 (вручную)' 30
  step "старый экземпляр завершился" wait_exit "$LPID" 20
  sleep 3
  NEW=$(launcher_pids | head -1)
  [ -n "$NEW" ] && [ "$NEW" != "$LPID" ] && ok "новый экземпляр запущен (pid $NEW)" || bad "новый экземпляр не запущен"
  check "новый экземпляр поднял dsh" wait_port 60
  check "старый экземпляр после обновления dsh не запускал (один старт)" sh -c '[ "$(grep -c "start pgid" /tmp/fakedsh.log)" -eq 1 ]'
  [ -n "$NEW" ] && { kill -TERM "$NEW"; wait_exit "$NEW" 15 >/dev/null; }
  ;;

launcher_update_nopkexec)
  mv /usr/bin/pkexec /usr/bin/pkexec.off
  start_launcher $FAKE || exit 1
  step "вопрос об обновлении лаунчера" drv wait-text 'Update DSH Launcher' 60
  drv eval "location.href='dshlauncher://update/install/'" >/dev/null
  step "нет pkexec — страница с командой" drv wait-text "pkexec was not found.*sudo apt install -y $UPD/dsh-launcher_1\.1\.1-1_amd64\.deb" 400
  drv eval "location.href='dshlauncher://update/later/'" >/dev/null
  step "«Не сейчас» — dsh запускается" wait_port 60
  kill -TERM "$LPID"; wait_exit "$LPID" 15 >/dev/null
  ;;

*) echo "неизвестный сценарий"; exit 2 ;;
esac

if [ "$FAILS" -ne 0 ]; then
  echo "  --- хвост лога лаунчера ---"; tail -25 "$OUT" | sed 's/^/  | /'
fi
echo "=== $NAME: $([ "$FAILS" -eq 0 ] && echo PASS || echo "FAIL ($FAILS)") ==="
exit "$FAILS"
