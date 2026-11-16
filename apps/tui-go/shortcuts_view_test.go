package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestShortcutGroupsCoverEveryContextOnce(t *testing.T) {
	contexts := map[string]int{}
	for _, group := range shortcutGroups {
		for _, context := range group.contexts {
			contexts[context]++
		}
	}
	for _, s := range tuiShortcuts {
		if contexts[s.context] != 1 || shortcutSection(s) == 0 {
			t.Fatalf("shortcut %s has a missing or ambiguous section: %s", s.keys, s.context)
		}
	}
	_, count := shortcutContent("", 0, 73)
	if count != len(tuiShortcuts) {
		t.Fatal("grouping lost or duplicated shortcuts")
	}
	for group := 1; group <= len(shortcutGroups); group++ {
		lines, count := shortcutContent("", group, 73)
		if count == 0 {
			t.Fatal("section contains no shortcuts")
		}
		for _, line := range lines {
			if line.group != group {
				t.Fatal("another section leaked into the filtered view")
			}
		}
	}
}

func TestShortcutHelpScrollSearchAndResize(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, english := range []bool{false, true} {
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 42}} {
			m := newModel()
			if english {
				m.prefs.Language = "en"
			}
			activeEnglish.Store(english)
			m.Update(size)
			m.input.SetValue("retained draft")
			m.openDialog("help")
			for group := 0; group <= len(shortcutGroups); group++ {
				m.chrome.helpGroup = group
				key(m, tea.KeyEnd)
				box, lines, _ := m.shortcutHelpLayout()
				view := m.View()
				if box.y+box.height > size.Height || lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height {
					t.Fatal("shortcut popup overflowed the terminal")
				}
				all, _ := shortcutContent("", group, box.width-4)
				last := clean(all[len(all)-1].text)
				if !strings.Contains(clean(strings.Join(lines, "\n")), last) {
					t.Fatalf("End left the final shortcut off screen (group=%d, English=%v, size=%+v, offset=%d, last=%q):\n%s", group, english, size, m.chrome.helpOffset, last, clean(strings.Join(lines, "\n")))
				}
				key(m, tea.KeyHome)
				m.dialogBox()
				if m.chrome.helpOffset != 0 {
					t.Fatal("Home did not restore the start")
				}
			}
			m.chrome.helpGroup = 0
			query := "расписания"
			if english {
				query = "Schedules"
			}
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(query), Paste: true})
			key(m, tea.KeyEnd)
			_, view := m.dialogBox()
			if !strings.Contains(clean(view), "Ctrl+G") || strings.Contains(clean(view), "Ctrl+]") {
				t.Fatal("search did not preserve its context filter")
			}
			key(m, tea.KeyCtrlU)
			key(m, tea.KeyRight)
			if m.chrome.helpGroup != 1 || m.chrome.helpOffset != 0 {
				t.Fatal("section change did not reset scrolling")
			}
			key(m, tea.KeyLeft)
			key(m, tea.KeyLeft)
			if m.chrome.helpGroup != len(shortcutGroups) {
				t.Fatal("section cycling did not wrap")
			}
			key(m, tea.KeyEsc)
			if m.input.Value() != "retained draft" || !m.input.Focused() {
				t.Fatal("shortcut navigation changed composer context")
			}
		}
	}
}

func TestShortcutHelpMouseAndStickyHeading(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.openDialog("help")
	box, _, hits := m.shortcutHelpLayout()
	for _, hit := range hits {
		if hit.action == "next" {
			mouse(m, hit.x, hit.y, tea.MouseButtonLeft, tea.MouseActionPress)
		}
	}
	if m.chrome.helpGroup != 1 {
		t.Fatal("section button missed its visible target")
	}
	m.chrome.helpOffset = 4
	_, lines, _ := m.shortcutHelpLayout()
	if !strings.Contains(clean(lines[5]), tr(shortcutGroups[0].title)) {
		t.Fatal("scrolling lost the current section heading")
	}
	mouse(m, box.x+4, box.y+8, tea.MouseButtonWheelUp, tea.MouseActionPress)
	if m.chrome.helpOffset >= 4 {
		t.Fatal("mouse wheel did not scroll help")
	}
	_, _, hits = m.shortcutHelpLayout()
	for _, hit := range hits {
		if hit.action == "close" {
			mouse(m, hit.x, hit.y, tea.MouseButtonLeft, tea.MouseActionPress)
			break
		}
	}
	if m.chrome.dialog != "" {
		t.Fatal("close button missed its visible target")
	}
}
