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

func TestSettingsReadProtocolAndErrors(t *testing.T) {
	for _, mode := range []string{"ok", "denied", "login", "invalid", "mismatch", "missing", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path == "/connect" {
					if r.URL.Query().Get("token") != "private-token" {
						t.Error("login token missing")
					}
					if mode == "login" {
						w.WriteHeader(401)
						return
					}
					http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "test-session", Path: "/"})
					w.Header().Set("Location", "/")
					w.WriteHeader(302)
					return
				}
				if r.URL.Path != "/api/settings/describe" || r.Method != "POST" {
					t.Errorf("unexpected endpoint: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
					return
				}
				if r.Header.Get("Cookie") != "dsh=test-session" || r.Header.Get("Origin") == "" {
					t.Error("authentication headers missing")
				}
				var request struct {
					Type, Method string
					RPCID        string `json:"rpcId"`
					Payload      struct{ Args map[string]any }
				}
				json.NewDecoder(r.Body).Decode(&request)
				if request.Type != "client-request" || request.Method != "settings/describe" || request.Payload.Args == nil || len(request.Payload.Args) != 0 {
					t.Error("invalid describe envelope")
				}
				if mode == "redirect" {
					w.Header().Set("Location", "http://example.invalid/")
					w.WriteHeader(302)
					return
				}
				if mode == "invalid" {
					w.Write([]byte("not JSON private-token"))
					return
				}
				if mode == "mismatch" {
					request.RPCID = "wrong"
				}
				value := map[string]any{"namespaces": []any{map[string]any{"ns": "z-last", "value": map[string]any{}}, map[string]any{"ns": "a-first", "value": map[string]any{"model": "actual-model"}}}}
				if mode == "missing" {
					value = map[string]any{}
				}
				json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": request.RPCID, "result": map[string]any{"ok": mode != "denied", "value": value, "error": map[string]any{"message": "private-token"}}})
			}))
			defer server.Close()
			snapshot, err := readSettings(context.Background(), server.URL+"/?token=private-token")
			if mode == "ok" {
				if err != nil || len(snapshot.Namespaces) != 2 || snapshot.Namespaces[0].NS != "a-first" {
					t.Fatalf("invalid snapshot: %v", err)
				}
			} else if err == nil {
				t.Fatal("invalid response was accepted")
			}
			if err != nil && strings.Contains(err.Error(), "private-token") {
				t.Fatal("error leaked credentials")
			}
			expected := 2
			if mode == "login" {
				expected = 1
			}
			if calls != expected {
				t.Fatalf("unexpected request count: %d", calls)
			}
		})
	}
	for _, raw := range []string{"https://example.com/?token=x", "http://user:pass@localhost/?token=x", "http://localhost/path?token=x", "http://localhost/", "file:///etc/passwd"} {
		if _, err := settingsURL(raw); err == nil {
			t.Fatal("invalid login URL accepted")
		}
	}
}

func TestSettingsFieldsHideSecrets(t *testing.T) {
	var ns settingsNamespace
	json.Unmarshal([]byte(`{"ns":"test","value":{"model":"real-model","nested":{"password":"never-display"},"items":["A","B"]},"secrets":[{"path":["nested","password"],"set":true}]}`), &ns)
	text := strings.Join(settingsFields(ns), "\n")
	if strings.Contains(text, "never-display") || !strings.Contains(text, "real-model") || !strings.Contains(text, "секрет: задан") || !strings.Contains(text, "items.1") {
		t.Fatal("incorrect settings rendering")
	}
}

func TestRootSecretIsHidden(t *testing.T) {
	var ns settingsNamespace
	json.Unmarshal([]byte(`{"ns":"root","value":"never-display-root","secrets":[{"path":[],"set":true}]}`), &ns)
	text := strings.Join(settingsFields(ns), "\n")
	if strings.Contains(text, "never-display-root") || !strings.Contains(text, "секрет: задан") {
		t.Fatal("root secret was not hidden")
	}
}

func TestSettingsRefreshPreservesSnapshotAndDraft(t *testing.T) {
	m := newModel()
	m.settingsLogin = "http://localhost/?token=private-token"
	m.settingsContext = context.Background()
	m.page = dshSettingsPage
	m.input.SetValue("draft")
	snapshot := &settingsSnapshot{Namespaces: []settingsNamespace{{NS: "first", Value: map[string]any{"model": "actual-value"}}, {NS: "second", Value: map[string]any{"limit": 10}}}}
	m.Update(settingsResult{snapshot: snapshot})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.settingsIndex != 1 || !strings.Contains(clean(m.View()), "limit = 10") {
		t.Fatal("settings namespace did not change")
	}
	if cmd := m.loadSettings(); cmd == nil || !m.settingsLoading {
		t.Fatal("refresh did not start")
	}
	if cmd := m.loadSettings(); cmd != nil {
		t.Fatal("duplicate refresh started")
	}
	m.Update(settingsResult{err: context.DeadlineExceeded})
	if m.settings != snapshot || m.settingsLoading || m.settingsError == "" || !strings.Contains(clean(m.View()), "предыдущий снимок") {
		t.Fatal("failed refresh lost snapshot or error state")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.input.Value() != "draft" || m.page != 0 {
		t.Fatal("settings lost the chat draft")
	}
}
