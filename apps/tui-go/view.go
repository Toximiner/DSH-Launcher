package main

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	accent  = lipgloss.NewStyle().Foreground(lipgloss.Color("#58a6ff"))
	heading = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e6edf3"))
	muted   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b949e"))
)

func panel(width, height int, content, color string) string {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(color)).
		Foreground(lipgloss.Color("#e6edf3")).Background(lipgloss.Color("#111820")).Padding(0, 1)
	return style.Width(max(1, width-style.GetHorizontalBorderSize())).
		Height(max(1, height-style.GetVerticalFrameSize())).Render(content)
}

func bar(left, right string, width int) string {
	left = ansi.Truncate(left, max(1, width-lipgloss.Width(right)-1), "…")
	return left + strings.Repeat(" ", max(1, width-lipgloss.Width(left)-lipgloss.Width(right))) + right
}

func (m *model) sidebar(height int) string {
	const width = sidebarWidth
	const inner = 20
	focus := "РАЗДЕЛЫ"
	if m.navFocused {
		focus += " · ФОКУС"
	}
	lines := []string{muted.Bold(true).Render("РАБОЧАЯ ОБЛАСТЬ"), heading.Render(ansi.Truncate(filepath.Base(m.workspace), inner, "…")), "", muted.Bold(true).Render(focus)}
	for i, page := range pages {
		text := "   " + page
		if i == m.page {
			text = lipgloss.NewStyle().Width(inner).Bold(true).Foreground(lipgloss.Color("#79c0ff")).Background(lipgloss.Color("#233044")).Render(" › " + page)
		}
		lines = append(lines, text)
	}
	lines = append(lines, "", muted.Bold(true).Render("СЕССИИ"), " ○ Локальный черновик")
	target := height - 2
	if len(lines) > target-2 {
		lines = lines[:max(0, target-2)]
	}
	for len(lines) < target-2 {
		lines = append(lines, "")
	}
	connection := "○ dsh не подключён"
	if m.settings != nil {
		connection = "✓ Снимок настроек dsh"
	}
	lines = append(lines, muted.Render(connection), muted.Render("Go · "+version))
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, inner, "…")
	}
	border := "#303d4d"
	if m.navFocused {
		border = "#58a6ff"
	}
	return panel(width, height, strings.Join(lines, "\n"), border)
}

func (m *model) View() string {
	if m.width < 44 || m.height < 18 {
		return "Увеличьте терминал (от 44×18). Ctrl+D — выход."
	}
	width := m.width - 1
	w, bodyHeight := m.contentSize()
	brand := accent.Bold(true).Render("◆ DSH Launcher")
	status := lipgloss.NewStyle().Foreground(lipgloss.Color("#d29922")).Render("○ Нет подключения")
	if m.settings != nil {
		status = accent.Render("✓ Снимок настроек dsh")
	}
	if m.settingsLoading {
		status = accent.Render("◌ Чтение настроек dsh")
	}
	if m.settingsError != "" {
		status = muted.Render("! Ошибка чтения dsh")
	}
	header := bar(brand, status, width) + "\n" + ansi.Truncate(muted.Render("  Проект: "+filepath.Base(m.workspace)+"  /  "+pages[m.page]), width, "…")
	title := pages[m.page]
	if m.page == 0 {
		title = "Новый разговор"
	}
	view := heading.Render(title) + "\n" + muted.Render(strings.Repeat("─", m.viewport.Width)) + "\n" + m.viewport.View()
	right := panel(w, bodyHeight, view, "#303d4d")
	if m.page == 0 {
		chat := panel(w, bodyHeight-6, view, "#303d4d")
		border := "#303d4d"
		if !m.navFocused {
			border = "#58a6ff"
		}
		composer := heading.Render("Сообщение") + muted.Render(" · локальный черновик") + "\n" + m.input.View() + "\n" + muted.Render("Enter — сохранить · ↑/↓ — история")
		right = chat + "\n" + panel(w, 5, composer, border)
	}
	body := right
	if width >= 78 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.sidebar(bodyHeight), " ", right)
	}
	hints := "Tab разделы · Esc чат · PgUp/PgDn"
	if m.page == 2 && m.settingsLogin != "" {
		hints = "←/→ раздел · r обновить · PgUp/PgDn"
	}
	footer := bar(muted.Render(hints), muted.Render("Ctrl+D выход"), width)
	return header + "\n" + body + "\n" + footer
}
