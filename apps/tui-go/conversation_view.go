package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
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
	return m.page == chatPage && !m.landingVisible()
}
func (m *model) chatColumn() area {
	return m.conversationLayout().chat
}
func (m *model) conversationArea() area {
	x, y := 0, bodyTop+3
	if m.sidebarVisible() {
		x = sidebarWidth + sidebarGap
	}
	if m.page == chatPage {
		x = m.chatColumn().x
		y = bodyTop
	}
	return area{x + 2, y, m.viewport.Width, m.viewport.Height}
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

// Session identity lives in the tab rail; composer controls carry the agent
// and model. Keep this compatibility API inert rather than leave invisible hits.
func (m *model) conversationHeader(title string) (string, string, []agentHit) {
	return "", "", nil
}
func (m *model) conversationView(title string, w, h int) string {
	column := m.chatColumn()
	lines := []string{}
	chatHeight := max(1, h-conversationComposerHeight)
	for _, line := range strings.Split(m.viewport.View(), "\n") {
		lines = append(lines, "  "+ansi.Truncate(line, column.width-4, "…"))
	}
	if len(lines) > chatHeight {
		lines = lines[:chatHeight]
	}
	for len(lines) < chatHeight {
		lines = append(lines, "")
	}
	lines = append(lines, m.conversationComposerView())
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

// Projected records can outlive an interface-language switch. Translate known
// application role keys at render time while preserving semantic reply labels.
func conversationRoleLabel(role string) string {
	for _, source := range []string{"ВЫ", "ВЫ · ОТВЕТЫ НА ВОПРОС", "АГЕНТ", "АГЕНТ · НЕЗАВЕРШЁННЫЙ ОТВЕТ", "СООБЩЕНИЕ DSH", "РАЗРЕШЕНИЕ", "РАЗРЕШЕНИЕ · ОЖИДАЕТ ОТВЕТА"} {
		if role == source || (englishCatalog[source] != "" && role == englishCatalog[source]) {
			return tr(source)
		}
	}
	return singleLine(role)
}
func (m *model) conversationMessage(message chatMessage, width, row int) string {
	width = max(4, width)
	role := conversationRoleLabel(message.role)
	human := message.human || role == tr("ВЫ") || strings.HasPrefix(role, tr("ВЫ ·"))
	part := ""
	if human {
		wrapped := ansi.Hardwrap(clean(message.text), width-2, true)
		rows := strings.Split(wrapped, "\n")
		surface := lipgloss.NewStyle().Width(width - 1).Foreground(theme.Color(theme.Text)).Background(theme.Color(theme.Panel))
		for i, line := range rows {
			rows[i] = accent().Render("┃") + surface.Render(" "+line)
		}
		part = strings.Join(rows, "\n")
	} else {
		part = ansi.Hardwrap(m.messageText(message, width, false), width, true)
	}
	if message.reasoning != "" {
		thoughts := renderReasoning(m.chat, reasoningEventKey(message.seq), message.reasoning, width, row)
		if part != "" {
			part = thoughts + "\n" + part
		} else {
			part = thoughts
		}
	}
	part = strings.TrimRight(part, "\n")
	if meta := messageMetadata(message, width); meta != "" {
		part += "\n" + muted().Render(meta)
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
		if (message.assistant || message.role == tr("АГЕНТ")) && message.text != "" && message.tool == nil && message.approvalID == "" {
			return m.copyConversation(message.seq), true
		}
	}
	return nil, true
}
func (m *model) conversationMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if !m.conversationVisible() {
		return nil, false
	}
	_, _, hits := m.landingControls()
	for _, hit := range hits {
		if hit.y == m.composerBox().y+3 && hit.contains(msg.X, msg.Y) {
			m.navFocused, m.toolFocused = false, false
			m.input.Focus()
			return m.agentControl(hit.action), true
		}
	}
	return nil, false
}
