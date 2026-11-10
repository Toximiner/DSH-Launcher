package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type planState struct {
	Active  bool `json:"active"`
	Pending bool `json:"pending"`
}
type goalState struct {
	Goal struct {
		ID            string `json:"id"`
		Revision      int    `json:"revision"`
		Phase         string `json:"phase"`
		Objective     string `json:"objective"`
		BlockedReason *struct {
			Message string `json:"message"`
		} `json:"blockedReason"`
	} `json:"goal"`
	Rounds int `json:"roundsStarted"`
}
type sessionView struct {
	ID            string
	Plan          *planState
	Goal          *goalState
	GoalSupported bool
	Running       bool
	PendingInput  []string
	StateCursor   int
}
type sessionItem struct {
	ID      string `json:"sessionId"`
	Running bool   `json:"running"`
}
type sessionAction struct{ action, value string }

type sessionResult struct {
	sequence int
	view     *sessionView
	items    []sessionItem
	list     bool
	err      error
}
type sessionTick struct{ sequence int }

func readSession(ctx context.Context, api *dshAPI, id string) (*sessionView, error) {
	var projection *struct {
		Kind   string                     `json:"kind"`
		Values map[string]json.RawMessage `json:"values"`
	}
	if err := api.rpc(ctx, "session/projections", map[string]any{"request": map[string]any{"sessionId": id}}, &projection); err != nil {
		return nil, err
	}
	if projection == nil {
		return nil, errors.New("сессия не найдена")
	}
	if projection.Kind != "sequenced" {
		return nil, errors.New("сессии нужна миграция; откройте её в dsh")
	}
	view := &sessionView{ID: id}
	if data, ok := projection.Values["plan"]; ok {
		if json.Unmarshal(data, &view.Plan) != nil {
			return nil, errors.New("неподдерживаемое состояние планирования")
		}
	}
	if data, ok := projection.Values["goal"]; ok {
		view.GoalSupported = true
		if json.Unmarshal(data, &view.Goal) != nil {
			return nil, errors.New("неподдерживаемое состояние цели")
		}
	}
	var state struct {
		Cursor       int      `json:"cursor"`
		Running      bool     `json:"running"`
		PendingInput []string `json:"pendingInput"`
	}
	if err := api.rpc(ctx, "session/state", map[string]any{"request": map[string]any{"sessionId": id}}, &state); err != nil {
		return nil, err
	}
	view.Running = state.Running
	view.PendingInput = state.PendingInput
	view.StateCursor = state.Cursor
	return view, nil
}

func executePlan(ctx context.Context, api *dshAPI, id string, active bool) error {
	line := "/plan"
	if !active {
		line = "/plan off"
	}
	var execution *struct {
		Result struct {
			Kind string `json:"kind"`
		} `json:"result"`
	}
	if err := api.rpc(ctx, "commands/execute", map[string]any{"agentId": id, "line": line, "submittedAttachments": []any{}}, &execution); err != nil {
		return err
	}
	if execution == nil || execution.Result.Kind != "success" {
		return errors.New("dsh не подтвердил команду планирования")
	}
	return nil
}

func changeSessionMode(ctx context.Context, api *dshAPI, id, action, objective string) (*sessionView, error) {
	current, err := readSession(ctx, api, id)
	if err != nil {
		return nil, err
	}
	goalActive := current.Goal != nil && current.Goal.Goal.Phase == "active"
	goalOp := func(operation string) error {
		if current.Goal == nil {
			return errors.New("в этой сессии нет цели")
		}
		ref := map[string]any{"id": current.Goal.Goal.ID, "revision": current.Goal.Goal.Revision}
		return api.rpc(ctx, "goals/"+operation, map[string]any{"agentId": id, "ref": ref}, nil)
	}
	switch action {
	case "default", "plan":
		if action == "plan" && current.Plan == nil {
			return nil, errors.New("планирование недоступно в этой сессии")
		}
		if goalActive {
			if err = goalOp("pause"); err != nil {
				return nil, err
			}
		}
		if current.Plan != nil {
			if err = executePlan(ctx, api, id, action == "plan"); err != nil {
				return nil, err
			}
		}
	case "goal", "resume":
		if !current.GoalSupported {
			return nil, errors.New("режим цели недоступен в этой сессии")
		}
		if current.Plan != nil && (current.Plan.Active || current.Plan.Pending) {
			if err = executePlan(ctx, api, id, false); err != nil {
				return nil, err
			}
			next, err := readSession(ctx, api, id)
			if err != nil {
				return nil, err
			}
			if next.Plan != nil && (next.Plan.Active || next.Plan.Pending) {
				return next, errors.New("выход из планирования ожидает следующего шага; повторите запуск цели после переключения")
			}
		}
		if action == "resume" {
			err = goalOp("resume")
		} else {
			if strings.TrimSpace(objective) == "" {
				return nil, errors.New("укажите цель: /goal <описание>")
			}
			if current.Goal != nil && current.Goal.Goal.Phase != "complete" {
				return nil, errors.New("цель уже существует; используйте /goal resume, /goal pause или /goal clear")
			}
			err = api.rpc(ctx, "goals/create", map[string]any{"agentId": id, "request": map[string]any{"objective": objective}}, nil)
		}
		if err != nil {
			return nil, err
		}
	case "pause", "clear":
		if err = goalOp(action); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("неизвестный режим")
	}
	return readSession(ctx, api, id)
}

