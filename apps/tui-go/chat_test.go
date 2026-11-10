package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
)

func decodeChat(t *testing.T, text string) *chatFrame {
	t.Helper()
	var frame chatFrame
	if err := json.Unmarshal([]byte(text), &frame); err != nil {
		t.Fatal(err)
	}
	return &frame
}

func chatModel(t *testing.T) *model {
	t.Helper()
	m := newModel()
	m.settingsLogin = "http://127.0.0.1:1/?token=private"
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	m.settingsContext = ctx
	m.session = &sessionView{ID: "s"}
	m.chat = &chatState{id: "s", ctx: ctx, cancel: cancel, epoch: 1, inbox: make(chan chatNotice, 32), cursor: -1, clearedThrough: -1}
	return m
}

func applyChat(t *testing.T, m *model, text string) {
	t.Helper()
	if err := m.applyChatFrame(m.chat, decodeChat(t, text)); err != nil {
		t.Fatal(err)
	}
}

func TestChatStreamingCommitAndReconnect(t *testing.T) {
	m := chatModel(t)
	m.prompts["s"] = &pendingPrompt{ID: "request-1", Text: "hello", Status: "unknown"}
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":1,"records":[{"type":"event","event":{"type":"turn/start","seq":0,"data":{"turn":1}}},{"type":"event","event":{"type":"user/message","seq":1,"surfaceOp":"append","data":{"content":[{"type":"text","text":"hello"}],"source":{"rpcId":"request-1"}}}}]}`)
	if m.prompts["s"] != nil || !m.chat.ready {
		t.Fatal("durable receipt did not reconcile an unknown send")
	}
	applyChat(t, m, `{"type":"assistant-stream","frame":{"type":"start","attemptId":"a","turn":1,"step":0}}`)
	chunk := `{"type":"assistant-stream","frame":{"type":"chunk","attemptId":"a","index":0,"chunk":{"type":"text-delta","index":0,"text":"Привет "}}}`
	applyChat(t, m, chunk)
	applyChat(t, m, chunk)
	if m.chat.live.text() != "Привет " {
		t.Fatal("duplicate live chunk duplicated text")
	}
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":1,"records":[{"type":"event","event":{"type":"user/message","seq":1,"surfaceOp":"append","data":{"content":[{"type":"text","text":"hello"}]}}}],"assistantStream":{"activeAttempt":{"attemptId":"a","turn":1,"step":0,"nextIndex":1,"stream":[{"type":"text-chunks","index":0,"texts":["Привет "]}]}}}`)
	applyChat(t, m, `{"type":"assistant-stream","frame":{"type":"chunk","attemptId":"a","index":1,"chunk":{"type":"text-delta","index":0,"text":"世界"}}}`)
	applyChat(t, m, `{"type":"assistant-stream","frame":{"type":"chunk","attemptId":"a","index":2,"chunk":{"type":"block-end","index":0,"block":{"type":"text","text":"Привет 世界"}}}}`)
	if m.chat.live.text() != "Привет 世界" {
		t.Fatal("baseline or block-end duplicated streamed content")
	}
	applyChat(t, m, `{"type":"assistant-stream","frame":{"type":"end","attemptId":"a","index":3,"outcome":{"kind":"committed","seq":2}}}`)
	applyChat(t, m, `{"type":"event","event":{"type":"assistant/message","seq":2,"surfaceOp":"append","data":{"turn":1,"step":0,"message":{"content":[{"type":"reasoning","text":"hidden"},{"type":"text","text":"Привет 世界"}]}}}}`)
	if m.chat.live != nil || strings.Count(m.chatView(), "Привет 世界") != 1 || strings.Contains(m.chatView(), "hidden") {
		t.Fatal("durable answer did not replace the live attempt")
	}
	applyChat(t, m, `{"type":"event","event":{"type":"turn/end","seq":3,"data":{"reason":{"kind":"aborted"}}}}`)
	if m.session.Running || !strings.Contains(m.chatView(), "остановлено") {
		t.Fatal("cancellation was not read from the backend")
	}
}

func TestChatRejectsGapsAndPreservesValidSnapshot(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":0,"records":[{"type":"event","event":{"type":"user/message","seq":0,"surfaceOp":"append","data":{"content":[{"type":"text","text":"preserved"}]}}}]}`)
	for _, text := range []string{
		`{"type":"snapshot","header":{"id":"other"},"cursor":0}`,
		`{"type":"snapshot","header":{"id":"s"},"cursor":0,"records":[{"type":"event","event":{"type":"user/message","seq":1,"data":{}}}]}`,
		`{"type":"event","event":{"type":"user/message","seq":2,"data":{}}}`,
		`{"type":"assistant-stream","frame":{"type":"chunk","attemptId":"missing","index":0,"chunk":{}}}`,
	} {
		if m.applyChatFrame(m.chat, decodeChat(t, text)) == nil {
			t.Fatal("invalid stream frame accepted")
		}
		if len(m.chat.messages) != 1 || m.chat.messages[0].text != "preserved" {
			t.Fatal("bad snapshot discarded the last valid history")
		}
	}
	applyChat(t, m, `{"type":"assistant-stream","frame":{"type":"start","attemptId":"a"}}`)
	if m.applyChatFrame(m.chat, decodeChat(t, `{"type":"assistant-stream","frame":{"type":"chunk","attemptId":"a","index":2,"chunk":{"type":"text-delta","text":"gap"}}}`)) == nil {
		t.Fatal("stream index gap accepted")
	}
}

