package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type pluginField struct {
	ID, Label, Type string
	Required        bool
	Default         any
	Choices         []string
}
type pluginAction struct{ Label, Command string }
type pluginTable struct {
	Title   string
	Columns []string
	Rows    [][]string
}
type pluginStatus struct{ Label, Value string }
type pluginPanel struct {
	Schema           string `json:"$schema"`
	Owner, Title     string
	Fields           []pluginField
	Actions          []pluginAction
	Tables           []pluginTable
	Status           []pluginStatus
	values           map[string]string
	index, lastIndex int
	editing          bool
	input            textinput.Model
}

var pluginCommandName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var pluginFieldName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)

// The existing human-command RPC carries JSON as text. No arbitrary RPC names,
// scripts, HTML or model requests are evaluated by the declarative renderer.
func decodePluginPanel(text, source string) (*pluginPanel, error) {
	var marker struct {
		Schema string `json:"$schema"`
	}
	if json.Unmarshal([]byte(text), &marker) != nil || marker.Schema == "" {
		return nil, nil
	}
	invalid := errors.New(tr("Панель плагина не поддерживается. Проверьте версию контракта TUI."))
	if marker.Schema != "dsh-tui-panel/v1" || len(text) > 65536 {
		return nil, invalid
	}
	var p pluginPanel
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if decoder.Decode(&p) != nil || !pluginCommandName.MatchString(p.Owner) || (source != p.Owner && !strings.HasPrefix(source, p.Owner+"-")) || p.Title == "" || len(p.Fields) > 20 || len(p.Actions) > 8 || len(p.Status) > 20 || len(p.Tables) > 4 {
		return nil, invalid
	}
	p.values = map[string]string{}
	for _, f := range p.Fields {
		if !pluginFieldName.MatchString(f.ID) || f.Label == "" {
			return nil, invalid
		}
		if _, exists := p.values[f.ID]; exists {
			return nil, invalid
		}
		switch f.Type {
		case "string", "integer", "boolean":
		case "enum":
			if len(f.Choices) == 0 || len(f.Choices) > 30 {
				return nil, invalid
			}
		default:
			return nil, invalid
		}
		value := ""
		if f.Default != nil {
			value = fmt.Sprint(f.Default)
		} else if f.Type == "boolean" {
			value = "false"
		} else if f.Type == "enum" {
			value = f.Choices[0]
		}
		if len(value) > 4096 {
			return nil, invalid
		}
		p.values[f.ID] = value
	}
	for _, a := range p.Actions {
		if !pluginCommandName.MatchString(a.Command) || !strings.HasPrefix(a.Command, p.Owner+"-") || a.Label == "" {
			return nil, invalid
		}
	}
	for _, table := range p.Tables {
		if len(table.Columns) == 0 || len(table.Columns) > 10 || len(table.Rows) > 200 {
			return nil, invalid
		}
		for _, row := range table.Rows {
			if len(row) != len(table.Columns) {
				return nil, invalid
			}
		}
	}
	if len(p.Actions)+len(p.Fields) == 0 {
		p.index = -1
	}
	return &p, nil
}
func (p *pluginPanel) arguments() (map[string]any, error) {
	values := map[string]any{}
	for _, f := range p.Fields {
		text := p.values[f.ID]
		if len(text) > 4096 || (f.Required && strings.TrimSpace(text) == "") {
			return nil, fmt.Errorf(tr("%s: нужно значение до 4096 байт"), clean(f.Label))
		}
		if !f.Required && text == "" {
			continue
		}
		switch f.Type {
		case "string":
			values[f.ID] = text
		case "integer":
			v, err := strconv.ParseInt(text, 10, 64)
			if err != nil || v < -9007199254740991 || v > 9007199254740991 {
				return nil, fmt.Errorf(tr("%s: ожидается целое число"), clean(f.Label))
			}
			values[f.ID] = v
		case "boolean":
			if text != "true" && text != "false" {
				return nil, errors.New(tr("Недопустимое логическое значение"))
			}
			values[f.ID] = text == "true"
		case "enum":
			found := false
			for _, option := range f.Choices {
				if option == text {
					found = true
				}
			}
			if !found {
				return nil, errors.New(tr("Выберите значение из списка"))
			}
			values[f.ID] = text
		}
	}
	encoded, _ := json.Marshal(values)
	if len(encoded) > 16384 {
		return nil, errors.New(tr("Форма плагина превышает 16 КиБ"))
	}
	return values, nil
}
func (m *model) pluginPanelKey(msg tea.KeyMsg) tea.Cmd {
	d := m.workbench
	p := d.pluginPanel
	if p.editing {
		if !msg.Paste && msg.Type == tea.KeyEsc {
			p.editing = false
			p.input.Blur()
			return nil
		}
		if !msg.Paste && msg.Type == tea.KeyEnter {
			p.values[p.Fields[p.index].ID] = p.input.Value()
			p.editing = false
			p.input.Blur()
			return nil
		}
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		return cmd
	}
	if msg.Paste {
		return nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		d.pluginPanel = nil
		d.note = tr("Снимок dsh · F5 обновить · Esc назад")
		return nil
	case tea.KeyF5:
		d.pluginPanel = nil
		return m.loadWorkbench()
	case tea.KeyUp, tea.KeyDown, tea.KeyTab:
		count := len(p.Fields) + len(p.Actions)
		if count > 0 {
			delta := 1
			if msg.Type == tea.KeyUp {
				delta = -1
			}
			p.index = (p.index + delta + count) % count
		}
		return nil
	case tea.KeyPgUp, tea.KeyPgDown:
		var cmd tea.Cmd
		d.view, cmd = d.view.Update(msg)
		return cmd
	case tea.KeyEnter, tea.KeySpace, tea.KeyLeft, tea.KeyRight:
		if p.index < 0 {
			return nil
		}
		if p.index < len(p.Fields) {
			f := p.Fields[p.index]
			if f.Type == "boolean" {
				if p.values[f.ID] == "true" {
					p.values[f.ID] = "false"
				} else {
					p.values[f.ID] = "true"
				}
				return nil
			}
			if f.Type == "enum" {
				index := 0
				for i, option := range f.Choices {
					if option == p.values[f.ID] {
						index = i
					}
				}
				delta := 1
				if msg.Type == tea.KeyLeft {
					delta = -1
				}
				p.values[f.ID] = f.Choices[(index+delta+len(f.Choices))%len(f.Choices)]
				return nil
			}
			if msg.Type == tea.KeyEnter {
				p.input = textinput.New()
				p.input.CharLimit = 4096
				p.input.SetValue(p.values[f.ID])
				p.editing = true
				return p.input.Focus()
			}
			return nil
		}
		if msg.Type != tea.KeyEnter || d.blocked {
			return nil
		}
		action := p.Actions[p.index-len(p.Fields)]
		known := false
		for _, row := range d.rows {
			if row.ID == action.Command {
				known = true
			}
		}
		if !known {
			d.note = tr("Действие плагина недоступно; F5 — перечитать команды.")
			return nil
		}
		values, err := p.arguments()
		if err != nil {
			d.note = err.Error()
			return nil
		}
		data, _ := json.Marshal(values)
		d.action = &benchAction{"commands/execute", map[string]any{"agentId": d.id, "line": "/" + action.Command + " " + string(data), "submittedAttachments": []any{}}, clean(action.Label)}
		d.confirm = true
		d.note = clean(action.Label) + tr(" · Enter — подтвердить · Esc — отменить")
	}
	return nil
}
func (m *model) pluginPanelView(base string) string {
	d := m.workbench
	p := d.pluginPanel
	box := m.benchBox()
	width := box.width - 4
	lines := []string{bar(heading().Render(clean(p.Title)), muted().Render("esc"), width), muted().Render(tr("Плагин: ") + p.Owner), ""}
	var body []string
	selectedRowIndex := 0
	for i, f := range p.Fields {
		prefix := "  "
		if p.index == i {
			prefix = "› "
			selectedRowIndex = len(body)
		}
		body = append(body, prefix+clean(f.Label))
		text := clean(p.values[f.ID])
		if p.editing && p.index == i {
			p.input.Width = max(1, width-4)
			text = p.input.View()
		}
		body = append(body, "  "+text)
	}
	for _, s := range p.Status {
		body = append(body, clean(s.Label)+": "+clean(s.Value))
	}
	for _, table := range p.Tables {
		body = append(body, "", clean(table.Title), clean(strings.Join(table.Columns, " │ ")))
		for _, row := range table.Rows {
			body = append(body, clean(strings.Join(row, " │ ")))
		}
	}
	for i, a := range p.Actions {
		prefix := "  "
		if p.index == len(p.Fields)+i {
			prefix = "› "
			selectedRowIndex = len(body)
		}
		body = append(body, prefix+"[ "+clean(a.Label)+" ]")
	}
	d.view.Width = width
	d.view.Height = max(1, box.height-9)
	d.view.SetContent(ansi.Hardwrap(strings.Join(body, "\n"), width, true))
	// Follow keyboard selection unless the user is paging through read-only data.
	if selectedRowIndex > 0 {
		selectedRowIndex = len(strings.Split(ansi.Hardwrap(strings.Join(body[:selectedRowIndex], "\n"), width, true), "\n"))
	}
	if p.index != p.lastIndex {
		if selectedRowIndex < d.view.YOffset {
			d.view.SetYOffset(selectedRowIndex)
		} else if selectedRowIndex >= d.view.YOffset+d.view.Height {
			d.view.SetYOffset(selectedRowIndex - d.view.Height + 1)
		}
		p.lastIndex = p.index
	}
	lines = append(lines, strings.Split(d.view.View(), "\n")...)
	hint := tr("↑/↓ поле/действие · Enter правка/запуск · PgUp/PgDn детали")
	if p.editing {
		hint = tr("Enter — принять поле · Esc — отменить правку")
	}
	if d.confirm {
		hint = tr("Enter — подтвердить действие · Esc — отменить")
	}
	lines = append(lines, ansi.Truncate(clean(d.note), width, "…"), hint, tr("F5 — перечитать команды · Esc — назад"))
	return overlay(dimBackdrop(base), flatDialogSurface(box, lines), box.x, box.y, m.width-1)
}
