package main

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

const (
	palettePageSize = 10
)

// paletteGroups is the fixed display order of item groups (tab cycles them).
var paletteGroups = []string{"Сессии", "Навигация", "Настройки", "Плагины и маркет", "Автоматизация", "Действия"}

type paletteState struct {
	open           bool
	search         textinput.Model
	offset         int
	groupFilter    string
	inputFocused   bool
	marketFocused  bool
	sessionFocused bool
	benchFocused   bool
	bench          *workbench
	note           string
}

type paletteItem struct {
	group   string
	label   string // Stable source key; translate only at search/render time.
	hint    string
	enabled func(*model) bool
	run     func(*model) tea.Cmd
}

// Shared enable predicates (gates from docs/tui-palette-catalog.md).
func paletteHasSession(m *model) bool { return m.session != nil }
func paletteHasURL(m *model) bool     { return m.settingsLogin != "" }
func paletteIsSubagent(m *model) bool {
	return m.session != nil && m.session.Address.Kind == "subagent"
}
func paletteCompose(m *model) bool { return m.session != nil && m.session.Address.Kind != "subagent" }
func paletteWritable(m *model) bool {
	return m.settingsLogin != "" && m.settings != nil && m.settings.Writable
}
func paletteHasApprovals(m *model) bool {
	return m.approvals != nil && m.approvals.ctx.Err() == nil && len(m.approvals.requests) > 0
}

// paletteLastAssistantSeq finds the last plain assistant message using the same
// rule as the Ctrl+Y handler (conversation_view.go:276-288); -1 when none.
func paletteLastAssistantSeq(m *model) int {
	messages := m.displayMessages()
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i].message
		if (message.assistant || message.role == tr("АГЕНТ")) && message.tool == nil && message.approvalID == "" {
			return message.seq
		}
	}
	return -1
}

