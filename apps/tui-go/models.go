package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type modelSelection struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Reasoning string `json:"reasoningEffort,omitempty"`
}
type modelEffort struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type catalogModel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Reasoning *struct {
		Efforts []modelEffort `json:"efforts"`
		Default string        `json:"defaultEffort"`
	} `json:"reasoning"`
}
type modelCatalog struct {
	Default modelSelection `json:"default"`
	Groups  []struct {
		ID     string         `json:"id"`
		Name   string         `json:"name"`
		Models []catalogModel `json:"models"`
	} `json:"groups"`
	Failures []struct {
		ID string `json:"id"`
	} `json:"failures"`
}
type modelOption struct {
	provider, name string
	model          catalogModel
}
type modelPicker struct {
	ctx                               context.Context
	cancel                            context.CancelFunc
	id                                string
	epoch, serial, index              int
	focused, busy, writing, uncertain bool
	search                            textinput.Model
	catalog                           *modelCatalog
	current                           modelSelection
	effort, note                      string
}
type modelPickerResult struct {
	dialog   *modelPicker
	serial   int
	catalog  *modelCatalog
	current  modelSelection
	selected *modelSelection
	err      error
}

func readModelDirectory(ctx context.Context, api *dshAPI, id string) (*modelCatalog, modelSelection, error) {
	var catalog modelCatalog
	if err := api.rpc(ctx, "session/modelCatalog", map[string]any{}, &catalog); err != nil {
		return nil, modelSelection{}, errors.New(tr("Каталог моделей недоступен. F5 — повторить чтение."))
	}
	count := 0
	if len(catalog.Groups) > 256 {
		return nil, modelSelection{}, errors.New(tr("Каталог моделей слишком велик."))
	}
	for _, group := range catalog.Groups {
		if group.ID == "" || len(group.ID) > 1024 {
			return nil, modelSelection{}, errors.New(tr("Некорректный провайдер в каталоге."))
		}
		for _, model := range group.Models {
			count++
			if count > 2000 || model.ID == "" || len(model.ID) > 1024 || model.Reasoning != nil && len(model.Reasoning.Efforts) > 32 {
				return nil, modelSelection{}, errors.New(tr("Некорректный или слишком большой каталог моделей."))
			}
			if model.Reasoning != nil {
				for _, effort := range model.Reasoning.Efforts {
					if effort.ID == "" || len(effort.ID) > 256 {
						return nil, modelSelection{}, errors.New(tr("Некорректный параметр рассуждения."))
					}
				}
			}
		}
	}
	var projection *struct {
		Kind   string `json:"kind"`
		Values struct {
			Model *struct {
				Next *modelSelection `json:"next"`
			} `json:"modelSelection"`
		} `json:"values"`
	}
	if err := api.rpc(ctx, "session/projections", map[string]any{"request": map[string]any{"sessionId": id}}, &projection); err != nil || projection == nil || projection.Kind != "sequenced" {
		return nil, modelSelection{}, errors.New(tr("Не удалось прочитать модель сессии. F5 — повторить."))
	}
	current := catalog.Default
	if projection.Values.Model != nil && projection.Values.Model.Next != nil {
		current = *projection.Values.Model.Next
	}
	return &catalog, current, nil
}

