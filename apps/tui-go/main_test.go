package main

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func enter(m *model, value string) tea.Cmd {
	m.input.SetValue(value)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return cmd
}

func TestLocalCommandsAndHistory(t *testing.T) {
	m := newModel()
	enter(m, "Привет 世界")
	if !strings.Contains(strings.Join(m.lines, "\n"), "Введено: Привет 世界") {
		t.Fatal("Unicode input was lost")
	}
	m.input.SetValue("draft")
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.input.Value() != "Привет 世界" {
		t.Fatal("history did not restore prior input")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.input.Value() != "draft" {
		t.Fatal("history did not restore draft")
	}
	enter(m, "/help")
	if !strings.Contains(m.lines[len(m.lines)-1], "/clear") {
		t.Fatal("help was not displayed")
	}
	enter(m, "/clear")
	if len(m.lines) != 0 {
		t.Fatal("clear left old lines")
	}
	if cmd := enter(m, "/exit"); cmd == nil {
		t.Fatal("exit did not return a command")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("exit did not quit")
	}
}

func TestPasteCannotExecuteCommand(t *testing.T) {
	m := newModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/exit\r"), Paste: true})
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("pasted input executed a command")
		}
	}
	if !strings.Contains(m.input.Value(), "/exit") {
		t.Fatal("paste was lost")
	}
}

func TestSanitizationAndResize(t *testing.T) {
	if got := clean("A\x1b[2JB\x1b]52;c;secret\x07\x00C"); got != "ABC" {
		t.Fatalf("unsafe output: %q", got)
	}
	m := newModel()
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 30}, {Width: 80, Height: 24}, {Width: 10, Height: 5}} {
		m.Update(size)
		if m.viewport.Width < 1 || m.viewport.Height < 1 || m.View() == "" {
			t.Fatal("invalid layout after resize")
		}
	}
}

func TestCLIWithoutTerminal(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--help"}, &out); err != nil || !strings.Contains(out.String(), "dsh-launcher-tui-go") {
		t.Fatal("help failed", err)
	}
	out.Reset()
	if err := run([]string{"--version"}, &out); err != nil || !strings.Contains(out.String(), version) {
		t.Fatal("version failed", err)
	}
	if err := run([]string{"--unknown"}, &out); err == nil {
		t.Fatal("unknown option was accepted")
	}
}
