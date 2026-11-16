package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestArchiveTransportConfirmsFilteredTargetAndRetainsDraft(t *testing.T) {
	for _, restore := range []bool{false, true} {
		t.Run(map[bool]string{false: "archive", true: "restore"}[restore], func(t *testing.T) {
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
				want := "workspace/archiveSession"
				ids := []string{"selected"}
				if restore {
					want = "workspace/unarchiveSession"
					ids = []string{}
				}
				args := request.Payload.Args.Request
				if request.Method != want || args["sessionId"] != "selected" {
					t.Error("wrong archive request", request)
				}
				if !restore && args["stopActivity"] != false {
					t.Error("archive requested implicit stop")
				}
				json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": request.ID, "result": map[string]any{"ok": true, "value": map[string]any{"archivedSessionIds": ids}}})
			}))
			defer server.Close()
			m := newModel()
			m.settingsLogin = server.URL + "/?token=test"
			m.settingsContext = context.Background()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m.archive = &archiveState{ctx: ctx, cancel: cancel, ids: map[string]bool{"other": !restore, "selected": restore}, ready: true}
			m.sessionItems = []sessionItem{{ID: "other"}, {ID: "selected"}}
			m.page = sessionsPage
			m.showArchived = restore
			m.input.SetValue("preserved draft")
			old := m.archive
			m.archiveKey(tea.KeyMsg{Type: tea.KeyCtrlX})
			if m.archiveDialog == nil || m.archiveDialog.id != "selected" {
				t.Fatal("action used raw list index instead of filtered target")
			}
			cmd, _ := m.archiveKey(tea.KeyMsg{Type: tea.KeyEnter})
			if m.isSessionArchived("selected") != restore {
				t.Fatal("archive state changed optimistically")
			}
			if repeated, _ := m.archiveKey(tea.KeyMsg{Type: tea.KeyEnter}); repeated != nil {
				t.Fatal("duplicate archive request")
			}
			result := cmd().(sessionArchiveResult)
			if result.err != nil {
				t.Fatal(result.err)
			}
			m.handleSessionArchive(result)
			defer m.archive.cancel()
			if m.archiveDialog != nil || m.isSessionArchived("selected") == restore || old.ctx.Err() == nil || m.input.Value() != "preserved draft" {
				t.Fatal("confirmed operation lost state or draft")
			}
			m.handleArchiveUpdate(archiveUpdate{state: old, ids: map[string]bool{"selected": restore}})
			if m.isSessionArchived("selected") == restore {
				t.Fatal("late baseline undid confirmed operation")
			}
		})
	}
}

func TestArchiveStateValidationAndDisconnectedSubmissionGuard(t *testing.T) {
	for _, raw := range []string{`{}`, `{"type":"archived","archivedSessionIds":[]}`, `{"type":"baseline","value":{}}`, `{"type":"baseline","value":{"archivedSessionIds":["x","x"]}}`} {
		if _, _, err := parseArchiveFrame(json.RawMessage(raw), false); err == nil {
			t.Fatal("invalid initial archive state accepted")
		}
	}
	ids, changed, err := parseArchiveFrame(json.RawMessage(`{"type":"baseline","value":{"archivedSessionIds":["s"]}}`), false)
	if err != nil || !changed || !ids["s"] {
		t.Fatal("valid baseline rejected")
	}
	m := chatModel(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.archive = &archiveState{ctx: ctx, cancel: cancel, ids: ids, ready: false}
	if m.submitText("do not send") != nil || m.input.Value() != "do not send" {
		t.Fatal("archived session accepted a prompt while disconnected")
	}
	if _, hint := m.composerText(); !strings.Contains(hint, "/restore") {
		t.Fatal("archived composer was not labeled")
	}
}

func TestArchiveFailureRetainsDialogWithoutRetry(t *testing.T) {
	m := newModel()
	m.settingsLogin = "http://unused"
	m.session = &sessionView{ID: "s"}
	m.input.SetValue("draft")
	m.openSessionArchive(true, false)
	d := m.archiveDialog
	d.attempted = true
	m.handleSessionArchive(sessionArchiveResult{dialog: d, err: context.DeadlineExceeded})
	if m.archiveDialog != d || d.busy || !strings.Contains(d.note, "Не подтверждено") {
		t.Fatal("uncertain archive was shown as complete")
	}
	if cmd, _ := m.archiveKey(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("uncertain archive retried")
	}
	m.archiveKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.archiveDialog != nil || m.input.Value() != "draft" || !m.input.Focused() {
		t.Fatal("cancel lost draft or focus")
	}
	m.openSessionArchive(true, false)
	m.handleSessionArchive(sessionArchiveResult{dialog: d, ids: map[string]bool{"s": true}})
	if m.archiveDialog == nil {
		t.Fatal("stale result changed a newer dialog")
	}
}