func (m *model) openModels() tea.Cmd {
	if m.session == nil || m.chat == nil || !m.chat.ready || m.chatSessionChanging() {
		m.append(tr("Для выбора модели подключитесь к сессии dsh."))
		return nil
	}
	ctx, cancel := context.WithCancel(m.chat.ctx)
	editor := textinput.New()
	editor.Prompt = tr("Поиск: ")
	editor.CharLimit = 128
	editor.Placeholder = tr("Провайдер или модель…")
	d := &modelPicker{ctx: ctx, cancel: cancel, id: m.session.ID, epoch: m.chat.epoch, focused: m.input.Focused(), search: editor}
	m.modelPicker = d
	m.input.Blur()
	return tea.Batch(d.search.Focus(), m.loadModels())
}
func (m *model) closeModels() tea.Cmd {
	d := m.modelPicker
	if d == nil {
		return nil
	}
	d.cancel()
	m.modelPicker = nil
	if d.focused && m.page == chatPage && !m.navFocused {
		return m.input.Focus()
	}
	return nil
}
func (m *model) loadModels() tea.Cmd {
	d := m.modelPicker
	if d == nil || d.busy {
		return nil
	}
	d.busy = true
	d.serial++
	d.note = tr("Чтение каталога и модели сессии…")
	serial, login := d.serial, m.settingsLogin
	return func() tea.Msg {
		result := modelPickerResult{dialog: d, serial: serial}
		ctx, cancel := context.WithTimeout(d.ctx, 20*time.Second)
		defer cancel()
		api, err := openAPI(ctx, login)
		if err == nil {
			defer api.close()
			result.catalog, result.current, result.err = readModelDirectory(ctx, api, d.id)
		} else {
			result.err = errors.New(tr("Не удалось прочитать модели. F5 — повторить чтение."))
		}
		return result
	}
}
func (d *modelPicker) options() []modelOption {
	var result []modelOption
	if d.catalog == nil {
		return result
	}
	query := strings.ToLower(strings.TrimSpace(d.search.Value()))
	for _, group := range d.catalog.Groups {
		for _, model := range group.Models {
			if query == "" || strings.Contains(strings.ToLower(group.ID+" "+group.Name+" "+model.ID+" "+model.Name), query) {
				result = append(result, modelOption{group.ID, group.Name, model})
			}
		}
	}
	return result
}
func (d *modelPicker) choose(index int) {
	options := d.options()
	if len(options) == 0 {
		d.index = 0
		d.effort = ""
		return
	}
	d.index = max(0, min(index, len(options)-1))
	option := options[d.index]
	d.effort = ""
	if option.provider == d.current.Provider && option.model.ID == d.current.Model {
		d.effort = d.current.Reasoning
	}
	if !option.accepts(d.effort) {
		d.effort = ""
	}
}
func (option modelOption) accepts(effort string) bool {
	if effort == "" {
		return true
	}
	if option.model.Reasoning != nil {
		for _, value := range option.model.Reasoning.Efforts {
			if value.ID == effort {
				return true
			}
		}
	}
	return false
}
func (m *model) applyModel() tea.Cmd {
	d := m.modelPicker
	if d == nil || d.busy || d.uncertain || d.catalog == nil || d.ctx.Err() != nil {
		return nil
	}
	options := d.options()
	if d.index < 0 || d.index >= len(options) {
		return nil
	}
	option := options[d.index]
	selection := modelSelection{Provider: option.provider, Model: option.model.ID, Reasoning: d.effort}
	if selection == d.current {
		d.note = tr("Эта модель и параметр уже выбраны.")
		return nil
	}
	d.busy, d.writing = true, true
	d.serial++
	d.note = tr("Применение выбора в dsh…")
	serial, login, baseline := d.serial, m.settingsLogin, d.current
	return func() tea.Msg {
		result := modelPickerResult{dialog: d, serial: serial}
		ctx, cancel := context.WithTimeout(d.ctx, 20*time.Second)
		defer cancel()
		api, err := openAPI(ctx, login)
		if err == nil {
			defer api.close()
			_, current, readErr := readModelDirectory(ctx, api, d.id)
			if readErr != nil {
				err = readErr
			} else if current != baseline {
				err = errors.New(tr("Выбор изменён другим клиентом. F5 — перечитать перед применением."))
				result.err = err
			} else {
				var accepted struct {
					Selected *modelSelection `json:"selected"`
				}
				request := map[string]any{"sessionId": d.id, "provider": selection.Provider, "model": selection.Model}
				if selection.Reasoning != "" {
					request["reasoningEffort"] = selection.Reasoning
				}
				err = api.rpc(ctx, "session/selectModel", map[string]any{"request": request}, &accepted)
				if err == nil && (accepted.Selected == nil || *accepted.Selected != selection) {
					err = errors.New(tr("Ответ dsh не подтверждает выбранную модель."))
				}
				if err == nil {
					result.selected = accepted.Selected
				}
			}
		}
		if err != nil {
			if result.err == nil {
				result.err = errors.New(tr("Выбор не подтверждён. F5 — перечитать; автоматического повтора нет."))
			}
		}
		return result
	}
}
func (m *model) handleModelPicker(result modelPickerResult) {
	d := m.modelPicker
	if d == nil || d != result.dialog || d.ctx.Err() != nil || d.serial != result.serial || m.chat == nil || d.epoch != m.chat.epoch || m.session == nil || m.session.ID != d.id {
		return
	}
	wasWriting := d.writing
	d.busy, d.writing = false, false
	if result.err != nil {
		d.note = result.err.Error()
		if wasWriting {
			d.uncertain = true
		}
		return
	}
	d.uncertain = false
	if result.selected != nil {
		m.modelRevision++
		d.current = *result.selected
		if m.session != nil {
			selected := *result.selected
			m.session.Model = &selected
		}
		d.note = tr("Сессия: выбор подтверждён. Сохранение общего значения выполняет dsh.")
		return
	}
	d.catalog, d.current = result.catalog, result.current
	if m.agentInfo != nil {
		m.agentInfo.catalog, m.agentInfo.selection = result.catalog, result.current
	}
	d.choose(0)
	for i, option := range d.options() {
		if option.provider == d.current.Provider && option.model.ID == d.current.Model {
			d.choose(i)
			break
		}
	}
	d.note = tr("Enter применяет выбор. Esc закрывает окно без нового выбора.")
	if d.catalog != nil && len(d.catalog.Failures) > 0 {
		d.note = fmt.Sprintf(tr("Каталог: %d провайдеров недоступны. F5 — обновить."), len(d.catalog.Failures))
	}
}
func (m *model) modelsKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	d := m.modelPicker
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
	if msg.Type == tea.KeyEsc && !d.writing {
		return m.closeModels(), true
	}
	if d.busy {
		return nil, true
	}
	switch msg.Type {
	case tea.KeyF5:
		return m.loadModels(), true
	case tea.KeyEnter:
		return m.applyModel(), true
	case tea.KeyUp, tea.KeyDown:
		options := d.options()
		if len(options) > 0 {
			delta := 1
			if msg.Type == tea.KeyUp {
				delta = -1
			}
			d.choose((d.index + delta + len(options)) % len(options))
		}
		return nil, true
	case tea.KeyPgUp, tea.KeyPgDown:
		delta := max(1, m.modelsLayout().geometry.List.height-1)
		if msg.Type == tea.KeyPgUp {
			delta = -delta
		}
		d.choose(d.index + delta)
		return nil, true
	case tea.KeyTab, tea.KeyShiftTab, tea.KeyF2:
		options := d.options()
		if d.index < len(options) && options[d.index].model.Reasoning != nil {
			efforts := append([]modelEffort{{ID: "", Name: tr("По умолчанию")}}, options[d.index].model.Reasoning.Efforts...)
			index := 0
			for i, e := range efforts {
				if e.ID == d.effort {
					index = i
					break
				}
			}
			delta := 1
			if msg.Type == tea.KeyShiftTab {
				delta = -1
			}
			d.effort = efforts[(index+delta+len(efforts))%len(efforts)].ID
		}
		return nil, true
	}
	var cmd tea.Cmd
	old := d.search.Value()
	d.search, cmd = d.search.Update(msg)
	if old != d.search.Value() {
		d.choose(0)
	}
	return cmd, true
}

