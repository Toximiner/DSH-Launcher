package main

import (
	"github.com/charmbracelet/lipgloss"
	"strings"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

func accent() lipgloss.Style { return lipgloss.NewStyle().Foreground(theme.Color(theme.Accent)) }
func heading() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(theme.Color(theme.Text))
}
func muted() lipgloss.Style { return lipgloss.NewStyle().Foreground(theme.Color(theme.Muted)) }

// sidebarVisible keeps the legacy geometry helpers; the permanent sidebar
// was removed in the opencode-style shell (sessions live in the top tab rail).
func (m *model) sidebarVisible() bool { return false }

func (m *model) applyClientAppearance() {
	theme.SetLight(m.prefs.Theme == "light")
	activeEnglish.Store(m.prefs.Language == "en")
	m.input.Placeholder = tr("Сообщение или /help…")
	m.input.PlaceholderStyle = lipgloss.NewStyle().Foreground(theme.Color(theme.Muted))
	m.input.PromptStyle = lipgloss.NewStyle().Foreground(theme.Color(theme.Accent))
	m.marketSearch.Prompt = tr("Поиск: ")
}

// Keep nested color spans on their containing surface when they reset SGR.
// Terminal-default backgrounds can otherwise leave dark seams in padded rows.
func surface(text string, color theme.Token) string {
	sample := lipgloss.NewStyle().Background(theme.Color(color)).Render(" ")
	split := strings.IndexByte(sample, ' ')
	if split <= 0 {
		return text
	}
	background := sample[:split]
	painted := strings.NewReplacer("\x1b[0m", "\x1b[0m"+background, "\x1b[m", "\x1b[m"+background, "\x1b[49m", background).Replace(text)
	lines := strings.Split(painted, "\n")
	for i := range lines {
		lines[i] = background + lines[i] + "\x1b[0m"
	}
	return strings.Join(lines, "\n")
}
