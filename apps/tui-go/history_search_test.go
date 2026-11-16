package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestHistorySearchCrossesPagesWithUnicodeAndExclusiveResume(t *testing.T) {
	reads := 0
	read := func(_ context.Context, before int) (historyPage, error) {
		reads++
		first := max(0, before-10)
		page := historyFixture(first, before-1, 69)
		for i := range page.Records {
			if page.Records[i].Event.Seq == 2 {
				page.Records[i].Event.Data = json.RawMessage(`{"content":[{"type":"text","text":"Начало: ИСТОРИЯ 🙂"}],"source":{"kind":"user"}}`)
			}
		}
		return page, nil
	}
	result := scanHistory(context.Background(), historySearchResult{cut: 69, nextBefore: 70, hitSeq: -1, query: "история 🙂"}, "s", read)
	if result.err != nil || result.hitSeq != 2 || result.nextBefore != 0 || reads != 7 || result.page.Cursor != 69 || result.before != 10 {
		t.Fatalf("search missed an older Unicode message: %+v", result)
	}
	result = scanHistory(context.Background(), historySearchResult{cut: 69, nextBefore: result.nextBefore, hitSeq: -1, query: "история 🙂"}, "s", read)
	if result.err != nil || !result.finished || result.hitSeq != -1 || reads != 8 {
		t.Fatal("exclusive resume repeated the same message or skipped exhaustion")
	}
}

func TestHistorySearchKeepsReplacementSemanticsBetweenMatches(t *testing.T) {
	page := historyFixture(0, 2, 2)
	page.Records[0].Event.Data = json.RawMessage(`{"content":[{"type":"text","text":"needle removed"}],"source":{"kind":"user"}}`)
	page.Records[1].Event.Data = json.RawMessage(`{"content":[{"type":"text","text":"needle retained"}],"source":{"kind":"user"}}`)
	page.Records[2].Event.Data = json.RawMessage(`{"content":[{"type":"text","text":"replacement"}],"source":{"kind":"user"}}`)
	page.Records[2].Event.SurfaceOp = json.RawMessage(`{"op":"replace","startSeq":0,"endSeq":0}`)
	result := scanHistory(context.Background(), historySearchResult{cut: 2, nextBefore: 3, hitSeq: -1, query: "needle"}, "s", func(context.Context, int) (historyPage, error) { return page, nil })
	if result.err != nil || result.hitSeq != 1 || len(result.hits) != 1 || result.nextBefore != 0 || !result.finished {
		t.Fatal("search would resurrect a message removed later in the same page")
	}
	m := chatModel(t)
	h := historyTestReader(t, m)
	m.editHistorySearch()
	h.search.editor.SetValue("history message")
	m.startHistorySearch(true)
	page = historyFixture(40, 59, 59)
	result = scanHistory(context.Background(), historySearchResult{reader: h, serial: h.serial, cut: 59, nextBefore: 60, hitSeq: -1, query: h.search.query}, "s", func(context.Context, int) (historyPage, error) { return page, nil })
	m.handleHistorySearch(result)
	if h.search.hitSeq != 59 {
		t.Fatal("search did not show the latest match")
	}
	if m.startHistorySearch(false) != nil || h.search.hitSeq != 58 || h.search.running {
		t.Fatal("next same-page match reread a truncated log window instead of using the validated page")
	}
}

func TestHistorySearchBatchLimitDoesNotClaimExhaustion(t *testing.T) {
	read := func(_ context.Context, before int) (historyPage, error) {
		return historyFixture(max(0, before-10), before-1, 199), nil
	}
	result := scanHistory(context.Background(), historySearchResult{cut: 199, nextBefore: 200, hitSeq: -1, query: "absent"}, "s", read)
	if result.err != nil || result.finished || result.pages != historySearchPages || result.nextBefore != 100 {
		t.Fatal("bounded scan claimed no results before reading the whole session")
	}
	result = scanHistory(context.Background(), historySearchResult{cut: 199, nextBefore: result.nextBefore, hitSeq: -1, query: "absent"}, "s", read)
	if result.err != nil || !result.finished || result.nextBefore != 0 {
		t.Fatal("continued scan did not reach actual history exhaustion")
	}
}

