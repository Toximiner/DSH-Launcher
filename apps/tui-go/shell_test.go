package main

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
)

func paletteTestSelect(t *testing.T, m *model, label string) tea.Cmd {
	t.Helper()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.palette == nil || !m.palette.open {
		t.Fatalf("palette unavailable for %s", label)
	}
	m.palette.search.SetValue(label)
	m.paletteClampOffset()
	for i, row := range m.paletteFiltered() {
		if row.kind == "item" && paletteCatalog[row.index].label == label {
			m.palette.offset = i
			_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			return cmd
		}
	}
	t.Fatalf("missing palette action %s", label)
	return cmd
}

func TestResizeKeepsConversationTailOrReadingPosition(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	for i := 0; i < 20; i++ {
		m.append(fmt.Sprintf("message-%02d with a sufficiently long line that wraps at the narrow terminal boundary", i))
	}
	m.viewport.GotoBottom()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if !strings.Contains(clean(m.viewport.View()), "message-19") || !m.viewport.AtBottom() {
		t.Fatal("resize lost conversation tail")
	}
	m.viewport.GotoTop()
	m.viewport.LineDown(2)
	before := m.viewport.YOffset
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.viewport.YOffset != before {
		t.Fatal("resize jumped away from reading position")
	}
}
