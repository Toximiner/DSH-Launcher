# Каталог действий TUI для палитры команд (Ctrl+P)

Preview `opencode.4`: источник описаний — `apps/tui-go/actions.go`. Из него строятся палитра, точные slash-команды без аргументов, основные сочетания и строки справочника; локальные клавиши редакторов остаются в `shortcuts.go`. Быстрый выбор сессий/областей отделён от полного управления (`/sessions-manager`, `/workspaces-manager`).


Основа для реализации единой палитры в стиле opencode: верхнее меню «Файл/Вид/Справка» и
Alt+*/F-навигация удаляются; всё достижимо из палитры Ctrl+P. Репо: apps/tui-go (package main).
Все ссылки — file:line. Решение пользователя: «чистим клавиатуру под новую навигацию» — раздел 3.

Общие гейты (используются в столбце «Блокировки»):
- Нет URL входа (m.settingsLogin==""): «Укажите URL входа dsh через --url или DSH_TUI_URL» session.go:281-284; market.go:240-244; session_archive.go:26-29.
- Нет подключённой сессии (операции кроме new/connect/list): «Сначала подключите сессию» session.go:295-298; openWorkbench «Подключите сессию dsh» workbench.go:99-101; openHistory/goal/compose/models — свои (history.go:69-72; goal_dialog.go:73-76; compose.go:43-45; models.go:114-117).
- Архивная сессия: «Сессия в архиве. /restore — восстановить» session.go:299-301; workbench.go:935-938.
- Busy: очередь запросов ≤8 session.go:285-293.

## 1. Каталог действий

Формат: Действие | Текущий триггер | Обработчик file:line | Нужна сессия | Блокировки.

### 1.1 Сессии
| Действие | Триггер | Обработчик file:line | Сессия | Блокировки |
|---|---|---|---|---|
| Новая сессия | /new; «⊕ Новая сессия» sidebar; Ctrl+N/Enter в «Рабочих областях» | sessionOperation("new","") session.go:280; main.go:720-721; sidebar.go:313-315; workbench.go:776-778 | Нет (URL) | busy→очередь; workspaceId — активная область sidebar.go:458-473 |
| Подключить сессию | Enter в «Сессии»; /session <id>; клик по строке sidebar | sessionOperation("connect",id) session.go:280; main.go:680-685, 773-775; sidebar.go:316-328 | Нет (URL) | chatSessionChanging — «Дождитесь подключения» |
| Список сессий (стр.) | Alt+S; /sessions; «Сессии» в «Ещё» sidebar | page=sessionsPage main.go:610, 722-725; sidebar.go:345-357 | Нет (URL) | без URL — «Сессии dsh недоступны» session.go:450-452; r — обновить main.go:645-648 |
| Переименовать текущую | /rename; «Переименовать…» [···] чата; F3 в списке | openSessionRename(true) session_rename.go:27; main.go:746-747; conversation_view.go:43; F3 — session_rename.go:56-59 | Да (или выбор) | нет URL 28-31; пустое — «Введите непустое название» 77-80 |
| Ветвление (fork) | /fork; «Создать ветку…» [···]; Ctrl+B в списке | openSessionFork(true) session_fork.go:29; main.go:744-745; conversation_view.go:44; Ctrl+B — session_fork.go:56-59 | Да (или выбор) | нет URL 30-33; подключение 34-37; без завершённых ходов (forkView 136-139) |
| Архивировать | /archive; «Архивировать…» [···]; Ctrl+X в «Сессии»; a — фильтр | openSessionArchive(true,false) session_archive.go:25; main.go:740-741; conversation_view.go:44; Ctrl+X — 55-58; a — 59-65 | Да (или выбор) | нет URL 26-29; работающие задачи могут запретить 127 |
| Восстановить из архива | /restore; a в архиве | openSessionArchive(true,true) session_archive.go:25; main.go:742-743 | Нет (URL) | как архивирование |
| К родителю (подагент) | /parent | main.go:752-757 → sessionOperation("connect",ParentID) | Да, Address.Kind=="subagent" | иначе — «нет адреса родителя» main.go:755-756 |
| Режим обычный/план/цель | F2; Alt+1/2/3; /plan [off], /mode <m> | main.go:652-656, 657-662, 726-729, 776-783 → chooseMode session.go:385-398 → session.go:209-278 | Да | подагент — «Режим подагента задаётся родителем» session.go:214-216; goal без поддержки 241-243; goal вставляет «/goal » в ввод 386-395 |

