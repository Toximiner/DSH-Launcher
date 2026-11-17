package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestConversationLayoutUsesAvailableWidthAndResponsiveContext(t *testing.T) {
	for _, columns := range []int{80, 100, 119, 120, 160, 240} {
		m := conversationModel(t)
		m.Update(tea.WindowSizeMsg{Width: columns, Height: 30})
		width, height := m.contentSize()
		layout := m.conversationLayout()
		visible := columns >= conversationContextMinWidth
		if layout.showContext != visible || layout.chat.x != 0 || layout.chat.y != bodyTop || layout.chat.height != height {
			t.Fatalf("invalid conversation layout at%dcolumns: %+v", columns, layout)
		}
		expected := width
		if visible {
			expected -= conversationContextGap + conversationContextWidth
			if layout.context.x != expected+conversationContextGap || layout.context.width != conversationContextWidth {
				t.Fatal("context does not occupy its measured right column")
			}
		}
		if layout.chat.width != expected {
			t.Fatalf("chat unexpectedly capped at%d instead of%d", layout.chat.width, expected)
		}
		m.contextPanelHidden = true
		m.resize()
		hidden := m.conversationLayout()
		if hidden.showContext || hidden.chat.width != width {
			t.Fatal("hidden context still reserves width")
		}
	}
}

func TestConversationContextShowsOnlyKnownPressureAndWorkspace(t *testing.T) {
	m := conversationModel(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m.session.Title = "Actual title"
	m.workspace = "/tmp/isolated-project"
	box := m.conversationLayout().context
	unknown := ansi.Strip(m.conversationContextView(box))
	if strings.Contains(unknown, "MCP") || strings.Contains(unknown, "%") || strings.Contains(unknown, tr("Контекст")) {
		t.Fatal("context fabricates inventory or token usage")
	}
	used, capacity := int64(25000), int64(100000)
	m.session.Pressure = &conversationPressure{Projected: &used, Capacity: &capacity}
	visible := m.conversationContextView(box)
	if lipgloss.Width(visible) != box.width || lipgloss.Height(visible) != box.height {
		t.Fatal("context panel overflowed")
	}
	text := ansi.Strip(visible)
	for _, wanted := range []string{"Actual title", countLabel(used), "25%", m.workspace} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("known context field missing:%s; rendered=%q", wanted, text)
		}
	}
	lines := strings.Split(text, "\n")
	if !strings.Contains(lines[box.height-2], m.workspace) {
		t.Fatal("workspace is not anchored to panel bottom")
	}
	m.sessionStale = true
	if strings.Contains(ansi.Strip(m.conversationContextView(box)), "25%") {
		t.Fatal("stale projection is presented as current usage")
	}
}

func TestHomeRetainsCompactComposerWithoutContext(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	layout := m.conversationLayout()
	if layout.showContext || layout.chat.width != 159 {
		t.Fatal("home inherited the conversation context panel")
	}
	box := m.landingBox()
	if box.width != 76 || box.x != (159-76)/2 {
		t.Fatal("home input lost its compact centering")
	}
}

func TestConversationLayoutLongWorkspaceFitsFullView(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 40}} {
		m := conversationModel(t)
		m.Update(size)
		m.workspace = "/tmp/" + strings.Repeat("очень-длинная-папка世界/", 12) + "project-tail"
		m.session.Title = strings.Repeat("界", 100)
		view := m.View()
		if lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height {
			t.Fatalf("long workspace overflowed %dx%d: rendered %dx%d", size.Width, size.Height, lipgloss.Width(view), lipgloss.Height(view))
		}
		if size.Width >= conversationContextMinWidth {
			panel := ansi.Strip(m.conversationContextView(m.conversationLayout().context))
			if !strings.Contains(panel, "project-tail") {
				t.Fatal("truncated workspace lost its identifying tail")
			}
		}
	}
}

func TestConversationGeometryReflowsHomeTransitionWithoutResize(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	homeWidth := m.viewport.Width
	m.input.SetValue("retained draft")
	m.append(strings.Repeat("first-message ", 100))
	layout := m.conversationLayout()
	if !layout.showContext || m.viewport.Width != layout.chat.width-4 || m.viewport.Width >= homeWidth {
		t.Fatal("first message uses the home viewport after the context panel appears")
	}
	if m.input.Value() != "retained draft" {
		t.Fatal("reflow lost draft")
	}
	m.contextPanelHidden = true
	m.rebuild()
	if m.viewport.Width != 159-4 || m.input.Value() != "retained draft" {
		t.Fatal("hidden context leaves stale narrow geometry or loses draft")
	}
}
