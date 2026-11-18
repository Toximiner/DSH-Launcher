package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func mouse(m *model, x, y int, button tea.MouseButton, action tea.MouseAction) {
	m.Update(tea.MouseMsg{X: x, Y: y, Button: button, Action: action})
}

func TestMouseNavigationAndComposer(t *testing.T) {
	for _, width := range []int{100, 80} {
		m := newModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		m.input.SetValue("draft 世界")
		// The lower More menu opens settings without exposing them in the tree.
		_, height := m.contentSize()
		moreY := bodyTop + 1 + height - 3
		mouse(m, 5, moreY, tea.MouseButtonLeft, tea.MouseActionPress)
		if m.chrome.dialog != "more" {
			t.Fatal("bottom More did not open")
		}
		box, _ := m.dialogBox()
		mouse(m, box.x+3, box.y+4, tea.MouseButtonLeft, tea.MouseActionPress)
		if m.page != dshSettingsPage || m.chrome.dialog != "" {
			t.Fatal("More did not open dsh settings")
		}
		mouse(m, 5, 13, tea.MouseButtonLeft, tea.MouseActionPress)
		if m.page != chatPage || !m.input.Focused() || m.input.Value() != "draft 世界" {
			t.Fatal("clicking the local draft lost input or failed to focus chat")
		}
		key(m, tea.KeyTab)
		mouse(m, m.composerBox().x+3, m.composerBox().y+1, tea.MouseButtonLeft, tea.MouseActionPress)
		if m.navFocused || !m.input.Focused() || m.input.Value() != "draft 世界" {
			t.Fatal("composer click did not restore input focus")
		}
	}
}

func TestMouseScrollAndIgnoredEvents(t *testing.T) {
	for _, width := range []int{120, 100} {
		m := newModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		m.append(strings.Repeat("scrollable content\n", 100))
		bottom := m.viewport.YOffset
		mainX := 0
		if width >= 80 {
			mainX = 25
		}
		mouse(m, mainX+3, 6, tea.MouseButtonWheelUp, tea.MouseActionPress)
		if m.viewport.YOffset >= bottom {
			t.Fatal("wheel over content did not scroll up")
		}
		mouse(m, mainX+3, 6, tea.MouseButtonWheelDown, tea.MouseActionPress)
		if m.viewport.YOffset != bottom {
			t.Fatal("wheel did not return to bottom")
		}
		for _, event := range []tea.MouseMsg{
			{X: mainX + 3, Y: 25, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress},
			{X: -1, Y: 6, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress},
			{X: 5, Y: 8, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease},
			{X: 5, Y: 8, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion},
			{X: 5, Y: 8, Button: tea.MouseButtonRight, Action: tea.MouseActionPress},
		} {
			m.Update(event)
		}
		if m.page != 0 || m.navFocused || m.viewport.YOffset != bottom {
			t.Fatal("an unrelated mouse event changed navigation or scroll")
		}
		if width >= 80 {
			mouse(m, 5, 8, tea.MouseButtonWheelUp, tea.MouseActionPress)
			if m.viewport.YOffset != bottom {
				t.Fatal("wheel over sidebar scrolled content")
			}
		}
		m.input.Blur()
		mouse(m, m.composerBox().x+3, m.composerBox().y+1, tea.MouseButtonLeft, tea.MouseActionPress)
		if !m.input.Focused() {
			t.Fatal("composer did not focus after resize")
		}
	}
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	mouse(m, 5, 8, tea.MouseButtonLeft, tea.MouseActionPress)
	if m.page != 0 {
		t.Fatal("mouse navigated an invisible layout")
	}
}

func TestHiddenDraftDoesNotCaptureSidebarStatusClick(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 18})
	m.page = logPage
	m.resize()
	mouse(m, 5, 14, tea.MouseButtonLeft, tea.MouseActionPress)
	if m.page != logPage {
		t.Fatal("hidden draft captured a click on sidebar status")
	}
}
