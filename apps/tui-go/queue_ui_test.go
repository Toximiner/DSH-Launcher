package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func queueModel(t *testing.T) *model {
	t.Helper()
	m := chatModel(t)
	m.chat.ready = true
	m.session.QueueKnown = true
	m.session.Queue = []queuedMessage{queueItem("one", "first pending task"), queueItem("two", "second pending task")}
	return m
}
func TestQueueRemovalRequiresExplicitConfirmationAndStableIdentity(t *testing.T) {
	m := queueModel(t)
	m.input.SetValue("preserved draft")
	key(m, tea.KeyF9)
	key(m, tea.KeyDown)
	key(m, tea.KeyEnter)
	if m.queueDialog.confirm != "item-two" || m.queueDialog.accept {
		t.Fatal("queue removal did not default to cancelling the action")
	}
	key(m, tea.KeyEnter)
	if m.queueDialog.confirm != "" || m.input.Value() != "preserved draft" {
		t.Fatal("default confirmation removed a task or lost the draft")
	}
	key(m, tea.KeyEnter)
	// A passive refresh can remove the selected item while confirmation is open.
	m.session.Queue = m.session.Queue[:1]
	view := clean(m.View())
	if !strings.Contains(view, "second pending task") {
		t.Fatal("confirmation details silently changed to another task")
	}
	key(m, tea.KeyRight)
	if cmd := m.removeQueuedItem(); cmd != nil || m.queueDialog.confirm != "" {
		t.Fatal("confirmation removed a different task after a queue update")
	}
	for _, guard := range []string{"stale", "stream", "scope", "unknown"} {
		m := queueModel(t)
		switch guard {
		case "stale":
			m.sessionStale = true
		case "stream":
			m.chat.ready = false
		case "scope":
			m.sessionBusy = true
			m.sessionAction = "connect"
		case "unknown":
			m.session.QueueKnown = false
		}
		m.requestQueueRemoval()
		if m.queueDialog.confirm != "" {
			t.Fatalf("unsafe queue mutation accepted with %s state", guard)
		}
	}
}
func TestQueueRemovalRPCAndUncertainResultDoNotRetry(t *testing.T) {
	for _, outcome := range []string{"accepted", "rejected", "uncertain"} {
		t.Run(outcome, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/connect" {
					http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "owner", Path: "/"})
					w.WriteHeader(302)
					return
				}
				calls++
				var req struct {
					ID      string `json:"rpcId"`
					Method  string `json:"method"`
					Payload struct {
						Args struct {
							Request struct {
								SessionID string `json:"sessionId"`
								ItemID    string `json:"itemId"`
								Action    struct {
									Kind string `json:"kind"`
								} `json:"action"`
							} `json:"request"`
						} `json:"args"`
					} `json:"payload"`
				}
				if json.NewDecoder(r.Body).Decode(&req) != nil {
					t.Error("invalid queue mutation body")
				}
				if req.Method != "session/updateQueue" || req.Payload.Args.Request.SessionID != "s" || req.Payload.Args.Request.ItemID != "item-one" || req.Payload.Args.Request.Action.Kind != "remove" {
					t.Error("queue mutation changed session, occurrence, or operation")
				}
				if outcome == "uncertain" {
					w.Write([]byte("invalid response"))
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": req.ID, "result": map[string]any{"ok": outcome != "rejected", "value": map[string]bool{"accepted": true}}})
			}))
			defer server.Close()
			m := queueModel(t)
			m.settingsLogin = server.URL + "/?token=test"
			m.requestQueueRemoval()
			m.queueDialog.accept = true
			cmd := m.removeQueuedItem()
			if cmd == nil || !m.queueDialog.busy {
				t.Fatal("confirmed removal did not start")
			}
			if m.removeQueuedItem() != nil {
				t.Fatal("duplicate mutation issued while receipt was pending")
			}
			result := cmd().(queueRemoveResult)
			m.sessionBusy = true // Suppress a read; this test exercises only the mutation.
			if next := m.handleQueueRemoval(result); next != nil {
				t.Fatal("mutation automatically retried")
			}
			if calls != 1 || m.queueDialog.busy || len(m.visibleQueue()) != 2 {
				t.Fatal("mutation retried or optimistically changed authoritative queue data")
			}
			if outcome == "accepted" && !strings.Contains(m.queueDialog.note, "Задание удалено") {
				t.Fatal("accepted mutation not reported")
			}
			if outcome == "uncertain" && !strings.Contains(m.queueDialog.note, "автоматического повтора нет") {
				t.Fatal("uncertain mutation not labelled")
			}
			oldNote := m.queueDialog.note
			result.epoch++
			result.err = errors.New("late result")
			m.handleQueueRemoval(result)
			if m.queueDialog.note != oldNote {
				t.Fatal("old session epoch changed the current dialog")
			}
		})
	}
}
func TestQueueDialogDimensionsMouseAndDraft(t *testing.T) {
	for _, size := range [][2]int{{120, 42}, {80, 24}, {44, 18}} {
		m := queueModel(t)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.input.SetValue("draft")
		key(m, tea.KeyF9)
		box := m.queueBox()
		view := m.View()
		if lipgloss.Height(view) > m.height || lipgloss.Width(view) > m.width-1 {
			t.Fatalf("queue dialog overflow at %v", size)
		}
		buttonY := box.y + box.height - 3
		lines := strings.Split(clean(view), "\n")
		if !strings.Contains(lines[buttonY], "Удалить выбранное") {
			t.Fatalf("queue action mouse row does not match rendering at %v", size)
		}
		m.queueMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		m.queueMouse(tea.MouseMsg{X: box.x + 3, Y: buttonY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.queueDialog.confirm != "item-two" {
			t.Fatal("mouse selected or confirmed the wrong task")
		}
		key(m, tea.KeyEsc)
		key(m, tea.KeyEsc)
		if m.queueDialog.visible || m.input.Value() != "draft" {
			t.Fatal("queue dialog did not close with the draft intact")
		}
	}
}
