package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func conversationModel(t *testing.T) *model {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":2,"records":[{"type":"event","event":{"seq":0,"type":"user/message","time":1700000000000,"data":{"source":{"kind":"user"},"content":[{"type":"text","text":"Привет 世界"}]}}},{"type":"event","event":{"seq":1,"type":"step/start","time":1700000001000,"data":{"turn":1,"step":1}}},{"type":"event","event":{"seq":2,"type":"assistant/message","time":1700000002500,"data":{"turn":1,"step":1,"message":{"content":[{"type":"text","text":"Ответ **готов**."}]}}}}]}`)
	return m
}
func TestConversationColumnMessagesAndKeyboardCopy(t *testing.T) {
	for _, language := range []string{"ru", "en"} {
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 40}} {
			t.Run(language+"/"+fmt.Sprint(size.Width), func(t *testing.T) {
				m := conversationModel(t)
				m.prefs.Language = language
				m.applyClientAppearance()
				defer activeEnglish.Store(false)
				m.Update(size)
				m.input.SetValue("preserved draft")
				m.viewport.GotoTop()
				m.rebuild()
				view := m.View()
				if !m.conversationVisible() || lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height {
					t.Fatalf("conversation overflowed or returned to landing: got %dx%d, terminal %dx%d", lipgloss.Width(view), lipgloss.Height(view), size.Width, size.Height)
				}
				col := m.chatColumn()
				content := m.conversationArea()
				if col.width <= 0 || col.x+col.width >= size.Width || content.x != col.x+2 || content.y != bodyTop || m.composerBox().x != col.x {
					t.Fatal("viewport, composer and column diverged")
				}
				if m.composerBox().y != bodyTop+content.height {
					t.Fatal("viewport and composer do not meet")
				}
				if size.Width >= 160 && col.width <= 100 {
					t.Fatal("wide conversation retained narrow centered column")
				}
				if strings.Contains(clean(view), "["+tr("Копировать")+"]") {
					t.Fatal("conversation retained permanent copy actions")
				}
				lines := strings.Split(clean(view), "\n")
				user, agent := -1, -1
				for _, line := range lines {
					if x := strings.Index(line, "Привет"); x >= 0 {
						user = lipgloss.Width(line[:x])
					}
					if x := strings.Index(line, "Ответ готов"); x >= 0 {
						agent = lipgloss.Width(line[:x])
					}
				}
				if agent < 0 || user != agent+2 {
					t.Fatal("user card and assistant are not aligned to the same left column")
				}
				var copied string
				m.copyClipboard = func(_ context.Context, value string) (string, error) { copied = value; return "copied", nil }
				_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
				if cmd == nil {
					t.Fatal("keyboard copy did not execute")
				}
				m.Update(cmd())
				if copied != "Ответ **готов**." || m.input.Value() != "preserved draft" {
					t.Fatal("copy changed source or lost draft")
				}
			})
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
func TestConversationHeaderHasNoInvisibleControls(t *testing.T) {
	m := conversationModel(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m.input.SetValue("draft")
	title, tabs, hits := m.conversationHeader("duplicate session title")
	if title != "" || tabs != "" || len(hits) != 0 {
		t.Fatal("conversation retained duplicate header or hidden hit targets")
	}
	view := clean(m.View())
	if strings.Contains(view, "[Траектория]") || strings.Contains(view, "[Чат]") || strings.Contains(view, "duplicate session title") {
		t.Fatal("conversation retained GUI navigation")
	}
	if m.input.Value() != "draft" || !m.input.Focused() {
		t.Fatal("rendering changed draft or focus")
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

func TestConversationMessageLayoutAfterWrapping(t *testing.T) {
	for _, language := range []string{"ru", "en"} {
		for _, width := range []int{75, 95, 96} {
			for _, human := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/%d/human=%t", language, width, human), func(t *testing.T) {
					m := conversationModel(t)
					m.prefs.Language = language
					m.applyClientAppearance()
					defer activeEnglish.Store(false)
					m.session.AgentPreset = strings.Repeat("long-agent-preset-", 12)
					message := chatMessage{seq: 42, human: human, assistant: !human, time: 1700000000000, elapsedMs: 1500, text: strings.Repeat("世界🙂 Пример é ", 22) + "\n\n```go\nfmt.Println(\"hello\")\n```"}
					rendered := m.conversationMessage(message, width, 17)
					rows := strings.Split(rendered, "\n")
					for _, line := range rows {
						if lipgloss.Width(line) > width {
							t.Fatalf("message row overflow: width=%d, available=%d", lipgloss.Width(line), width)
						}
					}
					if strings.Contains(clean(rendered), "["+tr("Копировать")+"]") {
						t.Fatal("wrapped message retained permanent copy action")
					}
					if human && len(rows) != strings.Count(ansi.Hardwrap(clean(message.text), width-2, true), "\n")+2 {
						t.Fatal("ordinary human message gained a permanent metadata row")
					}
					if human && !strings.HasPrefix(ansi.Strip(rows[0]), "┃ ") {
						t.Fatal("user message lost left accent")
					}
					if !human && strings.HasPrefix(ansi.Strip(rows[0]), tr("АГЕНТ")) {
						t.Fatal("assistant gained oversized role heading")
					}
				})
			}
		}
	}
}

func TestConversationDirectCopyPreservesSourcesWithoutButtons(t *testing.T) {
	m := conversationModel(t)
	m.input.SetValue("retained draft")
	for _, message := range m.chat.messages {
		if message.tool != nil || message.approvalID != "" {
			continue
		}
		var copied string
		m.copyClipboard = func(_ context.Context, source string) (string, error) { copied = source; return "copied", nil }
		cmd := m.copyConversation(message.seq)
		if cmd == nil {
			t.Fatal("message source copy was lost with hidden buttons")
		}
		m.Update(cmd())
		if copied != message.text || m.input.Value() != "retained draft" {
			t.Fatal("direct copy changed original message or draft")
		}
	}
}

func TestConversationScrollAndCopyDoNotJumpToTail(t *testing.T) {
	m := conversationModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.chat.messages[1].text = strings.Repeat("long answer line\n", 90)
	m.rebuild()
	m.viewport.GotoTop()
	content := m.conversationArea()
	m.mouse(tea.MouseMsg{X: content.x + 2, Y: content.y + 2, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if m.viewport.YOffset <= 0 {
		t.Fatal("conversation mouse wheel did not scroll history")
	}
	offset := m.viewport.YOffset
	m.input.SetValue("draft while reading")
	m.copyClipboard = func(_ context.Context, source string) (string, error) { return "copied", nil }
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	if cmd == nil {
		t.Fatal("copy shortcut unavailable while reading older messages")
	}
	m.Update(cmd())
	if m.viewport.YOffset != offset || m.input.Value() != "draft while reading" {
		t.Fatal("copy jumped to tail or lost draft")
	}
}

func TestConversationPreservesSemanticRolesAcrossLanguageSwitch(t *testing.T) {
	sources := []string{"ВЫ · ОТВЕТЫ НА ВОПРОС", "АГЕНТ · НЕЗАВЕРШЁННЫЙ ОТВЕТ", "СООБЩЕНИЕ DSH", "РАЗРЕШЕНИЕ", "РАЗРЕШЕНИЕ · ОЖИДАЕТ ОТВЕТА"}
	for _, language := range []string{"ru", "en"} {
		m := conversationModel(t)
		m.prefs.Language = language
		m.applyClientAppearance()
		defer activeEnglish.Store(false)
		for _, source := range sources {
			for _, storedRole := range []string{source, englishCatalog[source]} {
				message := chatMessage{role: storedRole, text: "semantic message", human: source == "ВЫ · ОТВЕТЫ НА ВОПРОС"}
				rendered := clean(m.conversationMessage(message, 95, 0))
				if !strings.Contains(rendered, tr(source)) {
					t.Fatalf("semantic role %q missing after switch to %s", source, language)
				}
			}
		}
	}
}
