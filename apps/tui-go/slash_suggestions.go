package main

import (
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

type slashSuggestion struct {
	alias, label string
	action       *actionDescriptor
	arguments    bool
}
type slashSuggestionState struct {
	query, dismissed string
	index            int
}

// Suggestions only inspect the shared command catalog; rendering never calls dsh.
func (m *model) slashSuggestions() []slashSuggestion {
	value := m.input.Value()
	if m.slash.query != value {
		m.slash.query = value
		m.slash.dismissed = ""
		m.slash.index = 0
	}
	if m.page != chatPage || m.navFocused || m.toolFocused || !m.input.Focused() || m.tabHasModal() || !m.terminalFits() {
		return nil
	}
	if !strings.HasPrefix(value, "/") || strings.IndexFunc(value, unicode.IsSpace) >= 0 || m.input.Position() != len([]rune(value)) {
		return nil
	}
	if m.slash.dismissed == value {
		return nil
	}
	rows := []slashSuggestion{}
	seen := map[string]bool{}
	for i := range actionRegistry {
		action := &actionRegistry[i]
		for _, alias := range action.aliases {
			if strings.HasPrefix(alias, value) && !seen[alias] {
				rows = append(rows, slashSuggestion{alias: alias, label: tr(action.label), action: action})
				seen[alias] = true
			}
		}
	}
	for _, extra := range []slashSuggestion{
		{alias: "/session", label: tr("Открыть сессию по ID"), arguments: true},
		{alias: "/mode", label: tr("Режим: default, plan или goal."), arguments: true},
		{alias: "/attach", label: tr("Прикрепить файл"), arguments: true},
	} {
		if strings.HasPrefix(extra.alias, value) {
			rows = append(rows, extra)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].alias == value && rows[j].alias != value })
	m.slash.index = max(0, min(m.slash.index, max(0, len(rows)-1)))
	return rows
}

func (m *model) slashSuggestionsKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if msg.Paste {
		return nil, false
	}
	rows := m.slashSuggestions()
	if len(rows) == 0 {
		return nil, false
	}
	state := &m.slash
	switch msg.Type {
	case tea.KeyUp, tea.KeyDown:
		delta := 1
		if msg.Type == tea.KeyUp {
			delta = -1
		}
		state.index = (state.index + delta + len(rows)) % len(rows)
		return nil, true
	case tea.KeyEsc:
		state.dismissed = m.input.Value()
		return nil, true
	case tea.KeyTab, tea.KeyEnter:
		row := rows[state.index]
		value := row.alias
		if msg.Type == tea.KeyTab || row.arguments {
			value += " "
		}
		m.input.SetValue(value)
		m.input.CursorEnd()
		state.query = value
		state.dismissed = value
		// Enter continues through the existing parser and all its operation guards.
		return nil, msg.Type == tea.KeyTab || row.arguments
	}
	return nil, false
}

func (m *model) slashSuggestionsLayout() (area, []slashSuggestion, int, int) {
	rows := m.slashSuggestions()
	if len(rows) == 0 {
		return area{}, nil, 0, 0
	}
	anchor := m.composerBox()
	count := min(8, len(rows), max(0, anchor.y-bodyTop-2))
	if count < 1 {
		return area{}, nil, 0, 0
	}
	start, end := chooserWindow(m.slash.index, len(rows), count)
	return area{anchor.x, anchor.y - count - 2, anchor.width, count + 2}, rows, start, end
}
func (m *model) slashSuggestionsView(base string) string {
	box, rows, start, end := m.slashSuggestionsLayout()
	if len(rows) == 0 || box.height == 0 {
		return base
	}
	inner := max(1, box.width-4)
	lines := []string{bar(heading().Render(tr("Команды")), muted().Render("esc"), inner)}
	for i := start; i < end; i++ {
		row := rows[i]
		lines = append(lines, chooserItem(row.alias, row.label, i == m.slash.index, true, inner))
	}
	hint := tr("↑/↓ выбор · Enter выполнить · Tab дополнить")
	selected := rows[m.slash.index]
	if selected.action != nil && !actionEnabled(m, *selected.action) {
		hint = actionDisabledReason(m, *selected.action)
	}
	lines = append(lines, muted().Render(ansi.Truncate(hint, inner, "…")))
	front := lipgloss.NewStyle().Width(box.width).Height(box.height).Padding(0, 2).
		Foreground(theme.Color(theme.Text)).Background(theme.Color(theme.Panel)).Render(strings.Join(lines, "\n"))
	return overlay(base, surface(front, theme.Panel), box.x, box.y, m.width-1)
}
func (m *model) slashSuggestionsMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	box, rows, start, end := m.slashSuggestionsLayout()
	if len(rows) == 0 || box.height == 0 || !box.contains(msg.X, msg.Y) {
		return nil, false
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		key := tea.KeyDown
		if msg.Button == tea.MouseButtonWheelUp {
			key = tea.KeyUp
		}
		return m.slashSuggestionsKey(tea.KeyMsg{Type: key})
	case tea.MouseButtonLeft:
		index := start + msg.Y - box.y - 1
		if index >= start && index < end {
			m.slash.index = index
			// A click completes the command; explicit Enter is still required to run it.
			return m.slashSuggestionsKey(tea.KeyMsg{Type: tea.KeyTab})
		}
	}
	return nil, true
}
