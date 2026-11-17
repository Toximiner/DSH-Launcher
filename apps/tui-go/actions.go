package main

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

type actionScope uint8

const (
	actionBase actionScope = iota
	actionChat
	actionChatSettings
)

// Registry actions cover no-argument commands and safe base-screen shortcuts.
// Context-specific editing keys and argumentful commands remain with their
// existing parsers; they are not global aliases and must not be guessed here.
type actionDescriptor struct {
	id, group, label, hint string
	aliases, keys          []string
	scope                  actionScope
	hidden                 bool
	enabled                func(*model) bool
	disabledReason         string
	helpContext            string
	run                    func(*model) tea.Cmd
}

var actionRegistry = []actionDescriptor{
	{id: "view.text-selection", group: "Навигация", label: "Выделить текст терминалом", hint: "/select · F10", aliases: []string{"/select"}, keys: []string{"f10"}, scope: actionBase,
		helpContext: "Весь интерфейс", enabled: func(m *model) bool { return m.terminalFits() },
		run: func(m *model) tea.Cmd { return func() tea.Msg { return textSelectionStart{} } }},
	// Сессии
	{id: "session.new", group: "Сессии", label: "Новая сессия", hint: "/new · Ctrl+n", aliases: []string{"/new"}, keys: []string{"ctrl+n"}, scope: actionChat,
		enabled: paletteHasURL,
		run:     func(m *model) tea.Cmd { return m.sessionOperation("new", m.sidebarNewWorkspace()) }},
	{id: "session.list", group: "Сессии", label: "Список сессий", hint: "/sessions · Alt+s", aliases: []string{"/sessions"}, keys: []string{"alt+s"}, scope: actionBase,
		enabled: paletteHasURL,
		run:     func(m *model) tea.Cmd { return m.openSessionPicker() }},
	{id: "session.rename", group: "Сессии", label: "Переименовать текущую", hint: "/rename", aliases: []string{"/rename"}, keys: []string{}, scope: actionChat,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openSessionRename(true) }},
	{id: "session.fork", group: "Сессии", label: "Ветвление (fork)", hint: "/fork", aliases: []string{"/fork"}, keys: []string{}, scope: actionChat,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openSessionFork(true) }},
	{id: "session.archive", group: "Сессии", label: "Архивировать", hint: "/archive", aliases: []string{"/archive"}, keys: []string{}, scope: actionChat,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openSessionArchive(true, false) }},
	{id: "session.restore", group: "Сессии", label: "Восстановить из архива", hint: "/restore", aliases: []string{"/restore"}, keys: []string{}, scope: actionChat,
		enabled: paletteHasURL,
		run:     func(m *model) tea.Cmd { return m.openSessionArchive(true, true) }},
	{id: "session.parent", group: "Сессии", label: "К родителю (подагент)", hint: "/parent", aliases: []string{"/parent"}, keys: []string{}, scope: actionChat,
		enabled: paletteIsSubagent, disabledReason: "У текущей сессии нет адреса родителя.",
		run: func(m *model) tea.Cmd { return m.sessionOperation("connect", m.session.Address.ParentID) }},
	{id: "mode.default", group: "Сессии", label: "Режим: обычный", hint: "/mode default", aliases: []string{}, keys: []string{}, scope: actionChat,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.chooseMode("default") }},
	{id: "mode.plan", group: "Сессии", label: "Режим: план", hint: "/plan", aliases: []string{"/plan"}, keys: []string{}, scope: actionChat,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.chooseMode("plan") }},
	{id: "mode.goal", group: "Сессии", label: "Режим: цель", hint: "/mode goal", aliases: []string{}, keys: []string{}, scope: actionChat,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.chooseMode("goal") }},
	// Навигация
	{id: "navigation.chat", group: "Навигация", label: "Чат", hint: "Alt+c", aliases: []string{}, keys: []string{"alt+c"}, scope: actionBase,
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.showChat() }},
	{id: "settings.dsh", group: "Навигация", label: "Настройки dsh", hint: "Alt+d", aliases: []string{}, keys: []string{"alt+d"}, scope: actionBase,
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
	{id: "settings.tui", group: "Навигация", label: "Настройки TUI", hint: "/settings · F4 · Alt+t", aliases: []string{"/settings"}, keys: []string{"f4", "alt+t"}, scope: actionBase,
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.openDialog("settings") }},
	{id: "navigation.log", group: "Навигация", label: "Журнал", hint: "Alt+l", aliases: []string{}, keys: []string{"alt+l"}, scope: actionBase,
		enabled: func(*model) bool { return true },
		run: func(m *model) tea.Cmd {
			m.page, m.navFocused = logPage, false
			m.input.Blur()
			m.marketSearch.Blur()
			m.resize()
			m.viewport.GotoTop()
			return nil
		}},
	{id: "navigation.context", group: "Навигация", label: "Показать/скрыть контекст", hint: "", aliases: []string{}, keys: []string{}, scope: actionChat,
		enabled: func(m *model) bool { return m.page == chatPage },
		run: func(m *model) tea.Cmd {
			m.contextPanelHidden = !m.contextPanelHidden
			m.resize()
			return nil
		}},
	{id: "runtime.manage", group: "Навигация", label: "Установка и dsh", hint: "/runtime", aliases: []string{"/runtime"}, keys: []string{}, scope: actionBase,
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.openRuntime("runtime") }},
	{id: "runtime.updates", group: "Навигация", label: "Обновления и откат", hint: "/updates", aliases: []string{"/updates"}, keys: []string{}, scope: actionBase,
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.openRuntime("updates") }},
	{id: "runtime.diagnostics", group: "Навигация", label: "Диагностика", hint: "/diagnostics", aliases: []string{"/diagnostics"}, keys: []string{}, scope: actionBase,
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.openRuntime("diagnostics") }},
	{id: "help.shortcuts", helpContext: "Основной экран и настройки TUI", group: "Навигация", label: "Горячие клавиши", hint: "F1", aliases: []string{}, keys: []string{"f1"}, scope: actionBase,
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.openDialog("help") }},
	{id: "help.about", group: "Навигация", label: "О программе", hint: "/about", aliases: []string{"/about"}, keys: []string{}, scope: actionBase,
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.openDialog("about") }},
	{id: "application.quit", helpContext: "Весь интерфейс", group: "Навигация", label: "Выход", hint: "/exit · Ctrl+c · Ctrl+d", aliases: []string{"/exit", "/quit"}, keys: []string{"ctrl+c", "ctrl+d"}, scope: actionBase,
		enabled: func(*model) bool { return true },
		run:     func(*model) tea.Cmd { return tea.Quit }},
	// Настройки
	{id: "settings.permission", group: "Настройки", label: "Доступ по умолчанию", hint: "F5 — сохранить", aliases: []string{}, keys: []string{}, scope: actionBase,
		enabled: paletteWritable,
		run:     func(m *model) tea.Cmd { return m.settingsHubAction("permission-default") }},
	{id: "settings.language", group: "Настройки", label: "Язык интерфейса", hint: "RU / EN", aliases: []string{}, keys: []string{}, scope: actionBase,
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.togglePreference(6) }},
	{id: "settings.theme", group: "Настройки", label: "Оформление (тема)", hint: "тёмная / светлая", aliases: []string{}, keys: []string{}, scope: actionBase,
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.togglePreference(4) }},
	{id: "settings.font", group: "Настройки", label: "Размер шрифта", hint: "терминал", aliases: []string{}, keys: []string{}, scope: actionBase,
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.settingsHubAction("font") }},
	{id: "settings.mouse", group: "Настройки", label: "Мышь в интерфейсе", hint: "вкл / выкл", aliases: []string{}, keys: []string{}, scope: actionBase,
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.togglePreference(0) }},
	{id: "settings.input-history", group: "Настройки", label: "История ввода", hint: "вкл / выкл", aliases: []string{}, keys: []string{}, scope: actionBase,
		enabled: func(*model) bool { return true },
		run:     func(m *model) tea.Cmd { return m.togglePreference(1) }},
	{id: "session.model", group: "Настройки", label: "Модель сессии", hint: "/models · F12", aliases: []string{"/models"}, keys: []string{"f12"}, scope: actionChatSettings,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openModels() }},
	{id: "settings.providers", group: "Настройки", label: "Провайдеры", hint: "/providers", aliases: []string{"/providers"}, keys: []string{}, scope: actionBase,
		enabled: paletteWritable,
		run:     func(m *model) tea.Cmd { return m.openSettingsForm(true) }},
	{id: "session.commands", group: "Настройки", label: "Команды агента", hint: "/commands", aliases: []string{"/commands"}, keys: []string{}, scope: actionChat,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("commands") }},
	// Плагины и маркет
	{id: "plugins.list", group: "Плагины и маркет", label: "Плагины dsh (страница)", hint: "/plugins", aliases: []string{"/plugins"}, keys: []string{}, scope: actionBase,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("plugins") }},
	{id: "plugins.market", group: "Плагины и маркет", label: "Маркет (страница)", hint: "/market · Alt+m", aliases: []string{"/market"}, keys: []string{"alt+m"}, scope: actionBase,
		enabled: paletteHasURL,
		run: func(m *model) tea.Cmd {
			m.page, m.navFocused = marketPage, false
			m.input.Blur()
			m.resize()
			return m.loadMarket()
		}},
	{id: "plugins.packages", group: "Плагины и маркет", label: "Пакеты dsh", hint: "/packages", aliases: []string{"/packages"}, keys: []string{}, scope: actionBase,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("packages") }},
	// Автоматизация
	{id: "automation.schedules", group: "Автоматизация", label: "Расписания", hint: "/schedules", aliases: []string{"/schedules"}, keys: []string{}, scope: actionBase,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("schedules") }},
	{id: "automation.tasks", group: "Автоматизация", label: "Задачи команды", hint: "/tasks", aliases: []string{"/tasks"}, keys: []string{}, scope: actionBase,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("tasks") }},
	{id: "automation.jobs", group: "Автоматизация", label: "Фоновые задания", hint: "/jobs", aliases: []string{"/jobs"}, keys: []string{}, scope: actionBase,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("jobs") }},
	{id: "automation.agents", group: "Автоматизация", label: "Подагенты", hint: "/agents", aliases: []string{"/agents"}, keys: []string{}, scope: actionBase,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("agents") }},
	{id: "workspace.terminals", group: "Автоматизация", label: "Терминалы", hint: "/terminals", aliases: []string{"/terminals"}, keys: []string{}, scope: actionBase,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("terminals") }},
	{id: "workspace.list", group: "Автоматизация", label: "Рабочие области", hint: "/workspaces", aliases: []string{"/workspaces"}, keys: []string{}, scope: actionBase,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkspacePicker() }},
	{id: "account.manage", group: "Автоматизация", label: "Аккаунт DeepSeek", hint: "/account", aliases: []string{"/account"}, keys: []string{}, scope: actionBase,
		enabled: paletteHasURL,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("account") }},
	{id: "plugins.operations", group: "Автоматизация", label: "Журнал операций пакетов", hint: "/operations", aliases: []string{"/operations"}, keys: []string{}, scope: actionBase,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("operations") }},
	{id: "workspace.files", group: "Автоматизация", label: "Файлы рабочей области", hint: "/files", aliases: []string{"/files"}, keys: []string{}, scope: actionBase,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("files") }},
	// Действия
	{id: "agent.stop", helpContext: "Весь интерфейс", group: "Действия", label: "Остановить агента", hint: "/stop · F8", aliases: []string{"/stop"}, keys: []string{"f8"}, scope: actionBase,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.cancelChat() }},
	{id: "message.retry", group: "Действия", label: "Повторить неопределённую отправку", hint: "/retry-send", aliases: []string{"/retry-send"}, keys: []string{}, scope: actionChat,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.retryPrompt() }},
	{id: "message.queue", group: "Действия", label: "Очередь сообщений", hint: "/queue · F9", aliases: []string{"/queue"}, keys: []string{"f9"}, scope: actionChat,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openQueue() }},
	{id: "message.compose", group: "Действия", label: "Многострочный редактор", hint: "/compose · Alt+e", aliases: []string{"/compose"}, keys: []string{"alt+e"}, scope: actionChat,
		enabled: paletteCompose,
		run:     func(m *model) tea.Cmd { return m.openCompose() }},
	{id: "conversation.clear", group: "Действия", label: "Очистить экран", hint: "/clear", aliases: []string{"/clear"}, keys: []string{}, scope: actionChat,
		enabled: func(*model) bool { return true },
		run: func(m *model) tea.Cmd {
			m.lines = nil
			if m.chat != nil {
				m.chat.clearedThrough = m.chat.cursor
			}
			m.rebuild()
			return nil
		}},
	{id: "conversation.history", group: "Действия", label: "История сессии", hint: "/history · F11", aliases: []string{"/history"}, keys: []string{"f11"}, scope: actionChat,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openHistory() }},
	{id: "conversation.thoughts", group: "Действия", label: "Показать/скрыть размышления", hint: "/thoughts · Ctrl+o", aliases: []string{"/thoughts"}, keys: []string{"ctrl+o"}, scope: actionChat,
		enabled: func(m *model) bool { return m.hasReasoning() },
		run:     func(m *model) tea.Cmd { return m.toggleReasoning() }},
	{id: "conversation.copy", group: "Действия", label: "Копировать последний ответ", hint: "Ctrl+y", aliases: []string{}, keys: []string{"ctrl+y"}, scope: actionChat,
		enabled: func(m *model) bool { return m.conversationVisible() && paletteLastAssistantSeq(m) >= 0 },
		run:     func(m *model) tea.Cmd { return m.copyConversation(paletteLastAssistantSeq(m)) }},
	{id: "session.goal", group: "Действия", label: "Цель", hint: "/goal", aliases: []string{"/goal"}, keys: []string{}, scope: actionChat,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openGoal() }},
	{id: "session.presets", group: "Действия", label: "Пресеты агента", hint: "/presets · Alt+a", aliases: []string{"/presets"}, keys: []string{"alt+a"}, scope: actionChat,
		enabled: paletteHasSession,
		run:     func(m *model) tea.Cmd { return m.openWorkbench("presets") }},
	{id: "agent.approvals", helpContext: "Весь интерфейс", group: "Действия", label: "Вопросы и разрешения", hint: "F7", aliases: []string{}, keys: []string{"f7"}, scope: actionBase,
		enabled: paletteHasApprovals,
		run: func(m *model) tea.Cmd {
			m.approvals.visible = true
			m.rebuild()
			return nil
		}},

	{id: "session.manage", group: "Сессии", label: "Управление сессиями", hint: "/sessions-manager", aliases: []string{"/sessions-manager"}, scope: actionBase, enabled: paletteHasURL, run: func(m *model) tea.Cmd { m.page = sessionsPage; m.resize(); return m.sessionOperation("list", "") }},
	{id: "workspace.manage", group: "Автоматизация", label: "Управление рабочими областями", hint: "/workspaces-manager", aliases: []string{"/workspaces-manager"}, scope: actionBase, enabled: paletteHasURL, run: func(m *model) tea.Cmd { return m.openWorkbench("workspaces") }},

	{id: "mode.toggle-plan", group: "Сессии", label: "Режим: обычный / план", keys: []string{"f2"}, scope: actionChat, hidden: true, enabled: paletteHasSession, run: func(m *model) tea.Cmd {
		if m.session.Plan != nil && m.session.Plan.Active {
			return m.chooseMode("default")
		}
		return m.chooseMode("plan")
	}},
	{id: "help.commands", group: "Навигация", label: "Справка по командам", aliases: []string{"/help"}, scope: actionBase, hidden: true, run: func(m *model) tea.Cmd { m.append(help); return nil }},
}

