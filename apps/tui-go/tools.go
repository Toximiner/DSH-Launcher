package main

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const toolArgumentsLimit = 64 << 10

type chatTool struct {
	ID, Name, Arguments, Result, ErrorCode, State string
	Turn                                          int
	ParentID, RootID                              string
	Nested                                        bool
	StartSeq                                      int
}
type chatToolHit struct {
	id  string
	row int
	end int
}

func boundedToolText(text string, limit int) string {
	text = clean(text)
	if len(text) <= limit {
		return text
	}
	return strings.ToValidUTF8(text[:limit], "") + tr("\n[Вывод обрезан по ограничению TUI]")
}
func (message chatMessage) byteSize() int {
	size := len(message.text) + len(message.reasoning)
	if t := message.tool; t != nil {
		size += len(t.ID) + len(t.ParentID) + len(t.RootID) + len(t.Name) + len(t.Arguments) + len(t.Result) + len(t.ErrorCode)
	}
	return size
}
func (m *model) toolView(t *chatTool) string { return m.toolViewWidth(t, m.viewport.Width) }

// toolArgumentSummary selects actual command/resource fields without replacing
// the stored arguments. Unknown JSON stays available in the expanded details.
func toolArgumentSummary(arguments string) string {
	if arguments == "" {
		return ""
	}
	if len(arguments) > toolArgumentsLimit || !toolJSONDepthOK(arguments) {
		return ""
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(arguments), &fields) != nil {
		var value string
		if json.Unmarshal([]byte(arguments), &value) == nil {
			return singleLine(value)
		}
		if json.Valid([]byte(arguments)) {
			return ""
		}
		return strings.Join(strings.Fields(clean(arguments)), " ")
	}
	text := func(key string) string {
		var value string
		if json.Unmarshal(fields[key], &value) == nil {
			return strings.Join(strings.Fields(clean(value)), " ")
		}
		return ""
	}
	for _, key := range []string{"command", "cmd"} {
		if value := text(key); value != "" {
			return value
		}
	}
	var argv []string
	if json.Unmarshal(fields["argv"], &argv) == nil && len(argv) > 0 {
		return singleLine(strings.Join(argv, " "))
	}
	resource := ""
	for _, key := range []string{"file_path", "path", "file", "filename", "url"} {
		if resource = text(key); resource != "" {
			break
		}
	}
	for _, key := range []string{"pattern", "query", "search"} {
		if value := text(key); value != "" {
			if resource != "" {
				return resource + " · " + value
			}
			return value
		}
	}
	if resource != "" {
		return resource
	}
	for _, key := range []string{"description", "code", "script"} {
		if value := text(key); value != "" {
			return value
		}
	}
	return ""
}

// Resource paths keep their identifying filename when the terminal is narrow.
// Shell commands retain their beginning, including the executable and operation.
func compactToolSummary(arguments, summary string, width int) string {
	if lipgloss.Width(summary) <= width {
		return summary
	}
	var fields map[string]json.RawMessage
	if len(arguments) <= toolArgumentsLimit && toolJSONDepthOK(arguments) && json.Unmarshal([]byte(arguments), &fields) == nil {
		for _, command := range []string{"command", "cmd", "argv"} {
			if len(fields[command]) > 0 {
				return ansi.Truncate(summary, width, "…")
			}
		}
		for _, key := range []string{"file_path", "path", "file", "filename"} {
			var resource string
			if json.Unmarshal(fields[key], &resource) == nil && resource != "" {
				return ansi.TruncateLeft(summary, lipgloss.Width(summary)-width+1, "…")
			}
		}
	}
	return ansi.Truncate(summary, width, "…")
}

