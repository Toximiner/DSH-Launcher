package main

import (
	"context"
	"errors"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type queueDialog struct {
	edit        *queueEditDraft
	visible     bool
	selected    string
	confirm     string
	confirmText string
	operationID string
	remove      bool
	busy        bool
	note        string
}
type queueRemoveResult struct {
	epoch             int
	sessionID, itemID string
	err               error
}

func (m *model) openQueue() tea.Cmd {
	m.queueDialog.visible = true
	m.queueDialog.confirm = ""
	m.queueDialog.remove = false
	if m.queueDialog.note == "" {
		m.queueDialog.note = "Текущая задача продолжает выполняться."
	}
	return nil
}
func (m *model) queueSelection() (queuedMessage, int) {
	items := m.visibleQueue()
	for i, item := range items {
		if item.ID == m.queueDialog.selected {
			return item, i
		}
	}
	if len(items) > 0 {
		m.queueDialog.selected = items[0].ID
		return items[0], 0
	}
	m.queueDialog.selected = ""
	return queuedMessage{}, -1
}
func (m *model) queueAvailable() bool {
	return m.session != nil && m.session.QueueKnown && !m.sessionStale && m.chat != nil && m.chat.ready && m.chat.id == m.session.ID && !m.chatSessionChanging() && !m.browserPreview && m.settingsLogin != ""
}
func (m *model) requestQueueRemoval() {
	item, _ := m.queueSelection()
	if m.queueDialog.busy || !m.queueAvailable() || item.ID == "" {
		return
	}
	m.queueDialog.confirm = item.ID
	m.queueDialog.confirmText = boundedToolText(contentText(item.Content), toolArgumentsLimit)
	m.queueDialog.remove = false
	m.queueDialog.note = "Удалить выбранное задание из очереди?"
}
func (m *model) removeQueuedItem() tea.Cmd {
	s := &m.queueDialog
	if s.busy || !s.remove || s.confirm == "" || !m.queueAvailable() {
		return nil
	}
	found := false
	for _, item := range m.visibleQueue() {
		if item.ID == s.confirm {
			found = true
		}
	}
	if !found {
		s.confirm = ""
		s.note = "Задание уже покинуло очередь. Состояние обновляется."
		return nil
	}
	id, itemID, epoch, login, ctx := m.session.ID, s.confirm, m.chat.epoch, m.settingsLogin, m.settingsContext
	if ctx == nil {
		ctx = context.Background()
	}
	s.busy = true
	s.operationID = itemID
	s.note = "Удаление задания…"
	return func() tea.Msg {
		result := queueRemoveResult{epoch: epoch, sessionID: id, itemID: itemID}
		api, err := openAPI(ctx, login)
		if err != nil {
			result.err = err
			return result
		}
		defer api.close()
		var receipt struct {
			Accepted bool `json:"accepted"`
		}
		result.err = api.rpc(ctx, "session/updateQueue", map[string]any{"request": map[string]any{"sessionId": id, "itemId": itemID, "action": map[string]any{"kind": "remove"}}}, &receipt)
		if result.err == nil && !receipt.Accepted {
			result.err = errors.New("неподдерживаемое подтверждение удаления")
		}
		return result
	}
}
func (m *model) handleQueueRemoval(msg queueRemoveResult) tea.Cmd {
	if m.chat == nil || m.chat.epoch != msg.epoch || m.chat.id != msg.sessionID || m.session == nil || m.session.ID != msg.sessionID {
		return nil
	}
	s := &m.queueDialog
	if !s.busy || s.operationID != msg.itemID {
		return nil
	}
	s.operationID = ""
	s.busy, s.confirm, s.remove = false, "", false
	if msg.err == nil {
		s.note = "Задание удалено. Текущая задача продолжает выполняться."
	} else {
		var rejected *rpcRejected
		if errors.As(msg.err, &rejected) {
			s.note = "dsh отклонил удаление: задание могло уже начаться. Обновляем очередь."
		} else {
			s.note = "Удаление не подтверждено. Обновляем очередь; автоматического повтора нет."
		}
	}
	m.rebuild()
	if !m.sessionBusy {
		return m.sessionOperation("refresh", "")
	}
	return nil
}
func (m *model) queueKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	s := &m.queueDialog
	if msg.Type == tea.KeyF9 && !msg.Paste {
		if s.visible {
			s.visible = false
			s.confirm = ""
		} else {
			return m.openQueue(), true
		}
		return nil, true
	}
	if !s.visible {
		return nil, false
	}
	if s.edit != nil {
		if !msg.Paste {
			switch msg.Type {
			case tea.KeyCtrlC, tea.KeyCtrlD:
				return tea.Quit, true
			case tea.KeyF8:
				return m.cancelChat(), true
			}
		}
		return m.queueEditKey(msg), true
	}
	if msg.Paste {
		return nil, true
	}
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyCtrlD:
		return tea.Quit, true
	case tea.KeyF8:
		return m.cancelChat(), true
	case tea.KeyEsc:
		if s.confirm != "" && !s.busy {
			s.confirm = ""
			s.note = "Удаление отменено."
		} else {
			s.visible = false
		}
	case tea.KeyUp, tea.KeyDown:
		if s.confirm == "" && !s.busy {
			_, i := m.queueSelection()
			items := m.visibleQueue()
			if i >= 0 {
				delta := 1
				if msg.Type == tea.KeyUp {
					delta = -1
				}
				s.selected = items[(i+delta+len(items))%len(items)].ID
			}
		}
	case tea.KeyLeft, tea.KeyRight, tea.KeyTab, tea.KeyShiftTab:
		if s.confirm != "" && !s.busy {
			s.remove = !s.remove
		}
	case tea.KeyF3:
		return m.editQueuedItem(), true
	case tea.KeyEnter:
		if s.confirm == "" {
			m.requestQueueRemoval()
		} else if s.remove {
			return m.removeQueuedItem(), true
		} else if !s.busy {
			s.confirm = ""
			s.note = "Удаление отменено."
		}
	}
	return nil, true
}
func (m *model) queueBox() area {
	width, height := min(82, m.width-3), min(25, m.height-4)
	return area{(m.width - 1 - width) / 2, max(2, (m.height-height)/2), width, height}
}
func (m *model) queueDialogView(base string) string {
	s := &m.queueDialog
	if !s.visible || m.width < 44 || m.height < 18 {
		return base
	}
	if s.edit != nil {
		return m.queueEditView(base)
	}
	box := m.queueBox()
	inner := box.width - 4
	item, index := m.queueSelection()
	items := m.visibleQueue()
	lines := []string{heading.Render("Очередь заданий dsh"), muted.Render("F8 останавливает текущую задачу, сохраняя очередь.")}
	rows := max(1, min(5, box.height-11))
	start := max(0, index-rows+1)
	for i := 0; i < rows; i++ {
		text := ""
		if n := start + i; n < len(items) {
			text = strconv.Itoa(n+1) + ". " + strings.ReplaceAll(boundedToolText(contentText(items[n].Content), 1024), "\n", " ")
			text = selectedRow(ansi.Truncate(text, inner, "…"), n == index, inner)
		}
		lines = append(lines, text)
	}
	if len(items) == 0 {
		lines[2] = "Нет ожидающих заданий."
	}
	detail := ""
	if item.ID != "" {
		detail = boundedToolText(contentText(item.Content), toolArgumentsLimit)
	}
	if s.confirm != "" {
		detail = s.confirmText
		lines[0] = heading.Render("Удалить задание из очереди?")
	}
	details := strings.Split(ansi.Hardwrap(detail, max(1, inner), true), "\n")
	detailRows := box.height - rows - 8
	for i := 0; i < detailRows; i++ {
		text := ""
		if i < len(details) {
			text = details[i]
		}
		lines = append(lines, text)
	}
	note := s.note
	if !m.queueAvailable() {
		note = "Нет свежего состояния dsh. Удаление недоступно."
	}
	// Keep two note rows visible even at the minimum terminal size.
	noteLines := strings.Split(ansi.Hardwrap(note, max(1, inner), true), "\n")
	for i := 0; i < 2; i++ {
		text := ""
		if i < len(noteLines) {
			text = noteLines[i]
		}
		lines = append(lines, muted.Render(text))
	}
	buttons := "[ Удалить выбранное ] [ Изменить F3 ]"
	if s.confirm != "" {
		left := inner / 2
		buttons = lipgloss.NewStyle().Width(left).Render(selectedRow("[ Назад ]", !s.remove, left)) + selectedRow("[ Удалить ]", s.remove, inner-left)
	}
	if s.busy {
		buttons = "Ожидание подтверждения dsh…"
	}
	hint := "↑/↓ выбор · F3 правка · Enter удалить"
	if s.confirm != "" {
		hint = "←/→ выбор · Enter ответ · Esc назад"
	}
	lines = append(lines, buttons, muted.Render(hint))
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, inner, "…")
	}
	return overlay(base, panel(box.width, box.height, strings.Join(lines, "\n"), "#58a6ff"), box.x, box.y, m.width-1)
}
func (m *model) queueMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if !m.queueDialog.visible {
		return nil, false
	}
	if m.queueDialog.edit != nil {
		return m.queueEditMouse(msg), true
	}
	if msg.Action != tea.MouseActionPress {
		return nil, true
	}
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		kind := tea.KeyDown
		if msg.Button == tea.MouseButtonWheelUp {
			kind = tea.KeyUp
		}
		return m.queueKey(tea.KeyMsg{Type: kind})
	}
	if msg.Button != tea.MouseButtonLeft {
		return nil, true
	}
	box := m.queueBox()
	inner := box.width - 4
	if !(area{box.x + 2, box.y + 1, inner, box.height - 2}).contains(msg.X, msg.Y) {
		return nil, true
	}
	rows := max(1, min(5, box.height-11))
	_, index := m.queueSelection()
	start := max(0, index-rows+1)
	if m.queueDialog.confirm == "" && !m.queueDialog.busy && msg.Y >= box.y+3 && msg.Y < box.y+3+rows {
		items := m.visibleQueue()
		n := start + msg.Y - box.y - 3
		if n < len(items) {
			m.queueDialog.selected = items[n].ID
		}
	}
	if msg.Y == box.y+box.height-3 {
		if m.queueDialog.confirm == "" && msg.X >= box.x+2+len([]rune("[ Удалить выбранное ] ")) {
			return m.editQueuedItem(), true
		}
		if m.queueDialog.confirm != "" {
			m.queueDialog.remove = msg.X >= box.x+2+inner/2
		}
		return m.queueKey(tea.KeyMsg{Type: tea.KeyEnter})
	}
	return nil, true
}