func paletteActions() []paletteItem {
	var result []paletteItem
	for _, group := range paletteGroups {
		for _, action := range actionRegistry {
			if !action.hidden && action.group == group {
				result = append(result, action)
			}
		}
	}
	return result
}

func actionDisabledReason(m *model, action actionDescriptor) string {
	if action.disabledReason != "" {
		return tr(action.disabledReason)
	}
	if m.settingsLogin == "" {
		return tr("Сначала подключитесь к dsh.")
	}
	if m.session == nil {
		return tr("Сначала выберите сессию dsh.")
	}
	return tr("Действие сейчас недоступно.")
}
func actionEnabled(m *model, action actionDescriptor) bool {
	return action.enabled == nil || action.enabled(m)
}
func (m *model) runRegisteredAction(action actionDescriptor) tea.Cmd {
	if !actionEnabled(m, action) {
		m.append(actionDisabledReason(m, action))
		return nil
	}
	return action.run(m)
}

// Exact aliases only: argumentful commands stay in the established parser.
func (m *model) dispatchSlashAction(line string) (tea.Cmd, bool) {
	if len(strings.Fields(line)) != 1 || line != strings.TrimSpace(line) {
		return nil, false
	}
	for _, action := range actionRegistry {
		for _, alias := range action.aliases {
			if alias == line {
				return m.runRegisteredAction(action), true
			}
		}
	}
	return nil, false
}