// Heading rows are presentation only; model indices retain their catalog identity.
type modelPickerRow struct {
	heading bool
	label   string
	index   int
}

type modelPickerLayout struct {
	spec       chooserSpec
	geometry   chooserGeometry
	rows       []modelPickerRow
	first, end int
}

func (d *modelPicker) groupedRows() ([]modelPickerRow, int) {
	rows := []modelPickerRow{}
	selected := 0
	previous := ""
	for i, option := range d.options() {
		if i == 0 || option.provider != previous {
			label := option.name
			if label == "" {
				label = option.provider
			}
			rows = append(rows, modelPickerRow{heading: true, label: singleLine(label), index: -1})
			previous = option.provider
		}
		label := option.model.Name
		if label == "" {
			label = option.model.ID
		}
		if i == d.index {
			selected = len(rows)
		}
		rows = append(rows, modelPickerRow{label: singleLine(label), index: i})
	}
	return rows, selected
}

func (m *model) modelsLayout() modelPickerLayout {
	d := m.modelPicker
	spec := chooserSpec{Title: tr("Модель сессии"), Width: 76, Height: 22}
	if d == nil {
		return modelPickerLayout{spec: spec, geometry: m.chooserLayout(spec)}
	}
	current := d.current.Provider + "/" + d.current.Model
	if d.current.Provider == "" && d.current.Model == "" {
		current = "—"
	}
	if d.current.Reasoning != "" {
		current += " · " + d.current.Reasoning
	}
	spec.Detail = tr("Текущая: ") + singleLine(current)
	effort := ""
	options := d.options()
	if d.index >= 0 && d.index < len(options) {
		effort = tr("Рассуждение: ") + "—"
		if reasoning := options[d.index].model.Reasoning; reasoning != nil && len(reasoning.Efforts) > 0 {
			value := tr("По умолчанию")
			selected := d.effort
			if selected == "" {
				selected = reasoning.Default
			}
			for _, e := range reasoning.Efforts {
				if e.ID == selected {
					name := e.Name
					if name == "" {
						name = e.ID
					}
					if d.effort == "" {
						value += " · " + singleLine(name)
					} else {
						value = singleLine(name)
					}
					break
				}
			}
			effort = tr("Рассуждение: ") + value
		}
	}
	spec.Footer = strings.Join([]string{muted().Render(effort), muted().Render(tr("dsh также сохраняет общую модель по умолчанию.")), muted().Render(tr("Текущий запрос продолжится; выбор — для следующего шага.")), muted().Render(singleLine(d.note)), muted().Render(tr("↑↓ модель · F2/Tab рассуждение · Enter применить · F5 обновить"))}, "\n")
	geometry := m.chooserLayout(spec)
	d.search.Width = max(1, geometry.InnerWidth-ansi.StringWidth(d.search.Prompt))
	search := d.search
	cursor := search.Position()
	search.CursorEnd()
	search.SetCursor(cursor)
	spec.Search = search.View()
	rows, selected := d.groupedRows()
	first, end := chooserWindow(selected, len(rows), geometry.List.height)
	// Keep the selected provider visible when its long model list scrolls past
	// the original heading. The synthetic header has no model identity or hit.
	heading := selected
	for heading > 0 && !rows[heading].heading {
		heading--
	}
	if len(rows) > 0 && heading < first && geometry.List.height > 1 {
		count := geometry.List.height - 1
		first = max(heading+1, min(selected-count/2, max(0, len(rows)-count)))
		end = min(len(rows), first+count)
		visible := append([]modelPickerRow{rows[heading]}, rows[first:end]...)
		rows = visible
		first, end = 0, len(rows)
	}
	return modelPickerLayout{spec: spec, geometry: geometry, rows: rows, first: first, end: end}
}

