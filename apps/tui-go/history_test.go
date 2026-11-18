package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func historyFixture(first, last, cut int) historyPage {
	p := historyPage{Cursor: cut, HasMore: first > 0}
	for seq := first; seq <= last; seq++ {
		data, _ := json.Marshal(map[string]any{"content": []any{map[string]any{"type": "text", "text": fmt.Sprintf("History message %03d", seq)}}, "source": map[string]any{"kind": "user", "rpcId": "receipt"}})
		p.Records = append(p.Records, chatRecord{Type: "event", Event: chatEvent{Type: "user/message", Seq: seq, Data: data, SurfaceOp: json.RawMessage(`"append"`)}})
	}
	return p
}

func historyTestReader(t *testing.T, m *model) *historyReader {
	t.Helper()
	ctx, cancel := context.WithCancel(m.chat.ctx)
	t.Cleanup(cancel)
	h := &historyReader{ctx: ctx, cancel: cancel, sessionID: m.chat.id, epoch: m.chat.epoch, cut: 59, before: 60, first: -1}
	m.historyReader = h
	m.resizeHistory()
	return h
}

func TestHistoryPagingPinsCutAndPreservesPagePosition(t *testing.T) {
	m := chatModel(t)
	h := historyTestReader(t, m)
	m.handleHistoryResult(historyResult{reader: h, serial: 0, direction: "latest", cut: 59, before: 60, page: historyFixture(40, 59, 59)})
	h.viewport.SetYOffset(5)
	m.handleHistoryResult(historyResult{reader: h, serial: 0, direction: "older", cut: 59, before: 40, page: historyFixture(20, 39, 59)})
	if h.number != 2 || h.cut != 59 || h.before != 40 || len(h.back) != 1 || h.back[0].offset != 5 {
		t.Fatal("older page lost its pinned cut or return location")
	}
	m.handleHistoryResult(historyResult{reader: h, serial: 0, direction: "newer", cut: 59, before: 60, page: historyFixture(40, 59, 59)})
	if h.number != 1 || h.viewport.YOffset != 5 || len(h.back) != 0 {
		t.Fatal("returning to a page lost its scroll position")
	}
	if !strings.Contains(h.viewport.View(), "History message") {
		t.Fatal("history content was not rendered")
	}
}

func TestHistoryReplayDoesNotTouchLiveState(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":0,"records":[{"type":"event","event":{"seq":0,"type":"user/message","data":{"content":[{"type":"text","text":"LIVE"}],"source":{"kind":"user"}}}}]}`)
	m.session.Running = true
	m.prompts["s"] = &pendingPrompt{ID: "receipt", Text: "unconfirmed", Status: "unknown"}
	m.input.SetValue("unsent draft")
	h := historyTestReader(t, m)
	page := historyFixture(0, 2, 2)
	page.Records[2].Event.Type = "turn/end"
	page.Records[2].Event.Data = json.RawMessage(`{"turn":1,"reason":{"kind":"aborted"}}`)
	m.handleHistoryResult(historyResult{reader: h, serial: 0, direction: "latest", cut: 2, before: 3, page: page})
	if !m.session.Running || m.prompts["s"] == nil || m.input.Value() != "unsent draft" || m.chat.cursor != 0 || m.chat.messages[0].text != "LIVE" {
		t.Fatal("historical replay changed live agent state, receipt, or composer")
	}
	if h.projection == nil || h.projection == m.chat {
		t.Fatal("history was not isolated")
	}
}

func TestHistoryRejectsGapsWrongCutsAndUnsupportedReplacement(t *testing.T) {
	good := historyFixture(10, 19, 19)
	for _, mutate := range []func(*historyPage){
		func(p *historyPage) { p.Cursor = 20 },
		func(p *historyPage) { p.Records = append(p.Records[:2], p.Records[3:]...) },
		func(p *historyPage) { p.HasMore = false },
		func(p *historyPage) { p.Records = p.Records[:len(p.Records)-1] },
		func(p *historyPage) {
			p.Records[0].Event.SurfaceOp = json.RawMessage(`{"op":"replace","startSeq":0,"endSeq":2}`)
		},
	} {
		p := historyFixture(10, 19, 19)
		mutate(&p)
		if _, _, err := projectHistory(p, "s", 19, 20); err == nil {
			t.Fatal("invalid history page was accepted")
		}
	}
	if _, _, err := projectHistory(good, "s", 19, 20); err != nil {
		t.Fatal(err)
	}
	if _, _, err := projectHistory(historyPage{Cursor: -1}, "s", -1, 0); err != nil {
		t.Fatal("empty session history was rejected", err)
	}
}

