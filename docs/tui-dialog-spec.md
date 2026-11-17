# Спецификация единого диалога TUI (Фаза 2)

Документ-спецификация: что именно меняется, чтобы перевести все диалоги
apps/tui-go на один шаблон по образцу opencode (plan §«Фаза 2»,
docs/tui-rebuild-plan.md:137-142). Исходники Go и golden/ — только для
чтения; единственный изменяемый файл — этот.

Критерий из плана: «нет ни одного диалога, рисующего свою рамку/подсказки
сам» (tui-rebuild-plan.md:142).

## 1. Как работают диалоги сейчас

Общая механика (одинакова для всех 18 диалоговых оверлеев; поиск в
истории, строка 5 таблицы, своего оверлея не имеет):

- **panel(width, height, content string, color theme.Token)** —
  apps/tui-go/view.go:33-38: RoundedBorder, BorderForeground(theme.Color(color)),
  Foreground(theme.Text), Background(theme.Panel), Padding(0, 1); размеры
  вычитают рамку (GetHorizontalBorderSize/GetVerticalFrameSize).
- **overlay(base, front string, x, y, width int)** — apps/tui-go/chrome.go:390-401:
  композитинг по ячейкам терминала (ansi.Cut), без затемнения фона: всё,
  что за диалогом, остаётся «живым». Backdrop-затемнения нигде нет.
- **area{ x, y, width, height int }** — apps/tui-go/mouse.go:15, с
  contains() (mouse.go:17).
- Цепочка обёрток View: apps/tui-go/view.go:139 — runtimeView ⊃
  approvalView ⊃ queueDialogView ⊃ settingsFormView ⊃ goalView ⊃
  composeView ⊃ benchView ⊃ modelsView ⊃ historyCodeView ⊃ archiveView ⊃
  forkView ⊃ renameView ⊃ chromeView ⊃ historyView (последний рисуется
  первым, runtime — верхний слой).
- Диспетчер клавиш в Update: apps/tui-go/main.go:527-584, порядок:
  runtimeKey(527), approvalKey(530), queueKey(533), settingsFormKey(536),
  composeKey(539), benchKey(542), goalKey(545), modelsKey(548),
  historyCopyKey(551), sessionSearchKey(554), archiveKey(557), forkKey(560),
  renameKey(563), chromeKey(566), historyKey(569), F8(572),
  conversationKey(575), agentKey(578), sidebarKey(581), toolKey(584).
  Внимание: **runtimeKey первым** — его нет в списке диалогов, но в
  диспетчере он выше approvalKey.

Размеры сегодня везде «потолок + центрирование»: width = min(потолок,
m.width-3), height = min(потолок, m.height-4/2), позиция
((m.width-1-w)/2, (m.height-h)/2). Ни один диалог размер не выводит из
контента. Заголовки: heading() (apps/tui-go/theme.go:10, Bold + Text);
вторичное — muted() (apps/tui-go/theme.go:11); акцентные кнопки — selectedRow/selectedRow-style
(chrome.go:343-352, Highlight на Active).

## 2. Инвентаризация диалогов (19 диалогов; 18 со своим оверлеем + 2 исключения)

«View» — функция, вызывающая overlay/panel; «Box» — функция вычисления
размера (строка = строка с min(...)); «рамка» — token в panel().

