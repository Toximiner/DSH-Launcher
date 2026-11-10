package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func approvalModel(t *testing.T) *model {
	m := chatModel(t)
	m.approvals = &approvalState{ctx: m.chat.ctx, epoch: 1, inbox: make(chan approvalNotice, 32), selection: -1}
	m.handleApprovalUpdate(approvalUpdate{epoch: 1, notice: approvalNotice{generation: 1, frame: &approvalFrame{Type: "ready", ClientID: "client"}}})
	return m
}
func requestApproval(m *model, id, agent string) {
	m.handleApprovalUpdate(approvalUpdate{epoch: 1, notice: approvalNotice{generation: 1, frame: &approvalFrame{Type: "waterfall", ID: id, AgentID: agent, Event: "approval/request", Request: approvalRequest{ToolName: "bash", CallID: "call", Reason: "Write a marker in the isolated test workspace."}}}})
}
func TestApprovalRequiresExplicitChoiceAndPreservesDraft(t *testing.T) {
	m := approvalModel(t)
	m.input.SetValue("my unsent draft")
	var outcomes []string
	m.approvals.answer = func(id, outcome string) error { outcomes = append(outcomes, outcome); return nil }
	requestApproval(m, "event", "s")
	for _, msg := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyRunes, Runes: []rune("enter")}, {Type: tea.KeyRunes, Runes: []rune("yes")}, {Type: tea.KeyRunes, Runes: []rune("pasted"), Paste: true}} {
		_, cmd := m.Update(msg)
		if cmd != nil {
			t.Fatal("unselected or ordinary text input produced an action")
		}
	}
	if m.input.Value() != "my unsent draft" || len(outcomes) != 0 {
		t.Fatal("permission consumed the composer draft")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("explicit grant did not produce a request")
	}
	if m.answerApproval() != nil {
		t.Fatal("double-click produced another answer")
	}
	m.Update(cmd())
	if len(outcomes) != 1 || outcomes[0] != "allowed-once" || len(m.approvals.requests) != 0 || m.input.Value() != "my unsent draft" {
		t.Fatal("grant or draft restoration failed")
	}
	requestApproval(m, "reject", "s")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(cmd())
	if outcomes[1] != "rejected" {
		t.Fatal("Reject did not use the rejection outcome")
	}
}
func TestApprovalQueueWithdrawalsAndStaleConnections(t *testing.T) {
	m := approvalModel(t)
	requestApproval(m, "other-session", "other")
	requestApproval(m, "one", "s")
	requestApproval(m, "one", "s")
	requestApproval(m, "two", "s")
	if len(m.approvals.requests) != 2 {
		t.Fatal("duplicate or foreign-session permission entered the queue")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.approvals.visible {
		t.Fatal("Escape should defer rather than answer")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF7})
	if !m.approvals.visible {
		t.Fatal("F7 did not reopen the pending permission")
	}
	m.handleApprovalUpdate(approvalUpdate{epoch: 1, notice: approvalNotice{generation: 1, frame: &approvalFrame{Type: "cancel", ID: "one"}}})
	if len(m.approvals.requests) != 1 || m.approvals.requests[0].ID != "two" || m.approvals.selection != -1 {
		t.Fatal("withdrawal did not advance the queue safely")
	}
	m.approvals.selection = 1
	m.handleApprovalUpdate(approvalUpdate{epoch: 1, notice: approvalNotice{generation: 1, err: errors.New("lost")}})
	if m.answerApproval() != nil {
		t.Fatal("disconnected stream allowed a stale answer")
	}
	m.handleApprovalUpdate(approvalUpdate{epoch: 1, notice: approvalNotice{generation: 2, frame: &approvalFrame{Type: "ready", ClientID: "replacement"}}})
	m.handleApprovalAnswer(approvalAnswerResult{epoch: 1, generation: 1, id: "two"})
	if len(m.approvals.requests) != 0 || m.approvals.visible {
		t.Fatal("reconnection retained stale requests")
	}
	m.handleApprovalUpdate(approvalUpdate{epoch: 0, notice: approvalNotice{generation: 9, frame: &approvalFrame{Type: "ready"}}})
	if m.approvals.generation != 2 {
		t.Fatal("old session changed the active interaction stream")
	}
}
func TestApprovalUnknownAnswerNeverAutomaticallyRetries(t *testing.T) {
	m := approvalModel(t)
	requestApproval(m, "one", "s")
	disconnected := false
	m.approvals.disconnect = func() { disconnected = true }
	m.approvals.answer = func(string, string) error { return errors.New("private token must not be displayed") }
	m.approvals.selection = 1
	cmd := m.answerApproval()
	m.Update(cmd())
	if !disconnected || m.approvals.ready || m.answerApproval() != nil || strings.Contains(m.approvals.note, "private token") {
		t.Fatal("uncertain result leaked details or permitted a retry on a stale stream")
	}
}
func TestApprovalLayoutAndMouseAtMinimumSize(t *testing.T) {
	for _, width := range []int{100, 44} {
		m := approvalModel(t)
		requestApproval(m, "one", "s")
		m.approvals.requests[0].Request.Reason = strings.Repeat("detailed reason line\n", 40) + "last reason line"
		m.Update(tea.WindowSizeMsg{Width: width, Height: 18})
		view := strings.Split(clean(m.View()), "\n")
		box := m.approvalBox()
		row := box.y + box.height - 4
		if row >= len(view) || !strings.Contains(view[row], "Отказать") || !strings.Contains(view[row], "Разрешить") {
			t.Fatal("permission buttons overflowed or their mouse row was misaligned")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		if m.approvals.viewport.YOffset == 0 {
			t.Fatal("long permission details cannot be scrolled")
		}
		var answer string
		m.approvals.answer = func(_ string, outcome string) error { answer = outcome; return nil }
		cmd := m.mouse(tea.MouseMsg{X: box.x + 2 + (box.width-4)/2, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if cmd == nil {
			t.Fatal("Allow mouse target did not produce an answer")
		}
		m.Update(cmd())
		if answer != "allowed-once" {
			t.Fatal("mouse selected the wrong permission action")
		}
	}
}
func TestApprovalAuditRestoresAuthoritativeDecision(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":1,"records":[{"type":"event","event":{"seq":0,"type":"approval/asked","data":{"id":"audit","toolName":"bash","reason":"test"}}},{"type":"event","event":{"seq":1,"type":"approval/decided","data":{"id":"audit","outcome":"rejected"}}}]}`)
	if len(m.chat.messages) != 1 || !strings.Contains(m.chatView(), "bash · Отказано") {
		t.Fatal("approval audit was duplicated or not restored from history")
	}
}
func TestApprovalTransportAnswersThroughReceivingPeer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var lock sync.Mutex
	connections := 0
	answered := make(chan string, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/connect":
			lock.Lock()
			connections++
			peer := fmt.Sprint(connections)
			lock.Unlock()
			http.SetCookie(w, &http.Cookie{Name: "peer", Value: peer, Path: "/"})
			w.WriteHeader(302)
		case "/api/remote.mux":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			cookie, _ := r.Cookie("peer")
			if cookie == nil || cookie.Value != "1" {
				t.Error("stream used an unexpected Peer")
			}
			var open struct {
				ID       string `json:"streamId"`
				Endpoint string `json:"endpoint"`
			}
			if conn.ReadJSON(&open) != nil {
				return
			}
			if open.Endpoint != "$events" {
				t.Error("wrong interaction endpoint")
				return
			}
			for _, frame := range []any{map[string]any{"type": "ready", "clientId": "client"}, map[string]any{"type": "waterfall", "eventId": "foreign", "agentId": "other", "event": "approval/request"}, map[string]any{"type": "waterfall", "eventId": "event", "agentId": "s", "event": "approval/request", "request": map[string]any{"toolName": "bash", "reason": "test"}}} {
				if conn.WriteJSON(map[string]any{"type": "item", "streamId": open.ID, "value": frame}) != nil {
					return
				}
			}
			<-ctx.Done()
		case "/api/$events/result":
			cookie, _ := r.Cookie("peer")
			if cookie == nil || cookie.Value != "1" {
				t.Error("answer did not reuse the receiving Peer")
			}
			var rpc struct {
				ID      string `json:"rpcId"`
				Payload struct {
					Args struct {
						ClientID, EventID string
						Outcome           struct{ Kind, Value string }
					}
				}
			}
			if json.NewDecoder(r.Body).Decode(&rpc) != nil {
				t.Error("invalid answer")
			}
			if rpc.Payload.Args.ClientID != "client" || rpc.Payload.Args.EventID != "event" || rpc.Payload.Args.Outcome.Kind != "result" {
				t.Error("answer lost event correlation")
			}
			answered <- rpc.Payload.Args.Outcome.Value
			json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": rpc.ID, "result": map[string]any{"ok": true}})
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer func() { cancel(); server.Close() }()
	inbox := make(chan approvalNotice, 32)
	done := make(chan struct{})
	go func() { followApprovalEvents(ctx, server.URL+"/?token=test", "s", inbox); close(done) }()
	var ready approvalNotice
	select {
	case ready = <-inbox:
	case <-time.After(3 * time.Second):
		t.Fatal("no ready frame")
	}
	select {
	case request := <-inbox:
		if request.frame.ID != "event" {
			t.Fatal("foreign permission was delivered")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no approval")
	}
	if err := ready.answer("event", "rejected"); err != nil {
		t.Fatal(err)
	}
	if value := <-answered; value != "rejected" {
		t.Fatal("wrong outcome")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("interaction reader leaked after cancellation")
	}
}

func TestApprovalOverflowReplaysAfterVisibleBatch(t *testing.T) {
	m := approvalModel(t)
	disconnected := false
	m.approvals.disconnect = func() { disconnected = true }
	for i := 0; i < 33; i++ {
		requestApproval(m, fmt.Sprint(i), "s")
	}
	if len(m.approvals.requests) != 32 || !m.approvals.overflow {
		t.Fatal("permission queue was not bounded")
	}
	for i := 0; i < 32; i++ {
		m.removeApproval(fmt.Sprint(i))
	}
	if !disconnected || m.approvals.ready {
		t.Fatal("excess requests were left inaccessible instead of being replayed")
	}
}

func TestTimedQuestionClaimUsesInteractionMux(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checked := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "peer", Value: "owner", Path: "/"})
			w.WriteHeader(302)
			return
		}
		if r.URL.Path != "/api/remote.mux" {
			t.Error("unexpected question transport route")
			w.WriteHeader(404)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var primary struct {
			ID       string `json:"streamId"`
			Endpoint string `json:"endpoint"`
		}
		if conn.ReadJSON(&primary) != nil {
			return
		}
		send := func(id string, value any) {
			_ = conn.WriteJSON(map[string]any{"type": "item", "streamId": id, "value": value})
		}
		send(primary.ID, map[string]any{"type": "ready", "clientId": "client"})
		send(primary.ID, map[string]any{"type": "waterfall", "event": "user-questions/request", "eventId": "question", "agentId": "s", "request": map[string]any{"wait": map[string]any{"callId": "call", "timed": true}, "questions": []any{map[string]any{"id": "q", "question": "Choose"}}}})
		var claim struct {
			ID       string `json:"streamId"`
			Endpoint string `json:"endpoint"`
			Payload  struct {
				Args struct {
					AgentID string `json:"agentId"`
					CallID  string `json:"callId"`
				}
			}
		}
		if conn.ReadJSON(&claim) != nil {
			return
		}
		if claim.Endpoint != "userQuestions/attachWait" || claim.Payload.Args.AgentID != "s" || claim.Payload.Args.CallID != "call" || claim.ID == primary.ID {
			t.Error("timed claim lost its scope or reused the primary logical stream")
		}
		send(claim.ID, map[string]any{"remainingMs": 1000})
		send(primary.ID, map[string]any{"type": "cancel", "eventId": "question"})
		var stop struct {
			Type string
			ID   string `json:"streamId"`
		}
		if conn.ReadJSON(&stop) != nil {
			return
		}
		if stop.Type != "cancel" || stop.ID != claim.ID {
			t.Error("withdrawal did not release the logical wait")
		}
		close(checked)
		<-ctx.Done()
	}))
	defer func() { cancel(); server.Close() }()
	inbox := make(chan approvalNotice, 32)
	done := make(chan struct{})
	go func() { followApprovalEvents(ctx, server.URL+"/?token=test", "s", inbox); close(done) }()
	select {
	case <-checked:
	case <-time.After(3 * time.Second):
		t.Fatal("timed wait was not attached/released on the receiving mux")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("question transport leaked after cancellation")
	}
}
