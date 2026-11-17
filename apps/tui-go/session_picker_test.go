package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func pickerFixture(t *testing.T) *model {
	t.Helper()
	m := chatModel(t)
	first := sessionItem{ID: "s"}
	first.Projections.Values.Title = "Current session"
	second := sessionItem{ID: "other"}
	second.Projections.Values.Title = "Build 世界"
	m.side.login = m.settingsLogin
	m.side.sessions = []sessionItem{first, second}
	m.side.workspaces = []sidebarWorkspace{{ID: "w1", Title: "Project One", Path: "/tmp/alpha", Sessions: []string{"s"}}, {ID: "w2", Title: "Project Two", Path: "/tmp/beta世界", Sessions: []string{"other"}}}
	m.input.SetValue("preserved draft 世界")
	m.openSessionPicker()
	return m
}

func TestSessionPickerSearchGroupsAndWorkspaceSelectionDoNotCreate(t *testing.T) {
	m := pickerFixture(t)
	defer m.closeSessionPicker()
	d := m.sessionPicker
	for query, want := range map[string]string{"Build": "other", "other": "other", "beta世界": "other", "Current": "s"} {
		d.search.SetValue(query)
		entries := m.sessionPickerEntries()
		found := false
		for _, e := range entries {
			if e.kind == "session" && e.id == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("search %q did not match title/id/workspace path", query)
		}
	}
	d.search.SetValue("")
	d.group = "workspaces"
	d.index = 1
	if cmd := m.sessionPickerChoose(false); cmd != nil || m.sessionBusy {
		t.Fatal("selecting a workspace created or attached a session")
	}
	entries := m.sessionPickerEntries()
	if m.side.activeWorkspace != "w2" || len(entries) != 1 || entries[0].id != "other" {
		t.Fatal("workspace selection does not filter associated sessions")
	}
	if m.input.Value() != "preserved draft 世界" {
		t.Fatal("workspace selection changed draft")
	}
	m.sessionPickerKey(tea.KeyMsg{Type: tea.KeyTab})
	if d.workspaceID != "" {
		t.Fatal("group navigation cannot escape workspace subset")
	}
}

func TestSessionPickerRejectsLateResultsAndRetainsPartialSnapshots(t *testing.T) {
	m := pickerFixture(t)
	d := m.sessionPicker
	old := &sessionPicker{}
	m.handleSessionPickerResult(sessionPickerResult{owner: old, sequence: d.sequence, login: d.login, sessionsOK: true})
	if len(d.sessions) != 2 || !d.busy {
		t.Fatal("result from another picker replaced active data")
	}
	m.handleSessionPickerResult(sessionPickerResult{owner: d, sequence: d.sequence - 1, login: d.login, sessionsOK: true})
	if len(d.sessions) != 2 {
		t.Fatal("old refresh replaced newer snapshot")
	}
	m.titleRevision = 3
	fresh := sessionItem{ID: "s"}
	fresh.Projections.Values.Title = "stale rename"
	m.handleSessionPickerResult(sessionPickerResult{owner: d, sequence: d.sequence, login: d.login, sessionsOK: true, sessions: []sessionItem{fresh}, workspacesErr: errors.New("fixture failure")})
	if d.busy || len(d.workspaces) != 2 || len(d.sessions) != 1 || d.sessions[0].Projections.Values.Title != "Current session" || d.note == "" {
		t.Fatal("partial refresh discarded usable data or overwrote confirmed title")
	}
	m.closeSessionPicker()
	m.handleSessionPickerResult(sessionPickerResult{owner: d, sequence: d.sequence, login: d.login, sessionsOK: true})
	if m.sessionPicker != nil {
		t.Fatal("late completion reopened closed picker")
	}
}

