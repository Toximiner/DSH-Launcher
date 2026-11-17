// A panel-based terminal client with read-only dsh settings.
package main

import (
	"context"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

const version = "0.2.0-preview.40-opencode.4"
const help = "Ctrl+P — палитра команд: все страницы, настройки и действия\n" + "/runtime — установка и процесс dsh\n/updates — обновления и откат dsh/TUI\n/diagnostics — журнал, отчёт, уведомления\n" + "/compose — многострочное сообщение\n/attach <путь> — прикрепить файл до 2 МиБ\n/workspaces · /files · /terminals — рабочая область\n/presets — пресеты · Ctrl+N новая сессия\n/plugins · /commands · /packages — плагины\n/agents · /tasks · /jobs · /schedules — выполнение\n/parent — вернуться от подагента к родителю\nВ списках: F5 обновить · Esc назад\n" + "/market — маркет плагинов\n/new — новая сессия · /sessions — быстрый выбор\n/sessions-manager — управление сессиями\n/workspaces-manager — управление областями\n/session <id> — подключить сессию\n/rename — название текущей · F3 в списке\n/archive · /restore — архив и возврат\nВ списке: a — архив · Ctrl+X — перенести\n/fork — новая ветка · Ctrl+B в списке\n/models — выбор модели сессии\n/providers — настройки провайдеров\nВ настройках: Enter — форма · F6 — провайдеры\n/mode default|plan|goal — режим\n/goal — состояние цели · F3 правка\n/goal <цель>|pause|resume|clear — работа с целью\n/stop — остановить агента · F8\n/retry-send — повторить неопределённую отправку\n/history — история сессии\nCtrl+F в истории — поиск · F3 — ещё\nCtrl+T в поиске — всё / сообщения / инструменты\nTab в истории — выбор · Ctrl+B — ветка\nCtrl+R в истории — Markdown/исходник\nВ истории: Ctrl+Y — копия · Ctrl+K — код\nF6/Shift+F6 — инструмент истории · Enter — детали\n/help — справка\n/clear — очистить экран\n/exit — выйти\nF5 — отправить ответы в окне вопросов\nF7 — вопросы и разрешения\nF9 — очередь · F3 правка · Ctrl+G уточнить\nF5 — сохранить задание в редакторе\nF6 — инструменты · Enter — раскрыть · Esc — ввод\n↑/↓ — история ввода · PgUp/PgDn — прокрутка\nCtrl+Tab / Shift+Tab — сессии · Ctrl+N — новая сессия\nМышь: клик — раздел или поле ввода · колесо — прокрутка"

const (
	chatPage = iota
	sessionsPage
	dshSettingsPage
	tuiSettingsPage
	logPage
	marketPage
	pluginsPage
	schedulesPage
)

var pages = []string{"Чат", "Сессии", "Настройки dsh", "Настройки TUI", "Журнал", "Маркет", "Плагины", "Автоматизация"}

type quitMsg struct{}

type model struct {
	sessionPicker       *sessionPicker
	contextPanelHidden  bool
	modelRevision       int
	agentInfo           *agentContext
	side                sidebarState
	runtime             runtimeControl
	runtimeState        runtimeSnapshot
	runtimeDialog       *runtimeDialog
	composer            *composeDialog
	workbench           *workbench
	goalDialog          *goalDialog
	settingsForm        *settingsForm
	modelPicker         *modelPicker
	copyClipboard       func(context.Context, string) (string, error)
	markdown            *markdownCache
	archive             *archiveState
	archiveDialog       *sessionArchive
	showArchived        bool
	forkDialog          *sessionFork
	renameDialog        *sessionRename
	titleRevision       int
	historyReader       *historyReader
	queueDialog         queueDialog
	approvals           *approvalState
	chat                *chatState
	chatToolHits        []chatToolHit
	toolFocused         bool
	chatEpoch           int
	prompts             map[string]*pendingPrompt
	chrome              chromeState
	tabState            sessionTabsState
	palette             *paletteState
	prefs               clientPreferences
	mouseForcedOff      bool
	savePreferences     func(clientPreferences) error
	preferencesNote     string
	input               textinput.Model
	viewport            viewport.Model
	width, height       int
	lines               []string
	history             []string
	historyIndex        int
	draft               string
	workspace           string
	page                int
	navFocused          bool
	events              []string
	settingsLogin       string
	settingsContext     context.Context
	settingsLoading     bool
	settingsError       string
	settings            *settingsSnapshot
	settingsIndex       int
	settingsUpdated     time.Time
	session             *sessionView
	sessionItems        []sessionItem
	sessionBusy         bool
	sessionAction       string
	sessionError        string
	sessionSequence     int
	sessionStale        bool
	sessionListIndex    int
	sessionQuery        string
	sessionSearch       textinput.Model
	sessionSearching    bool
	sessionHits         []sessionHit
	sessionQueue        []sessionAction
	initialSession      string
	createSession       bool
	market              *marketSnapshot
	marketSearch        textinput.Model
	marketEditing       bool
	marketInstalledOnly bool
	marketIndex         int
	marketBusy          bool
	marketOperating     bool
	marketSequence      int
	marketError         string
	marketMessage       string
	marketConfirm       *marketIntent
	marketHits          []marketHit
}

func newModel() *model {
	theme.SetLight(false)
	activeEnglish.Store(false)
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = tr("Сообщение или /help…")
	input.CharLimit = 65536
	input.TextStyle = lipgloss.NewStyle().Foreground(theme.Color(theme.Text))
	input.PlaceholderStyle = lipgloss.NewStyle().Foreground(theme.Color(theme.Muted))
	input.PromptStyle = lipgloss.NewStyle().Foreground(theme.Color(theme.Accent))
	input.Focus()
	cwd, _ := os.Getwd()
	m := &model{prefs: defaultClientPreferences(), input: input, viewport: viewport.New(78, 17), width: 80, height: 24, workspace: clean(cwd), events: []string{tr("Открыт интерфейс клиента. Подключение к dsh отсутствует.")}}
	m.marketSearch = newMarketSearch()
	m.prompts = make(map[string]*pendingPrompt)
	m.resize()
	return m
}

func clean(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, ansi.Strip(value))
}

