package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type planState struct {
	Active  bool `json:"active"`
	Pending bool `json:"pending"`
}
type goalState struct {
	Goal struct {
		Questions     []continuedQuestion
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

// Child logs require a durable parent-qualified address, including after reconnect.
type sessionReadAddress struct {
	Kind      string `json:"kind"`
	SessionID string `json:"sessionId,omitempty"`
	ParentID  string `json:"parentSessionId,omitempty"`
	ChildID   string `json:"childSessionId,omitempty"`
	Mode      string `json:"mode,omitempty"`
}

func (a sessionReadAddress) wire(id string) sessionReadAddress {
	if a.Kind == "subagent" && a.ChildID == id {
		return a
	}
	return sessionReadAddress{Kind: "session", SessionID: id}
}

type sessionView struct {
	Address sessionReadAddress

	Title          string
	Queue          []queuedMessage
	QueueKnown     bool
	QueueFetchedAt time.Time
	Questions      []continuedQuestion
	ID             string
	Plan           *planState
	Goal           *goalState
	GoalSupported  bool
	Running        bool
	PendingInput   []string
	StateCursor    int
}
type sessionItem struct {
	ID          string `json:"sessionId"`
	Running     bool   `json:"running"`
	Projections struct {
		Values struct {
			Title string `json:"title"`
		} `json:"values"`
	} `json:"projections"`
}
type sessionAction struct{ action, value string }

type sessionResult struct {
	titleRevision int
	sequence      int
	view          *sessionView
	items         []sessionItem
	list          bool
	err           error
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
		return nil, errors.New(tr("сессия не найдена"))
	}
	if projection.Kind != "sequenced" {
		return nil, errors.New(tr("сессии нужна миграция; откройте её в dsh"))
	}
	view := &sessionView{ID: id, Address: sessionReadAddress{Kind: "session", SessionID: id}}
	if raw := projection.Values["subagent"]; len(raw) > 0 && string(raw) != "null" {
		var identity struct {
			Mode string `json:"mode"`
		}
		if json.Unmarshal(raw, &identity) != nil {
			return nil, errors.New(tr("Не удалось прочитать адрес подагента."))
		}
		var list struct {
			Items []struct {
				ID       string `json:"sessionId"`
				ParentID string `json:"parentSessionId"`
				Origin   string `json:"origin"`
			} `json:"items"`
		}
		if err := api.rpc(ctx, "session/list", map[string]any{"_request": map[string]any{}}, &list); err != nil {
			return nil, err
		}
		found := false
		for _, item := range list.Items {
			if item.ID != id {
				continue
			}
			// An ordinary fork may inherit a descriptor; the durable origin wins.
			if item.Origin != "subagent" {
				found = true
				break
			}
			if item.ParentID != "" {
				view.Address = sessionReadAddress{Kind: "subagent", ParentID: item.ParentID, ChildID: id, Mode: identity.Mode}
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New(tr("Родитель подагента недоступен; откройте его из каталога родителя."))
		}
	}
	if data, ok := projection.Values["title"]; ok {
		_ = json.Unmarshal(data, &view.Title)
	}
	if data, ok := projection.Values["plan"]; ok {
		if json.Unmarshal(data, &view.Plan) != nil {
			return nil, errors.New(tr("неподдерживаемое состояние планирования"))
		}
	}
	if data, ok := projection.Values["goal"]; ok {
		view.GoalSupported = true
		if json.Unmarshal(data, &view.Goal) != nil {
			return nil, errors.New(tr("неподдерживаемое состояние цели"))
		}
	}
	if data, ok := projection.Values["userQuestions"]; ok {
		var questions struct {
			Active []continuedQuestion `json:"active"`
		}
		if json.Unmarshal(data, &questions) != nil {
			return nil, errors.New(tr("неподдерживаемые вопросы сессии"))
		}
		view.Questions = questions.Active
	}
	var state struct {
		Queue        *[]queuedMessage `json:"queue"`
		Cursor       int              `json:"cursor"`
		Running      bool             `json:"running"`
		PendingInput []string         `json:"pendingInput"`
	}
	view.QueueFetchedAt = time.Now()
	if err := api.rpc(ctx, "session/state", map[string]any{"request": map[string]any{"sessionId": id}}, &state); err != nil {
		return nil, err
	}
	view.QueueKnown = state.Queue != nil
	if state.Queue != nil {
		view.Queue = *state.Queue
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
		return errors.New(tr("dsh не подтвердил команду планирования"))
	}
	return nil
}

func changeSessionMode(ctx context.Context, api *dshAPI, id, action, objective string) (*sessionView, error) {
	current, err := readSession(ctx, api, id)
	if err != nil {
		return nil, err
	}
	if current.Address.Kind == "subagent" {
		return current, errors.New(tr("Режим подагента задаётся его родителем."))
	}
	goalActive := current.Goal != nil && current.Goal.Goal.Phase == "active"
	goalOp := func(operation string) error {
		if current.Goal == nil {
			return errors.New(tr("в этой сессии нет цели"))
		}
		ref := map[string]any{"id": current.Goal.Goal.ID, "revision": current.Goal.Goal.Revision}
		return api.rpc(ctx, "goals/"+operation, map[string]any{"agentId": id, "ref": ref}, nil)
	}
	switch action {
	case "default", "plan":
		if action == "plan" && current.Plan == nil {
			return nil, errors.New(tr("планирование недоступно в этой сессии"))
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
			return nil, errors.New(tr("режим цели недоступен в этой сессии"))
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
				return next, errors.New(tr("выход из планирования ожидает следующего шага; повторите запуск цели после переключения"))
			}
		}
		if action == "resume" {
			err = goalOp("resume")
		} else {
			if strings.TrimSpace(objective) == "" {
				return nil, errors.New(tr("укажите цель: /goal <описание>"))
			}
			if current.Goal != nil && current.Goal.Goal.Phase != "complete" {
				return nil, errors.New(tr("цель уже существует; используйте /goal resume, /goal pause или /goal clear"))
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
		return nil, errors.New(tr("неизвестный режим"))
	}
	return readSession(ctx, api, id)
}

func (m *model) sessionOperation(action, value string) tea.Cmd {
	if m.settingsLogin == "" {
		m.append(tr("Укажите URL входа dsh через --url или DSH_TUI_URL."))
		return nil
	}
	if m.sessionBusy {
		if action != "refresh" {
			if len(m.sessionQueue) >= 8 {
				m.append(tr("Очередь запросов dsh заполнена; повторите после завершения."))
			} else {
				m.sessionQueue = append(m.sessionQueue, sessionAction{action, value})
			}
		}
		return nil
	}
	if action != "new" && action != "connect" && action != "list" && m.session == nil {
		m.append(tr("Сначала подключите сессию: /session <id> или /new."))
		return nil
	}
	if m.session != nil && m.isSessionArchived(m.session.ID) && action != "refresh" && action != "connect" && action != "list" && action != "new" {
		m.append(tr("Сессия в архиве. /restore — восстановить."))
		return nil
	}
	m.sessionBusy = true
	m.sessionAction = action
	m.sessionError = ""
	m.sessionSequence++
	sequence := m.sessionSequence
	titleRevision := m.titleRevision
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
		result := sessionResult{sequence: sequence, titleRevision: titleRevision}
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
				result.err = errors.New(tr("dsh не вернул идентификатор сессии"))
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
		return tr("не подключён")
	}
	if m.sessionStale {
		return tr("не подтверждён · ошибка связи")
	}
	if m.session.Plan != nil && m.session.Plan.Active {
		return tr("планирование")
	}
	if m.session.Goal != nil && m.session.Goal.Goal.Phase == "active" {
		return tr("цель")
	}
	return tr("обычный")
}
func (m *model) chooseMode(mode string) tea.Cmd {
	if mode == "goal" {
		if m.session == nil {
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

// Background refreshes keep the current snapshot visible without loading indicators.
func (m *model) sessionRequestVisible() bool {
	return m.sessionBusy && m.sessionAction != "refresh"
}

func (m *model) sessionInfo() string {
	width := max(1, m.viewport.Width)
	m.sessionHits = nil
	lines := []string{heading().Render(tr("Сессии dsh")), tr("Enter вход · F3 имя · Ctrl+B ветка"), tr("Ctrl+X архив/возврат · a фильтр · r новые")}
	query := m.sessionQuery
	if m.sessionSearching {
		query = m.sessionSearch.View()
	}
	lines = append(lines, tr("Поиск / Ctrl+F: ")+query)
	if m.session != nil {
		lines = append(lines, ansi.Truncate(tr("Текущая: ")+clean(m.session.ID), width, "…"))
	}
	if m.sessionRequestVisible() {
		lines = append(lines, "", tr("Запрос dsh…"))
	}
	if m.sessionError != "" {
		lines = append(lines, "", tr("Ошибка: ")+m.sessionError)
	}
	filter := tr("Активные сессии · a — показать архив")
	if m.showArchived {
		filter = tr("Архив сессий · a — показать активные")
	}
	lines = append(lines, "", filter)
	if m.archive == nil || !m.archive.ready {
		lines = append(lines, tr("Статус архива не подтверждён · подключение…"))
	}
	for i, item := range m.visibleSessions() {
		state := ""
		if item.Running {
			state = tr(" · выполняется")
		}
		prefix := "  "
		if i == m.sessionListIndex {
			prefix = "› "
		}
		name := clean(item.Projections.Values.Title)
		if name == "" {
			name = clean(item.ID)
		} else {
			name += " · " + clean(item.ID)
		}
		row := strings.Count(ansi.Hardwrap(clean(strings.Join(lines, "\n")), width, true), "\n") + 1
		m.sessionHits = append(m.sessionHits, sessionHit{index: i, row: row})
		lines = append(lines, ansi.Truncate(prefix+name+state, width, "…"))
	}
	if m.settingsLogin == "" {
		lines = append(lines, "", tr("Сессии dsh недоступны без подключения."))
	}
	return strings.Join(lines, "\n")
}
func (m *model) modeDescription() string {
	label := tr("Режим: ") + m.modeLabel()
	if m.session != nil && m.isSessionArchived(m.session.ID) {
		label += tr("\nСессия в архиве. /restore — восстановить.")
	}
	if m.session != nil {
		if m.session.Running {
			label += tr("\nАгент: выполняется")
		} else {
			label += tr("\nАгент: не выполняется")
		}
		if len(m.session.PendingInput) > 0 {
			label += tr("\nОжидается подтверждение/ответ. F7 — вопросы/разрешения · F8 — отмена.")
			for _, kind := range m.session.PendingInput {
				if kind == "question" {
					label += tr("\nВопросы агента: F7 — открыть.")
				}
			}
		}
	}
	if m.sessionRequestVisible() {
		label += tr(" · запрос…")
	}
	if m.session != nil && m.session.Plan != nil && m.session.Plan.Pending {
		label += tr(" · смена ожидает следующего шага")
	}
	if m.session != nil && m.session.Goal != nil {
		label += tr("\nЦель: ") + clean(m.session.Goal.Goal.Objective) + " · " + m.session.Goal.Goal.Phase + fmt.Sprintf(tr(" · раундов %d"), m.session.Goal.Rounds)
		if m.session.Goal.Goal.BlockedReason != nil {
			label += tr("\nПричина: ") + clean(m.session.Goal.Goal.BlockedReason.Message)
		}
	}
	if m.sessionError != "" {
		label += tr("\nОшибка: ") + m.sessionError
	}
	return label
}
