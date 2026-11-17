package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type sessionPicker struct {
	ctx                          context.Context
	cancel                       context.CancelFunc
	login                        string
	sequence                     int
	titleRevision                int
	search                       textinput.Model
	group, workspaceID, note     string
	index                        int
	sessions                     []sessionItem
	workspaces                   []sidebarWorkspace
	busy, focused, marketFocused bool
}
type sessionPickerResult struct {
	owner                      *sessionPicker
	sequence, titleRevision    int
	login                      string
	sessions                   []sessionItem
	workspaces                 []sidebarWorkspace
	sessionsOK, workspacesOK   bool
	sessionsErr, workspacesErr error
}
type sessionPickerEntry struct{ kind, id, title, detail string }
type sessionPickerRow struct {
	entry int
	group string
}

func (m *model) openSessionPicker() tea.Cmd   { return m.openSessionPickerGroup("") }
func (m *model) openWorkspacePicker() tea.Cmd { return m.openSessionPickerGroup("workspaces") }
func (m *model) openSessionPickerGroup(group string) tea.Cmd {
	if m.sessionPicker != nil {
		m.closeSessionPicker()
	}
	parent := m.settingsContext
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	d := &sessionPicker{ctx: ctx, cancel: cancel, login: m.settingsLogin, group: group, search: textinput.New(), focused: m.input.Focused(), marketFocused: m.marketSearch.Focused()}
	d.search.Prompt = ""
	d.search.CharLimit = 512
	d.search.Placeholder = tr("Найти сессию или рабочую область…")
	if m.side.login == d.login && d.login != "" {
		d.sessions = append([]sessionItem(nil), m.side.sessions...)
		d.workspaces = append([]sidebarWorkspace(nil), m.side.workspaces...)
	}
	m.sessionPicker = d
	m.input.Blur()
	m.marketSearch.Blur()
	return tea.Batch(d.search.Focus(), m.loadSessionPicker())
}
func (m *model) closeSessionPicker() tea.Cmd {
	d := m.sessionPicker
	if d == nil {
		return nil
	}
	d.cancel()
	d.search.Blur()
	m.sessionPicker = nil
	if d.marketFocused && m.page == marketPage {
		return m.marketSearch.Focus()
	}
	if d.focused && m.page == chatPage && !m.navFocused {
		return m.input.Focus()
	}
	return nil
}
func (m *model) loadSessionPicker() tea.Cmd {
	d := m.sessionPicker
	if d == nil {
		return nil
	}
	if d.login != m.settingsLogin {
		d.cancel()
		parent := m.settingsContext
		if parent == nil {
			parent = context.Background()
		}
		d.ctx, d.cancel = context.WithCancel(parent)
		d.busy = false
		d.login = m.settingsLogin
		d.sessions = nil
		d.workspaces = nil
		d.index = 0
		d.workspaceID = ""
	}
	if d.busy {
		return nil
	}
	if d.login == "" {
		d.note = tr("Сессии dsh недоступны без подключения.")
		return nil
	}
	d.busy = true
	d.sequence++
	sequence, login, titleRevision := d.sequence, d.login, m.titleRevision
	parent := d.ctx
	return func() tea.Msg {
		result := sessionPickerResult{owner: d, sequence: sequence, login: login, titleRevision: titleRevision}
		ctx, cancel := context.WithTimeout(parent, 8*time.Second)
		defer cancel()
		api, err := openAPI(ctx, login)
		if err != nil {
			result.sessionsErr = err
			result.workspacesErr = err
			return result
		}
		defer api.close()
		var list struct {
			Items []sessionItem `json:"items"`
		}
		result.sessionsErr = api.rpc(ctx, "session/list", map[string]any{"_request": map[string]any{}}, &list)
		result.sessionsOK = result.sessionsErr == nil
		if result.sessionsOK {
			result.sessions = list.Items
		}
		// Finish HTTP reads before retiring the workspace snapshot operator.
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
		result.workspacesErr = err
		result.workspacesOK = err == nil
		if result.workspacesOK {
			result.workspaces = snapshot.Value.Items
		}
		return result
	}
}
func (m *model) handleSessionPickerResult(r sessionPickerResult) tea.Cmd {
	d := m.sessionPicker
	if d == nil || d != r.owner || r.sequence != d.sequence || r.login != d.login || r.login != m.settingsLogin || d.ctx.Err() != nil {
		return nil
	}
	entries := m.sessionPickerEntries()
	selected := ""
	if d.index >= 0 && d.index < len(entries) {
		selected = entries[d.index].kind + ":" + entries[d.index].id
	}
	d.busy = false
	d.note = ""
	if r.sessionsOK {
		if r.titleRevision != m.titleRevision {
			titles := map[string]string{}
			for _, s := range m.side.sessions {
				titles[s.ID] = s.Projections.Values.Title
			}
			if m.session != nil && m.session.Title != "" {
				titles[m.session.ID] = m.session.Title
			}
			for i := range r.sessions {
				if title, ok := titles[r.sessions[i].ID]; ok {
					r.sessions[i].Projections.Values.Title = title
				}
			}
		}
		d.sessions = r.sessions
	} else {
		d.note = tr("Сессии недоступны; F5 обновить.")
	}
	if r.workspacesOK {
		d.workspaces = r.workspaces
	} else {
		d.note += " " + tr("Рабочие области недоступны; F5 обновить.")
	}
	entries = m.sessionPickerEntries()
	d.index = min(d.index, max(0, len(entries)-1))
	for i, entry := range entries {
		if entry.kind+":"+entry.id == selected {
			d.index = i
			break
		}
	}
	return nil
}
func (m *model) sessionPickerEntries() []sessionPickerEntry {
	d := m.sessionPicker
	if d == nil {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(d.search.Value()))
	matches := func(values ...string) bool {
		return strings.Contains(strings.ToLower(strings.Join(values, " ")), query)
	}
	names := map[string]string{}
	paths := map[string]string{}
	allowed := map[string]bool{}
	for _, w := range d.workspaces {
		for _, id := range w.Sessions {
			names[id] = w.Title
			paths[id] = w.Path
			if w.ID == d.workspaceID {
				allowed[id] = true
			}
		}
	}
	entries := []sessionPickerEntry{}
	if d.group != "workspaces" {
		for _, s := range d.sessions {
			if s.ID == "" || d.workspaceID != "" && !allowed[s.ID] {
				continue
			}
			title := s.Projections.Values.Title
			if title == "" {
				title = s.ID
			}
			if matches(title, s.ID, names[s.ID], paths[s.ID]) {
				entries = append(entries, sessionPickerEntry{"session", s.ID, singleLine(title), singleLine(names[s.ID])})
			}
		}
	}
	if d.group != "sessions" {
		for _, w := range d.workspaces {
			if w.ID != "" && matches(w.Title, w.ID, w.Path) {
				title := w.Title
				if title == "" {
					title = w.Path
				}
				if title == "" {
					title = w.ID
				}
				entries = append(entries, sessionPickerEntry{"workspace", w.ID, singleLine(title), singleLine(w.Path)})
			}
		}
	}
	return entries
}
func (m *model) sessionPickerSpec() chooserSpec {
	d := m.sessionPicker
	detail := tr("Сессии и рабочие области")
	if d.busy {
		detail = tr("Загрузка…")
	} else if d.note != "" {
		detail = strings.TrimSpace(d.note)
	}
	if d.workspaceID != "" && d.note == "" {
		for _, w := range d.workspaces {
			if w.ID == d.workspaceID {
				detail = singleLine(w.Title + " · " + w.Path)
			}
		}
	}
	d.search.Placeholder = tr("Найти сессию или рабочую область…")
	spec := chooserSpec{Title: tr("Сессии и рабочие области"), Detail: detail, Footer: tr("↑↓ выбор · Enter открыть · Tab группа · F5 обновить") + "\n" + tr("Ctrl+N новая сессия · F2 сессии · F3 области · Esc"), Width: 76, Height: 24}
	d.search.Width = m.chooserLayout(spec).InnerWidth
	spec.Search = d.search.View()
	return spec
}
func (m *model) sessionPickerRows() ([]sessionPickerEntry, []sessionPickerRow, int) {
	entries := m.sessionPickerEntries()
	rows := []sessionPickerRow{}
	group := ""
	selected := 0
	for i, e := range entries {
		if e.kind != group {
			group = e.kind
			label := "Сессии"
			if group == "workspace" {
				label = "Рабочие области"
			}
			rows = append(rows, sessionPickerRow{entry: -1, group: label})
		}
		if i == m.sessionPicker.index {
			selected = len(rows)
		}
		rows = append(rows, sessionPickerRow{entry: i})
	}
	return entries, rows, selected
}
func (m *model) sessionPickerView(base string) string {
	d := m.sessionPicker
	if d == nil || !m.terminalFits() {
		return base
	}
	spec := m.sessionPickerSpec()
	g := m.chooserLayout(spec)
	entries, rows, selected := m.sessionPickerRows()
	start, end := chooserWindow(selected, len(rows), g.List.height)
	rendered := []string{}
	for _, row := range rows[start:end] {
		if row.entry < 0 {
			rendered = append(rendered, chooserGroup(tr(row.group)))
			continue
		}
		e := entries[row.entry]
		mark := "  "
		if e.kind == "session" && m.session != nil && e.id == m.session.ID || e.kind == "workspace" && e.id == m.side.activeWorkspace {
			mark = "● "
		}
		title := mark + ansi.Truncate(e.title, max(1, g.InnerWidth*2/3-2), "…")
		hint := ansi.Truncate(e.detail, max(1, g.InnerWidth-ansi.StringWidth(title)-2), "…")
		rendered = append(rendered, chooserItem(title, hint, row.entry == d.index, true, g.InnerWidth))
	}
	if len(rows) == 0 {
		rendered = append(rendered, muted().Render(tr("Ничего не найдено.")))
	}
	return m.chooserView(base, spec, rendered)
}

