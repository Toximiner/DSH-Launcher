package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type sidebarWorkspace struct {
	ID       string   `json:"workspaceId"`
	Title    string   `json:"title"`
	Path     string   `json:"path"`
	Sessions []string `json:"sessionIds"`
}
type sidebarState struct {
	activeWorkspace string
	reveal          bool
	workspaces      []sidebarWorkspace
	sessions        []sessionItem
	expanded        map[string]bool
	selected        string
	offset          int
	login           string
	sequence        int
	busy, failed    bool
}
type sidebarResult struct {
	titleRevision int
	login         string
	sequence      int
	workspaces    []sidebarWorkspace
	sessions      []sessionItem
	err           error
}
type sidebarTick struct{}
type sidebarItem struct{ key, label, action, value string }
type sidebarRow struct {
	sidebarItem
	y int
}

func sidebarPoll() tea.Cmd {
	return tea.Tick(10*time.Second, func(time.Time) tea.Msg { return sidebarTick{} })
}

func (m *model) loadSidebar() tea.Cmd {
	s := &m.side
	if s.login != m.settingsLogin {
		s.login = m.settingsLogin
		s.workspaces, s.sessions = nil, nil
		s.busy, s.failed = false, false
		s.expanded = nil
		s.activeWorkspace = ""
		s.selected = ""
		s.offset = 0
	}
	if s.login == "" || s.busy {
		return nil
	}
	s.busy = true
	s.sequence++
	login, sequence, titleRevision := s.login, s.sequence, m.titleRevision
	parent := m.settingsContext
	if parent == nil {
		parent = context.Background()
	}
	return func() tea.Msg {
		result := sidebarResult{login: login, sequence: sequence, titleRevision: titleRevision}
		ctx, cancel := context.WithTimeout(parent, 8*time.Second)
		defer cancel()
		api, err := openAPI(ctx, login)
		if err != nil {
			result.err = err
			return result
		}
		defer api.close()
		// Closing the snapshot socket retires its API operator. Finish HTTP reads first.
		var list struct {
			Items []sessionItem `json:"items"`
		}
		err = api.rpc(ctx, "session/list", map[string]any{"_request": map[string]any{}}, &list)
		if err != nil {
			result.err = err
			return result
		}
		raw, err := firstBenchFrame(ctx, api, "workspace/follow", map[string]any{})
		var snapshot struct {
			Type  string `json:"type"`
			Value struct {
				Items []sidebarWorkspace `json:"items"`
			} `json:"value"`
		}
		if err == nil {
			err = json.Unmarshal(raw, &snapshot)
			if err == nil && snapshot.Type != "baseline" {
				err = errors.New("invalid workspace baseline")
			}
		}
		if err != nil {
			result.err = err
			return result
		}

		result.workspaces, result.sessions = snapshot.Value.Items, list.Items
		return result
	}
}
func (m *model) handleSidebarResult(r sidebarResult) {
	s := &m.side
	if r.login != m.settingsLogin || r.login != s.login || r.sequence != s.sequence {
		return
	}
	s.busy, s.failed = false, r.err != nil
	if r.err != nil {
		return
	}
	first := s.expanded == nil
	if first {
		s.expanded = map[string]bool{}
	}
	if r.titleRevision != m.titleRevision {
		confirmed := map[string]string{}
		for _, item := range s.sessions {
			confirmed[item.ID] = item.Projections.Values.Title
		}
		for i := range r.sessions {
			if title, ok := confirmed[r.sessions[i].ID]; ok {
				r.sessions[i].Projections.Values.Title = title
			}
		}
	}
	s.workspaces, s.sessions = r.workspaces, r.sessions
	if first && m.session != nil {
		for _, w := range s.workspaces {
			for _, id := range w.Sessions {
				if id == m.session.ID {
					s.expanded[w.ID] = true
				}
			}
		}
	}
}

var sidebarTools = []menuEntry{
	{"Сессии", "sessions"}, {"Файлы", "files"}, {"Терминалы", "terminals"},
	{"Пресеты", "presets"}, {"Команды", "commands"}, {"Пакеты", "packages"},
	{"Подагенты", "agents"}, {"Задачи команды", "tasks"}, {"Задания", "jobs"},
}
var sidebarMore = []menuEntry{
	{"Настройки TUI", "settings"}, {"Настройки dsh", "dsh-settings"},
	{"Аккаунт DeepSeek", "account"}, {"Горячие клавиши", "help"},
	{"Диагностика…", "diagnostics"}, {"О программе", "about"},
}