func TestSessionPickerMeasuredMouseAndModalKeys(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, language := range []string{"", "en"} {
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 40}} {
			m := pickerFixture(t)
			m.prefs.Language = language
			m.Update(size)
			d := m.sessionPicker
			view := m.sessionPickerView(strings.Repeat(strings.Repeat("x", size.Width-1)+"\n", size.Height-1))
			if lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height {
				t.Fatal("picker overflowed terminal")
			}
			g := m.chooserLayout(m.sessionPickerSpec())
			lines := strings.Split(ansi.Strip(view), "\n")
			if !strings.Contains(lines[g.List.y], tr("Сессии")) || !strings.Contains(lines[g.List.y+1], "Current session") {
				t.Fatal("measured list does not match rendered group and first row")
			}
			for _, key := range []tea.KeyMsg{{Type: tea.KeyF7}, {Type: tea.KeyF4}, {Type: tea.KeyCtrlP}} {
				if _, handled := m.sessionPickerKey(key); !handled || m.sessionPicker != d {
					t.Fatal("modal key leaked to an underlying layer")
				}
			}
			m.sessionPickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
			if d.search.Value() != "enter" || m.sessionPicker != d || m.sessionBusy {
				t.Fatal("literal multi-rune input executed a named control")
			}
			d.search.SetValue("")
			_, handled := m.sessionPickerMouse(tea.MouseMsg{X: g.List.x, Y: g.List.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if !handled || m.sessionPicker != d {
				t.Fatal("group heading activated an entry")
			}
			_, handled = m.sessionPickerMouse(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if !handled || m.sessionPicker != d {
				t.Fatal("background click escaped modal")
			}
			m.sessionPickerMouse(tea.MouseMsg{X: g.List.x, Y: g.List.y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if m.sessionPicker != nil || m.sessionBusy || !m.input.Focused() || m.input.Value() != "preserved draft 世界" {
				t.Fatal("current-session click reattached or lost input state")
			}
		}
	}
}

func TestSessionPickerExplicitCreateAndManagerRemainSeparate(t *testing.T) {
	m := pickerFixture(t)
	d := m.sessionPicker
	d.group = "workspaces"
	d.index = 0
	if cmd := m.sessionPickerChoose(true); cmd == nil || m.sessionAction != "new" || m.sessionPicker != nil {
		t.Fatal("explicit workspace creation did not use session operation")
	}
	if m.input.Value() != "preserved draft 世界" {
		t.Fatal("explicit create discarded previous draft before switching")
	}
	m2 := pickerFixture(t)
	m2.sessionPicker.workspaceID = "w1"
	m2.sessionPicker.group = "sessions"
	cmd, handled := m2.sessionPickerKey(tea.KeyMsg{Type: tea.KeyF2})
	if !handled || cmd == nil || m2.sessionPicker != nil || m2.page != sessionsPage || m2.sessionAction != "list" {
		t.Fatal("manager shortcut did not restore full management page")
	}
}

func TestSessionPickerClosedContextAndLoginGuard(t *testing.T) {
	m := pickerFixture(t)
	d := m.sessionPicker
	oldLogin := d.login
	m.settingsLogin = "http://different.invalid/"
	if cmd := m.sessionPickerChoose(false); cmd != nil || m.sessionPicker != d {
		t.Fatal("old-profile entry was activated after profile change")
	}
	m.handleSessionPickerResult(sessionPickerResult{owner: d, sequence: d.sequence, login: oldLogin, sessionsOK: true})
	if len(d.sessions) != 2 {
		t.Fatal("old-profile result replaced data after profile change")
	}
	m.loadSessionPicker()
	if d.login != m.settingsLogin || len(d.sessions) != 0 || len(d.workspaces) != 0 {
		t.Fatal("refresh leaked previous profile snapshots")
	}
	m.closeSessionPicker()
	if !errors.Is(d.ctx.Err(), context.Canceled) {
		t.Fatal("closing picker did not cancel its readers")
	}
}

func TestSessionPickerReadsOnlyCatalogsAndFinishesHTTPBeforeWorkspaceStream(t *testing.T) {
	var streamed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/connect":
			http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "fixture", Path: "/"})
			w.WriteHeader(http.StatusFound)
		case "/api/session/list":
			if streamed.Load() {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var request struct {
				ID string `json:"rpcId"`
			}
			json.NewDecoder(r.Body).Decode(&request)
			json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": request.ID, "result": map[string]any{"ok": true, "value": map[string]any{"items": []sessionItem{{ID: "fixture-session"}}}}})
		case "/api/remote.mux":
			streamed.Store(true)
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			var request struct {
				ID     string `json:"streamId"`
				Method string `json:"endpoint"`
			}
			if err := conn.ReadJSON(&request); err != nil {
				t.Error(err)
				return
			}
			if request.Method != "workspace/follow" {
				t.Errorf("unexpected stream method: %s", request.Method)
			}
			conn.WriteJSON(map[string]any{"type": "item", "streamId": request.ID, "value": map[string]any{"type": "baseline", "value": map[string]any{"items": []sidebarWorkspace{{ID: "fixture-workspace", Path: "/tmp/fixture", Sessions: []string{"fixture-session"}}}}}})
		default:
			t.Errorf("picker used unexpected endpoint: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	m := newModel()
	m.settingsLogin = server.URL + "/?token=fixture"
	m.openSessionPicker()
	defer m.closeSessionPicker()
	m.sessionPicker.busy = false
	result := m.loadSessionPicker()().(sessionPickerResult)
	if !result.sessionsOK || !result.workspacesOK || result.sessionsErr != nil || result.workspacesErr != nil {
		t.Fatalf("catalog read failed: %v / %v", result.sessionsErr, result.workspacesErr)
	}
	m.handleSessionPickerResult(result)
	if len(m.sessionPicker.sessions) != 1 || len(m.sessionPicker.workspaces) != 1 || !streamed.Load() {
		t.Fatal("read snapshots were not applied")
	}
}

func TestSessionPickerWorkspaceManagerCancelsSnapshotBeforeOpening(t *testing.T) {
	m := pickerFixture(t)
	d := m.sessionPicker
	m.chat.ready = true
	_, handled := m.sessionPickerKey(tea.KeyMsg{Type: tea.KeyF3})
	if !handled || m.sessionPicker != nil || !errors.Is(d.ctx.Err(), context.Canceled) || m.workbench == nil || m.workbench.kind != "workspaces" {
		t.Fatal("workspace manager did not replace and cancel picker")
	}
	if m.workbench.writing || m.sessionBusy {
		t.Fatal("opening workspace management wrote a new session or workspace")
	}
	m.closeWorkbench()
}

func TestSessionPickerFreshWorkspaceSelectionSurvivesCloseAndGlobalNew(t *testing.T) {
	fixture, server := fakeSessions(t)
	defer server.Close()
	m := newModel()
	m.settingsLogin = server.URL + "/?token=fixture"
	m.settingsContext = context.Background()
	m.openWorkspacePicker()
	d := m.sessionPicker
	d.workspaces = []sidebarWorkspace{{ID: "fresh-choice", Title: "Fresh workspace", Sessions: []string{"s"}}}
	d.busy = false
	m.sessionPickerChoose(false)
	m.closeSessionPicker()
	if m.sidebarNewWorkspace() != "fresh-choice" {
		t.Fatal("closing picker lost workspace absent from background cache")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	settleSession(t, m, cmd)
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.createdWorkspace != "fresh-choice" {
		t.Fatal("global New Session created in the default workspace")
	}
}

func TestSessionPickerWorkspaceBindingDoesNotMixProfileCaches(t *testing.T) {
	m := pickerFixture(t)
	defer m.closeSessionPicker()
	d := m.sessionPicker
	m.side.login = "http://previous.invalid/"
	m.side.sessions = []sessionItem{{ID: "foreign"}}
	m.bindSessionPickerWorkspace(d, "w2")
	if m.side.login != m.settingsLogin || len(m.side.sessions) != 0 || m.sidebarNewWorkspace() != "w2" {
		t.Fatal("workspace binding mixed backend profiles or lost chosen ID")
	}
}
