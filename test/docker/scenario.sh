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
# Последний релиз лаунчера на GitHub (передаёт run.sh): тег vX.Y.Z → пакет X.Y.Z-1.
LATEST=${LATEST:?LATEST не задан — запускайте через run.sh}
LATEST_PKG=$(case "$LATEST" in *-*) echo "$LATEST" ;; *) echo "$LATEST-1" ;; esac)
LATEST_RE=$(printf '%s' "$LATEST" | sed 's/\./\\./g')
DEB_RE="dsh-launcher_$(printf '%s' "$LATEST_PKG" | sed 's/\./\\./g')_amd64\\.deb"

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
  step "вопрос об обновлении лаунчера (1.1.0-1 → $LATEST)" drv wait-text "Update DSH Launcher.*Version $LATEST_RE is available \\(installed: 1\\.1\\.0-1\\s*\\)" 60
  drv eval "document.querySelector('a[target=_blank]').click()" >/dev/null; sleep 1.5
  step "клик по ссылке релиза не уводит окно со страницы вопроса" drv wait-text 'Update DSH Launcher' 3
  drv eval "location.href='dshlauncher://update/install/'" >/dev/null
  step "скачано и установлено через pkexec" wait_log "dsh-launcher $LATEST установлен" 400
  grep -E 'скачал|шаг обновления' "$OUT" | sed 's/^/         /'
  check "sha256 совпал с digest релиза" grep -q 'совпадает с релизом' "$OUT"
  check "dpkg: установлен $LATEST_PKG" sh -c "dpkg-query -Wf '\${Version}' dsh-launcher | grep -qx $LATEST_PKG"
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
  step "polkit отказал — страница с командой для терминала" drv wait-text "Update manually.*Installation failed \(code 127\).*sudo apt install -y $UPD/$DEB_RE" 400
  check "dpkg: всё ещё 1.1.0-1" sh -c "dpkg-query -Wf '\${Version}' dsh-launcher | grep -qx 1.1.0-1"
  step "пользователь ставит пакет в терминале" apt-get install -y "$UPD/dsh-launcher_${LATEST_PKG}_amd64.deb"
  drv eval "location.href='dshlauncher://update/recheck/'" >/dev/null
  step "«Проверить ещё раз» — «Update installed»" wait_log "обновлён до $LATEST_PKG (вручную)" 30
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
  step "нет pkexec — страница с командой" drv wait-text "pkexec was not found.*sudo apt install -y $UPD/$DEB_RE" 400
  drv eval "location.href='dshlauncher://update/later/'" >/dev/null
  step "«Не сейчас» — dsh запускается" wait_port 60
  kill -TERM "$LPID"; wait_exit "$LPID" 15 >/dev/null
  ;;

menu_about)
  # «О программе» / «Проверить обновления» — модальное окно поверх главного,
  # после проверки обновлений при старте. Проверки включены, «последние» версии
  # подменены на старые — без вопроса об обновлении. Окно открывается по URL
  # (по нативному меню driver кликать не умеет); маршрут отвечает 204, так что
  # GUI dsh остаётся на месте. (1.3.0 на машине разработчика падал здесь —
  # electronNet.fetch + dpkg-query/dsh --version, — но в контейнере то падение
  # не воспроизводится.)
  start_launcher $FAKE DSH_LAUNCHER_FAKE_LAUNCHER_LATEST=1.0.0 DSH_LAUNCHER_FAKE_DSH_LATEST=0.0.1 || exit 1
  step "окно дошло до GUI dsh" drv wait-url '^http://127.0.0.1:3080/' 60
  step "проверка обновлений при старте прошла (npm)" wait_log 'последний @deepseek-ai/dsh' 30
  GUI='^http://127\.0\.0\.1:3080/'
  drv --page "$GUI" eval 'window.__mark = 42; 1' >/dev/null
  gui_kept() { [ "$(drv --page "$GUI" eval 'window.__mark')" = 42 ]; }
  dimmed()   { drv --page "$GUI" eval "getComputedStyle(document.documentElement, '::after').backgroundColor" | grep -q '0\.55'; }
  dialog_gone() { for _ in $(seq 1 20); do drv pages | grep -q '^\(About\|Update check\) |' || return 0; sleep 0.5; done; return 1; }
  for i in 1 2 3; do
    drv --page "$GUI" eval "location.href='dshlauncher://menu/about/'" >/dev/null
    step "«О программе» #$i — окно с версиями лаунчера и dsh" drv --page '^About$' wait-text 'DSH Launcher 1\.1\.0-1.*dsh backend 0\.2\.0-rc\.2.*Copy for report' 30
    check "GUI dsh под окном не перезагружался (#$i)" gui_kept
    check "GUI dsh под окном затемнён (#$i)" dimmed
    if [ "$i" = 1 ]; then
      drv --page '^About$' eval "location.href='dshlauncher://dialog/copy/'" >/dev/null
      step "«Copy for report» → «Copied»" drv --page '^About$' wait-text 'Copied' 10
    fi
    drv --page '^About$' eval "location.href='dshlauncher://dialog/close/'" >/dev/null
    step "«Close» #$i — окно закрылось" dialog_gone
    if dimmed; then bad "затемнение снято (#$i)"; else ok "затемнение снято (#$i)"; fi
    check "лаунчер жив после «О программе» #$i" kill -0 "$LPID"
  done
  drv --page "$GUI" eval "location.href='dshlauncher://menu/check/'" >/dev/null
  step "«Проверить обновления» — окно: оба компонента актуальны" drv --page '^Update check$' wait-text 'DSH Launcher.*up to date.*dsh backend.*up to date' 60
  check "GUI dsh под окном не перезагружался" gui_kept
  drv --page '^Update check$' eval "location.href='dshlauncher://dialog/close/'" >/dev/null
  step "окно проверки закрылось" dialog_gone
  check "лаунчер жив после проверки обновлений" kill -0 "$LPID"
  kill -TERM "$LPID"; wait_exit "$LPID" 15 >/dev/null
  ;;

