// Package component provides shared UI primitives for the TUI. Dialog is a
// single template for all overlay windows, following opencode's dialog shape
// (packages/tui/src/ui/dialog.tsx): fixed size tiers, centered box, header with
// a muted close hint, a blank line between header/body/footer.
package component

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

// Size is a dialog size tier (opencode: medium 60 / large 88 / xlarge 116
// cells; docs/tui-rebuild-plan.md:18-21).
type Size int

const (
	Small Size = iota // width 60: confirms and short forms
	Medium            // width 88: standard dialogs
	Large             // width 116: content-heavy dialogs
)

const (
	MinTerminalWidth  = 80
	MinTerminalHeight = 24
	DialogInset       = 3 // minimum distance of a dialog from the terminal edge
)

var sizeWidths = map[Size]int{Small: 60, Medium: 88, Large: 116}
var sizeHeights = map[Size]int{Small: 14, Medium: 24, Large: 34}

// Dialog describes one overlay window. Title/Footer arrive already
// translated; Body is caller-built lipgloss content that must fit
// InnerWidth x BodyRows. Dialog does not manage nested bubbles
// (viewports, editors) - the caller sizes them to BodyRows.
type Dialog struct {
	Title    string      // bold, left; a muted "Esc" is pinned right by Layout
	Subtitle string      // muted line under the title (optional)
	Body     string      // content, newline-separated
	Footer   string      // hint/button lines, newline-separated
	Border   theme.Token // frame: theme.Accent/Warning/Highlight per state
	Size     Size
	AlignTop bool        // pin to the top (runtime) instead of centering
}

// Width is the frame width in terminal cells, clamped to the terminal.
func Width(d Dialog, termW int) int {
	w := sizeWidths[d.Size]
	if w > termW-3 {
		w = termW - 3
	}
	return w
}

// Height is the frame height in terminal cells, clamped to the terminal.
func Height(d Dialog, termH int) int {
	h := sizeHeights[d.Size]
	if h > termH-4 {
		h = termH - 4
	}
	return h
}

// InnerWidth is the content width (frame minus border and padding).
func InnerWidth(d Dialog, termW int) int { return Width(d, termW) - 4 }

// BodyRows is the number of rows available for Body in the Layout template:
// (height-4) - header(1) - subtitle(1 when set) - blank(1) - blank(1) - footer(rows).
func BodyRows(d Dialog, termH int) int {
	rows := Height(d, termH) - 4 - 3
	if d.Subtitle != "" {
		rows--
	}
	if d.Footer != "" {
		rows -= strings.Count(d.Footer, "\n") + 1
	}
	return rows
}

// Rect is a box in terminal cells, ready to become the caller's hit area.
type Rect struct{ X, Y, W, H int }

// Box is the frame position: centered horizontally, centered (or pinned
// top when AlignTop) vertically, with DialogInset from the edges.
func Box(d Dialog, termW, termH int) Rect {
	w, h := Width(d, termW), Height(d, termH)
	x := (termW - 1 - w) / 2
	if x < 0 {
		x = 0
	}
	y := (termH - h) / 2
	if d.AlignTop {
		y = 2
	} else if y < DialogInset-1 {
		y = DialogInset - 1
	}
	return Rect{X: x, Y: y, W: w, H: h}
}

// Layout assembles the template: title (bold) with a muted "Esc" pinned
// right, optional subtitle, blank, Body, blank, Footer - exactly
// height-4 rows. Lines longer than the inner width are truncated.
func Layout(d Dialog, termW, termH int) []string {
	innerW := InnerWidth(d, termW)
	innerH := Height(d, termH) - 4
	title := lipgloss.NewStyle().Bold(true).Foreground(theme.Color(theme.Text)).Render(d.Title)
	right := lipgloss.NewStyle().Foreground(theme.Color(theme.Muted)).Render("Esc")
	title = ansi.Truncate(title, max(1, innerW-lipgloss.Width(right)-1), "\u2026")
	lines := []string{title + strings.Repeat(" ", max(1, innerW-lipgloss.Width(title)-lipgloss.Width(right))) + right}
	if d.Subtitle != "" {
		lines = append(lines, ansi.Truncate(lipgloss.NewStyle().Foreground(theme.Color(theme.Muted)).Render(d.Subtitle), innerW, "\u2026"))
	}
	lines = append(lines, "")
	body := strings.Split(d.Body, "\n")
	bodyRows := BodyRows(d, termH)
	if len(body) > bodyRows {
		body = body[:bodyRows]
	}
	lines = append(lines, body...)
	lines = append(lines, "")
	if d.Footer != "" {
		for _, f := range strings.Split(d.Footer, "\n") {
			lines = append(lines, ansi.Truncate(f, innerW, "\u2026"))
		}
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, innerW, "\u2026")
	}
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	return lines
}

// Panel renders the framed dialog (border + padding, like the main panel);
// lipgloss pads the box with the panel background.
func Panel(d Dialog, termW, termH int, content string) string {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(theme.Color(d.Border)).
		Foreground(theme.Color(theme.Text)).Background(theme.Color(theme.Panel)).Padding(0, 1)
	w, h := Width(d, termW), Height(d, termH)
	return style.Width(max(1, w-style.GetHorizontalBorderSize())).
		Height(max(1, h-style.GetVerticalFrameSize())).Render(content)
}

// Render returns (box, front): the frame position and the framed template,
// ready for the caller's overlay(base, front, box.X, box.Y, width).
func Render(d Dialog, termW, termH int) (Rect, string) {
	return Box(d, termW, termH), Panel(d, termW, termH, strings.Join(Layout(d, termW, termH), "\n"))
}
