package main

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme")

func (m *model) settingsFormKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	d := m.settingsForm
	if d == nil {
		if m.page == dshSettingsPage && !m.navFocused && m.historyReader == nil && m.chrome.dialog == "" && !m.chrome.menuOpen && !msg.Paste {
			if msg.Type == tea.KeyEnter {
				return m.openSettingsForm(false), true
			}
			if msg.Type == tea.KeyF6 {
				return m.openSettingsForm(true), true
			}
		}
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
	if d.busy {
		if msg.Type == tea.KeyEsc && !d.writing {
			if len(d.ops) == 0 {
				return m.closeSettingsForm(), true
			}
			d.serial++
			d.busy = false
			d.discard = true
			d.note = tr("Отбросить изменения? Enter — отбросить · Esc — продолжить.")
		}
		return nil, true
	}
	if d.discard {
		if msg.Type == tea.KeyEnter {
			return m.closeSettingsForm(), true
		}
		if msg.Type == tea.KeyEsc {
			d.discard = false
		}
		return nil, true
	}
	if d.field != nil {
		field := d.field
		if field.multiline != nil {
			if !msg.Paste {
				switch msg.Type {
				case tea.KeyEsc:
					field.multiline.SetValue("")
					d.field = nil
					return nil, true
				case tea.KeyF5:
					return m.acceptSettingsField(), true
				}
			}
			var cmd tea.Cmd
			*field.multiline, cmd = field.multiline.Update(msg)
			return cmd, true
		}
		if msg.Paste && msg.Type == tea.KeyRunes && strings.ContainsAny(string(msg.Runes), "\n\r") {
			d.note = tr("Поле принимает только одну строку. Вставка не изменена и не принята.")
			return nil, true
		}
		switch msg.Type {
		case tea.KeyCtrlR:
			if d.blocked && field.credential {
				d.field = nil
				return m.refreshSettingsForm(), true
			}
			return nil, true
		case tea.KeyEsc:
			field.editor.SetValue("")
			d.field = nil
			return nil, true
		case tea.KeyEnter:
			return m.acceptSettingsField(), true
		case tea.KeyUp, tea.KeyDown:
			if choices := d.schema.enums(field.row.node); len(choices) > 0 {
				index := -1
				for i, choice := range choices {
					raw, _ := json.Marshal(choice)
					if string(raw) == field.editor.Value() {
						index = i
						break
					}
				}
				delta := 1
				if msg.Type == tea.KeyUp {
					delta = -1
					if index < 0 {
						index = 0
					}
				}
				raw, _ := json.Marshal(choices[(index+delta+len(choices))%len(choices)])
				field.editor.SetValue(string(raw))
				return nil, true
			}
		}
		var cmd tea.Cmd
		field.editor, cmd = field.editor.Update(msg)
		return cmd, true
	}
	rows := d.rows()
	node, _ := d.schema.at(d.path)
	count := len(rows)
	if node.Type == "dict" || node.Type == "array" {
		count++
	}
	switch msg.Type {
	case tea.KeyUp, tea.KeyDown:
		delta := 1
		if msg.Type == tea.KeyUp {
			delta = -1
		}
		if count > 0 {
			d.index = (d.index + delta + count) % count
		}
		return nil, true
	case tea.KeyEnter:
		return m.formEnter(), true
	case tea.KeyCtrlE:
		if row, ok := d.selection(); ok && !d.blocked {
			return d.editMultiline(row), true
		}
		return nil, true
	case tea.KeyCtrlN:
		d.index = len(rows)
		return m.formEnter(), true
	case tea.KeyCtrlK:
		return m.openCredential(), true
	case tea.KeyCtrlX:
		if row, ok := d.selection(); ok {
			d.dropDraft(row.path)
		}
		return nil, true
	case tea.KeyCtrlU:
		if row, ok := d.selection(); ok && !d.blocked {
			d.stage(row.path, nil, true)
		}
		return nil, true
	case tea.KeyF5:
		return m.saveSettingsForm(), true
	case tea.KeyCtrlR:
		return m.refreshSettingsForm(), true
	case tea.KeyEsc, tea.KeyLeft:
		if len(d.path) > 0 {
			d.path = d.path[:len(d.path)-1]
			d.index = 0
			return nil, true
		}
		if len(d.ops) > 0 {
			d.discard = true
			d.note = tr("Отбросить изменения? Enter — отбросить · Esc — продолжить.")
			return nil, true
		}
		return m.closeSettingsForm(), true
	}
	return nil, true
}
func (m *model) settingsFormBox() area {
	width, height := min(98, m.width-3), min(32, m.height-2)
	return area{(m.width - 1 - width) / 2, (m.height - height) / 2, width, height}
}
func (m *model) settingsFormView(base string) string {
	d := m.settingsForm
	if d == nil {
		return base
	}
	box := m.settingsFormBox()
	width := box.width - 4
	trim := func(text string) string { return ansi.Truncate(clean(text), width, "…") }
	title := tr("Настройки · ") + d.namespace.NS
	if d.field != nil && d.field.credential {
		title = tr("API-ключ · ") + d.field.ref
	}
	lines := []string{heading().Render(trim(title)), trim("/" + strings.Join(d.path, "/")), fmt.Sprintf(tr("Ревизия %d · изменений %d · F5 сохранить"), d.namespace.Revision, len(d.ops)), ""}
	if d.field != nil && d.field.credential {
		lines[2] = tr("Enter — записать новый ключ · Esc — отменить")
	}
	available := box.height - 11
	if d.field != nil {
		field := d.field
		field.editor.Width = max(1, width-13)
		if field.credential {
			status := tr("не задан")
			if field.info.Configured {
				status = tr("задан")
			}
			lines = append(lines, tr("Состояние: ")+status, trim(tr("Источник: ")+field.info.Source), tr("Существующий ключ скрыт и не читается."), tr("Enter заменит ключ в общем хранилище dsh."), field.editor.View())
		} else if field.multiline != nil {
			field.multiline.SetWidth(width)
			field.multiline.SetHeight(max(1, box.height-13))
			lines = append(lines, trim(tr("Поле: ")+strings.Join(field.row.path, "/")), trim(tr("Тип: ")+field.row.node.Type+tr(" · многострочный редактор")))
			lines = append(lines, strings.Split(field.multiline.View(), "\n")...)
		} else {
			lines = append(lines, trim(tr("Поле: ")+strings.Join(field.row.path, "/")), trim(tr("Тип: ")+field.row.node.Type), field.editor.View())
			if field.add {
				lines[len(lines)-3] = tr("Новая запись")
			}
			if choices := d.schema.enums(field.row.node); len(choices) > 0 {
				lines = append(lines, tr("↑/↓ — выбрать значение схемы"))
			}
		}
	} else {
		rows := d.rows()
		node, _ := d.schema.at(d.path)
		count := len(rows)
		if node.Type == "dict" || node.Type == "array" {
			count++
		}
		start := max(0, min(d.index-available/2, count-available))
		for i := start; i < min(count, start+available); i++ {
			label := tr("＋ Добавить запись")
			if i < len(rows) {
				row := rows[i]
				label = row.key + " = " + formSummary(row)
				for key := range d.conflicts {
					var path []string
					_ = json.Unmarshal([]byte(key), &path)
					if pathUnder(path, row.path) || pathUnder(row.path, path) {
						label = "⚠ " + label
						break
					}
				}
				if !row.known {
					label += tr(" [только чтение]")
				}
			}
			prefix := "  "
			if i == d.index {
				prefix = "› "
			}
			label = trim(prefix + label)
			if i == d.index {
				label = heading().Render(label)
			}
			lines = append(lines, label)
		}
		if count == 0 {
			lines = append(lines, tr("Нет полей для изменения."))
		}
	}
	for len(lines) < box.height-7 {
		lines = append(lines, "")
	}
	note := d.note
	if d.busy {
		note = tr("Запрос dsh…")
	}
	notes := strings.Split(ansi.Hardwrap(clean(note), width, true), "\n")
	for len(notes) < 3 {
		notes = append(notes, "")
	}
	lines = append(lines, muted().Render(notes[0]), muted().Render(notes[1]), muted().Render(notes[2]))
	hint := tr("↑/↓ поле · Enter открыть · Ctrl+N добавить · Ctrl+E редактор")
	hint2 := tr("F5 сохранить · Ctrl+R перечитать · Ctrl+U сброс · Ctrl+X отмена правки")
	if d.field != nil {
		hint = tr("Enter — принять · Esc — отменить ввод")
		hint2 = tr("Ключи и секреты не попадают в историю ввода.")
		if d.field.multiline != nil {
			hint = tr("Enter — новая строка · F5 — принять · Esc — отменить")
			hint2 = tr("Принять в форму; затем F5 — сохранить в dsh.")
		}
	} else {
		hint2 += tr(" · Esc назад")
		if row, ok := d.selection(); ok && nodeRole(row.node) == "credential-ref" {
			hint = tr("Ctrl+K — задать API-ключ · Enter — имя ссылки")
		}
	}
	if d.discard {
		hint = tr("Enter — отбросить изменения")
		hint2 = tr("Esc — вернуться к форме")
	}
	lines = append(lines, trim(hint), trim(hint2))
	return overlay(base, panel(box.width, box.height, strings.Join(lines, "\n"), theme.Accent), box.x, box.y, m.width-1)
}
func (m *model) settingsFormMouse(msg tea.MouseMsg) tea.Cmd {
	d := m.settingsForm
	if d == nil || d.busy || d.field != nil || d.discard {
		return nil
	}
	box := m.settingsFormBox()
	available := box.height - 11
	rows := d.rows()
	node, _ := d.schema.at(d.path)
	count := len(rows)
	if node.Type == "dict" || node.Type == "array" {
		count++
	}
	if !box.contains(msg.X, msg.Y) {
		return nil
	}
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		delta := 1
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		d.index = max(0, min(max(0, count-1), d.index+delta))
	}
	if msg.Button == tea.MouseButtonLeft && (area{box.x + 2, box.y + 5, box.width - 4, available}).contains(msg.X, msg.Y) {
		start := max(0, min(d.index-available/2, count-available))
		index := start + msg.Y - (box.y + 5)
		if index < count {
			d.index = index
			return m.formEnter()
		}
	}
	return nil
}