func (m *model) rebuild() {
	followTail := m.chat != nil && m.page == chatPage && m.viewport.AtBottom()
	m.syncConversationGeometry()
	var lines []string
	for _, line := range m.lines {
		label := tr("СПРАВКА")
		text := line
		if strings.HasPrefix(line, tr("Введено: ")) {
			label = tr("ВЫ · ЛОКАЛЬНЫЙ ЧЕРНОВИК")
			text = strings.TrimPrefix(line, tr("Введено: "))
		}
		lines = append(lines, accent().Bold(true).Render(label)+"\n"+ansi.Hardwrap(clean(text), max(1, m.viewport.Width), true)+"\n")
	}
	content := strings.Join(lines, "\n")
	if len(m.lines) == 0 {
		welcome := heading().Render(tr("С чего начнём?")) + "\n\n" + muted().Render(tr("Нет подключения к dsh.")) + "\n" +
			tr("Здесь появится разговор с агентом.\n\n") + muted().Render(tr("Пока можно проверить ввод и навигацию.")) + "\n\n" + accent().Render(tr("F1 — справка   Tab — разделы"))
		content = lipgloss.Place(m.viewport.Width, m.viewport.Height, lipgloss.Center, lipgloss.Center, welcome)
	}
	if m.session != nil || m.sessionBusy || m.sessionError != "" {
		if m.chat != nil {
			content = m.chatView()
			if len(m.lines) > 0 {
				content += "\n\n" + strings.Join(m.lines, "\n")
			}
		}
		if len(m.lines) == 0 {
			if m.chat == nil {
				content = tr("Ожидание подключения чата. /new — создать сессию.\n/mode default|plan|goal · /goal <описание>")
			}
		}
		prefix := accent().Render(m.modeDescription()) + "\n\n"
		if m.session != nil && m.session.Address.Kind != "subagent" && len(m.session.PendingInput) == 0 && (m.session.Plan == nil || !m.session.Plan.Pending) && m.session.Goal == nil && m.sessionError == "" && !m.isSessionArchived(m.session.ID) {
			prefix = ""
		}
		for i := range m.chatToolHits {
			offset := strings.Count(ansi.Hardwrap(prefix, max(1, m.viewport.Width), true), "\n")
			m.chatToolHits[i].row += offset
			m.chatToolHits[i].end += offset
		}
		content = prefix + content
	}
	switch m.page {
	case sessionsPage:
		content = m.sessionInfo()
	case dshSettingsPage:
		content = m.settingsView()
	case marketPage:
		content = m.marketView()
	case logPage:
		content = heading().Render(tr("Журнал клиента · локально")) + "\n\n" + strings.Join(m.events, "\n")
	}
	m.viewport.SetContent(ansi.Hardwrap(content, max(1, m.viewport.Width), true))
	if followTail {
		m.viewport.GotoBottom()
	}
}

