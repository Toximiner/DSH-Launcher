package main

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func nestedEvent(id, parent, root, name string, failed bool) map[string]any {
	return map[string]any{"subCallId": id, "parentCallId": parent, "rootCallId": root, "name": name, "arguments": map[string]any{"path": "notes.txt"}, "isError": failed, "content": []any{map[string]any{"type": "text", "text": "nested output"}}}
}
func TestNestedToolsGroupByExplicitParentsAndRestore(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	records := []chatRecord{}
	add := func(seq int, kind string, data any) {
		toolEvent(t, m, seq, kind, data)
		raw, _ := json.Marshal(data)
		records = append(records, chatRecord{Type: "event", Event: chatEvent{Seq: seq, Type: kind, Data: raw}})
	}
	add(0, "tool/call", toolCall("root", "run_code", 1))
	add(1, "tool/ptc-dispatch-start", nestedEvent("first", "root", "root", "read", false))
	add(2, "tool/ptc-dispatch-start", nestedEvent("second", "root", "root", "bash", false))
	add(3, "tool/ptc-dispatch-start", nestedEvent("grandchild", "first", "root", "read", false))
	add(4, "tool/ptc-dispatch", nestedEvent("second", "root", "root", "bash", true))
	add(5, "tool/ptc-dispatch", nestedEvent("first", "root", "root", "read", false))
	add(6, "tool/result", toolResult("root", "program done", false))
	ids := m.toolIDs()
	if strings.Join(ids, ",") != "root,first,grandchild,second" {
		t.Fatalf("nested navigation did not follow explicit parent/submission order: %v", ids)
	}
	view := clean(m.chatView())
	if !strings.Contains(view, "Внутри: run_code") || !strings.Contains(view, "Внутри: read") || strings.Count(view, "bash · ! Ошибка") != 1 {
		t.Fatal("nested parent links or outcomes were lost")
	}
	m.chat.expandedTools = map[string]bool{"first": true}
	frame := &chatFrame{Type: "snapshot", Cursor: 6, Records: records}
	frame.Header.ID = "s"
	if err := m.applyChatFrame(m.chat, frame); err != nil {
		t.Fatal(err)
	}
	if len(m.toolIDs()) != 4 || !m.chat.expandedTools["first"] {
		t.Fatal("reconnect duplicated nested cards or lost expansion")
	}
	toolEvent(t, m, 7, "turn/end", map[string]any{"turn": 1, "reason": map[string]any{"kind": "aborted"}})
	for _, message := range m.chat.messages {
		if message.tool != nil && message.tool.ID == "grandchild" && message.tool.State != "stopped" {
			t.Fatal("nested running state survived cancellation")
		}
	}
	// Replacing the parent's surface result must also retire its log-only descendants.
	applyChat(t, m, `{"type":"event","event":{"seq":8,"type":"user/message","surfaceOp":{"op":"replace","startSeq":6,"endSeq":6},"data":{"content":[{"type":"text","text":"replacement"}]}}}`)
	for _, message := range m.chat.messages {
		if message.tool != nil && message.tool.Nested {
			t.Fatal("surface replacement left obsolete nested cards")
		}
	}
}
func TestNestedToolsDoNotInventOrLoopMissingRelationships(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	toolEvent(t, m, 0, "tool/ptc-dispatch", nestedEvent("outside", "missing", "missing", "read", false))
	if !strings.Contains(clean(m.chatView()), "Родитель вне окна истории") || len(m.toolIDs()) != 1 {
		t.Fatal("missing parent was invented or its child hidden")
	}
	toolEvent(t, m, 1, "tool/ptc-dispatch-start", nestedEvent("cycle-a", "cycle-b", "root", "read", false))
	toolEvent(t, m, 2, "tool/ptc-dispatch-start", nestedEvent("cycle-b", "cycle-a", "root", "bash", false))
	if len(m.displayMessages()) != 3 || !strings.Contains(clean(m.chatView()), "Связь вызовов не восстановлена") {
		t.Fatal("cyclic nesting was hidden or not marked")
	}
	toolEvent(t, m, 3, "turn/end", map[string]any{"turn": 1, "reason": map[string]any{"kind": "aborted"}})
	for _, message := range m.chat.messages {
		if message.tool != nil && message.tool.Nested && message.tool.State == "stopped" {
			t.Fatal("unknown-turn orphan was attributed to an unrelated cancellation")
		}
	}
	toolEvent(t, m, 4, "tool/call", toolCall("unrelated", "run_code", 1))
	toolEvent(t, m, 5, "tool/ptc-dispatch-start", nestedEvent("wrong-root", "unrelated", "other", "read", false))
	if depth, ok := nestedDepth(m.chat.messages[len(m.chat.messages)-1].tool, map[string]*chatTool{"unrelated": {ID: "unrelated"}}); ok || depth != 0 {
		t.Fatal("unrelated root was trusted")
	}
}
func TestNestedToolsKeyboardMouseAndSmallScreens(t *testing.T) {
	for _, size := range [][2]int{{120, 42}, {80, 24}, {44, 18}} {
		m := chatModel(t)
		applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
		toolEvent(t, m, 0, "tool/call", toolCall("root", "run_code", 1))
		toolEvent(t, m, 1, "tool/ptc-dispatch", nestedEvent("child", "root", "root", "read", false))
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.input.SetValue("preserved draft")
		key(m, tea.KeyF6)
		key(m, tea.KeyF6)
		if m.chat.selectedTool != "child" {
			t.Fatal("F6 did not reach a nested card")
		}
		key(m, tea.KeyEnter)
		if !m.chat.expandedTools["child"] || !strings.Contains(clean(m.chatView()), "nested output") {
			t.Fatal("nested details did not expand")
		}
		view := m.View()
		if lipgloss.Width(view) > m.width-1 || lipgloss.Height(view) > m.height {
			t.Fatalf("nested view overflow at %v", size)
		}
		m.rebuild()
		var hit chatToolHit
		for _, h := range m.chatToolHits {
			if h.id == "child" {
				hit = h
			}
		}
		x := 2
		if size[0] >= 79 {
			x += sidebarWidth + sidebarGap
		}
		y := bodyTop + 3 + hit.row - m.viewport.YOffset
		m.mouse(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.chat.expandedTools["child"] {
			t.Fatal("nested mouse hit did not collapse its own card")
		}
		key(m, tea.KeyEsc)
		if m.input.Value() != "preserved draft" || !m.input.Focused() {
			t.Fatal("nested navigation lost the composer draft")
		}
	}
}
func TestNestedToolErrorsAndControlsAreBounded(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	data := nestedEvent("child", "absent", "absent", "read\x1b[2J", true)
	data["content"] = []any{map[string]any{"type": "text", "text": strings.Repeat("x", chatTextLimit+1) + "\x1b[2J"}}
	data["error"] = map[string]any{"code": "ABORTED"}
	toolEvent(t, m, 0, "tool/ptc-dispatch", data)
	tool := m.chat.messages[0].tool
	if tool.State != "stopped" || len(tool.Result) > chatTextLimit+100 || strings.Contains(tool.Name, "\x1b") || strings.Contains(tool.Result, "\x1b") {
		t.Fatal("nested outcome was unsafe or unbounded")
	}
}
