package main

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestDeclarativePluginPanelValidationAndConfirmation(t *testing.T) {
	source := `{"$schema":"dsh-tui-panel/v1","owner":"sample","title":"Sample","fields":[{"id":"name","label":"Name","type":"string","required":true,"default":"draft"}],"actions":[{"label":"Save","command":"sample-save"}],"tables":[{"title":"Table","columns":["A"],"rows":[["value"]]}]}`
	p, err := decodePluginPanel(source, "sample")
	if err != nil || p == nil {
		t.Fatal("valid panel refused", err)
	}
	for _, invalid := range []string{strings.Replace(source, "sample-save", "other-save", 1), strings.Replace(source, "\"string\"", "\"secret\"", 1), strings.Replace(source, "dsh-tui-panel/v1", "dsh-tui-panel/v99", 1)} {
		if _, err := decodePluginPanel(invalid, "sample"); err == nil {
			t.Fatal("unsafe or unsupported panel accepted")
		}
	}
	m := benchTestModel(t, "commands")
	d := m.workbench
	d.pluginPanel = p
	d.rows = []benchRow{{ID: "sample-save"}}
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}} {
		m.Update(size)
		if lipgloss.Width(m.View()) > size.Width-1 || lipgloss.Height(m.View()) > size.Height {
			t.Fatal("plugin panel overflowed")
		}
	}
	m.benchKey(tea.KeyMsg{Type: tea.KeyEnter})
	p.input.SetValue("literal\" /other-command")
	m.benchKey(tea.KeyMsg{Type: tea.KeyEnter})
	m.benchKey(tea.KeyMsg{Type: tea.KeyDown})
	m.benchKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !d.confirm || d.action == nil {
		t.Fatal("plugin action skipped review")
	}
	line := stringValue(d.action.Args, "line")
	var input map[string]string
	if !strings.HasPrefix(line, "/sample-save ") || json.Unmarshal([]byte(strings.TrimPrefix(line, "/sample-save ")), &input) != nil || input["name"] != "literal\" /other-command" {
		t.Fatal("form payload was changed or escaped into another command")
	}
	m.benchKey(tea.KeyMsg{Type: tea.KeyEsc})
	if d.confirm || d.pluginPanel != p || p.values["name"] != "literal\" /other-command" {
		t.Fatal("cancel lost the plugin draft")
	}
}
