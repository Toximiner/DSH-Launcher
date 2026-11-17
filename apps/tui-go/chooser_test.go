package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestChooserFrameAndMouseGeometryRemainSingleRows(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 40}} {
		m := newModel()
		m.Update(size)
		for _, footer := range []string{"Enter", "reasoning\npermission note\nstatus\nEnter"} {
			spec := chooserSpec{Title: strings.Repeat("Long 世界 ", 30), Search: "Search", Detail: "Details", Footer: footer, Width: 88, Height: 24}
			g := m.chooserLayout(spec)
			row := chooserItem(strings.Repeat("界👋 ", 30), strings.Repeat("provider/model", 100), true, true, g.InnerWidth)
			if lipgloss.Height(row) != 1 || lipgloss.Width(row) > g.InnerWidth {
				t.Fatal("an untrusted model identifier wrapped a selectable row")
			}
			view := m.chooserView(m.View(), spec, []string{row})
			if lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height {
				t.Fatal("chooser frame exceeds terminal")
			}
			lines := strings.Split(clean(view), "\n")
			if !strings.Contains(lines[g.Search.y], "Search") || !strings.Contains(lines[g.List.y], "provider/model") {
				t.Fatal("shared search/list mouse coordinates don't match rendered rows")
			}
			if !strings.Contains(lines[g.Close.y], "esc") {
				t.Fatal("close target not visible")
			}
		}
	}
}
