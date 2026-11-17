package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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

type paletteItem = actionDescriptor

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

// Palette rows are a projection of the shared action registry.
var paletteCatalog = paletteActions()

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
		if query != "" && !strings.Contains(strings.ToLower(clean(tr(item.label)+" "+actionPaletteHint(*item)+" "+tr(item.group)+" "+strings.Join(item.aliases, " ")+" "+strings.Join(item.keys, " "))), query) {
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
		m.chrome.dialog == "" && !m.chrome.menuOpen && m.marketConfirm == nil && m.sessionPicker == nil &&
		(m.workbench == nil || (m.workbench.pageView && !m.workbench.editing && !m.workbench.confirm && !m.workbench.busy && !m.workbench.writing && m.workbench.pluginPanel == nil))
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
		p.note = actionDisabledReason(m, item)
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

func (m *model) paletteSpec() chooserSpec {
	detail := ""
	search := ""
	footer := tr("↑↓ выбрать · Enter открыть · Tab группа")
	if m.palette != nil {
		detail = fmt.Sprintf(tr("Найдено: %d"), len(paletteItemRows(m.paletteFiltered())))
		if m.palette.groupFilter != "" {
			detail += " · " + tr(m.palette.groupFilter)
		}
		search = m.palette.search.View()
		if m.palette.note != "" {
			footer = m.palette.note
		}
	}
	return chooserSpec{Title: tr("Команды"), Search: search, Detail: detail, Footer: footer}
}
func (m *model) paletteBox() area { return m.chooserLayout(chooserSpec{}).Box }

// The same row window is used for rendering and mouse hit testing.
func (m *model) paletteWindow() (int, int) {
	count := m.chooserLayout(chooserSpec{}).List.height
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
	if m.chooserLayout(chooserSpec{}).Close.contains(msg.X, msg.Y) {
		m.paletteClose()
		return nil, true
	}
	first, count := m.paletteWindow()
	line := msg.Y - m.chooserLayout(chooserSpec{}).List.y
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
	inner := m.chooserLayout(chooserSpec{}).InnerWidth
	p.search.Width = inner
	p.search.Placeholder = tr("Найти команду…")
	rows := m.paletteFiltered()
	lines := []string{}
	first, count := m.paletteWindow()
	for i := first; i < min(len(rows), first+count); i++ {
		row := rows[i]
		if row.kind == "heading" {
			lines = append(lines, chooserGroup(tr(row.group)))
			continue
		}
		item := paletteCatalog[row.index]
		lines = append(lines, chooserItem(tr(item.label), actionPaletteHint(item), i == p.offset, actionEnabled(m, item), inner))
	}
	if len(rows) == 0 {
		lines = append(lines, muted().Render(tr("Совпадений нет.")))
	}
	return m.chooserView(base, m.paletteSpec(), lines)
}
