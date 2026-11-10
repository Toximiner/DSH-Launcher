package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func steeringModel(t *testing.T) *model {
	t.Helper()
	m := queueModel(t)
	m.session.Running = true
	return m
}
func TestQueueSteeringRequiresConfirmationRunningStateAndSameContent(t *testing.T) {
	m := steeringModel(t)
	m.input.SetValue("chat draft")
	key(m, tea.KeyF9)
	key(m, tea.KeyCtrlG)
	if m.queueDialog.confirmKind != "steer" || m.queueDialog.confirm != "item-one" || m.queueDialog.accept {
		t.Fatal("steering did not default to Back with a fixed occurrence")
	}
	key(m, tea.KeyEnter)
	if m.queueDialog.confirm != "" || !strings.Contains(m.queueDialog.note, "Уточнение отменено") {
		t.Fatal("default confirmation submitted steering")
	}
	key(m, tea.KeyCtrlG)
	m.session.Queue[0].Content[0].Text = "changed by another client"
	key(m, tea.KeyRight)
	if m.steerQueuedItem() != nil || m.queueDialog.confirm != "" || !strings.Contains(m.queueDialog.note, "изменён другим") {
		t.Fatal("steering silently sent changed content")
	}
	if m.input.Value() != "chat draft" {
		t.Fatal("steering lost the composer draft")
	}
	for _, guard := range []string{"idle", "stale", "stream", "switch", "steering", "editor", "missing"} {
		m := steeringModel(t)
		switch guard {
		case "idle":
			m.session.Running = false
		case "stale":
			m.sessionStale = true
		case "stream":
			m.chat.ready = false
		case "switch":
			m.sessionBusy = true
			m.sessionAction = "connect"
		case "steering":
			m.session.Queue[0].Placement = "steering"
		case "editor":
			m.editQueuedItem()
		}
		m.requestQueueSteering()
		if guard == "missing" {
			m.session.Queue = m.session.Queue[1:]
			m.queueDialog.accept = true
			if m.steerQueuedItem() != nil || m.queueDialog.confirm != "" {
				t.Fatal("steering targeted another item after queue admission")
			}
		} else if m.queueDialog.confirm != "" {
			t.Fatalf("steering allowed with %s state", guard)
		}
	}
	m = steeringModel(t)
	m.requestQueueSteering()
	m.queueDialog.accept = true
	m.session.Running = false
	if m.steerQueuedItem() != nil || !strings.Contains(m.queueDialog.note, "завершена") {
		t.Fatal("steering proceeded after the active turn ended")
	}
}
func TestQueueSteeringRPCAndReceiptsNeverResend(t *testing.T) {
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
					t.Error("invalid steering body")
				}
				if req.Method != "session/updateQueue" || req.Payload.Args.Request.SessionID != "s" || req.Payload.Args.Request.ItemID != "item-one" || req.Payload.Args.Request.Action.Kind != "steer" {
					t.Error("steering changed the occurrence, session, or operation")
				}
				if outcome == "uncertain" {
					w.Write([]byte("invalid response"))
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": req.ID, "result": map[string]any{"ok": outcome != "rejected", "value": map[string]bool{"accepted": true}}})
			}))
			defer server.Close()
			m := steeringModel(t)
			m.settingsLogin = server.URL + "/?token=test"
			m.requestQueueSteering()
			m.queueDialog.accept = true
			cmd := m.steerQueuedItem()
			if cmd == nil || !m.queueDialog.busy || m.steerQueuedItem() != nil {
				t.Fatal("steering was not issued exactly once")
			}
			result := cmd().(queueSteerResult)
			old := result
			old.epoch++
			m.handleQueueSteering(old)
			if !m.queueDialog.busy {
				t.Fatal("late steering result modified the current session")
			}
			m.sessionBusy = true
			if m.handleQueueSteering(result) != nil || calls != 1 || m.queueDialog.busy || m.queueDialog.confirm != "" {
				t.Fatal("steering retried or left an armed confirmation")
			}
			if m.session.Queue[0].Placement != "queued" {
				t.Fatal("steering changed cached authoritative state optimistically")
			}
			if outcome == "accepted" && !strings.Contains(m.queueDialog.note, "следующем шаге") {
				t.Fatal("receipt claimed immediate delivery")
			}
			if outcome == "uncertain" && !strings.Contains(m.queueDialog.note, "автоповтора нет") {
				t.Fatal("uncertain steering was not labelled")
			}
		})
	}
}
func TestQueueSteeringLayoutMouseAndLiteralInput(t *testing.T) {
	for _, size := range [][2]int{{120, 42}, {80, 24}, {44, 18}} {
		m := steeringModel(t)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		key(m, tea.KeyF9)
		box := m.queueBox()
		lines := strings.Split(clean(m.View()), "\n")
		if !strings.Contains(lines[box.y+box.height-4], "Уточнить Ctrl+G") {
			t.Fatal("steering mouse row does not match rendering")
		}
		m.queueKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ctrl+g"), Paste: true})
		if m.queueDialog.confirm != "" {
			t.Fatal("pasted text triggered steering")
		}
		m.queueMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + box.height - 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		view := m.View()
		if !strings.Contains(clean(view), "Уточнить текущую задачу?") || lipgloss.Height(view) > m.height || lipgloss.Width(view) > m.width-1 {
			t.Fatalf("steering confirmation overflow or wrong title at %v", size)
		}
		m.queueMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + box.height - 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.queueDialog.confirm != "" {
			t.Fatal("mouse Back submitted steering")
		}
	}
}