func TestHistorySearchRejectsBadPagesAndIgnoresToolText(t *testing.T) {
	for _, read := range []func(context.Context, int) (historyPage, error){
		func(context.Context, int) (historyPage, error) {
			return historyPage{}, errors.New("private server detail")
		},
		func(context.Context, int) (historyPage, error) {
			p := historyFixture(0, 2, 2)
			p.Records = p.Records[:1]
			return p, nil
		},
	} {
		result := scanHistory(context.Background(), historySearchResult{cut: 2, nextBefore: 3, hitSeq: -1, query: "x"}, "s", read)
		if result.err == nil || result.finished || result.hitSeq != -1 || strings.Contains(result.err.Error(), "private") {
			t.Fatal("failed search fabricated completion or exposed backend details")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := scanHistory(ctx, historySearchResult{cut: 2, nextBefore: 3, hitSeq: -1, query: "x"}, "s", func(context.Context, int) (historyPage, error) {
		t.Fatal("cancelled search read another page")
		return historyPage{}, nil
	})
	if result.err == nil {
		t.Fatal("cancelled search did not stop")
	}
	p := historyFixture(0, 2, 2)
	p.Records[0].Event.Type = "tool/call"
	p.Records[0].Event.Data = json.RawMessage(`{"callId":"c","name":"needle","arguments":"needle"}`)
	p.Records[1].Event.Type = "tool/result"
	data, _ := json.Marshal(toolResult("c", "needle", false))
	p.Records[1].Event.Data = data
	result = scanHistory(context.Background(), historySearchResult{cut: 2, nextBefore: 3, hitSeq: -1, query: "needle"}, "s", func(context.Context, int) (historyPage, error) { return p, nil })
	if result.err != nil || !result.finished || result.hitSeq != -1 {
		t.Fatal("message search included tool arguments or results")
	}
}

func TestHistorySearchInputCancellationAndLateResultsPreserveLiveDraft(t *testing.T) {
	m := chatModel(t)
	h := historyTestReader(t, m)
	m.handleHistoryResult(historyResult{reader: h, direction: "latest", cut: 59, before: 60, page: historyFixture(40, 59, 59)})
	m.input.SetValue("live draft")
	m.historyKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	m.historyKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	if h.search.editor.Value() != "enter" || h.busy || m.input.Value() != "live draft" {
		t.Fatal("literal input triggered search controls or changed the composer")
	}
	h.search.editor.SetValue("message 042")
	cmd, _ := m.historyKey(tea.KeyMsg{Type: tea.KeyEnter})
	serial := h.serial
	if cmd == nil || !h.search.running {
		t.Fatal("explicit Enter did not start the search")
	}
	m.historyKey(tea.KeyMsg{Type: tea.KeyEsc})
	if h.search.running || h.busy || m.historyReader != h {
		t.Fatal("Escape did not cancel only the pending search")
	}
	m.handleHistorySearch(historySearchResult{reader: h, serial: serial, query: "message 042", cut: 59, before: 60, nextBefore: 42, hitSeq: 42, page: historyFixture(40, 59, 59)})
	if h.search.hitSeq != -1 {
		t.Fatal("cancelled search changed the page")
	}
	m.startHistorySearch(false)
	m.handleHistorySearch(historySearchResult{reader: h, serial: h.serial, query: h.search.query, cut: 59, before: 60, nextBefore: 42, hitSeq: 42, page: historyFixture(40, 59, 59)})
	if !strings.Contains(h.viewport.View(), "▶ Совпадение") || !strings.Contains(h.viewport.View(), "History message 042") || m.input.Value() != "live draft" {
		t.Fatal("search did not jump to its message safely")
	}
	m.closeHistory()
	m.handleHistorySearch(historySearchResult{reader: h, serial: h.serial, hitSeq: 43})
	if m.historyReader != nil {
		t.Fatal("late result reopened a closed history window")
	}
}

func TestHistorySearchEditorAndHitFitSmallTerminals(t *testing.T) {
	for _, size := range [][2]int{{120, 42}, {100, 30}, {80, 24}} {
		m := chatModel(t)
		h := historyTestReader(t, m)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.handleHistoryResult(historyResult{reader: h, direction: "latest", cut: 59, before: 60, page: historyFixture(40, 59, 59)})
		m.editHistorySearch()
		h.search.editor.SetValue(strings.Repeat("Я🙂", 128))
		if lipgloss.Width(m.View()) > m.width-1 || lipgloss.Height(m.View()) > m.height {
			t.Fatalf("search input overflowed at %v", size)
		}
		m.historyKey(tea.KeyMsg{Type: tea.KeyF11})
		if m.historyReader != nil {
			t.Fatal("F11 did not close history while editing a query")
		}
	}
}

func TestHistorySearchTransportOnlyReadsPinnedHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "test", Path: "/"})
			w.WriteHeader(http.StatusFound)
			return
		}
		if r.URL.Path != "/api/session/events" {
			t.Errorf("search used a non-history endpoint %s", r.URL.Path)
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
					} `json:"request"`
				} `json:"args"`
			} `json:"payload"`
		}
		if json.NewDecoder(r.Body).Decode(&rpc) != nil {
			t.Error("invalid search RPC")
		}
		q := rpc.Payload.Args.Request
		if q.Address.ID != "s" || q.Cut != 59 || q.Before != 60 {
			t.Error("search lost its fixed session/cut")
		}
		json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": rpc.ID, "result": map[string]any{"ok": true, "value": historyFixture(40, 59, 59)}})
	}))
	defer server.Close()
	m := chatModel(t)
	h := historyTestReader(t, m)
	m.settingsLogin = server.URL + "/?token=test"
	m.editHistorySearch()
	h.search.editor.SetValue("MESSAGE 042")
	result := m.startHistorySearch(true)().(historySearchResult)
	if result.err != nil || result.hitSeq != 42 {
		t.Fatalf("pinned search transport failed: %+v", result)
	}
}