### 1.2 Навигация (страницы и пункты меню)
Страницы: константы main.go:22-31, подписи pages main.go:33.
| Страница | Константа | Текущий триггер | Переход file:line | Сессия | Блокировки |
|---|---|---|---|---|---|
| Чат | chatPage=0 | Esc; Alt+C; «Чат» в [···] | showChat main.go:243-249; main.go:678-679, 610; conversation_view.go:43 | — | — |
| Сессии | sessionsPage | Alt+S; /sessions; «Сессии» | main.go:610, 722-725; sidebar.go:345-357 | Нет (URL) | см. 1.1 |
| Настройки dsh | dshSettingsPage | Alt+D; «Вид→Настройки dsh»; «Ещё» | main.go:610; chrome.go:48, 183-184; sidebar.go:158, 348-349 | Нет (URL) | без URL settings.go:86-88; ←/→ раздел main.go:634-644; r — перечитать main.go:649-651 |
| Настройки TUI | tuiSettingsPage | — | main.go:26, 33 — **мёртвая константа**, не referenced (grep); хаб — диалог «settings» chrome.go:72-78 | — | — |
| Журнал | logPage | Alt+L; «Справка→Журнал клиента» | main.go:610; chrome.go:49, 185-186 | — | содержимое — m.events (runtime_ui.go:210-217) |
| Маркет | marketPage | Alt+M; /market; sidebar; таб хаба | main.go:610-625, 714-719; sidebar.go:179, 343-354; settings_hub.go:39, 116-121 | Нет (URL) | без URL market.go:240-244; маркет занят 190-192 |
| Плагины | pluginsPage | sidebar «+ Плагины»; /plugins; таб хаба | sidebar.go:178, 359-364; main.go:760-761; settings_hub.go:31, 157-165 | Да | workbench.go:99-101 |
| Автоматизация | schedulesPage | sidebar «◷ Автоматизация»; /schedules | sidebar.go:180, 359-364; main.go:760-761 → openWorkbench pageView workbench.go:114-120 | Да | workbench.go:99-101 |

Верхнее меню (chrome.go:40-50): открывается F10/Alt+F/V/H chrome.go:238-254; активация
chrome.go:170-195; отрисовка chrome.go:329-341, 408-414. Пункты — будущие items палитры:
| Метка RU (меню) | Действие file:line | Сессия | Блокировки |
|---|---|---|---|
| Настройки TUI… (F4) — Файл | openDialog("settings") chrome.go:47, 173-174, 72-78 | Нет | m.settings==nil → loadSettings chrome.go:75-77 |
| Установка и dsh… — Файл | openRuntime("runtime") chrome.go:47, 175-177; runtime_ui.go:79-90 | Нет | m.runtime==nil — «нативный Linux-клиент» runtime_ui.go:80-83 |
| Выйти (Ctrl+D) — Файл | tea.Quit chrome.go:47, 178-179 | Нет | — |
| Настройки dsh — Вид | page=dshSettingsPage chrome.go:48, 183-184 | Нет (URL) | — |
| Горячие клавиши — Справка | openDialog("help") chrome.go:49, 173-174; F1 — chrome.go:232-237 | Нет | — |
| Обновления и откат… — Справка | openRuntime("updates") chrome.go:49, 175-177 | Нет | runtime_ui.go:80-83; stop/restart — «Нет своего процесса» 184-189 |
| Журнал клиента — Справка | page=logPage chrome.go:49, 185-186 | Нет | — |
| Диагностика… — Справка | openRuntime("diagnostics") chrome.go:49, 175-177 | Нет | runtime_ui.go:80-83 |
| О программе — Справка | openDialog("about") chrome.go:49, 173-174, 378-379 | Нет | — |

Боковая панель (удаляется как навигация, действия переносятся в палитру): пункты
sidebar.go:176-183 (new, plugins, market, schedules, navigation, workspaces), «⋯ Ещё»
sidebar.go:231; sidebarTools sidebar.go:152-156 (Сессии, Файлы, Терминалы, Пресеты, Команды,
Пакеты, Подагенты, Задачи команды, Задания); sidebarMore sidebar.go:157-161 (Настройки TUI,
Настройки dsh, Аккаунт DeepSeek, Горячие клавиши, Диагностика, О программе); контекстное
меню чата conversationMenu conversation_view.go:42-45 (История, Переименовать, Ветка,
Архивировать); активация sidebar.go:303-366, 445-456; клавиши sidebar.go:367-414
(F5 — обновить 408-409; Ctrl+F — поиск рабочих областей 410-411).

