package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type promptAttachment struct {
	Receipt string
	Name    string
	Bytes   int64
}
type composeDialog struct {
	id                     string
	epoch, serial          int
	editor                 textarea.Model
	attachments            []promptAttachment
	visible, busy, focused bool
	note                   string
}
type composeUpload struct {
	dialog     *composeDialog
	serial     int
	attachment *promptAttachment
	err        error
}

func (m *model) openCompose() tea.Cmd {
	if m.session != nil && m.session.Address.Kind == "subagent" {
		m.append(tr("Сообщения подагенту: /session ") + m.session.Address.ParentID + " → /agents → F3.")
		return nil
	}
	if m.chat == nil || m.session == nil || !m.chat.ready || m.chatSessionChanging() {
		m.append(tr("Для редактора подключите сессию dsh."))
		return nil
	}
	if m.composer != nil && m.composer.id == m.session.ID {
		m.composer.visible = true
		return m.composer.editor.Focus()
	}
	editor := textarea.New()
	editor.CharLimit = 65536
	editor.ShowLineNumbers = false
	editor.Prompt = "> "
	initial := m.input.Value()
	var attached []promptAttachment
	if p := m.prompts[m.session.ID]; p != nil && p.Status == "rejected" {
		initial = p.Text
		attached = append(attached, p.Attachments...)
	}
	editor.SetValue(initial)
	if editor.Value() != initial {
		m.append(tr("Черновик нельзя открыть без преобразования."))
		return nil
	}
	m.composer = &composeDialog{id: m.session.ID, epoch: m.chat.epoch, editor: editor, attachments: attached, visible: true, focused: m.input.Focused()}
	m.input.Blur()
	return m.composer.editor.Focus()
}
func (m *model) uploadCompose(name string) tea.Cmd {
	if m.composer == nil {
		m.openCompose()
	}
	d := m.composer
	if d == nil || d.busy || len(d.attachments) >= 4 || m.isSessionArchived(d.id) {
		return nil
	}
	d.visible = true
	d.busy = true
	d.serial++
	serial, login, ctx := d.serial, m.settingsLogin, m.chat.ctx
	d.note = tr("Загрузка файла в текущую сессию dsh…")
	return func() tea.Msg {
		result := composeUpload{dialog: d, serial: serial}
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		data, err := readComposeFile(name)
		if err == nil {
			api, e := openAPI(ctx, login)
			err = e
			if e == nil {
				defer api.close()
				var uploaded struct {
					Receipt string `json:"receiptId"`
					File    struct {
						Name  string `json:"name"`
						Bytes int64  `json:"bytes"`
					} `json:"file"`
				}
				err = api.rpc(ctx, "fileUploads/upload", map[string]any{"agentId": d.id, "request": map[string]any{"name": filepath.Base(name), "data": base64.StdEncoding.EncodeToString(data)}}, &uploaded)
				if err == nil && (uploaded.Receipt == "" || uploaded.File.Bytes != int64(len(data))) {
					err = errors.New("upload unconfirmed")
				}
				if err == nil {
					result.attachment = &promptAttachment{uploaded.Receipt, uploaded.File.Name, uploaded.File.Bytes}
				}
			}
		}
		if err != nil {
			result.err = errors.New(tr("Загрузка не подтверждена (обычный файл до 2 МиБ). Автоповтора нет."))
		}
		return result
	}
}
func (m *model) handleComposeUpload(result composeUpload) {
	d := m.composer
	if d == nil || d != result.dialog || d.serial != result.serial || m.chat == nil || m.chat.epoch != d.epoch || m.session == nil || m.session.ID != d.id {
		return
	}
	d.busy = false
	if result.err != nil {
		d.note = result.err.Error()
		return
	}
	if result.attachment != nil {
		d.attachments = append(d.attachments, *result.attachment)
		d.note = tr("Файл загружен и прикреплён к черновику. F5 — отправить сообщение.")
	}
}
func (m *model) composeKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	d := m.composer
	if d == nil || !d.visible {
		return nil, false
	}
	if !msg.Paste {
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlD, tea.KeyF7, tea.KeyF8:
			return nil, false
		}
	}
	if msg.Paste && msg.Type != tea.KeyRunes {
		return nil, true
	}
	if !msg.Paste && msg.Type == tea.KeyEsc {
		d.visible = false
		if d.focused {
			return m.input.Focus(), true
		}
		return nil, true
	}
	if d.busy {
		return nil, true
	}
	if !msg.Paste && msg.Type == tea.KeyCtrlX {
		if len(d.attachments) > 0 {
			d.attachments = d.attachments[:len(d.attachments)-1]
			d.note = tr("Последнее вложение убрано из черновика.")
		}
		return nil, true
	}
	if !msg.Paste && msg.Type == tea.KeyF5 {
		text := d.editor.Value()
		if strings.TrimSpace(text) == "" || len(text) > 65536 || clean(text) != text {
			d.note = tr("Введите сообщение до 64 КиБ без управляющих символов.")
			return nil, true
		}
		if m.chat == nil || !m.chat.ready || m.chatSessionChanging() || m.session == nil || m.session.ID != d.id || m.isSessionArchived(d.id) {
			d.note = tr("Сессия недоступна; черновик сохранён.")
			return nil, true
		}
		if p := m.prompts[d.id]; p != nil && p.Status != "rejected" {
			d.note = tr("Дождитесь подтверждения предыдущего сообщения; черновик сохранён.")
			return nil, true
		}
		cmd := m.submitContent(text, append([]promptAttachment(nil), d.attachments...))
		if cmd == nil {
			return nil, true
		}
		m.composer = nil
		m.input.SetValue("")
		return tea.Batch(cmd, m.input.Focus()), true
	}
	if msg.Type == tea.KeyRunes && (len(d.editor.Value())+len(string(msg.Runes)) > 65536 || strings.ContainsAny(string(msg.Runes), "\r\t") || clean(string(msg.Runes)) != string(msg.Runes)) {
		d.note = tr("Вставка не принята: лимит 64 КиБ, без табуляции/CR и управляющих символов.")
		return nil, true
	}
	var cmd tea.Cmd
	d.editor, cmd = d.editor.Update(msg)
	return cmd, true
}
func (m *model) composeView(base string) string {
	d := m.composer
	if d == nil || !d.visible {
		return base
	}
	box := m.goalBox()
	w := box.width - 4
	d.editor.SetWidth(w)
	d.editor.SetHeight(max(1, box.height-10))
	lines := []string{heading().Render(tr("Сообщение · многострочный редактор")), tr("Enter — новая строка · F5 — отправить/в очередь"), ""}
	lines = append(lines, strings.Split(d.editor.View(), "\n")...)
	names := []string{}
	for _, a := range d.attachments {
		names = append(names, fmt.Sprintf(tr("%s (%d Б)"), a.Name, a.Bytes))
	}
	note := d.note
	if d.busy {
		note = tr("Загрузка файла…")
	}
	lines = append(lines, shortText(tr("Вложения: ")+strings.Join(names, ", "), w), shortText(note, w), "", shortText(tr("Esc — скрыть с сохранением · /compose — вернуть"), w), shortText(tr("Ctrl+X — убрать последнее вложение · /attach <локальный путь>"), w))
	return overlay(base, panel(box.width, box.height, strings.Join(lines, "\n"), "#58a6ff"), box.x, box.y, m.width-1)
}

func shortText(s string, width int) string { return ansi.Truncate(clean(s), width, "…") }
