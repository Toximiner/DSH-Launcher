package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestPluginsCompactRowsAndSelectedDetailsAcrossSizes(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, english := range []bool{false, true} {
		for _, modal := range []bool{false, true} {
			for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 40}} {
				m := pluginTestModel(t)
				activeEnglish.Store(english)
				m.Update(size)
				m.workbench.pageView = !modal
				rows := m.workbench.rows
				for i := range rows {
					rows[i].Body = fmt.Sprintf("ONLY_DESCRIPTION_%02d long text 世界", i)
				}
				m.workbench.rows = rows
				m.workbench.index = 8
				box := m.pluginsBox()
				lines, hits := m.pluginsLayout(box)
				if len(lines) > box.height {
					t.Fatalf("plugin lines overflow %v modal%v: %d>%d", size, modal, len(lines), box.height)
				}
				for _, line := range lines {
					if lipgloss.Width(line) > box.width {
						t.Fatal("plugin line overflow")
					}
				}
				text := ansi.Strip(strings.Join(lines, "\n"))
				if !strings.Contains(text, "ONLY_DESCRIPTION_08") || strings.Contains(text, "ONLY_DESCRIPTION_07") || strings.Count(text, "ONLY_DESCRIPTION_") != 1 {
					t.Fatal("list repeated every description or lost selected details")
				}
				if !strings.Contains(text, "Theta") || !strings.Contains(text, "1.0.0") {
					t.Fatal("selected identity/version hidden")
				}
				selected := false
				for _, hit := range hits {
					if hit.x < box.x || hit.x+hit.width > box.x+box.width || hit.y < box.y || hit.y+hit.height > box.y+box.height {
						t.Fatal("plugin hit escaped rendered area")
					}
					if hit.action == "card" {
						if hit.height != 1 {
							t.Fatal("compact row retained old description hit area")
						}
						if hit.index == 8 {
							selected = true
						}
					}
					if hit.action == "toggle" {
						visible := ansi.Strip(ansi.Cut(lines[hit.y-box.y], hit.x-box.x, hit.x-box.x+hit.width))
						if !strings.HasPrefix(visible, "[") || !strings.HasSuffix(visible, "]") {
							t.Fatalf("toggle %v did not match visible state %q", hit, visible)
						}
					}
				}
				if !selected {
					t.Fatal("selected row missing from visible list")
				}
			}
		}
	}
}

func TestPluginsCompactStickyGroupAndUnicodeToggleHits(t *testing.T) {
	m := pluginTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	rows := []benchRow{}
	for i := 0; i < 40; i++ {
		rows = append(rows, benchRow{ID: fmt.Sprintf("plugin-%02d", i), Title: fmt.Sprintf("模型世界 é %02d", i), Body: "Selected description", Data: map[string]any{"pluginGroup": "installed", "enabled": false}})
	}
	m.workbench.rows = rows
	m.workbench.index = 39
	box := m.pluginsBox()
	lines, hits := m.pluginsLayout(box)
	if !strings.Contains(ansi.Strip(lines[4]), tr("Установленные плагины")) {
		t.Fatal("selected provider group was not pinned while scrolling")
	}
	var selectedToggle *pluginHit
	for i := range hits {
		hit := &hits[i]
		if hit.action == "toggle" && hit.index == 39 {
			selectedToggle = hit
			break
		}
	}
	if selectedToggle == nil {
		t.Fatal("last row toggle hidden")
	}
	hit := *selectedToggle
	if visible := ansi.Strip(ansi.Cut(lines[hit.y-box.y], hit.x-box.x, hit.x-box.x+hit.width)); visible != "["+tr("Выкл")+"]" {
		t.Fatalf("Unicode title shifted toggle cells: %q", visible)
	}
	mouse(m, hit.x, hit.y, tea.MouseButtonLeft, tea.MouseActionPress)
	if !m.workbench.confirm || m.workbench.action.Args["name"] != "plugin-39" {
		t.Fatal("compact toggle prepared wrong exact package")
	}
}

func TestPluginsCompactMoreHeadingAndEmptySearchHaveNoPhantomCards(t *testing.T) {
	m := pluginTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.workbench.rows = []benchRow{}
	for i := 0; i < 30; i++ {
		m.workbench.rows = append(m.workbench.rows, benchRow{ID: fmt.Sprintf("experimental-%02d", i), Title: fmt.Sprintf("Optional %02d", i), Data: map[string]any{"pluginGroup": "experimental", "enabled": false}})
	}
	m.workbench.index = 29
	box := m.pluginsBox()
	lines, hits := m.pluginsLayout(box)
	found := false
	for _, hit := range hits {
		if hit.action == "more" {
			found = true
			if hit.y != box.y+4 || !strings.Contains(ansi.Strip(lines[4]), tr("Ещё · Ctrl+O")) {
				t.Fatal("More target did not follow pinned group heading")
			}
		}
	}
	if !found {
		t.Fatal("pinned experimental group lost More action")
	}
	m.workbench.search.SetValue("NO_SUCH_PLUGIN")
	lines, hits = m.pluginsLayout(box)
	if !strings.Contains(ansi.Strip(strings.Join(lines, "\n")), tr("Нет плагинов по этому запросу.")) {
		t.Fatal("empty search missing")
	}
	for _, hit := range hits {
		if hit.action == "card" || hit.action == "toggle" || hit.action == "more" {
			t.Fatal("empty results retained actionable phantom row")
		}
	}
}
