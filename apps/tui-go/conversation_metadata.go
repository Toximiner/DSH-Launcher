package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// messageMetadata uses only facts stored on this message. In particular,
// chatMessage carries no historical model identity, so the current session's
// selected model must not be attributed to a previously generated reply.
// Copying stays available through Ctrl+Y and the history reader, not a permanent
// action or clock appended to every message.
func messageMetadata(message chatMessage, width int) string {
	if width <= 0 {
		return ""
	}
	role := conversationRoleLabel(message.role)
	human := message.human || role == tr("ВЫ") || strings.HasPrefix(role, tr("ВЫ ·"))
	if human {
		if role == "" || role == tr("ВЫ") {
			return ""
		}
		return ansi.Truncate(singleLine(role), width, "…")
	}
	assistant := message.assistant || role == tr("АГЕНТ") || strings.HasPrefix(role, tr("АГЕНТ ·"))
	if !assistant {
		return ansi.Truncate(singleLine(role), width, "…")
	}
	label := role
	if label == "" || label == tr("АГЕНТ") {
		label = tr("Агент")
	}
	if message.elapsedMs > 0 && validStat(message.elapsedMs) {
		label += " · " + fmt.Sprintf(tr("%.1f с"), float64(message.elapsedMs)/1000)
	}
	return ansi.Truncate(singleLine(label), width, "…")
}
