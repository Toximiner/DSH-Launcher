//go:build js && wasm

package main

import (
	"context"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
)

func startBenchTerminal(ctx context.Context, login, session, id string) tea.Cmd {
	return func() tea.Msg { return benchTerminalDone{err: errors.New("native terminal required")} }
}
