package main

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

const goalObjectiveLimit = 64 << 10
const maxGoalRounds int64 = 9007199254740991

type liveGoal struct {
	ID            string `json:"id"`
	Revision      int    `json:"revision"`
	Phase         string `json:"phase"`
	Objective     string `json:"objective"`
	MaxRounds     int64  `json:"maxGoalRounds"`
	Rounds        int64  `json:"roundsStarted"`
	Activation    string `json:"activation"`
	BlockedReason *struct {
		Message string `json:"message"`
	} `json:"blockedReason"`
}
type goalDraft struct {
	base      liveGoal
	objective textarea.Model
	cap       textinput.Model
	field     int
}
type goalDialog struct {
	ctx                                                              context.Context
	cancel                                                           context.CancelFunc
	id                                                               string
	epoch, serial                                                    int
	focused, busy, writing, editing, blocked, conflict, stale, ready bool
	current                                                          *liveGoal
	draft                                                            *goalDraft
	viewport                                                         viewport.Model
	note                                                             string
}
type goalDialogResult struct {
	dialog *goalDialog
	serial int
	goal   *liveGoal
	err    error
	saved  bool
}

func readLiveGoal(ctx context.Context, api *dshAPI, id string) (*liveGoal, error) {
	var goal *liveGoal
	if err := api.rpc(ctx, "goals/get", map[string]any{"agentId": id}, &goal); err != nil {
		return nil, errors.New("Состояние цели недоступно. F5 — обновить.")
	}
	if goal != nil {
		validPhase := goal.Phase == "active" || goal.Phase == "paused" || goal.Phase == "blocked" || goal.Phase == "complete"
		if goal.ID == "" || goal.Revision < 1 || !validPhase || goal.MaxRounds < 1 || goal.MaxRounds > maxGoalRounds || goal.Rounds < 0 || len(goal.Objective) > goalObjectiveLimit || goal.Activation != "armed" && goal.Activation != "disarmed" {
			return nil, errors.New("Состояние цели не поддержано или слишком велико.")
		}
		if goal.BlockedReason != nil && len(goal.BlockedReason.Message) > goalObjectiveLimit {
			return nil, errors.New("Описание блокировки слишком велико.")
		}
	}
	return goal, nil
}
func (m *model) openGoal() tea.Cmd {
	if m.browserPreview || m.session == nil || m.chat == nil || !m.chat.ready || m.chatSessionChanging() {
		m.append("Для просмотра цели подключитесь к сессии dsh.")
		return nil
	}
	ctx, cancel := context.WithCancel(m.chat.ctx)
	d := &goalDialog{ctx: ctx, cancel: cancel, id: m.session.ID, epoch: m.chat.epoch, focused: m.input.Focused(), viewport: viewport.New(1, 1)}
	m.goalDialog = d
	m.input.Blur()
	return m.loadGoal()
}
func (m *model) closeGoal() tea.Cmd {
	d := m.goalDialog
	if d == nil {
		return nil
	}
	d.cancel()
	d.draft = nil
	m.goalDialog = nil
	if d.focused && m.page == chatPage && !m.navFocused {
		return m.input.Focus()
	}
	return nil
}
func (m *model) loadGoal() tea.Cmd {
	d := m.goalDialog
	if d == nil || d.busy {
		return nil
	}
	d.busy = true
	d.serial++
	d.note = "Чтение цели dsh…"
	serial, login := d.serial, m.settingsLogin
	return func() tea.Msg {
		result := goalDialogResult{dialog: d, serial: serial}
		ctx, cancel := context.WithTimeout(d.ctx, 20*time.Second)
		defer cancel()
		api, err := openAPI(ctx, login)
		if err == nil {
			defer api.close()
			result.goal, result.err = readLiveGoal(ctx, api, d.id)
		} else {
			result.err = errors.New("Состояние цели недоступно. Ctrl+R — перечитать.")
		}
		return result
	}
}
func (m *model) editGoal() tea.Cmd {
	d := m.goalDialog
	if d == nil || d.busy || !d.ready || d.stale || d.current == nil || m.isSessionArchived(d.id) {
		return nil
	}
	if d.draft != nil {
		d.editing = true
		return d.focusField()
	}
	editor := textarea.New()
	editor.CharLimit = goalObjectiveLimit
	editor.ShowLineNumbers = false
	editor.Prompt = "> "
	editor.SetValue(d.current.Objective)
	if editor.Value() != d.current.Objective {
		d.note = "Описание нельзя открыть без преобразования."
		return nil
	}
	cap := textinput.New()
	cap.Prompt = "Лимит раундов: "
	cap.CharLimit = 16
	cap.SetValue(strconv.FormatInt(d.current.MaxRounds, 10))
	d.draft = &goalDraft{base: *d.current, objective: editor, cap: cap}
	d.editing = true
	d.note = "F5 — сохранить описание и лимит; фаза цели сохраняется."
	if d.current.Activation == "armed" {
		d.note += " Автопродолжение уже включено."
	}
	return d.focusField()
}
func (d *goalDialog) focusField() tea.Cmd {
	d.draft.objective.Blur()
	d.draft.cap.Blur()
	if d.draft.field == 0 {
		return d.draft.objective.Focus()
	}
	return d.draft.cap.Focus()
}
func (m *model) saveGoal() tea.Cmd {
	d := m.goalDialog
	if d == nil || d.busy || d.blocked || d.conflict || d.stale || !d.editing || d.draft == nil || m.chatSessionChanging() || m.isSessionArchived(d.id) {
		return nil
	}
	objective := strings.TrimSpace(d.draft.objective.Value())
	rounds, err := strconv.ParseInt(d.draft.cap.Value(), 10, 64)
	if objective == "" || len(objective) > goalObjectiveLimit || clean(objective) != objective {
		d.note = "Введите непустое описание до 64 КиБ без управляющих символов."
		return nil
	}
	if err != nil || rounds < 1 || rounds > maxGoalRounds {
		d.note = "Лимит — целое число от 1 до 9007199254740991."
		return nil
	}
	base := d.draft.base
	request := map[string]any{}
	if objective != base.Objective {
		request["objective"] = objective
	}
	if rounds != base.MaxRounds {
		request["maxGoalRounds"] = rounds
	}
	if len(request) == 0 {
		d.editing = false
		d.draft = nil
		d.note = "Изменений нет; запись не нужна."
		return nil
	}
	d.busy, d.writing = true, true
	d.serial++
	d.note = "Сохранение цели…"
	serial, login := d.serial, m.settingsLogin
	return func() tea.Msg {
		result := goalDialogResult{dialog: d, serial: serial}
		ctx, cancel := context.WithTimeout(d.ctx, 20*time.Second)
		defer cancel()
		api, err := openAPI(ctx, login)
		if err == nil {
			defer api.close()
			err = api.rpc(ctx, "goals/edit", map[string]any{"agentId": d.id, "ref": map[string]any{"id": base.ID, "revision": base.Revision}, "request": request}, &result.goal)
			if err == nil && (result.goal == nil || result.goal.ID != base.ID || result.goal.Revision <= base.Revision || result.goal.Objective != objective || result.goal.MaxRounds != rounds) {
				err = errors.New("unconfirmed goal edit")
			}
		}
		if err != nil {
			result.err = errors.New("Запись цели не подтверждена; черновик сохранён. Ctrl+R — перечитать, повтора нет.")
		} else {
			result.saved = true
		}
		return result
	}
}
func (m *model) handleGoalDialog(result goalDialogResult) {
	d := m.goalDialog
	if d == nil || d != result.dialog || d.ctx.Err() != nil || d.serial != result.serial || m.chat == nil || d.epoch != m.chat.epoch || m.session == nil || d.id != m.session.ID {
		return
	}
	wasWriting := d.writing
	d.busy, d.writing = false, false
	if result.err != nil {
		d.note = result.err.Error()
		d.stale = true
		if wasWriting {
			d.blocked = true
		}
		return
	}
	d.current = result.goal
	d.ready = true
	d.stale = false
	d.blocked = false
	d.conflict = false
	if result.saved {
		d.editing = false
		d.draft = nil
		d.note = "Изменения цели подтверждены dsh."
	} else {
		d.note = "Снимок цели обновлён. F3 — редактировать · F5 — обновить."
		if d.draft != nil && (d.current == nil || d.current.ID != d.draft.base.ID || d.current.Revision != d.draft.base.Revision) {
			d.conflict = true
			d.editing = false
			d.note = "Цель изменилась. Свежие данные показаны ниже. Ctrl+B — продолжить черновик с этой ревизией; Esc — отменить."
		}
	}
	d.viewport.GotoTop()
}