func (m *model) actionKeyContextSafe() bool {
	if !m.terminalFits() || !m.paletteMayOpen() || m.palette != nil && m.palette.open {
		return false
	}
	// Safe page workbenches allow base navigation after their own local keys have
	// been handled. Their editors, pending writes and embedded panels are gated
	// by paletteMayOpen above; chat-scoped bindings do not fire on those pages.
	return !m.marketEditing && !m.sessionSearching
}
func (m *model) dispatchKeyAction(msg tea.KeyMsg) (tea.Cmd, bool) {
	if msg.Paste || !m.actionKeyContextSafe() {
		return nil, false
	}
	// Bubble Tea String() also returns literal runes. A pasted/burst string named
	// "ctrl+d" is text, never a request to quit the application.
	if msg.Type == tea.KeyRunes && (!msg.Alt || len(msg.Runes) != 1) {
		return nil, false
	}
	for _, action := range actionRegistry {
		if action.scope == actionChat && (m.page != chatPage || m.navFocused || m.toolFocused) {
			continue
		}
		if action.scope == actionChatSettings && m.page != chatPage && m.page != dshSettingsPage {
			continue
		}
		for _, key := range action.keys {
			if key == msg.String() {
				return m.runRegisteredAction(action), true
			}
		}
	}
	return nil, false
}