| # | Диалог | View (overlay) | Box (размер) | Рамка | Клавиши |
|---|--------|----------------|--------------|-------|---------|
| 1 | Запрос разрешения dsh | approvals.go:271 approvalView (overlay :305) | approvals.go:267-269 approvalBox — 78×24 | Warning | approvalKey (approvals.go) |
| 2 | Вопрос агента (ask-user) | questions.go:303 questionView (overlay :390) | approvals.go:267 approvalBox — 78×24 | Highlight | approvalKey (approvals.go) |
| 3 | История сессии | history.go:307 historyView (overlay :353) | history.go:239-241 historyBox — 100×36 | Accent | historyKey (history.go:356) |
| 4 | Копирование блока кода | history_copy.go:203 historyCodeView (overlay :245) | history_copy.go:195-197 historyCodeBox — 88×26 | Accent | historyCopyKey (history_copy.go) |
| 5 | Поиск в истории | (нет своего оверлея) | — | — | historyKey (history.go); редактор в тулбаре history.go:344-347, состояние history_search.go:33-43 |
| 6 | Выбор модели | models.go:375 modelsView (overlay :425) | models.go:371-373 modelsBox — 94×28 | Accent | modelsKey (models.go) |
| 7 | Переименование сессии | session_rename.go:148 renameView (overlay :165) | инлайн :153 — 76×12 | Accent | renameKey (session_rename.go) |
| 8 | Ветвление сессии | session_fork.go:129 forkView (overlay :159) | инлайн :134 — 76×16 | Accent | forkKey (session_fork.go) |
| 9 | Архив/восстановление | session_archive.go:142 archiveView (overlay :170) | инлайн :153 — 76×16 | Accent | archiveKey (session_archive.go) |
| 10 | Форма настроек dsh | settings_form_ui.go:177 settingsFormView (overlay :288) | settings_form_ui.go:173-175 settingsFormBox — 98×32 | Accent | settingsFormKey (settings_form.go) |
| 11 | Плагины/расписания/все рабочие области | workbench.go:1202 benchView (overlay :1222 и :1324) | workbench.go:1198-1200 benchBox — 110×36 | Accent | benchKey (workbench.go) |
| 12 | Форма расписания | schedule_form.go:137 scheduleFormView (overlay :175) | workbench.go:1198 benchBox — 110×36 | Accent | benchKey (workbench.go) |
| 13 | Панель плагина (список команд) | plugin_ui.go:266 pluginPanelView (overlay :329) | workbench.go:1198 benchBox — 110×36 | Accent | benchKey (workbench.go) |
| 14 | Очередь заданий | queue_ui.go:228 queueDialogView (overlay :315) | queue_ui.go:224-226 queueBox — 82×25 | Accent | queueKey (queue_ui.go) |
| 15 | Правка задания в очереди | queue_edit.go:187 queueEditView (overlay :217) | queue_ui.go:224 queueBox — 82×25 | Accent | queueKey (queue_ui.go) |
| 16 | Runtime: установка/обновления/диагностика | runtime_ui.go:242 runtimeView (overlay :302) | инлайн :247 — 96×(m.height-4) | Highlight | runtimeKey (runtime_ui.go) |
| 17 | Диалоги chrome (настройки TUI, hotkeys, about, списки) | chrome.go:354 dialogBox (panel :386), обёртка chrome.go:403-407 | инлайн :361-362 — 64×(len+2); settings — settings_hub.go:212-213 — 112×34; help — shortcuts_view.go:105-107 — 100×34 | Highlight | chromeKey (chrome.go:197) |
| 18 | Цель сессии | goal_dialog_ui.go:116 goalView (overlay :183) | goal_dialog_ui.go:112-114 goalBox — 98×32 | Accent | goalKey (goal_dialog_ui.go) |
| 19 | Compose (многострочное сообщение) | compose.go:191 composeView (overlay :211) | goal_dialog_ui.go:112 goalBox — 98×32 | Accent | composeKey (compose.go) |

Не диалоги (исключения из миграции, но учитываются в спецификации):

- **Дропдаун верхнего меню** — chrome.go:408-414: panel(28, len+2, …,
  Highlight), позиция (menuStart, 2). Не центрируется, не оверлей в
  смысле «диалога»; миграция только на токены/размеры, шаблон Dialog
  не обязателен (open questions, Q4).
- **Страницы workbench** (pluginsPage/schedulesPage) — view.go:90-100:
  контент центрируется в основной области (lipgloss.PlaceHorizontal),
  panel() не применяется. Оверлей benchView из таблицы — только при
  открытии через палитру/команды.
- **Поиск по сессиям** — session_search.go:12-50: inline-фильтр списка
  сессий (textinput), оверлея нет.

### 2.1. Детали по диалогу

