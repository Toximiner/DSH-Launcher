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

func TestRenameUsesSelectedSessionAndConfirmedTitle(t *testing.T) {
	var calls atomic.Int32
	var fail atomic.Bool
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
					Request struct {
						ID    string `json:"sessionId"`
						Title string `json:"title"`
					} `json:"request"`
				} `json:"args"`
			} `json:"payload"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		calls.Add(1)
		if req.Method != "session/rename" || req.Payload.Args.Request.ID != "selected" || req.Payload.Args.Request.Title != "  Новое   имя 🙂 " {
			t.Error("incorrect rename request", req)
		}
		value := any(map[string]any{"title": "Новое имя 🙂", "seq": 42})
		if fail.Load() {
			value = nil
		}
		json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": req.ID, "result": map[string]any{"ok": true, "value": value}})
	}))
	defer server.Close()
	m := newModel()
	m.settingsLogin = server.URL + "/?token=test"
	m.settingsContext = context.Background()
	m.session = &sessionView{ID: "current", Title: "Keep current"}
	m.sessionItems = []sessionItem{{ID: "selected"}}
	m.page = sessionsPage
	m.input.SetValue("preserved draft")
	m.renameKey(tea.KeyMsg{Type: tea.KeyF3})
	d := m.renameDialog
	d.editor.SetValue("  Новое   имя 🙂 ")
	cmd, _ := m.renameKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.sessionItems[0].Projections.Values.Title != "" {
		t.Fatal("optimistic title")
	}
	result := cmd().(sessionRenameResult)
	if calls.Load() != 1 {
		t.Fatal("rename made extra requests")
	}
	m.handleSessionRename(result)
	if m.sessionItems[0].Projections.Values.Title != "Новое имя 🙂" || m.session.Title != "Keep current" || m.input.Value() != "preserved draft" {
		t.Fatal("rename corrupted selected/current session or draft")
	}
	m.openSessionRename(false)
	d = m.renameDialog
	d.editor.SetValue("  Новое   имя 🙂 ")
	fail.Store(true)
	cmd, _ = m.renameKey(tea.KeyMsg{Type: tea.KeyEnter})
	m.handleSessionRename(cmd().(sessionRenameResult))
	if m.renameDialog != d || d.editor.Value() != "  Новое   имя 🙂 " || !strings.Contains(d.note, "Не удалось") {
		t.Fatal("unconfirmed rename lost draft or showed success")
	}
	m.renameKey(tea.KeyMsg{Type: tea.KeyEsc})
	m.openSessionRename(false)
	m.handleSessionRename(result)
	if m.renameDialog == nil {
		t.Fatal("stale result closed a newer editor")
	}
}

func TestRenameEditorLiteralKeysAndCancellation(t *testing.T) {
	m := newModel()
	m.settingsLogin = "http://unused"
	m.session = &sessionView{ID: "current", Title: "Old"}
	m.input.SetValue("draft")
	m.openSessionRename(true)
	d := m.renameDialog
	d.editor.SetValue("")
	m.renameKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	if d.editor.Value() != "enter" || d.busy {
		t.Fatal("literal key name was executed")
	}
	d.editor.SetValue(" \t ")
	cmd, _ := m.renameKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || d.busy {
		t.Fatal("empty title submitted")
	}
	m.renameKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.renameDialog != nil || m.input.Value() != "draft" || !m.input.Focused() {
		t.Fatal("cancel lost composer state")
	}
	m.browserPreview = true
	if m.openSessionRename(true) != nil || m.renameDialog != nil {
		t.Fatal("browser demo initiated rename")
	}
}

func TestRenameDoesNotStealHistoryOrChromeShortcut(t *testing.T) {
	m := newModel()
	m.settingsLogin = "http://unused"
	m.page = sessionsPage
	m.sessionItems = []sessionItem{{ID: "selected"}}
	m.historyReader = &historyReader{}
	if _, handled := m.renameKey(tea.KeyMsg{Type: tea.KeyF3}); handled || m.renameDialog != nil {
		t.Fatal("rename intercepted history search")
	}
	m.historyReader = nil
	m.chrome.menuOpen = true
	if _, handled := m.renameKey(tea.KeyMsg{Type: tea.KeyF3}); handled || m.renameDialog != nil {
		t.Fatal("rename intercepted menu input")
	}
}

func TestOldPollCannotOverwriteConfirmedRename(t *testing.T) {
	m := newModel()
	m.browserPreview = true
	m.session = &sessionView{ID: "current", Title: "Confirmed"}
	m.titleRevision = 1
	m.sessionItems = []sessionItem{{ID: "current"}}
	m.sessionItems[0].Projections.Values.Title = "Confirmed"
	m.Update(sessionResult{sequence: m.sessionSequence, view: &sessionView{ID: "current", Title: "Old"}})
	if m.session.Title != "Confirmed" {
		t.Fatal("old poll replaced confirmed title")
	}
	m.Update(sessionResult{sequence: m.sessionSequence, list: true, items: []sessionItem{{ID: "current"}}})
	if m.sessionItems[0].Projections.Values.Title != "Confirmed" {
		t.Fatal("old list replaced confirmed title")
	}
}
