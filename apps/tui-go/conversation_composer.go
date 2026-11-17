package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

const conversationComposerHeight = 4
const landingComposerHeight = 5

// compactComposerControls measures the exact metadata cells used by both the
// home screen and the conversation composer. It exposes no hidden buttons.
func (m *model) compactComposerControls(box area) (string, []agentHit) {
	inner := max(1, box.width-4)
	panelStyle := lipgloss.NewStyle().Background(theme.Color(theme.Panel))
	fg := panelStyle.Foreground(theme.Color(theme.Accent))
	soft := panelStyle.Foreground(theme.Color(theme.Muted))
	hits := []agentHit{}
	permission := ansi.Truncate(singleLine(m.permissionButton()), max(1, inner/3), "…")
	available := inner
	if permission != "" {
		available -= lipgloss.Width(permission) + 2
	}
	preset := ansi.Truncate(m.agentPresetLabel(), max(1, available/3), "…")
	modelLabel := strings.TrimSuffix(m.modelButton(), " · "+tr("По умолчанию"))
	modelLabel = ansi.Truncate(singleLine(modelLabel), max(1, available-lipgloss.Width(preset)-3), "…")
	left := fg.Render(preset + " · " + modelLabel)
	metadata := bar(left, soft.Render(permission), inner)
	y := box.y + 3
	hits = append(hits, agentHit{area{box.x + 2, y, lipgloss.Width(preset), 1}, "presets"})
	hits = append(hits, agentHit{area{box.x + 2 + lipgloss.Width(preset) + 3, y, lipgloss.Width(modelLabel), 1}, "models"})
	if permission != "" {
		hits = append(hits, agentHit{area{box.x + box.width - 2 - lipgloss.Width(permission), y, lipgloss.Width(permission), 1}, "permissions"})
	}
	return metadata, hits
}

// compactComposerSurface keeps every row and nested ANSI span on one surface.
// Render a copy so opening a picker never changes the actual draft or cursor.
func (m *model) compactComposerSurface(box area, placeholder, metadata string) string {
	input := m.input
	input.Placeholder = placeholder
	input.Width = max(1, box.width-6)
	// Width changes alone do not recompute Bubbles' private visible window. Walk
	// the copied cursor through the end to remeasure it, then restore its position.
	cursor := input.Position()
	input.CursorEnd()
	input.SetCursor(cursor)
	panelStyle := lipgloss.NewStyle().Background(theme.Color(theme.Panel)).Foreground(theme.Color(theme.Text))
	input.TextStyle = input.TextStyle.Background(theme.Color(theme.Panel))
	input.PlaceholderStyle = input.PlaceholderStyle.Background(theme.Color(theme.Panel))
	input.Cursor.Style = input.Cursor.Style.Background(theme.Color(theme.Panel))
	input.PromptStyle = input.PromptStyle.Background(theme.Color(theme.Panel))
	edgeStyle := panelStyle.Foreground(theme.Color(theme.Accent))
	if m.navFocused || m.toolFocused {
		edgeStyle = panelStyle.Foreground(theme.Color(theme.Border))
	}
	edge := edgeStyle.Render("┃")
	row := func(content string) string { return edge + panelStyle.Width(box.width-1).Render(" "+content) }
	rows := []string{row(input.View()), row(""), row(""), row(metadata)}
	if box.height == landingComposerHeight {
		rows = []string{row(""), row(input.View()), row(""), row(metadata), row("")}
	}
	return surface(strings.Join(rows, "\n"), theme.Panel)
}

func (m *model) conversationComposerView() string {
	box := m.composerBox()
	metadata, _ := m.compactComposerControls(box)
	return m.compactComposerSurface(box, tr("Сообщение или /help…"), metadata)
}