// Bind the explicit choice into the shared workspace cache so subsequent global
// New Session uses this ID even if the background tree has not refreshed yet.
// Retain newer unrelated entries and never carry a cache across logins.
func (m *model) bindSessionPickerWorkspace(d *sessionPicker, id string) {
	if d.login != m.settingsLogin {
		return
	}
	for _, workspace := range d.workspaces {
		if workspace.ID != id {
			continue
		}
		if m.side.login != d.login {
			m.side = sidebarState{login: d.login, sequence: m.side.sequence + 1}
		}
		present := false
		for _, known := range m.side.workspaces {
			if known.ID == id {
				present = true
				break
			}
		}
		if !present {
			workspace.Sessions = append([]string(nil), workspace.Sessions...)
			m.side.workspaces = append(m.side.workspaces, workspace)
		}
		m.side.activeWorkspace = id
		return
	}
}

func (m *model) sessionPickerChoose(create bool) tea.Cmd {
	d := m.sessionPicker
	if d == nil || d.login != m.settingsLogin {
		return nil
	}
	entries := m.sessionPickerEntries()
	if d.index < 0 || d.index >= len(entries) {
		return nil
	}
	e := entries[d.index]
	if create {
		if e.kind != "workspace" {
			d.note = tr("Выберите рабочую область для новой сессии.")
			return nil
		}
		m.bindSessionPickerWorkspace(d, e.id)
		m.closeSessionPicker()
		m.showChat()
		return m.sessionOperation("new", e.id)
	}
	if e.kind == "workspace" {
		m.bindSessionPickerWorkspace(d, e.id)
		d.workspaceID = e.id
		d.group = "sessions"
		d.search.SetValue("")
		d.index = 0
		return nil
	}
	for _, w := range d.workspaces {
		for _, id := range w.Sessions {
			if id == e.id {
				m.bindSessionPickerWorkspace(d, w.ID)
			}
		}
	}
	m.closeSessionPicker()
	focus := m.showChat()
	if m.session != nil && m.session.ID == e.id {
		return focus
	}
	return tea.Batch(focus, m.sessionOperation("connect", e.id))
}
func (m *model) sessionPickerKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	d := m.sessionPicker
	if d == nil {
		return nil, false
	}
	if !msg.Paste && !(msg.Type == tea.KeyRunes && len(msg.Runes) > 1) {
		switch msg.String() {
		case "ctrl+c", "ctrl+d":
			return tea.Quit, true
		case "f8":
			return m.cancelChat(), true
		case "esc":
			return m.closeSessionPicker(), true
		case "f5":
			return m.loadSessionPicker(), true
		case "f2":
			m.closeSessionPicker()
			m.page = sessionsPage
			m.input.Blur()
			m.resize()
			return m.sessionOperation("list", ""), true
		case "f3":
			m.closeSessionPicker()
			return m.openWorkbench("workspaces"), true
		case "ctrl+n":
			return m.sessionPickerChoose(true), true
		case "enter":
			return m.sessionPickerChoose(false), true
		case "tab", "shift+tab":
			groups := []string{"", "sessions", "workspaces"}
			index := 0
			for i, group := range groups {
				if group == d.group {
					index = i
				}
			}
			delta := 1
			if msg.String() == "shift+tab" {
				delta = -1
			}
			d.group = groups[(index+delta+len(groups))%len(groups)]
			d.workspaceID = ""
			d.index = 0
			return nil, true
		case "up", "down", "pgup", "pgdown", "home", "end":
			count := len(m.sessionPickerEntries())
			delta := 1
			if msg.String() == "up" {
				delta = -1
			}
			if msg.String() == "pgup" {
				delta = -5
			}
			if msg.String() == "pgdown" {
				delta = 5
			}
			d.index = max(0, min(max(0, count-1), d.index+delta))
			if msg.String() == "home" {
				d.index = 0
			}
			if msg.String() == "end" {
				d.index = max(0, count-1)
			}
			return nil, true
		}
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
		msg.Paste = true
	}
	var cmd tea.Cmd
	d.search, cmd = d.search.Update(msg)
	d.index = 0
	return cmd, true
}
func (m *model) sessionPickerMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	d := m.sessionPicker
	if d == nil {
		return nil, false
	}
	if msg.Action == tea.MouseActionRelease {
		return nil, true
	}
	spec := m.sessionPickerSpec()
	g := m.chooserLayout(spec)
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		delta := 1
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		d.index = max(0, min(max(0, len(m.sessionPickerEntries())-1), d.index+delta))
		return nil, true
	}
	if msg.Button != tea.MouseButtonLeft {
		return nil, true
	}
	if g.Close.contains(msg.X, msg.Y) {
		return m.closeSessionPicker(), true
	}
	if g.List.contains(msg.X, msg.Y) {
		_, rows, selected := m.sessionPickerRows()
		start, end := chooserWindow(selected, len(rows), g.List.height)
		i := start + msg.Y - g.List.y
		if i < end && rows[i].entry >= 0 {
			d.index = rows[i].entry
			return m.sessionPickerChoose(false), true
		}
	}
	return nil, true
}
