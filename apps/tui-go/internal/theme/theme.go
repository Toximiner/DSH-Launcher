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
	Bg        Token = "bg"        // terminal background; text over Highlight chips
	Panel     Token = "panel"     // panels, dialogs, full-screen re-wrap background
	Active    Token = "active"    // selected rows, composer and message bubbles
	Border    Token = "border"    // panel border in the resting state
	Text      Token = "text"      // primary content
	Muted     Token = "muted"     // metadata, placeholders, key hints
	Accent    Token = "accent"    // brand, active state, focused borders
	Highlight Token = "highlight" // text on Active background, highlighted dialog borders
	Code      Token = "code"      // inline code in markdown
	Warning   Token = "warning"   // unconnected status, approval frames
)

type palette map[Token]string

var dark = palette{
	Bg:        "#101318",
	Panel:     "#111820",
	Active:    "#233044",
	Border:    "#303d4d",
	Text:      "#e6edf3",
	Muted:     "#8b949e",
	Accent:    "#58a6ff",
	Highlight: "#79c0ff",
	Code:      "#a5d6ff",
	Warning:   "#d29922",
}

// light mirrors the previous inline lightPalette. Code intentionally keeps
// the dark value: light blue on white is the existing (low-contrast)
// behavior, and fixing contrast belongs to a later visual phase, not to the
// mechanical token migration.
var light = palette{
	Bg:        "#ffffff",
	Panel:     "#f5f7fa",
	Active:    "#d9eaff",
	Border:    "#8294a8",
	Text:      "#17212b",
	Muted:     "#526171",
	Accent:    "#005fb8",
	Highlight: "#005fb8",
	Code:      "#a5d6ff",
	Warning:   "#805300",
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
