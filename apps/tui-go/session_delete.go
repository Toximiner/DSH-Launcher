package main

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func (m *model) openSessionDelete(current bool) tea.Cmd {
	cmd := m.openSessionArchive(current, false)
	if d := m.archiveDialog; d != nil {
		d.deleting = true
		d.login = m.settingsLogin
	}
	return cmd
}

func (m *model) sessionDeleteKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	d := m.archiveDialog
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyCtrlD, tea.KeyF7, tea.KeyF8:
		return nil, false
	case tea.KeyEsc:
		if !d.busy {
			m.archiveDialog = nil
			if d.focused && m.page == chatPage && !m.navFocused {
				return m.input.Focus(), true
			}
		}
	case tea.KeyTab, tea.KeyShiftTab, tea.KeyLeft, tea.KeyRight, tea.KeyUp, tea.KeyDown:
		if !d.busy && !d.attempted {
			d.confirmDelete = !d.confirmDelete
		}
	case tea.KeyEnter:
		if d.busy || d.attempted {
			return nil, true
		}
		if !d.confirmDelete {
			return m.sessionDeleteKey(tea.KeyMsg{Type: tea.KeyEsc})
		}
		if p := m.prompts[d.id]; p != nil && p.Status != "rejected" {
			d.attempted = true
			d.note = tr("Дождитесь подтверждения отправленного сообщения.")
			return nil, true
		}
		d.attempted, d.busy = true, true
		d.note = tr("Проверка активности и удаление…")
		login, ctx := d.login, m.settingsContext
		if ctx == nil {
			ctx = context.Background()
		}
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			result := sessionArchiveResult{dialog: d}
			api, err := openAPI(ctx, login)
			if err == nil {
				defer api.close()
				result.deletedIDs, err = deleteSessionPermanently(ctx, api, d.id)
			}
			result.err = err
			return result
		}, true
	}
	return nil, true
}

// Use the launcher module API; never remove profile files from the client.
func deleteSessionPermanently(ctx context.Context, api *dshAPI, id string) ([]string, error) {
	var catalog struct {
		Items []struct {
			ID string `json:"sessionId"`
		} `json:"items"`
	}
	if err := api.rpc(ctx, "session/list", map[string]any{"_request": map[string]any{}}, &catalog); err != nil {
		return nil, err
	}
	found := false
	for _, item := range catalog.Items {
		if item.ID == id {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New(tr("Сессия отсутствует в свежем списке dsh."))
	}
	var state *struct {
		Running      *bool            `json:"running"`
		Queue        *[]queuedMessage `json:"queue"`
		PendingInput []string         `json:"pendingInput"`
	}
	if err := api.rpc(ctx, "session/state", map[string]any{"request": map[string]any{"sessionId": id}}, &state); err != nil {
		return nil, err
	}
	if state == nil || state.Queue == nil || state.Running == nil {
		return nil, errors.New(tr("Не удалось проверить активность сессии."))
	}
	if *state.Running || len(*state.Queue) > 0 || len(state.PendingInput) > 0 {
		return nil, errors.New(tr("Сначала остановите работу сессии и очистите очередь."))
	}
	var receipt struct {
		Deleted    *bool    `json:"deleted"`
		SessionIDs []string `json:"sessionIds"`
		Reason     string   `json:"reason"`
	}
	if err := api.rpc(ctx, "launcherSessions/deleteSession", map[string]any{"sessionId": id}, &receipt); err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return nil, errors.New(tr("Встроенное удаление недоступно. Перезапустите dsh через обновлённый лаунчер."))
		}
		return nil, err
	}
	if receipt.Deleted != nil && !*receipt.Deleted {
		switch receipt.Reason {
		case "active":
			return nil, errors.New(tr("Сначала остановите работу сессии и её подагентов и очистите очередь."))
		case "unsupported":
			return nil, errors.New(tr("Встроенное удаление недоступно. Перезапустите dsh через обновлённый лаунчер."))
		case "missing":
			return nil, errors.New(tr("Сессия отсутствует в свежем списке dsh."))
		}
	}
	if receipt.Deleted == nil || !*receipt.Deleted {
		return nil, errors.New(tr("dsh не подтвердил удаление сессии."))
	}
	seen := map[string]bool{}
	if len(receipt.SessionIDs) == 0 || len(receipt.SessionIDs) > 100000 {
		return nil, errors.New(tr("dsh не подтвердил список удалённых сессий."))
	}
	for _, deletedID := range receipt.SessionIDs {
		if deletedID == "" || seen[deletedID] {
			return nil, errors.New(tr("dsh не подтвердил список удалённых сессий."))
		}
		seen[deletedID] = true
	}
	if !seen[id] {
		return nil, errors.New(tr("dsh не подтвердил список удалённых сессий."))
	}
	return receipt.SessionIDs, nil
}