### 1.3 Настройки
| Действие | Триггер | Обработчик file:line | Сессия | Блокировки |
|---|---|---|---|---|
| Хаб «Настройки TUI» (5 вкладок settingsTabs settings_hub.go:12) | F4/Alt+T; меню Файл; «Ещё» | openDialog("settings") chrome.go:226-231, 72-78; sidebar.go:158, 333-334 | Нет | box 112×34 settings_hub.go:212; навигация settingsHubKey 170-208 |
| ├ Доступ по умолчанию (Общие) | — | settingsHubAction("permission-default") settings_hub.go:73, 128-156 | Нет (URL) | loading/nil → «Дождитесь снимка» 129-132; !Writable — «только для чтения» 133-136 |
| ├ Язык (Общие) | — | togglePreference(6) settings_hub.go:74, 104-105 → chrome.go:119-125 | Нет | сохранение prefs chrome.go:126-143 |
| ├ Оформление (Общие) | — | togglePreference(4) settings_hub.go:75, 100-101 → chrome.go:112-116 | Нет | — |
| ├ Размер шрифта (Общие) | — | settingsHubAction("font") settings_hub.go:76, 106-108 | Нет | заметка «задаётся в терминале» |
| ├ Горячие клавиши (Общие) | — | settingsHubAction("shortcuts") settings_hub.go:77, 94-95 → settingsHelp chrome.go:84-88 | Нет | — |
| ├ Мышь (Общие) | — | togglePreference(0) settings_hub.go:78, 96-97 → chrome.go:103-109 | Нет | --no-mouse — «отключена флагом» chrome.go:104-107 |
| ├ История ввода (Общие) | — | togglePreference(1) settings_hub.go:79, 98-99 → chrome.go:109-111 | Нет | выключено — история очищается chrome.go:156-160 |
| ├ Боковая панель (Общие) | — | togglePreference(5) settings_hub.go:80, 102-103 → chrome.go:117-118 | Нет | — |
| ├ Модель сессии (Модели) | — | settingsHubAction("models") settings_hub.go:26, 157-164 → openModels models.go:113 | Да | «подключитесь к сессии dsh» settings_hub.go:158-160 |
| ├ Провайдеры (Модели) | — | settingsHubAction("providers") settings_hub.go:27, 128-138 → openSettingsForm(true) | Нет (URL) | gate «для записи» settings_form.go:61-64 |
| ├ Встроенные плагины / Команды (Встроенные плагины) | — | openWorkbench("plugins"/"commands") settings_hub.go:31, 33, 157-165 | Да | settings_hub.go:158-160 |
| ├ Параметры плагинов (Встроенные плагины) | — | settingsHubAction("config") settings_hub.go:32, 109-115 → dshSettingsPage+loadSettings | Нет (URL) | — |
| ├ Пресеты агента (Пресеты) | — | openWorkbench("presets") settings_hub.go:36, 157-165 | Да | settings_hub.go:158-160 |
| ├ Маркет / Пакеты (Маркет) | — | settingsHubAction("market") 116-121; openWorkbench("packages") settings_hub.go:40, 157-165 | Нет (URL) / Да | — |
| Настройки dsh (страница) | Alt+D; «Вид→Настройки dsh» | page=dshSettingsPage chrome.go:183-184; view settings.go:85-111 | Нет (URL) | без URL settings.go:86-88; ошибка — «r — повторить» 93-95 |
| Форма dsh / Провайдеры | Enter на странице; F6; /providers; «Провайдеры» хаба | openSettingsForm settings_form.go:60-101; main.go:748-749; settings_form_ui.go:17-22 | Нет (URL) | settings==nil/!Writable/loading 61-64; нет формы 78-81; F5 — сохранить settings_form_ui.go:154-155; Ctrl+R — перечитать 156-157; Ctrl+N/K/X/U 139-153 |
| Горячие клавиши (справка) | F1; «Справка→…» | openDialog("help") chrome.go:232-237, 49; view shortcuts_view.go:165-168 | Нет | box 100×34 shortcuts_view.go:105-108; данные tuiShortcuts shortcuts.go:6-74; группы shortcuts_view.go:18-26; Ctrl+U — сброс chrome.go:201-203 |

### 1.4 Плагины и маркет
| Действие | Триггер | Обработчик file:line | Сессия | Блокировки |
|---|---|---|---|---|
| Страница «Плагины dsh» | sidebar; /plugins; таб хаба | openWorkbench("plugins") workbench.go:98; content workbench.go:1216-1223 | Да | workbench.go:99-101 |
| ├ Установить пакет | Ctrl+N | benchPrepare KeyCtrlN workbench.go:825-833 → benchExecute 492-693 | Да | «Управление недоступно в профиле» 798-800; npm@ver или путь 1044-1051 |
| ├ Включить/выключить | Space | workbench.go:839-845 → pluginManager/setBundleEnabled | Да | readOnlyReason 835-837; preflight 575-577 |
| ├ Обновить / Удалить | F3 / Ctrl+X | workbench.go:850-853 / 847-848 | Да | updatable==true / removable==true |
| ├ Отменить установку | Ctrl+X при busy | cancelPackageOperation workbench.go:970-971 (package_cancel.go:17) | Да | packageRequestID |
| ├ Детали / панель плагина | Enter | workbench.go:811-823; builtin → openPluginSettings plugins.go:167 (workbench.go:805-807) | Да | «Компоненты только для чтения» 816-817 |
| ├ Компоненты/установленные · скрытые · маркет | F2 · Ctrl+O · Right/F6 | plugins.go:368-371 · 373-384 · 386-392 → marketPage | Нет (URL) | — |
| Маркет (страница) | Alt+M; /market; sidebar; таб хаба | page=marketPage main.go:610, 714-719; loadMarket market.go:236-261 | Нет (URL) | без URL 240-244; «установите dshmarket» 116 |
| ├ Установить / Обновить / Удалить | i / u / d | market.go:407-415 → confirmMarket 291-308 → executeMarket 309-332 → runMarketIntent 182-234 | Нет (URL) | подтверждение y/n 349-358; NPM-https 206-208; уже установлен 203-205; не установлен 211-213; занят 190-192 |
| ├ Поиск / Обновить / Только установленные | / / r / t | market.go:380-391 | Нет (URL) | — |
| Пакеты dsh | /packages; «Установленные пакеты» хаба | openWorkbench("packages") main.go:760-761; read workbench.go:224-230 | Да | — |
| ├ Версии пакета / Разрешить скрипты | F6 / F2 (pendingBuilds) | workbench.go:1132-1143 (read 203-222) / 1120-1131 | Да | updatable 1134-1139; несовместимая 786-788 / только при pendingBuilds 1121 |