func (m *model) append(line string) {
	m.lines = append(m.lines, clean(line))
	if len(m.lines) > 1000 {
		m.lines = m.lines[len(m.lines)-1000:]
	}
	m.rebuild()
	m.viewport.GotoBottom()
}

func (m *model) resize() {
	followTail := m.page == chatPage && m.viewport.AtBottom()
	if m.page != chatPage {
		m.toolFocused = false
	}
	w, h := m.contentSize()
	if m.page == chatPage {
		w = m.conversationLayout().chat.width
		h -= conversationComposerHeight
	}
	m.viewport.Width = max(1, w-4)
	m.viewport.Height = max(1, h-4)
	if m.page == chatPage {
		m.viewport.Height = max(1, h)
	}
	if m.page == marketPage {
		m.viewport.Width = min(w, 110)
		m.viewport.Height = max(1, h)
	}
	m.input.Width = max(1, w-6)
	m.marketSearch.Width = max(1, m.viewport.Width-8)
	m.rebuild()
	if followTail {
		m.viewport.GotoBottom()
	}
}

func (m *model) contentSize() (int, int) {
	w := max(1, m.width-1)
	if m.sidebarVisible() {
		w -= sidebarWidth + sidebarGap
	}
	return w, max(1, m.height-3)
}

func (m *model) showChat() tea.Cmd {
	m.page = 0
	m.navFocused = false
	m.toolFocused = false
	m.resize()
	return m.input.Focus()
}

