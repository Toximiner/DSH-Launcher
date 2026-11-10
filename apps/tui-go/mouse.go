package main

import tea "github.com/charmbracelet/bubbletea"

const (
	sidebarWidth   = 24
	sidebarGap     = 1
	bodyTop        = 2
	composerHeight = 6
)

type area struct{ x, y, width, height int }

func (a area) contains(x, y int) bool {
	return x >= a.x && x < a.x+a.width && y >= a.y && y < a.y+a.height
}

func (m *model) mouse(msg tea.MouseMsg) tea.Cmd {
	if m.width < 44 || m.height < 18 || msg.Action != tea.MouseActionPress {
		return nil
	}
	width, bodyHeight := m.contentSize()
	mainX := 0
	if m.width-1 >= 78 {
		mainX = sidebarWidth + sidebarGap
	}
	conversation := area{mainX + 2, bodyTop + 3, m.viewport.Width, m.viewport.Height}
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		if conversation.contains(msg.X, msg.Y) {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return cmd
		}
		return nil
	}
	if msg.Button != tea.MouseButtonLeft {
		return nil
	}
	if mainX != 0 {
		// Sidebar content starts below its border; four rows precede navigation.
		navigation := area{1, bodyTop + 1 + 4, sidebarWidth - 2, len(pages)}
		if navigation.contains(msg.X, msg.Y) {
			m.page = msg.Y - navigation.y
			m.navFocused = true
			m.input.Blur()
			m.resize()
			m.viewport.GotoTop()
			return nil
		}
		// A blank row and session heading follow navigation.
		draft := area{1, navigation.y + len(pages) + 2, sidebarWidth - 2, 1}
		if draft.y < bodyTop+bodyHeight-3 && draft.contains(msg.X, msg.Y) {
			return m.showChat()
		}
	}
	composer := area{mainX, bodyTop + bodyHeight - composerHeight - 1, width, composerHeight}
	if m.page == chatPage && msg.Y == composer.y+2 {
		x := mainX + 2 + len([]rune("Режим: "))
		for i, name := range []string{"Обычный", "План", "Цель"} {
			width := len([]rune(name)) + 2
			if (area{x, msg.Y, width, 1}).contains(msg.X, msg.Y) {
				return m.chooseMode([]string{"default", "plan", "goal"}[i])
			}
			x += width + 1
		}
	}
	if m.page == 0 && composer.contains(msg.X, msg.Y) {
		m.navFocused = false
		return m.input.Focus()
	}
	return nil
}
