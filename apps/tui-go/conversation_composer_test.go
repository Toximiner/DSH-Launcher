package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestCompactConversationComposerGeometryAndDraft(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, language := range []string{"", "en"} {
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 40}} {
			m := conversationModel(t)
			m.prefs.Language = language
			m.Update(size)
			m.session.AgentPreset = "fixture-agent"
			m.session.Model = &modelSelection{Provider: "fixture", Model: "qwen-3.8", Reasoning: "high"}
			m.session.Permission = "workspace-write"
			for _, draft := range []string{"", "preserved 世界", strings.Repeat("wide 世界 ", 30) + "TAIL"} {
				m.input.SetValue(draft)
				m.input.CursorEnd()
				cursor := m.input.Position()
				box := m.composerBox()
				_, height := m.contentSize()
				if box.y != bodyTop+height-conversationComposerHeight || box.height != conversationComposerHeight {
					t.Fatal("composer is not pinned to the bottom of the body")
				}
				view := m.conversationComposerView()
				if lipgloss.Width(view) != box.width || lipgloss.Height(view) != conversationComposerHeight {
					t.Fatalf("composer dimensions %dx%d differ from box %dx%d; draft=%q", lipgloss.Width(view), lipgloss.Height(view), box.width, box.height, draft)
				}
				lines := strings.Split(ansi.Strip(view), "\n")
				for i, line := range lines {
					if !strings.HasPrefix(line, "┃") || lipgloss.Width(line) != box.width {
						t.Fatalf("composer row%d has no continuous surface", i)
					}
				}
				context, _, hits := m.landingControls()
				if context != "" || len(hits) != 3 {
					t.Fatal("conversation exposes hidden workspace or compose buttons")
				}
				for _, hit := range hits {
					if hit.y != box.y+3 || hit.x < box.x+2 || hit.x+hit.width > box.x+box.width-2 {
						t.Fatalf("control%q falls outside metadata row", hit.action)
					}
					visible := ansi.Cut(lines[3], hit.x-box.x, hit.x-box.x+hit.width)
					if strings.TrimSpace(visible) == "" {
						t.Fatalf("control%q targets blank cells", hit.action)
					}
				}
				if draft != "" && !strings.Contains(lines[0], "preserved") && !strings.Contains(lines[0], "TAIL") {
					t.Fatal("input tail is hidden")
				}
				if m.input.Value() != draft || m.input.Position() != cursor {
					t.Fatal("composer rendering mutated draft or cursor")
				}
			}
		}
	}
}

func TestCompactComposerHomeWidthAfterLongDraftAndCursorMove(t *testing.T) {
	m := conversationModel(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	draft := strings.Repeat("界", 120) + "TAIL"
	m.input.SetValue(draft)
	for _, cursor := range []int{len([]rune(draft)), 60, 0} {
		m.input.SetCursor(cursor)
		box := m.landingBox()
		metadata, _ := m.compactComposerControls(box)
		view := m.compactComposerSurface(box, "", metadata)
		if lipgloss.Height(view) != 4 || lipgloss.Width(view) != box.width {
			t.Fatalf("narrow composer overflow after cursor move: %dx%d", lipgloss.Width(view), lipgloss.Height(view))
		}
		if m.input.Value() != draft || m.input.Position() != cursor {
			t.Fatal("rendering modified input state")
		}
	}
}
