package main

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func conversationModel(t *testing.T) *model {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":2,"records":[{"type":"event","event":{"seq":0,"type":"user/message","time":1700000000000,"data":{"source":{"kind":"user"},"content":[{"type":"text","text":"Привет 世界"}]}}},{"type":"event","event":{"seq":1,"type":"step/start","time":1700000001000,"data":{"turn":1,"step":1}}},{"type":"event","event":{"seq":2,"type":"assistant/message","time":1700000002500,"data":{"turn":1,"step":1,"message":{"content":[{"type":"text","text":"Ответ **готов**."}]}}}}]}`)
	return m
}
func TestConversationColumnMessagesAndCopyHitAreas(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 180, Height: 42}} {
			m := conversationModel(t)
			m.prefs.HideSidebar = hidden
			m.Update(size)
			m.input.SetValue("preserved draft")
			m.viewport.GotoTop()
			m.rebuild()
			view := m.View()
			if !m.conversationVisible() || lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height {
				t.Fatal("conversation layout overflowed or returned to the landing screen")
			}
			col := m.chatColumn()
			if col.width > conversationMaxWidth || m.conversationArea().x != col.x+2 || m.composerBox().x != col.x {
				t.Fatal("viewport, composer and centered column diverged")
			}
			if len(m.chatCopyHits) != 2 {
				t.Fatal("committed messages have no copy targets")
			}
			lines := strings.Split(clean(view), "\n")
			_, _, controls := m.landingControls()
			for _, hit := range controls {
				if hit.action == "workspaces" || hit.action == "presets" {
					continue
				}
				line := []rune(lines[hit.y])
				if hit.x >= len(line) || line[hit.x] != '[' {
					t.Fatalf("composer control %s is not aligned with its visible button", hit.action)
				}
			}
			user, agent := -1, -1
			for _, line := range lines {
				if x := strings.Index(line, "Привет"); x >= 0 {
					user = x
				}
				if x := strings.Index(line, "Ответ готов"); x >= 0 {
					agent = x
				}
			}
			if user <= agent || agent < 0 {
				t.Fatal("human bubble is not right-aligned relative to the assistant")
			}
			for _, hit := range m.chatCopyHits {
				y := m.conversationArea().y + hit.row - m.viewport.YOffset
				if y >= len(lines) || !strings.Contains(lines[y], "[Копировать]") {
					t.Fatal("copy control is not aligned with its visible line")
				}
			}
			var copied string
			m.copyClipboard = func(_ context.Context, value string) (string, error) { copied = value; return "copied", nil }
			hit := m.chatCopyHits[1]
			_, cmd := m.Update(tea.MouseMsg{X: m.conversationArea().x + hit.x, Y: m.conversationArea().y + hit.row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if cmd == nil {
				t.Fatal("copy click did not execute")
			}
			m.Update(cmd())
			if copied != "Ответ **готов**." || m.input.Value() != "preserved draft" {
				t.Fatal("copy included decorations, changed the source or lost the draft")
			}
		}
	}
}
func TestConversationStatsUseDisjointAuthoritativeCounters(t *testing.T) {
	m := conversationModel(t)
	unknown := m.conversationStatus()
	if strings.Contains(unknown, "ток") || strings.Contains(unknown, "ход") {
		t.Fatal("missing statistics were presented as known zeroes")
	}
	m.session.Stats = &conversationStats{Turns: 1, Steps: 2, DecodeMs: 500, DecodeTokens: 100}
	m.session.Usage = &conversationUsage{Input: 100, Output: 20, CacheRead: 50, CacheWrite: 25}
	projected, capacity := int64(100), int64(1000)
	m.session.Pressure = &conversationPressure{Projected: &projected, Capacity: &capacity}
	for _, value := range []string{"1 ход · 2 шаг", "200.0 ток/с", "195 ток", "кэш 29%", "контекст ≈10%"} {
		if !strings.Contains(m.conversationStatus(), value) {
			t.Fatalf("missing authoritative figure: %s", value)
		}
	}
	if m.chat.messages[1].elapsedMs != 1500 {
		t.Fatal("reply duration does not use the matching step boundary")
	}
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":5,"records":[{"type":"event","event":{"seq":5,"type":"assistant/message","time":1700000099999,"data":{"turn":1,"step":1,"message":{"content":[{"type":"text","text":"No visible start"}]}}}}]}`)
	if m.chat.messages[0].elapsedMs != 0 {
		t.Fatal("reconnect reused a step boundary outside the new snapshot")
	}
}
func TestConversationHeaderAndComposerControlsKeepDraft(t *testing.T) {
	m := conversationModel(t)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 42})
	m.input.SetValue("draft")
	_, _, hits := m.conversationHeader("")
	for _, hit := range hits {
		switch hit.action {
		case "history":
			m.mouse(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if m.historyReader == nil {
				t.Fatal("Trajectory tab did not open the actual history reader")
			}
			m.closeHistory()
		case "conversation":
			m.mouse(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if m.chrome.dialog != "conversation" {
				t.Fatal("session menu did not open")
			}
			m.closeChrome()
		case "presets":
			m.mouse(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if m.workbench == nil || m.workbench.kind != "presets" {
				t.Fatal("preset header hit area was misplaced")
			}
			m.closeWorkbench()
		}
	}
	if m.input.Value() != "draft" || !m.input.Focused() {
		t.Fatal("conversation controls lost the draft or focus")
	}
}
func TestConversationCopyRejectsOversizeAndStaleReceipt(t *testing.T) {
	m := conversationModel(t)
	m.chat.messages[1].text = strings.Repeat("x", clipboardTextLimit+1)
	if m.copyConversation(2) != nil || m.chat.copyBusy {
		t.Fatal("oversize copy was not refused")
	}
	old := m.chat
	m.chat = &chatState{ctx: old.ctx}
	m.handleConversationCopy(conversationCopyResult{owner: old, receipt: "stale"})
	if m.chat.copyNote != "" {
		t.Fatal("stale copy receipt affected another session")
	}
}

func TestConversationCopySurvivesLanguageSwitch(t *testing.T) {
	m := conversationModel(t)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.prefs.Language = "en"
	defer activeEnglish.Store(false)
	m.applyClientAppearance()
	var copied string
	m.copyClipboard = func(_ context.Context, value string) (string, error) { copied = value; return "copied", nil }
	cmd, handled := m.conversationKey(tea.KeyMsg{Type: tea.KeyCtrlY})
	if !handled || cmd == nil {
		t.Fatal("switching language disabled copy of the committed reply")
	}
	m.Update(cmd())
	if copied != "Ответ **готов**." {
		t.Fatal("language switch changed the copied content")
	}
}
