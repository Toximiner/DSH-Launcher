package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme")

type scheduleForm struct {
	title, minutes textinput.Model
	prompt         textarea.Model
	field          int
	create, every  bool
}

func (d *workbench) startScheduleForm(data map[string]any) tea.Cmd {
	f := &scheduleForm{title: textinput.New(), minutes: textinput.New(), prompt: textarea.New(), create: data == nil}
	f.title.CharLimit = 120
	f.title.Prompt = ""
	f.minutes.Prompt = ""
	f.minutes.CharLimit = 7
	f.minutes.SetValue("60")
	f.prompt.SetHeight(3)
	f.prompt.CharLimit = 60000
	f.prompt.ShowLineNumbers = false
	f.prompt.Prompt = ""
	if data != nil {
		f.title.SetValue(stringValue(data, "title"))
		f.prompt.SetValue(stringValue(data, "prompt"))
	}
	d.scheduleForm = f
	d.editor.Blur()
	return f.title.Focus()
}
func (m *model) scheduleFormKey(msg tea.KeyMsg) tea.Cmd {
	d := m.workbench
	f := d.scheduleForm
	if !msg.Paste {
		switch msg.Type {
		case tea.KeyEsc:
			d.scheduleForm = nil
			d.editing = false
			d.action = nil
			d.editor.SetValue("")
			return nil
		case tea.KeyF2:
			data := map[string]any{"title": f.title.Value(), "prompt": f.prompt.Value()}
			if f.create {
				minutes, err := strconv.Atoi(f.minutes.Value())
				if err != nil || minutes < 1 || minutes > 525600 {
					d.note = tr("Интервал: целое число минут от 1 до 525600.")
					return nil
				}
				key, kind := "after_seconds", "after"
				if f.every {
					key, kind = "every_seconds", "every"
				}
				data["timing"] = map[string]any{"kind": kind, key: minutes * 60}
			}
			d.editor.SetValue(prettyValue(data))
			d.scheduleForm = nil
			return d.editor.Focus()
		case tea.KeyTab, tea.KeyShiftTab:
			fields := 2
			if f.create {
				fields = 4
			}
			delta := 1
			if msg.Type == tea.KeyShiftTab {
				delta = -1
			}
			f.field = (f.field + delta + fields) % fields
			f.title.Blur()
			f.prompt.Blur()
			f.minutes.Blur()
			switch f.field {
			case 0:
				return f.title.Focus()
			case 1:
				return f.prompt.Focus()
			case 3:
				return f.minutes.Focus()
			}
			return nil
		case tea.KeyLeft, tea.KeyRight, tea.KeySpace:
			if f.field == 2 {
				f.every = !f.every
				return nil
			}
		case tea.KeyF5:
			title, prompt := strings.TrimSpace(f.title.Value()), strings.TrimSpace(f.prompt.Value())
			if title == "" || utf8.RuneCountInString(title) > 120 || prompt == "" {
				d.note = tr("Укажите название (1–120 символов) и инструкцию.")
				return nil
			}
			data := map[string]any{"title": title, "prompt": prompt}
			if f.create {
				minutes, err := strconv.Atoi(f.minutes.Value())
				if err != nil || minutes < 1 || minutes > 525600 {
					d.note = tr("Интервал: целое число минут от 1 до 525600.")
					return nil
				}
				key, kind := "after_seconds", "after"
				if f.every {
					key, kind = "every_seconds", "every"
				}
				data["timing"] = map[string]any{"kind": kind, key: minutes * 60}
				d.action.Args["request"] = data
			} else {
				objectValue(d.action.Args["request"])["patch"] = data
			}
			d.editor.SetValue(prettyValue(data))
			d.confirm = true
			d.note = d.action.Label
			return nil
		}
	}
	// Text widgets enforce field limits and never execute pasted control keys.
	var cmd tea.Cmd
	switch f.field {
	case 0:
		f.title, cmd = f.title.Update(msg)
	case 1:
		f.prompt, cmd = f.prompt.Update(msg)
	case 3:
		f.minutes, cmd = f.minutes.Update(msg)
	}
	return cmd
}
func (m *model) scheduleFormView(base string) string {
	d := m.workbench
	f := d.scheduleForm
	box := m.benchBox()
	w := box.width - 4
	f.title.Width = w
	f.minutes.Width = w
	f.prompt.SetWidth(w)
	title := tr("Новая задача")
	if !f.create {
		title = tr("Изменить задачу")
	}
	lines := []string{heading().Render(title), "", tr("Название"), f.title.View(), tr("Инструкция"), f.prompt.View()}
	if f.create {
		once, every := "["+tr("Однократно")+"]", "["+tr("Повторять")+"]"
		mode := accent().Render(once) + " " + muted().Render(every)
		if f.every {
			mode = muted().Render(once) + " " + accent().Render(every)
		}
		if f.field == 2 {
			mode = "› " + mode
		}
		lines = append(lines, tr("Повтор · ←/→"), mode, tr("Через / каждые, минут"), f.minutes.View())
	} else {
		lines = append(lines, muted().Render(tr("Правило времени сохранится. F2 — расширенный JSON с timing.")))
	}
	note := tr("Tab следующее поле · F5 — проверить · F2 JSON · Esc отмена")
	if d.confirm {
		note = tr("Enter — подтвердить · Esc — вернуться к правке")
		// Show exactly the request being confirmed rather than mutable live fields.
		var data map[string]any
		if json.Unmarshal([]byte(d.editor.Value()), &data) == nil {
			lines = append(lines, muted().Render(d.action.Label))
		}
	}
	lines = append(lines, "", muted().Render(d.note), accent().Render(note))
	text := strings.Join(lines, "\n")
	text = ansi.Hardwrap(text, w, true)
	return overlay(base, panel(box.width, box.height, text, theme.Accent), box.x, box.y, m.width-1)
}
