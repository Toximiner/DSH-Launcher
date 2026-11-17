package main

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestReasoningOnlyRecordDoesNotMaskLastReplyCopy(t *testing.T) {
	m := reasoningFixture(t)
	if err := m.applyChatFrame(m.chat, &chatFrame{Type: "event", Event: reasoningEvent(2, "", "actual thoughts only")}); err != nil {
		t.Fatal(err)
	}
	m.rebuild()
	if paletteLastAssistantSeq(m) != 1 {
		t.Fatal("reasoning-only record masked last reply in palette")
	}
	var copied string
	m.copyClipboard = func(_ context.Context, source string) (string, error) { copied = source; return "copied", nil }
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	if cmd == nil {
		t.Fatal("last reply copy unavailable")
	}
	cmd()
	if copied != "# Authoritative reply" {
		t.Fatal("reasoning-only record masked or changed copied reply")
	}
}

func TestHistoryReasoningHeaderTracksSelectionAndSearchLabels(t *testing.T) {
	for _, width := range []int{80, 100, 160} {
		m := reasoningFixture(t)
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		m.openHistory()
		h := m.historyReader
		h.busy = false
		h.projection = m.chat
		h.selectedSeq = 1
		m.resizeHistory()
		if len(h.projection.reasoningHits) != 1 {
			t.Fatal("history reasoning header missing")
		}
		h.viewport.GotoTop()
		box := m.historyBox()
		lines := strings.Split(clean(m.View()), "\n")
		row := box.y + 4 + h.projection.reasoningHits[0].row
		if !strings.Contains(lines[row], tr("Размышления")) {
			t.Fatal("history reasoning hit ignored selection prefix")
		}
		m.historyMouse(tea.MouseMsg{X: box.x + 3, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if !h.projection.expandedReasoning[reasoningEventKey(1)] {
			t.Fatal("visible history thought header did not expand")
		}
		m.historyKey(tea.KeyMsg{Type: tea.KeyCtrlO})
		if h.projection.expandedReasoning[reasoningEventKey(1)] {
			t.Fatal("history keyboard could not collapse thoughts")
		}
		h.search = &historySearch{hitSeq: 1, query: "Authoritative"}
		m.resizeHistory()
		h.viewport.GotoTop()
		lines = strings.Split(clean(m.View()), "\n")
		row = box.y + 4 + h.projection.reasoningHits[0].row
		if !strings.Contains(lines[row], tr("Размышления")) {
			t.Fatal("history reasoning hit ignored search prefix")
		}
	}
}
