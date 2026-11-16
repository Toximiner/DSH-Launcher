package main

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type sessionArchive struct {
	id                                string
	restore, focused, busy, attempted bool
	note                              string
}
type sessionArchiveResult struct {
	dialog *sessionArchive
	ids    map[string]bool
	err    error
}

func (m *model) openSessionArchive(current, restore bool) tea.Cmd {
	if m.settingsLogin == "" {
		m.append(tr("Архив доступен при подключении к dsh."))
		return nil
	}
	if m.chatSessionChanging() {
		m.append(tr("Дождитесь подключения сессии."))
		return nil
	}
	id := ""
	if current && m.session != nil {
		id = m.session.ID
	}
	if !current {
		items := m.visibleSessions()
		if len(items) > 0 {
			id = items[m.sessionListIndex].ID
		}
	}
	if id == "" {
		m.append(tr("Выберите сессию."))
		return nil
	}
	m.archiveDialog = &sessionArchive{id: id, restore: restore, focused: m.input.Focused()}
	m.input.Blur()
	return nil
}
func (m *model) archiveKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	d := m.archiveDialog
	if d == nil {
		if m.forkDialog == nil && m.renameDialog == nil && m.historyReader == nil && !m.chrome.menuOpen && m.chrome.dialog == "" && m.page == sessionsPage && !m.navFocused {
			if msg.Type == tea.KeyCtrlX {
				return m.openSessionArchive(false, m.showArchived), true
			}
			if msg.Type == tea.KeyRunes && !msg.Paste && !msg.Alt && string(msg.Runes) == "a" {
				m.showArchived = !m.showArchived
				m.sessionListIndex = 0
				m.rebuild()
				m.viewport.GotoTop()
				return m.ensureArchive(), true
			}
		}
		return nil, false
	}
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyCtrlD, tea.KeyF7, tea.KeyF8:
		return nil, false
	case tea.KeyEsc:
		if d.busy {
			return nil, true
		}
		m.archiveDialog = nil
		if d.focused && m.page == chatPage && !m.navFocused {
			return m.input.Focus(), true
		}
	case tea.KeyEnter:
		if d.attempted {
			return nil, true
		}
		d.attempted, d.busy = true, true
		d.note = tr("Сохранение в dsh…")
		ctx, login := m.settingsContext, m.settingsLogin
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
				method := "workspace/archiveSession"
				request := map[string]any{"sessionId": d.id, "stopActivity": false}
				if d.restore {
					method = "workspace/unarchiveSession"
					delete(request, "stopActivity")
				}
				var value struct {
					IDs []string `json:"archivedSessionIds"`
				}
				err = api.rpc(ctx, method, map[string]any{"request": request}, &value)
				if err == nil {
					result.ids, err = archiveIDs(value.IDs)
				}
				if err == nil && result.ids[d.id] == d.restore {
					err = errors.New(tr("dsh не подтвердил состояние сессии"))
				}
			}
			result.err = err
			return result
		}, true
	}
	return nil, true
}
func (m *model) handleSessionArchive(result sessionArchiveResult) tea.Cmd {
	d := m.archiveDialog
	if d == nil || d != result.dialog {
		return nil
	}
	d.busy = false
	if result.err != nil {
		d.note = tr("Не подтверждено. Проверьте архив; для активной сессии сначала остановите задачи. ") + clean(result.err.Error())
		return nil
	}
	m.archiveDialog = nil
	m.sessionListIndex = 0
	// A fresh stream supersedes any baseline captured before this receipt.
	stream := m.restartArchive(result.ids)
	m.rebuild()
	m.revealSelectedSession()
	var focus tea.Cmd
	if d.focused && m.page == chatPage && !m.navFocused {
		focus = m.input.Focus()
	}
	return tea.Batch(stream, focus)
}
func (m *model) archiveView(base string) string {
	d := m.archiveDialog
	if d == nil {
		return base
	}
	title := tr("Архивирование сессии")
	explanation := tr("История сохранится. Сессия перейдёт в архив. Работающие задачи могут запретить архивирование; сначала остановите их. Восстановить сессию можно из списка архива.")
	if d.restore {
		title = tr("Восстановление сессии")
		explanation = tr("Сессия вернётся в активный список с сохранённой историей. Новое сообщение можно отправить после восстановления.")
	}
	width, height := min(76, m.width-3), min(16, m.height-2)
	inner := width - 4
	lines := []string{heading().Render(title), ansi.Truncate(clean(d.id), inner, "…"), "", ansi.Wrap(explanation, inner, ""), ""}
	if d.note != "" {
		note := strings.Split(ansi.Wrap(d.note, inner, ""), "\n")
		if len(note) > 3 {
			note = note[:3]
		}
		lines = append(lines, strings.Join(note, "\n"))
	}
	hint := tr("Enter — подтвердить · Esc — назад")
	if d.busy {
		hint = tr("Ожидание ответа dsh…")
	} else if d.attempted {
		hint = tr("Esc — закрыть и проверить архив")
	}
	lines = append(lines, hint)
	return overlay(base, panel(width, height, strings.Join(lines, "\n"), "#58a6ff"), (m.width-1-width)/2, (m.height-height)/2, m.width-1)
}
