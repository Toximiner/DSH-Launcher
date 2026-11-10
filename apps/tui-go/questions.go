package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type questionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}
type userQuestion struct {
	ID          string           `json:"id"`
	Question    string           `json:"question"`
	Detail      string           `json:"detail,omitempty"`
	Header      string           `json:"header,omitempty"`
	Options     []questionOption `json:"options,omitempty"`
	MultiSelect bool             `json:"multiSelect,omitempty"`
	Intent      json.RawMessage  `json:"intent,omitempty"`
}
type questionWait struct {
	CallID string `json:"callId"`
	Timed  bool   `json:"timed"`
}
type continuedQuestion struct {
	CallID    string         `json:"callId"`
	Questions []userQuestion `json:"questions"`
	State     string         `json:"state"`
}
type questionAnswer struct {
	ID       string   `json:"id"`
	Selected []string `json:"selected"`
	Custom   string   `json:"custom,omitempty"`
}
type questionAnswers struct {
	Answers []questionAnswer `json:"answers"`
}
type questionDraft struct {
	selected map[int]bool
	custom   textarea.Model
	skipped  bool
}
type questionHit struct{ option, start, end int }
type questionForm struct {
	index, cursor int
	drafts        []questionDraft
	err           string
	invalid       bool
	reveal        bool
	hits          []questionHit
}

