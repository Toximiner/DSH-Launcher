package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDecisionShortcutRevealsPendingRequestWithoutAnswer(t *testing.T) {
	for _, picker := range []bool{false, true} {
		for _, visible := range []bool{false, true} {
			m := approvalModel(t)
			m.input.SetValue("unsent draft")
			if picker {
				m.openSessionPicker()
			} else {
				m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
			}
			requestApproval(m, "incoming", "s")
			m.approvals.visible = visible
			m.Update(tea.KeyMsg{Type: tea.KeyF10})
			answers := 0
			m.approvals.answer = func(string, string) error { answers++; return nil }
			m.Update(tea.KeyMsg{Type: tea.KeyF7})
			if m.sessionPicker != nil || m.palette != nil && m.palette.open || !m.approvals.visible {
				t.Fatalf("F7 did not reveal request (picker=%v, visible=%v)", picker, visible)
			}
			m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if answers != 0 || m.approvals.busy || m.approvals.selection != -1 || m.input.Value() != "unsent draft" {
				t.Fatal("opening request answered it or changed draft")
			}
		}
	}
}
