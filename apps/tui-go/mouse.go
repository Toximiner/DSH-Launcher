package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

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
	if !m.terminalFits() || msg.Action != tea.MouseActionPress {
		return nil
	}
	if !m.mouseEnabled() {
		return nil
	}
	if cmd, handled := m.sessionPickerMouse(msg); handled {
		return cmd
	}
	if cmd, handled := m.paletteMouse(msg); handled {
		return cmd
	}
	if cmd, handled := m.tabRailMouse(msg); handled {
		return cmd
	}
	if m.runtimeDialog != nil {
		return nil
	}
	if cmd, handled := m.approvalMouse(msg); handled {
		return cmd
	}
	if cmd, handled := m.queueMouse(msg); handled {
		return cmd
	}
	if m.settingsForm != nil {
		return m.settingsFormMouse(msg)
	}
	if m.composer != nil && m.composer.visible {
		return nil
	}
	if m.workbench != nil {
		d := m.workbench
		if d.pageView && !d.confirm && !d.editing {
			if m.page != d.pageNumber() {
				m.closeWorkbench()
			} else {
				if m.chrome.dialog != "" || m.chrome.menuOpen {
					cmd, _ := m.chromeMouse(msg)
					return cmd
				}
				if msg.Y == 1 {
					cmd, handled := m.chromeMouse(msg)
					if handled {
						return cmd
					}
				}
				if m.sidebarVisible() && msg.X < sidebarWidth {
					cmd, handled := m.sidebarMouse(msg)
					if handled {
						if m.workbench != d && (m.workbench == nil || m.workbench.parent != d) {
							d.cancel()
						} else if m.page != d.pageNumber() {
							m.closeWorkbench()
						}
						return cmd
					}
				}
				return m.benchMouse(msg)
			}
		}
		if m.workbench != nil {
			return m.benchMouse(msg)
		}
	}
	if m.goalDialog != nil {
		return m.goalMouse(msg)
	}
	if m.modelPicker != nil {
		return m.modelsMouse(msg)
	}
	if m.historyReader != nil && m.historyReader.codeDialog != nil {
		return m.historyCodeMouse(msg)
	}
	if m.archiveDialog != nil && m.archiveDialog.deleting {
		return m.sessionDeleteMouse(msg)
	}
	if m.archiveDialog != nil || m.renameDialog != nil || m.forkDialog != nil {
		return nil
	}
	if m.historyReader == nil || m.chrome.menuOpen || m.chrome.dialog != "" {
		if cmd, handled := m.chromeMouse(msg); handled {
			return cmd
		}
	}
	if cmd, handled := m.historyMouse(msg); handled {
		return cmd
	}
	if cmd, handled := m.sidebarMouse(msg); handled {
		return cmd
	}
	mainX := 0
	if m.sidebarVisible() {
		mainX = sidebarWidth + sidebarGap
	}
	conversation := m.conversationArea()
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		if m.pluginsPageBox().contains(msg.X, msg.Y) && m.page == marketPage && m.marketConfirm == nil && !m.marketEditing {
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

	if cmd, handled := m.conversationMouse(msg); handled {
		return cmd
	}
	if cmd, handled := m.reasoningMouse(msg); handled {
		return cmd
	}
	if m.page == chatPage && m.chat != nil && conversation.contains(msg.X, msg.Y) {
		viewLines := strings.Split(clean(m.viewport.View()), "\n")
		screenRow := msg.Y - conversation.y
		if screenRow >= 0 && screenRow < len(viewLines) && strings.Contains(viewLines[screenRow], tr("ОЧЕРЕДЬ DSH ·")) {
			return m.openQueue()
		}
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
	if m.page == sessionsPage && conversation.contains(msg.X, msg.Y) {
		row := msg.Y - conversation.y + m.viewport.YOffset
		for _, hit := range m.sessionHits {
			if hit.row == row {
				m.sessionListIndex = hit.index
				m.navFocused = false
				m.sessionSearching = false
				m.rebuild()
				return nil
			}
		}
	}
	marketBox := m.pluginsPageBox()
	if m.page == marketPage && marketBox.contains(msg.X, msg.Y) {
		x, y := msg.X-marketBox.x, msg.Y-marketBox.y
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
			case "refresh":
				if m.marketConfirm == nil {
					return m.loadMarket()
				}
			case "catalog", "installed":
				if m.marketConfirm == nil {
					m.marketInstalledOnly = hit.kind == "installed"
					m.marketIndex = 0
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
	if cmd, handled := m.landingMouse(msg); handled {
		return cmd
	}
	composer := m.composerBox()
	if m.conversationVisible() && msg.Y == composer.y+conversationComposerHeight {
		hint := ansi.Strip(m.conversationFooter())
		index := strings.Index(hint, tr("[F8 Стоп]"))
		if index < 0 {
			return nil
		}
		x := composer.x + lipgloss.Width(hint[:index])
		if (area{x, msg.Y, len([]rune(tr("[F8 Стоп]"))), 1}).contains(msg.X, msg.Y) {
			return m.cancelChat()
		}
	}
	if !m.conversationVisible() && !m.landingVisible() && m.page == chatPage && msg.Y == composer.y+3 && (m.session == nil || m.session.Address.Kind != "subagent") {
		x := mainX + 2 + len([]rune(tr("Режим: ")))
		for i, name := range []string{tr("Обычный"), tr("План"), tr("Цель")} {
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