**1. Approval (approvals.go).** Header: heading + muted «Ожидает ответа · запросов: N» (:289); body — viewport (box.height-8, :283); footer: note (:298-300) + кнопки «[ Отказать ] [ Разрешить ]» (selectedRow, :303) + 2 muted-подсказки (:304). Пусто: нет; ошибка — s.note. Мышь: :321-326.

**2. Question (questions.go).** Тот же approvalBox (:306); header:
heading + muted progress «Вопрос i/n · …» (:310-319); body — viewport
(box.height-11, :309) + label + editor (3 строки, :371-379); footer:
note/error (:380-388) + questionButtons + muted-подсказка Tab/Enter/F5
(:389). Ошибка — f.err в note (:384-385).

**3. History (history.go).** Header: heading «История сессии» (+ « · исходник», :348-351) + статус-строка (:314-323); тулбар [←][→][F5 Новые][Поиск][Esc] (:305, :343) либо редактор поиска (:344-345); body — viewport (box.height-8, :249); footer: 2 muted-note + hint (:352). Пусто: :298; загрузка: :326.

**4. History copy (history_copy.go).** Header: heading +
«Сообщение #N · блок i/n» (:211) + пустая строка; body: список блоков
(5) + preview, обрезанный до box.height-len-6 (:228-232); footer: 2
muted-note + hint (:244). Пусто: нет (открывается на блоке).

**6. Models (models.go).** Header: heading + id + «Текущая: …» +
«Каталог…» + search (:387); body: 5 строк-опций, пусто —
«Нет доступных моделей по этому запросу.» (:408-410); footer:
selection/effort/2 info-строки + 2 muted-note + 2 строки-подсказки
(:419-424).

**7. Rename (session_rename.go).** Header: heading + id (:164); body:
editor; footer: до 3 строк note + hint (busy — «Ожидание ответа dsh…»,
:160-163). Высота фиксирована 12 — минимальный диалог.

**8/9. Fork / Archive (session_fork.go / session_archive.go).**
Header: heading + id (+ preview у fork :141-143); body: объяснение
(ansi.Wrap) + до 3 строк note; footer: hint с состояниями
busy/attempted (:152-157 / :163-168). Оба 76×16, структура идентична —
кандидаты на один «confirm»-вариант.

**10. Settings form (settings_form_ui.go).** Header: heading «Настройки · NS» + путь + «Ревизия i · изменений n» (:185-189); body: редактор / многострочник (box.height-13, :205) / список строк (box.height-11, :193), пусто — :253; footer: 3 muted-note (:267) + 2 hint (:268-286).

**11. Workbench bench (workbench.go).** Header: heading (15 типов, :1227) + «id / location» + search (:1230); body: editor / document / список + detail (h-listHeight, :1265-1268), пусто — :1258; footer: 3 muted-note (:1287) + 2 hint с ветвлением по d.kind (:1288-1322).

**12. Schedule form (schedule_form.go).** Header: heading
«Новая/Изменить задачу» + пустая строка (:149); body: поля title/prompt/
mode/minutes или note «Правило времени сохранится…» (:161); footer:
muted note + accent-note (:172, единственный диалог с accent-hint).

**13. Plugin panel (plugin_ui.go).** Header: heading + «Плагин: owner»
+ пустая строка (:271); body: fields/tables/actions в viewport
(box.height-9, :306); footer: note + hint (3 состояния) + «F5 —
перечитать…» (:328).

**14/15. Queue / queue edit (queue_ui.go / queue_edit.go).** Queue: header + muted-строка про F8 (:240); body: 5 строк-задач (пусто — :255) + detail; confirm меняет header (:263-266) и кнопки (:296-303); footer: 2 muted-note + steering + buttons + hint (:311). Edit: header + hint (:199), editor (box.height-8, :193), 2 muted-note, кнопки (:211).

**16. Runtime (runtime_ui.go).** Header: heading (3 вида, :249) + статус-строка (:251); body: список действий + viewport (height-len-7, :269); footer: state/note (:275-287) + hint с состояниями (:288-296). Единственный прижатый к верху (y=2, :302) и с высотой m.height-4.

