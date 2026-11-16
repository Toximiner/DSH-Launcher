package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type sessionRename struct {
	id            string
	editor        textinput.Model
	focused, busy bool
	note          string
}
type sessionRenameResult struct {
	dialog *sessionRename
	title  string
	err    error
}

func (m *model) openSessionRename(current bool) tea.Cmd {
	if m.browserPreview || m.settingsLogin == "" {
		m.append("Переименование доступно при подключении к dsh.")
		return nil
	}
	id, title := "", ""
	if current && m.session != nil {
		id, title = m.session.ID, m.session.Title
	}
	if !current && len(m.visibleSessions()) > 0 {
		item := m.visibleSessions()[m.sessionListIndex]
		id, title = item.ID, item.Projections.Values.Title
	}
	if id == "" {
		m.append("Выберите сессию для переименования.")
		return nil
	}
	editor := textinput.New()
	editor.Prompt = "Название: "
	editor.CharLimit = 4096
	editor.SetValue(title)
	editor.CursorEnd()
	m.renameDialog = &sessionRename{id: id, editor: editor, focused: m.input.Focused()}
	m.input.Blur()
	return m.renameDialog.editor.Focus()
}

func (m *model) renameKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	d := m.renameDialog
	if d == nil {
		if msg.Type == tea.KeyF3 && m.page == sessionsPage && !m.navFocused && m.historyReader == nil && !m.chrome.menuOpen && m.chrome.dialog == "" {
			return m.openSessionRename(false), true
		}
		return nil, false
	}
	if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyCtrlD || msg.Type == tea.KeyF7 || msg.Type == tea.KeyF8 {
		return nil, false
	}
	if d.busy {
		return nil, true
	}
	switch msg.Type {
	case tea.KeyEsc:
		m.renameDialog = nil
		if d.focused && m.page == chatPage && !m.navFocused {
			return m.input.Focus(), true
		}
		return nil, true
	case tea.KeyEnter:
		title := d.editor.Value()
		if strings.TrimSpace(title) == "" {
			d.note = "Введите непустое название."
			return nil, true
		}
		d.busy = true
		d.note = "Сохранение в dsh…"
		login, ctx := m.settingsLogin, m.settingsContext
		if ctx == nil {
			ctx = context.Background()
		}
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			result := sessionRenameResult{dialog: d}
			api, err := openAPI(ctx, login)
			if err == nil {
				defer api.close()
				var accepted struct {
					Title string `json:"title"`
					Seq   *int   `json:"seq"`
				}
				err = api.rpc(ctx, "session/rename", map[string]any{"request": map[string]any{"sessionId": d.id, "title": title}}, &accepted)
				if err == nil && (accepted.Seq == nil || *accepted.Seq < 0 || strings.TrimSpace(accepted.Title) == "") {
					err = errors.New("dsh не подтвердил название")
				}
				result.title = accepted.Title
			}
			result.err = err
			return result
		}, true
	}
	if msg.Type == tea.KeyRunes {
		msg.Paste = true
	}
	var cmd tea.Cmd
	d.editor, cmd = d.editor.Update(msg)
	return cmd, true
}

func (m *model) handleSessionRename(result sessionRenameResult) tea.Cmd {
	d := m.renameDialog
	if d == nil || d != result.dialog {
		return nil
	}
	d.busy = false
	if result.err != nil {
		d.note = "Не удалось подтвердить. Проверьте список перед повтором. " + clean(result.err.Error())
		return nil
	}
	m.titleRevision++
	if m.session != nil && m.session.ID == d.id {
		m.session.Title = result.title
	}
	for i := range m.sessionItems {
		if m.sessionItems[i].ID == d.id {
			m.sessionItems[i].Projections.Values.Title = result.title
		}
	}
	m.renameDialog = nil
	m.rebuild()
	if d.focused && m.page == chatPage && !m.navFocused {
		return m.input.Focus()
	}
	return nil
}

func (m *model) renameView(base string) string {
	d := m.renameDialog
	if d == nil {
		return base
	}
	width, height := min(76, m.width-3), 12
	inner := width - 4
	d.editor.Width = max(1, inner-10)
	note := strings.Split(ansi.Hardwrap(d.note, inner, true), "\n")
	if len(note) > 3 {
		note = note[:3]
	}
	hint := "Enter — сохранить · Esc — отменить"
	if d.busy {
		hint = "Ожидание ответа dsh…"
	}
	content := strings.Join([]string{heading.Render("Название сессии"), ansi.Truncate(clean(d.id), inner, "…"), "", d.editor.View(), "", strings.Join(note, "\n"), hint}, "\n")
	return overlay(base, panel(width, height, content, "#58a6ff"), (m.width-1-width)/2, (m.height-height)/2, m.width-1)
}
