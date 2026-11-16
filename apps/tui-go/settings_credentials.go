package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) openCredential() tea.Cmd {
	d := m.settingsForm
	if d == nil || d.busy || d.blocked {
		return nil
	}
	row, ok := d.selection()
	if !ok || nodeRole(row.node) != "credential-ref" {
		d.note = "Ctrl+K доступен у поля ссылки на ключ (например apiKeyEnv)."
		return nil
	}
	if len(d.ops) > 0 {
		d.note = "Сначала сохраните изменения настроек через F5."
		return nil
	}
	ref, _ := row.value.(string)
	if !credentialRefPattern.MatchString(ref) {
		d.note = "Сначала задайте и сохраните имя ключа в этом поле."
		return nil
	}
	editor := textinput.New()
	editor.Prompt = "Новый ключ: "
	editor.CharLimit = 16384
	editor.EchoMode = textinput.EchoPassword
	editor.EchoCharacter = '•'
	field := &settingsField{row: row, editor: editor, credential: true, ref: ref}
	d.field = field
	_ = field.editor.Focus()
	d.busy = true
	d.serial++
	serial, login := d.serial, m.settingsLogin
	return func() tea.Msg {
		result := settingsFormResult{dialog: d, serial: serial}
		ctx, cancel := context.WithTimeout(d.ctx, 20*time.Second)
		defer cancel()
		api, err := openAPI(ctx, login)
		if err == nil {
			defer api.close()
			var views map[string]credentialInfo
			err = api.rpc(ctx, "credentials/describe", map[string]any{"refs": []string{ref}}, &views)
			if info, ok := views[ref]; err == nil && ok {
				result.credential = &info
			} else if err == nil {
				err = errors.New("missing credential status")
			}
		}
		if err != nil {
			result.err = errors.New("Статус ключа недоступен. Закройте поле и повторите чтение.")
		}
		return result
	}
}
func (m *model) writeCredential() tea.Cmd {
	d := m.settingsForm
	field := d.field
	if d.busy || d.blocked || d.ctx.Err() != nil {
		return nil
	}
	if !field.info.Writable {
		d.note = "Хранилище не разрешает запись этого ключа."
		return nil
	}
	value := field.editor.Value()
	if value == "" || len(value) > 16384 || clean(value) != value || strings.ContainsAny(value, "\n\r\t") {
		d.note = "Введите непустой однострочный ключ до 16 КиБ."
		return nil
	}
	field.editor.SetValue("")
	d.busy, d.writing = true, true
	d.serial++
	d.note = "Запись нового ключа…"
	serial, login, ref := d.serial, m.settingsLogin, field.ref
	return func() tea.Msg {
		result := settingsFormResult{dialog: d, serial: serial}
		ctx, cancel := context.WithTimeout(d.ctx, 20*time.Second)
		defer cancel()
		api, err := openAPI(ctx, login)
		if err == nil {
			defer api.close()
			err = api.rpc(ctx, "credentials/set", map[string]any{"ref": ref, "value": value}, nil)
		}
		value = ""
		if err != nil {
			result.err = errors.New("Запись ключа не подтверждена. Ввод очищен; Ctrl+R — перечитать, повтора нет.")
		} else {
			result.credentialSaved = true
		}
		return result
	}
}