var paletteCatalog = []paletteItem{
	// Сессии
	{group: "Сессии", label: "Новая сессия", hint: "/new · Ctrl+N",
		enabled: paletteHasURL,
		run:     func(m *model) tea.Cmd { return m.sessionOperation("new", "") }},
	{group: "Сессии", label: "Список сессий", hint: "/sessions · Alt+S",
		enabled: paletteHasURL,
		run: func(m *model) tea.Cmd {
			m.page = sessionsPage
			m.resize()
			return m.sessionOperation("list", "")
		}},
	{group: "Сессии", label: "Переименовать текущую", hint: "/rename · F3",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openSessionRename(true) }},
	{group: "Сессии", label: "Ветвление (fork)", hint: "/fork · Ctrl+B",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openSessionFork(true) }},
	{group: "Сессии", label: "Архивировать", hint: "/archive · Ctrl+X",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openSessionArchive(true, false) }},
	{group: "Сессии", label: "Восстановить из архива", hint: "/restore",
		enabled: paletteHasURL,
		run:     func(m *model) tea.Cmd { return m.openSessionArchive(true, true) }},
	{group: "Сессии", label: "К родителю (подагент)", hint: "/parent",
		enabled: paletteIsSubagent,
		run:     func(m *model) tea.Cmd { return m.sessionOperation("connect", m.session.Address.ParentID) }},
	{group: "Сессии", label: "Режим: обычный", hint: "/mode default",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.chooseMode("default") }},
	{group: "Сессии", label: "Режим: план", hint: "/plan · F2",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.chooseMode("plan") }},
	{group: "Сессии", label: "Режим: цель", hint: "/mode goal",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.chooseMode("goal") }},
	// Навигация
	{group: "Навигация", label: "Чат", hint: "Esc",
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.showChat() }},
	{group: "Навигация", label: "Настройки dsh", hint: "Alt+D",
		enabled: func(*model) bool { return true },
		run: func(m *model) tea.Cmd {
			m.page, m.navFocused = dshSettingsPage, false
			m.input.Blur()
			m.marketEditing = false
			m.marketSearch.Blur()
			m.resize()
			m.viewport.GotoTop()
			return m.loadSettings()
		}},
	{group: "Навигация", label: "Настройки TUI", hint: "F4",
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.openDialog("settings") }},
	{group: "Навигация", label: "Журнал", hint: "Alt+L",
		enabled: func(*model) bool { return true },
		run: func(m *model) tea.Cmd {
			m.page, m.navFocused = logPage, false
			m.input.Blur()
			m.marketSearch.Blur()
			m.resize()
			m.viewport.GotoTop()
			return nil
		}},
	{group: "Навигация", label: "Показать/скрыть контекст", hint: "",
		enabled: func(m *model) bool { return m.page == chatPage },
		run: func(m *model) tea.Cmd {
			m.contextPanelHidden = !m.contextPanelHidden
			m.resize()
			return nil
		}},
	{group: "Навигация", label: "Установка и dsh", hint: "/runtime",
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.openRuntime("runtime") }},
	{group: "Навигация", label: "Обновления и откат", hint: "/updates",
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.openRuntime("updates") }},
	{group: "Навигация", label: "Диагностика", hint: "/diagnostics",
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.openRuntime("diagnostics") }},
	{group: "Навигация", label: "Горячие клавиши", hint: "F1",
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.openDialog("help") }},
	{group: "Навигация", label: "О программе", hint: "",
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.openDialog("about") }},
	{group: "Навигация", label: "Выход", hint: "Ctrl+D",
		enabled: func(*model) bool { return true },
		run:     func(*model) tea.Cmd { return tea.Quit }},
	// Настройки
	{group: "Настройки", label: "Доступ по умолчанию", hint: "F5 — сохранить",
		enabled: paletteWritable,
		run:     func(m *model) tea.Cmd { return m.settingsHubAction("permission-default") }},
	{group: "Настройки", label: "Язык интерфейса", hint: "RU / EN",
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.togglePreference(6) }},
	{group: "Настройки", label: "Оформление (тема)", hint: "тёмная / светлая",
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.togglePreference(4) }},
	{group: "Настройки", label: "Размер шрифта", hint: "терминал",
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.settingsHubAction("font") }},
	{group: "Настройки", label: "Мышь в интерфейсе", hint: "вкл / выкл",
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.togglePreference(0) }},
	{group: "Настройки", label: "История ввода", hint: "вкл / выкл",
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.togglePreference(1) }},
	{group: "Настройки", label: "Модель сессии", hint: "/models · F12",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openModels() }},
	{group: "Настройки", label: "Провайдеры", hint: "/providers",
		enabled: paletteWritable,
		run:     func(m *model) tea.Cmd { return m.openSettingsForm(true) }},
	{group: "Настройки", label: "Команды агента", hint: "/commands",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("commands") }},
	// Плагины и маркет
	{group: "Плагины и маркет", label: "Плагины dsh (страница)", hint: "/plugins · F2 · Ctrl+O",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("plugins") }},
	{group: "Плагины и маркет", label: "Маркет (страница)", hint: "/market · i/u/d · r",
		enabled: paletteHasURL,
		run: func(m *model) tea.Cmd {
			m.page, m.navFocused = marketPage, false
			m.input.Blur()
			m.resize()
			return m.loadMarket()
		}},
	{group: "Плагины и маркет", label: "Пакеты dsh", hint: "/packages · F6 — версии",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("packages") }},
	// Автоматизация
	{group: "Автоматизация", label: "Расписания", hint: "/schedules",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("schedules") }},
	{group: "Автоматизация", label: "Задачи команды", hint: "/tasks",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("tasks") }},
	{group: "Автоматизация", label: "Фоновые задания", hint: "/jobs · Ctrl+X — остановить",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("jobs") }},
	{group: "Автоматизация", label: "Подагенты", hint: "/agents · F3 — сообщение",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("agents") }},
	{group: "Автоматизация", label: "Терминалы", hint: "/terminals · Ctrl+N — оболочка",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("terminals") }},
	{group: "Автоматизация", label: "Рабочие области", hint: "/workspaces · Ctrl+F поиск",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("workspaces") }},
	{group: "Автоматизация", label: "Аккаунт DeepSeek", hint: "/account · Ctrl+N — войти",
		enabled: paletteHasURL,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("account") }},
	{group: "Автоматизация", label: "Журнал операций пакетов", hint: "/operations",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("operations") }},
	{group: "Автоматизация", label: "Файлы рабочей области", hint: "/files",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("files") }},
	// Действия
	{group: "Действия", label: "Остановить агента", hint: "F8",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.cancelChat() }},
	{group: "Действия", label: "Повторить неопределённую отправку", hint: "/retry-send",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.retryPrompt() }},
	{group: "Действия", label: "Очередь сообщений", hint: "F9 · F3 правка · Ctrl+G уточнить",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openQueue() }},
	{group: "Действия", label: "Многострочный редактор", hint: "/compose · F5 отправить",
		enabled: paletteCompose,
		run:     func(m *model) tea.Cmd { return m.openCompose() }},
	{group: "Действия", label: "Очистить экран", hint: "/clear",
		enabled: func(*model) bool { return true },
		run: func(m *model) tea.Cmd {
			m.lines = nil
			if m.chat != nil {
				m.chat.clearedThrough = m.chat.cursor
			}
			m.rebuild()
			return nil
		}},
	{group: "Действия", label: "История сессии", hint: "F11 · Ctrl+F поиск · F3 ещё",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openHistory() }},
	{group: "Действия", label: "Копировать последний ответ", hint: "Ctrl+Y",
		enabled: func(m *model) bool { return m.conversationVisible() && paletteLastAssistantSeq(m) >= 0 },
		run:     func(m *model) tea.Cmd { return m.copyConversation(paletteLastAssistantSeq(m)) }},
	{group: "Действия", label: "Цель", hint: "/goal · F3 правка · F5 сохранить",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openGoal() }},
	{group: "Действия", label: "Пресеты агента", hint: "Alt+A · Ctrl+N новая сессия",
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("presets") }},
	{group: "Действия", label: "Вопросы и разрешения", hint: "F7",
		enabled: paletteHasApprovals,
		run: func(m *model) tea.Cmd {
			m.approvals.visible = true
			m.rebuild()
			return nil
		}},
}

