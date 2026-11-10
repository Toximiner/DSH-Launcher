package main

import (
	"bytes"
	"encoding/json"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

const toolArgumentsLimit = 64 << 10

type chatTool struct {
	ID, Name, Arguments, Result, ErrorCode, State string
	Turn                                          int
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
	return strings.ToValidUTF8(text[:limit], "") + "\n[Вывод обрезан по ограничению TUI]"
}
func (message chatMessage) byteSize() int {
	size := len(message.text)
	if t := message.tool; t != nil {
		size += len(t.ID) + len(t.Name) + len(t.Arguments) + len(t.Result) + len(t.ErrorCode)
	}
	return size
}
func (m *model) toolView(t *chatTool) string {
	labels := map[string]string{"running": "◌ Выполняется", "done": "✓ Готово", "error": "! Ошибка", "stopped": "■ Остановлено", "unknown": "? Результат не получен"}
	state := labels[t.State]
	if t.State == "running" && !m.chat.ready {
		state = "? Нет связи · состояние уточняется"
	}
	if t.State != "error" && t.State != "stopped" && m.approvals != nil {
		for _, request := range m.approvals.requests {
			callID := request.Request.CallID
			if request.Request.Wait != nil {
				callID = request.Request.Wait.CallID
			}
			if callID == t.ID {
				state = "◌ Ожидает разрешения · F7"
				if request.Event == "user-questions/request" {
					state = "◌ Ожидает ответа · F7"
				}
				break
			}
		}
	}
	name := t.Name
	if name == "" {
		name = "Инструмент · вызов вне окна истории"
	}
	indicator := "▸"
	if m.chat.expandedTools[t.ID] {
		indicator = "▾"
	}
	label := indicator + " " + name + " · " + state
	style := accent.Bold(true)
	if m.toolFocused && m.chat.selectedTool == t.ID {
		label = "> " + label
		style = heading
	}
	lines := []string{style.Render(ansi.Truncate(label, max(1, m.viewport.Width), "…"))}
	args := t.Arguments
	var pretty bytes.Buffer
	if args != "" && toolJSONDepthOK(args) && json.Indent(&pretty, []byte(args), "", "  ") == nil {
		args = boundedToolText(pretty.String(), toolArgumentsLimit)
	}
	if args != "" {
		if m.chat.expandedTools[t.ID] {
			lines = append(lines, muted.Render("Аргументы:"), args)
		} else {
			lines = append(lines, muted.Render(ansi.Truncate("  "+strings.Join(strings.Fields(args), " "), max(1, m.viewport.Width), "…")))
		}
	}
	if t.ErrorCode != "" {
		lines = append(lines, "Код ошибки: "+t.ErrorCode)
	}
	if t.Result != "" {
		result := t.Result
		if !m.chat.expandedTools[t.ID] {
			wrapped := strings.Split(ansi.Hardwrap(result, max(1, m.viewport.Width), true), "\n")
			if len(wrapped) > 3 {
				result = strings.Join(wrapped[:3], "\n") + "\n… [Раскрыть]"
			}
		}
		lines = append(lines, result)
	} else if t.State == "done" {
		lines = append(lines, muted.Render("Без текстового результата."))
	}
	if m.chat.expandedTools[t.ID] {
		lines = append(lines, muted.Render("[Свернуть] · Enter / клик"))
	}
	return strings.Join(lines, "\n")
}
func (m *model) toolIDs() []string {
	var ids []string
	if m.chat != nil {
		for _, message := range m.chat.messages {
			if message.tool != nil && message.seq > m.chat.clearedThrough {
				ids = append(ids, message.tool.ID)
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
	if key == "f6" || key == "shift+f6" || m.toolFocused && (key == "up" || key == "down") {
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
		if key == "shift+f6" || key == "up" {
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
