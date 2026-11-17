package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
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

func panel(width, height int, content string, color theme.Token) string {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(theme.Color(color)).
		Foreground(theme.Color(theme.Text)).Background(theme.Color(theme.Panel)).Padding(0, 1)
	return style.Width(max(1, width-style.GetHorizontalBorderSize())).
		Height(max(1, height-style.GetVerticalFrameSize())).Render(content)
}

func bar(left, right string, width int) string {
	left = ansi.Truncate(left, max(1, width-lipgloss.Width(right)-1), "…")
	return left + strings.Repeat(" ", max(1, width-lipgloss.Width(left)-lipgloss.Width(right))) + right
}

func (m *model) View() string {
	if m.selectionSnapshot != "" {
		return m.selectionSnapshot
	}
	theme.SetLight(m.prefs.Theme == "light")
	activeEnglish.Store(m.prefs.Language == "en")
	if !m.terminalFits() {
		return m.terminalSizeNotice()
	}
	width := m.width - 1
	w, bodyHeight := m.contentSize()

	status := lipgloss.NewStyle().Foreground(theme.Color(theme.Warning)).Render(tr("○ Нет подключения"))
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
	statusLine := ""
	if m.sessionRequestVisible() || m.sessionError != "" || m.settingsError != "" || m.chat != nil && (!m.chat.ready || m.chat.streamError != "") {
		statusLine = ansi.Truncate(status, width, "…")
	}
	header := m.tabRail() + "\n" + statusLine
	title := tr(pages[m.page])
	if m.page == sessionsPage {
		title = tr("Сессии dsh")
	}
	if m.page == logPage {
		title = tr("Журнал клиента · локально")
	}
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
	right := panel(w, bodyHeight, view, theme.Border)
	if m.page == pluginsPage && m.workbench != nil && m.workbench.kind == "plugins" {
		box := m.pluginsPageBox()
		right = lipgloss.PlaceHorizontal(w, lipgloss.Center, m.pluginsContent(box))
	}
	if m.page == marketPage {
		right = lipgloss.PlaceHorizontal(w, lipgloss.Center, m.viewport.View())
	}
	if m.page == schedulesPage && m.workbench != nil && m.workbench.kind == "schedules" {
		box := m.pluginsPageBox()
		right = lipgloss.PlaceHorizontal(w, lipgloss.Center, m.schedulesContent(box))
	}
	if m.page == 0 {
		right = m.conversationLayoutView(m.conversationView(title, m.chatColumn().width, bodyHeight))
		if m.landingVisible() {
			column := m.chatColumn()
			right = lipgloss.PlaceHorizontal(w, lipgloss.Center, m.landingView(title, column.width, bodyHeight))
		}
	}
	body := right
	hints := m.footerHints()
	footer := bar(muted().Render(hints), muted().Render(tr("Ctrl+P команды")), width)
	if m.page == chatPage {
		footer = m.conversationFooter()
	}
	view = m.paletteView(m.runtimeView(m.approvalView(m.queueDialogView(m.settingsFormView(m.goalView(m.composeView(m.benchView(m.modelsView(m.historyCodeView(m.archiveView(m.forkView(m.renameView(m.chromeView(m.historyView(header + "\n" + body + "\n" + footer)))))))))))))))
	view = m.sessionPickerView(view)
	view = lipgloss.NewStyle().Foreground(theme.Color(theme.Text)).Background(theme.Color(theme.Bg)).Width(width).Height(m.height).Render(view)
	return surface(view, theme.Bg)
}

// footerContextUsage returns opencode-style context usage, e.g. "8.7K (4%)".
func (m *model) footerContextUsage() string {
	if m.session == nil || m.sessionStale {
		return ""
	}
	p := m.session.Pressure
	if p == nil || p.Capacity == nil || *p.Capacity <= 0 || !validStat(*p.Capacity) {
		return ""
	}
	value := p.Projected
	if value == nil {
		value = p.Pressure
	}
	if value == nil || !validStat(*value) {
		return ""
	}
	return fmt.Sprintf("%s (%.0f%%)", countLabel(*value), 100*float64(*value)/float64(*p.Capacity))
}

// footerHints builds the compact status line shown in the footer bar.
func (m *model) footerHints() string {
	hint := tr("Ctrl+N сессия · Esc чат")
	switch {
	case m.approvals != nil && len(m.approvals.requests) > 0:
		hint = tr("F7 запросы · F8 остановить")
	case m.page == chatPage && len(m.visibleQueue()) > 0:
		hint = tr("F9 очередь · F8 остановить")
	case m.page == chatPage && m.session != nil:
		hint = tr("F6 инструменты · F8 стоп")
	case m.page == dshSettingsPage && m.settingsLogin != "":
		hint = tr("←/→ раздел · Enter правка")
	case m.page != chatPage:
		hint = tr("Esc чат · F5 обновить")
	}
	if usage := m.footerContextUsage(); usage != "" {
		return usage + " · " + hint
	}
	return hint
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
