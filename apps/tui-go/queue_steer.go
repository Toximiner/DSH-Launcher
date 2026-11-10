package main

import (
	"context"
	"errors"
	"reflect"

	tea "github.com/charmbracelet/bubbletea"
)

type queueSteerResult struct {
	epoch             int
	sessionID, itemID string
	err               error
}

func (m *model) requestQueueSteering() {
	s := &m.queueDialog
	if s.busy || s.edit != nil || s.confirm != "" || !m.queueAvailable() {
		return
	}
	if !m.session.Running {
		s.note = "Уточнение доступно только во время работы агента."
		return
	}
	item, _ := m.queueSelection()
	if item.ID == "" {
		return
	}
	if item.Placement != "queued" {
		s.note = "Это сообщение уже передано как уточнение."
		return
	}
	s.confirm = item.ID
	s.confirmKind = "steer"
	s.confirmText = boundedToolText(contentText(item.Content), toolArgumentsLimit)
	s.confirmContent = append([]queueContent(nil), item.Content...)
	s.accept = false
	s.note = "Учесть на следующем шаге? Остальная очередь сохранится."
}
func (m *model) steerQueuedItem() tea.Cmd {
	s := &m.queueDialog
	if s.busy || !s.accept || s.confirm == "" || s.confirmKind != "steer" || !m.queueAvailable() {
		return nil
	}
	if !m.session.Running {
		s.note = "Текущая задача завершена. Уточнение не отправлено."
		return nil
	}
	var item *queuedMessage
	for _, entry := range m.visibleQueue() {
		if entry.ID == s.confirm {
			copy := entry
			item = &copy
			break
		}
	}
	if item == nil || item.Placement != "queued" {
		s.confirm = ""
		s.confirmKind = ""
		s.note = "Сообщение уже покинуло очередь заданий. Обновляем состояние."
		return nil
	}
	if !reflect.DeepEqual(item.Content, s.confirmContent) {
		s.confirm = ""
		s.confirmKind = ""
		s.note = "Текст изменён другим клиентом. Выберите задание заново."
		return nil
	}
	id, itemID, epoch, login, ctx := m.session.ID, s.confirm, m.chat.epoch, m.settingsLogin, m.settingsContext
	if ctx == nil {
		ctx = context.Background()
	}
	s.busy = true
	s.operationID = itemID
	s.note = "Передача уточнения…"
	return func() tea.Msg {
		result := queueSteerResult{epoch: epoch, sessionID: id, itemID: itemID}
		api, err := openAPI(ctx, login)
		if err != nil {
			result.err = err
			return result
		}
		defer api.close()
		var receipt struct {
			Accepted bool `json:"accepted"`
		}
		result.err = api.rpc(ctx, "session/updateQueue", map[string]any{"request": map[string]any{"sessionId": id, "itemId": itemID, "action": map[string]any{"kind": "steer"}}}, &receipt)
		if result.err == nil && !receipt.Accepted {
			result.err = errors.New("неподдерживаемое подтверждение уточнения")
		}
		return result
	}
}
func (m *model) handleQueueSteering(msg queueSteerResult) tea.Cmd {
	if m.chat == nil || m.chat.epoch != msg.epoch || m.chat.id != msg.sessionID || m.session == nil || m.session.ID != msg.sessionID {
		return nil
	}
	s := &m.queueDialog
	if !s.busy || s.operationID != msg.itemID {
		return nil
	}
	s.busy = false
	s.operationID = ""
	s.confirm = ""
	s.confirmKind = ""
	s.accept = false
	if msg.err == nil {
		s.note = "Уточнение принято dsh. Оно будет учтено на следующем шаге."
	} else {
		var rejected *rpcRejected
		if errors.As(msg.err, &rejected) {
			s.note = "dsh отклонил уточнение: текущая задача могла уже завершиться."
		} else {
			s.note = "Передача не подтверждена; автоповтора нет. Обновляем очередь."
		}
	}
	m.rebuild()
	if !m.sessionBusy {
		return m.sessionOperation("refresh", "")
	}
	return nil
}
