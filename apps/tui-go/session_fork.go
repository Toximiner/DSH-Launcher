package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type sessionFork struct {
	atSeq                    *int
	preview                  string
	id                       string
	focused, busy, attempted bool
	note                     string
}

type sessionForkResult struct {
	dialog  *sessionFork
	childID string
	err     error
}

func (m *model) openSessionFork(current bool) tea.Cmd {
	if m.settingsLogin == "" {
		m.append(tr("Ветвление доступно при подключении к dsh."))
		return nil
	}
	if m.chatSessionChanging() {
		m.append(tr("Дождитесь подключения сессии перед ветвлением."))
		return nil
	}
	id := ""
	if current && m.session != nil {
		id = m.session.ID
	}
	if !current && len(m.visibleSessions()) > 0 {
		id = m.visibleSessions()[m.sessionListIndex].ID
	}
	if id == "" {
		m.append(tr("Выберите сессию для ветвления."))
		return nil
	}
	m.forkDialog = &sessionFork{id: id, focused: m.input.Focused()}
	m.input.Blur()
	return nil
}

func (m *model) forkKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	d := m.forkDialog
	if d == nil {
		if msg.Type == tea.KeyCtrlB && m.page == sessionsPage && !m.navFocused && m.renameDialog == nil && m.historyReader == nil && !m.chrome.menuOpen && m.chrome.dialog == "" {
			return m.openSessionFork(false), true
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
		m.forkDialog = nil
		if d.focused && m.historyReader == nil && m.page == chatPage && !m.navFocused {
			return m.input.Focus(), true
		}
		return nil, true
	case tea.KeyEnter:
		if d.attempted {
			return nil, true
		}
		d.attempted, d.busy = true, true
		d.note = tr("Создание ветки в dsh…")
		login, ctx := m.settingsLogin, m.settingsContext
		if ctx == nil {
			ctx = context.Background()
		}
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			result := sessionForkResult{dialog: d}
			api, err := openAPI(ctx, login)
			if err == nil {
				defer api.close()
				var created struct {
					ID string `json:"sessionId"`
				}
				request := map[string]any{"sessionId": d.id, "allowMigration": false}
				if d.atSeq != nil {
					request["atSeq"] = *d.atSeq
				}
				err = api.rpc(ctx, "session/fork", map[string]any{"request": request}, &created)
				if err == nil && (strings.TrimSpace(created.ID) == "" || created.ID == d.id) {
					err = errors.New(tr("dsh не подтвердил новую сессию"))
				}
				result.childID = created.ID
			}
			result.err = err
			return result
		}, true
	}
	return nil, true
}

func (m *model) handleSessionFork(result sessionForkResult) tea.Cmd {
	d := m.forkDialog
	if d == nil || d != result.dialog {
		return nil
	}
	d.busy = false
	if result.err != nil {
		d.note = tr("Создание не подтверждено. Закройте окно и обновите список перед повтором. ") + clean(result.err.Error())
		return nil
	}
	m.forkDialog = nil
	m.closeHistory()
	focus := m.showChat()
	// Keep the acknowledged identity visible if connecting fails; never create again.
	m.append(tr("Ветка создана: ") + clean(result.childID) + tr(". Для подключения: /session ") + clean(result.childID))
	return tea.Batch(focus, m.sessionOperation("connect", result.childID))
}

func (m *model) forkView(base string) string {
	d := m.forkDialog
	if d == nil {
		return base
	}
	width, height := min(76, m.width-3), min(16, m.height-2)
	inner := width - 4
	explanation := tr("История — до последнего завершённого хода. Незавершённый ответ не войдёт. Без завершённых ходов ветка недоступна. Источник продолжит работу. Откроется новая ветка.")
	if d.atSeq != nil {
		explanation = fmt.Sprintf(tr("История — по событие #%d включительно. Более поздние сообщения не войдут. Незавершённый ход закроет dsh. Источник продолжит работу."), *d.atSeq)
	}
	lines := []string{heading().Render(tr("Ветвление сессии")), ansi.Truncate(clean(d.id), inner, "…")}
	if d.preview != "" {
		lines = append(lines, ansi.Truncate(d.preview, inner, "…"))
	}
	lines = append(lines, "", ansi.Wrap(explanation, inner, ""), "")
	if d.note != "" {
		note := strings.Split(ansi.Wrap(d.note, inner, ""), "\n")
		if len(note) > 3 {
			note = note[:3]
		}
		lines = append(lines, strings.Join(note, "\n"))
	}
	hint := tr("Enter — создать · Esc — отменить")
	if d.busy {
		hint = tr("Ожидание ответа dsh…")
	} else if d.attempted {
		hint = tr("Esc — закрыть и проверить список")
	}
	lines = append(lines, hint)
	return overlay(base, panel(width, height, strings.Join(lines, "\n"), "#58a6ff"), (m.width-1-width)/2, (m.height-height)/2, m.width-1)
}