func actionKeyLabel(key string) string {
	parts := strings.Split(key, "+")
	for i, part := range parts {
		switch part {
		case "ctrl":
			parts[i] = "Ctrl"
		case "alt":
			parts[i] = "Alt"
		case "shift":
			parts[i] = "Shift"
		default:
			if strings.HasPrefix(part, "f") {
				parts[i] = strings.ToUpper(part)
			} else if len([]rune(part)) == 1 {
				parts[i] = strings.Map(unicode.ToUpper, part)
			}
		}
	}
	return strings.Join(parts, "+")
}
func actionShortcuts() []shortcut {
	var result []shortcut
	for _, action := range actionRegistry {
		keys := []string{}
		for _, key := range action.keys {
			keys = append(keys, actionKeyLabel(key))
		}
		keys = append(keys, action.aliases...)
		if len(keys) == 0 {
			continue
		}
		context := "Основной экран"
		if action.scope == actionChat {
			context = "Чат"
		}
		if action.scope == actionChatSettings {
			context = "Чат и настройки dsh"
		}
		if action.helpContext != "" {
			context = action.helpContext
		}
		result = append(result, shortcut{strings.Join(keys, " / "), action.label, context})
	}
	return result
}

func actionPaletteHint(action actionDescriptor) string {
	parts := []string{}
	if len(action.aliases) > 0 {
		parts = append(parts, action.aliases[0])
	}
	for _, key := range action.keys {
		parts = append(parts, actionKeyLabel(key))
	}
	if len(parts) > 0 {
		return strings.Join(parts, " · ")
	}
	return tr(action.hint)
}