### 1.5 Автоматизация
| Действие | Триггер | Обработчик file:line | Сессия | Блокировки |
|---|---|---|---|---|
| Расписания (стр., весь Host) | sidebar «◷»; /schedules | openWorkbench("schedules") pageView workbench.go:114-120; read 327-341; layout schedules_view.go:100-172 | Да | workbench.go:99-101 |
| ├ Создать / История / Изменить | Ctrl+N / Enter / F3 | workbench.go:873-877 → scheduleForm schedule_form.go:23-41 / 879-882 / 891-894 | Да | F2 → JSON schedule_form.go:53-69; F5 — валидация 1–120 симв./инструкция 97-120 / нет ревизии 884-887 |
| ├ Пауза / Запустить / Удалить / Фильтр | Space / Ctrl+G / Ctrl+X / F2 | workbench.go:898-900 / 889-890 / 895-897 / 1007-1011 (scheduleStatuses schedules_view.go:13) | Да | — |
| Задачи команды | /tasks; «Задачи команды» | openWorkbench("tasks") main.go:760-761; read workbench.go:351-356 | Да | workbench.go:99-101 |
| Фоновые задания (jobs) | /jobs; «Задания» | openWorkbench("jobs"); read workbench.go:303-326 | Да | чужое задание — «F5 перечитать» 317-319 |
| ├ Снимок вывода / Остановить | Enter / Ctrl+X | workbench.go:713-720 / 902-907 (job/kill) | Да | — |
| Подагенты | /agents; «Подагенты» | openWorkbench("agents"); read workbench.go:346-350 | Да | — |
| ├ Подключить / Сообщение в очередь / Остановить | Enter / F3 / Ctrl+X (continuable) | workbench.go:722-728 / 909-917 (subagents/prompt) / 919-921 | Да | external/unknown 723-726 / mode!=continuable |
| Терминалы | /terminals; «Терминалы» | openWorkbench("terminals"); read workbench.go:358-365 | Да | — |
| ├ Открыть / Запустить оболочку / Закрыть | Enter / Ctrl+N / Ctrl+X | workbench.go:730-732 → startBenchTerminal terminal_bridge_native.go:229 / 922-929 / 930-932 | Да | Ctrl+] — вернуться (shortcuts.go:48) |
| Рабочие области | /workspaces; sidebar; Ctrl+F в sidebar | openWorkbench("workspaces"); read workbench.go:190-202; sidebar.go:410-411 | Да | лимит каталога 287-289 |
| ├ Новая сессия здесь / в другой папке | Ctrl+N·Enter / F3 | workbench.go:775-778 / 779-783 | Да | — |
| Аккаунт DeepSeek | /account; «Ещё» | openWorkbench("account"); readAccount account.go:20-45 | Нет (URL) | неподдерживаемое состояние 26-28 |
| ├ Войти / Выйти / Отменить вход / Профиль | Ctrl+N / Ctrl+X / Ctrl+G / F3 | workbench.go:753-761 / 762-763 / 764-770 / 771-772 (account.go:55-64) | Нет (URL) | localhost account.go:33, 755-758 / running tasks 565-570 / нет попытки 765-769 |
| Журнал операций пакетов | /operations | openWorkbench("operations"); read workbench.go:189 (packageReceipts) | Да | — |
| Файлы рабочей области | /files; «Файлы» | openWorkbench("files"); read workbench.go:266-302 | Да | превью ≤6 МиБ documents.go:44-46 |