func (m *model) handleSessionDelete(result sessionArchiveResult) tea.Cmd {
	d := m.archiveDialog
	d.busy = false
	if result.err != nil {
		d.note = tr("Удаление не подтверждено. ") + clean(result.err.Error()) + "\n" + tr("Проверьте список сессий перед повтором.")
		return nil
	}
	m.archiveDialog = nil
	if d.login != m.settingsLogin {
		return nil
	}
	deleted := map[string]bool{d.id: true}
	for _, id := range result.deletedIDs {
		deleted[id] = true
	}
	for id := range deleted {
		m.tabForgetDeletedSession(id)
		delete(m.prompts, id)
	}
	kept := m.sessionItems[:0]
	for _, item := range m.sessionItems {
		if !deleted[item.ID] {
			kept = append(kept, item)
		}
	}
	m.sessionItems = kept
	// Invalidate old sidebar refreshes before fetching the confirmed catalog.
	m.side.sequence++
	m.side.busy = false
	m.side.sessions = nil
	for i := range m.side.workspaces {
		ids := m.side.workspaces[i].Sessions[:0]
		for _, id := range m.side.workspaces[i].Sessions {
			if !deleted[id] {
				ids = append(ids, id)
			}
		}
		m.side.workspaces[i].Sessions = ids
	}
	ids := map[string]bool{}
	if m.archive != nil {
		for id, value := range m.archive.ids {
			if !deleted[id] {
				ids[id] = value
			}
		}
	}
	m.sessionListIndex = 0
	m.rebuild()
	return tea.Batch(m.restartArchive(ids), m.loadSidebar(), m.sessionOperation("list", ""))
}

func (m *model) sessionDeleteLayout() (area, []string, []area) {
	d := m.archiveDialog
	box := m.chooserLayout(chooserSpec{Width: 76, Height: 24}).Box
	// Destructive explanations need room even on the minimum terminal.
	box.height = min(18, m.height-2)
	box.y = (m.height - box.height) / 2
	width := box.width - 4
	lines := []string{bar(heading().Render(tr("Удалить сессию навсегда?")), muted().Render("esc"), width), ansi.Truncate(clean(d.id), width, "…"), ""}
	explanation := tr("История и вложения сессии будут удалены без возможности восстановления. Подагенты также будут удалены. Файлы проекта и отдельные fork-сессии сохраняются.")
	lines = append(lines, strings.Split(ansi.Wrap(explanation, width, ""), "\n")...)
	lines = append(lines, "")
	choices := []area{{box.x + 2, box.y + 1 + len(lines), width, 1}, {box.x + 2, box.y + 2 + len(lines), width, 1}}
	lines = append(lines, chooserItem(tr("Отмена"), "", !d.confirmDelete, true, width), chooserItem(tr("Удалить навсегда"), "", d.confirmDelete, true, width))
	note := strings.Split(ansi.Wrap(clean(d.note), width, ""), "\n")
	lines = append(lines, note[:min(3, len(note))]...)
	hint := tr("Tab / ← → выбор · Enter подтвердить · Esc отмена")
	if d.busy {
		hint = tr("Ожидание ответа dsh…")
	} else if d.attempted {
		hint = tr("Esc — закрыть и проверить список сессий")
	}
	lines = append(lines, "", hint)
	return box, lines, choices
}

func (m *model) sessionDeleteView(base string) string {
	box, lines, _ := m.sessionDeleteLayout()
	return overlay(dimBackdrop(base), flatDialogSurface(box, lines), box.x, box.y, m.width-1)
}
func (m *model) sessionDeleteMouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return nil
	}
	box, _, choices := m.sessionDeleteLayout()
	if flatDialogClose(box).contains(msg.X, msg.Y) {
		cmd, _ := m.sessionDeleteKey(tea.KeyMsg{Type: tea.KeyEsc})
		return cmd
	}
	d := m.archiveDialog
	if d.busy || d.attempted {
		return nil
	}
	for i, choice := range choices {
		if choice.contains(msg.X, msg.Y) {
			d.confirmDelete = i == 1
			cmd, _ := m.sessionDeleteKey(tea.KeyMsg{Type: tea.KeyEnter})
			return cmd
		}
	}
	return nil
}
