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
)

const version = "0.2.0-preview.7"
const help = "/market — маркет плагинов\n/new — новая сессия · /sessions — список\n/session <id> — подключить сессию\n/mode default|plan|goal — режим\n/goal <цель>|pause|resume|clear — работа с целью\n/help — справка\n/clear — очистить экран\n/exit — выйти\n↑/↓ — история ввода · PgUp/PgDn — прокрутка\nTab — боковая панель · Esc — вернуться в чат\nМышь: клик — раздел или поле ввода · колесо — прокрутка"

const (
	chatPage = iota
	sessionsPage
	dshSettingsPage
	tuiSettingsPage
	logPage
	marketPage
)

var pages = []string{"Чат", "Сессии", "Настройки dsh", "Настройки TUI", "Журнал", "Маркет"}

type quitMsg struct{}

type model struct {
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
	browserPreview      bool
	session             *sessionView
	sessionItems        []sessionItem
	sessionBusy         bool
	sessionError        string
	sessionSequence     int
	sessionStale        bool
	sessionListIndex    int
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
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "Сообщение или /help…"
	input.CharLimit = 65536
	input.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e6edf3"))
	input.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b949e"))
	input.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#58a6ff"))
	input.Focus()
	cwd, _ := os.Getwd()
	m := &model{input: input, viewport: viewport.New(78, 17), width: 80, height: 24, workspace: clean(cwd), events: []string{"Открыт интерфейс клиента. Подключение к dsh отсутствует."}}
	m.marketSearch = newMarketSearch()
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
	var lines []string
	for _, line := range m.lines {
		label := "СПРАВКА"
		text := line
		if strings.HasPrefix(line, "Введено: ") {
			label = "ВЫ · ЛОКАЛЬНЫЙ ЧЕРНОВИК"
			text = strings.TrimPrefix(line, "Введено: ")
		}
		lines = append(lines, accent.Bold(true).Render(label)+"\n"+ansi.Hardwrap(clean(text), max(1, m.viewport.Width), true)+"\n")
	}
	content := strings.Join(lines, "\n")
	if len(m.lines) == 0 {
		welcome := heading.Render("С чего начнём?") + "\n\n" + muted.Render("Нет подключения к dsh.") + "\n" +
			"Здесь появится разговор с агентом.\n\n" + muted.Render("Пока можно проверить ввод и навигацию.") + "\n\n" + accent.Render("F1 — справка   Tab — разделы")
		content = lipgloss.Place(m.viewport.Width, m.viewport.Height, lipgloss.Center, lipgloss.Center, welcome)
	}
	if m.session != nil || m.sessionBusy || m.sessionError != "" {
		if len(m.lines) == 0 {
			content = "Текст чата пока сохраняется локально.\nСмена режима применяется к подключённой сессии.\n/mode default|plan|goal · /goal <описание>"
		}
		content = accent.Render(m.modeDescription()) + "\n\n" + content
	}
	switch m.page {
	case sessionsPage:
		content = m.sessionInfo()
	case dshSettingsPage:
		content = m.settingsView()
	case tuiSettingsPage:
		content = m.tuiSettingsView()
	case marketPage:
		content = m.marketView()
	case logPage:
		content = heading.Render("Журнал клиента · локально") + "\n\n" + strings.Join(m.events, "\n")
	}
	m.viewport.SetContent(ansi.Hardwrap(content, max(1, m.viewport.Width), true))
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
	w, h := m.contentSize()
	if m.page == 0 {
		h -= composerHeight + 1
	}
	m.viewport.Width = max(1, w-4)
	m.viewport.Height = max(1, h-4)
	m.input.Width = max(1, w-6)
	m.marketSearch.Width = max(1, m.viewport.Width-8)
	m.rebuild()
}

func (m *model) contentSize() (int, int) {
	w := max(1, m.width-1)
	if w >= 78 {
		w -= sidebarWidth + sidebarGap
	}
	return w, max(1, m.height-3)
}