context_menu)
  # Правый клик настоящим событием мыши (CDP) → нативное контекстное меню.
  # Кликнуть по пунктам driver не может; что показано — сверяем по строке
  # «контекстное меню: …» в логе лаунчера. Главное — лаунчер не падает.
  start_launcher $FAKE $NOUPD || exit 1
  step "окно дошло до GUI dsh" drv wait-url '^http://127.0.0.1:3080/' 60
  # Абзац, поле ввода и ссылка в известных координатах (CSS px = координаты клика).
  drv eval "document.body.innerHTML = '<p id=t style=\"position:absolute;left:20px;top:20px;margin:0;font-size:20px\">Hello context menu</p>' +
    '<input id=i value=\"some input text\" style=\"position:absolute;left:20px;top:80px;width:300px;height:30px\">' +
    '<a id=l href=\"https://example.com/\" style=\"position:absolute;left:20px;top:140px;font-size:20px\">a link</a>'; 'ok'" >/dev/null
  step "на странице поле ввода, текст и ссылка" drv eval "Boolean(document.getElementById('i') && document.getElementById('l'))"
  ctx() { # x y <regexp последней строки «контекстное меню: …»>
    local n; n=$(grep -c 'контекстное меню:' "$OUT")
    drv rclick "$1" "$2" || return 1
    for _ in $(seq 1 20); do [ "$(grep -c 'контекстное меню:' "$OUT")" -gt "$n" ] && break; sleep 0.25; done
    local last; last=$(grep 'контекстное меню:' "$OUT" | tail -1)
    echo "$last" | grep -Eq -- "$3" || { echo "${last:-меню не показано}"; return 1; }
  }
  drv eval "getSelection().removeAllRanges(); 'ok'" >/dev/null
  step "пустое место — только «Select all»" ctx 600 400 'меню: Select all$'
  check "лаунчер жив" kill -0 "$LPID"
  drv eval "const r = document.createRange(); r.selectNodeContents(document.getElementById('t')); getSelection().removeAllRanges(); getSelection().addRange(r); 'ok'" >/dev/null
  step "выделенный текст — «Copy»" ctx 60 30 'меню: Copy, \|, Select all$'
  check "лаунчер жив" kill -0 "$LPID"
  drv eval "const i = document.getElementById('i'); i.focus(); i.select(); 'ok'" >/dev/null
  step "поле ввода с выделением — Cut / Copy / Paste" ctx 100 95 'меню: Cut, Copy, Paste( \(off\))?, \|, Select all$'
  check "лаунчер жив" kill -0 "$LPID"
  drv eval "getSelection().removeAllRanges(); document.activeElement.blur(); 'ok'" >/dev/null
  step "ссылка — «Copy link address»" ctx 40 150 'меню: Copy link address, \|, Select all$'
  check "лаунчер жив" kill -0 "$LPID"
  kill -TERM "$LPID"
  step "лаунчер штатно завершился после контекстных меню" wait_exit "$LPID" 15
  check "дерево dsh остановлено" tree_gone
  ;;

*) echo "неизвестный сценарий"; exit 2 ;;
esac

if [ "$FAILS" -ne 0 ]; then
  echo "  --- хвост лога лаунчера ---"; tail -25 "$OUT" | sed 's/^/  | /'
fi
echo "=== $NAME: $([ "$FAILS" -eq 0 ] && echo PASS || echo "FAIL ($FAILS)") ==="
exit "$FAILS"
