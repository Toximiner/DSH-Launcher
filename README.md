<p align="center"><img src="icon.png" width="128" alt="dsh-launcher"></p>

# dsh-launcher

**Русский** | [English](README.en.md)

Небольшое Electron-приложение: окно, которое открывает **DeepSeek Harness**
(`dsh web`) и ведёт его процесс заодно.

## Как работает

- **Клик по ярлыку «DeepSeek Harness»**:
  - если порт `3080` не слушается — лаунчер сам запускает
    `dsh --profile web --no-open`, подхватывает строку
    `dsh web: http://127.0.0.1:3080/?token=…` из stdout dsh и открывает
    её в окне (сервер выдаёт сессионный куки, он запоминается в профиле
    приложения и обычно больше не нужен);
  - если dsh **уже запущен** (например, из терминала) — окно подключается
    к нему. Куки, если он ещё валиден, подхватывается автоматически;
    иначе окно попросит вставить URL с токеном из того терминала, где
    работает dsh.
- **Плагин маркета.** Перед тем как запустить dsh, лаунчер проверяет, что в
  профиле (`$DSH_HOME/profiles/<профиль>/package.json`, по умолчанию
  `~/.dsh`) стоит плагин `dshmarket`. Если нет — окно спрашивает:
  «Установить» (выполняет `dsh plugin --profile web add dshmarket` с живым
  логом), «Не сейчас» или «Не спрашивать больше». Отказ навсегда хранится в
  `~/.config/dsh-launcher/market-prompt.json` — удалите файл, чтобы вопрос
  вернулся. К уже запущенному dsh вопрос не задаётся.
- **Закрытие окна** → весь процесс dsh останавливается (SIGTERM,
  через 4 с — SIGKILL) — но **только если процесс запустил именно лаунчер**.
  Чужой (запущенный вручную в терминале) dsh не трогается.

## Запуск из терминала

Из каталога проекта:

```sh
npm install          # только первый раз
npx install-electron # Electron 44+ не скачивает бинарник автоматически
npm start
```

## Настройка

Константы в самом верху `main.js`, либо переменные окружения:

