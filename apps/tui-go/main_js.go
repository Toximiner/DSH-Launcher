//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The browser drives the same model as the native terminal. It exposes no shell
// or filesystem bridge, and deliberately does not run native/network commands.
func main() {
	lipgloss.SetColorProfile(termenv.TrueColor)
	m := newModel()
	m.workspace = "browser-preview"
	functions := []js.Func{}
	bind := func(name string, fn func([]js.Value) any) {
		callback := js.FuncOf(func(_ js.Value, args []js.Value) any { return fn(args) })
		functions = append(functions, callback)
		js.Global().Get("dshTui").Set(name, callback)
	}
	js.Global().Set("dshTui", js.Global().Get("Object").New())
	bind("currentPage", func(args []js.Value) any { return m.page })
	bind("page", func(args []js.Value) any {
		page := args[0].Int()
		if page < 0 || page >= len(pages) {
			return m.View()
		}
		m.page = page
		m.navFocused = false
		if page == chatPage {
			m.input.Focus()
		} else {
			m.input.Blur()
		}
		m.resize()
		m.viewport.GotoTop()
		return m.View()
	})
	bind("resize", func(args []js.Value) any {
		m.Update(tea.WindowSizeMsg{Width: args[0].Int(), Height: args[1].Int()})
		return m.View()
	})
	bind("key", func(args []js.Value) any {
		key := args[0].String()
		// Browser shortcuts must never quit the WASM runtime or start network reads.
		if key == "ctrl+c" || key == "ctrl+d" || (m.page == dshSettingsPage && key == "r") {
			return m.View()
		}
		keys := map[string]tea.KeyType{"enter": tea.KeyEnter, "tab": tea.KeyTab, "esc": tea.KeyEsc, "up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "home": tea.KeyHome, "end": tea.KeyEnd, "backspace": tea.KeyBackspace, "delete": tea.KeyDelete, "ctrl+u": tea.KeyCtrlU, "ctrl+a": tea.KeyCtrlA, "ctrl+e": tea.KeyCtrlE, "f1": tea.KeyF1}
		if kind, ok := keys[key]; ok {
			m.Update(tea.KeyMsg{Type: kind})
		} else {
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		}
		return m.View()
	})
	bind("text", func(args []js.Value) any {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(args[0].String()), Paste: true})
		return m.View()
	})
	bind("mouse", func(args []js.Value) any {
		button := tea.MouseButtonLeft
		if args[2].String() == "up" {
			button = tea.MouseButtonWheelUp
		} else if args[2].String() == "down" {
			button = tea.MouseButtonWheelDown
		}
		m.Update(tea.MouseMsg{X: args[0].Int(), Y: args[1].Int(), Button: button, Action: tea.MouseActionPress})
		return m.View()
	})
	bind("snapshot", func(args []js.Value) any {
		var snapshot settingsSnapshot
		if err := json.Unmarshal([]byte(args[0].String()), &snapshot); err != nil {
			return fmt.Sprint(err)
		}
		m.settingsLogin = "http://127.0.0.1:3198/?token=browser-demo"
		m.settingsUpdated = time.Date(2026, 11, 10, 6, 22, 0, 0, time.UTC)
		m.settings = &snapshot
		m.rebuild()
		return m.View()
	})
	js.Global().Call("dispatchEvent", js.Global().Get("Event").New("dsh-tui-ready"))
	select {}
}
