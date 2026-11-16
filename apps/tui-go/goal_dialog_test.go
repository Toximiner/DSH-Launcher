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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestGoalAbsentResultIsOptionalOnlyForGoalGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "test", Path: "/"})
			w.WriteHeader(302)
			return
		}
		var req struct {
			ID string `json:"rpcId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": req.ID, "result": map[string]any{"ok": true}})
	}))
	defer server.Close()
	api, err := openAPI(context.Background(), server.URL+"/?token=test")
	if err != nil {
		t.Fatal(err)
	}
	defer api.close()
	goal, err := readLiveGoal(context.Background(), api, "s")
	if err != nil || goal != nil {
		t.Fatal("absent goal was treated as a failed read", err)
	}
	var target map[string]any
	if api.rpc(context.Background(), "session/state", map[string]any{}, &target) == nil {
		t.Fatal("required result silently became optional")
	}
}

func goalTestDialog(t *testing.T) *model {
	t.Helper()
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	m.input.SetValue("retained draft")
	m.openGoal()
	d := m.goalDialog
	m.handleGoalDialog(goalDialogResult{dialog: d, serial: d.serial, goal: &liveGoal{ID: "g", Revision: 2, Phase: "paused", Objective: "Original objective", MaxRounds: 8, Rounds: 3, Activation: "disarmed"}})
	return m
}
func TestGoalDialogGeometryEditingPasteAndCancellation(t *testing.T) {
	m := goalTestDialog(t)
	d := m.goalDialog
	for _, size := range [][2]int{{120, 42}, {100, 30}, {80, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if !strings.Contains(m.View(), "Раунды: 3 / 8") || !strings.Contains(m.View(), "приостановлена") || !strings.Contains(m.View(), "выключено") {
			t.Fatal("goal snapshot missing")
		}
		m.editGoal()
		if lipgloss.Width(m.View()) > m.width-1 || lipgloss.Height(m.View()) > m.height {
			t.Fatal("goal editor overflow", size)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if d.draft != nil || d.current.Objective != "Original objective" || m.input.Value() != "retained draft" {
			t.Fatal("cancel changed goal or composer")
		}
	}
	m.editGoal()
	d.draft.objective.SetValue("")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("First\nПривет\n\nlast"), Paste: true})
	m.Update(tea.KeyMsg{Type: tea.KeyF5, Paste: true})
	if d.busy || d.draft.objective.Value() != "First\nПривет\n\nlast" {
		t.Fatal("paste mutated goal or lost multiline text")
	}
	before := d.draft.objective.Value()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.Repeat("x", goalObjectiveLimit)), Paste: true})
	if d.draft.objective.Value() != before {
		t.Fatal("oversized paste was truncated into a different objective")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if d.draft.field != 1 {
		t.Fatal("cap field not focused")
	}
	d.draft.cap.SetValue("0")
	if m.saveGoal() != nil || !strings.Contains(d.note, "Лимит") {
		t.Fatal("invalid cap accepted")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF3})
	if d.editing || d.draft == nil {
		t.Fatal("snapshot toggle lost draft")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF3})
	if !d.editing || d.draft.cap.Value() != "0" {
		t.Fatal("snapshot toggle did not restore draft")
	}
	m.Update(tea.WindowSizeMsg{Width: 79, Height: 24})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if d.draft == nil {
		t.Fatal("undersized window discarded a hidden draft")
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.stopChat()
	if m.goalDialog != nil {
		t.Fatal("session switch retained goal dialog")
	}
	m.handleGoalDialog(goalDialogResult{dialog: d, serial: d.serial, err: errors.New("late")})
	if m.goalDialog != nil {
		t.Fatal("late goal response resurrected a dialog")
	}
}

func TestGoalRevisionWriteConflictReviewPreservesUntouchedFields(t *testing.T) {
	m := goalTestDialog(t)
	d := m.goalDialog
	current := *d.current
	var mu sync.Mutex
	writes := 0
	reject := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "test", Path: "/"})
			w.WriteHeader(302)
			return
		}
		var request struct {
			ID      string `json:"rpcId"`
			Payload struct {
				Args struct {
					Agent string `json:"agentId"`
					Ref   struct {
						ID       string `json:"id"`
						Revision int    `json:"revision"`
					} `json:"ref"`
					Request map[string]json.RawMessage `json:"request"`
				} `json:"args"`
			} `json:"payload"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		mu.Lock()
		defer mu.Unlock()
		ok := true
		switch r.URL.Path {
		case "/api/goals/get":
		case "/api/goals/edit":
			writes++
			if request.Payload.Args.Agent != "s" || request.Payload.Args.Ref.ID != "g" {
				t.Error("goal identity missing")
			}
			if _, exists := request.Payload.Args.Request["maxGoalRounds"]; exists {
				t.Error("untouched cap was rewritten")
			}
			if request.Payload.Args.Ref.Revision != current.Revision || reject {
				ok = false
			} else {
				_ = json.Unmarshal(request.Payload.Args.Request["objective"], &current.Objective)
				current.Revision++
			}
		default:
			t.Error("unexpected goal endpoint", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": request.ID, "result": map[string]any{"ok": ok, "value": current, "error": map[string]any{"message": "PRIVATE KEY IN BACKEND ERROR"}}})
	}))
	defer server.Close()
	m.settingsLogin = server.URL + "/?token=test"
	m.editGoal()
	d.draft.objective.SetValue("New\nUnicode: привет")
	mu.Lock()
	current.Revision++
	current.MaxRounds = 13
	mu.Unlock()
	cmd := m.saveGoal()
	if cmd == nil || m.saveGoal() != nil {
		t.Fatal("goal writes not serialized")
	}
	m.handleGoalDialog(cmd().(goalDialogResult))
	if !d.blocked || m.saveGoal() != nil || d.draft == nil || strings.Contains(m.View(), "PRIVATE") {
		t.Fatal("failed write retried, lost draft or exposed error")
	}
	m.handleGoalDialog(m.loadGoal()().(goalDialogResult))
	if !d.conflict || d.editing || d.current.MaxRounds != 13 || d.draft.cap.Value() != "8" {
		t.Fatal("reread silently rebased a draft")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	if d.conflict || !d.editing || d.draft.cap.Value() != "13" || d.draft.objective.Value() != "New\nUnicode: привет" {
		t.Fatal("explicit review lost foreign untouched field or draft")
	}
	m.handleGoalDialog(m.saveGoal()().(goalDialogResult))
	if d.editing || d.draft != nil || d.current.Phase != "paused" || d.current.MaxRounds != 13 || d.current.Objective != "New\nUnicode: привет" {
		t.Fatal("edit was not acknowledged or changed goal phase")
	}
	m.editGoal()
	if m.saveGoal() != nil || d.draft != nil {
		t.Fatal("no-op goal edit wrote to server")
	}
	mu.Lock()
	if writes != 2 {
		t.Error("unexpected automatic retries", writes)
	}
	mu.Unlock()
}

func TestGoalDifferentIdentityAndFailedRereadDoNotAllowRebase(t *testing.T) {
	m := goalTestDialog(t)
	d := m.goalDialog
	m.editGoal()
	d.draft.objective.SetValue("kept")
	m.handleGoalDialog(goalDialogResult{dialog: d, serial: d.serial, err: errors.New("read failed")})
	if !d.stale || m.saveGoal() != nil {
		t.Fatal("failed reread allowed stale write")
	}
	other := *d.current
	other.ID = "different"
	m.handleGoalDialog(goalDialogResult{dialog: d, serial: d.serial, goal: &other})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	if d.editing || d.draft.base.ID != "g" || d.draft.objective.Value() != "kept" {
		t.Fatal("draft was silently retargeted to another goal")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.editGoal()
	if d.draft == nil || d.draft.base.ID != "different" {
		t.Fatal("explicit new edit could not target the new goal")
	}
}
