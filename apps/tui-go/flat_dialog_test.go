package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestSettingsFormFlatSurfaceAndRowTargets(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {100, 30}, {160, 40}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := formTestModel(t)
			m.width, m.height = size[0], size[1]
			m.resize()
			d := m.settingsForm
			d.path = []string{"providers", "local"}
			d.index = 0
			box := m.settingsFormBox()
			view := m.settingsFormView(strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", m.width-1)+"\n", m.height), "\n"))
			lines := strings.Split(view, "\n")
			if len(lines) != m.height {
				t.Fatalf("height %d", len(lines))
			}
			for _, line := range lines {
				if lipgloss.Width(line) > m.width-1 {
					t.Fatal("overflow")
				}
			}
			if strings.Contains(view, "NEVER DISPLAY") {
				t.Fatal("secret leaked")
			}
			close := flatDialogClose(box)
			if !strings.Contains(lipgloss.NewStyle().Render(lines[close.y]), "esc") {
				t.Fatal("close target missing")
			}
			body := m.settingsFormListArea()
			rows := d.rows()
			if len(rows) == 0 || !strings.Contains(lines[body.y], rows[0].key) {
				t.Fatal("row target differs from drawing")
			}
			m.settingsFormMouse(tea.MouseMsg{X: body.x, Y: body.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if d.field == nil || d.field.row.key != rows[0].key {
				t.Fatal("click opened wrong field")
			}
		})
	}
}

func TestSettingsFormHeaderCloseRetainsWriteAndDiscardGuards(t *testing.T) {
	for _, state := range []string{"clean", "draft", "writing", "field"} {
		t.Run(state, func(t *testing.T) {
			m := formTestModel(t)
			d := m.settingsForm
			d.path = nil
			switch state {
			case "draft":
				d.stage([]string{"providers", "local", "baseURL"}, "http://new", false)
			case "writing":
				d.busy = true
				d.writing = true
			case "field":
				row := selectFormPath(t, m, []string{"providers", "local", "baseURL"})
				d.edit(row, false)
			}
			close := flatDialogClose(m.settingsFormBox())
			m.settingsFormMouse(tea.MouseMsg{X: close.x, Y: close.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			switch state {
			case "clean":
				if m.settingsForm != nil {
					t.Fatal("clean form did not close")
				}
			case "draft":
				if m.settingsForm != d || !d.discard || len(d.ops) != 1 {
					t.Fatal("staged edits silently discarded")
				}
			case "writing":
				if m.settingsForm != d || !d.busy || !d.writing {
					t.Fatal("active write interrupted")
				}
			case "field":
				if m.settingsForm != d || d.field != nil || len(d.ops) != 0 {
					t.Fatal("field close must cancel only input")
				}
			}
		})
	}
}
