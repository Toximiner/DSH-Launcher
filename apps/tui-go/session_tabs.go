package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

const sessionTabRow = 0

// Open tabs are navigation state, not a projection of a filtered session catalog.
type sessionTabsState struct {
	items  []sessionItem
	drafts map[string]string
}

type sessionTabHit struct {
	area
	id  string
	new bool
}

type sessionTabLayout struct {
	view string
	hits []sessionTabHit
}

// Call only for an accepted session result, before replacing m.session.
func (m *model) tabSessionChanged(next *sessionView) {
	if next == nil || next.ID == "" {
		return
	}
	previous := m.activeTabID()
	if previous != next.ID {
		if m.tabState.drafts == nil {
			m.tabState.drafts = make(map[string]string)
		}
		if previous != "" {
			m.tabState.drafts[previous] = m.input.Value()
			m.input.SetValue(m.tabState.drafts[next.ID])
			m.input.CursorEnd()
			m.historyIndex = len(m.history)
			m.draft = m.input.Value()
		}
	}
	// Retain a session first opened before the tab state was initialized.
	if previous != "" {
		m.tabRemember(m.session)
	}
	m.tabRemember(next)
}

func (m *model) tabRemember(view *sessionView) {
	if view == nil || view.ID == "" {
		return
	}
	for i := range m.tabState.items {
		if m.tabState.items[i].ID == view.ID {
			m.tabState.items[i].Running = view.Running
			m.tabState.items[i].Projections.Values.Title = view.Title
			return
		}
	}
	item := sessionItem{ID: view.ID, Running: view.Running}
	item.Projections.Values.Title = view.Title
	m.tabState.items = append(m.tabState.items, item)
}

// Catalog filtering, reordering, and incomplete pages must not close open tabs.
func (m *model) tabRefreshItems(items []sessionItem) {
	for i := range m.tabState.items {
		for _, item := range items {
			if item.ID == m.tabState.items[i].ID {
				m.tabState.items[i] = item
				break
			}
		}
	}
}

func (m *model) tabList() []sessionItem {
	tabs := append([]sessionItem(nil), m.tabState.items...)
	if m.session == nil || m.session.ID == "" {
		return tabs
	}
	for i := range tabs {
		if tabs[i].ID == m.session.ID {
			tabs[i].Running = m.session.Running
			tabs[i].Projections.Values.Title = m.session.Title
			return tabs
		}
	}
	item := sessionItem{ID: m.session.ID, Running: m.session.Running}
	item.Projections.Values.Title = m.session.Title
	return append(tabs, item)
}

func (m *model) activeTabID() string {
	if m.session == nil {
		return ""
	}
	return m.session.ID
}