**17. Chrome-диалоги (chrome.go:354-387).** Списки «Инструменты/Ещё/Сессия»: heading + пустая + selectedRow-строки + пустая + muted-hint (:373-377); about: блок + «[ Закрыть · Enter / Esc ]» (:379). Settings hub — свой layout (settings_hub.go:211-271, две колонки, header bar() с «[×]» :218); help — свой layout (shortcuts_view.go:114-163, header bar() + search :123-128). У обоих «Esc» в hint, не в header.

**18. Goal (goal_dialog_ui.go).** Header: heading (2 варианта, :125-128) + id + пустая (:129); body: draft-редакторы (bodyHeight-2, :132) или viewport (box.height-10, :158); footer: 3 muted-note (:170) + 2 hint с 4 состояниями (:171-181). Пусто: :140.

**19. Compose (compose.go).** Header: heading + hint-строка + пустая
строка (:200); body: editor (box.height-10, :199); footer: 2
shortText-строки (вложения/note) + 2 hint (:210).

## 3. Целевой API (internal/component/dialog.go, новый пакет)

Один файл-компонент; каждый диалог передаёт свои строки, рамку и размер.

```go
package component

import "github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"

// Размер диалога: ширина в ячейках, как в opencode (medium 60 / large 88 /
// xlarge 116, DIALOG_INSET = 3 — tui-rebuild-plan.md:18-21).
type Size int

const (
    Small  Size = iota // ширина 60: confirm и короткие формы
    Medium             // ширина 88: основные диалоги
    Large              // ширина 116: контентные (история, workbench, help)
)

const (
    MinTerminalWidth  = 80
    MinTerminalHeight = 24
    DialogInset       = 3
)

var sizeWidths = map[Size]int{Small: 60, Medium: 88, Large: 116}

// Dialog — описание одного диалогового оверлея. Body — готовый
// lipgloss-контент (viewport/editor/list), который диалог строит сам
// под ширину innerWidth(d); Dialog не управляет вложенными буббами.
type Dialog struct {
    Title    string       // heading() слева
    Subtitle string       // muted-строка под заголовком (опц.)
    Body     string       // контент; высота ограничена bodyHeight(d)
    Footer   string       // строки-подсказки/кнопки, разделённые \n
    Border   theme.Token  // рамка: см. семантику ниже
    Size     Size
    AlignTop bool         // runtime — прижат к верху (y = Inset), иначе центрирование
}

func innerWidth(d Dialog, termW int) int // min(sizeWidths[d.Size], termW-3) - 4
func bodyHeight(d Dialog, termH int) int // (высота по размеру, см. Q2) - зарезервированные строки

// Box — позиция и размер рамки в ячейках терминала:
// x = (termW-1-w)/2, y = max(Inset, (termH-h)/2); AlignTop: y = Inset.
func Box(d Dialog, termW, termH int) area

// Layout — шаблон DialogLayout (opencode: header / blank / body / footer):
// строка 1: Title (bold) + прижатый справа muted "Esc" (bar-стиль, как
// settings_hub.go:218); строка 2: Subtitle (muted, если задан); строка 3:
// пустая; далее Body; пустая; Footer. Возвращает строки без рамки.
func Layout(d Dialog, body string) []string

// Render — Layout + панель + центрирование/прижатие: возвращает (area,
// готовый panel-string), готовый к overlay().
func Render(d Dialog, body string, termW, termH int) (area, string)

// Backdrop — затемнение основы: липгloss-обёртка всего base
// Background(theme.Color(theme.Dim)) + Width(termW-1). theme.Dim — новый
// токен (Q1); тёмная #0b0d10, светлая #e8ecf1.
func Backdrop(base string, termW int) string
// РАЗЁМ под позднюю фазу (Q1): в фазе 2 не вызывается.
```

Точки интеграции:

1. view.go:33 panel() остаётся (используется основными панелями, не
   только диалогами); Render() внутри вызывает её — panel() не меняется.
2. В цепочке view.go:139 каждый *View* делает:
   box, front := component.Render(d, body, m.width, m.height),
   return overlay(base, front, box.x, box.y, m.width-1). overlay()
   (chrome.go:390) не меняется. Backdrop() в фазе 2 не вызывается (Q1).

