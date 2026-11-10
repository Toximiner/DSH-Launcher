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
	if !m.mouseEnabled() {
		return nil
	}
	if cmd, handled := m.chromeMouse(msg); handled {
		return cmd
	}
	width, bodyHeight := m.contentSize()
	mainX := 0
	if m.width-1 >= 78 {
		mainX = sidebarWidth + sidebarGap
	}
	conversation := area{mainX + 2, bodyTop + 3, m.viewport.Width, m.viewport.Height}
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		if conversation.contains(msg.X, msg.Y) && m.page == marketPage && m.marketConfirm == nil && !m.marketEditing {
			delta := 1
			if msg.Button == tea.MouseButtonWheelUp {
				delta = -1
			}
			m.marketIndex = max(0, min(max(0, len(m.marketRows())-1), m.marketIndex+delta))
			m.rebuild()
			return nil
		}
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
		navigation := area{1, bodyTop + 1 + 4, sidebarWidth - 2, len(navigationPages)}
		if navigation.contains(msg.X, msg.Y) {
			m.page = navigationPages[msg.Y-navigation.y]
			m.toolFocused = false
			m.navFocused = true
			m.input.Blur()
			m.resize()
			m.viewport.GotoTop()
			if m.page == marketPage {
				m.navFocused = false
				return m.loadMarket()
			}
			return nil
		}
		// A blank row and session heading follow navigation.
		draft := area{1, navigation.y + len(navigationPages) + 2, sidebarWidth - 2, 1}
		if draft.y < bodyTop+bodyHeight-3 && draft.contains(msg.X, msg.Y) {
			return m.showChat()
		}
	}
	if m.page == chatPage && m.chat != nil && conversation.contains(msg.X, msg.Y) {
		row := msg.Y - conversation.y + m.viewport.YOffset
		for _, hit := range m.chatToolHits {
			if row >= hit.row && row < hit.end {
				m.toolFocused = true
				m.navFocused = false
				m.chat.selectedTool = hit.id
				m.input.Blur()
				m.toggleTool(hit.id)
				return nil
			}
		}
	}
	if m.page == marketPage && conversation.contains(msg.X, msg.Y) {
		x, y := msg.X-conversation.x, msg.Y-conversation.y
		for _, hit := range m.marketHits {
			if !hit.box.contains(x, y) {
				continue
			}
			m.navFocused = false
			switch hit.kind {
			case "yes":
				return m.executeMarket()
			case "no":
				m.marketConfirm = nil
			case "row":
				if m.marketConfirm == nil {
					m.marketIndex = hit.index
				}
			case "tab":
				if m.marketConfirm == nil {
					m.marketInstalledOnly = !m.marketInstalledOnly
					m.marketIndex = 0
				}
			case "search":
				if m.marketConfirm == nil {
					m.marketEditing = true
					cmd := m.marketSearch.Focus()
					m.rebuild()
					return cmd
				}
			default:
				m.confirmMarket(hit.kind)
			}
			m.rebuild()
			return nil
		}
	}
	composer := area{mainX, bodyTop + bodyHeight - composerHeight - 1, width, composerHeight}
	if m.page == chatPage && m.session != nil && !m.browserPreview && msg.Y == composer.y+4 {
		x := mainX + 2 + len([]rune("Enter — отправить · "))
		if (area{x, msg.Y, len([]rune("[F8 Стоп]")), 1}).contains(msg.X, msg.Y) {
			return m.cancelChat()
		}
	}
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
		m.toolFocused = false
		m.rebuild()
		return m.input.Focus()
	}
	return nil
}
