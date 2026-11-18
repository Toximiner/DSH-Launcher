package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAccountSignOutChecksRunningWork(t *testing.T) {
	for _, running := range []bool{true, false} {
		writes := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/connect" {
				http.SetCookie(w, &http.Cookie{Name: "auth", Value: "test", Path: "/"})
				w.WriteHeader(302)
				return
			}
			var request struct {
				ID     string `json:"rpcId"`
				Method string `json:"method"`
			}
			json.NewDecoder(r.Body).Decode(&request)
			var value any = map[string]any{"status": "signed-out", "attempt": nil}
			if request.Method == "account/hasRunningAccountTasks" {
				value = running
			} else if request.Method == "account/signOut" {
				writes++
			}
			json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": request.ID, "result": map[string]any{"ok": true, "value": value}})
		}))
		m := benchTestModel(t, "account")
		m.settingsLogin = server.URL + "/?token=test"
		m.benchPrepare(tea.KeyCtrlX)
		if !m.workbench.confirm {
			t.Fatal("sign out requires confirmation")
		}
		result := m.benchExecute()().(benchResult)
		if running && (writes != 0 || result.err == nil) {
			t.Fatal("sign out interrupted running account work")
		}
		if !running && (writes != 1 || result.err != nil) {
			t.Fatal("confirmed sign out failed", result.err)
		}
		server.Close()
	}
}
func TestAccountLinksRejectUnsafeSchemes(t *testing.T) {
	for _, link := range []string{"javascript:alert(1)", "file:///etc/passwd", "https://user:password@example.com/login"} {
		if safeAccountURL(link) != "" {
			t.Fatal("unsafe account link exposed")
		}
	}
	if !strings.HasPrefix(safeAccountURL("https://example.com/login"), "https://") {
		t.Fatal("valid account link refused")
	}
}
