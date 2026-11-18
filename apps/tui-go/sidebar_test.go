package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func sidebarClick(t *testing.T, m *model, name string) {
	t.Helper()
	_, height := m.contentSize()
	for _, row := range m.sidebarRows(height) {
		if row.key == name {
			y := bodyTop + 1 + row.y
			if !strings.Contains(clean(strings.Split(m.View(), "\n")[y]), strings.TrimSpace(row.label)) {
				t.Fatalf("sidebar hit area is not aligned with %q", row.label)
			}
			mouse(m, 3, y, tea.MouseButtonLeft, tea.MouseActionPress)
			return
		}
	}
	t.Fatalf("sidebar entry %s is not visible", name)
}

func TestSidebarWorkspaceTreeAndFixedMore(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, english := range []bool{false, true} {
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}} {
			m := newModel()
			m.prefs.Language = map[bool]string{true: "en", false: ""}[english]
			activeEnglish.Store(english)
			m.Update(size)
			m.input.SetValue("unsent 世界")
			m.session = &sessionView{ID: "s", Title: "Current"}
			m.side.workspaces = []sidebarWorkspace{{ID: "w", Title: "Project", Sessions: []string{"s", "old"}}}
			m.archive = &archiveState{ids: map[string]bool{"old": true}}
			sidebarClick(t, m, "workspace:w")
			if !m.side.expanded["w"] || m.sessionBusy || m.session.ID != "s" || m.workbench != nil {
				t.Fatal("expanding a workspace invoked a backend action")
			}
			for _, item := range m.sidebarItems() {
				if item.value == "old" {
					t.Fatal("archived session remains in the tree")
				}
			}
			sidebarClick(t, m, "session:s")
			if !m.input.Focused() || m.navFocused || m.input.Value() != "unsent 世界" {
				t.Fatal("current session click lost the composer")
			}
			for i := 0; i < 35; i++ {
				m.side.workspaces = append(m.side.workspaces, sidebarWorkspace{ID: fmt.Sprint(i), Title: fmt.Sprintf("Folder %02d", i)})
			}
			key(m, tea.KeyTab)
			key(m, tea.KeyEnd)
			key(m, tea.KeyUp)
			view := m.View()
			if !strings.Contains(clean(view), "Folder 34") || lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height {
				t.Fatal("keyboard did not reveal the last folder within terminal bounds")
			}
			sidebarClick(t, m, "more")
			if m.chrome.dialog != "more" {
				t.Fatal("fixed bottom menu was lost after scrolling")
			}
			key(m, tea.KeyEnter)
			if m.chrome.dialog != "settings" {
				t.Fatal("More did not open client settings")
			}
			key(m, tea.KeyEsc)
		}
	}
}
func TestSidebarMoreAndToolsDestinations(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.input.SetValue("draft")
	sidebarClick(t, m, "navigation")
	key(m, tea.KeyEnter)
	if m.page != sessionsPage || m.chrome.dialog != "" || m.input.Value() != "draft" {
		t.Fatal("Tools did not open sessions with the draft intact")
	}
	sidebarClick(t, m, "more")
	box, _ := m.dialogBox()
	mouse(m, box.x+3, box.y+4, tea.MouseButtonLeft, tea.MouseActionPress)
	if m.page != dshSettingsPage || m.chrome.dialog != "" {
		t.Fatal("More mouse selection did not open backend settings")
	}
}
func TestSidebarRejectsStaleSnapshotsAndRetainsFailedRefresh(t *testing.T) {
	m := newModel()
	m.settingsLogin = "http://localhost/?token=current"
	m.side = sidebarState{login: m.settingsLogin, sequence: 2, busy: true, workspaces: []sidebarWorkspace{{ID: "w", Title: "Keep"}}}
	for _, r := range []sidebarResult{{login: "http://localhost/?token=old", sequence: 2}, {login: m.settingsLogin, sequence: 1}} {
		m.handleSidebarResult(r)
		if !m.side.busy || len(m.side.workspaces) != 1 {
			t.Fatal("stale snapshot replaced current backend state")
		}
	}
	m.handleSidebarResult(sidebarResult{login: m.settingsLogin, sequence: 2, err: errors.New("private server detail")})
	if m.side.busy || !m.side.failed || len(m.side.workspaces) != 1 || strings.Contains(m.sidebar(21), "private server") {
		t.Fatal("failed refresh lost the snapshot or exposed a backend error")
	}
	m.settingsLogin = ""
	m.loadSidebar()
	if len(m.side.workspaces) != 0 || m.side.busy || m.side.login != "" {
		t.Fatal("disconnect retained a different backend's tree")
	}
}
func TestSidebarNewSessionUsesSelectedWorkspace(t *testing.T) {
	fixture, server := fakeSessions(t)
	defer server.Close()
	m := newModel()
	m.settingsLogin = server.URL + "/?token=test"
	m.settingsContext = context.Background()
	m.side.workspaces = []sidebarWorkspace{{ID: "chosen", Title: "Chosen"}, {ID: "other", Title: "Other", Sessions: []string{"current"}}}
	m.session = &sessionView{ID: "current"}
	m.activateSidebar(sidebarItem{action: "workspace", value: "chosen"})
	if m.sidebarNewWorkspace() != "chosen" {
		t.Fatal("selected workspace was ignored")
	}
	settleSession(t, m, m.activateSidebar(sidebarItem{action: "new"}))
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.createdWorkspace != "chosen" {
		t.Fatal("new session request targeted a different workspace")
	}
}