func TestChatSurfaceReplacementAndSafeErrors(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":1,"records":[{"type":"event","event":{"type":"user/message","seq":0,"surfaceOp":"append","data":{"content":[{"type":"text","text":"old"}]}}},{"type":"event","event":{"type":"assistant/message","seq":1,"surfaceOp":"append","data":{"message":{"content":[{"type":"text","text":"answer"}]}}}}]}`)
	applyChat(t, m, `{"type":"event","event":{"type":"user/message","seq":2,"surfaceOp":{"op":"replace","startSeq":0,"endSeq":0},"data":{"content":[{"type":"text","text":"edited\u001b[2J"}]}}}`)
	if m.chat.messages[0].text != "edited" || m.chat.messages[1].text != "answer" {
		t.Fatal("replacement changed visible order or retained terminal controls")
	}
	applyChat(t, m, `{"type":"event","event":{"type":"plugin/custom","seq":3,"data":42}}`)
	applyChat(t, m, `{"type":"event","event":{"type":"turn/end","seq":4,"data":{"reason":{"kind":"error","error":{"code":"MISSING_CREDENTIAL","message":"secret-token"}}}}}`)
	if strings.Contains(m.chatView(), "secret-token") || !strings.Contains(m.chatView(), "доступа к модели") {
		t.Fatal("provider failure was leaked or not explained")
	}
	before := m.chatView()
	m.Update(chatUpdate{epoch: 0, notice: chatNotice{frame: decodeChat(t, `{"type":"snapshot","header":{"id":"other"}}`)}})
	if m.chatView() != before {
		t.Fatal("late frame from another subscription changed the view")
	}
}

func TestChatHidesServiceContextAndPreservesItsSurface(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s","cwd":"/server/project"},"cursor":1,"records":[{"type":"event","event":{"type":"user/message","seq":0,"surfaceOp":"append","data":{"source":{"kind":"user"},"content":[{"type":"text","text":"actual user"}]}}},{"type":"event","event":{"type":"user/message","seq":1,"surfaceOp":"append","data":{"source":{"kind":"runtime-context"},"content":[{"type":"text","text":"hidden context"}]}}}]}`)
	applyChat(t, m, `{"type":"event","event":{"type":"user/message","seq":2,"surfaceOp":{"op":"replace","startSeq":1,"endSeq":1},"data":{"source":{"kind":"runtime-context"},"content":[{"type":"text","text":"new hidden context"}]}}}`)
	if strings.Contains(m.chatView(), "hidden context") || !strings.Contains(m.chatView(), "actual user") || m.workspace != "/server/project" {
		t.Fatal("backend context was shown as user text or workspace was not authoritative")
	}
	m.chat.ready = true
	m.sessionBusy = true
	m.sessionAction = "connect"
	if m.submitText("new draft") != nil || len(m.prompts) != 0 || m.input.Value() != "new draft" {
		t.Fatal("message was sent during session switching")
	}
	if m.cancelChat() != nil {
		t.Fatal("cancel targeted an old session during switching")
	}
	m.sessionBusy = false
	m.sessionQueue = []sessionAction{{action: "new"}}
	if m.submitText("queued switch draft") != nil {
		t.Fatal("message ignored a queued session switch")
	}
}

func TestChatNoticesFollowTailWithoutStealingHistoryScroll(t *testing.T) {
	for _, text := range []string{"enter", "ctrl+d", "tab"} {
		m := newModel()
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
		if m.input.Value() != text {
			t.Fatal("ordinary text was interpreted as a control key")
		}
	}
	m := chatModel(t)
	m.chat.ready = true
	for i := 0; i < 30; i++ {
		m.chat.messages = append(m.chat.messages, chatMessage{seq: i, role: "АГЕНТ", text: "record"})
	}
	m.rebuild()
	m.viewport.GotoBottom()
	m.chat.note = "Выполнение остановлено."
	m.rebuild()
	if !m.viewport.AtBottom() || !strings.Contains(clean(m.viewport.View()), "Выполнение остановлено.") {
		t.Fatal("new status notice fell below the visible tail")
	}
	m.viewport.LineUp(3)
	offset := m.viewport.YOffset
	m.chat.note = strings.Repeat("new notice ", 30)
	m.rebuild()
	if m.viewport.YOffset != offset {
		t.Fatal("incoming status stole the user's history scroll")
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	_, height := m.contentSize()
	y := bodyTop + height - composerHeight - 1 + 4
	x := sidebarWidth + sidebarGap + 2 + len([]rune("Enter — отправить · "))
	if !strings.Contains(clean(strings.Split(m.View(), "\n")[y]), "[F8 Стоп]") {
		t.Fatal("stop hit area is not aligned with its label")
	}
	if cmd := m.mouse(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}); cmd == nil {
		t.Fatal("clicking Stop did not request cancellation")
	}
}

