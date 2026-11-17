package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

// Delivery remains explicit: an accepted request is not yet a recorded reply.
func promptDeliveryLabel(status string) string {
	switch status {
	case "sending":
		return tr("◌ Отправка…")
	case "accepted":
		return tr("◌ Ожидание ответа…")
	case "unknown":
		return tr("! Отправка не подтверждена · /retry-send")
	case "rejected":
		return tr("! Не отправлено")
	default:
		return tr("◌ Ожидание ответа…")
	}
}

func pendingPromptView(text, status string, width int) string {
	inner := max(1, width-3)
	lines := strings.Split(ansi.Hardwrap(clean(text), inner, true), "\n")
	for i, line := range lines {
		lines[i] = accent().Render("┃") + " " + line + strings.Repeat(" ", max(0, width-2-lipgloss.Width(line)))
	}
	return surface(strings.Join(lines, "\n"), theme.Panel) + "\n" + muted().Render(ansi.Truncate(promptDeliveryLabel(status), max(1, width), "…")) + "\n"
}

// Keep send/queue/readonly guidance visible before optional usage statistics.
func (m *model) conversationFooter() string {
	_, hint := m.composerText()
	width := m.chatColumn().width
	stateParts := strings.Split(m.conversationStatus(), " · ")
	var details []string
	for _, part := range stateParts {
		if part != tr("Режим: ")+m.modeLabel() {
			details = append(details, part)
		}
	}
	state := strings.Join(details, " · ")
	if m.chat != nil && !m.chat.ready {
		state = tr("◌ Подключение чата…")
		if m.chat.streamError != "" {
			state = tr("! Чат: нет связи")
		}
	} else if m.session != nil {
		if p := m.prompts[m.session.ID]; p != nil {
			state = promptDeliveryLabel(p.Status)
		} else if m.session.Running || m.chat != nil && m.chat.live != nil {
			state = tr("◌ Агент отвечает…")
		}
	}
	// The actionable hint takes precedence; long diagnostics remain in the transcript.
	if m.session != nil {
		hint = m.modeLabel() + " · " + hint
	}
	left := ansi.Truncate(hint, max(1, width), "…")
	remaining := width - lipgloss.Width(left) - 2
	if remaining < 10 || state == "" {
		return muted().Render(left)
	}
	return bar(muted().Render(left), muted().Render(ansi.Truncate(state, remaining, "…")), width)
}
