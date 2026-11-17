package main

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

func accent() lipgloss.Style          { return lipgloss.NewStyle().Foreground(theme.Color(theme.Accent)) }
func heading() lipgloss.Style         { return lipgloss.NewStyle().Bold(true).Foreground(theme.Color(theme.Text)) }
func muted() lipgloss.Style           { return lipgloss.NewStyle().Foreground(theme.Color(theme.Muted)) }
func (m *model) sidebarVisible() bool { return !m.prefs.HideSidebar && m.width-1 >= 78 }

func (m *model) applyClientAppearance() {
	theme.SetLight(m.prefs.Theme == "light")
	activeEnglish.Store(m.prefs.Language == "en")
	m.input.Placeholder = tr("Сообщение или /help…")
	m.input.PlaceholderStyle = lipgloss.NewStyle().Foreground(theme.Color(theme.Muted))
	m.input.PromptStyle = lipgloss.NewStyle().Foreground(theme.Color(theme.Accent))
	m.marketSearch.Prompt = tr("Поиск: ")
}
