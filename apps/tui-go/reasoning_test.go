package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func reasoningEvent(seq int, text, thought string) chatEvent {
	data, _ := json.Marshal(map[string]any{"turn": 1, "step": 1, "message": map[string]any{"content": []map[string]string{{"type": "reasoning", "text": thought}, {"type": "text", "text": text}, {"type": "tool-call", "text": "PROTOCOL_ONLY"}}}})
	return chatEvent{Seq: seq, Type: "assistant/message", Data: data}
}
func reasoningFixture(t *testing.T) *model {
	t.Helper()
	m := chatModel(t)
	frame := chatFrame{Type: "snapshot", Cursor: 1, Records: []chatRecord{{Type: "event", Event: reasoningEvent(1, "# Authoritative reply", "ACTUAL_THOUGHT 世界")}}}
	frame.Header.ID = "s"
	if err := m.applyChatFrame(m.chat, &frame); err != nil {
		t.Fatal(err)
	}
	m.rebuild()
	return m
}

func TestReasoningDurableProjectionKeepsPublicBlocksSeparate(t *testing.T) {
	m := reasoningFixture(t)
	message := m.chat.messages[0]
	if message.text != "# Authoritative reply" || message.reasoning != "ACTUAL_THOUGHT 世界" {
		t.Fatal("reasoning was discarded or mixed into reply")
	}
	if !m.hasReasoning() {
		t.Fatal("public reasoning not exposed")
	}
	plain := ansi.Strip(m.View())
	if strings.Contains(plain, "ACTUAL_THOUGHT") || strings.Contains(plain, "PROTOCOL_ONLY") || !strings.Contains(plain, tr("Размышления")) {
		t.Fatal("collapsed projection leaked hidden reasoning or protocol")
	}
	m.input.SetValue("/thoughts")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(ansi.Strip(m.View()), "ACTUAL_THOUGHT 世界") {
		t.Fatal("registered thoughts action did not reveal actual source")
	}
	if message.text != m.chat.messages[0].text || m.messageText(message, 50, true) != "# Authoritative reply" {
		t.Fatal("thoughts changed authoritative reply source")
	}
	var copied string
	m.copyClipboard = func(_ context.Context, source string) (string, error) { copied = source; return "copied", nil }
	cmd := m.copyConversation(message.seq)
	if cmd == nil {
		t.Fatal("reply copy unavailable")
	}
	cmd()
	if copied != "# Authoritative reply" {
		t.Fatal("reply copy included reasoning/protocol")
	}
}

