package main

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestActionRegistryIDsAliasesAndKeysAreUnambiguous(t *testing.T) {
	ids, aliases, keys := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, action := range actionRegistry {
		if action.id == "" || ids[action.id] || action.label == "" || action.group == "" || action.run == nil {
			t.Fatalf("invalid or duplicate action descriptor: %+v", action)
		}
		ids[action.id] = true
		for _, alias := range action.aliases {
			if aliases[alias] || len(strings.Fields(alias)) != 1 || !strings.HasPrefix(alias, "/") {
				t.Fatalf("ambiguous or argumentful alias: %s", alias)
			}
			aliases[alias] = true
		}
		for _, key := range action.keys {
			if keys[key] {
				t.Fatalf("duplicate global key: %s", key)
			}
			keys[key] = true
		}
	}
	for _, alias := range []string{"/runtime", "/updates", "/diagnostics", "/market", "/new", "/sessions", "/plan", "/exit", "/quit", "/clear", "/queue", "/archive", "/restore", "/fork", "/rename", "/providers", "/models", "/parent", "/compose", "/operations", "/account", "/workspaces", "/packages", "/presets", "/files", "/commands", "/plugins", "/jobs", "/schedules", "/agents", "/tasks", "/terminals", "/goal", "/history", "/stop", "/settings", "/about", "/retry-send", "/help"} {
		if !aliases[alias] {
			t.Fatalf("legacy no-argument command missing: %s", alias)
		}
	}
}

func TestActionSlashOnlyHandlesExactNoArgumentCommands(t *testing.T) {
	m := newModel()
	m.input.SetValue("retained draft")
	for _, line := range []string{"hello", "/session s", "/mode default", "/goal pause", "/attach /tmp/file", "/plan off", "/clear extra", "/unknown", " /quit", "/quit\nhello"} {
		cmd, handled := m.dispatchSlashAction(line)
		if handled || cmd != nil {
			t.Fatalf("registry intercepted text or argumentful command: %q", line)
		}
	}
	m.lines = []string{"old output"}
	if cmd, handled := m.dispatchSlashAction("/clear"); !handled || cmd != nil || len(m.lines) != 0 {
		t.Fatal("exact clear action did not run")
	}
	if m.input.Value() != "retained draft" {
		t.Fatal("action dispatch changed draft")
	}
	for _, line := range []string{"/quit", "/exit"} {
		cmd, handled := m.dispatchSlashAction(line)
		if !handled || cmd == nil {
			t.Fatal("explicit quit alias missing")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("quit alias did not return quit command")
		}
	}
}

func TestActionDisabledSlashDoesNotFallback(t *testing.T) {
	m := newModel()
	m.input.SetValue("keep draft")
	before := m.page
	cmd, handled := m.dispatchSlashAction("/new")
	if !handled || cmd != nil || m.page != before || len(m.lines) != 1 || !strings.Contains(m.lines[0], tr("Сначала подключитесь к dsh.")) {
		t.Fatal("disabled slash action ran, fell back or failed to explain")
	}
	if m.input.Value() != "keep draft" {
		t.Fatal("disabled action changed draft")
	}
}

func TestActionKeyRejectsPasteAndLiteralControlNames(t *testing.T) {
	m := newModel()
	for _, msg := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("ctrl+d")}, {Type: tea.KeyRunes, Runes: []rune("f4")}, {Type: tea.KeyRunes, Runes: []rune("alt+t")}, {Type: tea.KeyCtrlD, Paste: true}, {Type: tea.KeyRunes, Runes: []rune("t"), Alt: true, Paste: true}} {
		if cmd, handled := m.dispatchKeyAction(msg); handled || cmd != nil {
			t.Fatalf("text treated as control action: %+v", msg)
		}
	}
	cmd, handled := m.dispatchKeyAction(tea.KeyMsg{Type: tea.KeyCtrlD})
	if !handled || cmd == nil {
		t.Fatal("real Ctrl+D did not execute registered quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("real Ctrl+D produced a different command")
	}
}