func (m *model) Init() tea.Cmd {
	var session tea.Cmd
	if m.createSession {
		session = m.sessionOperation("new", "")
	} else if m.initialSession != "" {
		session = m.sessionOperation("connect", m.initialSession)
	}
	var poll tea.Cmd
	if m.runtime != nil {
		poll = runtimePoll()
	}
	return tea.Batch(textinput.Blink, m.loadSettings(), m.loadSidebar(), sidebarPoll(), session, poll)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case conversationCopyResult:
		m.handleConversationCopy(msg)
		return m, nil
	case agentContextResult:
		m.handleAgentContext(msg)
		return m, nil
	case sidebarTick:
		return m, tea.Batch(m.loadSidebar(), m.loadAgentContext(), sidebarPoll())
	case sidebarResult:
		m.handleSidebarResult(msg)
		return m, m.ensureArchive()
	case runtimeTick:
		return m, m.handleRuntimeTick()
	case runtimeDone:
		m.runtimeState = m.runtime.Snapshot()
		if msg.err != nil {
			m.runtimeState.Note = redactDiagnostic(msg.err.Error())
			if m.runtimeDialog != nil {
				m.runtimeDialog.note = m.runtimeState.Note
			}
		}
		return m, nil
	case approvalUpdate:
		return m, m.handleApprovalUpdate(msg)
	case approvalAnswerResult:
		m.handleApprovalAnswer(msg)
		return m, nil
	case chatUpdate:
		return m, m.handleChatUpdate(msg)
	case settingsFormResult:
		m.handleSettingsForm(msg)
		return m, nil
	case sessionPickerResult:
		return m, m.handleSessionPickerResult(msg)
	case modelPickerResult:
		m.handleModelPicker(msg)
		return m, nil
	case composeUpload:
		m.handleComposeUpload(msg)
		return m, nil
	case packageCancelResult:
		m.handlePackageCancel(msg)
		return m, nil
	case benchTerminalDone:
		if m.workbench != nil {
			m.workbench.note = tr("Терминал отключён. Ввод не повторяется; F5 — перечитать.")
			if msg.err != nil {
				m.workbench.note = tr("Поток терминала прерван. Ввод не повторяется; F5 — перечитать.")
			}
		}
		return m, nil
	case benchResult:
		return m, m.handleBench(msg)
	case goalDialogResult:
		m.handleGoalDialog(msg)
		return m, nil
	case historyCopyResult:
		m.handleHistoryCopy(msg)
		return m, nil
	case historyResult:
		m.handleHistoryResult(msg)
		return m, nil
	case historySearchResult:
		m.handleHistorySearch(msg)
		return m, nil
	case toolOutputUpdate:
		return m, m.handleToolOutput(msg)
	case promptResult:
		m.handlePromptResult(msg)
		if m.session != nil && m.session.ID == msg.sessionID && !m.sessionBusy {
			return m, m.sessionOperation("refresh", "")
		}
		return m, nil
	case queueSteerResult:
		return m, m.handleQueueSteering(msg)
	case queueEditResult:
		return m, m.handleQueueEdit(msg)
	case queueRemoveResult:
		return m, m.handleQueueRemoval(msg)
	case cancelResult:
		if m.chat != nil && m.chat.id == msg.sessionID {
			if m.chat.note == tr("Выполнение остановлено.") {
				return m, nil
			}
			if msg.err != nil {
				m.chat.note = tr("Не удалось подтвердить отмену. Проверьте состояние dsh.")
			} else {
				m.chat.note = tr("Запрос отмены принят dsh; состояние обновляется.")
			}
			m.rebuild()
		}
		return m, nil
	case marketResult:
		if msg.sequence != m.marketSequence {
			return m, nil
		}
		m.marketBusy = false
		m.marketOperating = false
		if msg.snapshot != nil {
			if msg.snapshot.Plugins == nil && m.market != nil && msg.snapshot.Profile == m.market.Profile {
				msg.snapshot.Plugins = m.market.Plugins
			}
			m.market = msg.snapshot
		}
		if msg.err != nil {
			m.marketError = msg.err.Error()
		} else {
			m.marketError = ""
			if msg.operation {
				m.marketMessage = tr("Операция завершена; список обновлён.")
			} else {
				m.marketMessage = tr("Данные маркета обновлены.")
			}
		}
		m.rebuild()
		return m, nil
	case marketPulse:
		if msg.sequence != m.marketSequence || !m.marketOperating {
			return m, nil
		}
		ctx, login, sequence := m.settingsContext, m.settingsLogin, m.marketSequence
		return m, func() tea.Msg {
			api, err := openAPI(ctx, login)
			if err != nil {
				return marketProgress{sequence: sequence, err: err}
			}
			defer api.close()
			var status marketStatus
			err = api.marketRequest(ctx, "GET", "status", nil, &status)
			return marketProgress{sequence: sequence, status: status, err: err}
		}
	case marketProgress:
		if msg.sequence != m.marketSequence || !m.marketOperating {
			return m, nil
		}
		if msg.err == nil && m.market != nil {
			m.market.Status = msg.status
		}
		m.rebuild()
		return m, m.marketTimer()
	case archiveUpdate:
		return m, m.handleArchiveUpdate(msg)
	case sessionArchiveResult:
		return m, m.handleSessionArchive(msg)
	case sessionForkResult:
		return m, m.handleSessionFork(msg)
	case sessionRenameResult:
		return m, m.handleSessionRename(msg)
	case sessionTick:
		if msg.sequence == m.sessionSequence && m.session != nil && !m.sessionBusy {
			return m, m.sessionOperation("refresh", "")
		}
		return m, nil
	case sessionResult:
		if msg.sequence != m.sessionSequence {
			return m, nil
		}
		m.sessionBusy = false
		if msg.err != nil {
			m.sessionError = msg.err.Error()
			m.sessionStale = msg.view == nil
		} else {
			m.sessionError = ""
			m.sessionStale = false
		}
		var chatCmd tea.Cmd
		if msg.view != nil {
			if m.session != nil && m.chat != nil && m.chat.id == msg.view.ID && m.session.ID == msg.view.ID && msg.view.StateCursor < m.chat.cursor {
				msg.view.Running = m.session.Running
			}
			if m.session == nil || m.session.ID != msg.view.ID {
				m.lines = nil
			}
			if msg.modelRevision != m.modelRevision && m.session != nil && m.session.ID == msg.view.ID {
				msg.view.Model = m.session.Model
			}
			if msg.titleRevision != m.titleRevision && m.session != nil && m.session.ID == msg.view.ID {
				msg.view.Title = m.session.Title
			}
			m.tabSessionChanged(msg.view)
			m.session = msg.view
			chatCmd = m.startChat(msg.view.ID)
			m.syncContinuedQuestions()
			m.reconcileQueuedPrompt()
		}
		if msg.list {
			if msg.titleRevision != m.titleRevision {
				for i := range msg.items {
					for _, current := range m.sessionItems {
						if current.ID == msg.items[i].ID {
							msg.items[i].Projections.Values.Title = current.Projections.Values.Title
						}
					}
				}
			}
			m.sessionItems = msg.items
			m.tabRefreshItems(msg.items)
			chatCmd = tea.Batch(chatCmd, m.ensureArchive())
			m.sessionListIndex = 0
		}
		m.rebuild()
		if msg.list {
			m.revealSelectedSession()
		}
		if msg.err != nil && len(m.sessionQueue) > 0 {
			m.sessionQueue = nil
			m.append(tr("Ожидающие запросы не выполнены из-за ошибки dsh."))
		}
		if len(m.sessionQueue) > 0 {
			next := m.sessionQueue[0]
			m.sessionQueue = m.sessionQueue[1:]
			nextCmd := m.sessionOperation(next.action, next.value)
			if chatCmd != nil {
				return m, tea.Batch(chatCmd, nextCmd)
			}
			return m, nextCmd
		}
		if m.session != nil {
			if chatCmd != nil {
				return m, tea.Batch(chatCmd, m.sessionTimer())
			}
			return m, m.sessionTimer()
		}
		return m, nil
	case settingsResult:
		m.settingsLoading = false
		if msg.err != nil {
			m.settingsError = msg.err.Error()
		} else {
			selected := ""
			if m.settings != nil && len(m.settings.Namespaces) > 0 {
				selected = m.settings.Namespaces[m.settingsIndex].NS
			}
			m.settings = msg.snapshot
			m.settingsIndex = 0
			m.settingsUpdated = time.Now()
			for i, ns := range m.settings.Namespaces {
				if ns.NS == selected {
					m.settingsIndex = i
					break
				}
			}
		}
		m.rebuild()
		return m, nil
	case quitMsg:
		m.stopChat()
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		m.resizeHistory()
	case tea.MouseMsg:
		return m, m.mouse(msg)
	case tea.KeyMsg:
		if !m.terminalFits() {
			switch msg.Type {
			case tea.KeyCtrlC, tea.KeyCtrlD:
				return m, tea.Quit
			case tea.KeyF8:
				return m, m.cancelChat()
			}
			return m, nil
		}
		if cmd, handled := m.sessionPickerKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.paletteKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.runtimeKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.approvalKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.queueKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.settingsFormKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.composeKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.benchKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.goalKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.modelsKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.historyCopyKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.sessionSearchKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.archiveKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.forkKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.renameKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.dispatchKeyAction(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.chromeKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.historyKey(msg); handled {
			return m, cmd
		}
		if msg.Type == tea.KeyF8 {
			return m, m.cancelChat()
		}
		if cmd, handled := m.conversationKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.agentKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.sidebarKey(msg); handled {
			return m, cmd
		}
		if cmd, handled := m.toolKey(msg); handled {
			return m, cmd
		}
		// A burst of ordinary text is not a named control key such as "enter".
		if msg.Type == tea.KeyRunes && !msg.Alt && len(msg.Runes) > 1 {
			if m.page == marketPage && !m.navFocused {
				if !m.marketEditing && !msg.Paste && m.marketConfirm == nil && msg.Runes[0] == '/' {
					cmd, _ := m.marketKey(msg)
					return m, cmd
				}
				if !m.marketEditing {
					return m, nil
				}
				msg.Paste = true
				cmd, _ := m.marketKey(msg)
				return m, cmd
			}
			// Bubbles also matches key names by String; mark this burst as literal text.
			msg.Paste = true
			if m.page == chatPage && !m.navFocused {
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				return m, cmd
			}
			return m, nil
		}

		if cmd, handled := m.tabKey(msg); handled {
			return m, cmd
		}
		if m.page == marketPage && !m.navFocused && msg.String() != "ctrl+c" && msg.String() != "ctrl+d" && msg.String() != "tab" {
			if cmd, handled := m.marketKey(msg); handled {
				return m, cmd
			}
		}
		switch msg.String() {
		case "left", "right":
			if m.page == dshSettingsPage && m.settings != nil && len(m.settings.Namespaces) > 0 {
				delta := 1
				if msg.String() == "left" {
					delta = -1
				}
				m.settingsIndex = (m.settingsIndex + delta + len(m.settings.Namespaces)) % len(m.settings.Namespaces)
				m.rebuild()
				m.viewport.GotoTop()
				return m, nil
			}
		case "r":
			if m.page == sessionsPage {
				return m, m.sessionOperation("list", "")
			}
			if m.page == dshSettingsPage {
				return m, m.loadSettings()
			}
		case "ctrl+c", "ctrl+d":
			return m, tea.Quit
		case "esc":
			return m, m.showChat()
		case "enter":
			if m.page == sessionsPage && !m.navFocused && len(m.visibleSessions()) > 0 {
				id := m.visibleSessions()[m.sessionListIndex].ID
				m.showChat()
				return m, m.sessionOperation("connect", id)
			}
			if m.navFocused {
				m.navFocused = false
				if m.page == marketPage {
					return m, m.loadMarket()
				}
				if m.page == 0 {
					return m, m.input.Focus()
				}
				return m, nil
			}
			if m.page != 0 {
				return m, nil
			}
			line := strings.TrimSpace(m.input.Value())
			m.input.SetValue("")
			if line == "" {
				return m, nil
			}
			if m.prefs.InputHistory && (len(m.history) == 0 || m.history[len(m.history)-1] != line) {
				m.history = append(m.history, line)
			}
			if len(m.history) > 50 {
				m.history = m.history[len(m.history)-50:]
			}
			m.historyIndex = len(m.history)
			if cmd, handled := m.dispatchSlashAction(line); handled {
				return m, cmd
			}
			switch line {
			case "/plan off":
				return m, m.chooseMode("default")
			default:
				if strings.HasPrefix(line, "/session ") {
					return m, m.sessionOperation("connect", strings.TrimSpace(strings.TrimPrefix(line, "/session ")))
				}
				if strings.HasPrefix(line, "/mode ") {
					mode := strings.TrimSpace(strings.TrimPrefix(line, "/mode "))
					if mode != "default" && mode != "plan" && mode != "goal" {
						m.append(tr("Режим: default, plan или goal."))
						return m, nil
					}
					return m, m.chooseMode(mode)
				}
				if strings.HasPrefix(line, "/attach ") {
					return m, m.uploadCompose(strings.TrimSpace(strings.TrimPrefix(line, "/attach ")))
				}
				if strings.HasPrefix(line, "/goal ") {
					value := strings.TrimSpace(strings.TrimPrefix(line, "/goal "))
					action := "goal"
					if value == "pause" || value == "resume" || value == "clear" {
						action = value
						value = ""
					}
					return m, m.sessionOperation(action, value)
				}
				if strings.HasPrefix(line, "/") {
					m.append(tr("Неизвестная команда. /help — список команд."))
					return m, nil
				}
				return m, m.submitText(line)
			}
			return m, nil
		case "up", "down":

			if m.page == sessionsPage && len(m.visibleSessions()) > 0 {
				delta := 1
				if msg.String() == "up" {
					delta = -1
				}
				m.sessionListIndex = (m.sessionListIndex + delta + len(m.visibleSessions())) % len(m.visibleSessions())
				m.rebuild()
				return m, nil
			}
			if m.page != 0 {
				m.viewport, _ = m.viewport.Update(msg)
				return m, nil
			}
			if !m.prefs.InputHistory {
				return m, nil
			}
			if m.historyIndex == len(m.history) {
				m.draft = m.input.Value()
			}
			if msg.String() == "up" {
				m.historyIndex = max(0, m.historyIndex-1)
			} else {
				m.historyIndex = min(len(m.history), m.historyIndex+1)
			}
			if m.historyIndex == len(m.history) {
				m.input.SetValue(m.draft)
			} else {
				m.input.SetValue(m.history[m.historyIndex])
			}
			m.input.CursorEnd()
			return m, nil
		case "pgup", "pgdown", "ctrl+pgup", "ctrl+pgdown":
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}
	if d := m.renameDialog; d != nil {
		var cmd tea.Cmd
		d.editor, cmd = d.editor.Update(msg)
		return m, cmd
	}
	if h := m.historyReader; h != nil && h.search != nil && h.search.editing {
		var cmd tea.Cmd
		h.search.editor, cmd = h.search.editor.Update(msg)
		return m, cmd
	}
	if cmd, handled := m.questionMessage(msg); handled {
		return m, cmd
	}
	if m.navFocused || m.page != 0 {
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}
