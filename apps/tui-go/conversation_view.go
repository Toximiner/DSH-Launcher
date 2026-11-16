package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const conversationMaxWidth = 100

type conversationStats struct {
	Turns        int64 `json:"turns"`
	Steps        int64 `json:"steps"`
	DecodeMs     int64 `json:"decodeMs"`
	DecodeTokens int64 `json:"decodeTokens"`
}
type conversationUsage struct {
	Input      int64 `json:"uncachedInputTokens"`
	Output     int64 `json:"outputTokens"`
	CacheRead  int64 `json:"cacheReadTokens"`
	CacheWrite int64 `json:"cacheWriteTokens"`
}
type conversationPressure struct {
	Projected *int64 `json:"projectedTokens"`
	Pressure  *int64 `json:"pressureTokens"`
	Capacity  *int64 `json:"contextWindow"`
}
type conversationCopyHit struct{ row, x, width, seq int }
type conversationCopyResult struct {
	owner     *chatState
	operation int
	receipt   string
	err       error
}

var conversationMenu = []menuEntry{
	{"История сессии", "history"}, {"Переименовать…", "rename"},
	{"Создать ветку…", "fork"}, {"Архивировать…", "archive"},
}

func (m *model) conversationVisible() bool {
	return m.page == chatPage && m.chat != nil && m.session != nil && !m.landingVisible()
}
func (m *model) chatColumn() area {
	w, h := m.contentSize()
	x := 0
	if m.sidebarVisible() {
		x = sidebarWidth + sidebarGap
	}
	column := min(w, conversationMaxWidth)
	return area{x + (w-column)/2, bodyTop, column, h}
}
func (m *model) conversationArea() area {
	x := 0
	if m.sidebarVisible() {
		x = sidebarWidth + sidebarGap
	}
	if m.page == chatPage {
		x = m.chatColumn().x
	}
	return area{x + 2, bodyTop + 3, m.viewport.Width, m.viewport.Height}
}
func (m *model) agentPresetLabel() string {
	label := tr("Агент")
	if m.session != nil && m.session.AgentPreset != "" {
		label = m.session.AgentPreset
		if m.agentInfo != nil && m.agentInfo.presets[label] != "" {
			label = m.agentInfo.presets[label]
		}
	}
	return singleLine(label)
}
func (m *model) conversationHeader(title string) (string, string, []agentHit) {
	box := m.chatColumn()
	inner := box.width - 4
	if title == "" && m.session != nil {
		name := m.session.Title
		if name == "" {
			name = m.session.ID
		}
		title = tr("Сессия · ") + name
	}
	title = singleLine(title)
	preset := "[" + ansi.Truncate(m.agentPresetLabel(), max(5, inner/3)-2, "…") + "]"
	more := "[···]"
	name := ansi.Truncate(title, max(1, inner-lipgloss.Width(preset)-lipgloss.Width(more)-3), "…")
	left := heading().Render(name) + " " + accent().Render(preset)
	top := bar(left, accent().Render(more), inner)
	tab := "[" + tr("Чат") + "]"
	trajectory := "[" + tr("Траектория · F11") + "]"
	tabs := accent().Bold(true).Render(tab) + "  " + muted().Render(trajectory)
	hits := []agentHit{
		{area{box.x + 2 + lipgloss.Width(name) + 1, box.y, lipgloss.Width(preset), 1}, "presets"},
		{area{box.x + box.width - 2 - lipgloss.Width(more), box.y, lipgloss.Width(more), 1}, "conversation"},
		{area{box.x + 2, box.y + 1, lipgloss.Width(tab), 1}, "chat"},
		{area{box.x + 2 + lipgloss.Width(tab) + 2, box.y + 1, lipgloss.Width(trajectory), 1}, "history"},
	}
	return top, tabs, hits
}
func (m *model) conversationView(title string, w, h int) string {
	column := m.chatColumn()
	top, tabs, _ := m.conversationHeader(title)
	lines := []string{"  " + top, "  " + tabs, "  " + muted().Render(strings.Repeat("─", m.viewport.Width))}
	for _, line := range strings.Split(m.viewport.View(), "\n") {
		lines = append(lines, "  "+line)
	}
	// Keep the viewport origin and composer geometry stable at every supported size.
	chatHeight := h - composerHeight - 1
	for len(lines) < chatHeight {
		lines = append(lines, "")
	}
	_, tools, _ := m.landingControls()
	label, hint := m.composerText()
	border := "#58a6ff"
	if m.navFocused {
		border = "#303d4d"
	}
	composer := heading().Render(tr("Сообщение")) + muted().Render(label) + "\n" + m.input.View() + "\n" + tools + "\n" + muted().Render(ansi.Truncate(hint, column.width-4, "…"))
	lines = append(lines, panel(column.width, composerHeight, composer, border))
	lines = append(lines, muted().Render(ansi.Truncate(m.conversationStatus(), column.width, "…")))
	content := strings.Join(lines, "\n")
	return lipgloss.PlaceHorizontal(w, lipgloss.Center, lipgloss.NewStyle().Width(column.width).Height(h).Render(content))
}
func countLabel(value int64) string {
	if value >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(value)/1000000)
	}
	if value >= 1000 {
		return fmt.Sprintf("%.1fK", float64(value)/1000)
	}
	return fmt.Sprint(value)
}
func validStat(value int64) bool { return value >= 0 && value <= 1<<53 }
func (m *model) conversationStatus() string {
	if m.session == nil {
		return ""
	}
	parts := []string{tr("Режим: ") + m.modeLabel()}
	if p := m.session.Stats; p != nil && validStat(p.Turns) && validStat(p.Steps) {
		parts = append(parts, fmt.Sprintf(tr("%d ход · %d шаг"), p.Turns, p.Steps))
		if p.DecodeMs > 0 && validStat(p.DecodeMs) && validStat(p.DecodeTokens) {
			parts = append(parts, fmt.Sprintf(tr("%.1f ток/с"), float64(p.DecodeTokens)*1000/float64(p.DecodeMs)))
		}
	}
	if p := m.session.Usage; p != nil && validStat(p.Input) && validStat(p.Output) && validStat(p.CacheRead) && validStat(p.CacheWrite) {
		input := p.Input + p.CacheRead + p.CacheWrite
		parts = append(parts, countLabel(input+p.Output)+tr(" ток"))
		if input > 0 {
			parts = append(parts, fmt.Sprintf(tr("кэш %.0f%%"), 100*float64(p.CacheRead)/float64(input)))
		}
	}
	if p := m.session.Pressure; p != nil && p.Capacity != nil && *p.Capacity > 0 && validStat(*p.Capacity) {
		value := p.Projected
		if value == nil {
			value = p.Pressure
		}
		if value != nil && validStat(*value) {
			parts = append(parts, fmt.Sprintf(tr("контекст ≈%.0f%%"), 100*float64(*value)/float64(*p.Capacity)))
		}
	}
	if m.chat != nil && m.chat.copyNote != "" {
		parts = append([]string{singleLine(m.chat.copyNote)}, parts...)
	}
	return strings.Join(parts, " · ")
}
func readConversationProjection(values map[string]json.RawMessage, view *sessionView) {
	var stats *conversationStats
	var usage *conversationUsage
	var pressure *conversationPressure
	if json.Unmarshal(values["sessionStats"], &stats) == nil {
		view.Stats = stats
	}
	if json.Unmarshal(values["tokenUsage"], &usage) == nil {
		view.Usage = usage
	}
	if json.Unmarshal(values["contextPressure"], &pressure) == nil {
		view.Pressure = pressure
	}
}
func messageClock(ms int64) string {
	if ms <= 0 || ms > 253402300799999 {
		return ""
	}
	return time.UnixMilli(ms).Local().Format("15:04")
}
func (m *model) conversationMessage(message chatMessage, width, row int) string {
	human := message.human || message.role == tr("ВЫ") || strings.HasPrefix(message.role, tr("ВЫ ·"))
	content := m.messageText(message, width, false)
	role := message.role
	if message.assistant {
		role = tr("АГЕНТ")
	}
	label := accent().Bold(true).Render(role)
	if message.elapsedMs > 0 {
		label += " · " + muted().Render(fmt.Sprintf(tr("Ответ за %.1f с"), float64(message.elapsedMs)/1000))
	}
	meta := "[" + tr("Копировать") + "]"
	if clock := messageClock(message.time); clock != "" {
		meta += "  " + clock
	}
	canCopy := message.tool == nil && message.approvalID == "" && message.text != ""
	if !canCopy {
		meta = messageClock(message.time)
	}
	part := ""
	copyX := 0
	if human {
		bubbleMax := max(4, min(width, max(12, width*3/4)))
		wrapped := ansi.Hardwrap(message.text, bubbleMax-2, true)
		bubbleWidth := min(bubbleMax, lipgloss.Width(wrapped)+2)
		bubble := lipgloss.NewStyle().Width(bubbleWidth).Padding(0, 1).Background(uiColor("#233044")).Foreground(uiColor("#e6edf3")).Render(wrapped)
		part = lipgloss.PlaceHorizontal(width, lipgloss.Right, label) + "\n" + lipgloss.PlaceHorizontal(width, lipgloss.Right, bubble)
		copyX = max(0, width-lipgloss.Width(meta))
	} else {
		part = label + "\n" + content
	}
	if meta != "" {
		meta = ansi.Truncate(meta, width, "…")
		if canCopy {
			m.chatCopyHits = append(m.chatCopyHits, conversationCopyHit{row: row + strings.Count(ansi.Hardwrap(part, width, true), "\n") + 1, x: copyX, width: lipgloss.Width("[" + tr("Копировать") + "]"), seq: message.seq})
		}
		if human {
			meta = lipgloss.PlaceHorizontal(width, lipgloss.Right, muted().Render(meta))
		} else {
			meta = muted().Render(meta)
		}
		part += "\n" + meta
	}
	return part + "\n"
}
func (m *model) copyConversation(seq int) tea.Cmd {
	s := m.chat
	if s == nil || !s.ready || s.ctx.Err() != nil || s.copyBusy {
		return nil
	}
	var source string
	for _, display := range m.displayMessages() {
		message := display.message
		if message.seq == seq && message.tool == nil && message.approvalID == "" {
			source = message.text
			break
		}
	}
	if source == "" || len(source) > clipboardTextLimit {
		s.copyNote = tr("Фрагмент пуст или больше 64 КиБ; буфер не изменён.")
		return nil
	}
	s.copyBusy = true
	s.copyOperation++
	operation, ctx, write := s.copyOperation, s.ctx, m.copyClipboard
	if write == nil {
		write = writeClipboard
	}
	return func() tea.Msg {
		receipt, err := write(ctx, source)
		return conversationCopyResult{s, operation, receipt, err}
	}
}
func (m *model) handleConversationCopy(r conversationCopyResult) {
	s := m.chat
	if s == nil || s != r.owner || s.ctx.Err() != nil || s.copyOperation != r.operation {
		return
	}
	s.copyBusy = false
	s.copyNote = r.receipt
	if r.err != nil {
		s.copyNote = tr("Копирование не подтверждено.")
	}
}
func (m *model) conversationKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if msg.Type != tea.KeyCtrlY || msg.Paste || !m.conversationVisible() || m.navFocused || m.toolFocused {
		return nil, false
	}
	messages := m.displayMessages()
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i].message
		if (message.assistant || message.role == tr("АГЕНТ")) && message.tool == nil && message.approvalID == "" {
			return m.copyConversation(message.seq), true
		}
	}
	return nil, true
}
func (m *model) conversationMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if !m.conversationVisible() {
		return nil, false
	}
	_, _, headerHits := m.conversationHeader("")
	for _, hit := range headerHits {
		if hit.contains(msg.X, msg.Y) {
			m.navFocused, m.toolFocused = false, false
			m.input.Focus()
			return m.agentControl(hit.action), true
		}
	}
	_, _, hits := m.landingControls()
	for _, hit := range hits {
		if hit.y == m.composerBox().y+3 && hit.contains(msg.X, msg.Y) {
			m.navFocused, m.toolFocused = false, false
			m.input.Focus()
			return m.agentControl(hit.action), true
		}
	}
	content := m.conversationArea()
	if content.contains(msg.X, msg.Y) {
		row := msg.Y - content.y + m.viewport.YOffset
		for _, hit := range m.chatCopyHits {
			if row == hit.row && msg.X >= content.x+hit.x && msg.X < content.x+hit.x+hit.width {
				return m.copyConversation(hit.seq), true
			}
		}
	}
	return nil, false
}
