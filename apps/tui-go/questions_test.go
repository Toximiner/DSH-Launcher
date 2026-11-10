package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func questionModel(t *testing.T, questions []userQuestion) *model {
	m := approvalModel(t)
	m.handleApprovalUpdate(approvalUpdate{epoch: 1, notice: approvalNotice{generation: 1, frame: &approvalFrame{Type: "waterfall", ID: "question-event", AgentID: "s", Event: "user-questions/request", Request: approvalRequest{Questions: questions}}}})
	return m
}
func questionKey(m *model, kind tea.KeyType) tea.Cmd {
	_, cmd := m.Update(tea.KeyMsg{Type: kind})
	return cmd
}
func questionText(m *model, text string) {
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
}
func TestQuestionStructuredBatchAndDraftIsolation(t *testing.T) {
	m := questionModel(t, []userQuestion{
		{ID: "language", Question: "Choose", Options: []questionOption{{Label: "Go"}, {Label: "Rust"}}},
		{ID: "checks", Question: "Which checks?", MultiSelect: true, Options: []questionOption{{Label: "Unit"}, {Label: "PTY"}}},
		{ID: "detail", Question: "Anything else?"},
	})
	m.input.SetValue("unsent chat draft")
	var got questionAnswers
	m.approvals.reply = func(id string, value any) error {
		if id != "question-event" {
			t.Fatal("wrong event")
		}
		got = value.(questionAnswers)
		return nil
	}
	if questionKey(m, tea.KeyEnter) != nil {
		t.Fatal("unselected Enter submitted a question")
	}
	questionText(m, "enter")
	if m.input.Value() != "unsent chat draft" {
		t.Fatal("question input changed the chat draft")
	}
	questionKey(m, tea.KeyTab)
	questionKey(m, tea.KeyEnter)
	questionKey(m, tea.KeyRight)
	questionKey(m, tea.KeyTab)
	questionKey(m, tea.KeyEnter)
	questionKey(m, tea.KeyDown)
	questionKey(m, tea.KeySpace)
	questionKey(m, tea.KeyRight)
	questionKey(m, tea.KeyTab)
	questionText(m, "enter")
	questionKey(m, tea.KeyEnter)
	questionText(m, "世界")
	questionKey(m, tea.KeyEnter)
	if len(m.prompts) != 0 || m.approvals.busy {
		t.Fatal("text Enter sent a normal prompt or submitted the batch")
	}
	cmd := questionKey(m, tea.KeyF5)
	if cmd == nil {
		t.Fatal("complete batch did not submit")
	}
	m.Update(cmd())
	if len(got.Answers) != 3 || got.Answers[0].Selected[0] != "Go" || len(got.Answers[1].Selected) != 2 || got.Answers[2].Custom != "enter\n世界" {
		t.Fatalf("incorrect structured answer: %#v", got)
	}
	if len(m.approvals.requests) != 0 || m.input.Value() != "unsent chat draft" || len(m.prompts) != 0 {
		t.Fatal("batch answer lost the draft or entered ordinary chat")
	}
}
func TestQuestionCompletenessDeferralAndSingleChoiceCustom(t *testing.T) {
	m := questionModel(t, []userQuestion{{ID: "q", Question: "Choose or explain", Options: []questionOption{{Label: "Exact label 世界"}}}})
	m.approvals.reply = func(string, any) error { return nil }
	if questionKey(m, tea.KeyF5) != nil {
		t.Fatal("incomplete batch submitted")
	}
	questionKey(m, tea.KeyTab)
	questionKey(m, tea.KeyEnter)
	questionKey(m, tea.KeyTab)
	questionText(m, "custom answer")
	f, _, d := m.questionCurrent()
	if len(d.selected) != 0 {
		t.Fatal("custom text retained a single-choice selection")
	}
	questionKey(m, tea.KeyEsc)
	if m.approvals.visible {
		t.Fatal("Escape did not defer")
	}
	questionKey(m, tea.KeyF7)
	if !m.approvals.visible || d.custom.Value() != "custom answer" {
		t.Fatal("reopening lost question progress")
	}
	m.handleApprovalUpdate(approvalUpdate{epoch: 1, notice: approvalNotice{generation: 1, err: errors.New("lost")}})
	if questionKey(m, tea.KeyF5) != nil {
		t.Fatal("disconnected question accepted an answer")
	}
	m.handleApprovalUpdate(approvalUpdate{epoch: 1, notice: approvalNotice{generation: 2, frame: &approvalFrame{Type: "ready", ClientID: "next"}, reply: func(string, any) error { return nil }}})
	m.handleApprovalUpdate(approvalUpdate{epoch: 1, notice: approvalNotice{generation: 2, frame: &approvalFrame{Type: "waterfall", ID: "question-event", AgentID: "s", Event: "user-questions/request", Request: approvalRequest{Questions: []userQuestion{{ID: "q", Question: "Choose or explain", Options: []questionOption{{Label: "Exact label 世界"}}}}}}}})
	restored, _, draft := m.questionCurrent()
	if restored != f || draft.custom.Value() != "custom answer" {
		t.Fatal("connection replay lost an unfinished answer")
	}
}
func TestQuestionContinuedProjectionAndDuplicateSuppression(t *testing.T) {
	m := approvalModel(t)
	m.session.Questions = []continuedQuestion{{CallID: "late", State: "continued", Questions: []userQuestion{{ID: "id", Question: "Late answer?", Options: []questionOption{{Label: "Yes"}}}}}}
	var id string
	m.approvals.reply = func(event string, _ any) error { id = event; return nil }
	m.syncContinuedQuestions()
	m.syncContinuedQuestions()
	if len(m.approvals.requests) != 1 {
		t.Fatal("projection created duplicate questions")
	}
	questionKey(m, tea.KeyTab)
	questionKey(m, tea.KeyEnter)
	cmd := questionKey(m, tea.KeyF5)
	m.Update(cmd())
	m.syncContinuedQuestions()
	if id != "continued:late" || len(m.approvals.requests) != 0 {
		t.Fatal("accepted continued answer was offered again before admission")
	}
	m.session.Questions = nil
	m.syncContinuedQuestions()
	if len(m.approvals.answeredQuestions) != 0 {
		t.Fatal("settled question receipts were retained indefinitely")
	}
}
func TestQuestionMinimumLayoutMouseAndExplicitSkip(t *testing.T) {
	for _, width := range []int{44, 100} {
		m := questionModel(t, []userQuestion{{ID: "q", Question: "Question?", Options: []questionOption{{Label: "One", Description: strings.Repeat("detail ", 40)}, {Label: "Two"}}}})
		var batch questionAnswers
		m.approvals.reply = func(_ string, value any) error { batch = value.(questionAnswers); return nil }
		m.Update(tea.WindowSizeMsg{Width: width, Height: 18})
		lines := strings.Split(clean(m.View()), "\n")
		box := m.approvalBox()
		y := box.y + box.height - 3
		if y >= len(lines) || !strings.Contains(lines[y], "Ответить") || !strings.Contains(lines[y], "Пропуск") {
			t.Fatal("question buttons overflowed or mouse row is incorrect")
		}
		f, _, _ := m.questionCurrent()
		hit := f.hits[0]
		m.approvals.viewport.SetYOffset(hit.start)
		m.View()
		m.mouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		_, _, d := m.questionCurrent()
		if !d.selected[0] {
			t.Fatal("mouse did not choose the visible option")
		}
		m.mouse(tea.MouseMsg{X: box.x + 13, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if !d.skipped {
			t.Fatal("explicit Skip did not mark the question skipped")
		}
		cmd := m.mouse(tea.MouseMsg{X: box.x + 25, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if cmd == nil {
			t.Fatal("mouse did not submit")
		}
		m.Update(cmd())
		if len(batch.Answers) != 1 || len(batch.Answers[0].Selected) != 0 || batch.Answers[0].Custom != "" {
			t.Fatal("skipped question was turned into a selection")
		}
	}
}
func TestQuestionInvalidIdentityAndSafeHistory(t *testing.T) {
	for _, questions := range [][]userQuestion{{{ID: "q"}, {ID: "q"}}, {{ID: "q", Options: []questionOption{{Label: "duplicate"}, {Label: "duplicate"}}}}} {
		m := questionModel(t, questions)
		m.approvals.reply = func(string, any) error { t.Fatal("invalid request was answered"); return nil }
		if questionKey(m, tea.KeyF5) != nil {
			t.Fatal("invalid question produced an action")
		}
	}
	m := chatModel(t)
	raw, _ := json.Marshal(map[string]any{"source": map[string]any{"kind": "user-question-reply"}, "content": []any{map[string]any{"type": "text", "text": `{"answers":[{"id":"q","selected":["Yes"],"custom":"why"}]}`}}})
	if err := m.applyChatEvent(m.chat, chatEvent{Seq: 0, Type: "user/message", Data: raw}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.chatView(), "ОТВЕТЫ НА ВОПРОС") || !strings.Contains(m.chatView(), "q: Yes, why") {
		t.Fatal("continued answer was hidden as service context")
	}
}

func TestQuestionProgressSurvivesTimedContinuation(t *testing.T) {
	m := approvalModel(t)
	request := approvalFrame{Type: "waterfall", ID: "live-event", AgentID: "s", Event: "user-questions/request", Request: approvalRequest{Wait: &questionWait{CallID: "stable", Timed: true}, Questions: []userQuestion{{ID: "q", Question: "Question?"}}}}
	m.handleApprovalUpdate(approvalUpdate{epoch: 1, notice: approvalNotice{generation: 1, frame: &request}})
	questionKey(m, tea.KeyTab)
	questionText(m, "unfinished answer")
	m.removeApproval("live-event")
	m.session.Questions = []continuedQuestion{{CallID: "stable", State: "continued", Questions: request.Request.Questions}}
	m.syncContinuedQuestions()
	_, _, d := m.questionCurrent()
	if d.custom.Value() != "unfinished answer" {
		t.Fatal("timed continuation discarded the unfinished reply")
	}
}