// paletteRow is one row of the flat list: a muted group heading or an item.
type paletteRow struct {
	kind  string // "heading" or "item"
	group string
	index int // index into paletteCatalog; -1 for headings
}

// paletteFiltered searches labels, groups and shortcuts in the current locale.
// Catalog labels are stable source keys; translation never happens at startup.
func (m *model) paletteFiltered() []paletteRow {
	p := m.palette
	query := strings.ToLower(strings.TrimSpace(p.search.Value()))
	rows := []paletteRow{}
	lastGroup := ""
	for i := range paletteCatalog {
		item := &paletteCatalog[i]
		if p.groupFilter != "" && item.group != p.groupFilter {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(clean(tr(item.label)+" "+tr(item.hint)+" "+tr(item.group))), query) {
			continue
		}
		if item.group != lastGroup {
			rows = append(rows, paletteRow{kind: "heading", group: item.group})
			lastGroup = item.group
		}
		rows = append(rows, paletteRow{kind: "item", group: item.group, index: i})
	}
	return rows
}

// paletteItemRows returns the flat-row indices of the item rows (headings
// are not navigable).
func paletteItemRows(rows []paletteRow) []int {
	indices := []int{}
	for i, row := range rows {
		if row.kind == "item" {
			indices = append(indices, i)
		}
	}
	return indices
}

// paletteMove shifts the selection by delta item rows with wrap-around,
// skipping heading rows.
func (m *model) paletteMove(delta int) {
	p := m.palette
	p.note = ""
	items := paletteItemRows(m.paletteFiltered())
	if len(items) == 0 {
		p.offset = 0
		return
	}
	pos := -1
	for k, row := range items {
		if row == p.offset {
			pos = k
			break
		}
	}
	if pos == -1 {
		p.offset = items[0]
		return
	}
	pos = (pos + delta + len(items)) % len(items)
	p.offset = items[pos]
}

// paletteClampOffset keeps the selection on a visible item row (kept when the
// row still matches, otherwise the first item).
func (m *model) paletteClampOffset() {
	p := m.palette
	items := paletteItemRows(m.paletteFiltered())
	valid := false
	for _, row := range items {
		if row == p.offset {
			valid = true
			break
		}
	}
	if !valid {
		if len(items) > 0 {
			p.offset = items[0]
		} else {
			p.offset = 0
		}
	}
}

