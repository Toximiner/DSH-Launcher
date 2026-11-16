package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPackageCancellationUsesRequestIDWithoutReplayingInstall(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "fixture", Path: "/"})
			w.WriteHeader(302)
			return
		}
		if r.URL.Path != "/api/pluginManager/cancelInstall" {
			t.Errorf("unexpected package mutation: %s", r.URL.Path)
		}
		var req struct {
			ID      string `json:"rpcId"`
			Payload struct {
				Args map[string]any `json:"args"`
			} `json:"payload"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Payload.Args["requestId"] != "active-request" {
			t.Error("wrong install request cancelled")
		}
		writes++
		json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": req.ID, "result": map[string]any{"ok": true, "value": map[string]any{"status": "cancelled"}}})
	}))
	defer server.Close()
	m := benchTestModel(t, "packages")
	d := m.workbench
	m.settingsLogin = server.URL + "/?token=fixture"
	if m.cancelPackageOperation() != nil {
		t.Fatal("idle panel issued cancellation")
	}
	d.busy = true
	d.writing = true
	d.packageRequestID = "active-request"
	cmd := m.cancelPackageOperation()
	if cmd == nil || m.cancelPackageOperation() != nil {
		t.Fatal("cancellation not restricted to one explicit request")
	}
	result := cmd().(packageCancelResult)
	m.handlePackageCancel(result)
	if writes != 1 || !d.busy {
		t.Fatal("cancellation replaced the authoritative install result")
	}
	m.handleBench(benchResult{dialog: d, serial: d.serial, action: true, feedback: "final result"})
	m.handlePackageCancel(result)
	if d.busy || d.lastResult != "final result" {
		t.Fatal("late cancellation overwrote completed install")
	}
}
