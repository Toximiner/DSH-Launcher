package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

type approvalRequest struct {
	ToolName  string         `json:"toolName"`
	CallID    string         `json:"callId"`
	Reason    string         `json:"reason"`
	Questions []userQuestion `json:"questions"`
	Wait      *questionWait  `json:"wait"`
}
type approvalFrame struct {
	Type     string          `json:"type"`
	ClientID string          `json:"clientId"`
	ID       string          `json:"eventId"`
	AgentID  string          `json:"agentId"`
	Event    string          `json:"event"`
	Request  approvalRequest `json:"request"`
}
type approvalNotice struct {
	generation int
	frame      *approvalFrame
	err        error
	answer     func(string, string) error
	reply      func(string, any) error
	disconnect context.CancelFunc
}
type approvalUpdate struct {
	epoch  int
	notice approvalNotice
}
type approvalAnswerResult struct {
	epoch, generation int
	id                string
	err               error
}
type approvalState struct {
	ctx                  context.Context
	epoch, generation    int
	overflow             bool
	inbox                chan approvalNotice
	ready, visible, busy bool
	requests             []approvalFrame
	selection            int
	viewport             viewport.Model
	answer               func(string, string) error
	reply                func(string, any) error
	forms                map[string]*questionForm
	answeredQuestions    map[string]bool
	disconnect           context.CancelFunc
	note                 string
}