// paletteCycleGroup cycles the group filter: "" -> first group -> ... -> "".
func (m *model) paletteCycleGroup(forward bool) {
	p := m.palette
	p.note = ""
	pos := 0
	for i, g := range paletteGroups {
		if g == p.groupFilter {
			pos = i + 1
			break
		}
	}
	delta := 1
	if !forward {
		delta = -1
	}
	pos = (pos + delta + len(paletteGroups) + 1) % (len(paletteGroups) + 1)
	if pos == 0 {
		p.groupFilter = ""
	} else {
		p.groupFilter = paletteGroups[pos-1]
	}
	m.paletteClampOffset()
}

// paletteMayOpen keeps modal editors and pending decisions in charge of focus.
// A page workbench is safe: its search focus is saved and restored explicitly.
func (m *model) paletteMayOpen() bool {
	return m.runtimeDialog == nil && (m.approvals == nil || !m.approvals.visible) &&
		!m.queueDialog.visible && m.settingsForm == nil && (m.composer == nil || !m.composer.visible) &&
		m.goalDialog == nil && m.modelPicker == nil && m.historyReader == nil &&
		m.archiveDialog == nil && m.renameDialog == nil && m.forkDialog == nil &&
		m.chrome.dialog == "" && !m.chrome.menuOpen && m.marketConfirm == nil &&
		(m.workbench == nil || (m.workbench.pageView && !m.workbench.editing && !m.workbench.confirm))
}

func (m *model) paletteClose() {
	p := m.palette
	if p == nil || !p.open {
		return
	}
	p.open = false
	p.search.Blur()
	if p.inputFocused {
		m.input.Focus()
	}
	if p.marketFocused {
		m.marketSearch.Focus()
	}
	if p.sessionFocused {
		m.sessionSearch.Focus()
	}
	if p.benchFocused && p.bench != nil && m.workbench == p.bench {
		p.bench.search.Focus()
	}
}

// Execute exactly the selected action. Disabled entries remain visible and do
// not redirect the user's intent to a different command.
func (m *model) paletteExecute() tea.Cmd {
	p := m.palette
	rows := m.paletteFiltered()
	if p.offset < 0 || p.offset >= len(rows) || rows[p.offset].kind != "item" {
		return nil
	}
	item := paletteCatalog[rows[p.offset].index]
	if item.enabled != nil && !item.enabled(m) {
		p.note = tr("Действие сейчас недоступно.")
		return nil
	}
	m.paletteClose()
	return item.run(m)
}

func (m *model) paletteKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.palette == nil {
		m.palette = &paletteState{}
	}
	p := m.palette
	if !p.open {
		if msg.String() != "ctrl+p" || msg.Paste {
			return nil, false
		}
		if !m.paletteMayOpen() {
			return nil, true
		}
		p.inputFocused, p.marketFocused = m.input.Focused(), m.marketSearch.Focused()
		p.sessionFocused = m.sessionSearch.Focused()
		p.bench, p.benchFocused = m.workbench, m.workbench != nil && m.workbench.search.Focused()
		m.input.Blur()
		m.marketSearch.Blur()
		m.sessionSearch.Blur()
		if p.benchFocused {
			p.bench.search.Blur()
		}
		p.search = textinput.New()
		p.search.Prompt = ""
		p.search.Placeholder = tr("Найти команду…")
		p.search.CharLimit = 128
		p.search.Width = m.paletteBox().width - 4
		p.groupFilter, p.note, p.offset, p.open = "", "", 0, true
		m.paletteClampOffset()
		return p.search.Focus(), true
	}
	// Bracketed paste is always text, even when its contents resemble a key.
	if msg.Paste {
		var cmd tea.Cmd
		p.search, cmd = p.search.Update(msg)
		p.note = ""
		m.paletteClampOffset()
		return cmd, true
	}
	switch msg.String() {
	case "ctrl+c", "ctrl+d":
		m.paletteClose()
		return tea.Quit, true
	case "f8":
		m.paletteClose()
		return m.cancelChat(), true
	case "f7":
		m.paletteClose()
		return nil, false
	case "ctrl+p", "esc":
		m.paletteClose()
		return nil, true
	case "tab", "shift+tab":
		m.paletteCycleGroup(msg.String() == "tab")
	case "ctrl+u":
		p.search.SetValue("")
		m.paletteClampOffset()
	case "up":
		m.paletteMove(-1)
	case "down":
		m.paletteMove(1)
	case "pgup", "ctrl+pgup":
		m.paletteMove(-palettePageSize)
	case "pgdown", "ctrl+pgdown":
		m.paletteMove(palettePageSize)
	case "home", "end":
		items := paletteItemRows(m.paletteFiltered())
		if len(items) > 0 {
			p.offset = items[0]
			if msg.String() == "end" {
				p.offset = items[len(items)-1]
			}
		}
	case "enter":
		return m.paletteExecute(), true
	default:
		var cmd tea.Cmd
		p.search, cmd = p.search.Update(msg)
		p.note = ""
		m.paletteClampOffset()
		return cmd, true
	}
	return nil, true
}

