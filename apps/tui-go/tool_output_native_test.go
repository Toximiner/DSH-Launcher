//go:build !js

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestToolOutputTransportResumesWithoutConsumingOrRerunning(t *testing.T) {
	var opens atomic.Int32
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "test", Path: "/"})
			w.WriteHeader(http.StatusFound)
			return
		}
		if r.URL.Path != "/api/remote.mux" {
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			w.WriteHeader(404)
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
					Request struct {
						SessionID string `json:"sessionId"`
						JobID     string `json:"jobId"`
						From      *int64 `json:"from"`
					} `json:"request"`
				} `json:"args"`
			} `json:"payload"`
		}
		if conn.ReadJSON(&open) != nil {
			return
		}
		n := opens.Add(1)
		request := open.Payload.Args.Request
		if open.Endpoint != "job/follow" || request.SessionID != "s" || request.JobID != "j" {
			t.Error("output used a mutation or lost job/session identity")
		}
		from := int64(0)
		if n == 1 && request.From != nil {
			t.Error("fresh observation sent an invented cursor")
		}
		if n == 2 {
			if request.From == nil || *request.From != 3 {
				t.Error("reconnect did not resume original UTF-8 bytes")
			}
			from = 3
		}
		send := func(raw string) {
			if err := conn.WriteJSON(map[string]any{"type": "item", "streamId": open.ID, "value": json.RawMessage(raw)}); err != nil {
				t.Error(err)
			}
		}
		anchor := map[string]any{"type": "opened", "from": from, "job": map[string]any{"id": "j", "owner": "s", "output": map[string]any{"earliest": 0, "total": 6}}}
		raw, _ := json.Marshal(anchor)
		send(string(raw))
		if n == 1 {
			send(`{"type":"output","chunks":[{"at":0,"text":"я\n"}],"next":3}`)
			return
		}
		send(`{"type":"output","chunks":[{"at":3,"text":"end"}],"next":6}`)
		send(`{"type":"status","job":{"id":"j","owner":"s","status":"completed"}}`)
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan toolOutput, 16)
	done := make(chan struct{})
	go func() { defer close(done); followToolOutput(ctx, server.URL+"/?token=test", "s", "j", out) }()
	deadline := time.After(5 * time.Second)
	sawStale := false
	for {
		select {
		case value := <-out:
			sawStale = sawStale || value.Stale
			if value.Done {
				if value.Text != "я\nend" || !sawStale || value.Gap || opens.Load() != 2 {
					t.Fatalf("incorrect reconnect state: %+v", value)
				}
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("settled observation leaked")
				}
				return
			}
		case <-deadline:
			t.Fatal("output did not resume and settle")
		}
	}
}
