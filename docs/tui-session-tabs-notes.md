# Рейка вкладок сессий: как сейчас устроено переключение (для реализации)

Факты с file:line; путь = `apps/tui-go/`. Всё, что не указано, — не существует в коде.

## 1. Что значит «подключить сессию»

Текущая сессия — `m.session *sessionView` (main.go:89). RPC `session/connect`
нет: «connect» — имя клиентского действия в `sessionOperation`.

- `func (m *model) sessionOperation(action, value string) tea.Cmd` — session.go:280.
  Гварды: нужен `m.settingsLogin` (session.go:281); при `m.sessionBusy` действие
  в очередь `m.sessionQueue`, макс. 8 (session.go:285-294); кроме
  `new`/`connect`/`list` нужна уже подключённая `m.session` (session.go:295-298);
  для архивированной — только `refresh`/`connect`/`list`/`new` (session.go:299-302).
- Запуск: `sessionBusy=true`, `sessionAction=action`, `sessionSequence++`,
  `rebuild()` (session.go:303-318). Cmd внутри:
  - `list` → RPC `session/list` (session.go:327-335);
  - `new` → RPC `session/create` (session.go:336-352), затем общий шаг;
  - `connect`/`refresh`/`new` → `readSession(ctx, api, id)` (session.go:353-354).
- `readSession` — session.go:93-188: RPC `session/projections` (session.go:98;
  kind обязан `sequenced`, session.go:104); title из `values["title"]`
  (session.go:147-149); plan (150-154), goal (155-160), userQuestions (161-169);
  RPC `session/state` (session.go:177) → `Queue`, `Cursor`, `Running`,
  `PendingInput` (session.go:170-186); адрес подагента — через `session/list`
  (session.go:110-146).
- Обработка — `case sessionResult` (main.go:418-486): guard `msg.sequence`
  (main.go:419); при смене id **`m.lines = nil`** (main.go:435-437); локальные
  model/title переносятся по ревизиям (main.go:438-443); **`m.session = msg.view`**
  (main.go:444); **`chatCmd = m.startChat(msg.view.ID)`** (main.go:445); затем
  `syncContinuedQuestions()`, `reconcileQueuedPrompt()` (main.go:446-447);
  для `msg.list`: merge локальных title (main.go:449-458),
  `m.sessionItems = msg.items` (main.go:459), `m.sessionListIndex = 0` (main.go:461).
  Пока `m.session != nil` — `sessionTimer()` 2 c (session.go:366-369) →
  `sessionTick` (main.go:413-417) → `sessionOperation("refresh","")`.

Кто вызывает connect: Enter на странице «Сессии» — main.go:680-685: `id :=
m.visibleSessions()[m.sessionListIndex].ID; m.showChat(); m.sessionOperation("connect", id)`.
Также: `/session <id>` — main.go:773-775; `/parent` — main.go:752-757 (connect на `m.session.Address.ParentID`);
CLI `--session` — main.go:255-256, cli_native.go:241; сайдбар «session» — sidebar.go:316-328;
workbench «agents» Enter — workbench.go:722-729.
`sessionListIndex` (main.go:96) — курсор списка: ↑/↓ — main.go:803-813;
сброс в 0 — main.go:461, archive_state.go:105, session_archive.go:131.

## 2. Как создаёт сессию `/new`

`/new` — main.go:720-721 → `sessionOperation("new", "")`; CLI `--new` — main.go:253-254,
cli_native.go:36,242; пункт «⊕ Новая сессия» сайдбара — sidebar.go:313-315 с `m.sidebarNewWorkspace()`
(sidebar.go:458-474, передаёт workspaceId активной рабочей области). Внутри: `session/create` с
`request: {workspaceId}` (если не пусто; session.go:340-343), ответ `{sessionId}` (session.go:337-339);
`session/create` принимает `agentPreset` (workbench.go:860-861). Созданная сессия **немедленно
подключается** общим шагом `readSession` (session.go:353-354) — после `/new` `m.session` уже новая,
чат стартует.

## 3. `visibleSessions()` и откуда id/title

`func (m *model) visibleSessions() []sessionItem` — archive_state.go:112-120.
Фильтры: (а) `m.isSessionArchived(item.ID) == m.showArchived` (archive_state.go:115);
(б) `m.sessionQuery` по `ID+" "+Title`, регистронезависимо (archive_state.go:115).
**Без workspace-фильтра и лимита; порядок = порядок `m.sessionItems`.**
`sessionItem` — session.go:71-79: `ID` (json `sessionId`), `Running`
(json `running`), `Projections.Values.Title` (json `projections.values.title`).
Источник: RPC `session/list` (session.go:332; копия в сайдбаре — sidebar.go:89).
`m.sessionItems` (main.go:90) обновляется только через
`sessionOperation("list","")` (main.go:459); «r» — main.go:645-648.
Пустой title → показывают id (session.go:440-445, sidebar.go:209-212, view.go:80-86).
Переименование: `handleSessionRename` (session_rename.go:116-146):
`m.titleRevision++` (session_rename.go:126) + патч `m.session.Title`,
`m.sessionItems`, `m.side.sessions`; свежие `list` получают локальные title
при смене ревизии (main.go:449-458, sidebar.go:129-139).
Workspace сессии **нет** ни в `sessionItem`, ни в `sessionView`; соответствие
только из `workspace/follow` (sidebar.go:94-112; `sidebarWorkspace.Sessions` —
sidebar.go:17-22; использование sidebar.go:205-218, workbench.go:199).

