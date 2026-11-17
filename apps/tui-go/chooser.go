package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

// Choosers share one measured frame. Callers own asynchronous data and actions;
// only rows passed to this frame may become mouse targets.
type chooserSpec struct {
	Title, Search, Detail, Footer string
	Width, Height                 int
}
type chooserGeometry struct {
	Box, List, Search, Close area
	InnerWidth               int
}

func (m *model) chooserLayout(spec chooserSpec) chooserGeometry {
	width := spec.Width
	if width <= 0 {
		width = 60
	}
	height := spec.Height
	if height <= 0 {
		height = 20
	}
	width = min(width, max(1, m.width-4))
	// The executable reference places chooser frames around the middle half
	// of the terminal. Reserve a small minimum for model-write disclosures.
	height = min(height, min(max(14, m.height/2+1), max(1, m.height-4)))
	box := area{max(0, (m.width-1-width)/2), max(1, min((m.height+3)/4, m.height-height-1)), width, height}
	inner := max(1, width-4)
	footerRows := 1
	if spec.Footer != "" {
		footerRows = strings.Count(spec.Footer, "\n") + 1
	}
	return chooserGeometry{Box: box, InnerWidth: inner,
		List:   area{box.x + 2, box.y + 5, inner, max(1, height-6-footerRows)},
		Search: area{box.x + 2, box.y + 3, inner, 1},
		Close:  area{box.x + width - 5, box.y + 1, 3, 1},
	}
}
func (m *model) chooserView(base string, spec chooserSpec, rows []string) string {
	g := m.chooserLayout(spec)
	lines := []string{"", bar(heading().Render(spec.Title), muted().Render("esc"), g.InnerWidth), muted().Render(spec.Detail), spec.Search, ""}
	for _, row := range rows[:min(len(rows), g.List.height)] {
		lines = append(lines, row)
	}
	for len(lines) < 5+g.List.height {
		lines = append(lines, "")
	}
	footer := strings.Split(spec.Footer, "\n")
	for _, line := range footer {
		lines = append(lines, muted().Render(line))
	}
	lines = append(lines, "")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], g.InnerWidth, "…")
	}
	front := lipgloss.NewStyle().Width(g.Box.width).Height(g.Box.height).Padding(0, 2).
		Foreground(theme.Color(theme.Text)).Background(theme.Color(theme.Panel)).Render(strings.Join(lines, "\n"))
	return overlay(dimBackdrop(base), surface(front, theme.Panel), g.Box.x, g.Box.y, m.width-1)
}
func chooserItem(label, hint string, selected, enabled bool, width int) string {
	if !enabled {
		hint = "—"
	}
	hint = ansi.Truncate(clean(hint), max(1, width/2), "…")
	text := bar(clean(label), hint, max(1, width))
	if selected {
		return lipgloss.NewStyle().Width(width).Foreground(theme.Color(theme.SelectionText)).Background(theme.Color(theme.Selection)).Render(ansi.Strip(text))
	}
	return muted().Render(text)
}
func chooserGroup(label string) string {
	return lipgloss.NewStyle().Foreground(theme.Color(theme.Secondary)).Render(label)
}
func chooserWindow(selected, length, count int) (int, int) {
	count = max(1, count)
	start := max(0, min(selected-count/2, max(0, length-count)))
	return start, min(length, start+count)
}
