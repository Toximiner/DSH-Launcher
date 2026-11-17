package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

const (
	conversationContextMinWidth = 120
	conversationContextWidth    = 30
	conversationContextGap      = 2
)

type conversationLayout struct {
	chat        area
	context     area
	showContext bool
}

// The chat uses every available cell. A contextual right column is optional
// rather than a fixed centering margin; home always keeps its compact input.
func (m *model) conversationLayout() conversationLayout {
	width, height := m.contentSize()
	layout := conversationLayout{chat: area{0, bodyTop, width, height}}
	if m.page != chatPage || m.landingVisible() || m.width < conversationContextMinWidth || m.contextPanelHidden {
		return layout
	}
	layout.showContext = true
	layout.chat.width = width - conversationContextWidth - conversationContextGap
	layout.context = area{layout.chat.width + conversationContextGap, bodyTop, conversationContextWidth, height}
	return layout
}

func (m *model) conversationContextView(box area) string {
	inner := max(1, box.width-4)
	base := lipgloss.NewStyle().Background(theme.Color(theme.Panel)).Foreground(theme.Color(theme.Text))
	label := base.Bold(true)
	soft := base.Foreground(theme.Color(theme.Muted))
	lines := make([]string, box.height)
	title := tr("Локальный черновик")
	if m.session != nil {
		title = m.session.Title
		if title == "" {
			title = m.session.ID
		}
	}
	if box.height > 1 {
		lines[1] = label.Render(ansi.Truncate(singleLine(title), inner, "…"))
	}
	if m.session != nil && !m.sessionStale && m.session.Pressure != nil {
		pressure := m.session.Pressure
		used := pressure.Projected
		if used == nil {
			used = pressure.Pressure
		}
		if used != nil && validStat(*used) && box.height > 6 {
			lines[4] = label.Render(tr("Контекст"))
			lines[5] = soft.Render(countLabel(*used) + tr(" ток"))
			if pressure.Capacity != nil && *pressure.Capacity > 0 && validStat(*pressure.Capacity) {
				lines[6] = soft.Render(fmt.Sprintf("%.0f%%", 100*float64(*used)/float64(*pressure.Capacity)))
			}
		}
	}
	if m.workspace != "" && box.height > 2 {
		lines[box.height-3] = soft.Render(tr("Рабочая папка:"))
		path := singleLine(m.workspace)
		if lipgloss.Width(path) > inner {
			path = ansi.TruncateLeft(path, lipgloss.Width(path)-inner+1, "…")
		}
		lines[box.height-2] = soft.Render(path)
	}
	for i, line := range lines {
		lines[i] = base.Width(box.width).Render("  " + line)
	}
	return surface(strings.Join(lines, "\n"), theme.Panel)
}

// Called once by the outer view so the context column never changes the
// viewport/composer origin or introduces additional horizontal centering.
func (m *model) conversationLayoutView(chat string) string {
	layout := m.conversationLayout()
	body := lipgloss.NewStyle().Width(layout.chat.width).Height(layout.chat.height).Render(chat)
	if !layout.showContext {
		return body
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, body, strings.Repeat(" ", conversationContextGap), m.conversationContextView(layout.context))
}

// Streaming and local messages can leave home without a window-size event.
// Reflow their content before rendering against the newly visible right panel.
func (m *model) syncConversationGeometry() {
	if m.page != chatPage {
		return
	}
	box := m.conversationLayout().chat
	m.viewport.Width = max(1, box.width-4)
	m.viewport.Height = max(1, box.height-conversationComposerHeight)
	m.input.Width = max(1, box.width-6)
}