func (m *model) modelsBox() area { return m.modelsLayout().geometry.Box }

func (m *model) modelsView(base string) string {
	d := m.modelPicker
	if d == nil {
		return base
	}
	layout := m.modelsLayout()
	options := d.options()
	rows := []string{}
	for _, row := range layout.rows[layout.first:layout.end] {
		if row.heading {
			rows = append(rows, chooserGroup(row.label))
			continue
		}
		option := options[row.index]
		hint := ""
		if row.label != option.model.ID {
			hint = ansi.Truncate(singleLine(option.model.ID), max(1, layout.geometry.InnerWidth/2), "…")
		}
		if option.provider == d.current.Provider && option.model.ID == d.current.Model {
			hint = "✓"
		}
		rows = append(rows, chooserItem(row.label, hint, row.index == d.index, !d.busy && !d.uncertain, layout.geometry.InnerWidth))
	}
	if len(rows) == 0 && !d.busy {
		rows = append(rows, muted().Render(tr("Нет доступных моделей по этому запросу.")))
	}
	return m.chooserView(base, layout.spec, rows)
}

func (m *model) modelsMouse(msg tea.MouseMsg) tea.Cmd {
	d := m.modelPicker
	if d == nil || msg.Action != tea.MouseActionPress {
		return nil
	}
	layout := m.modelsLayout()
	g := layout.geometry
	if msg.Button == tea.MouseButtonLeft && g.Close.contains(msg.X, msg.Y) && !d.writing {
		return m.closeModels()
	}
	if d.busy {
		return nil
	}
	if msg.Button == tea.MouseButtonLeft && g.List.contains(msg.X, msg.Y) {
		index := layout.first + msg.Y - g.List.y
		if index >= layout.first && index < layout.end && !layout.rows[index].heading {
			d.choose(layout.rows[index].index)
		}
	}
	if (msg.Button == tea.MouseButtonWheelDown || msg.Button == tea.MouseButtonWheelUp) && g.List.contains(msg.X, msg.Y) {
		delta := 1
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		d.choose(d.index + delta)
	}
	return nil
}
