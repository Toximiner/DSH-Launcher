package main

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type queueContent = struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type queuedMessage struct {
	ID        string         `json:"id"`
	RequestID string         `json:"rpcId"`
	Placement string         `json:"placement"`
	Content   []queueContent `json:"content"`
}

// Admission IDs prevent a late passive queue sample from duplicating chat history.
func (s *chatState) recordAdmission(id string, seq int) {
	if id == "" || len(id) > 256 {
		return
	}
	if s.admitted == nil {
		s.admitted = make(map[string]int)
	}
	s.admitted[id] = seq
	if len(s.admitted) > 2048 {
		oldestID := ""
		oldest := seq + 1
		for key, value := range s.admitted {
			if value < oldest {
				oldestID = key
				oldest = value
			}
		}
		delete(s.admitted, oldestID)
	}
}
func (m *model) reconcileQueuedPrompt() {
	if m.session == nil || m.chat == nil || m.session.ID != m.chat.id {
		return
	}
	p := m.prompts[m.session.ID]
	if p == nil {
		return
	}
	if _, ok := m.chat.admitted[p.ID]; ok {
		delete(m.prompts, m.session.ID)
		return
	}
	if !m.session.QueueKnown {
		return
	}
	for _, item := range m.session.Queue {
		if item.RequestID == p.ID {
			delete(m.prompts, m.session.ID)
			return
		}
	}
	// Absence alone never resolves an uncertain send. A confirmed receipt can
	// leave the queue via another client while our recent-history window is cold.
	if p.Status == "accepted" && !p.AcceptedAt.IsZero() && m.session.QueueFetchedAt.After(p.AcceptedAt) && m.chat.ready && m.chat.cursor >= m.session.StateCursor {
		delete(m.prompts, m.session.ID)
		m.chat.note = "Принятое сообщение больше не находится в очереди dsh."
	}
}
func (m *model) visibleQueue() []queuedMessage {
	if m.session == nil {
		return nil
	}
	var items []queuedMessage
	for _, item := range m.session.Queue {
		if item.Placement != "queued" && item.Placement != "steering" {
			continue
		}
		if m.chat != nil && m.chat.id == m.session.ID && item.RequestID != "" {
			if _, ok := m.chat.admitted[item.RequestID]; ok {
				continue
			}
		}
		items = append(items, item)
	}
	return items
}
func (m *model) queueView() string {
	items := m.visibleQueue()
	if len(items) == 0 {
		return ""
	}
	lines := []string{heading.Render("ОЧЕРЕДЬ DSH · " + strconv.Itoa(len(items))), muted.Render("F8: остановить текущую задачу; очередь сохранится.")}
	if m.sessionStale || m.chat == nil || !m.chat.ready {
		lines = append(lines, muted.Render("Последнее полученное состояние; связь восстанавливается."))
	}
	for i, item := range items {
		label := "Ожидает выполнения"
		if item.Placement == "steering" {
			label = "Уточнение текущему заданию"
		}
		text := boundedToolText(contentText(item.Content), toolArgumentsLimit)
		preview := strings.Split(ansi.Hardwrap(text, max(1, m.viewport.Width-2), true), "\n")
		if len(preview) > 3 {
			preview = append(preview[:3], "…")
		}
		lines = append(lines, accent.Render(strconv.Itoa(i+1)+". "+label)+"\n"+strings.Join(preview, "\n"))
	}
	return strings.Join(lines, "\n")
}
