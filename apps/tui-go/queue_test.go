package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func queueItem(request, text string) queuedMessage {
	return queuedMessage{ID: "item-" + request, RequestID: request, Placement: "queued", Content: []queueContent{{Type: "text", Text: text}}}
}
func TestQueueAcceptsFollowupWhileRunningAndUsesServerProof(t *testing.T) {
	m := chatModel(t)
	m.chat.ready = true
	m.chat.cursor = 2
	m.session.Running = true
	m.chat.live = &chatAttempt{ID: "live"}
	if cmd := m.submitText("second task"); cmd == nil {
		t.Fatal("running agent rejected a queued follow-up")
	}
	id := m.prompts["s"].ID
	m.input.SetValue("third draft")
	if m.submitText("third draft") != nil || m.input.Value() != "third draft" {
		t.Fatal("in-flight receipt was overwritten")
	}
	m.handlePromptResult(promptResult{sessionID: "s", requestID: id})
	if m.prompts["s"].Status != "accepted" {
		t.Fatal("acknowledged follow-up has no receipt state")
	}
	m.session.QueueKnown = true
	m.session.Queue = []queuedMessage{queueItem(id, "second task")}
	m.reconcileQueuedPrompt()
	if m.prompts["s"] != nil || !strings.Contains(m.queueView(), "second task") {
		t.Fatal("server queue did not reconcile the prompt receipt")
	}
	if cmd := m.submitText("third task"); cmd == nil || m.prompts["s"].ID == id {
		t.Fatal("next queued task was blocked or reused an unrelated request ID")
	}
}
func TestQueueLateStateDoesNotDuplicateAdmittedHistory(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":0,"records":[]}`)
	m.session.QueueKnown = true
	m.session.Queue = []queuedMessage{queueItem("request", "queued text")}
	applyChat(t, m, `{"type":"event","event":{"seq":1,"type":"user/message","surfaceOp":"append","data":{"source":{"kind":"user","rpcId":"request"},"content":[{"type":"text","text":"queued text"}]}}}`)
	if len(m.visibleQueue()) != 0 || strings.Count(m.chatView(), "queued text") != 1 {
		t.Fatal("late queue sample duplicated an already admitted message")
	}
	m.session.Queue = append(m.session.Queue, queueItem("other", "waiting text"))
	if len(m.visibleQueue()) != 1 {
		t.Fatal("admission hid unrelated pending work")
	}
}
func TestQueueAbsenceNeverResolvesUncertainSubmission(t *testing.T) {
	m := chatModel(t)
	m.chat.ready = true
	m.chat.cursor = 5
	now := time.Now()
	m.session.QueueKnown = true
	m.session.StateCursor = 5
	m.session.QueueFetchedAt = now.Add(time.Second)
	m.prompts["s"] = &pendingPrompt{ID: "request", Text: "uncertain", Status: "unknown"}
	m.reconcileQueuedPrompt()
	if m.prompts["s"] == nil {
		t.Fatal("queue absence was treated as evidence for an uncertain RPC")
	}
	m.session.Queue = []queuedMessage{queueItem("request", "uncertain")}
	m.reconcileQueuedPrompt()
	if m.prompts["s"] != nil {
		t.Fatal("queue evidence did not reconcile an uncertain receipt")
	}
	m.session.Queue = nil
	m.prompts["s"] = &pendingPrompt{ID: "accepted", Text: "accepted", Status: "accepted", AcceptedAt: now}
	m.session.QueueFetchedAt = now.Add(-time.Second)
	m.reconcileQueuedPrompt()
	if m.prompts["s"] == nil {
		t.Fatal("an old passive read erased a new receipt")
	}
	m.session.QueueFetchedAt = now.Add(time.Second)
	m.session.StateCursor = 6
	m.reconcileQueuedPrompt()
	if m.prompts["s"] == nil {
		t.Fatal("queue absence was trusted before history caught up")
	}
	m.chat.cursor = 6
	m.reconcileQueuedPrompt()
	if m.prompts["s"] != nil || !strings.Contains(m.chat.note, "больше не находится") {
		t.Fatal("confirmed receipt outside recent history blocked subsequent tasks forever")
	}
}
func TestQueueReaderDistinguishesColdAndKnownEmptyState(t *testing.T) {
	for _, sample := range []string{`null`, `[]`, `[{"id":"item","rpcId":"r","placement":"queued","content":[{"type":"text","text":"task"}]}]`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/connect" {
				http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "owner", Path: "/"})
				w.WriteHeader(302)
				return
			}
			var request struct {
				ID     string `json:"rpcId"`
				Method string `json:"method"`
			}
			json.NewDecoder(r.Body).Decode(&request)
			value := json.RawMessage(`{"kind":"sequenced","values":{}}`)
			if request.Method == "session/state" {
				value = json.RawMessage(`{"cursor":2,"running":true,"queue":` + sample + `}`)
			}
			json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": request.ID, "result": map[string]any{"ok": true, "value": value}})
		}))
		api, err := openAPI(context.Background(), server.URL+"/?token=test")
		if err != nil {
			server.Close()
			t.Fatal(err)
		}
		view, err := readSession(context.Background(), api, "s")
		api.close()
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		if view.QueueKnown != (sample != "null") || view.QueueFetchedAt.IsZero() {
			t.Fatal("queue state availability was not preserved")
		}
		if strings.Contains(sample, "task") && (len(view.Queue) != 1 || view.Queue[0].RequestID != "r") {
			t.Fatal("wire queue lost request correlation")
		}
	}
}
func TestQueueMasksContextAndKeepsStopMouseAligned(t *testing.T) {
	m := chatModel(t)
	m.chat.ready = true
	m.session.Running = true
	m.session.QueueKnown = true
	contextItem := queueItem("context", "hidden service context")
	contextItem.Placement = "context"
	m.session.Queue = []queuedMessage{queueItem("r", "task\x1b[2J"), contextItem}
	m.sessionStale = true
	view := m.queueView()
	if strings.Contains(view, "hidden service context") || strings.Contains(view, "\x1b[2J") || !strings.Contains(view, "Последнее полученное") {
		t.Fatal("queue exposed service context, terminal controls, or unlabeled stale data")
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	_, hint := m.composerText()
	if !strings.Contains(hint, "в очередь") {
		t.Fatal("busy composer did not explain queue submission")
	}
	_, height := m.contentSize()
	y := bodyTop + height - composerHeight - 1 + 4
	x := sidebarWidth + sidebarGap + 2 + len([]rune(hint[:strings.Index(hint, "[F8 Стоп]")]))
	if !strings.Contains(strings.Split(clean(m.View()), "\n")[y], "[F8 Стоп]") || m.mouse(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}) == nil {
		t.Fatal("queue hint broke the Stop mouse target")
	}
}
