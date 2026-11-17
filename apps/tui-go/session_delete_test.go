package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// All data belongs to this test server. A deletion must follow a fresh catalog
// and activity read, and its direct module arguments must contain only the ID.
func deleteTestServer(t *testing.T, catalog, state, receipt string) (string, *[]string) {
	t.Helper()
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "fixture", Path: "/"})
			w.WriteHeader(http.StatusFound)
			return
		}
		var request struct {
			ID      string `json:"rpcId"`
			Method  string `json:"method"`
			Payload struct {
				Args map[string]json.RawMessage `json:"args"`
			} `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		calls = append(calls, request.Method)
		value := catalog
		switch request.Method {
		case "session/list":
			if len(request.Payload.Args) != 1 || string(request.Payload.Args["_request"]) != "{}" {
				t.Errorf("wrong list args: %+v", request.Payload.Args)
			}
		case "session/state":
			value = state
			if len(request.Payload.Args) != 1 || string(request.Payload.Args["request"]) != `{"sessionId":"target"}` {
				t.Errorf("wrong state args: %+v", request.Payload.Args)
			}
		case "launcherSessions/deleteSession":
			value = receipt
			if len(request.Payload.Args) != 1 || string(request.Payload.Args["sessionId"]) != `"target"` {
				t.Errorf("wrong direct delete args: %+v", request.Payload.Args)
			}
			if receipt == "unknown-method" {
				json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": request.ID, "result": map[string]any{"ok": false, "error": map[string]any{"code": "method/not-found"}}})
				return
			}
			if receipt == "uncertain" {
				w.Write([]byte(`{"type":"server-response"`))
				return
			}
		default:
			t.Errorf("unexpected RPC %s", request.Method)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": request.ID, "result": map[string]any{"ok": true, "value": json.RawMessage(value)}})
	}))
	t.Cleanup(server.Close)
	return server.URL + "/?token=fixture", &calls
}

func deleteTestDialog(t *testing.T, login string) *model {
	t.Helper()
	m := newModel()
	m.settingsLogin = login
	m.settingsContext = context.Background()
	m.session = &sessionView{ID: "target"}
	m.input.SetValue("retained draft")
	m.openSessionDelete(true)
	if m.archiveDialog == nil || !m.archiveDialog.deleting || m.archiveDialog.confirmDelete {
		t.Fatal("delete did not open with cancellation selected")
	}
	return m
}

func TestSessionDeleteDefaultCancelAndPasteNeverWrite(t *testing.T) {
	login, calls := deleteTestServer(t, `{"items":[{"sessionId":"target"}]}`, `{"running":false,"queue":[],"pendingInput":[]}`, `{"deleted":true,"sessionIds":["target"]}`)
	for _, msg := range []tea.KeyMsg{{Type: tea.KeyEnter, Paste: true}, {Type: tea.KeyTab, Paste: true}, {Type: tea.KeyRunes, Runes: []rune("delete\n"), Paste: true}} {
		m := deleteTestDialog(t, login)
		cmd, handled := m.archiveKey(msg)
		if !handled || cmd != nil || m.archiveDialog == nil || m.archiveDialog.confirmDelete {
			t.Fatal("paste changed confirmation or submitted")
		}
	}
	m := deleteTestDialog(t, login)
	cmd, _ := m.archiveKey(tea.KeyMsg{Type: tea.KeyEnter})
	// A focus command may be returned by cancellation; it must perform no RPC.
	if cmd != nil {
		cmd()
	}
	if m.archiveDialog != nil || m.input.Value() != "retained draft" || !m.input.Focused() {
		t.Fatal("default Enter failed to cancel and restore draft")
	}
	if len(*calls) != 0 {
		t.Fatal("default cancellation or pasted input reached backend")
	}
}

func TestSessionDeleteConfirmedTransportAndUncertainReceipts(t *testing.T) {
	for _, receipt := range []string{`{"deleted":true,"sessionIds":["target"]}`, `{}`, `{"deleted":null}`, `{"deleted":false}`, `{"deleted":true}`, `{"deleted":true,"sessionIds":[]}`, `{"deleted":true,"sessionIds":["other"]}`, `{"deleted":true,"sessionIds":["target","target"]}`, `{"deleted":false,"reason":"active","sessionIds":[]}`, `{"deleted":false,"reason":"unsupported","sessionIds":[]}`, `{"deleted":false,"reason":"missing","sessionIds":[]}`, "unknown-method", "uncertain"} {
		t.Run(receipt, func(t *testing.T) {
			login, calls := deleteTestServer(t, `{"items":[{"sessionId":"other"},{"sessionId":"target"}]}`, `{"running":false,"queue":[],"pendingInput":[]}`, receipt)
			m := deleteTestDialog(t, login)
			d := m.archiveDialog
			m.archiveKey(tea.KeyMsg{Type: tea.KeyTab})
			cmd, handled := m.archiveKey(tea.KeyMsg{Type: tea.KeyEnter})
			if !handled || cmd == nil || !d.busy || !d.attempted {
				t.Fatal("explicit confirmation did not start one attempt")
			}
			if repeated, _ := m.archiveKey(tea.KeyMsg{Type: tea.KeyEnter}); repeated != nil {
				t.Fatal("duplicate submit while busy")
			}
			result := cmd().(sessionArchiveResult)
			if (result.err == nil) != (receipt == `{"deleted":true,"sessionIds":["target"]}`) {
				t.Fatalf("receipt classification wrong: %v", result.err)
			}
			if !reflect.DeepEqual(*calls, []string{"session/list", "session/state", "launcherSessions/deleteSession"}) {
				t.Fatalf("wrong request sequence: %v", *calls)
			}
			if result.err != nil {
				m.handleSessionArchive(result)
				if m.archiveDialog != d || d.busy || !d.attempted || d.note == "" {
					t.Fatal("uncertain response discarded confirmation state")
				}
				if repeated, _ := m.archiveKey(tea.KeyMsg{Type: tea.KeyEnter}); repeated != nil {
					t.Fatal("uncertain delete automatically retried")
				}
				if len(*calls) != 3 {
					t.Fatal("failed receipt caused extra request")
				}
			}
		})
	}
}

func TestSessionDeleteRequiresFreshPresenceAndInactiveState(t *testing.T) {
	for _, tc := range []struct {
		name, catalog, state string
		count                int
	}{
		{"absent", `{"items":[{"sessionId":"other"}]}`, `{}`, 1},
		{"running", `{"items":[{"sessionId":"target"}]}`, `{"running":true,"queue":[],"pendingInput":[]}`, 2},
		{"queued", `{"items":[{"sessionId":"target"}]}`, `{"running":false,"queue":[{}],"pendingInput":[]}`, 2},
		{"pending", `{"items":[{"sessionId":"target"}]}`, `{"running":false,"queue":[],"pendingInput":["request"]}`, 2},
		{"missing-state", `{"items":[{"sessionId":"target"}]}`, `null`, 2},
		{"missing-running", `{"items":[{"sessionId":"target"}]}`, `{"queue":[],"pendingInput":[]}`, 2},
		{"missing-queue", `{"items":[{"sessionId":"target"}]}`, `{"running":false}`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			login, calls := deleteTestServer(t, tc.catalog, tc.state, `{"deleted":true,"sessionIds":["target"]}`)
			api, err := openAPI(context.Background(), login)
			if err != nil {
				t.Fatal(err)
			}
			defer api.close()
			if _, err := deleteSessionPermanently(context.Background(), api, "target"); err == nil {
				t.Fatal("unsafe target accepted")
			}
			if len(*calls) != tc.count {
				t.Fatalf("unsafe target advanced transport: %v", *calls)
			}
			for _, method := range *calls {
				if method == "launcherSessions/deleteSession" {
					t.Fatal("unsafe state was deleted")
				}
			}
		})
	}
}

func TestSessionDeleteConfirmedCleanupAndLateResultIsolation(t *testing.T) {
	m := chatModel(t)
	m.session.ID = "other"
	m.chat.id = "other"
	m.input.SetValue("other draft")
	m.tabState = sessionTabsState{items: []sessionItem{{ID: "target"}, {ID: "other"}}, drafts: map[string]string{"target": "deleted draft", "other": "other draft"}}
	m.sessionItems = []sessionItem{{ID: "target"}, {ID: "other"}}
	m.side.login = m.settingsLogin
	m.side.sessions = append([]sessionItem{}, m.sessionItems...)
	m.side.workspaces = []sidebarWorkspace{{ID: "project", Sessions: []string{"target", "other"}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.archive = &archiveState{ctx: ctx, cancel: cancel, ready: true, ids: map[string]bool{"target": true, "other": true}}
	d := &sessionArchive{id: "target", deleting: true, login: m.settingsLogin, attempted: true, busy: true}
	m.archiveDialog = d
	m.prompts["target"] = &pendingPrompt{ID: "rejected", Status: "rejected"}
	m.prompts["other"] = &pendingPrompt{ID: "other-pending", Status: "unknown"}
	oldChat := m.chat
	oldSequence := m.side.sequence
	m.handleSessionArchive(sessionArchiveResult{dialog: d})
	t.Cleanup(m.archive.cancel)
	if m.prompts["target"] != nil || m.prompts["other"] == nil {
		t.Fatal("prompt cleanup affected another session or retained deleted data")
	}
	if m.archiveDialog != nil || m.session.ID != "other" || m.chat != oldChat || m.input.Value() != "other draft" {
		t.Fatal("deleting another session changed the active chat")
	}
	if _, ok := m.tabState.drafts["target"]; ok {
		t.Fatal("deleted tab draft remains")
	}
	if m.tabState.drafts["other"] != "other draft" || len(m.tabState.items) != 1 || m.tabState.items[0].ID != "other" {
		t.Fatal("tab cleanup removed unrelated tab")
	}
	if len(m.sessionItems) != 1 || m.sessionItems[0].ID != "other" || m.isSessionArchived("target") || !m.isSessionArchived("other") {
		t.Fatal("confirmed inventory cleanup incorrect")
	}
	if !reflect.DeepEqual(m.side.workspaces[0].Sessions, []string{"other"}) || m.side.sequence <= oldSequence {
		t.Fatal("sidebar cleanup or stale refresh invalidation missing")
	}
	newer := &sessionArchive{id: "other", deleting: true, login: m.settingsLogin, busy: true}
	m.archiveDialog = newer
	m.handleSessionArchive(sessionArchiveResult{dialog: d})
	if m.archiveDialog != newer || !newer.busy {
		t.Fatal("late result changed newer confirmation")
	}
	newer.login = "http://different-profile"
	m.handleSessionArchive(sessionArchiveResult{dialog: newer})
	if m.session.ID != "other" || m.tabState.drafts["other"] != "other draft" {
		t.Fatal("old-login result deleted another profile's cache")
	}
}

func TestSessionDeleteDialogGeometryAndHints(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, english := range []bool{false, true} {
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 40}} {
			m := deleteTestDialog(t, "http://unused")
			if english {
				m.prefs.Language = "en"
			}
			activeEnglish.Store(english)
			m.Update(size)
			for _, state := range []string{"initial", "busy", "failed"} {
				d := m.archiveDialog
				d.busy = state == "busy"
				d.attempted = state != "initial"
				if state == "failed" {
					d.note = strings.Repeat("problem details ", 80)
				}
				view := m.View()
				_, _, choices := m.sessionDeleteLayout()
				rendered := strings.Split(clean(view), "\n")
				for index, choice := range choices {
					label := tr("Отмена")
					if index == 1 {
						label = tr("Удалить навсегда")
					}
					if !strings.Contains(rendered[choice.y], label) {
						t.Fatal("delete mouse choice differs from rendered row")
					}
				}
				if lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height {
					t.Fatalf("delete dialog overflow English=%v size=%+v", english, size)
				}
				hint := tr("Tab / ← → выбор · Enter подтвердить · Esc отмена")
				if state == "busy" {
					hint = tr("Ожидание ответа dsh…")
				}
				if state == "failed" {
					hint = tr("Esc — закрыть и проверить список сессий")
				}
				if !strings.Contains(clean(view), hint) {
					t.Fatalf("visible delete hint clipped English=%v size=%+v state=%s", english, size, state)
				}
			}
		}
	}
}

func TestSessionDeleteCurrentTabDetachesWithoutCreatingSession(t *testing.T) {
	m := chatModel(t)
	m.tabState = sessionTabsState{items: []sessionItem{{ID: "s"}, {ID: "other"}}, drafts: map[string]string{"s": "old deleted draft", "other": "saved other", "": "new unsent draft"}}
	m.input.SetValue("current removed draft")
	oldChat := m.chat
	oldSequence := m.sessionSequence
	d := &sessionArchive{id: "s", deleting: true, login: m.settingsLogin, attempted: true}
	m.archiveDialog = d
	// Inspect state only; returned commands must remain read-only inventory refreshes.
	m.handleSessionArchive(sessionArchiveResult{dialog: d})
	t.Cleanup(m.archive.cancel)
	if m.session != nil || m.chat != nil || oldChat.ctx.Err() == nil || m.sessionSequence <= oldSequence {
		t.Fatal("deleted active session remains attached or can be reopened by old results")
	}
	if m.input.Value() != "new unsent draft" || m.tabState.drafts["other"] != "saved other" || len(m.tabState.items) != 1 || m.tabState.items[0].ID != "other" {
		t.Fatal("new-shell or unrelated draft was lost")
	}
	if _, ok := m.tabState.drafts["s"]; ok {
		t.Fatal("deleted active draft retained")
	}
	if m.sessionAction == "create" || m.sessionAction == "new" {
		t.Fatal("cleanup created a replacement session")
	}
}

func TestSessionDeleteBlocksUnresolvedLocalDelivery(t *testing.T) {
	for _, status := range []string{"sending", "unknown", "accepted", "rejected"} {
		t.Run(status, func(t *testing.T) {
			m := deleteTestDialog(t, "http://unused")
			m.prompts["target"] = &pendingPrompt{ID: "pending", Status: status, Text: "unsent"}
			m.archiveKey(tea.KeyMsg{Type: tea.KeyTab})
			cmd, _ := m.archiveKey(tea.KeyMsg{Type: tea.KeyEnter})
			d := m.archiveDialog
			if status == "rejected" {
				if cmd == nil {
					t.Fatal("rejected prompt incorrectly blocked deletion preflight")
				}
			} else if cmd != nil || !d.attempted || d.busy || !strings.Contains(d.note, tr("Дождитесь подтверждения отправленного сообщения.")) {
				t.Fatal("unresolved local send reached deletion transport")
			}
			if m.prompts["target"] == nil || m.prompts["target"].Text != "unsent" {
				t.Fatal("opening delete lost pending message")
			}
		})
	}
}

func TestBuiltinDeleteReceiptCleansDescendantTabsAndKeepsFork(t *testing.T) {
	m := deleteTestDialog(t, "http://fixture/?token=local")
	d := m.archiveDialog
	m.session = &sessionView{ID: "child"}
	m.tabState.items = []sessionItem{{ID: "target"}, {ID: "child"}, {ID: "fork"}}
	m.tabState.drafts = map[string]string{"target": "parent draft", "child": "child draft", "fork": "fork draft"}
	m.sessionItems = []sessionItem{{ID: "target"}, {ID: "child"}, {ID: "fork"}}
	m.handleSessionArchive(sessionArchiveResult{dialog: d, deletedIDs: []string{"target", "child"}})
	if m.session != nil {
		t.Fatal("deleted child remained attached")
	}
	if len(m.tabState.items) != 1 || m.tabState.items[0].ID != "fork" || m.tabState.drafts["fork"] != "fork draft" {
		t.Fatal("unrelated fork lost")
	}
	if len(m.sessionItems) != 1 || m.sessionItems[0].ID != "fork" {
		t.Fatal("descendants remained in catalog")
	}
}
