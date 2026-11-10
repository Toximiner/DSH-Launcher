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
	for _, page := range navigationPages {
		text := "   " + pages[page]
		if page == m.page {
			text = lipgloss.NewStyle().Width(inner).Bold(true).Foreground(lipgloss.Color("#79c0ff")).Background(lipgloss.Color("#233044")).Render(" › " + pages[page])
		}
		lines = append(lines, text)
	}
	chatLink := " ○ Локальный черновик"
	if m.session != nil {
		chatLink = " ● Текущая сессия"
	}
	lines = append(lines, "", muted.Bold(true).Render("СЕССИИ"), chatLink)
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
	if m.session != nil {
		connection = "✓ dsh · " + m.modeLabel()
	}
	lines = append(lines, muted.Render(connection), muted.Render("v"+version))
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
	if m.session != nil {
		status = accent.Render("✓ dsh · " + m.modeLabel())
	}
	if m.sessionBusy {
		status = accent.Render("◌ Запрос сессии dsh")
	}
	if m.chat != nil && !m.chat.ready {
		status = accent.Render("◌ Подключение чата")
		if m.chat.streamError != "" {
			status = muted.Render("! Чат: нет связи")
		}
	}
	header := bar(brand, status, width) + "\n" + m.menuBar(width)
	title := pages[m.page]
	if m.page == 0 {
		title = "Новый разговор"
		if m.session != nil {
			title = "Сессия · " + ansi.Truncate(clean(m.session.ID), max(1, w-15), "…")
		}
	}
	view := heading.Render(title) + "\n" + muted.Render(strings.Repeat("─", m.viewport.Width)) + "\n" + m.viewport.View()
	right := panel(w, bodyHeight, view, "#303d4d")
	if m.page == 0 {
		chat := panel(w, bodyHeight-composerHeight-1, view, "#303d4d")
		border := "#303d4d"
		if !m.navFocused {
			border = "#58a6ff"
		}
		label, hint := m.composerText()
		composer := heading.Render("Сообщение") + muted.Render(label) + "\n" + m.modeControls() + "\n" + m.input.View() + "\n" + muted.Render(hint)
		right = chat + "\n" + panel(w, composerHeight, composer, border)
	}
	body := right
	if width >= 78 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.sidebar(bodyHeight), " ", right)
	}
	hints := "F10 меню · Tab разделы · Esc чат"
	if m.page == dshSettingsPage && m.settingsLogin != "" {
		hints = "←/→ раздел · r обновить · PgUp/PgDn"
	}
	if m.page == chatPage && len(m.toolIDs()) > 0 {
		hints = "F6 инструменты · Enter раскрыть · Esc ввод"
	}
	footer := bar(muted.Render(hints), muted.Render("Ctrl+D выход"), width)
	return m.chromeView(header + "\n" + body + "\n" + footer)
}

func (m *model) modeControls() string {
	names := []string{"Обычный", "План", "Цель"}
	selected := -1
	if m.session != nil && !m.sessionStale {
		selected = 0
		if m.session.Plan != nil && m.session.Plan.Active {
			selected = 1
		} else if m.session.Goal != nil && m.session.Goal.Goal.Phase == "active" {
			selected = 2
		}
	}
	text := "Режим: "
	for i, name := range names {
		button := "[" + name + "]"
		if i == selected {
			button = accent.Bold(true).Render(button)
		} else {
			button = muted.Render(button)
		}
		if i > 0 {
			text += " "
		}
		text += button
	}
	return text
}