func sidebarDialogEntries(kind string) []menuEntry {
	if kind == "conversation" {
		return conversationMenu
	}
	if kind == "navigation" {
		return sidebarTools
	}
	if kind == "more" {
		return sidebarMore
	}
	return nil
}
func (m *model) sidebarSections() ([]sidebarItem, []sidebarItem, sidebarItem) {
	top := []sidebarItem{
		{"new", "⊕ " + tr("Новая сессия"), "new", ""},
		{"plugins", "+ " + tr("Плагины"), "plugins", ""},
		{"market", "▦ " + tr("Маркет плагинов"), "market", ""},
		{"schedules", "◷ " + tr("Автоматизация"), "schedules", ""},
		{"navigation", "▦ " + tr("Ещё"), "navigation", ""},
		{"workspaces", tr("Рабочие области") + "  ⌕", "workspaces", ""},
	}
	var tree []sidebarItem
	titles := map[string]string{}
	for _, item := range m.side.sessions {
		titles[item.ID] = item.Projections.Values.Title
	}
	if m.session != nil {
		titles[m.session.ID] = m.session.Title
	}
	for _, w := range m.side.workspaces {
		name := clean(w.Title)
		if name == "" {
			name = filepath.Base(clean(w.Path))
		}
		mark := "▸ "
		if m.side.expanded[w.ID] {
			mark = "▾ "
		}
		tree = append(tree, sidebarItem{"workspace:" + w.ID, mark + name, "workspace", w.ID})
		if !m.side.expanded[w.ID] {
			continue
		}
		for _, id := range w.Sessions {
			if m.isSessionArchived(id) {
				continue
			}
			name := clean(titles[id])
			if name == "" {
				name = id
			}
			mark := "  · "
			if m.session != nil && m.session.ID == id {
				mark = "  ● "
			}
			tree = append(tree, sidebarItem{"session:" + id, mark + name, "session", id})
		}
	}
	if len(m.side.workspaces) == 0 {
		tree = append(tree, sidebarItem{"local", "▾ " + filepath.Base(m.workspace), "chat", ""})
		name := tr("Локальный черновик")
		if m.session != nil {
			name = clean(m.session.Title)
			if name == "" {
				name = m.session.ID
			}
		}
		tree = append(tree, sidebarItem{"chat", "  ● " + name, "chat", ""})
	}
	return top, tree, sidebarItem{"more", "⋯ " + tr("Ещё"), "more", ""}
}
func (m *model) sidebarItems() []sidebarItem {
	top, tree, bottom := m.sidebarSections()
	return append(append(top, tree...), bottom)
}
func (m *model) sidebarRows(height int) []sidebarRow {
	top, tree, bottom := m.sidebarSections()
	rows := []sidebarRow{}
	for i, item := range top {
		y := i + 1
		if i > 0 {
			y++
		}
		if i == len(top)-1 {
			y++
		}
		rows = append(rows, sidebarRow{item, y})
	}
	start := 9
	count := max(1, height-2-4-start)
	m.side.offset = min(max(0, m.side.offset), max(0, len(tree)-count))
	if m.navFocused && m.side.reveal {
		for i, item := range tree {
			if item.key == m.side.selected {
				if i < m.side.offset {
					m.side.offset = i
				}
				if i >= m.side.offset+count {
					m.side.offset = i - count + 1
				}
			}
		}
	}
	m.side.reveal = false
	end := min(len(tree), m.side.offset+count)
	for i := m.side.offset; i < end; i++ {
		rows = append(rows, sidebarRow{tree[i], start + i - m.side.offset})
	}
	rows = append(rows, sidebarRow{bottom, height - 3})
	return rows
}
func (m *model) sidebar(height int) string {
	const inner = sidebarWidth - 4
	lines := make([]string, max(1, height-2))
	for _, row := range m.sidebarRows(height) {
		text := ansi.Truncate(strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(clean(row.label)), inner, "…")
		selected := m.navFocused && row.key == m.side.selected
		active := row.action == "schedules" && m.page == schedulesPage || row.action == "plugins" && m.page == pluginsPage || row.action == "market" && m.page == marketPage || row.action == "session" && m.session != nil && m.session.ID == row.value || row.key == "chat" && m.page == chatPage
		if selected || active {
			text = lipgloss.NewStyle().Width(inner).Bold(true).Foreground(uiColor("#79c0ff")).Background(uiColor("#233044")).Render(text)
		}
		lines[row.y] = text
	}
	status := tr("○ dsh не подключён")
	if m.settings != nil {
		status = tr("✓ Снимок настроек dsh")
	}
	if m.session != nil {
		status = "✓ dsh · " + m.modeLabel()
	}
	if m.side.failed {
		status = tr("! Рабочие области · F5")
	}
	lines[len(lines)-4] = muted().Render(ansi.Truncate(status, inner, "…"))
	lines[len(lines)-3] = muted().Render("v" + version)
	border := "#303d4d"
	if m.navFocused {
		border = "#58a6ff"
	}
	return panel(sidebarWidth, height, strings.Join(lines, "\n"), border)
}
func (m *model) activateSidebar(item sidebarItem) tea.Cmd {
	m.toolFocused = false
	switch item.action {
	case "workspace":
		if m.side.expanded == nil {
			m.side.expanded = map[string]bool{}
		}
		m.side.expanded[item.value] = !m.side.expanded[item.value]
		m.side.activeWorkspace = item.value
		return nil
	case "new":
		m.showChat()
		return m.sessionOperation("new", m.sidebarNewWorkspace())
	case "session":
		for _, w := range m.side.workspaces {
			for _, id := range w.Sessions {
				if id == item.value {
					m.side.activeWorkspace = w.ID
				}
			}
		}
		focus := m.showChat()
		if m.session != nil && m.session.ID == item.value {
			return focus
		}
		return tea.Batch(focus, m.sessionOperation("connect", item.value))
	case "chat":
		return m.showChat()
	case "navigation", "more":
		return m.openDialog(item.action)
	case "settings", "help", "about":
		return m.openDialog(item.action)
	case "diagnostics":
		return m.openRuntime(item.action)
	case "market", "sessions", "dsh-settings":
		m.navFocused = false
		m.input.Blur()
		m.marketEditing = false
		m.marketSearch.Blur()
		switch item.action {
		case "market":
			m.page = marketPage
		case "sessions":
			m.page = sessionsPage
		default:
			m.page = dshSettingsPage
		}
		m.resize()
		m.viewport.GotoTop()
		if item.action == "market" {
			return m.loadMarket()
		}
		if item.action == "sessions" {
			return m.sessionOperation("list", "")
		}
		return nil
	default:
		m.navFocused = false
		if m.page == chatPage {
			m.input.Focus()
		}
		return m.openWorkbench(item.action)
	}
}
func (m *model) sidebarKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !m.navFocused || !m.sidebarVisible() {
		return nil, false
	}
	if msg.Paste {
		return nil, true
	}
	items := m.sidebarItems()
	index := 5
	for i, item := range items {
		if item.key == m.side.selected {
			index = i
			break
		}
	}
	switch msg.Type {
	case tea.KeyUp, tea.KeyDown, tea.KeyHome, tea.KeyEnd:
		switch msg.Type {
		case tea.KeyUp:
			index = (index + len(items) - 1) % len(items)
		case tea.KeyDown:
			index = (index + 1) % len(items)
		case tea.KeyHome:
			index = 0
		case tea.KeyEnd:
			index = len(items) - 1
		}
		m.side.selected = items[index].key
		m.side.reveal = true
		return nil, true
	case tea.KeyLeft, tea.KeyRight:
		item := items[index]
		if item.action == "workspace" {
			expand := msg.Type == tea.KeyRight
			if m.side.expanded[item.value] != expand {
				return m.activateSidebar(item), true
			}
		}
		return nil, true
	case tea.KeyEnter:
		return m.activateSidebar(items[index]), true
	case tea.KeyF5:
		return m.loadSidebar(), true
	case tea.KeyCtrlF:
		return m.activateSidebar(sidebarItem{action: "workspaces"}), true
	}
	return nil, false
}
func (m *model) sidebarMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if !m.sidebarVisible() || msg.X < 1 || msg.X >= sidebarWidth-1 {
		return nil, false
	}
	_, height := m.contentSize()
	if msg.Y <= bodyTop || msg.Y >= bodyTop+height-1 {
		return nil, false
	}
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		delta := 1
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		m.side.offset += delta
		return nil, true
	}
	if msg.Button != tea.MouseButtonLeft {
		return nil, true
	}
	for _, row := range m.sidebarRows(height) {
		if msg.Y == bodyTop+1+row.y {
			m.side.selected = row.key
			m.navFocused = true
			m.input.Blur()
			return m.activateSidebar(row.sidebarItem), true
		}
	}
	return nil, true
}

func (m *model) activateSidebarDialog() tea.Cmd {
	entries := sidebarDialogEntries(m.chrome.dialog)
	entry := entries[m.chrome.selection]
	conversation := m.chrome.dialog == "conversation"
	m.closeChrome()
	if conversation {
		m.navFocused = false
		m.input.Focus()
		return m.agentControl(entry.action)
	}
	return m.activateSidebar(sidebarItem{action: entry.action})
}

func (m *model) sidebarNewWorkspace() string {
	for _, w := range m.side.workspaces {
		if w.ID == m.side.activeWorkspace {
			return w.ID
		}
	}
	if m.session != nil {
		for _, w := range m.side.workspaces {
			for _, id := range w.Sessions {
				if id == m.session.ID {
					return w.ID
				}
			}
		}
	}
	return ""
}
