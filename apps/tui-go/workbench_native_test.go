//go:build !js

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
)

func TestWorkbenchJobOutputSnapshotUsesValidatedByteCursor(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "test", Path: "/"})
			w.WriteHeader(302)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var open struct {
			ID       string `json:"streamId"`
			Endpoint string `json:"endpoint"`
			Payload  struct {
				Args struct {
					Request map[string]any `json:"request"`
				} `json:"args"`
			} `json:"payload"`
		}
		if conn.ReadJSON(&open) != nil {
			return
		}
		if open.Endpoint != "job/follow" || open.Payload.Args.Request["jobId"] != "j" || open.Payload.Args.Request["sessionId"] != "s" {
			t.Error("unexpected output subscription")
			return
		}
		send := func(value any) {
			_ = conn.WriteJSON(map[string]any{"type": "item", "streamId": open.ID, "value": value})
		}
		send(map[string]any{"type": "opened", "from": 0, "job": map[string]any{"id": "j", "owner": "s", "output": map[string]any{"earliest": 0, "total": len("Привет")}}})
		send(map[string]any{"type": "output", "next": len("Привет"), "chunks": []any{map[string]any{"at": 0, "text": "Привет"}}})
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	api, err := openAPI(ctx, server.URL+"/?token=test")
	if err != nil {
		t.Fatal(err)
	}
	defer api.close()
	output, err := readBenchJob(ctx, api, "s", "j")
	if err != nil || !strings.Contains(output, "Привет") {
		t.Fatal("snapshot lost validated UTF-8 output", err, output)
	}
}
func TestWorkbenchPackageResultReportsRollbackWithoutPrivateDiagnostic(t *testing.T) {
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
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": req.ID, "result": map[string]any{"ok": true, "value": map[string]any{"result": map[string]any{"application": "failed", "error": map[string]any{"code": "INSTALL_FAILED", "diagnostic": "secret-key"}}, "restore": map[string]any{"application": "applied"}}}})
	}))
	defer server.Close()
	m := benchTestModel(t, "packages")
	d := m.workbench
	m.settingsLogin = server.URL + "/?token=test"
	d.rows[0].Data = map[string]any{"updatable": true}
	m.benchPrepare(tea.KeyF3)
	d.editor.SetValue("1.2.3")
	m.benchKey(tea.KeyMsg{Type: tea.KeyF5})
	result := m.benchExecute()().(benchResult)
	m.handleBench(result)
	if !strings.Contains(d.lastResult, "откат: applied") || strings.Contains(d.lastResult, "secret-key") || !d.blocked {
		t.Fatal("rollback or private diagnostic mishandled", d.lastResult)
	}
}
