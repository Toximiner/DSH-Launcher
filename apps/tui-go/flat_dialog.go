package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

// Auxiliary dialogs retain their measured content origin at x+2,y+1. Drawing
// and backdrop composition are separate so nested dialogs dim only once.
func flatDialogSurface(box area, blocks []string) string {
	width, height := max(1, box.width-4), max(1, box.height-2)
	lines := []string{}
	for _, block := range blocks {
		for _, line := range strings.Split(block, "\n") {
			lines = append(lines, ansi.Truncate(line, width, "…"))
		}
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	front := lipgloss.NewStyle().Width(box.width).Height(box.height).Padding(1, 2).
		Foreground(theme.Color(theme.Text)).Background(theme.Color(theme.Panel)).Render(strings.Join(lines, "\n"))
	return surface(front, theme.Panel)
}

func flatDialogClose(box area) area { return area{box.x + box.width - 5, box.y + 1, 3, 1} }
