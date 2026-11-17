package main

import (
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type reasoningHit struct {
	key string
	row int
}

func reasoningEventKey(seq int) string     { return "event:" + strconv.Itoa(seq) }
func reasoningAttemptKey(id string) string { return "attempt:" + id }

// Only explicit public reasoning blocks belong here. Tool arguments, encrypted
// protocol fields and normal assistant prose are never inferred as thoughts.
func reasoningContent(content []struct {
	Type string `json:"type"`
	Text string `json:"text"`
}) string {
	var parts []string
	for _, part := range content {
		if part.Type == "reasoning" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func copyReasoningBlocks(source map[int]string) map[int]string {
	if source == nil {
		return nil
	}
	result := make(map[int]string, len(source))
	for index, text := range source {
		result[index] = text
	}
	return result
}
func copyReasoningExpansion(source map[string]bool) map[string]bool {
	if source == nil {
		return nil
	}
	result := make(map[string]bool, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
func (a *chatAttempt) reasoning() string {
	indices := make([]int, 0, len(a.reasoningBlocks))
	for index := range a.reasoningBlocks {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	parts := make([]string, 0, len(indices))
	for _, index := range indices {
		parts = append(parts, a.reasoningBlocks[index])
	}
	return clean(strings.Join(parts, "\n"))
}

// Count raw retained bytes, not just sanitized output: invisible escape payloads
// must consume the same stream budget as visible source. Reserve per-block
// bookkeeping too, so empty deltas cannot grow the retained maps unboundedly.
func (a *chatAttempt) byteSize() int {
	size := 32 * (len(a.blocks) + len(a.reasoningBlocks))
	for _, text := range a.blocks {
		size += len(text)
	}
	for _, text := range a.reasoningBlocks {
		size += len(text)
	}
	return size
}

func reasoningKeys(s *chatState) []string {
	if s == nil {
		return nil
	}
	keys := []string{}
	for _, message := range s.messages {
		if message.seq > s.clearedThrough && strings.TrimSpace(message.reasoning) != "" {
			keys = append(keys, reasoningEventKey(message.seq))
		}
	}
	if s.live != nil && strings.TrimSpace(s.live.reasoning()) != "" {
		keys = append(keys, reasoningAttemptKey(s.live.ID))
	}
	return keys
}
func (m *model) hasReasoning() bool { return len(reasoningKeys(m.chat)) > 0 }

func toggleReasoningState(s *chatState) bool {
	keys := reasoningKeys(s)
	if len(keys) == 0 {
		return false
	}
	expanded := true
	for _, key := range keys {
		if !s.expandedReasoning[key] {
			expanded = false
			break
		}
	}
	if s.expandedReasoning == nil {
		s.expandedReasoning = make(map[string]bool)
	}
	for _, key := range keys {
		if expanded {
			delete(s.expandedReasoning, key)
		} else {
			s.expandedReasoning[key] = true
		}
	}
	return true
}
func (m *model) toggleReasoning() tea.Cmd {
	if toggleReasoningState(m.chat) {
		m.rebuild()
	}
	return nil
}
func pruneReasoningExpansion(s *chatState) {
	if s == nil || len(s.expandedReasoning) == 0 {
		return
	}
	present := map[string]bool{}
	for _, key := range reasoningKeys(s) {
		present[key] = true
	}
	for key := range s.expandedReasoning {
		if !present[key] {
			delete(s.expandedReasoning, key)
		}
	}
}

func renderReasoning(s *chatState, key, text string, width, row int) string {
	text = clean(text)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	width = max(1, width)
	expanded := s != nil && s.expandedReasoning[key]
	marker := "▸ "
	if expanded {
		marker = "▾ "
	}
	label := muted().Render(ansi.Truncate(marker+tr("Размышления"), width, "…"))
	if s != nil {
		s.reasoningHits = append(s.reasoningHits, reasoningHit{key: key, row: row})
	}
	if !expanded {
		return label
	}
	lines := strings.Split(ansi.Hardwrap(text, max(1, width-2), true), "\n")
	for i := range lines {
		lines[i] = muted().Render("  " + lines[i])
	}
	return label + "\n" + strings.Join(lines, "\n")
}

func (m *model) reasoningMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if m.chat == nil || !m.conversationVisible() || msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil, false
	}
	box := m.conversationArea()
	if !box.contains(msg.X, msg.Y) {
		return nil, false
	}
	row := msg.Y - box.y + m.viewport.YOffset
	for _, hit := range m.chat.reasoningHits {
		if hit.row != row {
			continue
		}
		if m.chat.expandedReasoning == nil {
			m.chat.expandedReasoning = make(map[string]bool)
		}
		if m.chat.expandedReasoning[hit.key] {
			delete(m.chat.expandedReasoning, hit.key)
		} else {
			m.chat.expandedReasoning[hit.key] = true
		}
		m.rebuild()
		return nil, true
	}
	return nil, false
}
