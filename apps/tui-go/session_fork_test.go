package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestForkConfirmsSelectedSourceAndQueuesOnlyConnection(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "test", Path: "/"})
			w.WriteHeader(302)
			return
		}
		var req struct {
			ID      string `json:"rpcId"`
			Method  string `json:"method"`
			Payload struct {
				Args struct {
					Request map[string]any `json:"request"`
				} `json:"args"`
			} `json:"payload"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		calls.Add(1)
		args := req.Payload.Args.Request
		if req.Method != "session/fork" || args["sessionId"] != "selected" || args["allowMigration"] != false || len(args) != 2 {
			t.Error("wrong fork request", req)
		}
		json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": req.ID, "result": map[string]any{"ok": true, "value": map[string]any{"sessionId": "child"}}})
	}))
	defer server.Close()
	m := newModel()
	m.settingsLogin = server.URL + "/?token=test"
	m.settingsContext = context.Background()
	m.page = sessionsPage
	m.session = &sessionView{ID: "current"}
	m.sessionItems = []sessionItem{{ID: "selected"}}
	m.input.SetValue("preserved draft")
	m.forkKey(tea.KeyMsg{Type: tea.KeyCtrlB})
	d := m.forkDialog
	if d == nil || d.id != "selected" || calls.Load() != 0 {
		t.Fatal("fork bypassed confirmation or chose wrong source")
	}
	m.forkKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	if d.attempted {
		t.Fatal("literal key name confirmed fork")
	}
	cmd, _ := m.forkKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !d.busy || m.session.ID != "current" {
		t.Fatal("fork should wait for server")
	}
	if duplicate, _ := m.forkKey(tea.KeyMsg{Type: tea.KeyEnter}); duplicate != nil {
		t.Fatal("duplicate fork submitted")
	}
	result := cmd().(sessionForkResult)
	// A background poll may finish after the fork request. Queue connection by the
	// acknowledged child ID rather than repeating the mutation or changing its source.
	m.sessionBusy = true
	m.sessionAction = "refresh"
	m.handleSessionFork(result)
	if calls.Load() != 1 || m.forkDialog != nil || len(m.sessionQueue) != 1 || m.sessionQueue[0] != (sessionAction{action: "connect", value: "child"}) {
		t.Fatal("fork did not queue exactly one child connection")
	}
	if m.page != chatPage || m.input.Value() != "preserved draft" || !m.chatSessionChanging() || !strings.Contains(strings.Join(m.lines, "\n"), "/session child") {
		t.Fatal("draft, switching guard, or recovery identity lost")
	}
}

func TestForkUncertainResultCannotRepeatMutation(t *testing.T) {
	for _, value := range []any{nil, map[string]any{"sessionId": "source"}, map[string]any{"sessionId": ""}} {
		t.Run("invalid acknowledgement", func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/connect" {
					http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "test", Path: "/"})
					w.WriteHeader(302)
					return
				}
				var req struct {
					ID string `json:"rpcId"`
				}
				json.NewDecoder(r.Body).Decode(&req)
				json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": req.ID, "result": map[string]any{"ok": true, "value": value}})
			}))
			defer server.Close()
			m := newModel()
			m.settingsLogin = server.URL + "/?token=test"
			m.settingsContext = context.Background()
			m.session = &sessionView{ID: "source"}
			m.input.SetValue("draft")
			m.openSessionFork(true)
			cmd, _ := m.forkKey(tea.KeyMsg{Type: tea.KeyEnter})
			result := cmd().(sessionForkResult)
			m.handleSessionFork(result)
			if m.forkDialog == nil || m.forkDialog.busy || !strings.Contains(m.forkDialog.note, "не подтверждено") || m.session.ID != "source" {
				t.Fatal("unconfirmed result changed session")
			}
			if cmd, _ := m.forkKey(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
				t.Fatal("uncertain mutation retried")
			}
			m.forkKey(tea.KeyMsg{Type: tea.KeyEsc})
			if m.forkDialog != nil || !m.input.Focused() || m.input.Value() != "draft" {
				t.Fatal("close did not restore composer")
			}
			m.openSessionFork(true)
			m.handleSessionFork(result)
			if m.forkDialog == nil || m.forkDialog.attempted {
				t.Fatal("stale result affected newer dialog")
			}
		})
	}
}

func TestForkShortcutRespectsOverlaysAndSessionSwitch(t *testing.T) {
	m := newModel()
	m.settingsLogin = "http://unused"
	m.page = sessionsPage
	m.sessionItems = []sessionItem{{ID: "selected"}}
	for _, setup := range []func(){func() { m.historyReader = &historyReader{} }, func() { m.historyReader = nil; m.renameDialog = &sessionRename{} }, func() { m.renameDialog = nil; m.chrome.menuOpen = true }} {
		setup()
		if _, handled := m.forkKey(tea.KeyMsg{Type: tea.KeyCtrlB}); handled || m.forkDialog != nil {
			t.Fatal("fork shortcut intercepted overlay")
		}
	}
	m.chrome.menuOpen = false
	m.sessionBusy = true
	m.sessionAction = "connect"
	m.openSessionFork(false)
	if m.forkDialog != nil {
		t.Fatal("fork opened during session switch")
	}
}
