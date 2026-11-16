package main

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestSettingsJSONEditorStagesValidContentAndPreservesDraftOnInvalidInput(t *testing.T) {
	m := formTestModel(t)
	d := m.settingsForm
	path := []string{"providers", "local", "models"}
	selectFormPath(t, m, path)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	if d.field == nil || d.field.multiline == nil {
		t.Fatal("JSON editor did not open")
	}
	original := d.field.multiline.Value()
	if !strings.Contains(original, `"base"`) || !strings.Contains(original, `"other"`) {
		t.Fatal("existing array members missing")
	}
	for _, size := range [][2]int{{120, 42}, {100, 30}, {80, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if lipgloss.Width(m.View()) > m.width-1 || lipgloss.Height(m.View()) > m.height {
			t.Fatal("editor overflow", size)
		}
	}
	d.field.multiline.SetValue(`[{`)
	m.Update(tea.KeyMsg{Type: tea.KeyF5})
	if len(d.ops) != 0 || d.field == nil || d.field.multiline.Value() != `[{` {
		t.Fatal("invalid JSON lost the editor or staged changes")
	}
	d.field.multiline.SetValue("[\n  {\"id\": \"новая\"}\n]")
	m.Update(tea.KeyMsg{Type: tea.KeyF5})
	value, _ := valueAt(d.value, path)
	if d.field != nil || len(d.ops) != 1 || value.([]any)[0].(map[string]any)["id"] != "новая" {
		t.Fatal("JSON edit was not staged")
	}
	if m.input.Value() != "retained draft" {
		t.Fatal("settings editor altered chat draft")
	}
	d.conflicts = map[string]bool{
		pathKey(append(append([]string{}, path...), "0", "id")): true,
		pathKey([]string{"providers", "local", "enabled"}):      true,
	}
	row, _ := d.selection()
	d.editMultiline(row)
	m.Update(tea.KeyMsg{Type: tea.KeyF5})
	if len(d.conflicts) != 1 || !d.conflicts[pathKey([]string{"providers", "local", "enabled"})] {
		t.Fatal("explicit whole-array review did not resolve its child conflict or cleared an unrelated conflict")
	}
}

func TestSettingsMultilineStringPasteAndCancellationDoNotSendOrNormalize(t *testing.T) {
	m := formTestModel(t)
	d := m.settingsForm
	row := selectFormPath(t, m, []string{"providers", "local", "baseURL"})
	d.editMultiline(row)
	d.field.multiline.SetValue("")
	text := "first\nUnicode: привет\n\nlast"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
	if d.field.multiline.Value() != text || len(d.ops) != 0 {
		t.Fatal("pasted multiline string changed or committed")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF5})
	if len(d.ops) != 1 || d.ops[0].Value != text || d.field != nil {
		t.Fatal("multiline value lost newlines")
	}
	row, _ = d.selection()
	d.editMultiline(row)
	d.field.multiline.SetValue("discarded")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if d.field != nil || d.ops[0].Value != text {
		t.Fatal("cancel changed staged value")
	}
	root := formRow{path: []string{"hidden"}, known: true, node: d.schema.node(9)}
	d.editMultiline(root)
	if d.field != nil {
		t.Fatal("secret entered unmasked multiline editor")
	}
	d.namespace.Secrets = append(d.namespace.Secrets, settingsSecret{Path: []string{"providers", "local"}, Set: true})
	row = selectFormPath(t, m, []string{"providers", "local", "baseURL"})
	d.editMultiline(row)
	if d.field != nil {
		t.Fatal("descendant of a redacted container was editable")
	}
}

func TestSettingsPortableConstraintsRejectBeforeStaging(t *testing.T) {
	raw := `{"uid":0,"refs":{"0":{"type":"object","dict":{"count":1,"name":2,"variant":3,"items":6}},"1":{"type":"number","meta":{"min":1,"max":9,"step":2}},"2":{"type":"string","meta":{"required":true,"max":2}},"3":{"type":"union","list":[4,5]},"4":{"type":"const","value":false},"5":{"type":"dict","inner":1},"6":{"type":"array","inner":2,"meta":{"min":1,"max":2}}}}`
	s, err := decodeFormSchema(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		id    int
		value any
		valid bool
	}{
		{1, float64(3), true}, {1, float64(2), false}, {1, float64(11), false},
		{2, "😀", true}, {2, "😀a", false}, {2, nil, false},
		{3, false, true}, {3, map[string]any{"low": float64(3)}, true}, {3, map[string]any{"low": float64(2)}, false},
		{6, []any{"ok"}, true}, {6, []any{}, false}, {6, []any{"too long"}, false},
		{0, map[string]any{}, false},
	} {
		if valid := s.validate(s.node(test.id), test.value) == nil; valid != test.valid {
			t.Fatal("unexpected portable validation", test.id, test.value, valid)
		}
	}
	m := formTestModel(t)
	d := m.settingsForm
	row := selectFormPath(t, m, []string{"providers", "local", "enabled"})
	row.node = formNode{Type: "number", Meta: map[string]any{"min": float64(1)}}
	d.edit(row, false)
	d.field.editor.SetValue("-4")
	m.acceptSettingsField()
	if len(d.ops) != 0 || d.field == nil || !strings.Contains(d.note, "Минимум") {
		t.Fatal("out-of-range value reached draft")
	}
}
