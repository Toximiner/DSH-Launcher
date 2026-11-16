package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

const historyExpandedLimit = 256

type historyToolKey struct {
	id    string
	start int
}

func (m *model) historyToolsAvailable() bool {
	h := m.historyReader
	return h != nil && !h.busy && h.ctx.Err() == nil && h.projection != nil && m.chat != nil && m.chat.id == h.sessionID && m.chat.epoch == h.epoch
}

func (m *model) restoreHistoryTools(latest bool) {
	h := m.historyReader
	if latest {
		h.expandedTools = nil
		h.expandedOrder = nil
	}
	h.toolFocused = false
	for _, message := range h.projection.messages {
		if tool := message.tool; tool != nil {
			h.projection.expandedTools[tool.ID] = h.expandedTools[historyToolKey{tool.ID, tool.StartSeq}]
		}
	}
}

func (h *historyReader) rememberHistoryTool(key historyToolKey, expanded bool) {
	if h.expandedTools == nil {
		h.expandedTools = make(map[historyToolKey]bool)
	}
	for i, old := range h.expandedOrder {
		if old == key {
			h.expandedOrder = append(h.expandedOrder[:i], h.expandedOrder[i+1:]...)
			break
		}
	}
	delete(h.expandedTools, key)
	if !expanded {
		return
	}
	if len(h.expandedOrder) >= historyExpandedLimit {
		delete(h.expandedTools, h.expandedOrder[0])
		h.expandedOrder = h.expandedOrder[1:]
	}
	h.expandedTools[key] = true
	h.expandedOrder = append(h.expandedOrder, key)
}

func (m *model) revealHistoryTool() {
	h := m.historyReader
	for _, hit := range h.toolRows {
		if hit.id == h.projection.selectedTool {
			h.viewport.SetYOffset(hit.row)
			break
		}
	}
}

func (m *model) selectHistoryTool(id string, reveal bool) {
	if !m.historyToolsAvailable() {
		return
	}
	h := m.historyReader
	for _, hit := range h.toolRows {
		if hit.id == id {
			h.toolFocused = true
			h.selectedSeq = -1
			h.projection.selectedTool = id
			h.note = tr("F6/Shift+F6 — инструмент · Enter/клик — раскрыть или свернуть · Tab — сообщение.")
			m.resizeHistory()
			if reveal {
				m.revealHistoryTool()
			}
			return
		}
	}
}

func (m *model) moveHistoryTool(backward bool) {
	if !m.historyToolsAvailable() {
		return
	}
	h := m.historyReader
	if len(h.toolRows) == 0 {
		h.note = tr("На странице нет карточек инструментов.")
		return
	}
	index := -1
	if h.toolFocused {
		for i, hit := range h.toolRows {
			if hit.id == h.projection.selectedTool {
				index = i
				break
			}
		}
	}
	if index < 0 {
		index = 0
		if backward {
			index = len(h.toolRows) - 1
		}
		for i, hit := range h.toolRows {
			if hit.end > h.viewport.YOffset && hit.row < h.viewport.YOffset+h.viewport.Height {
				index = i
				if !backward {
					break
				}
			}
		}
	} else if backward {
		index = (index + len(h.toolRows) - 1) % len(h.toolRows)
	} else {
		index = (index + 1) % len(h.toolRows)
	}
	m.selectHistoryTool(h.toolRows[index].id, true)
}

func (m *model) toggleHistoryTool(id string) {
	if !m.historyToolsAvailable() {
		return
	}
	h := m.historyReader
	for _, message := range h.projection.messages {
		if tool := message.tool; tool != nil && tool.ID == id {
			expanded := !h.projection.expandedTools[id]
			h.projection.expandedTools[id] = expanded
			h.rememberHistoryTool(historyToolKey{id, tool.StartSeq}, expanded)
			m.resizeHistory()
			m.revealHistoryTool()
			state := tr("свёрнуты")
			if expanded {
				state = tr("раскрыты")
			}
			h.note = fmt.Sprintf(tr("Детали %s · Enter/клик — переключить · F6/Shift+F6 — другой инструмент."), state)
			return
		}
	}
}

func (m *model) historyToolKey(msg tea.KeyMsg) bool {
	if msg.Paste {
		return false
	}
	switch msg.Type {
	// Bubble Tea decodes the terminal Shift+F6 sequence as F18.
	case tea.KeyF6, tea.KeyF18:
		m.moveHistoryTool(msg.Type == tea.KeyF18)
		return true
	case tea.KeyEnter:
		if h := m.historyReader; h != nil && h.toolFocused {
			m.toggleHistoryTool(h.projection.selectedTool)
			return true
		}
	}
	return false
}
