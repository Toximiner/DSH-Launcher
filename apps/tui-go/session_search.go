package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type sessionHit struct{ index, row int }

func (m *model) sessionSearchKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.page != sessionsPage {
		m.sessionSearching = false
		return nil, false
	}
	if m.navFocused || m.archiveDialog != nil || m.forkDialog != nil || m.renameDialog != nil || m.historyReader != nil || m.chrome.menuOpen || m.chrome.dialog != "" {
		return nil, false
	}
	if !msg.Paste && (msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyCtrlD || msg.Type == tea.KeyF7 || msg.Type == tea.KeyF8 || msg.Alt) {
		return nil, false
	}
	if !m.sessionSearching {
		if !msg.Paste && (msg.Type == tea.KeyCtrlF || (msg.Type == tea.KeyRunes && string(msg.Runes) == "/")) {
			m.sessionSearch = textinput.New()
			m.sessionSearch.CharLimit = 128
			m.sessionSearch.SetValue(m.sessionQuery)
			m.sessionSearching = true
			m.rebuild()
			return m.sessionSearch.Focus(), true
		}
		return nil, false
	}
	if !msg.Paste && (msg.Type == tea.KeyEsc || msg.Type == tea.KeyEnter || msg.Type == tea.KeyUp || msg.Type == tea.KeyDown) {
		m.sessionSearching = false
		m.sessionSearch.Blur()
		m.rebuild()
		return nil, msg.Type != tea.KeyUp && msg.Type != tea.KeyDown
	}
	if !msg.Paste && msg.Type == tea.KeyCtrlU {
		m.sessionSearch.SetValue("")
	}
	var cmd tea.Cmd
	m.sessionSearch, cmd = m.sessionSearch.Update(msg)
	m.sessionQuery = strings.TrimSpace(m.sessionSearch.Value())
	m.sessionListIndex = 0
	m.rebuild()
	m.revealSelectedSession()
	return cmd, true
}
