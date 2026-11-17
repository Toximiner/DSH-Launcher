package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	questionKey(m, tea.KeySpace)
	questionKey(m, tea.KeyRight)
	questionKey(m, tea.KeyTab)
	questionKey(m, tea.KeySpace)
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
	questionKey(m, tea.KeySpace)
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
	questionKey(m, tea.KeySpace)
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
	for _, width := range []int{80, 100} {
		m := questionModel(t, []userQuestion{{ID: "q", Question: "Question?", Options: []questionOption{{Label: "One", Description: strings.Repeat("detail ", 40)}, {Label: "Two"}}}})
		var batch questionAnswers
		m.approvals.reply = func(_ string, value any) error { batch = value.(questionAnswers); return nil }
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
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
		m.mouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 3 + hit.start - m.approvals.viewport.YOffset, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
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

func TestQuestionFlatFrameGeometryAndDeferredCustomDraft(t *testing.T) {
	for _, language := range []string{"ru", "en"} {
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 40}} {
			t.Run(fmt.Sprintf("%s/%dx%d", language, size.Width, size.Height), func(t *testing.T) {
				m := questionModel(t, []userQuestion{{ID: "q", Header: "Header", Question: "Question geometry?", Detail: strings.Repeat("long question detail\n", 40) + "DETAIL_LAST", Options: []questionOption{{Label: "Choice 世界", Description: "Choice detail"}}}})
				m.prefs.Language = language
				m.applyClientAppearance()
				defer activeEnglish.Store(false)
				m.input.SetValue("chat draft")
				m.Update(size)
				box := m.approvalBox()
				flat := m.questionView(strings.Repeat(strings.Repeat(" ", size.Width-1)+"\n", size.Height-1))
				if lipgloss.Width(flat) > size.Width || lipgloss.Height(flat) > size.Height {
					t.Fatal("flat question frame overflowed terminal")
				}
				plain := clean(flat)
				if strings.ContainsAny(plain, "╭╮╰╯") {
					t.Fatal("question retained rounded frame")
				}
				lines := strings.Split(plain, "\n")
				buttons := box.y + box.height - 3
				if !strings.Contains(lines[buttons], strings.Trim(tr("[Пропуск]"), "[]")) || !strings.Contains(lines[buttons], strings.Trim(tr("[Ответить]"), "[]")) {
					t.Fatal("flat question controls diverged from mouse row")
				}
				m.approvals.viewport.GotoBottom()
				if !strings.Contains(clean(m.View()), "DETAIL_LAST") {
					t.Fatal("long question detail was clipped instead of scrollable")
				}
				m.questionFocus(1)
				m.questionKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("custom draft 世界"), Paste: true})
				_, _, draft := m.questionCurrent()
				close := m.decisionClose(box)
				if cmd := m.questionMouse(tea.MouseMsg{X: close.x, Y: close.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}); cmd != nil || m.approvals.visible || m.approvals.busy {
					t.Fatal("closing question submitted an answer")
				}
				if draft.custom.Value() != "custom draft 世界" || m.input.Value() != "chat draft" || len(m.approvals.requests) != 1 {
					t.Fatal("deferring question lost draft or pending request")
				}
				m.approvalKey(tea.KeyMsg{Type: tea.KeyF7})
				if !m.approvals.visible || draft.custom.Value() != "custom draft 世界" {
					t.Fatal("reopening flat question lost custom response")
				}
			})
		}
	}
}

func TestQuestionEnterSingleChoiceSubmitsExactlyOnce(t *testing.T) {
	m := questionModel(t, []userQuestion{{ID: "q", Question: "Choose?", Options: []questionOption{{Label: "Yes"}, {Label: "No"}}}})
	m.input.SetValue("preserved chat draft")
	calls := 0
	var batch questionAnswers
	m.approvals.reply = func(_ string, value any) error { calls++; batch = value.(questionAnswers); return nil }
	if cmd := questionKey(m, tea.KeyEnter); cmd != nil || m.approvals.busy {
		t.Fatal("Enter without an explicit focus sent a default answer")
	}
	questionKey(m, tea.KeyTab)
	cmd := questionKey(m, tea.KeyEnter)
	if cmd == nil || !m.approvals.busy || calls != 0 {
		t.Fatal("focused single-choice Enter did not prepare explicit batch")
	}
	if next := questionKey(m, tea.KeyEnter); next != nil {
		t.Fatal("repeated Enter prepared another reply")
	}
	m.Update(cmd())
	if calls != 1 || len(batch.Answers) != 1 || len(batch.Answers[0].Selected) != 1 || batch.Answers[0].Selected[0] != "Yes" || m.input.Value() != "preserved chat draft" {
		t.Fatal("Enter submitted wrong choice, duplicated reply or changed chat draft")
	}
}

func TestQuestionEnterAdvancesOnlyToUnansweredAndPreservesOtherDrafts(t *testing.T) {
	m := questionModel(t, []userQuestion{{ID: "first", Question: "First?", Options: []questionOption{{Label: "A"}}}, {ID: "already", Question: "Already answered?"}, {ID: "last", Question: "Last?", Options: []questionOption{{Label: "B"}}}})
	var batch questionAnswers
	m.approvals.reply = func(_ string, value any) error { batch = value.(questionAnswers); return nil }
	f, _, _ := m.questionCurrent()
	f.drafts[1].custom.SetValue("retained custom answer")
	questionKey(m, tea.KeyTab)
	if cmd := questionKey(m, tea.KeyEnter); cmd != nil || f.index != 2 || f.cursor != -1 || m.approvals.busy {
		t.Fatal("Enter did not skip answered draft and advance to next unanswered question")
	}
	if f.drafts[1].custom.Value() != "retained custom answer" || !f.drafts[0].selected[0] {
		t.Fatal("advancing lost another answer")
	}
	questionKey(m, tea.KeyTab)
	cmd := questionKey(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("last complete choice did not submit batch")
	}
	m.Update(cmd())
	if len(batch.Answers) != 3 || batch.Answers[1].Custom != "retained custom answer" || batch.Answers[0].Selected[0] != "A" || batch.Answers[2].Selected[0] != "B" {
		t.Fatal("completed batch lost or changed an answer")
	}
}

func TestQuestionEnterConfirmsMultiSelectionWithoutTogglingItAway(t *testing.T) {
	for _, preselect := range []bool{false, true} {
		m := questionModel(t, []userQuestion{{ID: "multi", Question: "Choose checks", MultiSelect: true, Options: []questionOption{{Label: "Unit"}, {Label: "PTY"}}}})
		var batch questionAnswers
		m.approvals.reply = func(_ string, value any) error { batch = value.(questionAnswers); return nil }
		questionKey(m, tea.KeyTab)
		if preselect {
			questionKey(m, tea.KeySpace)
			questionKey(m, tea.KeyDown)
			questionKey(m, tea.KeySpace)
		}
		cmd := questionKey(m, tea.KeyEnter)
		if cmd == nil {
			t.Fatal("explicit multiselect Enter did not confirm current choices")
		}
		m.Update(cmd())
		want := 1
		if preselect {
			want = 2
		}
		if len(batch.Answers) != 1 || len(batch.Answers[0].Selected) != want || batch.Answers[0].Selected[0] != "Unit" {
			t.Fatal("Enter removed current selection or failed to choose focused option")
		}
	}
}
