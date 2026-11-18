package main

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestSettingsHubGeometryNavigationAndMouse(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, english := range []bool{false, true} {
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 40}} {
			m := newModel()
			activeEnglish.Store(english)
			m.Update(size)
			m.input.SetValue("retained draft")
			m.openDialog("settings")
			for tab := range settingsTabs {
				m.chrome.settingsTab = tab
				m.chrome.settingsNav = true
				key(m, tea.KeyRight)
				key(m, tea.KeyEnd)
				box, _, hits := m.settingsHubLayout()
				view := m.View()
				if lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height || box.y+box.height > size.Height {
					t.Fatal("settings hub overflowed the terminal")
				}
				lines := strings.Split(clean(view), "\n")
				visible := false
				for _, hit := range hits {
					if hit.action == "row" && hit.index == m.chrome.settingsRow {
						visible = true
						if strings.TrimSpace(lines[hit.y]) == "" || hit.y+1 >= box.y+box.height-2 {
							t.Fatal("selected row is outside the content window")
						}
					}
					if hit.action == "close" && !strings.Contains(string([]rune(lines[hit.y])[hit.x:]), "[×]") {
						t.Fatal("close hit target differs from the visible control")
					}
				}
				if !visible {
					t.Fatal("keyboard selection was not scrolled into view")
				}
				key(m, tea.KeyLeft)
				if !m.chrome.settingsNav {
					t.Fatal("Left did not focus section navigation")
				}
			}
			m.chrome.settingsTab, m.chrome.settingsRow, m.chrome.settingsOffset = 0, 0, 0
			selectHubSetting(t, m, "language")
			_, _, hits := m.settingsHubLayout()
			for _, hit := range hits {
				if hit.action == "row" && hit.index == m.chrome.settingsRow {
					mouse(m, hit.x, hit.y, tea.MouseButtonLeft, tea.MouseActionPress)
				}
			}
			if m.prefs.Language != "en" {
				t.Fatal("language row click failed")
			}
			key(m, tea.KeyEsc)
			if !m.input.Focused() || m.input.Value() != "retained draft" {
				t.Fatal("settings lost the composer context")
			}
		}
	}
}

func TestSettingsHubDefaultPermissionUsesSharedRevisionCheckedForm(t *testing.T) {
	m := chatModel(t)
	m.settings = &settingsSnapshot{Writable: true, Namespaces: []settingsNamespace{{NS: "permission", Revision: 7,
		Schema: json.RawMessage(`{"uid":0,"refs":{"0":{"type":"object","dict":{"defaultPreset":1}},"1":{"type":"string"}}}`),
		Value:  map[string]any{"defaultPreset": "workspace-write"}}}}
	m.openDialog("settings")
	// The cached descriptor is a valid snapshot; a background refresh must settle before editing.
	m.settingsLoading = false
	m.settingsHubAction("permission-default")
	d := m.settingsForm
	if d == nil || d.namespace.NS != "permission" || d.namespace.Revision != 7 || d.field == nil || pathKey(d.field.row.path) != `["defaultPreset"]` {
		t.Fatal("default permission did not open the exact shared setting")
	}
	if len(d.ops) != 0 || m.session.Permission != "" {
		t.Fatal("opening settings changed permissions")
	}
	d.field.editor.SetValue("read-only")
	m.acceptSettingsField()
	if len(d.ops) != 1 || d.ops[0].Value != "read-only" || d.namespace.Revision != 7 {
		t.Fatal("default permission bypassed staged revision-checked editing")
	}
	m.closeSettingsForm()
	if m.chrome.dialog != "settings" || m.input.Focused() {
		t.Fatal("form did not return to the modal hub")
	}
	key(m, tea.KeyEsc)
	if !m.input.Focused() {
		t.Fatal("closing settings did not restore focus")
	}
}
