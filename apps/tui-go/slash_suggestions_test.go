package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func slashFixture(value string) *model {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.input.SetValue(value)
	m.input.CursorEnd()
	return m
}
func TestSlashSuggestionsUseRegistryAndFilterWithoutOperations(t *testing.T) {
	m := slashFixture("/")
	rows := m.slashSuggestions()
	if len(rows) < 15 {
		t.Fatal("missing shared commands")
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.alias] {
			t.Fatal("duplicate alias")
		}
		seen[r.alias] = true
	}
	for _, alias := range []string{"/plugins", "/market", "/schedules", "/session", "/mode", "/attach", "/help"} {
		if !seen[alias] {
			t.Fatal("missing", alias)
		}
	}
	m.input.SetValue("/pl")
	m.input.CursorEnd()
	rows = m.slashSuggestions()
	if len(rows) != 2 || rows[0].alias != "/plan" || rows[1].alias != "/plugins" {
		t.Fatalf("filtered commands %#v", rows)
	}
	m.input.SetValue("/sessions")
	m.input.CursorEnd()
	rows = m.slashSuggestions()
	if rows[0].alias != "/sessions" {
		t.Fatal("exact alias did not win")
	}
	if m.sessionBusy || m.workbench != nil || m.session != nil {
		t.Fatal("suggestions started an operation")
	}
}
func TestSlashSuggestionsDispatchEnterAndCompleteTabThroughUpdate(t *testing.T) {
	m := slashFixture("/sett")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.chrome.dialog != "settings" || m.input.Value() != "" {
		t.Fatal("Enter did not execute selected command through existing parser")
	}
	m = slashFixture("/sett")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.chrome.dialog != "" || m.input.Value() != "/settings " || len(m.slashSuggestions()) != 0 {
		t.Fatal("Tab executed action or lost completion")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.chrome.dialog != "settings" {
		t.Fatal("completed command not accepted by normal Enter")
	}
	for _, prefix := range []string{"/session", "/attach", "/mode"} {
		m = slashFixture(prefix)
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if !strings.HasSuffix(m.input.Value(), " ") || m.sessionBusy || m.session != nil {
			t.Fatal("argument command executed without arguments", prefix)
		}
	}
}
func TestSlashSuggestionsKeysPreserveHistoryDismissAndModalIsolation(t *testing.T) {
	m := slashFixture("/pl")
	m.history = []string{"previous"}
	m.historyIndex = 1
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.slash.index != 1 || m.input.Value() != "/pl" || m.historyIndex != 1 {
		t.Fatal("Down went to input history")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.input.Value() != "/pl" || len(m.slashSuggestions()) != 0 {
		t.Fatal("Esc lost draft or failed to dismiss")
	}
	m.input.SetValue("")
	m.input.CursorEnd()
	m.slashSuggestions()
	m.input.SetValue("/pl")
	m.input.CursorEnd()
	if len(m.slashSuggestions()) != 2 {
		t.Fatal("retyping dismissed prefix did not reopen")
	}
	m.openDialog("settings")
	if len(m.slashSuggestions()) != 0 {
		t.Fatal("suggestions covered a modal")
	}
	for _, value := range []string{"hello /pl", "/mode plan", " /plugins", "/not-a-command"} {
		m = slashFixture(value)
		if len(m.slashSuggestions()) != 0 {
			t.Fatal("inappropriate suggestions", value)
		}
	}
	m = slashFixture("/plugins")
	m.input.SetCursor(2)
	if len(m.slashSuggestions()) != 0 {
		t.Fatal("editing the middle opened suggestions")
	}
}
func TestSlashSuggestionsGeometryAndMouseCompletion(t *testing.T) {
	for _, english := range []bool{false, true} {
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 40}} {
			for _, conversation := range []bool{false, true} {
				m := slashFixture("/pl")
				if english {
					m.prefs.Language = "en"
				}
				m.Update(size)
				if conversation {
					m.lines = []string{"A conversation"}
				}
				view := m.View()
				box, rows, start, end := m.slashSuggestionsLayout()
				if len(rows) != 2 || end-start != 2 || box.y < bodyTop || box.y+box.height != m.composerBox().y || box.x+box.width > m.width-1 {
					t.Fatal("invalid anchored geometry", box)
				}
				if len(strings.Split(view, "\n")) != m.height {
					t.Fatal("overlay changed terminal height")
				}
				for _, line := range strings.Split(view, "\n") {
					if lipgloss.Width(line) > m.width-1 {
						t.Fatal("overflow")
					}
				}
				lines := strings.Split(ansi.Strip(view), "\n")
				if !strings.Contains(lines[box.y+2], "/plugins") {
					t.Fatal("rendered row differs from target")
				}
				cmd, handled := m.slashSuggestionsMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				if !handled || cmd != nil || m.input.Value() != "/plugins " || m.workbench != nil {
					t.Fatal("click must complete without running")
				}
			}
		}
	}
}
func TestSlashSuggestionsNeverBypassDisabledActionOrTreatPasteAsEnter(t *testing.T) {
	m := slashFixture("/plug")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.workbench != nil || m.sessionBusy {
		t.Fatal("disabled backend action executed")
	}
	m = slashFixture("/")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter"), Paste: true})
	if m.input.Value() != "/enter" || m.sessionBusy || m.chrome.dialog != "" {
		t.Fatal("paste ran a command")
	}
}