### 1.6 Действия (чат, агент, сессия)
| Действие | Триггер | Обработчик file:line | Сессия | Блокировки |
|---|---|---|---|---|
| Остановить агента | F8 (глоб.); /stop | main.go:522-523, 572-574; approvals.go:239; queue_ui.go:170; main.go:766-767 → cancelChat chat.go:312-352 | Да | смена сессии 313-316; нет сессии 317-320; подагент non-continuable 323-326 |
| Повторить неопределённую отправку | /retry-send | main.go:768-769 → retryPrompt chat.go:293-310 | Да | нет prompt "unknown" 302-307 |
| Очередь сообщений (toggle) | F9; /queue | queue_ui.go:141-149; main.go:738-739 → openQueue 34-43 | Да | queueAvailable 58-60 (QueueKnown, !stale, chat ready) |
| ├ Удалить / Редактировать / Уточнить | Enter / F3 / Ctrl+G в окне | queue_ui.go:204-206 → 61-70 → 72-112 / 202-203 → queue_edit.go:31+ / 200-201 → queue_steer.go:17-40, 41+ | Да | подтверждение 196-199 / текст ≤64 КиБ queue_edit.go:37-50 / только при работе агента queue_steer.go:22-25, 30-33 |
| Многострочный редактор | /compose; Alt+E; «+» лендинг | main.go:758-759; agent_landing.go:246, 177 → openCompose compose.go:38-69 | Да | подагент 39-42; F5 — отправить compose.go:161 |
| Прикрепить файл | /attach <путь> | main.go:784-786 → uploadCompose compose.go:70-114 | Да | ≤4 вложений 75; файл до 2 МиБ 110 |
| Очистить экран | /clear | main.go:732-737 (lines=nil; clearedThrough=cursor) | — | — |
| История сессии (toggle) | F11 (чат); /history | main.go:569 → historyKey history.go:370-372; main.go:764-765 → openHistory 65-80 (toggle 66-68) | Да | chat ready+сессия 69-72 |
| ├ Поиск в истории | Ctrl+F | historySearchKey history_search.go:284-286 → editHistorySearch 57 | Да | Ctrl+T — область 301-303; F3 — ещё/начать 289-291 |
| ├ Ветка от сообщения | Ctrl+B (выбрано) | history.go:412-413 → forkHistorySelection history_fork.go:76 | Да | только сообщения-границы history_fork.go:12-15 |
| ├ Копировать / Код / Markdown-исходник | Ctrl+Y / Ctrl+K / Ctrl+R | history.go:384-388 / 389-392 → dialog history_copy.go:168-193 / 394-406 | Да | — |
| ├ Выбор / Раньше-позже-свежая / Инструмент | Tab·Shift+Tab / Left·Right·F5 / F6·Shift+F6·Enter | history.go:407-411 / 414-425 / historyToolKey history_tools.go:148-163 | Да | — |
| Инструменты в чате (раскрытие) | F6/Shift+F6; Enter; Esc — ввод | toolKey tools.go:165-184 (main.go:584) | Да | нет инструментов 172-174 |
| Копировать последний ответ | Ctrl+Y (чат) | conversationKey conversation_view.go:276-288 → copyConversation 237-264 | Да | фрагмент ≤64 КиБ 250-252 |
| Модель и рассуждение | F12 (чат/настройки); /models | main.go:548 → modelsKey models.go:306-311; main.go:750-751 → openModels 113-127 | Да | 114-117; F5 — каталог 330-331; Tab/Shift+Tab — уровень 344-359; Enter — применить 332-333 |
| Цель | /goal; /goal <цель>; /goal pause|resume|clear | main.go:762-763 → openGoal goal_dialog.go:72-82; main.go:787-795 → session.go:209-278 | Да | goal_dialog.go:73-76; F3 — правка goal_dialog_ui.go:46-52; F5 — сохранить 53-57; Ctrl+R — перечитать 44-45; Ctrl+B — конфликт 58-81 |
| Пресеты агента | Alt+A; /presets; «Пресеты» | agent_landing.go:246; main.go:760-761 → workbench "presets" read workbench.go:232-254 | Да | Enter — выбрать (до 1-го хода) 862-863; Ctrl+N — новая сессия 860-861; broken 856-858 |
| Провайдеры | /providers; F6 на «Настройки dsh»; хаб | main.go:748-749 → openSettingsForm(true) settings_form.go:60-101; settings_form_ui.go:20-22 | Нет (URL) | settings_form.go:61-81 |
| Доступ (permission) | Alt+P; «Доступ по умолчанию» хаба | agent_landing.go:246 → workbench "permissions" read workbench.go:185; Enter — /permission 739-748 | Да | preflight 572-574 |
| Вопросы и разрешения (toggle) | F7 | approvalKey approvals.go:219-266 (F7 224-229) | Да (есть запрос) | без запросов F7 мимо 225-228; Enter — ответить 260-261; F5 — отправить вопросы questions.go:231-232 (answerQuestion 182) |
| Отправить сообщение | Enter (комposer) | main.go:699+ → submitText chat.go:202 | — | «/» — диспетчер main.go:711-800 |
| Выход | Ctrl+D / Ctrl+C | main.go:663-664; chrome.go:223-224; runtime_ui.go:105-107 | Нет | — |