func TestReasoningMouseUsesRenderedHeaderWithTranscriptPrefix(t *testing.T) {
	for _, width := range []int{80, 100, 160} {
		m := reasoningFixture(t)
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		// The archived-state prefix shifts all transcript interaction rows.
		m.session.PendingInput = []string{"question"}
		m.input.SetValue("preserved draft")
		m.rebuild()
		m.viewport.GotoTop()
		view := ansi.Strip(m.View())
		row := -1
		for y, line := range strings.Split(view, "\n") {
			if strings.Contains(line, tr("Размышления")) {
				row = y
				break
			}
		}
		if row < 0 {
			t.Fatal("visible reasoning header missing")
		}
		box := m.conversationArea()
		m.mouse(tea.MouseMsg{X: box.x + 1, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if !m.chat.expandedReasoning[reasoningEventKey(1)] || !strings.Contains(ansi.Strip(m.View()), "ACTUAL_THOUGHT") {
			t.Fatal("reasoning click ignored displayed transcript offset")
		}
		if m.input.Value() != "preserved draft" {
			t.Fatal("reasoning click lost draft")
		}
	}
}

func TestReasoningStreamRestoreCommitAndRawLimitAreAtomic(t *testing.T) {
	m := chatModel(t)
	frame := decodeChat(t, `{"type":"snapshot","header":{"id":"s"},"cursor":-1,"assistantStream":{"activeAttempt":{"attemptId":"a","turn":1,"step":1,"nextIndex":2,"stream":[{"type":"reasoning-chunks","index":0,"texts":["ACTUAL_","THOUGHT"]},{"type":"text-chunks","index":1,"texts":["ANSWER"]}]}}}`)
	frame.Header.ID = "s"
	if err := m.applyChatFrame(m.chat, frame); err != nil {
		t.Fatal(err)
	}
	if m.chat.live.reasoning() != "ACTUAL_THOUGHT" || m.chat.live.text() != "ANSWER" {
		t.Fatal("compacted reasoning stream did not restore separately")
	}
	m.toggleReasoning()
	if !m.chat.expandedReasoning[reasoningAttemptKey("a")] {
		t.Fatal("live reasoning did not expand")
	}
	before := m.chat.live
	oversized := chatFrame{Type: "assistant-stream", Frame: assistantChatFrame{Type: "chunk", ID: "a", Index: 2, Chunk: chatChunk{Type: "reasoning-delta", Index: 0, Text: strings.Repeat("x", chatTextLimit)}}}
	// Match the wire frame envelope used by the existing chat stream adapter.
	oversized.Type = "assistant-stream"
	if err := m.applyChatFrame(m.chat, &oversized); err == nil {
		t.Fatal("oversized reasoning stream was accepted")
	}
	if m.chat.live != before || before.Next != 2 || before.reasoning() != "ACTUAL_THOUGHT" {
		t.Fatal("rejected reasoning chunk mutated accepted state")
	}
	event := reasoningEvent(0, "ANSWER", "ACTUAL_THOUGHT")
	if err := m.applyChatFrame(m.chat, &chatFrame{Type: "event", Event: event}); err != nil {
		t.Fatal(err)
	}
	if m.chat.live != nil || !m.chat.expandedReasoning[reasoningEventKey(0)] {
		t.Fatal("durable reasoning lost live expansion on commit")
	}
	if m.chat.expandedReasoning[reasoningAttemptKey("a")] {
		t.Fatal("completed attempt leaked expansion state")
	}
}

func TestReasoningSnapshotLimitsAndRetainedBudget(t *testing.T) {
	m := reasoningFixture(t)
	original := m.chat.messages[0]
	frame := chatFrame{Type: "snapshot", Cursor: 2, Records: []chatRecord{{Type: "event", Event: reasoningEvent(2, "reply", strings.Repeat("x", chatTextLimit))}}}
	frame.Header.ID = "s"
	if err := m.applyChatFrame(m.chat, &frame); err == nil {
		t.Fatal("combined reply/reasoning limit was bypassed")
	}
	if len(m.chat.messages) != 1 || m.chat.messages[0].text != original.text || m.chat.messages[0].reasoning != original.reasoning {
		t.Fatal("invalid snapshot replaced accepted projection")
	}
	for i := 2; i < 9; i++ {
		if err := m.applyChatFrame(m.chat, &chatFrame{Type: "event", Event: reasoningEvent(i, "reply", strings.Repeat("x", chatTextLimit*3/4))}); err != nil {
			t.Fatal(err)
		}
	}
	bytes := 0
	for _, message := range m.chat.messages {
		bytes += message.byteSize()
	}
	if bytes > 4*chatTextLimit || !m.chat.hasMore {
		t.Fatal("retained reasoning bypassed transcript memory cap")
	}
}

func TestReasoningOnlyContentAndUnsupportedMetadataAreTruthful(t *testing.T) {
	m := chatModel(t)
	frame := &chatFrame{Type: "snapshot", Cursor: 0, Records: []chatRecord{{Type: "event", Event: reasoningEvent(0, "", "only actual thoughts")}}}
	frame.Header.ID = "s"
	if err := m.applyChatFrame(m.chat, frame); err != nil {
		t.Fatal(err)
	}
	if len(m.chat.messages) != 1 || m.chat.messages[0].text != "" || !m.hasReasoning() {
		t.Fatal("reasoning-only record was discarded or given an invented answer")
	}
	m.chat.messages[0].reasoning = " \n\t"
	if m.hasReasoning() || renderReasoning(m.chat, "empty", " \n", 50, 0) != "" || toggleReasoningState(m.chat) {
		t.Fatal("empty reasoning fabricated a thought section")
	}
	m.chat.expandedReasoning = map[string]bool{"stale": true}
	pruneReasoningExpansion(m.chat)
	if len(m.chat.expandedReasoning) != 0 {
		t.Fatal("stale reasoning UI state leaked")
	}
}

func TestReasoningHistoryToggleAndHeaderMouseStayInHistoricalProjection(t *testing.T) {
	m := reasoningFixture(t)
	m.input.SetValue("history draft")
	h := historyTestReader(t, m)
	page := historyFixture(0, 0, 0)
	page.Records[0].Event = reasoningEvent(0, "HISTORICAL_REPLY", "HISTORICAL_THOUGHT")
	m.handleHistoryResult(historyResult{reader: h, cut: 0, before: 1, page: page})
	if strings.Contains(ansi.Strip(h.viewport.View()), "HISTORICAL_THOUGHT") {
		t.Fatal("history reasoning started expanded")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	if !strings.Contains(ansi.Strip(h.viewport.View()), "HISTORICAL_THOUGHT") {
		t.Fatal("history keyboard action did not reveal thought source")
	}
	if len(m.chat.expandedReasoning) != 0 || m.input.Value() != "history draft" {
		t.Fatal("history action mutated live projection or draft")
	}
	h.selectedSeq = 0
	m.resizeHistory()
	h.viewport.GotoTop()
	row := -1
	for y, line := range strings.Split(ansi.Strip(h.viewport.View()), "\n") {
		if strings.Contains(line, tr("Размышления")) {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatal("history reasoning header missing")
	}
	box := m.historyBox()
	m.historyMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 4 + row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if strings.Contains(ansi.Strip(h.viewport.View()), "HISTORICAL_THOUGHT") {
		t.Fatal("selected-message prefix shifted the historical header click")
	}
	if h.projection.messages[0].text != "HISTORICAL_REPLY" || m.chat.messages[0].text != "# Authoritative reply" {
		t.Fatal("history reasoning changed copy sources")
	}
}
