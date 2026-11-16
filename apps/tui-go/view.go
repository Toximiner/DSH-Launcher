package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const minTerminalWidth = 80
const minTerminalHeight = 24

func (m *model) terminalFits() bool {
	return m.width >= minTerminalWidth && m.height >= minTerminalHeight
}

func (m *model) terminalSizeNotice() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	lines := []string{fmt.Sprintf(tr("Увеличьте терминал: минимум %d×%d."), minTerminalWidth, minTerminalHeight), fmt.Sprintf(tr("Сейчас: %d×%d."), m.width, m.height), tr("Ctrl+D — выход · F8 — остановить агента.")}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], max(1, m.width-1), "…")
	}
	return strings.Join(lines, "\n")
}

func panel(width, height int, content, color string) string {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(uiColor(color)).
		Foreground(uiColor("#e6edf3")).Background(uiColor("#111820")).Padding(0, 1)
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
	focus := tr("РАЗДЕЛЫ")
	if m.navFocused {
		focus += tr(" · ФОКУС")
	}
	lines := []string{muted().Bold(true).Render(tr("РАБОЧАЯ ОБЛАСТЬ")), heading().Render(ansi.Truncate(filepath.Base(m.workspace), inner, "…")), "", muted().Bold(true).Render(focus)}
	for _, page := range navigationPages {
		text := "   " + tr(pages[page])
		if page == m.page {
			text = lipgloss.NewStyle().Width(inner).Bold(true).Foreground(uiColor("#79c0ff")).Background(uiColor("#233044")).Render(" › " + tr(pages[page]))
		}
		lines = append(lines, text)
	}
	chatLink := tr(" ○ Локальный черновик")
	if m.session != nil {
		chatLink = tr(" ● Текущая сессия")
	}
	lines = append(lines, "", muted().Bold(true).Render(tr("СЕССИИ")), chatLink)
	target := height - 2
	if len(lines) > target-2 {
		lines = lines[:max(0, target-2)]
	}
	for len(lines) < target-2 {
		lines = append(lines, "")
	}
	connection := tr("○ dsh не подключён")
	if m.settings != nil {
		connection = tr("✓ Снимок настроек dsh")
	}
	if m.session != nil {
		connection = "✓ dsh · " + m.modeLabel()
	}
	lines = append(lines, muted().Render(connection), muted().Render("v"+version))
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
	activeTheme.Store(m.prefs.Theme == "light")
	activeEnglish.Store(m.prefs.Language == "en")
	if !m.terminalFits() {
		return m.terminalSizeNotice()
	}
	width := m.width - 1
	w, bodyHeight := m.contentSize()
	brand := accent().Bold(true).Render("◆ DSH Launcher")
	status := lipgloss.NewStyle().Foreground(uiColor("#d29922")).Render(tr("○ Нет подключения"))
	if m.settings != nil {
		status = accent().Render(tr("✓ Снимок настроек dsh"))
	}
	if m.settingsLoading {
		status = accent().Render(tr("◌ Чтение настроек dsh"))
	}
	if m.settingsError != "" {
		status = muted().Render(tr("! Ошибка чтения dsh"))
	}
	if m.session != nil {
		status = accent().Render("✓ dsh · " + m.modeLabel())
	}
	if m.sessionRequestVisible() {
		status = accent().Render(tr("◌ Запрос сессии dsh"))
	}
	if m.chat != nil && !m.chat.ready {
		status = accent().Render(tr("◌ Подключение чата"))
		if m.chat.streamError != "" {
			status = muted().Render(tr("! Чат: нет связи"))
		}
	}
	header := bar(brand, status, width) + "\n" + m.menuBar(width)
	title := tr(pages[m.page])
	if m.page == 0 {
		title = tr("Новый разговор")
		if m.session != nil {
			name := m.session.Title
			if name == "" {
				name = m.session.ID
			}
			title = tr("Сессия · ") + ansi.Truncate(clean(name), max(1, w-15), "…")
		}
	}
	view := heading().Render(title) + "\n" + muted().Render(strings.Repeat("─", m.viewport.Width)) + "\n" + m.viewport.View()
	right := panel(w, bodyHeight, view, "#303d4d")
	if m.page == 0 {
		chat := panel(w, bodyHeight-composerHeight-1, view, "#303d4d")
		border := "#303d4d"
		if !m.navFocused {
			border = "#58a6ff"
		}
		label, hint := m.composerText()
		composer := heading().Render(tr("Сообщение")) + muted().Render(label) + "\n" + m.modeControls() + "\n" + m.input.View() + "\n" + muted().Render(hint)
		right = chat + "\n" + panel(w, composerHeight, composer, border)
	}
	body := right
	if m.sidebarVisible() {
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.sidebar(bodyHeight), " ", right)
	}
	hints := tr("F10 меню · Tab разделы · Esc чат")
	if m.page == chatPage && m.session != nil {
		hints = tr("F11 история · F10 меню · Tab разделы")
	}
	if m.page == dshSettingsPage && m.settingsLogin != "" {
		hints = tr("←/→ раздел · Enter правка · F6 провайдеры")
	}
	if m.page == chatPage && len(m.toolIDs()) > 0 {
		hints = tr("F11 история · F6 инструменты · Enter детали")
	}
	if m.approvals != nil && len(m.approvals.requests) > 0 {
		hints = tr("F7 запросы · F8 остановить агента")
	}
	if m.page == chatPage && len(m.visibleQueue()) > 0 {
		hints = tr("F9 очередь · F8 остановить задачу")
	}
	footer := bar(muted().Render(hints), muted().Render(tr("Ctrl+D выход")), width)
	view = m.runtimeView(m.approvalView(m.queueDialogView(m.settingsFormView(m.goalView(m.composeView(m.benchView(m.modelsView(m.historyCodeView(m.archiveView(m.forkView(m.renameView(m.chromeView(m.historyView(header + "\n" + body + "\n" + footer))))))))))))))
	if m.prefs.Theme == "light" {
		view = lipgloss.NewStyle().Foreground(uiColor("#e6edf3")).Background(uiColor("#111820")).Width(width).Render(view)
	}
	return view
}

func (m *model) modeControls() string {
	if m.session != nil && m.session.Address.Kind == "subagent" {
		return tr("Просмотр подагента · /parent — к родителю")
	}
	names := []string{tr("Обычный"), tr("План"), tr("Цель")}
	selected := -1
	if m.session != nil && !m.sessionStale {
		selected = 0
		if m.session.Plan != nil && m.session.Plan.Active {
			selected = 1
		} else if m.session.Goal != nil && m.session.Goal.Goal.Phase == "active" {
			selected = 2
		}
	}
	text := tr("Режим: ")
	for i, name := range names {
		button := "[" + name + "]"
		if i == selected {
			button = accent().Bold(true).Render(button)
		} else {
			button = muted().Render(button)
		}
		if i > 0 {
			text += " "
		}
		text += button
	}
	return text
}
