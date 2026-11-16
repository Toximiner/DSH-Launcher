package main

import (
	"encoding/json"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

func (d *settingsForm) editMultiline(row formRow) tea.Cmd {
	if !row.known || len(row.path) == 0 || d.secretUnder(row.path) {
		d.note = "Редактор доступен для полей схемы без секретов."
		return nil
	}
	text := ""
	if row.node.Type == "string" {
		text, _ = row.value.(string)
	} else {
		value := row.value
		if value == nil {
			value = formDefault(row.node)
		}
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil
		}
		text = string(raw)
	}
	if len(text) > settingsDraftLimit {
		d.note = "Значение превышает 64 КиБ."
		return nil
	}
	editor := textarea.New()
	editor.CharLimit = settingsDraftLimit
	editor.ShowLineNumbers = false
	editor.Prompt = "> "
	editor.SetValue(text)
	if editor.Value() != text {
		d.note = "Значение нельзя открыть без преобразования."
		return nil
	}
	d.field = &settingsField{row: row, multiline: &editor}
	d.note = "F5 — принять в черновик формы; сохранение в dsh — следующим F5."
	return editor.Focus()
}
