package main

import (
	"strings"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type textSelectionStart struct{}

// The terminal owns native selection and clipboard gestures. Keep the visible
// frame stable while backend messages continue updating the model underneath.
// This mode is transient and does not change the saved mouse preference.
func (m *model) beginTextSelection() tea.Cmd {
	if m.selectionSnapshot != "" || !m.terminalFits() {
		return nil
	}
	lines := strings.Split(m.View(), "\n")
	if len(lines) > 0 {
		lines[len(lines)-1] = muted().Render(ansi.Truncate(tr("Выделение: мышь · Ctrl+Shift+C копия · Esc/F10 назад"), max(1, m.width-1), "…"))
	}
	m.selectionSnapshot = surface(strings.Join(lines, "\n"), theme.Bg)
	return tea.DisableMouse
}

func (m *model) endTextSelection() tea.Cmd {
	m.selectionSnapshot = ""
	if m.mouseEnabled() {
		return tea.EnableMouseCellMotion
	}
	return tea.DisableMouse
}

func (m *model) textSelectionKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.selectionSnapshot == "" {
		if msg.Type == tea.KeyF10 && !msg.Paste && m.terminalFits() {
			return m.beginTextSelection(), true
		}
		return nil, false
	}
	if msg.Paste {
		return nil, true
	}
	switch msg.Type {
	case tea.KeyEsc, tea.KeyF10:
		cmd := m.endTextSelection()
		m.rebuild()
		return cmd, true
	case tea.KeyF7:
		cmd := m.endTextSelection()
		if m.approvals != nil && len(m.approvals.requests) > 0 {
			if m.sessionPicker != nil {
				m.closeSessionPicker()
			}
			if m.palette != nil && m.palette.open {
				m.paletteClose()
			}
			m.approvals.visible = true
		}
		m.rebuild()
		return cmd, true
	case tea.KeyF8:
		return tea.Batch(m.endTextSelection(), m.cancelChat()), true
	case tea.KeyCtrlC, tea.KeyCtrlD:
		m.endTextSelection()
		return tea.Quit, true
	}
	// Selection gestures and paste must never reach a hidden decision/editor.
	return nil, true
}
