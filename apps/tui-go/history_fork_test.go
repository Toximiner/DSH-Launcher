package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestHistoryForkSelectionUsesEventIdentityAndPreservesReader(t *testing.T) {
	for _, size := range [][2]int{{120, 42}, {100, 30}, {80, 24}} {
		m := chatModel(t)
		h := historyTestReader(t, m)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.handleHistoryResult(historyResult{reader: h, cut: 59, before: 60, page: historyFixture(40, 59, 59)})
		m.input.SetValue("preserved composer")
		m.historyKey(tea.KeyMsg{Type: tea.KeyHome})
		m.historyKey(tea.KeyMsg{Type: tea.KeyTab})
		if h.selectedSeq != 40 {
			t.Fatal("first visible message was not selected")
		}
		m.historyKey(tea.KeyMsg{Type: tea.KeyTab})
		if h.selectedSeq != 41 {
			t.Fatal("Tab did not move selection")
		}
		m.historyKey(tea.KeyMsg{Type: tea.KeyShiftTab})
		if h.selectedSeq != 40 {
			t.Fatal("Shift+Tab did not move backward")
		}
		box := m.historyBox()
		h.viewport.GotoTop()
		row := h.messageRows[1]
		m.historyMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 4 + row.start, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if h.selectedSeq != 41 {
			t.Fatal("click selected the wrong event")
		}
		m.historyKey(tea.KeyMsg{Type: tea.KeyCtrlB})
		d := m.forkDialog
		if d == nil || d.atSeq == nil || *d.atSeq != 41 || d.id != h.sessionID {
			t.Fatal("fork used an index or latest cursor instead of selected seq")
		}
		if !strings.Contains(m.View(), "#41") || lipgloss.Height(m.View()) > m.height || lipgloss.Width(m.View()) > m.width-1 {
			t.Fatal("exact-cut confirmation overflowed")
		}
		m.forkKey(tea.KeyMsg{Type: tea.KeyEsc})
		if m.historyReader != h || h.selectedSeq != 41 || m.input.Focused() || m.input.Value() != "preserved composer" {
			t.Fatal("cancel lost reader, selection, focus, or draft")
		}
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		if h.selectedSeq != 41 {
			t.Fatal("resize changed selected event")
		}
		m.handleHistoryResult(historyResult{reader: h, direction: "older", cut: 59, before: 40, page: historyFixture(20, 39, 59)})
		if h.selectedSeq != -1 {
			t.Fatal("page change retained an unavailable boundary")
		}
	}
}

func TestHistoryForkRejectsBusyStaleAndNonConversationSelection(t *testing.T) {
	m := chatModel(t)
	h := historyTestReader(t, m)
	m.handleHistoryResult(historyResult{reader: h, cut: 2, before: 3, page: historyFixture(0, 2, 2)})
	h.selectedSeq = 0
	h.busy = true
	m.forkHistorySelection()
	if m.forkDialog != nil {
		t.Fatal("busy history was forked")
	}
	h.busy = false
	m.chat.epoch++
	m.forkHistorySelection()
	if m.forkDialog != nil {
		t.Fatal("stale reader was forked")
	}
	m.chat.epoch--
	for _, message := range []chatMessage{
		{seq: 0, role: "АГЕНТ · НЕЗАВЕРШЁННЫЙ ОТВЕТ", text: "preview"},
		{seq: 0, role: "СООБЩЕНИЕ DSH", text: "service context"},
		{seq: 0, role: "АГЕНТ", text: "tool", tool: &chatTool{}},
		{seq: 0, role: "РАЗРЕШЕНИЕ", text: "question", approvalID: "approval"},
	} {
		h.projection.messages = []chatMessage{message}
		m.forkHistorySelection()
		if m.forkDialog != nil {
			t.Fatal("non-conversation row was forked")
		}
	}
	h.projection.messages = []chatMessage{{seq: 0, role: "ВЫ", text: "zero boundary"}}
	m.forkHistorySelection()
	if m.forkDialog == nil || *m.forkDialog.atSeq != 0 {
		t.Fatal("valid event zero was rejected")
	}
}

func TestHistoryForkTransportPinsInclusiveCutAndClosesReaderAfterSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "test", Path: "/"})
			w.WriteHeader(302)
			return
		}
		var request struct {
			ID      string `json:"rpcId"`
			Method  string `json:"method"`
			Payload struct {
				Args struct {
					Request map[string]any `json:"request"`
				} `json:"args"`
			} `json:"payload"`
		}
		json.NewDecoder(r.Body).Decode(&request)
		args := request.Payload.Args.Request
		if request.Method != "session/fork" || args["atSeq"] != float64(11) || args["allowMigration"] != false || args["sessionId"] != "s" {
			t.Error("wrong exact-cut request", request)
		}
		json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": request.ID, "result": map[string]any{"ok": true, "value": map[string]any{"sessionId": "exact-child"}}})
	}))
	defer server.Close()
	m := chatModel(t)
	h := historyTestReader(t, m)
	m.settingsLogin = server.URL + "/?token=test"
	m.handleHistoryResult(historyResult{reader: h, cut: 19, before: 20, page: historyFixture(10, 19, 19)})
	h.selectedSeq = 11
	m.forkHistorySelection()
	// Later live events and UI selection must not change an already reviewed boundary.
	h.selectedSeq = 12
	m.chat.cursor = 99
	cmd, _ := m.forkKey(tea.KeyMsg{Type: tea.KeyEnter})
	result := cmd().(sessionForkResult)
	if result.err != nil {
		t.Fatal(result.err)
	}
	m.sessionBusy = true
	m.sessionAction = "refresh"
	m.handleSessionFork(result)
	if m.historyReader != nil || h.ctx.Err() == nil || len(m.sessionQueue) != 1 || m.sessionQueue[0].value != "exact-child" {
		t.Fatal("success did not close reader and connect to acknowledged child")
	}
}
