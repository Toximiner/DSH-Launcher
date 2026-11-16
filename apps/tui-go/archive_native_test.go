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

func TestArchiveFeedReconnectsWithNewBaselineAndCancels(t *testing.T) {
	var opens atomic.Int32
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "test", Path: "/"})
			w.WriteHeader(302)
			return
		}
		if r.URL.Path != "/api/remote.mux" {
			t.Error("archive observation invoked a mutation", r.URL.Path)
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
				Args map[string]any `json:"args"`
			} `json:"payload"`
		}
		if conn.ReadJSON(&open) != nil {
			return
		}
		if open.Endpoint != "workspace/follow" || len(open.Payload.Args) != 0 {
			t.Error("wrong registry subscription", open)
		}
		send := func(raw string) {
			if err := conn.WriteJSON(map[string]any{"type": "item", "streamId": open.ID, "value": json.RawMessage(raw)}); err != nil {
				t.Error(err)
			}
		}
		if opens.Add(1) == 1 {
			send(`{"type":"baseline","value":{"archivedSessionIds":["s"]}}`)
			send(`{"type":"pinned","pinnedSessionIds":[]}`)
			send(`{"type":"archived","archivedSessionIds":[]}`)
			return
		}
		send(`{"type":"baseline","value":{"archivedSessionIds":["other"]}}`)
		conn.ReadMessage()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := &archiveState{ctx: ctx, cancel: cancel, inbox: make(chan archiveUpdate, 16)}
	done := make(chan struct{})
	go func() { defer close(done); followArchive(ctx, server.URL+"/?token=test", state) }()
	next := func() archiveUpdate {
		select {
		case update := <-state.inbox:
			return update
		case <-time.After(5 * time.Second):
			t.Fatal("archive update did not arrive")
			return archiveUpdate{}
		}
	}
	first := next()
	if first.err != nil || !first.ids["s"] {
		t.Fatal("initial archive baseline missing")
	}
	second := next()
	if second.err != nil || len(second.ids) != 0 {
		t.Fatal("restore increment was not applied")
	}
	if next().err == nil {
		t.Fatal("disconnect was not labeled")
	}
	resumed := next()
	if resumed.err != nil || !resumed.ids["other"] || resumed.ids["s"] {
		t.Fatal("reconnect did not replace the old generation")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("archive feed did not cancel")
	}
}
