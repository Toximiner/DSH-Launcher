package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func historyToolsFixture(t *testing.T, offset, cut int) historyPage {
	t.Helper()
	page := historyPage{Cursor: cut, HasMore: offset > 0}
	add := func(kind string, data any) {
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		event := chatEvent{Seq: offset + len(page.Records), Type: kind, Data: raw}
		if kind == "user/message" || kind == "assistant/message" || kind == "tool/result" {
			event.SurfaceOp = json.RawMessage(`"append"`)
		}
		page.Records = append(page.Records, chatRecord{Type: "event", Event: event})
	}
	add("user/message", map[string]any{"content": []any{map[string]any{"type": "text", "text": "selected source"}}, "source": map[string]any{"kind": "user"}})
	add("tool/call", toolCall("root", "run_code", 1))
	add("tool/ptc-dispatch-start", nestedEvent("child", "root", "root", "read", false))
	add("tool/ptc-dispatch", nestedEvent("child", "root", "root", "read", false))
	add("tool/result", toolResult("root", "one\ntwo\nthree\nfour\nfive\nTAIL_ONLY", false))
	add("tool/call", toolCall("failed", "bash", 1))
	add("tool/result", toolResult("failed", "failed command", true))
	add("assistant/message", map[string]any{"message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "**committed answer**"}}}})
	return page
}

func TestHistoryToolControlsAndNestedRowsKeepLiveStateIsolated(t *testing.T) {
	for _, size := range [][2]int{{120, 42}, {100, 30}, {80, 24}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := chatModel(t)
			h := historyTestReader(t, m)
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m.input.SetValue("retained draft")
			m.chat.expandedTools = map[string]bool{"live": true}
			m.chat.selectedTool = "live"
			liveOffset, liveCursor := m.viewport.YOffset, m.chat.cursor
			m.handleHistoryResult(historyResult{reader: h, cut: 7, before: 8, page: historyToolsFixture(t, 0, 7)})
			if len(h.toolRows) != 3 || h.toolRows[0].id != "root" || h.toolRows[1].id != "child" || len(h.messageRows) != 2 {
				t.Fatal("history lost nested order or mixed tools into message boundaries")
			}
			if strings.Contains(h.viewport.View(), "TAIL_ONLY") {
				t.Fatal("history tools start expanded")
			}
			m.selectHistoryMessage(0, false)
			m.selectHistoryTool("root", true)
			m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if !h.projection.expandedTools["root"] || h.selectedSeq != -1 {
				t.Fatal("tool details did not expand or retained a message target")
			}
			if !strings.Contains(h.viewport.View(), "Аргументы:") {
				t.Fatal("expanded tool arguments missing")
			}
			h.viewport.GotoBottom()
			m.Update(tea.KeyMsg{Type: tea.KeyF6})
			if h.projection.selectedTool != "child" || !strings.Contains(h.viewport.View(), "Внутри: run_code") {
				t.Fatal("F6 lost nested card order")
			}
			m.Update(tea.KeyMsg{Type: tea.KeyF18})
			if h.projection.selectedTool != "root" {
				t.Fatal("Shift+F6 did not select previous history tool")
			}
			m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
			if m.forkDialog != nil || m.copyHistoryMessage() != nil || m.openHistoryCode() != nil {
				t.Fatal("tool focus reused a stale message for fork or copying")
			}
			box := m.historyBox()
			row := h.toolRows[0].row - h.viewport.YOffset
			m.historyMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 4 + row, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			if h.projection.expandedTools["root"] {
				t.Fatal("mouse did not collapse the selected card")
			}
			m.Update(tea.KeyMsg{Type: tea.KeyTab})
			if h.toolFocused || h.selectedSeq < 0 {
				t.Fatal("Tab did not return to message selection")
			}
			m.forkHistorySelection()
			if m.forkDialog == nil || *m.forkDialog.atSeq != h.selectedSeq {
				t.Fatal("folding changed a message fork boundary")
			}
			m.forkDialog = nil
			if m.input.Value() != "retained draft" || m.chat.cursor != liveCursor || m.viewport.YOffset != liveOffset || !m.chat.expandedTools["live"] || len(m.chat.expandedTools) != 1 || m.chat.selectedTool != "live" || m.toolFocused {
				t.Fatal("history changed the composer or live tools")
			}
			if lipgloss.Width(m.View()) > m.width-1 || lipgloss.Height(m.View()) > m.height {
				t.Fatal("history tool view overflowed")
			}
		})
	}
}