func (m *model) startApprovals(ctx context.Context, id string, epoch int) tea.Cmd {
	s := &approvalState{ctx: ctx, epoch: epoch, inbox: make(chan approvalNotice, 32), selection: -1, viewport: viewport.New(60, 8)}
	m.approvals = s
	login := m.settingsLogin
	return func() tea.Msg { go followApprovalEvents(ctx, login, id, s.inbox); return waitApprovals(s)() }
}
func waitApprovals(s *approvalState) tea.Cmd {
	return func() tea.Msg {
		select {
		case n := <-s.inbox:
			return approvalUpdate{epoch: s.epoch, notice: n}
		case <-s.ctx.Done():
			return nil
		}
	}
}
func (m *model) handleApprovalUpdate(msg approvalUpdate) tea.Cmd {
	s := m.approvals
	if s == nil || s.epoch != msg.epoch || s.ctx.Err() != nil {
		return nil
	}
	n := msg.notice
	if n.err != nil {
		if n.generation == s.generation {
			s.ready = false
			s.busy = false
			s.note = tr("Нет связи с каналом запросов. Ответы недоступны до переподключения.")
		}
	} else if f := n.frame; f != nil {
		if f.Type == "ready" {
			s.generation = n.generation
			s.ready = true
			s.requests = nil
			s.visible = false
			s.busy = false
			s.selection = -1
			s.answer = n.answer
			s.reply = n.reply
			s.disconnect = n.disconnect
			s.note = ""
			s.overflow = false
		} else if n.generation == s.generation && s.ready {
			switch f.Type {
			case "waterfall":
				if m.chat == nil || f.AgentID != m.chat.id || (f.Event != "approval/request" && f.Event != "user-questions/request") || f.ID == "" {
					break
				}
				for _, existing := range s.requests {
					if existing.ID == f.ID {
						return waitApprovals(s)
					}
				}
				if len(s.requests) >= 32 {
					s.overflow = true
					s.note = tr("Остальные запросы будут загружены после этой очереди.")
					break
				}
				request := *f
				request.Request.ToolName = boundedToolText(request.Request.ToolName, 256)
				if len(request.Request.CallID) > 1024 {
					request.Request.CallID = ""
				}
				request.Request.Reason = boundedToolText(request.Request.Reason, toolArgumentsLimit)
				s.requests = append(s.requests, request)
				m.runtimeNotice(tr("Агент ожидает ответа пользователя · F7."))
				if request.Event == "user-questions/request" {
					if form := m.prepareQuestion(request); form.invalid {
						s.requests[len(s.requests)-1].Request.Questions = nil
					}
				}
				if len(s.requests) == 1 {
					s.visible = true
					s.selection = -1
					s.viewport.GotoTop()
					s.note = ""
				}
			case "cancel":
				m.removeApproval(f.ID)
				s.note = tr("Запрос отозван или обработан другим клиентом.")
			}
		}
	}
	m.syncContinuedQuestions()
	m.rebuild()
	return waitApprovals(s)
}
func (m *model) removeApproval(id string) {
	s := m.approvals
	for i, request := range s.requests {
		if request.ID == id {
			delete(s.forms, id)
			current := i == 0
			s.requests = append(s.requests[:i], s.requests[i+1:]...)
			if current {
				s.busy = false
				s.selection = -1
				s.viewport.GotoTop()
				s.visible = len(s.requests) > 0
				if !s.visible && s.overflow && s.disconnect != nil {
					s.ready = false
					s.disconnect()
				}
			}
			return
		}
	}
}
func (m *model) answerApproval() tea.Cmd {
	s := m.approvals
	if s == nil || !s.ready || s.busy || len(s.requests) == 0 || s.selection < 0 || s.answer == nil || m.chatSessionChanging() {
		return nil
	}
	outcome := "rejected"
	if s.selection == 1 {
		outcome = "allowed-once"
	}
	id, epoch, generation, answer := s.requests[0].ID, s.epoch, s.generation, s.answer
	s.busy = true
	s.note = tr("Отправка ответа…")
	return func() tea.Msg {
		return approvalAnswerResult{epoch: epoch, generation: generation, id: id, err: answer(id, outcome)}
	}
}
func (m *model) handleApprovalAnswer(msg approvalAnswerResult) {
	s := m.approvals
	if s == nil || s.epoch != msg.epoch || s.generation != msg.generation || len(s.requests) == 0 || s.requests[0].ID != msg.id {
		return
	}
	question := s.requests[0].Event == "user-questions/request"
	if msg.err != nil {
		s.ready = false
		s.note = tr("Ответ не подтверждён. Восстанавливаем канал; автоматического повтора нет.")
		if s.disconnect != nil {
			s.disconnect()
		}
	} else {
		if question {
			delete(s.forms, questionFormKey(s.requests[0]))
		}
		if question && strings.HasPrefix(msg.id, "continued:") {
			if s.answeredQuestions == nil {
				s.answeredQuestions = make(map[string]bool)
			}
			s.answeredQuestions[msg.id] = true
		}
		m.removeApproval(msg.id)
		if m.chat != nil {
			m.chat.note = tr("Ответ на запрос передан dsh; итог отображается в истории разрешений.")
			if question {
				m.chat.note = tr("Ответы на вопросы переданы dsh.")
			}
		}
	}
	m.rebuild()
}
func (m *model) approvalKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	s := m.approvals
	if s == nil || s.ctx.Err() != nil {
		return nil, false
	}
	if msg.Type == tea.KeyF7 && !msg.Paste {
		if len(s.requests) > 0 {
			s.visible = !s.visible
		}
		return nil, true
	}
	if !s.visible || len(s.requests) == 0 {
		return nil, false
	}
	if s.requests[0].Event == "user-questions/request" {
		return m.questionKey(msg), true
	}
	if msg.Paste {
		return nil, true
	}
	if msg.Type == tea.KeyF8 {
		return m.cancelChat(), true
	}
	if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyCtrlD {
		return tea.Quit, true
	}
	// Ordinary text, including a burst named "enter", never answers a permission.
	if msg.Type == tea.KeyRunes {
		return nil, true
	}
	switch msg.String() {
	case "esc":
		s.visible = false
	case "left", "right", "tab", "shift+tab":
		if !s.busy && s.ready {
			if s.selection < 0 {
				s.selection = 0
			} else {
				s.selection = 1 - s.selection
			}
		}
	case "enter":
		return m.answerApproval(), true
	case "up", "down", "pgup", "pgdown":
		s.viewport, _ = s.viewport.Update(msg)
	}
	return nil, true
}
func (m *model) approvalBox() area {
	width, height := min(78, m.width-3), min(24, m.height-4)
	return area{(m.width - 1 - width) / 2, max(2, (m.height-height)/2), width, height}
}