func validateQuestions(questions []userQuestion) error {
	if len(questions) == 0 || len(questions) > 32 {
		return errors.New("Поддерживается от 1 до 32 вопросов в запросе.")
	}
	ids := make(map[string]bool)
	size := 0
	for _, q := range questions {
		if q.ID == "" || len(q.ID) > 256 || ids[q.ID] {
			return errors.New("Некорректные идентификаторы вопросов.")
		}
		ids[q.ID] = true
		if len(q.Options) > 64 {
			return errors.New("В одном вопросе поддерживается до 64 вариантов.")
		}
		labels := make(map[string]bool)
		for _, o := range q.Options {
			if o.Label == "" || len(o.Label) > 4096 || labels[o.Label] {
				return errors.New("Некорректные варианты ответа.")
			}
			labels[o.Label] = true
			size += len(o.Label) + len(o.Description)
		}
		size += len(q.Question) + len(q.Detail) + len(q.Header) + len(q.Intent)
		if size > chatTextLimit {
			return errors.New("Вопросы превышают ограничение размера TUI.")
		}
	}
	return nil
}
func (m *model) prepareQuestion(request approvalFrame) *questionForm {
	s := m.approvals
	if s.forms == nil {
		s.forms = make(map[string]*questionForm)
	}
	key := questionFormKey(request)
	if f := s.forms[key]; f != nil {
		return f
	}
	f := &questionForm{cursor: -1}
	if err := validateQuestions(request.Request.Questions); err != nil {
		f.invalid = true
		f.err = err.Error()
	} else {
		for range request.Request.Questions {
			input := textarea.New()
			input.CharLimit = 65536
			input.ShowLineNumbers = false
			input.Prompt = "> "
			input.Placeholder = "Свой ответ…"
			input.SetHeight(3)
			input.Blur()
			f.drafts = append(f.drafts, questionDraft{selected: make(map[int]bool), custom: input})
		}
	}
	// Keep draft memory bounded even when a server no longer replays old requests.
	if len(s.forms) >= 32 {
		for id := range s.forms {
			present := false
			for _, r := range s.requests {
				if questionFormKey(r) == id {
					present = true
					break
				}
			}
			if !present {
				delete(s.forms, id)
				break
			}
		}
	}
	s.forms[key] = f
	return f
}
func (m *model) questionCurrent() (*questionForm, *userQuestion, *questionDraft) {
	s := m.approvals
	f := m.prepareQuestion(s.requests[0])
	if f.invalid {
		return f, nil, nil
	}
	q := &s.requests[0].Request.Questions[f.index]
	return f, q, &f.drafts[f.index]
}
func (m *model) questionFocus(cursor int) tea.Cmd {
	f, q, d := m.questionCurrent()
	if f.invalid {
		return nil
	}
	f.cursor = cursor
	f.reveal = true
	if cursor == len(q.Options) {
		return d.custom.Focus()
	}
	d.custom.Blur()
	return nil
}
func (m *model) questionPage(delta int) {
	f, _, d := m.questionCurrent()
	if f.invalid {
		return
	}
	d.custom.Blur()
	f.index = max(0, min(len(f.drafts)-1, f.index+delta))
	f.cursor = -1
	f.reveal = false
	f.err = ""
	m.approvals.viewport.GotoTop()
}
func (m *model) chooseQuestionOption(index int) {
	f, q, d := m.questionCurrent()
	if f.invalid || index < 0 || index >= len(q.Options) {
		return
	}
	d.skipped = false
	f.err = ""
	if q.MultiSelect {
		d.selected[index] = !d.selected[index]
	} else {
		d.selected = map[int]bool{index: true}
		d.custom.SetValue("")
	}
}
func (m *model) answerQuestion() tea.Cmd {
	s := m.approvals
	if !s.ready || s.busy || s.reply == nil || m.chatSessionChanging() {
		return nil
	}
	f, _, _ := m.questionCurrent()
	if f.invalid {
		return nil
	}
	batch := questionAnswers{Answers: make([]questionAnswer, 0, len(f.drafts))}
	for i, q := range s.requests[0].Request.Questions {
		d := &f.drafts[i]
		a := questionAnswer{ID: q.ID, Selected: []string{}}
		if !d.skipped {
			a.Custom = strings.TrimSpace(clean(d.custom.Value()))
			for j, o := range q.Options {
				if d.selected[j] && (a.Custom == "" || q.MultiSelect) {
					a.Selected = append(a.Selected, o.Label)
				}
			}
			if len(a.Selected) == 0 && a.Custom == "" {
				f.index = i
				f.cursor = -1
				f.err = "Ответьте на вопрос или нажмите «Пропуск»."
				s.viewport.GotoTop()
				return nil
			}
		}
		batch.Answers = append(batch.Answers, a)
	}
	id, epoch, generation, reply := s.requests[0].ID, s.epoch, s.generation, s.reply
	s.busy = true
	s.note = "Отправка ответов…"
	f.err = ""
	return func() tea.Msg {
		return approvalAnswerResult{epoch: epoch, generation: generation, id: id, err: reply(id, batch)}
	}
}
func (m *model) questionKey(msg tea.KeyMsg) tea.Cmd {
	s := m.approvals
	if !msg.Paste {
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlD:
			return tea.Quit
		case tea.KeyF8:
			return m.cancelChat()
		case tea.KeyEsc:
			s.visible = false
			return nil
		case tea.KeyF5:
			return m.answerQuestion()
		}
	}
	f, q, d := m.questionCurrent()
	if f.invalid || s.busy || !s.ready {
		return nil
	}
	editing := f.cursor == len(q.Options)
	if !msg.Paste && msg.Type != tea.KeyRunes {
		switch msg.String() {
		case "tab", "shift+tab":
			delta := 1
			if msg.String() == "shift+tab" {
				delta = -1
			}
			cursor := f.cursor
			if cursor < 0 && delta < 0 {
				cursor = 0
			}
			return m.questionFocus((cursor + delta + len(q.Options) + 1) % (len(q.Options) + 1))
		case "pgup", "pgdown":
			s.viewport, _ = s.viewport.Update(msg)
			return nil
		case "left", "right":
			if !editing {
				delta := 1
				if msg.String() == "left" {
					delta = -1
				}
				m.questionPage(delta)
				return nil
			}
		case "up", "down":
			if !editing {
				delta := 1
				if msg.String() == "up" {
					delta = -1
				}
				return m.questionFocus((f.cursor + delta + len(q.Options) + 1) % (len(q.Options) + 1))
			}
		case "enter":
			if !editing {
				m.chooseQuestionOption(f.cursor)
				return nil
			}
		}
	}
	if editing {
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Alt {
			msg.Paste = true
		}
		before := d.custom.Value()
		var cmd tea.Cmd
		d.custom, cmd = d.custom.Update(msg)
		if before != d.custom.Value() {
			d.skipped = false
			f.err = ""
			if !q.MultiSelect {
				d.selected = make(map[int]bool)
			}
		}
		return cmd
	}
	if !msg.Paste && (msg.Type == tea.KeySpace || msg.Type == tea.KeyRunes && string(msg.Runes) == " ") {
		m.chooseQuestionOption(f.cursor)
	}
	return nil
}
func questionButtons(inner int) string {
	return lipgloss.NewStyle().Width(5).Render("[←]") + lipgloss.NewStyle().Width(5).Render("[→]") + lipgloss.NewStyle().Width(11).Render("[Пропуск]") + "[Ответить]"
}
func (m *model) questionView(base string) string {
	s := m.approvals
	f, q, d := m.questionCurrent()
	box := m.approvalBox()
	inner := box.width - 4
	s.viewport.Width = inner
	s.viewport.Height = max(1, box.height-11)
	title, progress, body := "Вопрос агента", "", ""
	if !f.invalid {
		progress = fmt.Sprintf("Вопрос %d/%d · ", f.index+1, len(f.drafts))
		if len(q.Options) == 0 {
			progress += "Свободный ответ"
		} else if q.MultiSelect {
			progress += "Выбор нескольких"
		} else {
			progress += "Выбор одного"
		}
		body = clean(q.Header) + "\n" + clean(q.Question)
		if len(q.Intent) > 0 {
			var intent struct {
				Kind string `json:"kind"`
			}
			if json.Unmarshal(q.Intent, &intent) == nil {
				if intent.Kind == "approval" {
					body += "\nПодтверждение действия"
				} else if intent.Kind == "plan-review" {
					body += "\nСогласование плана"
				}
			}
		}
		if q.Detail != "" {
			body += "\n\n" + clean(q.Detail)
		}
		body += "\n"
		f.hits = nil
		for i, o := range q.Options {
			mark := "[ ] "
			if d.selected[i] {
				mark = "[✓] "
			}
			if f.cursor == i {
				mark = "> " + mark
			} else {
				mark = "  " + mark
			}
			row := strings.Count(ansi.Hardwrap(body, inner, true), "\n") + 1
			part := mark + clean(o.Label)
			if o.Description != "" {
				part += "\n      " + clean(o.Description)
			}
			body += "\n" + part
			f.hits = append(f.hits, questionHit{option: i, start: row, end: row + strings.Count(ansi.Hardwrap(part, inner, true), "\n") + 1})
		}
	}
	s.viewport.SetContent(ansi.Hardwrap(body, inner, true))
	if f.reveal {
		for _, hit := range f.hits {
			if hit.option == f.cursor {
				if hit.start < s.viewport.YOffset {
					s.viewport.SetYOffset(hit.start)
				} else if hit.end > s.viewport.YOffset+s.viewport.Height {
					s.viewport.SetYOffset(hit.end - s.viewport.Height)
				}
			}
		}
		f.reveal = false
	}
	label := "Свой ответ · Tab — выбрать поле"
	editor := strings.Repeat("\n", 2)
	if d != nil {
		d.custom.SetWidth(inner)
		d.custom.SetHeight(3)
		editor = d.custom.View()
		if f.cursor == len(q.Options) {
			label = "Свой ответ · Enter — новая строка"
		}
	}
	note := s.note
	if note == "" {
		note = "F5 — отправить все ответы"
	}
	if f.err != "" {
		note = f.err
	} else if d != nil && d.skipped {
		note = "Вопрос пропущен. F5 — отправить."
	}
	lines := []string{heading.Render(title), muted.Render(ansi.Truncate(progress, inner, "…")), s.viewport.View(), muted.Render(ansi.Truncate(label, inner, "…")), editor, ansi.Truncate(note, inner, "…"), questionButtons(inner), muted.Render(ansi.Truncate("Tab выбор · Enter отметить · F5 ответ", inner, "…"))}
	return overlay(base, panel(box.width, box.height, strings.Join(lines, "\n"), "#79c0ff"), box.x, box.y, m.width-1)
}
func (m *model) questionMouse(msg tea.MouseMsg) tea.Cmd {
	s := m.approvals
	f, _, _ := m.questionCurrent()
	box := m.approvalBox()
	inner := box.width - 4
	bodyHeight := max(1, box.height-11)
	body := area{box.x + 2, box.y + 3, inner, bodyHeight}
	if (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) && body.contains(msg.X, msg.Y) {
		s.viewport, _ = s.viewport.Update(msg)
		return nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft || !s.ready || s.busy || f.invalid {
		return nil
	}
	if body.contains(msg.X, msg.Y) {
		row := msg.Y - body.y + s.viewport.YOffset
		for _, hit := range f.hits {
			if row >= hit.start && row < hit.end {
				m.questionFocus(hit.option)
				m.chooseQuestionOption(hit.option)
				return nil
			}
		}
	}
	editor := area{body.x, body.y + bodyHeight + 1, inner, 3}
	if editor.contains(msg.X, msg.Y) {
		_, q, _ := m.questionCurrent()
		return m.questionFocus(len(q.Options))
	}
	if msg.Y == box.y+box.height-3 && msg.X >= box.x+2 && msg.X < box.x+box.width-2 {
		x := msg.X - box.x - 2
		switch {
		case x < 5:
			m.questionPage(-1)
		case x < 10:
			m.questionPage(1)
		case x < 21:
			_, _, d := m.questionCurrent()
			d.skipped = true
			d.selected = make(map[int]bool)
			d.custom.SetValue("")
		default:
			return m.answerQuestion()
		}
	}
	return nil
}

func (m *model) syncContinuedQuestions() {
	s := m.approvals
	if s == nil || !s.ready || m.session == nil || m.chat == nil || m.chat.id != m.session.ID {
		return
	}
	active := make(map[string]bool)
	for _, pending := range m.session.Questions {
		if pending.State != "continued" || pending.CallID == "" || len(pending.CallID) > 1024 {
			continue
		}
		id := "continued:" + pending.CallID
		active[id] = true
		if s.answeredQuestions[id] {
			continue
		}
		present := false
		for _, r := range s.requests {
			if r.ID == id {
				present = true
				break
			}
		}
		if present || len(s.requests) >= 32 {
			continue
		}
		frame := approvalFrame{Type: "waterfall", ID: id, AgentID: m.session.ID, Event: "user-questions/request", Request: approvalRequest{Questions: pending.Questions, Wait: &questionWait{CallID: pending.CallID}}}
		s.requests = append(s.requests, frame)
		m.prepareQuestion(frame)
		if len(s.requests) == 1 {
			s.visible = true
			s.selection = -1
			s.viewport.GotoTop()
		}
	}
	for i := len(s.requests) - 1; i >= 0; i-- {
		r := s.requests[i]
		if strings.HasPrefix(r.ID, "continued:") && !active[r.ID] {
			m.removeApproval(r.ID)
		}
	}
	for id := range s.answeredQuestions {
		if !active[id] {
			delete(s.answeredQuestions, id)
		}
	}
}

func (m *model) questionMessage(msg tea.Msg) (tea.Cmd, bool) {
	s := m.approvals
	if s == nil || !s.visible || len(s.requests) == 0 || s.requests[0].Event != "user-questions/request" {
		return nil, false
	}
	f, q, d := m.questionCurrent()
	if f.invalid || f.cursor != len(q.Options) || !s.ready || s.busy {
		return nil, true
	}
	before := d.custom.Value()
	var cmd tea.Cmd
	d.custom, cmd = d.custom.Update(msg)
	if before != d.custom.Value() {
		d.skipped = false
		f.err = ""
		if !q.MultiSelect {
			d.selected = make(map[int]bool)
		}
	}
	return cmd, true
}

func questionReplyText(text string) string {
	var batch questionAnswers
	if json.Unmarshal([]byte(text), &batch) != nil || len(batch.Answers) == 0 {
		return text
	}
	lines := []string{"Ответы пользователя:"}
	for _, a := range batch.Answers {
		values := append([]string{}, a.Selected...)
		if a.Custom != "" {
			values = append(values, a.Custom)
		}
		value := strings.Join(values, ", ")
		if value == "" {
			value = "Пропущен"
		}
		lines = append(lines, clean(a.ID)+": "+clean(value))
	}
	return boundedToolText(strings.Join(lines, "\n"), chatTextLimit)
}

func questionFormKey(request approvalFrame) string {
	if request.Request.Wait != nil && request.Request.Wait.CallID != "" && len(request.Request.Wait.CallID) <= 1024 {
		return "call:" + request.Request.Wait.CallID
	}
	return request.ID
}
