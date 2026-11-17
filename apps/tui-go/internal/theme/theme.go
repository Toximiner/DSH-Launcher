// Package theme centralizes the TUI color palette as semantic tokens with
// dark and light variants, following the shape of opencode's packages/theme.
//
// Call sites resolve a role (Text, Muted, Accent, ...) to a concrete color at
// render time; no view code stores raw hex values. Switching themes is a
// single SetLight call, and the palette stays in one place.
package theme

import (
	"sync/atomic"

	"github.com/charmbracelet/lipgloss"
)

// Token names a semantic role in the interface, not a raw color.
type Token string

const (
	Bg            Token = "bg"             // terminal background; text over Highlight chips
	Panel         Token = "panel"          // panels, dialogs, full-screen re-wrap background
	Active        Token = "active"         // neutral active tab and conversation surfaces
	Border        Token = "border"         // panel border in the resting state
	Text          Token = "text"           // primary content
	Muted         Token = "muted"          // metadata, placeholders, key hints
	Accent        Token = "accent"         // brand, active state, focused borders
	Highlight     Token = "highlight"      // text on Active background, highlighted dialog borders
	Code          Token = "code"           // inline code in markdown
	Warning       Token = "warning"        // unconnected status, approval frames
	Selection     Token = "selection"      // highlighted list row
	SelectionText Token = "selection-text" // readable text over Selection
	Secondary     Token = "secondary"      // teal section headings
	Backdrop      Token = "backdrop"       // dimmed content behind a modal
	BackdropText  Token = "backdrop-text"  // text on Backdrop
)

type palette map[Token]string

var dark = palette{
	Bg:        "#1a1a1a",
	Panel:     "#242424",
	Active:    "#303030",
	Border:    "#484848",
	Text:      "#eeeeee",
	Muted:     "#9a9a9a",
	Accent:    "#b7a8f0",
	Highlight: "#b7a8f0",
	Code:      "#a8c7fa",
	Warning:   "#f5c76b",
	Selection: "#8bd5ca", SelectionText: "#152623", Secondary: "#8bd5ca", Backdrop: "#101010", BackdropText: "#555555",
}

var light = palette{
	Bg:        "#faf9f6",
	Panel:     "#f0eeea",
	Active:    "#e4dfd8",
	Border:    "#9b9690",
	Text:      "#25231f",
	Muted:     "#6c665e",
	Accent:    "#7044a0",
	Highlight: "#7044a0",
	Code:      "#245c94",
	Warning:   "#865b08",
	Selection: "#a5d8cf", SelectionText: "#17312b", Secondary: "#287f75", Backdrop: "#b8b6b1", BackdropText: "#73706b",
}

var lightMode atomic.Bool

// SetLight switches the active palette variant.
func SetLight(enabled bool) { lightMode.Store(enabled) }

// IsLight reports whether the light palette is active.
func IsLight() bool { return lightMode.Load() }

// Color resolves a semantic token to a concrete color of the active palette.
func Color(token Token) lipgloss.Color {
	table := dark
	if lightMode.Load() {
		table = light
	}
	return lipgloss.Color(table[token])
}