## 4. Песессия-состояние, нужное вкладке

АКТИВНОЙ (из `sessionView`, session.go:49-70):
- `Running` (session.go:67; живёт от `turn/start`/`turn/end` — chat.go:604-607, 635-637) — спиннер;
- `Goal` (session.go:65) + `GoalSupported` (66); активна при `Goal.Goal.Phase == "active"` (session.go:380, 481-486) — маркер цели;
- `Plan` (session.go:64, `Active`/`Pending`; session.go:377-378);
- `Queue []queuedMessage` (session.go:59) + `QueueKnown` (60); длина очереди — `visibleQueue()` (queue.go:70-87: placement `queued`/`steering` минус принятые `chat.admitted`);
- `Model *modelSelection` (session.go:54; тип models.go:16-20; запись models.go:281-286);
- `PendingInput []string` (session.go:68,174,185) — вопросы/разрешения (session.go:466-473, main.go:177);
- `Title` (session.go:58), `AgentPreset` (53), `Permission` (55);
- `Address sessionReadAddress` (session.go:56, тип 34-47); `Kind=="subagent"` — просмотр подагента (main.go:177, chat.go:204-212);
- `m.sessionStale` (main.go:95; session.go:374-376);
- `m.prompts map[string]*pendingPrompt` (main.go:65; тип chat.go:105-109) — незавершённая отправка **пер сессию**, переживает переключение (chat.go:236-251, 966-992).

ЛЮБОЙ из списка (без доп. RPC): `sessionItem.Running` (session.go:73, 433-435);
`sessionItem.Projections.Values.Title` (session.go:74-78); архив —
`m.isSessionArchived(id)` (archive_state.go:111; карта `m.archive.ids` из потока,
archive_state.go:68-110).
Goal/Model/Queue/Address неактивной сессии **не читаются**: `session/state` /
`session/projections` — только для подключённой (session.go:93-188).

## 5. Как чат прикрепляется к сессии и что при смене

`m.chat *chatState` (main.go:61); `chatState.id` — id сессии (chat.go:110-135); инвариант `m.chat.id == m.session.ID`.
`startChat(id)` — chat.go:154-175: если `m.chat.id == id` — nil (chat.go:155-157);
иначе `stopChat()` (chat.go:177-189: гасит composer, workbench, goal/models/history,
`queueDialog`, `m.chat.cancel()`), `m.chatEpoch++` (chat.go:166), новый
`chatState{id, epoch, cursor:-1}` (chat.go:167), `m.chat = s` (chat.go:168),
`tea.Batch(followChat(...), m.startApprovals(...), m.ensureArchive(),
m.loadAgentContext())` (chat.go:174).
`followChat` — chat_native.go:16-46: WS-поток `session/follow`
(chat_native.go:58-60; `maxMessages:50, assistantStream:true`), бэкофф 1с→15с
(chat_native.go:44).
При смене: `m.lines = nil` (main.go:435-437), `m.session = view` (main.go:444),
`startChat(newID)` (main.go:445). Старый чат **отбрасывается и пересоздаётся с нуля**
(snapshot, chat.go:742-783); кэша сообщений нет. `m.prompts` не сбрасывается;
`reconcileQueuedPrompt` (queue.go:42-68) — только для `m.session.ID`.
`m.agentInfo *agentContext` (main.go:40; тип agent_landing.go:16-23) — каталог
моделей/пресеты АКТИВНОЙ сессии; перечитывается `loadAgentContext`
(agent_landing.go:35-72), если `id != m.session.ID || epoch != m.chat.epoch`
(agent_landing.go:39-41); вызов — из `startChat` (chat.go:174).
На время «new»/«connect» всё блокирует `chatSessionChanging()` (chat.go:354-364):
submitContent (chat.go:224-229), cancelChat (chat.go:313), retryPrompt (chat.go:294),
диалоги (goal_dialog.go:73,159; models.go:114; queue_ui.go:59; session_archive.go:30;
compose.go:43,167). Автопереподключение чата при провале кадра — chat.go:913-964
(пересоздаёт `startChat(id)`, переносит messages/live/admitted, chat.go:928-938).

## 6. Ctrl+N

**Глобального Ctrl+N «новая сессия» на чат-странице НЕТ**: `tea.KeyCtrlN` не
обрабатывается в main.go; на чат-странице клавиша уходит в `m.input.Update`
(main.go:858-860) и игнорируется. Ctrl+N — только в диалогах: workbench — вход
(workbench.go:753-761), сессия в рабочей области `session/create{workspaceId}` (workbench.go:775-778),
пакет (workbench.go:825-834), сессия с пресетом `session/create{agentPreset}`
(workbench.go:860-861), расписание (workbench.go:874-878); плюс settings_form_ui.go:139,
account.go:33, plugins.go:414-450, schedules_view.go:224. Справка обещает «Ctrl+N
новая сессия» только для `/presets` (main.go:20; translations.en.json:104,132); план
фиксирует Ctrl+N как сохраняемую клавишу (docs/tui-rebuild-plan.md:176) — привязку
на главной странице придётся добавить в main.go.