### 1.7 Команды (slash-диспетчер main.go:711-800; текст help — main.go:20)
Каждая команда — item палитры группы «Команды»; <…> — аргумент. Сгруппировано по обработчику.
| Команды | Описание RU (кратко) | Диспетчер file:line → обработчик |
|---|---|---|
| /runtime · /updates · /diagnostics | установка/процесс dsh · обновления и откат dsh/TUI · журнал, отчёт, уведомления | main.go:712-713 → openRuntime(kind) runtime_ui.go:79 (действия: runtimeActions 42-52, updateActions 53-60, diagnosticActions 61-65) |
| /market | маркет плагинов | main.go:714-719 → marketPage+loadMarket |
| /new | новая сессия | main.go:720-721 → sessionOperation("new") |
| /sessions | список сессий | main.go:722-725 → sessionsPage+list |
| /session <id> | подключить сессию | main.go:773-775 → sessionOperation("connect",id) |
| /rename | название текущей · F3 в списке | main.go:746-747 → openSessionRename(true) |
| /archive · /restore | архив текущей · возврат из архива | main.go:740-743 → openSessionArchive(true,false/true) |
| /fork | новая ветка · Ctrl+B в списке | main.go:744-745 → openSessionFork(true) |
| /models · F12 | выбор модели сессии | main.go:750-751 → openModels() |
| /providers | провайдеры · F6 — форма | main.go:748-749 → openSettingsForm(true) |
| /plan · /plan off · /mode default|plan|goal | режим плана вкл/выкл · режим сессии (невалидно — «Режим: default, plan или goal») | main.go:726-729, 776-783 → chooseMode |
| /goal · /goal <цель>|pause|resume|clear | состояние цели (F3 правка) · работа с целью | main.go:762-763 → openGoal(); main.go:787-795 → sessionOperation |
| /stop · F8 | остановить агента | main.go:766-767 → cancelChat() |
| /retry-send | повторить неопределённую отправку | main.go:768-769 → retryPrompt() |
| /history · F11 | история сессии (Ctrl+F поиск, F3 ещё, Ctrl+T область, Tab выбор, Ctrl+B ветка, Ctrl+R исходник, Ctrl+Y копия, Ctrl+K код, F6 инструмент) | main.go:764-765 → openHistory() |
| /queue · F9 | очередь (F3 правка, Ctrl+G уточнить) | main.go:738-739 → openQueue() |
| /compose · /attach <путь> | многострочное сообщение · файл до 2 МиБ | main.go:758-759 → openCompose(); main.go:784-786 → uploadCompose(path) |
| /workspaces · /files · /terminals · /presets · /plugins · /commands · /packages · /agents · /tasks · /jobs · /schedules · /operations · /account | рабочие области · файлы · терминалы · пресеты (Ctrl+N новая сессия) · плагины · команды агента · пакеты · подагенты · задачи команды · задания · расписания · журнал операций · аккаунт | main.go:760-761 → openWorkbench(trim) workbench.go:98 |
| /parent | вернуться от подагента к родителю | main.go:752-757 |
| /clear · /help · /exit,/quit | очистить экран · справка · выйти | main.go:732-737; 770-771; 730-731 |
Прочие «/…» — «Неизвестная команда. /help — список команд» main.go:796-799.

## 2. Спецификация палитры (Ctrl+P)

### 2.1 Размер диалога
Medium (88×24) из internal/component/dialog.go:20-33 (sizeWidths/sizeHeights 32-33):
BodyRows = 24-4-3 = 17 (BodyRows dialog.go:72-81) минус футер (2 строки) ≈ 15 строк списка —
хватает для ~40 items со скроллом; Small (60) тесен для «метка + hint», Large (116) избыточен.
Clamping — Width/Height dialog.go:50-65; позиция Box 88-101; шаблон Layout 106-136 (muted
«Esc» справа 110-112); рендер Render 150-152.

### 2.2 Поиск
- Простое префиксное, регистронезависимое, по RU-метке — как opencode (plain prefix):
  strings.HasPrefix(strings.ToLower(label), strings.ToLower(query)).
- Фолбэк: подстрока strings.Contains — только если префиксных совпадений нет (без fuzzy-скоринга).
- Поле — bubbles/textinput (прецеденты: session_search.go:25-27; market.go:567-571; workbench.go:107-109).
- Ctrl+U — сброс (chrome.go:201-202); Esc — из поиска, затем из диалога.

### 2.3 Группировка и отрисовка
- Плоский список: строки-заголовки групп + строки items; порядок групп фиксирован:
  Сессии, Навигация, Настройки, Плагины и маркет, Автоматизация, Действия, Команды.