func (m *model) sessionOperation(action, value string) tea.Cmd {
	if m.browserPreview {
		m.append("Браузерное демо: подключение к сессии и смена режима доступны в Linux-клиенте.")
		return nil
	}
	if m.settingsLogin == "" {
		m.append("Укажите URL входа dsh через --url или DSH_TUI_URL.")
		return nil
	}
	if m.sessionBusy {
		if action != "refresh" {
			if len(m.sessionQueue) >= 8 {
				m.append("Очередь запросов dsh заполнена; повторите после завершения.")
			} else {
				m.sessionQueue = append(m.sessionQueue, sessionAction{action, value})
			}
		}
		return nil
	}
	if action != "new" && action != "connect" && action != "list" && m.session == nil {
		m.append("Сначала подключите сессию: /session <id> или /new.")
		return nil
	}
	m.sessionBusy = true
	m.sessionAction = action
	m.sessionError = ""
	m.sessionSequence++
	sequence := m.sessionSequence
	login, ctx := m.settingsLogin, m.settingsContext
	id := ""
	if m.session != nil {
		id = m.session.ID
	}
	if action == "connect" {
		id = strings.TrimSpace(value)
	}
	m.rebuild()
	return func() tea.Msg {
		result := sessionResult{sequence: sequence}
		api, err := openAPI(ctx, login)
		if err != nil {
			result.err = err
			return result
		}
		defer api.close()
		if action == "list" {
			result.list = true
			var list struct {
				Items []sessionItem `json:"items"`
			}
			result.err = api.rpc(ctx, "session/list", map[string]any{"_request": map[string]any{}}, &list)
			result.items = list.Items
			return result
		}
		if action == "new" {
			var created struct {
				ID string `json:"sessionId"`
			}
			if result.err = api.rpc(ctx, "session/create", map[string]any{"request": map[string]any{}}, &created); result.err != nil {
				return result
			}
			id = created.ID
			if id == "" {
				result.err = errors.New("dsh не вернул идентификатор сессии")
				return result
			}
		}
		if action == "connect" || action == "refresh" || action == "new" {
			result.view, result.err = readSession(ctx, api, id)
		} else {
			result.view, result.err = changeSessionMode(ctx, api, id, action, value)
			// A later step may fail after a successful pause; always reconcile actual state.
			if result.err != nil && result.view == nil {
				result.view, _ = readSession(ctx, api, id)
			}
		}
		return result
	}
}

func (m *model) sessionTimer() tea.Cmd {
	sequence := m.sessionSequence
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return sessionTick{sequence} })
}
func (m *model) modeLabel() string {
	if m.session == nil {
		return "не подключён"
	}
	if m.sessionStale {
		return "не подтверждён · ошибка связи"
	}
	if m.session.Plan != nil && m.session.Plan.Active {
		return "планирование"
	}
	if m.session.Goal != nil && m.session.Goal.Goal.Phase == "active" {
		return "цель"
	}
	return "обычный"
}
func (m *model) chooseMode(mode string) tea.Cmd {
	if mode == "goal" {
		if m.session == nil || m.browserPreview {
			return m.sessionOperation("goal", "")
		}
		value := m.input.Value()
		if !strings.HasPrefix(value, "/goal ") {
			m.input.SetValue("/goal " + value)
		}
		m.input.CursorEnd()
		return m.showChat()
	}
	return m.sessionOperation(mode, "")
}
func (m *model) sessionInfo() string {
	lines := []string{heading.Render("Сессии dsh"), "/sessions — обновить список · /new — новая сессия", "↑/↓ — выбор · Enter — подключить · r — обновить"}
	if m.session != nil {
		lines = append(lines, "", "Текущая: "+clean(m.session.ID), "Режим: "+m.modeLabel())
	}
	if m.sessionBusy {
		lines = append(lines, "", "Запрос dsh…")
	}
	if m.sessionError != "" {
		lines = append(lines, "", "Ошибка: "+m.sessionError)
	}
	for i, item := range m.sessionItems {
		state := ""
		if item.Running {
			state = " · выполняется"
		}
		prefix := "  "
		if i == m.sessionListIndex {
			prefix = "› "
		}
		lines = append(lines, prefix+clean(item.ID)+state)
	}
	if m.settingsLogin == "" {
		lines = append(lines, "", "Сессии dsh недоступны без подключения.")
	}
	return strings.Join(lines, "\n")
}
func (m *model) modeDescription() string {
	label := "Режим: " + m.modeLabel()
	if m.session != nil {
		if m.session.Running {
			label += "\nАгент: выполняется"
		} else {
			label += "\nАгент: не выполняется"
		}
		if len(m.session.PendingInput) > 0 {
			label += "\nОжидается подтверждение/ответ. F7 — разрешения · F8 — отмена."
			for _, kind := range m.session.PendingInput {
				if kind == "question" {
					label += "\nВопросы агента пока недоступны в TUI."
				}
			}
		}
	}
	if m.sessionBusy {
		label += " · запрос…"
	}
	if m.session != nil && m.session.Plan != nil && m.session.Plan.Pending {
		label += " · смена ожидает следующего шага"
	}
	if m.session != nil && m.session.Goal != nil {
		label += "\nЦель: " + clean(m.session.Goal.Goal.Objective) + " · " + m.session.Goal.Goal.Phase + fmt.Sprintf(" · раундов %d", m.session.Goal.Rounds)
		if m.session.Goal.Goal.BlockedReason != nil {
			label += "\nПричина: " + clean(m.session.Goal.Goal.BlockedReason.Message)
		}
	}
	if m.sessionError != "" {
		label += "\nОшибка: " + m.sessionError
	}
	return label
}
