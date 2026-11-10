package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestQueueEditorPreservesDraftAndRejectsLostOrChangedItems(t *testing.T) {
	m := queueModel(t)
	m.input.SetValue("chat draft")
	key(m, tea.KeyF9)
	key(m, tea.KeyF3)
	if m.queueDialog.edit == nil || m.queueDialog.edit.editor.Value() != "first pending task" {
		t.Fatal("editor did not load the selected occurrence")
	}
	m.queueDialog.edit.editor.SetValue("edited\nsecond line")
	key(m, tea.KeyF9)
	key(m, tea.KeyF9)
	if m.queueDialog.edit.editor.Value() != "edited\nsecond line" {
		t.Fatal("hiding the queue lost the editor draft")
	}
	m.session.Queue[0].Content[0].Text = "changed by another client"
	if m.saveQueuedEdit() != nil || !strings.Contains(m.queueDialog.note, "другим клиентом") {
		t.Fatal("edit overwrote a newer sampled value")
	}
	m.session.Queue = m.session.Queue[1:]
	if m.saveQueuedEdit() != nil || m.queueDialog.edit.editor.Value() != "edited\nsecond line" {
		t.Fatal("edit targeted another occurrence or lost the draft after admission")
	}
	key(m, tea.KeyEsc)
	if m.queueDialog.edit != nil || !m.queueDialog.visible || m.input.Value() != "chat draft" {
		t.Fatal("Escape did not return to the queue with the chat draft intact")
	}
	for _, item := range []queuedMessage{
		{ID: "image", Placement: "queued", Content: []queueContent{{Type: "image"}}},
		queueItem("large", strings.Repeat("x", queueEditLimit+1)),
		queueItem("control", "text\x1b[2J"),
		queueItem("tabs", "a\tb"),
		queueItem("lines", strings.Repeat("x\n", 10001)),
	} {
		m := queueModel(t)
		m.session.Queue = []queuedMessage{item}
		m.editQueuedItem()
		if m.queueDialog.edit != nil {
			t.Fatal("editor silently converted unsupported, oversized, or truncated source content")
		}
	}
}
func TestQueueEditRPCReceiptAndManualRecovery(t *testing.T) {
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
									Kind    string         `json:"kind"`
									Content []queueContent `json:"content"`
								} `json:"action"`
							} `json:"request"`
						} `json:"args"`
					} `json:"payload"`
				}
				json.NewDecoder(r.Body).Decode(&req)
				action := req.Payload.Args.Request.Action
				if req.Method != "session/updateQueue" || req.Payload.Args.Request.SessionID != "s" || req.Payload.Args.Request.ItemID != "item-one" || action.Kind != "edit" || len(action.Content) != 1 || action.Content[0].Text != "edited\nnext line" {
					t.Error("queue edit changed identity, text, or operation")
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
			m.editQueuedItem()
			m.queueDialog.edit.editor.SetValue("edited\nnext line")
			cmd := m.saveQueuedEdit()
			if cmd == nil || !m.queueDialog.busy || m.saveQueuedEdit() != nil {
				t.Fatal("edit did not start exactly once")
			}
			result := cmd().(queueEditResult)
			// A late response must not unlock a new session's editor.
			old := result
			old.epoch++
			m.handleQueueEdit(old)
			if !m.queueDialog.busy {
				t.Fatal("old stream epoch changed the editor")
			}
			m.sessionBusy = true
			if m.handleQueueEdit(result) != nil || calls != 1 {
				t.Fatal("edit automatically retried")
			}
			if m.session.Queue[0].Content[0].Text != "first pending task" {
				t.Fatal("edit optimistically rewrote authoritative state")
			}
			if outcome == "accepted" {
				if m.queueDialog.edit != nil {
					t.Fatal("acknowledged edit left a stale editor")
				}
				return
			}
			if m.queueDialog.edit == nil || m.queueDialog.edit.editor.Value() != "edited\nnext line" || m.saveQueuedEdit() != nil {
				t.Fatal("failed edit lost its draft or retried without a new server sample")
			}
			m.session.QueueFetchedAt = time.Now().Add(time.Second)
			m.session.Queue[0].Content = []queueContent{{Type: "text", Text: "edited\nnext line"}}
			if m.saveQueuedEdit() != nil || m.queueDialog.edit != nil || !strings.Contains(m.queueDialog.note, "уже сохранён") {
				t.Fatal("uncertain successful edit was resent after read reconciliation")
			}
		})
	}
}
func TestQueueEditorMinimumLayoutKeysAndMouse(t *testing.T) {
	for _, size := range [][2]int{{120, 42}, {80, 24}, {44, 18}} {
		m := queueModel(t)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		key(m, tea.KeyF9)
		box := m.queueBox()
		m.queueMouse(tea.MouseMsg{X: box.x + 2 + len([]rune("[ Удалить выбранное ] ")), Y: box.y + box.height - 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.queueDialog.edit == nil {
			t.Fatal("mouse did not open the queue editor")
		}
		m.queueDialog.edit.editor.SetValue("")
		key(m, tea.KeyEnter)
		m.queueKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("pasted f5"), Paste: true})
		if !strings.Contains(m.queueDialog.edit.editor.Value(), "pasted f5") || m.queueDialog.busy {
			t.Fatal("pasted text was interpreted as a save shortcut")
		}
		view := m.View()
		if lipgloss.Height(view) > m.height || lipgloss.Width(view) > m.width-1 {
			t.Fatalf("editor overflow at %v", size)
		}
		lines := strings.Split(clean(view), "\n")
		if !strings.Contains(lines[box.y+box.height-3], "Сохранить F5") {
			t.Fatal("save mouse row does not match rendering")
		}
		m.queueDialog.edit.editor.SetValue(" \n ")
		if m.saveQueuedEdit() != nil || !strings.Contains(m.queueDialog.note, "непустой") {
			t.Fatal("empty edit reached the backend")
		}
		m.queueDialog.edit.editor.SetValue("draft")
		m.sessionStale = true
		if m.saveQueuedEdit() != nil {
			t.Fatal("stale editor state allowed a mutation")
		}
		m.queueEditMouse(tea.MouseMsg{X: box.x + 2 + 20, Y: box.y + box.height - 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.queueDialog.edit != nil {
			t.Fatal("Back mouse button did not discard the unsaved edit")
		}
	}
}