func TestPromptReceiptLossAndManualRetry(t *testing.T) {
	var mu sync.Mutex
	requests := []string{}
	mode := "drop"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "test", Path: "/"})
			w.Header().Set("Location", "/")
			w.WriteHeader(302)
			return
		}
		if _, err := r.Cookie("dsh"); err != nil {
			t.Error("RPC lost the login cookie")
		}
		var body struct {
			RPCID   string `json:"rpcId"`
			Method  string `json:"method"`
			Payload struct {
				Args struct {
					Request struct {
						ID      string `json:"requestId"`
						Session string `json:"sessionId"`
						Mode    string `json:"mode"`
						Content []struct {
							Text string `json:"text"`
						} `json:"content"`
					} `json:"request"`
				} `json:"args"`
			} `json:"payload"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		if body.Method == "session/prompt" {
			if body.Payload.Args.Request.Session != "s" || body.Payload.Args.Request.Mode != "queue" || body.Payload.Args.Request.Content[0].Text != "test prompt" {
				t.Error("invalid prompt request")
			}
			requests = append(requests, body.Payload.Args.Request.ID)
			if mode == "drop" {
				conn, _, _ := w.(http.Hijacker).Hijack()
				conn.Close()
				return
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": body.RPCID, "result": map[string]any{"ok": mode != "reject", "value": map[string]any{"accepted": true}}})
	}))
	defer server.Close()
	m := chatModel(t)
	m.settingsLogin = server.URL + "/?token=private"
	m.chat.ready = true
	cmd := m.submitText("test prompt")
	m.Update(cmd())
	if m.prompts["s"].Status != "unknown" {
		t.Fatal("lost receipt was presented as rejection or success")
	}
	m.input.SetValue("new draft")
	if m.submitText("new draft") != nil || m.input.Value() != "new draft" {
		t.Fatal("uncertain send was overwritten by another message")
	}
	mu.Lock()
	mode = "ok"
	mu.Unlock()
	m.Update(m.retryPrompt()())
	mu.Lock()
	if len(requests) != 2 || requests[0] == "" || requests[0] != requests[1] {
		t.Fatal("manual retry minted another request or transport retried automatically")
	}
	mu.Unlock()
	id := m.prompts["s"].ID
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":0,"records":[{"type":"event","event":{"type":"user/message","seq":0,"surfaceOp":"append","data":{"content":[{"type":"text","text":"test prompt"}],"source":{"rpcId":"`+id+`"}}}}]}`)
	if m.prompts["s"] != nil {
		t.Fatal("history did not reconcile the acknowledged prompt")
	}
	m.handlePromptResult(promptResult{sessionID: "s", requestID: id, err: errors.New("late")})
	if m.prompts["s"] != nil {
		t.Fatal("late receipt recreated an already durable prompt")
	}
	m.Update(m.cancelChat()())
	mu.Lock()
	mode = "reject"
	mu.Unlock()
	m.Update(m.submitText("test prompt")())
	if m.prompts["s"].Status != "rejected" {
		t.Fatal("explicit refusal was presented as uncertain acceptance")
	}
}

func TestWebSocketFollowAuthenticationAndReconnect(t *testing.T) {
	var mu sync.Mutex
	opens := 0
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == "http://"+r.Host }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "test", Path: "/"})
			w.Header().Set("Location", "/")
			w.WriteHeader(302)
			return
		}
		if r.URL.Path != "/api/remote.mux" || r.URL.RawQuery != "" {
			t.Error("invalid WebSocket target or leaked login query")
		}
		if _, err := r.Cookie("dsh"); err != nil {
			t.Error("WebSocket lost cookie authentication")
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var open struct {
			Type, StreamID, Endpoint string
			Payload                  struct {
				Args struct {
					Request struct {
						Address         struct{ Kind, SessionID string }
						AssistantStream bool
						MaxMessages     int
					}
				}
			}
		}
		if err = conn.ReadJSON(&open); err != nil {
			return
		}
		if open.Type != "open" || open.Endpoint != "session/follow" || open.Payload.Args.Request.Address.SessionID != "s" || !open.Payload.Args.Request.AssistantStream || open.Payload.Args.Request.MaxMessages != 50 {
			t.Error("invalid subscription envelope")
		}
		mu.Lock()
		opens++
		number := opens
		mu.Unlock()
		conn.WriteJSON(map[string]any{"type": "item", "streamId": open.StreamID, "value": map[string]any{"type": "snapshot", "header": map[string]any{"id": "s"}, "cursor": -1, "records": []any{}}})
		if number == 1 {
			return
		}
		for {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan chatNotice, 16)
	done := make(chan struct{})
	go func() { defer close(done); followChat(ctx, server.URL+"/?token=private", "s", out) }()
	snapshots := 0
	deadline := time.After(5 * time.Second)
	for snapshots < 2 {
		select {
		case notice := <-out:
			if notice.frame != nil && notice.frame.Type == "snapshot" {
				snapshots++
			}
		case <-deadline:
			t.Fatal("stream did not reconnect")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("subscription leaked after cancellation")
	}
}