func TestActionKeyScopeAndModalGuards(t *testing.T) {
	m := newModel()
	m.page = logPage
	if cmd, handled := m.dispatchKeyAction(tea.KeyMsg{Type: tea.KeyCtrlN}); handled || cmd != nil {
		t.Fatal("chat action stole a list key")
	}
	m.queueDialog.visible = true
	if cmd, handled := m.dispatchKeyAction(tea.KeyMsg{Type: tea.KeyF4}); handled || cmd != nil || m.chrome.dialog != "" {
		t.Fatal("global action stole modal focus")
	}
	m.queueDialog.visible = false
	m.marketEditing = true
	if cmd, handled := m.dispatchKeyAction(tea.KeyMsg{Type: tea.KeyF4}); handled || cmd != nil {
		t.Fatal("global action stole page editor key")
	}
	m.marketEditing = false
	if _, handled := m.dispatchKeyAction(tea.KeyMsg{Type: tea.KeyF4}); !handled || m.chrome.dialog != "settings" {
		t.Fatal("base action failed outside editor context")
	}
}

func TestActionPaletteAndShortcutReferenceShareRegistry(t *testing.T) {
	descriptorByID := map[string]actionDescriptor{}
	for _, action := range actionRegistry {
		descriptorByID[action.id] = action
	}
	for _, item := range paletteCatalog {
		action, ok := descriptorByID[item.id]
		if !ok || action.hidden || item.label != action.label || item.hint != action.hint || !reflect.DeepEqual(item.aliases, action.aliases) || !reflect.DeepEqual(item.keys, action.keys) {
			t.Fatalf("palette drifted from registered action: %s", item.id)
		}
	}
	generated := actionShortcuts()
	for _, action := range actionRegistry {
		if len(action.aliases) == 0 && len(action.keys) == 0 {
			continue
		}
		found := false
		for _, row := range generated {
			if row.action == action.label {
				found = true
				for _, alias := range action.aliases {
					if !strings.Contains(row.keys, alias) {
						t.Fatalf("help omitted registered alias %s", alias)
					}
				}
				break
			}
		}
		if !found {
			t.Fatalf("registered action missing help: %s", action.id)
		}
	}
}

func TestActionPageWorkbenchAllowsNavigationButOwnsEditingKeys(t *testing.T) {
	m := pluginTestModel(t)
	if _, handled := m.dispatchKeyAction(tea.KeyMsg{Type: tea.KeyCtrlN}); handled {
		t.Fatal("chat new-session key stole plugin-page create action")
	}
	m.workbench.editing = true
	if cmd, handled := m.dispatchKeyAction(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}, Alt: true}); handled || cmd != nil {
		t.Fatal("navigation stole plugin editor focus")
	}
	m.workbench.editing = false
	if _, handled := m.dispatchKeyAction(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}, Alt: true}); !handled || m.page != chatPage {
		t.Fatal("safe page workbench trapped base navigation")
	}
}

func TestActionPaletteSearchFindsEveryVisibleAlias(t *testing.T) {
	m := openTestPalette(t)
	for _, action := range paletteCatalog {
		for _, alias := range action.aliases {
			m.palette.search.SetValue(alias)
			found := false
			for _, row := range m.paletteFiltered() {
				if row.kind == "item" && paletteCatalog[row.index].id == action.id {
					found = true
				}
			}
			if !found {
				t.Fatalf("palette cannot find registered alias %s", alias)
			}
		}
	}
}

func TestActionUpdateRoutesExactSlashAndKeepsTextLiteral(t *testing.T) {
	m := newModel()
	m.lines = []string{"old output"}
	m.input.SetValue("/clear")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || len(m.lines) != 0 || m.input.Value() != "" {
		t.Fatal("Update did not route exact registered slash action")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ctrl+d"), Paste: true})
	if m.input.Value() != "ctrl+d" || m.chrome.dialog != "" {
		t.Fatal("Update treated literal control-name paste as an action")
	}
	m.input.SetValue("/about")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.chrome.dialog != "about" {
		t.Fatal("Update did not execute shared about action")
	}
}