func TestHistoryFailedAndLateReadsPreserveVisiblePage(t *testing.T) {
	m := chatModel(t)
	h := historyTestReader(t, m)
	m.handleHistoryResult(historyResult{reader: h, serial: 0, direction: "latest", cut: 59, before: 60, page: historyFixture(40, 59, 59)})
	previous := h.projection
	m.handleHistoryResult(historyResult{reader: h, serial: 0, direction: "older", cut: 59, before: 40, page: historyFixture(20, 38, 59)})
	if h.projection != previous || len(h.back) != 0 || h.number != 1 {
		t.Fatal("failed page replaced history or advanced navigation")
	}
	h.serial++
	m.handleHistoryResult(historyResult{reader: h, serial: 0, direction: "older", cut: 59, before: 40, page: historyFixture(20, 39, 59)})
	if h.projection != previous {
		t.Fatal("old request changed the visible page")
	}
	m.closeHistory()
	m.handleHistoryResult(historyResult{reader: h, serial: 1, direction: "older", cut: 59, before: 40, page: historyFixture(20, 39, 59)})
	if m.historyReader != nil || h.ctx.Err() == nil {
		t.Fatal("closed history was reopened by a late response")
	}
}

func TestHistoryKeyboardMouseAndMinimumSizePreserveDraft(t *testing.T) {
	for _, size := range [][2]int{{120, 42}, {100, 30}, {80, 24}} {
		m := chatModel(t)
		h := historyTestReader(t, m)
		m.input.SetValue("draft stays here")
		m.viewport.SetContent(strings.Repeat("live line\n", 60))
		m.viewport.SetYOffset(7)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		liveOffset := m.viewport.YOffset
		m.handleHistoryResult(historyResult{reader: h, serial: 0, direction: "latest", cut: 59, before: 60, page: historyFixture(40, 59, 59)})
		view := m.View()
		if lipgloss.Width(view) > m.width-1 || lipgloss.Height(view) > m.height {
			t.Fatalf("history overflowed at %v", size)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("do not send"), Paste: true})
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.input.Value() != "draft stays here" || m.viewport.YOffset != liveOffset {
			t.Fatal("history consumed draft or scrolled the live chat")
		}
		m.historyKey(tea.KeyMsg{Type: tea.KeyHome})
		box := m.historyBox()
		m.historyMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		if h.viewport.YOffset == 0 {
			t.Fatal("wheel did not scroll history")
		}
		h.focused = true
		m.historyKey(tea.KeyMsg{Type: tea.KeyEsc})
		if m.historyReader != nil || !m.input.Focused() || m.input.Value() != "draft stays here" {
			t.Fatal("closing history did not restore composer")
		}
	}
}

func TestHistoryTransportUsesReadOnlyPinnedPageRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "test", Path: "/"})
			w.WriteHeader(http.StatusFound)
			return
		}
		if r.URL.Path != "/api/session/events" {
			t.Errorf("history invoked an unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		var rpc struct {
			ID      string `json:"rpcId"`
			Payload struct {
				Args struct {
					Request struct {
						Address struct {
							ID string `json:"sessionId"`
						} `json:"address"`
						Cut    int `json:"throughSeq"`
						Before int `json:"beforeSeq"`
						Max    int `json:"maxMessages"`
					} `json:"request"`
				} `json:"args"`
			} `json:"payload"`
		}
		if json.NewDecoder(r.Body).Decode(&rpc) != nil {
			t.Error("invalid history RPC")
		}
		q := rpc.Payload.Args.Request
		if q.Address.ID != "s" || q.Cut != 59 || q.Before < 20 || q.Before > 60 || q.Before%20 != 0 || q.Max != historyPageMessages {
			t.Error("history lost session identity or pinned page coordinates")
		}
		json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": rpc.ID, "result": map[string]any{"ok": true, "value": historyFixture(max(0, q.Before-20), q.Before-1, 59)}})
	}))
	defer server.Close()
	m := chatModel(t)
	h := historyTestReader(t, m)
	m.settingsLogin = server.URL + "/?token=test"
	cmd := m.requestHistory("older", 59, 40)
	if cmd == nil || m.requestHistory("older", 59, 40) != nil {
		t.Fatal("concurrent history requests were allowed")
	}
	result := cmd().(historyResult)
	if result.err != nil || result.page.Cursor != 59 || result.reader != h {
		t.Fatal("history read failed", result.err)
	}
}
