package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const modelFixture = `{"default":{"provider":"local","model":"base"},"groups":[{"id":"local","name":"Local","models":[{"id":"base","name":"Base"},{"id":"next","name":"Next","reasoning":{"efforts":[{"id":"off","name":"Off"},{"id":"high","name":"High"}],"defaultEffort":"high"}}]}],"failures":[{"id":"offline","message":"secret backend detail"}]}`

func modelTestPicker(t *testing.T) *model {
	t.Helper()
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	m.session.ID = "s"
	m.input.SetValue("retained draft")
	m.openModels()
	var catalog modelCatalog
	if err := json.Unmarshal([]byte(modelFixture), &catalog); err != nil {
		t.Fatal(err)
	}
	d := m.modelPicker
	m.handleModelPicker(modelPickerResult{dialog: d, serial: d.serial, catalog: &catalog, current: catalog.Default})
	return m
}

func TestModelsPickerSearchReasoningMouseAndGeometry(t *testing.T) {
	m := modelTestPicker(t)
	d := m.modelPicker
	for _, size := range [][2]int{{120, 42}, {100, 30}, {80, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		d.choose(0)
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m.Update(tea.KeyMsg{Type: tea.KeyTab})
		if d.index != 1 || d.effort != "off" {
			t.Fatal("model/effort navigation failed")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyTab})
		if d.effort != "high" {
			t.Fatal("adapter-owned effort was not offered")
		}
		if lipgloss.Width(m.View()) > m.width-1 || lipgloss.Height(m.View()) > m.height {
			t.Fatal("models dialog overflowed")
		}
		view := m.View()
		if !strings.Contains(view, "общую модель") || strings.Contains(view, "secret backend detail") {
			t.Fatal("shared-default side effect absent or private provider error exposed")
		}
		box := m.modelsBox()
		m.modelsMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 7, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
		if d.index != 0 {
			t.Fatal("model row mouse region is incorrect")
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("NEXT"), Paste: true})
	if len(d.options()) != 1 || d.options()[0].model.ID != "next" {
		t.Fatal("catalog search lost case-insensitive aliases")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter, Paste: true})
	if d.busy {
		t.Fatal("pasted Enter applied a model")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.modelPicker != nil || m.input.Value() != "retained draft" {
		t.Fatal("cancel lost composer draft")
	}
	m.handleModelPicker(modelPickerResult{dialog: d, serial: d.serial, err: errors.New("late")})
	if m.modelPicker != nil {
		t.Fatal("late read reopened the model picker")
	}
}

func TestModelsSelectionPreflightAcknowledgementAndNoRetry(t *testing.T) {
	var mu sync.Mutex
	current := modelSelection{Provider: "local", Model: "base"}
	writes := 0
	expectedEffort := ""
	reject := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "test", Path: "/"})
			w.WriteHeader(302)
			return
		}
		var request struct {
			ID      string `json:"rpcId"`
			Payload struct {
				Args struct {
					Request map[string]json.RawMessage `json:"request"`
				} `json:"args"`
			} `json:"payload"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid API request")
		}
		mu.Lock()
		defer mu.Unlock()
		var value any
		ok := true
		switch r.URL.Path {
		case "/api/session/modelCatalog":
			json.Unmarshal([]byte(modelFixture), &value)
		case "/api/session/projections":
			value = map[string]any{"kind": "sequenced", "values": map[string]any{"modelSelection": map[string]any{"next": current}}}
		case "/api/session/selectModel":
			writes++
			raw, exists := request.Payload.Args.Request["reasoningEffort"]
			if expectedEffort == "" && exists {
				t.Error("default effort was sent as null or an invented value")
			}
			if expectedEffort != "" && string(raw) != `"`+expectedEffort+`"` {
				t.Error("selected reasoning effort was not sent exactly")
			}
			var id string
			json.Unmarshal(request.Payload.Args.Request["sessionId"], &id)
			if id != "s" {
				t.Error("model write lost session identity")
			}
			if reject {
				ok = false
			} else {
				current = modelSelection{Provider: "local", Model: "next", Reasoning: expectedEffort}
				value = map[string]any{"selected": current}
			}
		default:
			t.Errorf("unexpected model endpoint %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": request.ID, "result": map[string]any{"ok": ok, "value": value}})
	}))
	defer server.Close()
	m := modelTestPicker(t)
	m.settingsLogin = server.URL + "/?token=test"
	d := m.modelPicker
	d.choose(1)
	cmd := m.applyModel()
	if cmd == nil || m.applyModel() != nil {
		t.Fatal("model mutation was not serialized")
	}
	result := cmd().(modelPickerResult)
	m.handleModelPicker(result)
	if result.err != nil || d.current.Model != "next" || d.uncertain {
		t.Fatal("accepted selection was not acknowledged", result.err)
	}
	mu.Lock()
	expectedEffort = "high"
	mu.Unlock()
	d.effort = "high"
	result = m.applyModel()().(modelPickerResult)
	m.handleModelPicker(result)
	if result.err != nil || d.current.Reasoning != "high" {
		t.Fatal("reasoning selection was not acknowledged", result.err)
	}
	mu.Lock()
	expectedEffort = ""
	n := writes
	current = modelSelection{Provider: "other-client", Model: "changed"}
	mu.Unlock()
	d.choose(0)
	result = m.applyModel()().(modelPickerResult)
	m.handleModelPicker(result)
	mu.Lock()
	if writes != n {
		t.Error("preflight overwrote another client's selection")
	}
	mu.Unlock()
	if !d.uncertain || !strings.Contains(d.note, "другим клиентом") || m.applyModel() != nil {
		t.Fatal("conflict did not require an explicit reread")
	}
	// A successful reread clears uncertainty. A rejected mutation is never retried.
	result = m.loadModels()().(modelPickerResult)
	m.handleModelPicker(result)
	mu.Lock()
	reject = true
	mu.Unlock()
	d.choose(1)
	result = m.applyModel()().(modelPickerResult)
	m.handleModelPicker(result)
	if !d.uncertain || m.applyModel() != nil {
		t.Fatal("unconfirmed model mutation retried")
	}
	if m.input.Value() != "retained draft" {
		t.Fatal("selection changed the composer")
	}
}

func TestModelDialogIgnoresTooSmallAndStaleActions(t *testing.T) {
	m := modelTestPicker(t)
	d := m.modelPicker
	m.Update(tea.WindowSizeMsg{Width: 79, Height: 24})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.index != 0 {
		t.Fatal("undersized terminal changed selection")
	}
	d.cancel()
	if m.applyModel() != nil {
		t.Fatal("cancelled model dialog wrote settings")
	}
	m.handleModelPicker(modelPickerResult{dialog: d, serial: d.serial, selected: &modelSelection{Provider: "late", Model: "late"}})
	if d.current.Model != "base" {
		t.Fatal("stale result changed current model")
	}
}