### 3.1. Карта размеров

| Размер | Ширина | Высота (зафиксировано, Q2) | Диалоги | Сейчас |
|--------|--------|--------------------------|---------|--------|
| Small | 60 | 14 | rename (76×12), fork (76×16), archive (76×16), about (64×~10) | — |
| Medium | 88 | 24 | approval (78×24), question (78×24), models (94×28), queue (82×25), queue edit (82×25), history copy (88×26), goal (98×32), compose (98×32), settings form (98×32), schedule form (110×36) | — |
| Large | 116 | 34 | history (100×36), workbench (110×36), plugin panel (110×36), settings hub (112×34), help (100×34), runtime (96×h-4, AlignTop) | — |

Сужение width (94→88, 98→88, 98→116 и т. п.) меняет per-row truncation —
это и есть намеренный golden-diff фазы.

### 3.2. Токен рамки по состоянию (верифицировано по текущему коду)

| Состояние | Токен | Где уже так |
|-----------|-------|-------------|
| Интерактивный (фокус ввода, навигация, формы) | theme.Accent | 14 из 18 оверлеев (§2) |
| Approval (разрешение) | theme.Warning | approvals.go:305 |
| Чтение/информация (question, runtime, chrome-диалоги, menu) | theme.Highlight | questions.go:390, runtime_ui.go:302, chrome.go:386,413, settings_hub.go:274, shortcuts_view.go:167 |
| Нейтральный (основные панели, не диалоги) | theme.Border | view.go:89,102 |

## 4. Миграция по диалогам (что меняется в каждом)

1. **Approval** — Border: Warning сохранится; Box → Medium(88×24) (сегодня
   78×24 — сужение на 10 ячеек, viewport.Width = innerWidth); header/
   footer переносятся в Subtitle/Footer; кнопки остаются в Body (высота
   body = bodyHeight). approvalBox() удаляется.
2. **Question** — Border: Highlight; Box → Medium; viewport =
   bodyHeight; questionButtons в Body; progress → Subtitle.
3. **History** — Box → Large(116×34) (сегодня 100×36 — +16 ширины, −2
   высоты); тулбар/редактор поиска остаются в Body; footer: note+hint.
   historyBox() удаляется.
4. **History copy** — Box → Medium(88×24) (88×26 — высота −2, проверить
   preview-обрезку :228); preview в Body.
5. **Search-in-history** — ничего (нет своего оверлея); только
   последовательность hint-строк history.go:346.
6. **Models** — Box → Medium(88×24) (94×28 — сужение −6, −4; 5 строк-
   опций + search в Body); footer-инфо (2 строки :419) — решение:
   Subtitle+Footer (Q5).
7. **Rename** — Box → Small(60×14) (76×12 — сужение −16; editor.Width =
   innerWidth−10 :155 пересчитать); content :164 → Subtitle+Body+Footer.
8. **Fork** — Box → Small(60×14) (76×16 — −16, −2; объяснение
   :136-139 обёрнута под innerWidth 56 — проверить wrap).
9. **Archive** — Box → Small(60×14); структура как fork.
10. **Settings form** — Box → Medium(88×24) (98×32 — сужение −10, −8;
    multiline.SetHeight :205 и available :193 пересчитать под bodyHeight;
    ⚠ самый плотный диалог — приоритет 1 в фазе, Q2).
11. **Workbench bench** — Box → Large(116×34) (110×36 — +6, −2; listHeight
    :1226, :1246 и view.Height :1237/:1266 пересчитать).