func (m *model) paletteBox() area {
	w, h := min(62, m.width-4), min(20, m.height-4)
	return area{(m.width - 1 - w) / 2, max(1, (m.height-h)/3), w, h}
}

// The same row window is used for rendering and mouse hit testing.
func (m *model) paletteWindow() (int, int) {
	count := max(1, m.paletteBox().height-7)
	rows := m.paletteFiltered()
	first := max(0, m.palette.offset-count+1)
	first = min(first, max(0, len(rows)-count))
	return first, count
}

func (m *model) paletteMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if m.palette == nil || !m.palette.open {
		return nil, false
	}
	if msg.Action != tea.MouseActionPress {
		return nil, true
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.paletteMove(-1)
		return nil, true
	case tea.MouseButtonWheelDown:
		m.paletteMove(1)
		return nil, true
	case tea.MouseButtonLeft:
	default:
		return nil, true
	}
	box := m.paletteBox()
	if !box.contains(msg.X, msg.Y) {
		return nil, true
	}
	if msg.Y == box.y+1 && msg.X >= box.x+box.width-6 {
		m.paletteClose()
		return nil, true
	}
	first, count := m.paletteWindow()
	line := msg.Y - box.y - 5
	rows := m.paletteFiltered()
	if line >= 0 && line < count && first+line < len(rows) && rows[first+line].kind == "item" {
		m.palette.offset = first + line
		return m.paletteExecute(), true
	}
	return nil, true
}

func (m *model) paletteView(base string) string {
	if m.palette == nil || !m.palette.open || !m.terminalFits() {
		return base
	}
	p := m.palette
	box := m.paletteBox()
	inner := box.width - 4
	p.search.Width = inner
	p.search.Placeholder = tr("Найти команду…")
	rows := m.paletteFiltered()
	title := bar(heading().Render(tr("Команды")), muted().Render("esc"), inner)
	subtitle := fmt.Sprintf(tr("Найдено: %d"), len(paletteItemRows(rows)))
	if p.groupFilter != "" {
		subtitle += " · " + tr(p.groupFilter)
	}
	lines := []string{"", title, p.search.View(), muted().Render(subtitle), ""}
	first, count := m.paletteWindow()
	for i := first; i < min(len(rows), first+count); i++ {
		row := rows[i]
		if row.kind == "heading" {
			lines = append(lines, lipgloss.NewStyle().Foreground(theme.Color(theme.Secondary)).Render(tr(row.group)))
			continue
		}
		item := paletteCatalog[row.index]
		text := muted().Render(bar(tr(item.label), tr(item.hint), inner))
		if item.enabled != nil && !item.enabled(m) {
			text = muted().Render(bar(tr(item.label), "—", inner))
		}
		if i == p.offset {
			text = lipgloss.NewStyle().Width(inner).Foreground(theme.Color(theme.SelectionText)).Background(theme.Color(theme.Selection)).Render(ansi.Strip(text))
		}
		lines = append(lines, text)
	}
	for len(lines) < box.height-2 {
		lines = append(lines, "")
	}
	footer := tr("↑↓ выбрать · Enter открыть · Tab группа")
	if p.note != "" {
		footer = p.note
	}
	lines = append(lines, muted().Render(footer), "")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], inner, "…")
	}
	front := lipgloss.NewStyle().Width(box.width).Height(box.height).Padding(0, 2).Foreground(theme.Color(theme.Text)).Background(theme.Color(theme.Panel)).Render(strings.Join(lines, "\n"))
	return overlay(dimBackdrop(base), surface(front, theme.Panel), box.x, box.y, m.width-1)
}