// Rendering and mouse routing consume this exact sequence of cells, including
// separators and running markers. Overflow always reveals the active session.
func (m *model) sessionTabLayout() sessionTabLayout {
	width := max(0, m.width-1)
	layout := sessionTabLayout{}
	var parts []string
	x := 0
	add := func(text string, style lipgloss.Style, id string, create bool) {
		remaining := width - x
		if remaining <= 0 {
			return
		}
		text = ansi.Truncate(text, remaining, "…")
		cells := lipgloss.Width(text)
		parts = append(parts, style.Render(text))
		if id != "" || create {
			layout.hits = append(layout.hits, sessionTabHit{area: area{x, sessionTabRow, cells, 1}, id: id, new: create})
		}
		x += cells
	}
	neutral := lipgloss.NewStyle().Foreground(theme.Color(theme.Muted))
	createLabel := " + " + tr("Новая") + " "
	fullWidth := width
	width = max(0, width-lipgloss.Width(createLabel))
	addCreate := func() {
		width = fullWidth
		add(createLabel, neutral, "", true)
	}
	tabs := m.tabList()
	if len(tabs) == 0 {
		addCreate()
		layout.view = strings.Join(parts, "")
		return layout
	}
	available := max(0, width-x-1)
	capacity := max(1, available/16)
	overflow := len(tabs) > capacity
	if overflow {
		capacity = max(1, (available-6)/16)
	}
	count := min(len(tabs), capacity)
	active := 0
	for i, tab := range tabs {
		if tab.ID == m.activeTabID() {
			active = i
			break
		}
	}
	start := max(0, min(active-count/2, len(tabs)-count))
	add(" ", neutral, "", false)
	if overflow {
		id := ""
		marker := "   "
		if start > 0 {
			id = tabs[start-1].ID
			marker = " ‹ "
		}
		add(marker, neutral, id, false)
	}
	reserved := 0
	if overflow {
		reserved = 3
	}
	perTab := max(1, min(24, (width-x-reserved)/count))
	for i := start; i < start+count; i++ {
		tab := tabs[i]
		label := strings.Join(strings.Fields(clean(tab.Projections.Values.Title)), " ")
		if label == "" {
			label = tab.ID
		}
		marker := ""
		if tab.Running {
			marker = " ●"
		}
		inner := max(0, perTab-2-lipgloss.Width(marker))
		text := " " + ansi.Truncate(label, inner, "…") + marker
		text += strings.Repeat(" ", max(0, perTab-lipgloss.Width(text)))
		style := neutral
		if tab.ID == m.activeTabID() {
			style = lipgloss.NewStyle().Foreground(theme.Color(theme.Text)).Background(theme.Color(theme.Active)).Bold(true)
		}
		add(text, style, tab.ID, false)
	}
	if overflow {
		id := ""
		marker := "   "
		if start+count < len(tabs) {
			id = tabs[start+count].ID
			marker = " › "
		}
		add(marker, neutral, id, false)
	}
	addCreate()
	layout.view = strings.Join(parts, "")
	return layout
}

func (m *model) tabRail() string { return m.sessionTabLayout().view }

func (m *model) tabHasModal() bool {
	return (m.palette != nil && m.palette.open) || m.runtimeDialog != nil || (m.workbench != nil && (!m.workbench.pageView || m.workbench.busy || m.workbench.editing || m.workbench.confirm || m.workbench.pluginPanel != nil)) || m.goalDialog != nil || m.settingsForm != nil || m.modelPicker != nil || m.archiveDialog != nil || m.forkDialog != nil || m.renameDialog != nil || m.historyReader != nil || m.queueDialog.visible || m.chrome.dialog != "" || m.chrome.menuOpen || (m.composer != nil && m.composer.visible) || (m.approvals != nil && m.approvals.visible)
}

func (m *model) tabRailMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress || msg.Y != sessionTabRow || m.tabHasModal() {
		return nil, false
	}
	for _, hit := range m.sessionTabLayout().hits {
		if hit.contains(msg.X, msg.Y) {
			if hit.new {
				return m.sessionOperation("new", m.sidebarNewWorkspace()), true
			}
			return m.switchTab(hit.id), true
		}
	}
	return nil, true
}

func (m *model) switchTab(id string) tea.Cmd {
	if id == "" || id == m.activeTabID() {
		return nil
	}
	m.showChat()
	return m.sessionOperation("connect", id)
}

func (m *model) tabKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if msg.Paste || m.page != chatPage || m.navFocused || m.toolFocused || m.tabHasModal() {
		return nil, false
	}
	switch msg.String() {
	case "ctrl+tab", "shift+tab":
		tabs := m.tabList()
		if len(tabs) < 2 {
			return nil, false
		}
		index := 0
		for i, item := range tabs {
			if item.ID == m.activeTabID() {
				index = i
				break
			}
		}
		delta := 1
		if msg.String() == "shift+tab" {
			delta = -1
		}
		return m.switchTab(tabs[(index+delta+len(tabs))%len(tabs)].ID), true
	case "ctrl+n":
		return m.sessionOperation("new", m.sidebarNewWorkspace()), true
	}
	return nil, false
}