- Заголовок: muted() (theme.go:11), «── Сессии ──» (прецедент shortcutHeading shortcuts_view.go:48-50).
  НЕ sticky — скроллятся как список; при скролле в середину группы можно дорисовать заголовок
  (прецедент shortcuts_view.go:146-149).
- Строка item: bar(label, muted(hint), inner) (bar view.go:40-43) — метка слева, hint справа;
  выбранная — selectedRow chrome.go:343-352 («› », bg Active, fg Highlight).
- Отключённые (enabled==false): muted, hint скрыт, Enter перескакивает.
- Футер: «↑/↓ · PgUp/PgDn · Enter открыть · Esc закрыть · n/m».

### 2.4 Клавиатура
- ↑/↓ — по плоскому списку, wrap, заголовки пропускаются (прецедент sidebar.go:383-396).
- PgUp/PgDn — страница ≈ 10-12 rows; Home/End — первый/последний (chrome.go:272-279; sidebar.go:389-392).
- Enter — выполнить (закрыть диалог до/после run); Esc — закрыть (из поиска — сначала поиск,
  прецедент session_search.go:34-39); печать — в поисковое поле.
- Мышь: клик — выбор+выполнение, колесо — прокрутка (паттерны chrome.go:418-471; settings_hub.go:276-311).

### 2.5 Реестр и подключение к Update
```go
type paletteItem struct {
    group  string // «Сессии», … порядок = порядок в slice
    label  string // RU-метка, tr()
    hint   string // подсказка справа (muted): "F9", "Ctrl+B", "/fork"
    enabled func(*model) bool
    run     func(*model) tea.Cmd
}

var paletteItems = []paletteItem{
    {group: "Сессии", label: tr("Новая сессия"), hint: "/new",
        enabled: func(m *model) bool { return m.settingsLogin != "" },
        run:     func(m *model) tea.Cmd { return m.sessionOperation("new", m.sidebarNewWorkspace()) }},
    {group: "Действия", label: tr("Остановить агента"), hint: "F8",
        enabled: func(m *model) bool { return m.session != nil },
        run:     func(m *model) tea.Cmd { return m.cancelChat() }},
    // …полный список из раздела 1 (≈60 items)
}

// state в model: paletteOpen bool; paletteQuery string; paletteIndex, paletteOffset int
// paletteSearch textinput.Model

func (m *model) paletteKey(msg tea.KeyMsg) (tea.Cmd, bool) {
    if !m.paletteOpen && msg.String() == "ctrl+p" { m.openPalette(); return nil, true }
    if !m.paletteOpen { return nil, false }
    // esc / enter / up / down / pgup / pgdown / home / end / ctrl+u / runes→paletteSearch
}
```
Вставка в Update (main.go:265) — после terminalFits-гейта (main.go:520-523), ДО runtimeKey
(main.go:527). Отрисовка — component.Render поверх View (overlay chrome.go:389-401, паттерн
chromeView chrome.go:403-416). Ctrl+P раньше не использовалась (проверено grep).

## 3. Клавиатура: что удаляется, что остаётся

Решение: «чистим клавиатуру под новую навигацию». Удаляются клавиши, служащие ТОЛЬКО
навигации (страницы/меню); операционные контекстные F-клавиши в диалогах/списках — остаются.

### 3.1 УДАЛЯЕТСЯ
| Клавиша | Что делает сейчас | Место |
|---|---|---|
| F10 / Alt+F / Alt+V / Alt+H | верхнее меню Файл/Вид/Справка | chrome.go:238-254; menuNames chrome.go:40; menuEntries 46-50; menuBar 329-341; mouse 444-470 |
| Alt+C / Alt+S / Alt+D / Alt+L / Alt+M | страницы Чат / Сессии / Настройки dsh / Журнал / Маркет | main.go:610-625 |
| Alt+W / Alt+A / Alt+P / Alt+E | рабочие области / агент / доступ / редактор (лендинг) | agent_landing.go:242-251 |
| Alt+1 / Alt+2 / Alt+3 | режим default/plan/goal | main.go:657-662 (заменяется пунктом палитры) |
| Tab | фокус на боковую панель (navFocused) | main.go:665-677; sidebarKey sidebar.go:367-414 |
| F11 (как навигация) | открыть/закрыть историю из чата | history.go:370-372 (main.go:569); таб «Траектория · F11» conversation_view.go:96; hints view.go:124, 130 |
| F12 (как навигация) | открыть выбор модели | models.go:309-311 (main.go:548); «Модель · F12» agent_landing.go:120; settings.go:89 |
| F1 / F4 / Alt+T | открыватели chrome-диалогов help / settings | chrome.go:232-237, 226-231; F4 в «Плагины»→dshSettingsPage workbench.go:1144-1151 |

Примечание: F1, F4/Alt+T, Alt+1/2/3 не названы в явном списке удалений пользователя
(F10, Alt+C/S/D/L/M/P/W/E/A, Tab, F11, F12), но это открыватели той же навигации — открытие
переносится в палитру; решение за исполнителем, помечено.

