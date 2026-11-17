package main

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func toolEvent(t *testing.T, m *model, seq int, kind string, data any) {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	event := chatEvent{Seq: seq, Type: kind, Data: raw}
	if kind == "tool/result" {
		event.SurfaceOp = json.RawMessage(`"append"`)
	}
	if err := m.applyChatFrame(m.chat, &chatFrame{Type: "event", Event: event}); err != nil {
		t.Fatal(err)
	}
}
func toolCall(id, name string, turn int) any {
	return map[string]any{"callId": id, "name": name, "turn": turn, "arguments": `{"file_path":"notes.txt"}`}
}
func toolResult(id, text string, failed bool) any {
	return map[string]any{"turn": 1, "message": map[string]any{"source": map[string]any{"kind": "tool", "callId": id}, "toolCallId": id, "isError": failed, "content": []any{map[string]any{"type": "text", "text": text}}}}
}

func TestToolsCorrelateParallelResultsAndReconnect(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	toolEvent(t, m, 0, "tool/call", toolCall("a", "read", 1))
	toolEvent(t, m, 1, "tool/call", toolCall("b", "bash", 1))
	if !strings.Contains(m.chatView(), "Выполняется") || len(m.toolIDs()) != 2 {
		t.Fatal("running tools were not shown")
	}
	toolEvent(t, m, 2, "tool/result", toolResult("b", "command failed", true))
	toolEvent(t, m, 3, "tool/result", toolResult("a", "file contents", false))
	if len(m.chat.messages) != 2 || m.chat.messages[0].tool.Name != "bash" || m.chat.messages[1].tool.Name != "read" {
		t.Fatal("parallel tool results were not matched by call ID")
	}
	view := m.chatView()
	if strings.Count(view, "read ·") != 1 || !strings.Contains(view, "Ошибка") || !strings.Contains(view, "Готово") {
		t.Fatal("result cards were duplicated or mislabeled")
	}
	records := []chatRecord{}
	for i, v := range []struct {
		kind string
		data any
	}{{"tool/call", toolCall("a", "read", 1)}, {"tool/call", toolCall("b", "bash", 1)}, {"tool/result", toolResult("b", "command failed", true)}, {"tool/result", toolResult("a", "file contents", false)}} {
		raw, _ := json.Marshal(v.data)
		event := chatEvent{Seq: i, Type: v.kind, Data: raw}
		if v.kind == "tool/result" {
			event.SurfaceOp = json.RawMessage(`"append"`)
		}
		records = append(records, chatRecord{Type: "event", Event: event})
	}
	frame := &chatFrame{Type: "snapshot", Cursor: 3, Records: records}
	frame.Header.ID = "s"
	m.chat.expandedTools = map[string]bool{"a": true}
	if err := m.applyChatFrame(m.chat, frame); err != nil {
		t.Fatal(err)
	}
	if len(m.chat.messages) != 2 || !m.chat.expandedTools["a"] {
		t.Fatal("reconnect lost tool state or duplicated results")
	}
	if err := m.applyChatFrame(m.chat, &chatFrame{Type: "event", Event: records[3].Event}); err != nil {
		t.Fatal(err)
	}
	if len(m.chat.messages) != 2 {
		t.Fatal("duplicate result was appended")
	}
}
func TestToolsCancelMissingCallAndSurfaceReplacement(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	toolEvent(t, m, 0, "tool/call", toolCall("a", "read", 1))
	toolEvent(t, m, 1, "turn/end", map[string]any{"turn": 1, "reason": map[string]any{"kind": "aborted"}})
	if m.chat.messages[0].tool.State != "stopped" {
		t.Fatal("cancelled tool retained a running state")
	}
	toolEvent(t, m, 2, "tool/result", toolResult("a", "late result", false))
	if len(m.chat.messages) != 1 || m.chat.messages[0].tool.State != "done" {
		t.Fatal("late authoritative result did not replace cancellation")
	}
	toolEvent(t, m, 3, "tool/result", toolResult("outside-window", "orphan result", false))
	if !strings.Contains(m.chatView(), "вне окна истории") {
		t.Fatal("missing call was shown with an invented name")
	}
	applyChat(t, m, `{"type":"event","event":{"type":"user/message","seq":4,"surfaceOp":{"op":"replace","startSeq":2,"endSeq":3},"data":{"content":[{"type":"text","text":"replacement"}]}}}`)
	if len(m.toolIDs()) != 0 || strings.Contains(m.chatView(), "late result") {
		t.Fatal("surface replacement retained obsolete tool cards")
	}
}
func TestToolsBoundOutputAndEscapeTerminalControls(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	toolEvent(t, m, 0, "tool/call", toolCall("a", "read\u001b[2J", 1))
	toolEvent(t, m, 1, "tool/result", toolResult("a", strings.Repeat("я", chatTextLimit)+"\x1b[2J", false))
	card := m.chat.messages[0].tool
	if len(card.Result) > chatTextLimit+100 || !strings.Contains(card.Result, "обрезан") {
		t.Fatal("oversized tool result was not bounded")
	}
	if strings.Contains(card.Result, "\x1b") {
		t.Fatal("tool result retained terminal controls")
	}
	toolEvent(t, m, 2, "tool/call", toolCall("b", "bash", 2))
	toolEvent(t, m, 3, "turn/end", map[string]any{"turn": 2, "reason": map[string]any{"kind": "error"}})
	if m.chat.messages[1].tool.State != "unknown" {
		t.Fatal("missing result was labeled as successful")
	}
}
func TestToolsKeyboardMouseAndNarrowLayoutPreserveDraft(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	toolEvent(t, m, 0, "tool/call", toolCall("a", "read", 1))
	toolEvent(t, m, 1, "tool/result", toolResult("a", "one\ntwo\nthree\nfour\nfive\nsix", false))
	m.input.SetValue("unsent draft")
	for _, width := range []int{100, 80} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		m.Update(tea.KeyMsg{Type: tea.KeyF6})
		if !m.toolFocused || m.input.Focused() {
			t.Fatal("F6 did not move focus into tool cards")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if !m.chat.expandedTools["a"] || !strings.Contains(m.chatView(), "six") {
			t.Fatal("Enter did not expand the full tool result")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if !m.input.Focused() || m.input.Value() != "unsent draft" || len(m.prompts) != 0 {
			t.Fatal("tool navigation sent or discarded the composer draft")
		}
		m.viewport.GotoTop()
		m.rebuild()
		if len(m.chatToolHits) != 1 {
			t.Fatal("tool header hit area is missing")
		}
		x := m.conversationArea().x
		y := m.conversationArea().y + m.chatToolHits[0].row - m.viewport.YOffset
		rendered := strings.Split(clean(m.View()), "\n")
		if y >= len(rendered) || !strings.Contains(rendered[y], "read ·") {
			t.Fatalf("tool hit area did not align at width %d: row %d", width, y)
		}
		m.mouse(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.chat.expandedTools["a"] {
			t.Fatal("mouse did not collapse the tool card")
		}
	}
}

func TestToolAbortOutcomeAndBoundedExpansion(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	toolEvent(t, m, 0, "tool/call", toolCall("a", "bash", 1))
	m.chat.expandedTools = map[string]bool{"a": true, "stale": true}
	toolEvent(t, m, 1, "tool/result", map[string]any{"error": map[string]any{"code": "ABORTED"}, "message": map[string]any{"toolCallId": "a", "isError": true}})
	if m.chat.messages[0].tool.State != "stopped" || len(m.chat.expandedTools) != 1 {
		t.Fatal("abort outcome or expansion pruning was incorrect")
	}
	deep := strings.Repeat("[", 1000) + "0" + strings.Repeat("]", 1000)
	m.chat.messages[0].tool.Arguments = deep
	if len(m.toolView(m.chat.messages[0].tool)) > len(deep)+1000 {
		t.Fatal("nested arguments amplified output size")
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.focusTool("a")
	paletteTestSelect(t, m, "Настройки dsh")
	if m.toolFocused || m.page != dshSettingsPage {
		t.Fatal("page navigation retained tool keyboard focus")
	}
}

func TestToolCollapsedSummariesUseCommandsAndIdentifyingResourceNames(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	for _, item := range []struct{ name, args, want string }{
		{"bash", `{"command":"go test ./...","description":"Run tests","cwd":"/tmp"}`, "go test ./..."},
		{"read", `{"file_path":"/tmp/notes.txt"}`, "notes.txt"},
		{"search", `{"path":"src","pattern":"interface"}`, "src · interface"},
		{"web", `{"url":"https://example.test/docs"}`, "https://example.test/docs"},
		{"exec", `{"argv":["git","status","--short"]}`, "git status --short"},
	} {
		card := &chatTool{ID: "summary", Name: item.name, Arguments: item.args, State: "done"}
		view := clean(m.toolViewWidth(card, 100))
		if !strings.Contains(view, item.want) || strings.Contains(view, `{"`) || strings.Count(view, "\n") > 1 {
			t.Fatalf("tool summary is noisy or hides its operation:%s", view)
		}
	}
	path := "/tmp/" + strings.Repeat("long-directory/", 20) + "important-file.go"
	raw, _ := json.Marshal(map[string]string{"file_path": path})
	view := clean(m.toolViewWidth(&chatTool{ID: "long", Name: "read", State: "done", Arguments: string(raw)}, 76))
	if !strings.Contains(view, "important-file.go") {
		t.Fatal("narrow resource summary hides identifying filename")
	}
}

func TestToolCollapsedStatesNestedRelationAndResultsStayWithinTwoRows(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, english := range []bool{false, true} {
		m := chatModel(t)
		applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
		activeEnglish.Store(english)
		m.chat.messages = []chatMessage{{tool: &chatTool{ID: "parent", Name: "run_code"}}}
		for _, state := range []string{"running", "done", "error", "stopped", "unknown", "future-outcome"} {
			card := &chatTool{ID: "child", Name: "read", ParentID: "parent", RootID: "parent", Nested: true, State: state, Arguments: `{"path":"notes.txt"}`, Result: "first useful line\nsecond\nthird\nfourth"}
			if state == "error" {
				card.ErrorCode = "E_TEST"
			}
			view := m.toolViewWidth(card, 76)
			if strings.Count(view, "\n") > 1 || lipgloss.Width(view) > 76 {
				t.Fatalf("collapsed tool state%q expanded vertically", state)
			}
			if !strings.Contains(clean(view), tr("Внутри: ")+"run_code") {
				t.Fatal("compact nested tool lost parent relationship")
			}
			if state == "error" && !strings.Contains(clean(view), "E_TEST") {
				t.Fatal("collapsed failure hides its error code")
			}
			if state == "future-outcome" && !strings.Contains(clean(view), tr("? Результат не получен")) {
				t.Fatal("unknown outcome is labeled successful or blank")
			}
		}
	}
}

func TestToolExpansionRetainsFullArgumentsResultsAndRawState(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	args := `{"file_path":"notes.txt","offset":5,"limit":200,"unknown_field":"retained"}`
	result := "first\n\nsecond\nthird\nlast-result-line"
	card := &chatTool{ID: "a", Name: "read", State: "done", Arguments: args, Result: result}
	collapsed := clean(m.toolViewWidth(card, 76))
	if strings.Contains(collapsed, "last-result-line") || strings.Contains(collapsed, "unknown_field") {
		t.Fatal("collapsed tool leaked full details")
	}
	m.chat.expandedTools = map[string]bool{"a": true}
	expanded := clean(m.toolViewWidth(card, 76))
	if !strings.Contains(expanded, result) || !strings.Contains(expanded, `"unknown_field": "retained"`) || !strings.Contains(expanded, tr("Аргументы:")) {
		t.Fatal("expanded tool lost full result or unknown argument fields")
	}
	if card.Arguments != args || card.Result != result {
		t.Fatal("rendering rewrote stored tool source")
	}
}

func TestCompactToolLiveLossAndDisconnectRemainVisible(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	card := &chatTool{ID: "a", Name: "bash", State: "running", Arguments: `{"command":"build"}`}
	m.chat.jobs = map[string]toolJob{"a": {ID: "j"}}
	m.chat.observers = map[string]*toolOutputObserver{"j": {value: toolOutput{Text: "old\nnewest-output", Stale: true, Gap: true}}}
	view := clean(m.toolViewWidth(card, 100))
	if strings.Count(view, "\n") > 1 || !strings.Contains(view, tr(" · нет связи")) || !strings.Contains(view, tr(" · вывод неполный")) || !strings.Contains(view, "newest-output") {
		t.Fatalf("compact live tool hid uncertainty or most recent output:%s", view)
	}
	m.chat.expandedTools = map[string]bool{"a": true}
	expanded := clean(m.toolViewWidth(card, 100))
	if !strings.Contains(expanded, "old\nnewest-output") || !strings.Contains(expanded, tr("\n[Часть вывода недоступна или обрезана]")) {
		t.Fatal("expanded live tool lost raw output or loss notice")
	}
}

func TestCompactToolPrioritizesLossFlagsAtMinimumWidth(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	m.chat.ready = false
	card := &chatTool{ID: "a", Name: "tool-with-a-long-name", State: "running", Arguments: `{"command":"important-command"}`}
	m.chat.jobs = map[string]toolJob{"a": {ID: "j"}}
	m.chat.observers = map[string]*toolOutputObserver{"j": {value: toolOutput{Text: "last output", Stale: true, Gap: true}}}
	for _, width := range []int{75, 126} {
		view := clean(m.toolViewWidth(card, width))
		if strings.Count(view, "\n") > 1 || !strings.Contains(view, strings.TrimPrefix(tr(" · нет связи"), " · ")) || !strings.Contains(view, strings.TrimPrefix(tr(" · вывод неполный"), " · ")) {
			t.Fatalf("loss flags hidden at%dcolumns:%s", width, view)
		}
	}
}

func TestExpandedToolHeaderRemainsAnchoredWhenResized(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	toolEvent(t, m, 0, "tool/call", toolCall("read", "read", 1))
	toolEvent(t, m, 1, "tool/result", toolResult("read", strings.Repeat("result row\n", 20), false))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 42})
	m.input.SetValue("resize tool draft 世界")
	m.focusTool("read")
	m.toggleTool("read")
	if !m.viewport.AtBottom() {
		t.Fatal("fixture must reproduce initial tail-follow state")
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 160, Height: 40}, {Width: 100, Height: 30}} {
		m.Update(size)
		if len(m.chatToolHits) != 1 {
			t.Fatal("resizing lost tool hit")
		}
		hit := m.chatToolHits[0]
		y := m.conversationArea().y + hit.row - m.viewport.YOffset
		lines := strings.Split(clean(m.View()), "\n")
		if y < bodyTop || y >= m.composerBox().y || !strings.Contains(lines[y], "read · ✓ Готово") {
			t.Fatal("resizing scrolled the selected expanded tool header out of view")
		}
		if !m.chat.expandedTools["read"] || m.input.Value() != "resize tool draft 世界" || !m.toolFocused {
			t.Fatal("resizing lost expansion, focus or draft")
		}
	}
}