| Переменная              | По умолчанию             | Что делает                    |
|-------------------------|--------------------------|-------------------------------|
| `DSH_BIN`               | поиск (см. ниже)         | путь к бинарнику dsh          |
| `DSH_ARGS`              | `--profile web --no-open`| аргументы запуска (как в shell: пробелы, `"…"`, `'…'`, `\`) |
| `DSH_CWD`               | домашний каталог         | рабочая директория dsh        |
| `DSH_PORT`              | `3080`                   | порт web-интерфейса           |
| `DSH_START_TIMEOUT_MS`  | `120000`                 | сколько ждать старта, мс      |
| `DSH_LAUNCHER_LANG`     | выбор в окне / локаль    | язык окон лаунчера: `ru` или `en` |

Язык служебных окон лаунчера (запуск, установка dsh, вопрос про маркет,
ошибки) переключается в самом окне — **RU | EN** в правом верхнем углу;
выбор запоминается в `~/.config/dsh-launcher/ui-lang.json`. Пока выбора не
было — язык по локали системы (`LC_ALL` / `LC_MESSAGES` / `LANG`): `ru*` —
русский, иначе английский. `DSH_LAUNCHER_LANG=ru|en` задаёт язык
принудительно и перекрывает сохранённый выбор.

Если `DSH_BIN` не задан, лаунчер ищет dsh сам: `/usr/bin/dsh`, `PATH`,
`~/.npm-global/bin`, `~/.local/bin`, `/usr/local/bin`, все версии nvm
(`~/.nvm/versions/node/*/bin`) и `$(npm prefix -g)/bin`. Найденный путь
запоминается в `~/.config/dsh-launcher/dsh-bin.json`. dsh запускается с
своим каталогом в начале `PATH` — так установленный через nvm dsh получает
«свой» node и при запуске с ярлыка.

Журнал процесса dsh пишется в `logs/dsh.log`.

## Клавиши в окне

Меню у окна отключено, поэтому управление — клавишами, как в браузере
(работают при любой раскладке — RU/EN):

| Клавиши             | Действие                    |
|---------------------|-----------------------------|
| `F5` / `Ctrl+R`     | перезагрузить страницу      |
| `Ctrl+Shift+R`      | перезагрузить без кэша      |
| `Ctrl+=` / `Ctrl+-` | шаг масштаба страницы       |
| `Ctrl+0`            | сброс масштаба              |

Масштаб сохраняется между запусками.

## Ярлык (если ещё не поставлен)

Готового desktop-файла для dev-ярлыка в репозитории **нет**
(`deb/dsh.desktop` — шаблон для .deb-пакета): файл создаётся локально,
потому что содержит абсолютные пути вашей машины:

```sh
# из каталога проекта:
chmod +x run.sh
cat > dsh.desktop <<EOF
[Desktop Entry]
Type=Application
Version=1.0
Name=DeepSeek Harness
GenericName=AI Chat
Comment=Launcher for DeepSeek Harness (dsh --profile web)
Exec=$PWD/run.sh
Icon=$PWD/icon.png
Terminal=false
Categories=Development;Utility;
Keywords=dsh;deepseek;ai;chat;harness;
StartupWMClass=dsh-launcher
EOF
cp dsh.desktop ~/.local/share/applications/      # пункт в меню приложений
cp dsh.desktop ~/Desktop/ && chmod +x ~/Desktop/dsh.desktop   # иконка на рабочем столе
# GNOME: на иконке на рабочем столе — ПКМ → «Allow Launching»
```

## Пакет для Ubuntu 26.04 (.deb)

Готовые пакеты публикуются в **Releases** репозитория на GitHub —
проще всего скачать свежий `.deb` оттуда и установить. Что изменилось в
каждой версии — в [CHANGELOG.md](CHANGELOG.md).

### Сборка самостоятельно

Нужны (только на **машине сборки**): Ubuntu/Debian amd64 с `dpkg-deb`
(пакет `dpkg-dev`), Node.js 22.12+ (требование Electron 44) и доступ в
интернет (скачается бинарник Electron ~290 МБ). На целевой машине node/npm
**не нужны** — самодостаточный пакет, Electron входит внутрь.

```sh
git clone https://github.com/Toximiner/DSH-Launcher.git
cd DSH-Launcher
npm install            # только на машине сборки
npx install-electron   # Electron 44+ не скачивает бинарник автоматически
./build-deb.sh         # ~2 минуты (xz-сжатие) → dsh-launcher_<версия>-<ревизия>_amd64.deb

sudo dpkg -i dsh-launcher_*_amd64.deb
# либо: sudo apt install ./dsh-launcher_*_amd64.deb
```

Версия пакета задаётся **в одном месте** — `Version:` в `deb/control`
(формат `X.Y.Z-R`, например `1.0.0-9`); `build-deb.sh` читает её
автоматически. Для локальной сборки новой ревизии меняйте только этот
файл.

Устанавливается:

- `/opt/dsh-launcher/` — приложение и рантайм Electron;
- `/usr/bin/dsh-launcher` — команда запуска;
- `/usr/share/applications/dsh-launcher.desktop` — пункт меню «DeepSeek Harness»;
- `/usr/share/icons/hicolor/256x256/apps/dsh-launcher.png` — иконка.

`postinst` (выполняется от root при установке) настраивает SUID-хелпер
sandbox Chromium (`chown root:root` + `chmod 4755 chrome-sandbox`); в этом
случае `run.sh` запускается **без** `--no-sandbox`, а если хелпер не удалось
настроить — автоматически откатывается на `--no-sandbox`.

Журнал dsh в установленной версии: `~/.local/state/dsh-launcher/dsh.log`.

> **Внимание.** Если раньше вручную ставили ярлык
> (`~/.local/share/applications/dsh-launcher.desktop` и/или иконку на
> рабочем столе) — он перекрывает системный (пользовательские файлы
> имеют приоритет). После установки пакета удалите его:
>
> ```sh
> rm ~/.local/share/applications/dsh-launcher.desktop
> rm ~/Desktop/dsh-launcher.desktop 2>/dev/null
> ```

## Про `--no-sandbox`

`run.sh` запускает Electron с `--no-sandbox`: на системах Ubuntu, где
непривилегированные user-namespace ограничены политикой, SUID-хелпер
Chromium без `sudo` не настроишь. Для локального доверенного
интерфейса это нормально. Если хотите включить sandbox Chromium:

```sh
# из каталога проекта:
sudo chown root:root node_modules/electron/dist/chrome-sandbox
sudo chmod 4755 node_modules/electron/dist/chrome-sandbox
```

и уберите `--no-sandbox` из последней строки `run.sh`.

## Если что-то пошло не так

- Если dsh не найден (или `DSH_BIN` указывает на отсутствующий файл),
  окно предложит его установить: команда `npm install -g @deepseek-ai/dsh`
  с кнопкой «Скопировать», а если глобальный npm prefix доступен без sudo —
  кнопку «Установить» (ставит прямо из окна, с логом). «Проверить ещё раз»
  продолжает запуск, когда dsh появился.
- Если dsh был перезапущен (например, после обновления плагина), окно
  может показывать «сессия завершена». Лаунчер следит за портом и сам
  перезагружает страницу, когда dsh снова отвечает (обычно через 2–3 с).
  Если ждать не хочется — просто `F5` в окне: куки, сессии и состояние
  интерфейса сохраняются на диске (`~/.dsh/.credentials.yaml`,
  `~/.dsh/sessions/`), поэтому разговор восстанавливается без потерь.
- Окно показывает страницу с ошибкой и последними строками журнала dsh.
- Полный лог: `logs/dsh.log`.
- Проверьте, что в терминале команда `dsh --profile web --no-open`
  поднимается и печатает URL.
- Куки живёт в профиле Electron (`~/.config/dsh-launcher`); если хотите
  «забыть» авторизацию — закройте окно (dsh остановится) и удалите этот
  каталог.
