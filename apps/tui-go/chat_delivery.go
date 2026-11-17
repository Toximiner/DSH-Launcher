package main

import (
	"os"
	"path/filepath"
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

// One quiet footer mirrors the reference: workspace at left, context/actions at right.
// Exceptional states replace idle metrics, keeping recovery and readonly guidance visible.
func (m *model) conversationFooter() string {
	width := m.chatColumn().width
	if m.landingVisible() {
		return bar("", muted().Render(version), max(1, m.width-1))
	}
	path := singleLine(filepath.Clean(m.workspace))
	if home, err := os.UserHomeDir(); err == nil && (path == home || strings.HasPrefix(path, home+string(filepath.Separator))) {
		path = "~" + strings.TrimPrefix(path, home)
	}

	state := ""
	if m.chat != nil && m.chat.copyNote != "" {
		state = singleLine(m.chat.copyNote)
	}
	if m.chat != nil && !m.chat.ready {
		state = tr("◌ Подключение чата…")
		if m.chat.streamError != "" {
			state = tr("! Чат: нет связи")
		}
	} else if m.session != nil {
		if p := m.prompts[m.session.ID]; p != nil {
			state = promptDeliveryLabel(p.Status)
		} else if m.session.Running || m.chat != nil && m.chat.live != nil {
			_, state = m.composerText()
		} else if m.session.Address.Kind == "subagent" || m.isSessionArchived(m.session.ID) {
			_, state = m.composerText()
		}
	}
	if state == "" {
		state = m.footerContextUsage()
	}
	if m.session != nil && m.modeLabel() != tr("обычный") {
		state = m.modeLabel() + " · " + state
		state = strings.TrimSuffix(state, " · ")
	}
	commands := tr("Ctrl+P команды")
	right := commands
	if state != "" {
		right = ansi.Truncate(state, max(1, width-min(12, width/3)-lipgloss.Width(commands)-3), "…") + "  " + commands
	}
	return bar(muted().Render(path), muted().Render(right), width)
}
