package main

import (
	"github.com/charmbracelet/lipgloss"
	"sync/atomic"
)

var activeTheme atomic.Bool
var lightPalette = map[string]string{
	"#e6edf3": "#17212b", "#8b949e": "#526171", "#111820": "#f5f7fa",
	"#101318": "#ffffff", "#79c0ff": "#005fb8", "#58a6ff": "#005fb8",
	"#233044": "#d9eaff", "#303d4d": "#8294a8", "#d29922": "#805300",
}

func uiColor(value string) lipgloss.Color {
	if activeTheme.Load() {
		if next, ok := lightPalette[value]; ok {
			value = next
		}
	}
	return lipgloss.Color(value)
}
func accent() lipgloss.Style          { return lipgloss.NewStyle().Foreground(uiColor("#58a6ff")) }
func heading() lipgloss.Style         { return lipgloss.NewStyle().Bold(true).Foreground(uiColor("#e6edf3")) }
func muted() lipgloss.Style           { return lipgloss.NewStyle().Foreground(uiColor("#8b949e")) }
func (m *model) sidebarVisible() bool { return !m.prefs.HideSidebar && m.width-1 >= 78 }

func (m *model) applyClientAppearance() {
	activeTheme.Store(m.prefs.Theme == "light")
	activeEnglish.Store(m.prefs.Language == "en")
	m.input.Placeholder = tr("Сообщение или /help…")
	m.input.PlaceholderStyle = lipgloss.NewStyle().Foreground(uiColor("#8b949e"))
	m.input.PromptStyle = lipgloss.NewStyle().Foreground(uiColor("#58a6ff"))
	m.marketSearch.Prompt = tr("Поиск: ")
}
