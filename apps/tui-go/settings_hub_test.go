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
			for tab := -1; tab < len(settingsTabs); tab++ {
				m.chrome.settingsTab = tab
				m.chrome.settingsNav = false
				key(m, tea.KeyEnd)
				box, _, hits := m.settingsHubLayout()
				view := m.View()
				if lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height || box.y+box.height > size.Height {
					t.Fatal("settings hub overflowed the terminal")
				}
				lines := strings.Split(clean(view), "\n")
				visible := false
				rows := m.settingsHubRows()
				for _, hit := range hits {
					if hit.action == "row" && !strings.Contains(lines[hit.y], rows[hit.index].label) {
						t.Fatalf("row %s hit differs from rendered row", rows[hit.index].action)
					}
					if hit.action == "row" && hit.index == m.chrome.settingsRow {
						visible = true
						if strings.TrimSpace(lines[hit.y]) == "" || hit.y+1 >= box.y+box.height-2 {
							t.Fatal("selected row is outside the content window")
						}
					}
					if hit.action == "close" && !strings.Contains(string([]rune(lines[hit.y])[hit.x:]), "esc") {
						t.Fatal("close hit target differs from the visible control")
					}
				}
				if !visible {
					t.Fatal("keyboard selection was not scrolled into view")
				}
			}
			m.chrome.settingsTab, m.chrome.settingsRow, m.chrome.settingsOffset = -1, 0, 0
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

func TestSettingsHubSearchGroupsAndLiteralPaste(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, english := range []bool{false, true} {
		m := newModel()
		activeEnglish.Store(english)
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		m.input.SetValue("retained draft")
		m.openDialog("settings")
		seen := map[string]bool{}
		for _, row := range m.settingsHubRows() {
			if seen[row.action] {
				t.Fatalf("duplicate action %s", row.action)
			}
			seen[row.action] = true
		}
		if len(seen) != 15 {
			t.Fatalf("settings action lost: %v", seen)
		}
		query := tr("Язык")
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(query), Paste: true})
		rows := m.settingsHubRows()
		if len(rows) != 1 || rows[0].action != "language" {
			t.Fatalf("translated search did not filter: %+v", rows)
		}
		key(m, tea.KeyF1)
		key(m, tea.KeyEsc)
		if m.chrome.dialog != "settings" || m.chrome.settingsQuery != query {
			t.Fatal("help return lost settings query")
		}
		key(m, tea.KeyCtrlU)
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/quit\r\nunsafe"), Paste: true})
		if m.chrome.dialog != "settings" || m.input.Value() != "retained draft" || len(m.settingsHubRows()) != 0 {
			t.Fatal("pasted command escaped search or unexpectedly matched")
		}
		before := m.prefs
		key(m, tea.KeyEnter)
		key(m, tea.KeyRight)
		if m.prefs != before || m.settingsForm != nil {
			t.Fatal("empty search activated a hidden setting")
		}
		key(m, tea.KeyCtrlU)
		for group := 0; group < len(settingsTabs); group++ {
			key(m, tea.KeyTab)
			if m.chrome.settingsTab != group || len(m.settingsHubRows()) == 0 {
				t.Fatal("Tab failed to filter settings group")
			}
			for _, entry := range m.settingsHubEntries() {
				if entry.group != group {
					t.Fatal("group filter leaked")
				}
			}
		}
		key(m, tea.KeyTab)
		if m.chrome.settingsTab != -1 || len(m.settingsHubRows()) != 15 {
			t.Fatal("group filter did not return to all settings")
		}
		key(m, tea.KeyShiftTab)
		if m.chrome.settingsTab != len(settingsTabs)-1 {
			t.Fatal("reverse filter did not wrap")
		}
		key(m, tea.KeyEsc)
		if !m.input.Focused() || m.input.Value() != "retained draft" {
			t.Fatal("settings search lost composer")
		}
	}
}

func TestSettingsHubArrowsOnlyChangeLocalPreferences(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.openDialog("settings")
	writes := 0
	m.savePreferences = func(clientPreferences) error { writes++; return nil }
	for _, action := range []string{"permission-default", "providers", "models", "plugins", "presets", "market", "packages", "config", "commands", "shortcuts", "font"} {
		selectHubSetting(t, m, action)
		key(m, tea.KeyRight)
		key(m, tea.KeyLeft)
		if writes != 0 || m.settingsForm != nil || m.chrome.dialog != "settings" || m.page != chatPage {
			t.Fatalf("arrow unexpectedly executed %s", action)
		}
	}
	for _, action := range []string{"theme", "language", "mouse", "input-history"} {
		selectHubSetting(t, m, action)
		before := m.prefs
		key(m, tea.KeyRight)
		if m.prefs == before {
			t.Fatalf("arrow did not change %s", action)
		}
		key(m, tea.KeyLeft)
		if m.prefs != before {
			t.Fatalf("reverse arrow did not restore %s", action)
		}
	}
	if writes != 8 {
		t.Fatalf("unexpected preference writes: %d", writes)
	}
}

func TestSettingsAndHelpSearchAcceptPhysicalSpace(t *testing.T) {
	for _, dialog := range []string{"settings", "help"} {
		m := newModel()
		m.openDialog(dialog)
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("История")})
		m.Update(tea.KeyMsg{Type: tea.KeySpace})
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ввода")})
		query := m.chrome.helpQuery
		if dialog == "settings" {
			query = m.chrome.settingsQuery
			if len(m.settingsHubRows()) == 0 {
				t.Fatal("spaced query did not match setting")
			}
		}
		if query != "История ввода" {
			t.Fatalf("%s search lost space: %q", dialog, query)
		}
	}
}