12. **Schedule form** — Box → Medium(88×24) (110×36 — −22, −12; поля
    title/minutes/prompt :142-144 пересчитать; решение Q3 — Medium, не Small.
13. **Plugin panel** — Box → Large(116×34); view.Height :306 пересчитать.
14. **Queue** — Box → Medium(88×24) (82×25 — +6, −1; rows :241,
    detailRows :269 пересчитать).
15. **Queue edit** — Box → Medium(88×24); editor.SetHeight :193
    пересчитать.
16. **Runtime** — Box → Large(116×34), AlignTop: true (y=Inset≈2, :302
    сохраняется, Q3); bodyHeight фиксированный (Q2);
    lines[:height-2] :301 → Layout.
17. **Chrome-диалоги** — dialogBox() :354-387: три списка → Small
    (64→60); about → Small; settings hub (settings_hub.go:212, 112×34) и
    help (shortcuts_view.go:106, 100×34) → Large(116×34). Их
    собственные header с bar()+[×] (:218, :123) — решить: сохранить
    [×] как кнопку в header (Q4) или только muted «Esc».
18. **Goal** — Box → Medium(88×24) (98×32 — −10, −8; viewport :158 и
    draft :132 пересчитать).
19. **Compose** — Box → Medium(88×24); editor.SetHeight :199 пересчитать.

## 5. Решения по открытым вопросам (согласовано с ведущим, 2026-11-17)

- **Q1. Токен backdrop.** Новый токен `theme.Dim` (dark ~#0b0d10,
  light ~#e8ecf1) — в internal/theme/theme.go рядом с остальными
  (tokens: theme.go:18-29). Backdrop-затемнение **отложено на позднюю
  фазу** (non-goal, §6): план его не требует явно, а затемнение
  меняет все golden сразу. Функция Backdrop() в §3 — зарезервированная
  точка интеграции, в фазе 2 не вызывается.
- **Q2. Фиксированные высоты.** Да: Small 14, Medium 24, Large 34.
  bodyHeight(d) = высота размера − зарезервированные строки шаблона
  (header 1 + (subtitle 0/1) + blank 1 + blank 1 + footer N).
- **Q3. Schedule form → Medium(88×24)** (не Small); **runtime → Large
  (116×34) + AlignTop: true** (y = Inset, как сегодняшние y=2).
- **Q4. Дропдаун верхнего меню (chrome.go:408-414) не трогаем** —
  остаётся как есть (panel 28, Highlight, позиция menuStart).
- **Q5. Muted «Esc» — правый slot в header** (bar()-стиль, chrome.go:340;
  как [×] у settings_hub.go:218 и shortcuts_view.go:123). Footer-hint
  с перечнем клавиш остаётся, как есть.
- **Q6. Click-outside-закрытие — НЕ в этой фазе (non-goal).** Мышь
  пересчитывается только под новые box-размеры; hit-логика не
  меняется.
- **Q7. Чипы selectedRow (Highlight на Active) остаются в Body** —
  кнопки «[ Отказать ] [ Разрешить ]» (approvals.go:303) и confirm-
  кнопки очереди (queue_ui.go:296-303) переносятся в Body без смены
  стиля.
- **Q8. Общий confirmDialog(fork/archive) — на позже**, вне фазы 2.

## 6. Не цели (non-goals)
- **Backdrop-затемнение и click-outside — отложены** (решение по Q1/Q6):
  план их не требует явно; затемнение меняет все golden, поэтому —
  отдельная фаза. Токен Dim (решение Q1) добавляется в internal/theme
  отдельно (механический коммит, не меняет отрисовку) или вместе с той
  фазой — на выбор исполнителя.

- Не меняем клавиши и их диспетчер (порядок main.go:527-584 сохранён;
  «клавиши не перемещаем» — tui-rebuild-plan.md:176-177).
- Не меняем протокол dsh, команды, содержимое диалогов (тексты, логику
  состояний busy/confirm/editing).
- Не меняем overlay() и area; не трогает mouse-хит-области, кроме
  пересчёта под новые box-размеры.
- Не добавляем backdrop на non-dialog панели (sidebar, основные
  панели, композер) — только под диалоговые оверлеи.
- Не трогаем: страницы workbench как full-page (view.go:90-100),
  inline-поиск сессий (session_search.go), terminalSizeNotice
  (view.go:19-31), light-theme обёртку (view.go:140-142).
- Не вводим анимации (fade-in, shimmer) — это Фаза 5 (plan :164-167).
- Не переименовываем *Box/*View функции раньше, чем диалог не переведён
  на component.Dialog (один коммит на диалог).