func TestHiddenSidebarRestoresComposerAndIgnoresTab(t *testing.T) {
	m := newModel()
	key(m, tea.KeyTab)
	key(m, tea.KeyF4)
	selectHubSetting(t, m, "sidebar")
	key(m, tea.KeyEnter)
	key(m, tea.KeyEsc)
	if m.sidebarVisible() || m.navFocused || !m.input.Focused() {
		t.Fatal("hiding focused sidebar trapped the composer")
	}
	key(m, tea.KeyTab)
	if m.navFocused || !m.input.Focused() {
		t.Fatal("Tab focused the hidden sidebar")
	}
}
func TestRemovedSidebarSelectionCannotCreateSession(t *testing.T) {
	m := newModel()
	m.navFocused = true
	m.side.selected = "session:removed"
	cmd, _ := m.sidebarKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.sessionBusy || m.sessionAction == "new" || cmd != nil {
		t.Fatal("removed selection triggered an unrelated write")
	}
}

func TestSidebarReadsHTTPBeforeSnapshotRetiresOperator(t *testing.T) {
	var socketOpened atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/connect":
			http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "fixture", Path: "/"})
			w.WriteHeader(http.StatusFound)
		case "/api/session/list":
			if socketOpened.Load() {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var request struct {
				ID string `json:"rpcId"`
			}
			json.NewDecoder(r.Body).Decode(&request)
			json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": request.ID, "result": map[string]any{"ok": true, "value": map[string]any{"items": []any{map[string]any{"sessionId": "s", "projections": map[string]any{"values": map[string]any{"title": "Renamed session"}}}}}}})
		case "/api/remote.mux":
			socketOpened.Store(true)
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			var request struct {
				ID string `json:"streamId"`
			}
			if err := conn.ReadJSON(&request); err != nil {
				t.Error(err)
				return
			}
			conn.WriteJSON(map[string]any{"type": "item", "streamId": request.ID, "value": map[string]any{"type": "baseline", "value": map[string]any{"items": []sidebarWorkspace{{ID: "w", Title: "Workspace", Sessions: []string{"s"}}}}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	m := newModel()
	m.settingsLogin = server.URL + "/?token=test"
	r := m.loadSidebar()().(sidebarResult)
	if r.err != nil {
		t.Fatal(r.err)
	}
	m.handleSidebarResult(r)
	if !socketOpened.Load() || m.side.failed || len(m.side.workspaces) != 1 || len(m.side.sessions) != 1 || m.side.sessions[0].Projections.Values.Title != "Renamed session" {
		t.Fatal("snapshot or renamed session was lost after the operator's socket closed")
	}
}

func TestSidebarTitlesReconcileLocalAndExternalRenames(t *testing.T) {
	m := newModel()
	m.settingsLogin = "http://localhost/?token=test"
	m.side.login = m.settingsLogin
	m.side.sequence = 1
	m.titleRevision = 2
	item := sessionItem{ID: "s"}
	item.Projections.Values.Title = "Local rename"
	m.side.sessions = []sessionItem{item}
	item.Projections.Values.Title = "Old snapshot"
	m.handleSidebarResult(sidebarResult{login: m.settingsLogin, sequence: 1, titleRevision: 1, sessions: []sessionItem{item}})
	if m.side.sessions[0].Projections.Values.Title != "Local rename" {
		t.Fatal("in-flight snapshot undid a confirmed local rename")
	}
	item.Projections.Values.Title = "GUI rename"
	m.handleSidebarResult(sidebarResult{login: m.settingsLogin, sequence: 1, titleRevision: 2, sessions: []sessionItem{item}})
	if m.side.sessions[0].Projections.Values.Title != "GUI rename" {
		t.Fatal("fresh external rename was hidden by the old session list")
	}
}
