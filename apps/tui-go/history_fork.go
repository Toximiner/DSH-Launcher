package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Only committed conversation messages can be selected. Tool/approval cards,
// attempt previews and hidden service context are not exact-message boundaries.
func historyForkable(message chatMessage) bool {
	return message.tool == nil && message.approvalID == "" && message.text != "" &&
		(message.role == "АГЕНТ" || strings.HasPrefix(message.role, "ВЫ"))
}

func (m *model) selectHistoryMessage(seq int, reveal bool) {
	h := m.historyReader
	if h == nil || h.busy {
		return
	}
	found := false
	for _, row := range h.messageRows {
		if row.seq == seq {
			found = true
			break
		}
	}
	if !found {
		return
	}
	h.selectedSeq = seq
	h.note = fmt.Sprintf("Событие #%d · Ctrl+B — ветка. Tab/Shift+Tab — выбор.", seq)
	m.resizeHistory()
	if reveal {
		for _, row := range h.messageRows {
			if row.seq == seq {
				h.viewport.SetYOffset(row.start)
				break
			}
		}
	}
}

func (m *model) moveHistorySelection(backward bool) {
	h := m.historyReader
	if h == nil || len(h.messageRows) == 0 {
		return
	}
	index := -1
	for i, row := range h.messageRows {
		if row.seq == h.selectedSeq {
			index = i
			break
		}
	}
	if index < 0 {
		index = 0
		for i, row := range h.messageRows {
			if row.end > h.viewport.YOffset && row.start < h.viewport.YOffset+h.viewport.Height {
				index = i
				if !backward {
					break
				}
			}
		}
	} else if backward {
		index = (index + len(h.messageRows) - 1) % len(h.messageRows)
	} else {
		index = (index + 1) % len(h.messageRows)
	}
	m.selectHistoryMessage(h.messageRows[index].seq, true)
}

func (m *model) forkHistorySelection() tea.Cmd {
	h := m.historyReader
	if h == nil || h.busy || h.ctx.Err() != nil || h.projection == nil || m.browserPreview || m.chatSessionChanging() {
		return nil
	}
	if m.chat == nil || m.chat.id != h.sessionID || m.chat.epoch != h.epoch {
		return nil
	}
	for _, message := range h.projection.messages {
		if message.seq == h.selectedSeq && message.seq >= 0 && message.seq <= h.cut && historyForkable(message) {
			seq := message.seq
			preview := strings.Join(strings.Fields(clean(message.role+": "+message.text)), " ")
			m.input.Blur()
			m.forkDialog = &sessionFork{id: h.sessionID, atSeq: &seq, preview: preview, focused: h.focused}
			return nil
		}
	}
	h.note = "Tab/Shift+Tab или клик — выберите сообщение; Ctrl+B — ветка."
	return nil
}
