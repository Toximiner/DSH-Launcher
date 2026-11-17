package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestPendingPromptKeepsReceiptUncertaintyAndRetryVisible(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, english := range []bool{false, true} {
		activeEnglish.Store(english)
		labels := map[string]bool{}
		for _, status := range []string{"sending", "accepted", "unknown", "rejected"} {
			label := promptDeliveryLabel(status)
			if labels[label] {
				t.Fatal("distinct delivery outcomes look identical")
			}
			labels[label] = true
			view := pendingPromptView(strings.Repeat("Привет 世界 👋 ", 20)+"\x1b[31m", status, 75)
			if lipgloss.Width(view) > 75 {
				t.Fatal("pending text overflows conversation")
			}
			if strings.Contains(clean(view), "[31m") {
				t.Fatal("terminal escape shown as message content")
			}
			if status == "unknown" && !strings.Contains(clean(view), "/retry-send") {
				t.Fatal("uncertain delivery lost explicit retry guidance")
			}
		}
	}
}

func TestConversationFooterKeepsReadonlyAndQueueGuidance(t *testing.T) {
	m := conversationModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.session.Running = true
	m.chat.copyNote = strings.Repeat("copied ", 40)
	if !strings.Contains(clean(m.conversationFooter()), "Enter — в очередь") {
		t.Fatal("running session lost enqueue hint")
	}
	m.session.Address.Kind = "subagent"
	if !strings.Contains(clean(m.conversationFooter()), "/agents") {
		t.Fatal("subagent lost readonly guidance")
	}
	if !strings.Contains(clean(m.conversationFooter()), m.modeLabel()) {
		t.Fatal("copy receipt hid the current mode")
	}
	if lipgloss.Width(m.conversationFooter()) > m.chatColumn().width {
		t.Fatal("footer exceeds conversation")
	}
}
