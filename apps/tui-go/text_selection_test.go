package main

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestTextSelectionFreezesViewButKeepsReceivingChat(t *testing.T) {
	m := reasoningFixture(t)
	m.input.SetValue("preserved draft")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF10})
	if cmd == nil || reflect.TypeOf(cmd()) != reflect.TypeOf(tea.DisableMouse()) || m.mouseEnabled() {
		t.Fatal("selection did not release terminal mouse reporting")
	}
	frozen := m.View()
	if !strings.Contains(clean(frozen), tr("Выделение: мышь · Ctrl+Shift+C копия · Esc/F10 назад")) {
		t.Fatal("selection mode has no exit/copy instructions")
	}
	applyChat(t, m, `{"type":"event","event":{"seq":2,"type":"assistant/message","data":{"message":{"content":[{"type":"text","text":"LATER_REPLY"}]}}}}`)
	m.rebuild()
	if m.View() != frozen || m.chat.cursor != 2 || !strings.Contains(m.chatView(), "LATER_REPLY") {
		t.Fatal("selection repainted the frame or stopped receiving messages")
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyRunes, Runes: []rune("pasted text"), Paste: true}, {Type: tea.KeyCtrlN}} {
		m.Update(key)
	}
	if m.input.Value() != "preserved draft" || len(m.prompts) != 0 || m.sessionBusy {
		t.Fatal("selection input reached the underlying composer")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.selectionSnapshot != "" || !m.mouseEnabled() || cmd == nil || reflect.TypeOf(cmd()) != reflect.TypeOf(tea.EnableMouseCellMotion()) || !strings.Contains(clean(m.View()), "LATER_REPLY") {
		t.Fatal("leaving selection did not restore mouse and current view")
	}
}

func TestTextSelectionProtectsDecisionsAndRestoresMousePreference(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		for _, forced := range []bool{false, true} {
			m := approvalModel(t)
			m.prefs.Mouse, m.mouseForcedOff = mouse, forced
			requestApproval(m, "pending", "s")
			m.approvals.selection = 1
			m.Update(tea.KeyMsg{Type: tea.KeyF10})
			for _, key := range []tea.KeyMsg{{Type: tea.KeyTab}, {Type: tea.KeyEnter}, {Type: tea.KeyF5}} {
				_, cmd := m.Update(key)
				if cmd != nil || m.approvals.busy {
					t.Fatal("selection key answered a hidden permission")
				}
			}
			box := m.approvalBox()
			if cmd := m.mouse(tea.MouseMsg{X: box.x + box.width - 5, Y: box.y + box.height - 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}); cmd != nil || m.approvals.busy {
				t.Fatal("selection gesture reached a hidden permission")
			}
			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF7})
			wantMouse := mouse && !forced
			wantCmd := tea.DisableMouse
			if wantMouse {
				wantCmd = tea.EnableMouseCellMotion
			}
			if m.selectionSnapshot != "" || !m.approvals.visible || m.approvals.busy || m.prefs.Mouse != mouse || m.mouseForcedOff != forced || m.mouseEnabled() != wantMouse || reflect.TypeOf(cmd()) != reflect.TypeOf(wantCmd()) {
				t.Fatal("selection changed preferences or answered/deferred the pending request")
			}
		}
	}
}

func TestTextSelectionResizeAndLiteralKeys(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, language := range []string{"ru", "en"} {
		m := reasoningFixture(t)
		m.prefs.Language = language
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 40}} {
			m.Update(size)
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f10")})
			if m.selectionSnapshot != "" {
				t.Fatal("literal named key activated selection")
			}
			m.Update(tea.KeyMsg{Type: tea.KeyF10})
			if lipgloss.Width(m.View()) > size.Width || lipgloss.Height(m.View()) > size.Height {
				t.Fatal("selection hint overflowed terminal")
			}
			_, cmd := m.Update(size)
			if m.selectionSnapshot != "" || cmd == nil || !m.mouseEnabled() {
				t.Fatal("resize retained invalid frozen geometry or disabled mouse")
			}
		}
	}
}