func (m *model) showChat() tea.Cmd {
	m.page = 0
	m.navFocused = false
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
	return tea.Batch(textinput.Blink, m.loadSettings(), session)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
				m.marketMessage = "Операция завершена; список обновлён."
			} else {
				m.marketMessage = "Данные маркета обновлены."
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
		if msg.view != nil {
			m.session = msg.view
		}
		if msg.list {
			m.sessionItems = msg.items
			m.sessionListIndex = 0
		}
		m.rebuild()
		if msg.err != nil && len(m.sessionQueue) > 0 {
			m.sessionQueue = nil
			m.append("Ожидающие запросы не выполнены из-за ошибки dsh.")
		}
		if len(m.sessionQueue) > 0 {
			next := m.sessionQueue[0]
			m.sessionQueue = m.sessionQueue[1:]
			return m, m.sessionOperation(next.action, next.value)
		}
		if m.session != nil {
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
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
	case tea.MouseMsg:
		return m, m.mouse(msg)
	case tea.KeyMsg:
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
		case "f2":
			if m.session != nil && m.session.Plan != nil && m.session.Plan.Active {
				return m, m.chooseMode("default")
			}
			return m, m.chooseMode("plan")
		case "alt+1":
			return m, m.chooseMode("default")
		case "alt+2":
			return m, m.chooseMode("plan")
		case "alt+3":
			return m, m.chooseMode("goal")
		case "ctrl+c", "ctrl+d":
			return m, tea.Quit
		case "tab":
			m.navFocused = !m.navFocused
			if m.navFocused || m.page != 0 {
				m.input.Blur()
				return m, nil
			}
			return m, m.input.Focus()
		case "esc":
			return m, m.showChat()
		case "f1":
			m.showChat()
			m.append(help)
			return m, nil
		case "enter":
			if m.page == sessionsPage && !m.navFocused && len(m.sessionItems) > 0 {
				id := m.sessionItems[m.sessionListIndex].ID
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
			if len(m.history) == 0 || m.history[len(m.history)-1] != line {
				m.history = append(m.history, line)
			}
			if len(m.history) > 50 {
				m.history = m.history[len(m.history)-50:]
			}
			m.historyIndex = len(m.history)
			switch line {
			case "/market":
				m.page = marketPage
				m.navFocused = false
				m.input.Blur()
				m.resize()
				return m, m.loadMarket()
			case "/new":
				return m, m.sessionOperation("new", "")
			case "/sessions":
				m.page = sessionsPage
				m.resize()
				return m, m.sessionOperation("list", "")
			case "/plan":
				return m, m.chooseMode("plan")
			case "/plan off":
				return m, m.chooseMode("default")
			case "/exit", "/quit":
				return m, tea.Quit
			case "/clear":
				m.lines = nil
				m.rebuild()
			case "/help":
				m.append(help)
			default:
				if strings.HasPrefix(line, "/session ") {
					return m, m.sessionOperation("connect", strings.TrimSpace(strings.TrimPrefix(line, "/session ")))
				}
				if strings.HasPrefix(line, "/mode ") {
					mode := strings.TrimSpace(strings.TrimPrefix(line, "/mode "))
					if mode != "default" && mode != "plan" && mode != "goal" {
						m.append("Режим: default, plan или goal.")
						return m, nil
					}
					return m, m.chooseMode(mode)
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
				m.append("Введено: " + line)
				m.events = append(m.events, "Сохранён локальный черновик: "+clean(line))
				if len(m.events) > 50 {
					m.events = m.events[len(m.events)-50:]
				}
			}
			return m, nil
		case "up", "down":
			if m.navFocused {
				delta := 1
				if msg.String() == "up" {
					delta = -1
				}
				m.page = (m.page + delta + len(pages)) % len(pages)
				m.resize()
				return m, nil
			}
			if m.page == sessionsPage && len(m.sessionItems) > 0 {
				delta := 1
				if msg.String() == "up" {
					delta = -1
				}
				m.sessionListIndex = (m.sessionListIndex + delta + len(m.sessionItems)) % len(m.sessionItems)
				m.rebuild()
				return m, nil
			}
			if m.page != 0 {
				m.viewport, _ = m.viewport.Update(msg)
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
	if m.navFocused || m.page != 0 {
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}
