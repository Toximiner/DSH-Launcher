package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type sessionFixture struct {
	mu                         sync.Mutex
	plan, pending, delay, fail bool
	goalPhase, objective       string
	revision                   int
	calls                      []string
}

func fakeSessions(t *testing.T) (*sessionFixture, *httptest.Server) {
	t.Helper()
	state := &sessionFixture{}
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
				Args map[string]json.RawMessage `json:"args"`
			} `json:"payload"`
		}
		json.NewDecoder(r.Body).Decode(&request)
		state.mu.Lock()
		defer state.mu.Unlock()
		state.calls = append(state.calls, request.Method)
		var value any
		ok := true
		switch request.Method {
		case "session/create":
			value = map[string]any{"sessionId": "session-test"}
		case "session/list":
			value = map[string]any{"items": []any{map[string]any{"sessionId": "session-test"}}}
		case "session/state":
			value = map[string]any{"running": false}
		case "session/projections":
			var goal any
			if state.goalPhase != "" {
				goal = map[string]any{"goal": map[string]any{"id": "goal-test", "revision": state.revision, "phase": state.goalPhase, "objective": state.objective}, "roundsStarted": 0}
			}
			value = map[string]any{"kind": "sequenced", "values": map[string]any{"plan": map[string]any{"active": state.plan, "pending": state.pending}, "goal": goal}}
		case "commands/execute":
			if state.fail {
				ok = false
				break
			}
			var line string
			json.Unmarshal(request.Payload.Args["line"], &line)
			if line != "/plan" && line != "/plan off" {
				t.Error("unexpected command line", line)
			}
			if _, ok := request.Payload.Args["agentId"]; !ok {
				t.Error("missing agent scope")
			}
			if state.delay {
				state.pending = true
			} else {
				state.plan = line == "/plan"
				state.pending = false
			}
			value = map[string]any{"result": map[string]any{"kind": "success"}}
		case "goals/create":
			var args struct {
				Objective string `json:"objective"`
			}
			json.Unmarshal(request.Payload.Args["request"], &args)
			state.objective = args.Objective
			state.goalPhase = "active"
			state.revision++
		case "goals/pause", "goals/resume", "goals/clear":
			var ref struct {
				ID       string `json:"id"`
				Revision int    `json:"revision"`
			}
			json.Unmarshal(request.Payload.Args["ref"], &ref)
			if ref.ID != "goal-test" || ref.Revision != state.revision {
				ok = false
				break
			}
			state.revision++
			switch request.Method {
			case "goals/pause":
				state.goalPhase = "paused"
			case "goals/resume":
				state.goalPhase = "active"
			case "goals/clear":
				state.goalPhase = ""
			}
		default:
			t.Error("unexpected endpoint", request.Method)
			ok = false
		}
		json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": request.ID, "result": map[string]any{"ok": ok, "value": value}})
	}))
	t.Cleanup(server.Close)
	return state, server
}
func settleSession(t *testing.T, m *model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("missing session command")
	}
	m.Update(cmd())
}

func TestSessionModesAreBackendConfirmed(t *testing.T) {
	state, server := fakeSessions(t)
	m := newModel()
	m.settingsLogin = server.URL + "/?token=private"
	m.settingsContext = context.Background()
	settleSession(t, m, m.sessionOperation("new", ""))
	if m.session == nil || m.session.ID != "session-test" {
		t.Fatal("session was not attached")
	}
	cmd := m.chooseMode("plan")
	if m.session.Plan.Active {
		t.Fatal("mode changed before backend confirmation")
	}
	settleSession(t, m, cmd)
	if !m.session.Plan.Active || m.modeLabel() != "планирование" {
		t.Fatal("plan mode did not follow backend")
	}
	settleSession(t, m, m.chooseMode("default"))
	if m.session.Plan.Active {
		t.Fatal("plan mode did not turn off")
	}
	state.mu.Lock()
	before := len(state.calls)
	state.mu.Unlock()
	m.chooseMode("goal")
	state.mu.Lock()
	after := len(state.calls)
	state.mu.Unlock()
	if m.input.Value() != "/goal " || after != before {
		t.Fatal("goal selector should ask for an objective before any write")
	}
	settleSession(t, m, m.sessionOperation("goal", "Implement the task"))
	if m.modeLabel() != "цель" || m.session.Goal.Goal.Objective != "Implement the task" {
		t.Fatal("goal mode did not follow backend")
	}
	settleSession(t, m, m.chooseMode("default"))
	if m.session.Goal.Goal.Phase != "paused" || m.modeLabel() != "обычный" {
		t.Fatal("leaving goal mode did not pause the goal")
	}
	settleSession(t, m, m.sessionOperation("resume", ""))
	if m.modeLabel() != "цель" {
		t.Fatal("goal did not resume")
	}
	settleSession(t, m, m.sessionOperation("clear", ""))
	if m.session.Goal != nil {
		t.Fatal("goal did not clear")
	}
	state.mu.Lock()
	calls := append([]string{}, state.calls...)
	state.mu.Unlock()
	for _, call := range calls {
		if call == "session/prompt" {
			t.Fatal("mode control sent a model prompt")
		}
	}
}
func TestPendingAndFailedModeChangesStayHonest(t *testing.T) {
	state, server := fakeSessions(t)
	m := newModel()
	m.settingsLogin = server.URL + "/?token=private"
	m.settingsContext = context.Background()
	settleSession(t, m, m.sessionOperation("connect", "session-test"))
	settleSession(t, m, m.chooseMode("plan"))
	state.mu.Lock()
	state.delay = true
	state.mu.Unlock()
	settleSession(t, m, m.chooseMode("default"))
	if !m.session.Plan.Active || !m.session.Plan.Pending || !strings.Contains(m.modeDescription(), "ожидает") {
		t.Fatal("pending mode was presented as active")
	}
	state.mu.Lock()
	state.delay = false
	state.fail = true
	state.mu.Unlock()
	settleSession(t, m, m.chooseMode("default"))
	if m.sessionError == "" || !m.session.Plan.Active {
		t.Fatal("failed mode change replaced backend state")
	}
	sequence := m.sessionSequence
	m.Update(sessionResult{sequence: sequence - 1, view: &sessionView{ID: "stale"}})
	if m.session.ID == "stale" {
		t.Fatal("stale session response was accepted")
	}
}
func TestBrowserModeControlNeverContactsBackend(t *testing.T) {
	m := newModel()
	m.browserPreview = true
	m.settingsLogin = "http://127.0.0.1:1/?token=demo"
	if m.chooseMode("plan") != nil || m.sessionBusy {
		t.Fatal("browser demo started a backend command")
	}
	if !strings.Contains(strings.Join(m.lines, "\n"), "Браузерное демо") {
		t.Fatal("browser limitation was not explained")
	}
}

func TestModeCommandWaitsForBackgroundRead(t *testing.T) {
	_, server := fakeSessions(t)
	m := newModel()
	m.settingsLogin = server.URL + "/?token=test"
	m.settingsContext = context.Background()
	settleSession(t, m, m.sessionOperation("connect", "session-test"))
	poll := m.sessionOperation("refresh", "")
	if m.chooseMode("plan") != nil || len(m.sessionQueue) != 1 {
		t.Fatal("mode change was not queued during background read")
	}
	_, next := m.Update(poll())
	settleSession(t, m, next)
	if !m.session.Plan.Active || len(m.sessionQueue) != 0 {
		t.Fatal("queued mode change was lost")
	}
}
