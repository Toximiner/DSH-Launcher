package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestModelChooserGroupsCurrentCheckAndMeasuredMouse(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, english := range []bool{false, true} {
		activeEnglish.Store(english)
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 40}} {
			m := modelTestPicker(t)
			activeEnglish.Store(english)
			m.width, m.height = size.Width, size.Height
			d := m.modelPicker
			d.catalog.Failures = nil
			group := d.catalog.Groups[0]
			group.ID = "other"
			group.Name = "Other provider"
			group.Models = []catalogModel{{ID: "different", Name: "Next"}}
			d.catalog.Groups = append(d.catalog.Groups, group)

			base := strings.Repeat(strings.Repeat(" ", m.width-1)+"\n", m.height-1)
			for selected, option := range d.options() {
				d.choose(selected)
				view := m.modelsView(base)
				if lipgloss.Width(view) > m.width-1 || lipgloss.Height(view) > m.height {
					t.Fatal("flat model chooser overflowed")
				}
				plain := ansi.Strip(view)
				for _, want := range []string{option.name, option.model.Name, "local/base", "esc"} {
					if !strings.Contains(plain, want) {
						t.Fatalf("missing selected provider/model or current detail %q", want)
					}
				}
				layout := m.modelsLayout()
				for row := layout.first; row < layout.end; row++ {
					// Each hit is checked against the exact freshly rendered window. A
					// previous click may move the selected-centered window independently.
					d.choose(selected)
					current := m.modelsLayout()
					item := current.rows[row]
					before := d.index
					m.modelsMouse(tea.MouseMsg{X: current.geometry.List.x + 1, Y: current.geometry.List.y + row - current.first, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
					if item.heading {
						if d.index != before {
							t.Fatal("provider heading selected a model")
						}
						continue
					}
					if d.index != item.index {
						t.Fatal("rendered row selected another provider/model")
					}
					if d.busy {
						t.Fatal("mouse selection applied a model without Enter")
					}
				}
			}
			d.choose(0)
			if !strings.Contains(ansi.Strip(m.modelsView(base)), "✓") {
				t.Fatal("visible current model lost its checkmark")
			}

			if m.input.Value() != "retained draft" {
				t.Fatal("list navigation changed composer draft")
			}
		}
	}
}

func TestModelChooserLongCatalogAndUnicodeStaySingleRow(t *testing.T) {
	m := modelTestPicker(t)
	m.width, m.height = 80, 24
	d := m.modelPicker
	for i := 0; i < 80; i++ {
		d.catalog.Groups[0].Models = append(d.catalog.Groups[0].Models, catalogModel{ID: fmt.Sprintf("model-%d-", i) + strings.Repeat("id", 100), Name: fmt.Sprintf("Choice %d 世界🙂", i) + strings.Repeat("Long name ", 20)})
	}
	d.choose(len(d.options()) - 1)
	layout := m.modelsLayout()
	lastFound := false
	for _, row := range layout.rows[layout.first:layout.end] {
		if row.index == d.index {
			lastFound = true
		}
	}
	if !lastFound {
		t.Fatal("last selected model hidden by grouped scrolling")
	}
	if len(layout.rows[layout.first:layout.end]) < 2 || !layout.rows[layout.first].heading || layout.rows[layout.first].label != "Local" {
		t.Fatal("scrolled model lost provider identity")
	}
	view := m.modelsView(strings.Repeat("\n", m.height-1))
	if lipgloss.Width(view) > m.width-1 || lipgloss.Height(view) > m.height {
		t.Fatal("long model name/id expanded modal rows")
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.width-1 {
			t.Fatal("Unicode model row overflowed")
		}
	}
	firstItem := -1
	for i := layout.first; i < layout.end; i++ {
		if !layout.rows[i].heading {
			firstItem = i
			break
		}
	}
	if firstItem < 0 {
		t.Fatal("no visible model rows")
	}
	expected := layout.rows[firstItem].index
	m.modelsMouse(tea.MouseMsg{X: layout.geometry.List.x, Y: layout.geometry.List.y + firstItem - layout.first, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if d.index != expected {
		t.Fatal("scrolled click used catalog index as display row")
	}
	m.modelsKey(tea.KeyMsg{Type: tea.KeyPgDown})
	if d.index <= expected {
		t.Fatal("PageDown failed to advance model selection")
	}
}

func TestModelChooserReasoningNamesAndExplicitF2(t *testing.T) {
	m := modelTestPicker(t)
	d := m.modelPicker
	d.choose(1)
	if !strings.Contains(ansi.Strip(m.modelsView(strings.Repeat("\n", m.height-1))), tr("По умолчанию")+" · High") {
		t.Fatal("catalog default reasoning not identified")
	}
	m.modelsKey(tea.KeyMsg{Type: tea.KeyF2})
	if d.effort != "off" || !strings.Contains(ansi.Strip(m.modelsView(strings.Repeat("\n", m.height-1))), tr("Рассуждение: ")+"Off") {
		t.Fatal("F2 did not select explicit catalog effort")
	}
	m.modelsKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	if d.effort != "" {
		t.Fatal("reverse effort cycle did not restore default")
	}
	d.choose(0)
	if !strings.Contains(ansi.Strip(m.modelsView(strings.Repeat("\n", m.height-1))), tr("Рассуждение: ")+"—") {
		t.Fatal("unsupported reasoning was presented as selectable")
	}
}

func TestModelChooserEscMouseCancelsReadButNotWrite(t *testing.T) {
	m := modelTestPicker(t)
	d := m.modelPicker
	g := m.modelsLayout().geometry
	d.busy, d.writing = true, true
	m.modelsMouse(tea.MouseMsg{X: g.Close.x, Y: g.Close.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.modelPicker != d {
		t.Fatal("closing picker abandoned an active write")
	}
	d.writing = false
	m.modelsMouse(tea.MouseMsg{X: g.Close.x, Y: g.Close.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.modelPicker != nil || d.ctx.Err() == nil || m.input.Value() != "retained draft" {
		t.Fatal("closing read did not cancel cleanly")
	}
}

func TestModelChooserSearchResizeAndUncertainWriteStaySafe(t *testing.T) {
	m := modelTestPicker(t)
	d := m.modelPicker
	d.search.SetValue(strings.Repeat("世界", 60) + "TAIL")
	d.search.CursorEnd()
	cursor := d.search.Position()
	for _, width := range []int{160, 80, 100} {
		m.width = width
		view := m.modelsView(strings.Repeat("\n", m.height-1))
		if lipgloss.Width(view) > width-1 || lipgloss.Height(view) > m.height {
			t.Fatal("search reflow expanded the picker")
		}
		if d.search.Position() != cursor || d.search.Value() != strings.Repeat("世界", 60)+"TAIL" {
			t.Fatal("search rendering changed query or cursor")
		}
	}
	d.search.SetValue("")
	d.choose(1)
	d.uncertain = true
	d.note = tr("Выбор не подтверждён. F5 — перечитать; автоматического повтора нет.")
	layout := m.modelsLayout()
	for row := layout.first; row < layout.end; row++ {
		if !layout.rows[row].heading {
			m.modelsMouse(tea.MouseMsg{X: layout.geometry.List.x, Y: layout.geometry.List.y + row - layout.first, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		}
	}
	if m.applyModel() != nil || d.busy || !d.uncertain {
		t.Fatal("model browsing retried uncertain mutation")
	}
}
