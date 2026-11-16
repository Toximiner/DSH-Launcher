package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestChildSessionAddressIsResolvedFromDurableParent(t *testing.T) {
	for _, tc := range []struct {
		name, origin string
		available    bool
	}{{"parent available", "subagent", true}, {"parent missing", "subagent", false}, {"ordinary fork with inherited descriptor", "", true}} {
		available, origin := tc.available, tc.origin
		t.Run(tc.name, func(t *testing.T) {
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
				var value any
				switch r.URL.Path {
				case "/api/session/projections":
					value = map[string]any{"kind": "sequenced", "values": map[string]any{"subagent": map[string]any{"mode": "continuable"}}}
				case "/api/session/list":
					items := []any{}
					if available {
						items = append(items, map[string]any{"sessionId": "child", "origin": origin, "parentSessionId": "parent"})
					}
					value = map[string]any{"items": items}
				case "/api/session/state":
					value = map[string]any{"queue": []any{}, "running": false}
				default:
					t.Errorf("unexpected child mutation: %s", r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": req.ID, "result": map[string]any{"ok": true, "value": value}})
			}))
			defer server.Close()
			api, err := openAPI(context.Background(), server.URL+"/?token=test")
			if err != nil {
				t.Fatal(err)
			}
			defer api.close()
			view, err := readSession(context.Background(), api, "child")
			if !available {
				if err == nil {
					t.Fatal("child fell back to an ordinary session address")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if origin == "" {
				if view.Address.Kind != "session" {
					t.Fatal("inherited descriptor changed ordinary fork origin")
				}
				return
			}
			raw, _ := json.Marshal(view.Address.wire("child"))
			if string(raw) != `{"kind":"subagent","parentSessionId":"parent","childSessionId":"child","mode":"continuable"}` {
				t.Fatal("incorrect durable child address", string(raw))
			}
			if _, err = changeSessionMode(context.Background(), api, "child", "plan", ""); err == nil {
				t.Fatal("ordinary mode mutation attempted for a child")
			}
		})
	}
}
func TestChildViewerRetainsDraftAndUsesParentForCancellation(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	m.session.Address = sessionReadAddress{Kind: "subagent", ParentID: "parent", ChildID: "s", Mode: "continuable"}
	if m.submitText("retained child draft") != nil || m.input.Value() != "retained child draft" {
		t.Fatal("viewer sent or lost a child draft")
	}
	if m.openCompose() != nil || m.composer != nil {
		t.Fatal("viewer opened an ordinary sender")
	}
	for _, size := range [][2]int{{100, 30}, {80, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if lipgloss.Width(m.View()) > m.width-1 || lipgloss.Height(m.View()) > m.height {
			t.Fatal("child viewer overflowed", size)
		}
	}
	_, hint := m.composerText()
	if !strings.Contains(hint, "родителя") {
		t.Fatal("child viewer limitation absent")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "test", Path: "/"})
			w.WriteHeader(302)
			return
		}
		var req struct {
			ID      string `json:"rpcId"`
			Payload struct {
				Args map[string]any `json:"args"`
			} `json:"payload"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if r.URL.Path != "/api/subagents/interruptByParent" || req.Payload.Args["parentSessionId"] != "parent" || req.Payload.Args["childSessionId"] != "s" {
			t.Error("child stop bypassed its parent", r.URL.Path, req.Payload.Args)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": req.ID, "result": map[string]any{"ok": true, "value": map[string]any{"accepted": true}}})
	}))
	defer server.Close()
	m.settingsLogin = server.URL + "/?token=test"
	result := m.cancelChat()().(cancelResult)
	if result.err != nil {
		t.Fatal(result.err)
	}
}