// Decision dialogs share flat surfaces with the command choosers. Geometry is
// deliberately separate because all reasons and structured answer fields must
// remain accessible rather than being clipped into a short picker list.
func decisionTitle(title string, width int) string {
	return bar(heading().Render(title), muted().Render("esc"), width)
}
func decisionButton(label string, selected bool, width int) string {
	style := lipgloss.NewStyle().Width(max(1, width)).Align(lipgloss.Center).Foreground(theme.Color(theme.Text)).Background(theme.Color(theme.Panel))
	if selected {
		style = style.Foreground(theme.Color(theme.SelectionText)).Background(theme.Color(theme.Selection)).Bold(true)
	}
	return style.Render(ansi.Truncate(label, max(1, width), "…"))
}
func (m *model) decisionClose(box area) area { return area{box.x + box.width - 5, box.y + 1, 3, 1} }
func (m *model) decisionSurface(base string, box area, lines []string) string {
	inner := max(1, box.width-4)
	flat := []string{}
	for _, block := range lines {
		for _, line := range strings.Split(block, "\n") {
			flat = append(flat, ansi.Truncate(line, inner, "…"))
		}
	}
	front := lipgloss.NewStyle().Width(box.width).Height(box.height).Padding(0, 2).Foreground(theme.Color(theme.Text)).Background(theme.Color(theme.Panel)).Render(strings.Join(flat, "\n"))
	return overlay(dimBackdrop(base), surface(front, theme.Panel), box.x, box.y, m.width-1)
}

func (m *model) approvalView(base string) string {
	s := m.approvals
	if s == nil || !s.visible || len(s.requests) == 0 || !m.terminalFits() {
		return base
	}
	if s.requests[0].Event == "user-questions/request" {
		return m.questionView(base)
	}
	request := s.requests[0].Request
	box := m.approvalBox()
	inner := box.width - 4
	s.viewport.Width = inner
	s.viewport.Height = box.height - 8
	reason := clean(request.Reason)
	if reason == "" {
		reason = tr("Причина не передана dsh.")
	}
	content := tr("Инструмент: ") + clean(request.ToolName) + tr("\nПричина:\n") + reason + tr("\n\nАргументы вызова недоступны в текущей истории.")
	if m.chat != nil && request.CallID != "" {
		for _, message := range m.chat.messages {
			if message.tool != nil && message.tool.ID == request.CallID {
				content = tr("Инструмент: ") + clean(request.ToolName) + tr("\nПричина:\n") + reason + tr("\n\nАргументы:\n") + clean(message.tool.Arguments)
				break
			}
		}
	}
	s.viewport.SetContent(ansi.Hardwrap(content, max(1, inner), true))
	note := s.note
	if note == "" {
		note = tr("Разрешение только на этот вызов.")
	}
	left := inner / 2
	buttons := decisionButton(strings.Trim(tr("[ Отказать ]"), "[] "), s.selection == 0, left) + decisionButton(strings.Trim(tr("[ Разрешить ]"), "[] "), s.selection == 1, inner-left)
	lines := []string{"", decisionTitle(tr("Запрос разрешения dsh"), inner), muted().Render(fmt.Sprintf(tr("Ожидает ответа · запросов: %d"), len(s.requests))), s.viewport.View(), ansi.Truncate(note, inner, "…"), buttons, muted().Render(ansi.Truncate(tr("←/→ выбор · Enter ответ · Esc позже"), inner, "…")), muted().Render(ansi.Truncate(tr("PgUp/PgDn детали · F8 остановить"), inner, "…")), ""}
	return m.decisionSurface(base, box, lines)
}
func (m *model) approvalMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	s := m.approvals
	if s == nil || !s.visible || len(s.requests) == 0 {
		return nil, false
	}
	if s.requests[0].Event == "user-questions/request" {
		return m.questionMouse(msg), true
	}
	box := m.approvalBox()
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.decisionClose(box).contains(msg.X, msg.Y) {
		s.visible = false
		return nil, true
	}
	body := area{box.x + 2, box.y + 3, box.width - 4, box.height - 8}
	if (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) && body.contains(msg.X, msg.Y) {
		s.viewport, _ = s.viewport.Update(msg)
		return nil, true
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y == box.y+box.height-4 && msg.X >= box.x+2 && msg.X < box.x+box.width-2 && s.ready && !s.busy {
		s.selection = 0
		if msg.X >= box.x+2+(box.width-4)/2 {
			s.selection = 1
		}
		return m.answerApproval(), true
	}
	return nil, true
}

func approvalOutcomeLabel(outcome string) string {
	switch outcome {
	case "allowed-once":
		return tr("Разрешено один раз")
	case "rejected":
		return tr("Отказано")
	case "cancelled":
		return tr("Запрос отменён")
	case "unavailable":
		return tr("Канал подтверждения недоступен")
	default:
		return tr("Неизвестное решение dsh")
	}
}
