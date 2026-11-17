package main

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme")

func (m *model) goalKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	d := m.goalDialog
	if d == nil {
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
			return m.closeGoal(), true
		}
		return nil, true
	}
	if !msg.Paste {
		switch msg.Type {
		case tea.KeyEsc:
			if d.draft != nil {
				d.draft = nil
				d.editing = false
				d.conflict = false
				d.note = tr("Черновик отменён. Цель в dsh не изменена.")
				return nil, true
			}
			return m.closeGoal(), true
		case tea.KeyCtrlR:
			return m.loadGoal(), true
		case tea.KeyF3:
			if d.editing {
				d.editing = false
				d.note = tr("Снимок цели. F3 — вернуться к черновику.")
				return nil, true
			}
			return m.editGoal(), true
		case tea.KeyF5:
			if d.editing {
				return m.saveGoal(), true
			}
			return m.loadGoal(), true
		case tea.KeyCtrlB:
			if !d.editing && d.conflict && !d.stale && d.current != nil && d.draft != nil {
				if d.current.ID != d.draft.base.ID {
					d.note = tr("Это другая цель. Esc — отменить старый черновик; F3 — новая правка.")
					return nil, true
				}
				// Fields untouched in the draft follow the fresh server snapshot.
				if strings.TrimSpace(d.draft.objective.Value()) == d.draft.base.Objective {
					d.draft.objective.SetValue(d.current.Objective)
					if d.draft.objective.Value() != d.current.Objective {
						d.note = tr("Новое описание нельзя открыть без преобразования. Esc — отменить.")
						return nil, true
					}
				}
				if cap, err := strconv.ParseInt(d.draft.cap.Value(), 10, 64); err == nil && cap == d.draft.base.MaxRounds {
					d.draft.cap.SetValue(strconv.FormatInt(d.current.MaxRounds, 10))
				}
				d.draft.base = *d.current
				d.conflict = false
				d.editing = true
				d.note = tr("Свежая ревизия принята для черновика. Проверьте поля; F5 — сохранить.")
				return d.focusField(), true
			}
			return nil, true
		}
	}
	if d.editing && d.draft != nil {
		if !msg.Paste && (msg.Type == tea.KeyTab || msg.Type == tea.KeyShiftTab) {
			d.draft.field = 1 - d.draft.field
			return d.focusField(), true
		}
		var cmd tea.Cmd
		if d.draft.field == 0 {
			if msg.Type == tea.KeyRunes {
				text := string(msg.Runes)
				if len(d.draft.objective.Value())+len(text) > goalObjectiveLimit || clean(text) != text || strings.ContainsAny(text, "\r\t") {
					d.note = tr("Ввод не принят: лимит 64 КиБ, табуляция и управляющие символы недоступны.")
					return nil, true
				}
			}
			d.draft.objective, cmd = d.draft.objective.Update(msg)
		} else {
			if msg.Type == tea.KeyRunes && (len(d.draft.cap.Value())+len(string(msg.Runes)) > 16 || strings.ContainsAny(string(msg.Runes), "\r\n\t")) {
				d.note = tr("Лимит раундов принимает до 16 цифр в одной строке.")
				return nil, true
			}
			d.draft.cap, cmd = d.draft.cap.Update(msg)
		}
		return cmd, true
	}
	var cmd tea.Cmd
	d.viewport, cmd = d.viewport.Update(msg)
	return cmd, true
}
func (m *model) goalBox() area {
	w, h := min(98, m.width-3), min(32, m.height-2)
	return area{(m.width - 1 - w) / 2, (m.height - h) / 2, w, h}
}
func (m *model) goalView(base string) string {
	d := m.goalDialog
	if d == nil {
		return base
	}
	box := m.goalBox()
	width := box.width - 4
	bodyHeight := max(1, box.height-10)
	trim := func(s string) string { return ansi.Truncate(clean(s), width, "…") }
	title := tr("Цель сессии")
	if d.editing {
		title = tr("Цель · редактирование")
	}
	lines := []string{heading().Render(title), trim(d.id), ""}
	if d.editing && d.draft != nil {
		d.draft.objective.SetWidth(width)
		d.draft.objective.SetHeight(max(1, bodyHeight-2))
		d.draft.cap.Width = max(1, width-17)
		lines = append(lines, tr("Описание:"))
		lines = append(lines, strings.Split(d.draft.objective.View(), "\n")...)
		lines = append(lines, d.draft.cap.View())
	} else {
		body := tr("Состояние ещё не прочитано.")
		if d.ready && d.current == nil {
			body = tr("В этой сессии нет цели.\n\n/goal <описание> — создать цель.\nСоздание включает режим цели и может запустить работу.")
		}
		if goal := d.current; goal != nil {
			phase := map[string]string{"active": tr("активна"), "paused": tr("приостановлена"), "blocked": tr("заблокирована"), "complete": tr("завершена")}[goal.Phase]
			activation := tr("выключено")
			if goal.Activation == "armed" {
				activation = tr("включено")
			}
			body = fmt.Sprintf(tr("Фаза: %s · ревизия %d\nРаунды: %d / %d\nАвтопродолжение: %s\n"), phase, goal.Revision, goal.Rounds, goal.MaxRounds, activation)
			if goal.BlockedReason != nil {
				body += tr("Блокировка: ") + clean(goal.BlockedReason.Message) + "\n"
			}
			body += tr("\nОписание:\n") + clean(goal.Objective)
		}
		if d.stale {
			body = tr("Снимок не подтверждён · Ctrl+R — перечитать\n\n") + body
		}
		d.viewport.Width = width
		d.viewport.Height = bodyHeight
		d.viewport.SetContent(ansi.Hardwrap(body, width, true))
		lines = append(lines, strings.Split(d.viewport.View(), "\n")...)
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
	hint, hint2 := tr("F3 правка · F5 обновить · ↑/↓ прокрутка"), tr("Ctrl+R перечитать · Esc закрыть")
	if d.draft != nil {
		hint2 = tr("F3 черновик · Ctrl+R перечитать · Esc отменить черновик")
	}
	if d.editing {
		hint = tr("Tab поле · Enter новая строка · F5 сохранить")
		hint2 = tr("F3 снимок · Ctrl+R перечитать · Esc отменить")
	}
	if d.conflict && !d.editing {
		hint = tr("Ctrl+B принять ревизию · F3 черновик · Esc отменить")
	}
	lines = append(lines, trim(hint), trim(hint2))
	return overlay(base, panel(box.width, box.height, strings.Join(lines, "\n"), theme.Accent), box.x, box.y, m.width-1)
}
func (m *model) goalMouse(msg tea.MouseMsg) tea.Cmd {
	d := m.goalDialog
	if d == nil || d.busy || d.editing || !m.goalBox().contains(msg.X, msg.Y) {
		return nil
	}
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		var cmd tea.Cmd
		d.viewport, cmd = d.viewport.Update(msg)
		return cmd
	}
	return nil
}