### 3.2 ОСТАЁТСЯ (операционные, контекстные)
| Клавиша | Контекст/роль | Место |
|---|---|---|
| F2 | план-toggle (чат); фильтр (расписания); компоненты (плагины); разрешить сборки (пакеты); JSON (форма расписания) | main.go:652-656; workbench.go:1007-1011; plugins.go:368-371; workbench.go:1120-1131; schedule_form.go:53-69 |
| F3 | переименовать (список); поиск истории; правка очереди; правка цели; обновления/правки в workbench (пакет, расписание, подагент, аккаунт, workspace) | session_rename.go:56-59; history_search.go:289-291; queue_ui.go:202-203; goal_dialog_ui.go:46-52; workbench.go:850, 879-894, 909-917, 771-783 |
| F5 | обновление снимков во всех диалогах; отправить ответы (вопросы); отправить /compose; сохранение форм | sidebar.go:408-409; settings_hub.go:204-205; workbench.go:1114-1119; plugins.go:366-367; goal_dialog_ui.go:53-57; models.go:330-331; history.go:422-425; queue_edit.go:173; compose.go:161; questions.go:231-232; settings_form_ui.go:154-155; schedule_form.go:97-120 |
| F6 | инструменты (чат/история); провайдеры (настройки dsh); версии (пакеты); маркет (страница Плагины) | tools.go:171-174; history_tools.go:154; settings_form_ui.go:20-22; workbench.go:1132-1143; plugins.go:386-392 |
| F7 | вопросы и разрешения (toggle) | approvals.go:224-229 |
| F8 | остановить агента (глоб.); pass-through во всех диалогах | main.go:522-523, 572-574; approvals.go:239-241; queue_ui.go:170-171; session_archive.go:70; session_fork.go:63; session_rename.go:62; workbench.go:962; history.go:360; models.go:316; compose.go:137; settings_form_ui.go:28; goal_dialog_ui.go:20; history_copy.go:175 |
| F9 | очередь сообщений (toggle) | queue_ui.go:141-149 |
| Esc | назад/закрыть в диалогах/списках; showChat в чате | main.go:678-679; chrome.go:258-259; history.go:382-383; и все dialog-гейты |
| Ctrl+D / Ctrl+C | выход (и отмена операции runtime) | main.go:663-664; chrome.go:223-224; runtime_ui.go:105-107 |
| Ctrl+N | создание записи: расписания, пакеты, пресеты, терминалы, рабочие области, аккаунт, форма | workbench.go:873, 825, 860, 922-929, 775-778, 753-761; settings_form_ui.go:139-141 |
| Стрелки / PgUp/PgDn / Home/End | списки, диалоги, viewport, история ввода (gate prefs.InputHistory) | main.go:803-840; sidebar.go:383-396; chrome.go:268-281 |
| Enter | подтвердить/отправить/открыть во всех list-диалогах | main.go:680-699; benchPrepare workbench.go:694-944 |
| Space | пауза/вкл (расписания, пакеты, плагины) | workbench.go:898-900, 839-845, 1181-1184 |
| Ctrl+X / Ctrl+B / Ctrl+F / Ctrl+G / Ctrl+Y / Ctrl+K / Ctrl+R / Ctrl+T / Ctrl+O | операции внутри списков/диалогов (архив, ветка, поиск, уточнить, копия, код, перечитать, область поиска, скрытые плагины) — не навигация | session_archive.go:55-58; session_fork.go:56-59; session_search.go:23-24; queue_ui.go:200-201; history.go:384-406; history_search.go:284-303; workbench.go:847-932; plugins.go:373-378 |
| Ctrl+P | НОВОЕ: открыть палитру | — (ранее не использовалась) |

### 3.3 Выводы для исполнителя
- Из Update (main.go:265) вырезаются: Alt-карта страниц main.go:610-625; tab main.go:665-677;
  alt+1/2/3 main.go:657-662. Из chromeKey: F10/Alt+F/V/H chrome.go:238-254; F4/Alt+T 226-231;
  F1 232-237. Из historyKey: глобальный F11 history.go:370-372. Из modelsKey: глобальный F12
  models.go:309-311. openDialog("settings"|"help"|"about") и openRuntime остаются — вызываются
  только из run() палитры (chrome.go:64-80; runtime_ui.go:79-90).
- Страницы plugins/schedules/market/dshSettings/sessions/log достигаются через page (main.go:22-33)
  из items палитры; tuiSettingsPage (main.go:26) мёртвая — не подключать.
- Переписать подсказки: футер view.go:122-138; «Траектория · F11» conversation_view.go:96;
  «Модель · F12» agent_landing.go:120; modeHint agent_landing.go:209; hint settings.go:89;
  menuBar chrome.go:329-341; tuiShortcuts shortcuts.go:6-74; help main.go:20 (черновик — 1.7).