func TestHistoryToolExpansionFollowsPinnedPagesAndResetsOnRefresh(t *testing.T) {
	m := chatModel(t)
	h := historyTestReader(t, m)
	latest := historyToolsFixture(t, 8, 15)
	m.handleHistoryResult(historyResult{reader: h, cut: 15, before: 16, page: latest})
	m.selectHistoryTool("root", true)
	m.toggleHistoryTool("root")
	m.handleHistoryResult(historyResult{reader: h, direction: "older", cut: 15, before: 8, page: historyToolsFixture(t, 0, 15)})
	if h.projection.expandedTools["root"] {
		t.Fatal("same call ID from a different event inherited expansion")
	}
	m.handleHistoryResult(historyResult{reader: h, direction: "newer", cut: 15, before: 16, page: latest})
	if !h.projection.expandedTools["root"] || h.toolFocused {
		t.Fatal("returning to pinned page lost expansion or kept stale focus")
	}
	m.handleHistoryResult(historyResult{reader: h, direction: "latest", cut: 15, before: 16, page: latest})
	if h.projection.expandedTools["root"] || len(h.expandedTools) != 0 {
		t.Fatal("fresh snapshot retained old expansion preferences")
	}
	for i := 0; i < historyExpandedLimit+30; i++ {
		h.rememberHistoryTool(historyToolKey{fmt.Sprint(i), i}, true)
	}
	if len(h.expandedTools) != historyExpandedLimit || len(h.expandedOrder) != historyExpandedLimit {
		t.Fatal("history expansion memory grew past limit")
	}
	key := h.expandedOrder[10]
	h.rememberHistoryTool(key, true)
	h.rememberHistoryTool(key, false)
	if len(h.expandedOrder) != historyExpandedLimit-1 || h.expandedTools[key] {
		t.Fatal("retoggle duplicated expansion history")
	}
}

func TestHistoryToolControlsIgnoreHiddenStaleAndPastedActions(t *testing.T) {
	m := chatModel(t)
	h := historyTestReader(t, m)
	m.handleHistoryResult(historyResult{reader: h, cut: 7, before: 8, page: historyToolsFixture(t, 0, 7)})
	m.selectHistoryTool("root", true)
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter, Paste: true}, {Type: tea.KeyF6, Paste: true}, {Type: tea.KeyRunes, Runes: []rune("enter")}, {Type: tea.KeyRunes, Runes: []rune("f6")}} {
		m.Update(key)
	}
	if h.projection.expandedTools["root"] || h.projection.selectedTool != "root" {
		t.Fatal("pasted/literal key name triggered a tool action")
	}
	m.editHistorySearch()
	m.Update(tea.KeyMsg{Type: tea.KeyF6})
	if h.projection.selectedTool != "root" {
		t.Fatal("search editor selected a tool")
	}
	h.search.editing = false
	h.busy = true
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyF6})
	h.busy = false
	m.chat.epoch++
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyF6})
	m.chat.epoch--
	m.Update(tea.WindowSizeMsg{Width: 79, Height: 24})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if h.projection.expandedTools["root"] || h.projection.selectedTool != "root" {
		t.Fatal("busy/stale/hidden card accepted an action")
	}
}

func TestLiveToolsDecodeShiftF6AsPreviousCard(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	toolEvent(t, m, 0, "tool/call", toolCall("a", "read", 1))
	toolEvent(t, m, 1, "tool/call", toolCall("b", "bash", 1))
	m.Update(tea.KeyMsg{Type: tea.KeyF6})
	m.Update(tea.KeyMsg{Type: tea.KeyF6})
	m.Update(tea.KeyMsg{Type: tea.KeyF18})
	if m.chat.selectedTool != "a" {
		t.Fatal("Shift+F6 did not select previous live card")
	}
}