func toolResultPreview(result string, tail bool) string {
	lines := strings.Split(clean(result), "\n")
	if tail {
		for i := len(lines) - 1; i >= 0; i-- {
			if line := strings.TrimSpace(lines[i]); line != "" {
				return line
			}
		}
	}
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

func (m *model) toolViewWidth(t *chatTool, width int) string {
	width = max(1, width)
	labels := map[string]string{"running": tr("◌ Выполняется"), "done": tr("✓ Готово"), "error": tr("! Ошибка"), "stopped": tr("■ Остановлено"), "unknown": tr("? Результат не получен")}
	state := labels[t.State]
	if state == "" {
		state = labels["unknown"]
	}
	if t.State == "running" && (m.chat == nil || !m.chat.ready) {
		state = tr("? Нет связи · состояние уточняется")
	}
	if t.State != "error" && t.State != "stopped" && m.approvals != nil {
		for _, request := range m.approvals.requests {
			callID := request.Request.CallID
			if request.Request.Wait != nil {
				callID = request.Request.Wait.CallID
			}
			if callID == t.ID {
				state = tr("◌ Ожидает разрешения · F7")
				if request.Event == "user-questions/request" {
					state = tr("◌ Ожидает ответа · F7")
				}
				break
			}
		}
	}
	expanded := m.chat != nil && m.chat.expandedTools[t.ID]
	result, notice := t.Result, ""
	uncertainty := []string{}
	if t.State == "running" {
		result, notice = m.liveToolOutput(t)
		if m.chat != nil {
			if job, ok := m.chat.jobs[t.ID]; ok {
				if observer := m.chat.observers[job.ID]; observer != nil {
					if observer.value.Stale {
						uncertainty = append(uncertainty, strings.TrimPrefix(tr(" · нет связи"), " · "))
					}
					if observer.value.Gap {
						uncertainty = append(uncertainty, strings.TrimPrefix(tr(" · вывод неполный"), " · "))
					}
				}
			}
		}
	}
	if strings.Contains(result, tr("\n[Вывод обрезан по ограничению TUI]")) {
		uncertainty = append(uncertainty, strings.TrimPrefix(tr(" · вывод обрезан"), " · "))
	}
	name := singleLine(t.Name)
	if name == "" {
		name = tr("Инструмент · вызов вне окна истории")
	}
	indicator := "▸"
	if expanded {
		indicator = "▾"
	}
	prefix := indicator + " "
	focused := m.toolFocused && m.chat != nil && m.chat.selectedTool == t.ID
	if focused {
		prefix = "> " + prefix
	}
	// If state and loss flags cannot share a readable header, reserve the
	// second row for those flags before nesting or output previews.
	detailUncertainty := ""
	if len(uncertainty) > 0 {
		flags := strings.Join(uncertainty, " · ")
		required := lipgloss.Width(prefix) + min(8, lipgloss.Width(name)) + lipgloss.Width(state) + lipgloss.Width(flags) + 6
		if required <= width {
			state += " · " + flags
		} else {
			detailUncertainty = flags
		}
	}
	// Keep uncertainty and errors visible before spending space on arguments.
	name = ansi.Truncate(name, max(1, width-lipgloss.Width(prefix)-lipgloss.Width(state)-3), "…")
	label := prefix + name + " · " + state
	summary := toolArgumentSummary(t.Arguments)
	if available := width - lipgloss.Width(label) - 3; available >= 6 && summary != "" {
		label += " · " + compactToolSummary(t.Arguments, summary, available)
	}
	style := muted()
	if t.State == "running" {
		style = accent()
	}
	if t.State == "error" {
		style = lipgloss.NewStyle().Foreground(theme.Color(theme.Error))
	}
	if t.State == "unknown" || t.State == "stopped" || labels[t.State] == "" {
		style = lipgloss.NewStyle().Foreground(theme.Color(theme.Warning))
	}
	if focused {
		style = style.Bold(true)
	}
	lines := []string{style.Render(ansi.Truncate(label, width, "…"))}
	if !expanded {
		detail := []string{}
		if detailUncertainty != "" {
			detail = append(detail, detailUncertainty)
		}
		if t.Nested && m.chat != nil {
			detail = append(detail, m.nestedRelation(t))
		}
		if t.ErrorCode != "" {
			detail = append(detail, tr("Код ошибки: ")+singleLine(t.ErrorCode))
		}
		if preview := toolResultPreview(result, t.State == "running"); preview != "" {
			detail = append(detail, preview)
		}
		if len(detail) > 0 {
			lines = append(lines, muted().Render(ansi.Truncate(strings.Join(detail, " · "), width, "…")))
		}
		return strings.Join(lines, "\n")
	}
	if t.Nested && m.chat != nil {
		lines = append(lines, muted().Render(m.nestedRelation(t)))
	}
	args := t.Arguments
	var pretty bytes.Buffer
	if args != "" && len(args) <= toolArgumentsLimit && toolJSONDepthOK(args) && json.Indent(&pretty, []byte(args), "", "  ") == nil {
		args = boundedToolText(pretty.String(), toolArgumentsLimit)
	}
	if args != "" {
		lines = append(lines, muted().Render(tr("Аргументы:")), args)
	}
	if t.ErrorCode != "" {
		lines = append(lines, tr("Код ошибки: ")+singleLine(t.ErrorCode))
	}
	if notice != "" {
		lines = append(lines, muted().Render(notice))
	}
	if result != "" {
		lines = append(lines, result)
	} else if t.State == "done" {
		lines = append(lines, muted().Render(tr("Без текстового результата.")))
	}
	lines = append(lines, muted().Render(tr("[Свернуть] · Enter / клик")))
	return strings.Join(lines, "\n")
}
func (m *model) toolIDs() []string {
	var ids []string
	if m.chat != nil {
		for _, display := range m.displayMessages() {
			if display.message.tool != nil {
				ids = append(ids, display.message.tool.ID)
			}
		}
	}
	return ids
}
func (m *model) focusTool(id string) {
	m.toolFocused = true
	m.navFocused = false
	m.chat.selectedTool = id
	m.input.Blur()
	m.rebuild()
	for _, hit := range m.chatToolHits {
		if hit.id == id {
			m.viewport.SetYOffset(hit.row)
			break
		}
	}
}
func (m *model) toggleTool(id string) {
	if m.chat.expandedTools == nil {
		m.chat.expandedTools = make(map[string]bool)
	}
	m.chat.expandedTools[id] = !m.chat.expandedTools[id]
	offset := m.viewport.YOffset
	m.rebuild()
	m.viewport.SetYOffset(offset)
	if m.toolFocused && m.chat.selectedTool == id {
		for _, hit := range m.chatToolHits {
			if hit.id == id {
				m.viewport.SetYOffset(hit.row)
				break
			}
		}
	}
}
func (m *model) toolKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.page != chatPage || m.chat == nil || msg.Paste || msg.Type == tea.KeyRunes && !msg.Alt {
		return nil, false
	}
	key := msg.String()
	ids := m.toolIDs()
	if key == "f6" || key == "shift+f6" || key == "f18" || m.toolFocused && (key == "up" || key == "down") {
		if len(ids) == 0 {
			return nil, true
		}
		index := -1
		for i, id := range ids {
			if id == m.chat.selectedTool {
				index = i
				break
			}
		}
		delta := 1
		if key == "shift+f6" || key == "f18" || key == "up" {
			delta = -1
			if index < 0 {
				index = 0
			}
		}
		m.focusTool(ids[(index+delta+len(ids))%len(ids)])
		return nil, true
	}
	if !m.toolFocused {
		return nil, false
	}
	switch key {
	case "enter":
		for _, id := range ids {
			if id == m.chat.selectedTool {
				m.toggleTool(id)
				break
			}
		}
		return nil, true
	case "esc":
		m.toolFocused = false
		m.rebuild()
		return m.input.Focus(), true
	case "tab":
		m.toolFocused = false
		return nil, false
	}
	return nil, false
}

// Expansion state follows only retained cards, so long sessions stay bounded.
func pruneToolExpansion(s *chatState) {
	next := make(map[string]bool)
	for _, message := range s.messages {
		if message.tool != nil && s.expandedTools[message.tool.ID] {
			next[message.tool.ID] = true
		}
	}
	s.expandedTools = next
}

// Do not amplify adversarially nested JSON into megabytes of indentation.
func toolJSONDepthOK(text string) bool {
	depth := 0
	quoted, escaped := false, false
	for _, r := range text {
		if quoted {
			if escaped {
				escaped = false
			} else if r == '\\' {
				escaped = true
			} else if r == '"' {
				quoted = false
			}
		} else {
			switch r {
			case '"':
				quoted = true
			case '{', '[':
				depth++
				if depth > 32 {
					return false
				}
			case '}', ']':
				depth--
			}
		}
	}
	return true
}