## 7. Понятие «открытые вкладки / недавние сессии»

В Go-коде **нет**: grep `tabOrder|openTabs|recentSession|tabIndex` по `*.go` — 0 совпадений.
Есть только `sessionListIndex` (курсор списка) и `m.side.sessions` (дерево сайдбара: активная —
«●», sidebar.go:213-217; архивные скрыты, sidebar.go:206-208). «Вкладки» [Чат]/[Траектория]
(conversation_view.go:95-97) — подрежимы одной сессии, не сессии. Свежесть:
`sidebarTick` 10 c → `loadSidebar()` (sidebar.go:50-52, 54-115) + `sessionTimer` 2 c активной (session.go:366-369).

## 8. Предлагаемый минимальный state API для рейки

Состояние (добавить в `model`, main.go:37-116):
- `tabOrder []string` — id сессий, **самая недавняя первым**; активная вкладка =
  `m.session.ID` (main.go:89). «+ Новая сессия» — кнопка в конце рейки, не вкладка.
- `tabFocus int` (или переиспользовать паттерн `m.sessionListIndex`, main.go:96).

Действия (только существующий код, без новых RPC):
- «+ Новая сессия» — `m.sessionOperation("new", workspaceID)` (session.go:280;
  workspaceID из `sidebarNewWorkspace()`, sidebar.go:458-474, или `""`).
- Переключение — **точно существующий путь** `m.sessionOperation("connect", id)`
  (session.go:280), как main.go:680-685 (опционально `m.showChat()` перед,
  main.go:243-249).
- Точка мутирования: в `case sessionResult` при `msg.view != nil` (main.go:444-445)
  после `m.session = msg.view` перенести `msg.view.ID` в начало `tabOrder`
  (дубликат удалить); работает и для new, и для connect.
- Закрытие вкладки — RPC удаления сессии в TUI нет; либо
  `workspace/archiveSession` (session_archive.go:97-106), либо клиентское
  удаление id из `tabOrder`.

Поля подписки вкладки (по id из `tabOrder`):
- title: из `m.sessionItems` (session.go:71-79); активная — `m.session.Title`
  (session.go:58); пустой → id (session.go:440-445); обрезка
  `ansi.Truncate(clean(name), width, "…")` (session.go:448, sidebar.go:277).
- running: неактивная — `sessionItem.Running` (session.go:73); активная —
  `m.session.Running` (session.go:67, chat.go:604-637). Готовых спиннер-символов
  нет (docs/tui-rebuild-plan.md:40-41 требует one-cell spinner); существующие
  индикаторы: «· выполняется» (session.go:433-435), «◌» (chat.go:869, view.go:59,71).
- archived: `m.isSessionArchived(id)` (archive_state.go:111) — дёшево для любого id;
  маркер «· архив» (chat.go:998-999).
- goal active: только активная — `m.session.Goal.Goal.Phase == "active"` (session.go:380).
- очередь: только активная — `len(m.visibleQueue())` (queue.go:70-87); неактивные
  не читаются (session.go:93-188) — без маркера либо доп. `session/state`.
- model: только активная — `m.session.Model` (session.go:54; models.go:16-20).
- subagent: только активная — `m.session.Address.Kind == "subagent"` (session.go:56,137-141);
  вкладка-просмотр, ввод отключён (chat.go:204-212).
- pending-отправка: `m.prompts[id] != nil && Status != "rejected"` (main.go:65;
  chat.go:105-109, 966-992) — работает для любых id.

Освежение рейки:
- `m.sessionItems`/`m.side.sessions` — `sidebarTick` 10 c (sidebar.go:50-52, 54-115);
  активная — `sessionTimer` 2 c (session.go:366-369, main.go:413-417). Рейка
  подписывается на те же тики; доп. `sessionOperation("list","")` опционален
  (паттерн main.go:645-648).
- Title при переименовании — патч по `m.titleRevision` (session_rename.go:126-139,
  main.go:449-458).

Опасные места:
- не обходить `sessionOperation`: гварды `sessionBusy`/`sessionQueue`
  (session.go:285-294) и guard `sessionSequence` (main.go:419) защищают от гонок;
- во время «new»/«connect» отправки блокированы `chatSessionChanging()`
  (chat.go:354-364) — рейка не должна слать сообщения;
- архивная активная сессия читательна, но не принимает ввод (session.go:299-302,
  chat.go:214-219);
- `m.lines` и чат сбрасываются при каждой смене (main.go:435-437, chat.go:154-175) —
  «запоминание прокрутки» для вкладок делать не на чём;
- мышь: `mouse.go:59` и `sidebar.go:415-443` заняты сайдбаром; рейке нужен свой
  хит-тест, аналог `sidebarMouse` (sidebar.go:434-442).
