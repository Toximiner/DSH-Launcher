package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestAgentLandingGeometryAndReadOnlyControls(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, language := range []string{"", "en"} {
		for _, hidden := range []bool{false, true} {
			for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}} {
				m := chatModel(t)
				m.prefs.Language = language
				m.prefs.HideSidebar = hidden
				m.Update(size)
				m.input.SetValue("preserved 世界")
				applyChat(t, m, `{"type":"snapshot","header":{"id":"s","cwd":"/tmp/project"},"cursor":-1}`)
				m.session.AgentPreset = "ptc"
				m.session.Permission = "workspace-write"
				m.session.Model = &modelSelection{Provider: "fixture", Model: "qwen", Reasoning: "high"}
				m.agentInfo = &agentContext{id: "s", epoch: m.chat.epoch, presets: map[string]string{"ptc": "PTC mode"}}
				if !m.landingVisible() {
					t.Fatal("empty session did not use the landing screen")
				}
				m.sessionBusy, m.sessionAction = true, "refresh"
				if !m.landingVisible() {
					t.Fatal("background refresh would flash between layouts")
				}
				m.sessionBusy = false
				view := m.View()
				if lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height {
					t.Fatal("landing screen overflowed")
				}
				box := m.composerBox()
				lines := strings.Split(clean(view), "\n")
				if !strings.Contains(lines[box.y+1], "preserved 世界") || !strings.Contains(lines[box.y+3], "PTC mode") || !strings.Contains(lines[box.y+3], "qwen") {
					t.Fatal("composer/context hit areas are misaligned")
				}
				for y := box.y; y < box.y+box.height; y++ {
					cells := ansi.Cut(lines[y], box.x, box.x+box.width)
					if !strings.HasPrefix(cells, "┃") || lipgloss.Width(cells) != box.width {
						t.Fatalf("composer row %d does not occupy its shared hit geometry: %q", y, cells)
					}
				}
				_, _, hits := m.landingControls()
				for _, hit := range hits {
					if hit.x < box.x || hit.x+hit.width > box.x+box.width || hit.y < 2 || hit.y >= size.Height {
						t.Fatal("landing control lies outside its visible area")
					}
					if strings.TrimSpace(ansi.Cut(lines[hit.y], hit.x, hit.x+hit.width)) == "" {
						t.Fatalf("control %q points at blank cells", hit.action)
					}
					if hit.action != "workspaces" && hit.y != box.y+3 {
						t.Fatalf("control %q is outside the composer metadata row", hit.action)
					}
				}
				for _, hit := range hits {
					if hit.action == "models" {
						m.navFocused = true
						m.input.Blur()
						mouse(m, hit.x, hit.y, tea.MouseButtonLeft, tea.MouseActionPress)
						if m.modelPicker == nil || m.navFocused {
							t.Fatal("model click did not transfer focus from the sidebar")
						}
						m.closeModels()
						if !m.input.Focused() || m.input.Value() != "preserved 世界" {
							t.Fatal("closing the model picker lost the draft or composer focus")
						}
					}
				}
				m.input.Blur()
				m.mouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				if !m.input.Focused() || m.input.Value() != "preserved 世界" {
					t.Fatal("composer click lost the draft")
				}
				for _, action := range []string{"models", "presets", "permissions", "compose", "workspaces"} {
					m.agentControl(action)
					if m.input.Value() != "preserved 世界" || m.sessionBusy {
						t.Fatal("opening an agent control changed the session or draft")
					}
					m.closeModels()
					m.closeWorkbench()
					if m.composer != nil {
						m.composer.visible = false
					}
				}
				m.chat.live = &chatAttempt{}
				if m.landingVisible() {
					t.Fatal("landing screen hid a live reply")
				}
				m.chat.live = nil
				m.chat.clearedThrough = 10
				if m.landingVisible() {
					t.Fatal("clearing conversation history made an existing chat look new")
				}
			}
		}
	}
}
func TestAgentContextRejectsStaleSessionAndUsesBackendSelection(t *testing.T) {
	m := chatModel(t)
	old := &agentContext{id: "previous", epoch: 0}
	current := &agentContext{id: m.session.ID, epoch: m.chat.epoch, busy: true}
	m.agentInfo = current
	m.handleAgentContext(agentContextResult{owner: old, selection: modelSelection{Model: "wrong"}})
	if m.agentInfo != current || !current.busy {
		t.Fatal("stale context replaced the current session")
	}
	m.handleAgentContext(agentContextResult{owner: current, catalog: &modelCatalog{}, selection: modelSelection{Provider: "test", Model: "actual", Reasoning: "high"}})
	if current.busy || !strings.Contains(m.modelButton(), "actual") || strings.Contains(m.modelButton(), "wrong") {
		t.Fatal("model badge does not use the current backend selection")
	}
}
func TestPermissionSelectionConfirmsAndRefusesConcurrentChange(t *testing.T) {
	var mu sync.Mutex
	current := "workspace-write"
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "fixture", Path: "/"})
			w.WriteHeader(302)
			return
		}
		var req struct {
			ID      string `json:"rpcId"`
			Method  string `json:"method"`
			Payload struct {
				Args map[string]any `json:"args"`
			} `json:"payload"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		defer mu.Unlock()
		var value any
		switch req.Method {
		case "session/projections":
			value = map[string]any{"kind": "sequenced", "values": map[string]any{"permissions": map[string]any{"currentValue": current}}}
		case "permissionPresets/catalog":
			value = map[string]any{"options": []any{map[string]any{"value": "read-only", "name": "Read Only"}, map[string]any{"value": "workspace-write", "name": "Workspace Write"}}}
		case "commands/execute":
			writes++
			current = strings.TrimPrefix(stringValue(req.Payload.Args, "line"), "/permission ")
			value = map[string]any{"result": map[string]any{"kind": "success", "text": ""}}
		default:
			t.Errorf("unexpected endpoint: %s", req.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": req.ID, "result": map[string]any{"ok": true, "value": value}})
	}))
	defer server.Close()
	m := chatModel(t)
	m.settingsLogin = server.URL + "/?token=fixture"
	makePanel := func() {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		m.workbench = &workbench{ctx: ctx, cancel: cancel, id: m.session.ID, epoch: m.chat.epoch, kind: "permissions", editor: textarea.New(), search: textinput.New(), view: viewport.New(1, 1)}
		r := m.loadWorkbench()().(benchResult)
		m.handleBench(r)
	}
	makePanel()
	m.benchPrepare(tea.KeyEnter)
	mu.Lock()
	if writes != 0 {
		t.Fatal("permission changed before confirmation")
	}
	mu.Unlock()
	m.benchKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.workbench.confirm {
		t.Fatal("permission confirmation did not cancel")
	}
	m.benchPrepare(tea.KeyEnter)
	m.handleBench(m.benchExecute()().(benchResult))
	mu.Lock()
	if writes != 1 || current != "read-only" {
		t.Fatal("confirmed selection was not applied")
	}
	mu.Unlock()
	makePanel()
	m.workbench.index = 1
	m.benchPrepare(tea.KeyEnter)
	mu.Lock()
	current = "external-change"
	mu.Unlock()
	m.handleBench(m.benchExecute()().(benchResult))
	mu.Lock()
	defer mu.Unlock()
	if writes != 1 || !m.workbench.blocked {
		t.Fatal("concurrent permission change was overwritten or retried")
	}
}

func TestAgentModelBadgeKeepsConfirmedChoiceAcrossInFlightPoll(t *testing.T) {
	m := chatModel(t)
	m.session.Model = &modelSelection{Provider: "test", Model: "confirmed"}
	m.modelRevision = 1
	m.Update(sessionResult{sequence: m.sessionSequence, modelRevision: 0, view: &sessionView{ID: m.session.ID, Model: &modelSelection{Provider: "test", Model: "old"}}})
	if m.session.Model.Model != "confirmed" {
		t.Fatal("in-flight poll undid the acknowledged model choice")
	}
	m.Update(sessionResult{sequence: m.sessionSequence, modelRevision: 1, view: &sessionView{ID: m.session.ID, Model: &modelSelection{Provider: "test", Model: "external"}}})
	if m.session.Model.Model != "external" {
		t.Fatal("fresh external model choice was hidden")
	}
}
