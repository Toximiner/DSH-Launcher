package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const queueEditLimit = 65536

type queueEditDraft struct {
	itemID         string
	original       []queueContent
	editor         textarea.Model
	waitFreshAfter time.Time
}
type queueEditResult struct {
	epoch             int
	sessionID, itemID string
	err               error
}

func (m *model) editQueuedItem() tea.Cmd {
	s := &m.queueDialog
	item, _ := m.queueSelection()
	if s.busy || s.confirm != "" || !m.queueAvailable() || item.ID == "" {
		return nil
	}
	if len(item.Content) == 0 {
		return nil
	}
	for _, block := range item.Content {
		if block.Type != "text" || clean(block.Text) != block.Text {
			s.note = "Только обычный текст: редактирование вложений недоступно."
			return nil
		}
	}
	text := contentText(item.Content)
	if len(text) > queueEditLimit {
		s.note = "Текст слишком большой для редактора (лимит 64 КиБ)."
		return nil
	}
	editor := textarea.New()
	editor.CharLimit = queueEditLimit
	editor.ShowLineNumbers = false
	editor.Prompt = "> "
	editor.Placeholder = "Текст ожидающего задания…"
	editor.SetWidth(max(1, m.queueBox().width-4))
	editor.SetHeight(max(1, m.queueBox().height-8))
	editor.SetValue(text)
	if editor.Value() != text {
		s.note = "Текст нельзя открыть без преобразования. Изменения не внесены."
		return nil
	}
	s.edit = &queueEditDraft{itemID: item.ID, original: append([]queueContent(nil), item.Content...), editor: editor}
	s.note = "Enter — новая строка. F5 — сохранить в dsh."
	return s.edit.editor.Focus()
}
func (m *model) saveQueuedEdit() tea.Cmd {
	s := &m.queueDialog
	d := s.edit
	if d == nil || s.busy {
		return nil
	}
	if !m.queueAvailable() || !d.waitFreshAfter.IsZero() && !m.session.QueueFetchedAt.After(d.waitFreshAfter) {
		s.note = "Дождитесь свежего состояния dsh. Черновик сохранён."
		return nil
	}
	text := d.editor.Value()
	if strings.TrimSpace(text) == "" {
		s.note = "Введите непустой текст задания."
		return nil
	}
	if len(text) > queueEditLimit || clean(text) != text {
		s.note = "Текст превышает 64 КиБ или содержит управляющие символы."
		return nil
	}
	var current *queuedMessage
	for _, item := range m.visibleQueue() {
		if item.ID == d.itemID {
			copy := item
			current = &copy
			break
		}
	}
	if current == nil {
		s.note = "Задание уже покинуло очередь. Черновик остался в редакторе."
		return nil
	}
	content := []queueContent{{Type: "text", Text: text}}
	if reflect.DeepEqual(current.Content, content) {
		s.edit = nil
		s.note = "Этот текст уже сохранён в очереди dsh."
		return nil
	}
	if !reflect.DeepEqual(current.Content, d.original) {
		s.note = "Задание изменено другим клиентом. Запись заблокирована."
		return nil
	}
	id, itemID, epoch, login, ctx := m.session.ID, d.itemID, m.chat.epoch, m.settingsLogin, m.settingsContext
	if ctx == nil {
		ctx = context.Background()
	}
	s.busy = true
	s.operationID = itemID
	s.note = "Сохранение задания…"
	return func() tea.Msg {
		result := queueEditResult{epoch: epoch, sessionID: id, itemID: itemID}
		api, err := openAPI(ctx, login)
		if err != nil {
			result.err = err
			return result
		}
		defer api.close()
		var receipt struct {
			Accepted bool `json:"accepted"`
		}
		result.err = api.rpc(ctx, "session/updateQueue", map[string]any{"request": map[string]any{"sessionId": id, "itemId": itemID, "action": map[string]any{"kind": "edit", "content": content}}}, &receipt)
		if result.err == nil && !receipt.Accepted {
			result.err = errors.New("неподдерживаемое подтверждение сохранения")
		}
		return result
	}
}
func (m *model) handleQueueEdit(msg queueEditResult) tea.Cmd {
	if m.chat == nil || m.chat.epoch != msg.epoch || m.chat.id != msg.sessionID || m.session == nil || m.session.ID != msg.sessionID {
		return nil
	}
	s := &m.queueDialog
	if !s.busy || s.operationID != msg.itemID || s.edit == nil || s.edit.itemID != msg.itemID {
		return nil
	}
	s.busy = false
	s.operationID = ""
	if msg.err == nil {
		s.edit = nil
		s.note = "Задание изменено. Порядок очереди сохранён."
	} else {
		s.edit.waitFreshAfter = time.Now()
		var rejected *rpcRejected
		if errors.As(msg.err, &rejected) {
			s.note = "dsh отклонил изменение. Черновик сохранён; обновляем очередь."
		} else {
			s.note = "Сохранение не подтверждено. Черновик сохранён; автоповтора нет."
		}
	}
	m.rebuild()
	if !m.sessionBusy {
		return m.sessionOperation("refresh", "")
	}
	return nil
}
func (m *model) queueEditKey(msg tea.KeyMsg) tea.Cmd {
	s := &m.queueDialog
	if !msg.Paste {
		switch msg.Type {
		case tea.KeyEsc:
			if !s.busy {
				s.edit = nil
				s.note = "Изменения не сохранены."
			} else {
				s.visible = false
			}
			return nil
		case tea.KeyF5:
			return m.saveQueuedEdit()
		}
	}
	if s.busy {
		return nil
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Alt {
		msg.Paste = true
	}
	var cmd tea.Cmd
	s.edit.editor, cmd = s.edit.editor.Update(msg)
	return cmd
}
func (m *model) queueEditView(base string) string {
	s := &m.queueDialog
	d := s.edit
	box := m.queueBox()
	inner := box.width - 4
	d.editor.SetWidth(inner)
	d.editor.SetHeight(max(1, box.height-8))
	note := s.note
	if !m.queueAvailable() {
		note = "Нет связи с dsh. Черновик сохранён; запись недоступна."
	}
	noteLines := strings.Split(ansi.Hardwrap(note, inner, true), "\n")
	lines := []string{heading.Render("Изменить задание в очереди"), muted.Render("Enter — новая строка · F5 — сохранить"), d.editor.View()}
	for i := 0; i < 2; i++ {
		line := ""
		if i < len(noteLines) {
			line = noteLines[i]
		}
		lines = append(lines, muted.Render(line))
	}
	buttons := "[ Сохранить F5 ]   [ Назад Esc ]"
	if s.busy {
		buttons = "Ожидание подтверждения dsh…"
	}
	lines = append(lines, buttons, muted.Render("Порядок очереди и текущая задача сохранятся."))
	for i, line := range lines {
		if i != 2 {
			lines[i] = ansi.Truncate(line, inner, "…")
		}
	}
	return overlay(base, panel(box.width, box.height, strings.Join(lines, "\n"), "#58a6ff"), box.x, box.y, m.width-1)
}
func (m *model) queueEditMouse(msg tea.MouseMsg) tea.Cmd {
	box := m.queueBox()
	if msg.Action != tea.MouseActionPress {
		return nil
	}
	if msg.Button == tea.MouseButtonLeft && msg.Y == box.y+box.height-3 && msg.X >= box.x+2 && msg.X < box.x+box.width-2 {
		if msg.X < box.x+2+lipgloss.Width("[ Сохранить F5 ]") {
			return m.saveQueuedEdit()
		}
		return m.queueEditKey(tea.KeyMsg{Type: tea.KeyEsc})
	}
	// Keep scrolling inside the editor; background chat and list stay untouched.
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		kind := tea.KeyDown
		if msg.Button == tea.MouseButtonWheelUp {
			kind = tea.KeyUp
		}
		return m.queueEditKey(tea.KeyMsg{Type: kind})
	}
	return nil
}
