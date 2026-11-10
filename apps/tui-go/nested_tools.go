package main

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type nestedToolEvent struct {
	RootID    string          `json:"rootCallId"`
	ParentID  string          `json:"parentCallId"`
	ID        string          `json:"subCallId"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Content   []queueContent  `json:"content"`
	IsError   bool            `json:"isError"`
	Error     *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func nestedTool(s *chatState, event chatEvent) (*chatTool, error) {
	var data nestedToolEvent
	if json.Unmarshal(event.Data, &data) != nil {
		return nil, errors.New("неподдерживаемые данные вложенного вызова")
	}
	for _, id := range []string{data.ID, data.ParentID, data.RootID} {
		if id == "" || len(id) > 1024 {
			return nil, errors.New("вложенный вызов без корректных идентификаторов")
		}
	}
	tool := &chatTool{ID: data.ID, ParentID: data.ParentID, RootID: data.RootID, Nested: true, StartSeq: event.Seq, Turn: -1, Name: boundedToolText(data.Name, 256), Arguments: boundedToolText(string(data.Arguments), toolArgumentsLimit), State: "running"}
	for _, message := range s.messages {
		if p := message.tool; p != nil {
			if p.ID == tool.ID {
				tool.StartSeq = p.StartSeq
			}
			if p.ID == tool.RootID && !p.Nested {
				tool.Turn = p.Turn
			}
		}
	}
	if event.Type == "tool/ptc-dispatch" {
		tool.State = "done"
		tool.Result = boundedToolText(contentText(data.Content), chatTextLimit)
		if data.IsError || data.Error != nil {
			tool.State = "error"
		}
		if data.Error != nil {
			tool.ErrorCode = boundedToolText(data.Error.Code, 128)
			if data.Error.Code == "ABORTED" {
				tool.State = "stopped"
			}
		}
	}
	return tool, nil
}

type chatDisplay struct {
	message chatMessage
	depth   int
}

// A nesting edge is trusted only when its explicit chain reaches its declared root.
func nestedDepth(t *chatTool, tools map[string]*chatTool) (int, bool) {
	if !t.Nested {
		return 0, true
	}
	seen := map[string]bool{t.ID: true}
	parent := t.ParentID
	for depth := 1; depth <= len(tools); depth++ {
		p := tools[parent]
		if p == nil || seen[p.ID] {
			return 0, false
		}
		seen[p.ID] = true
		if !p.Nested {
			if p.ID != t.RootID {
				return 0, false
			}
			return depth, true
		}
		if p.RootID != t.RootID {
			return 0, false
		}
		parent = p.ParentID
	}
	return 0, false
}
func (m *model) displayMessages() []chatDisplay {
	if m.chat == nil {
		return nil
	}
	var messages []chatMessage
	tools := map[string]*chatTool{}
	for _, message := range m.chat.messages {
		if message.seq > m.chat.clearedThrough {
			messages = append(messages, message)
			if message.tool != nil {
				tools[message.tool.ID] = message.tool
			}
		}
	}
	children := map[string][]chatMessage{}
	nested := map[string]bool{}
	for _, message := range messages {
		if t := message.tool; t != nil && t.Nested {
			if _, ok := nestedDepth(t, tools); ok {
				children[t.ParentID] = append(children[t.ParentID], message)
				nested[t.ID] = true
			}
		}
	}
	for parent := range children {
		sort.SliceStable(children[parent], func(i, j int) bool { return children[parent][i].tool.StartSeq < children[parent][j].tool.StartSeq })
	}
	var result []chatDisplay
	var emit func(chatMessage, int)
	emit = func(message chatMessage, depth int) {
		result = append(result, chatDisplay{message: message, depth: depth})
		if message.tool != nil {
			for _, child := range children[message.tool.ID] {
				emit(child, depth+1)
			}
		}
	}
	for _, message := range messages {
		if message.tool == nil || !nested[message.tool.ID] {
			emit(message, 0)
		}
	}
	return result
}
func (m *model) nestedRelation(t *chatTool) string {
	tools := map[string]*chatTool{}
	for _, message := range m.chat.messages {
		if message.tool != nil && message.seq > m.chat.clearedThrough {
			tools[message.tool.ID] = message.tool
		}
	}
	id := boundedToolText(t.ParentID, 128)
	if parent := tools[t.ParentID]; parent != nil {
		if _, ok := nestedDepth(t, tools); ok {
			name := parent.Name
			if name == "" {
				name = "Инструмент"
			}
			label := "Внутри: " + name
			if m.chat.expandedTools[t.ID] {
				label += " · " + id
			}
			return label
		}
		return "Связь вызовов не восстановлена · " + id
	}
	return "Родитель вне окна истории · " + id
}
func (m *model) displayTool(t *chatTool, depth int) string {
	indent := 0
	if t.Nested {
		indent = 2 + min(depth, 4)*2
	}
	width := max(1, m.viewport.Width-indent)
	card := m.toolViewWidth(t, width)
	if !t.Nested {
		return card
	}
	lines := strings.Split(ansi.Hardwrap(card, width, true), "\n")
	for i, line := range lines {
		prefix := strings.Repeat(" ", indent)
		if i == 0 {
			prefix = strings.Repeat(" ", indent-2) + "↳ "
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
