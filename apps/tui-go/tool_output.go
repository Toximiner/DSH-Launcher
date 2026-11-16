package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const toolOutputLimit = 128 << 10
const toolObserverLimit = 16

type toolJob struct {
	ID       string `json:"jobId"`
	Owner    string `json:"owner"`
	ToolCall struct {
		ID     string `json:"callId"`
		RootID string `json:"rootCallId"`
	} `json:"toolCall"`
}

type jobOutputFrame struct {
	Type  string `json:"type"`
	From  int64  `json:"from"`
	Next  int64  `json:"next"`
	Lossy bool   `json:"lossy"`
	Job   struct {
		ID     string `json:"id"`
		Owner  string `json:"owner"`
		Status string `json:"status"`
		Output struct {
			Earliest int64 `json:"earliest"`
			Total    int64 `json:"total"`
		} `json:"output"`
	} `json:"job"`
	Chunks []struct {
		At   int64  `json:"at"`
		Text string `json:"text"`
		Gap  bool   `json:"gapBefore"`
	} `json:"chunks"`
}

type toolOutput struct {
	Text                     string
	Cursor                   int64
	Gap, Opened, Done, Stale bool
}

// Offsets count original UTF-8 bytes, before terminal controls are removed.
func (o *toolOutput) accept(f jobOutputFrame, sessionID, jobID string) error {
	next := *o
	switch f.Type {
	case "opened":
		if f.Job.ID != jobID || f.Job.Owner != sessionID || f.From < 0 || f.From > f.Job.Output.Total || f.Job.Output.Earliest < 0 || f.Job.Output.Earliest > f.Job.Output.Total || o.Opened && f.From != o.Cursor {
			return errors.New("invalid job output anchor")
		}
		next.Gap = next.Gap || f.From > 0 && !o.Opened || f.From < f.Job.Output.Earliest
		next.Cursor, next.Opened, next.Stale = f.From, true, false
	case "output":
		if !o.Opened || o.Done || f.Next < o.Cursor {
			return errors.New("invalid job output cursor")
		}
		cursor := o.Cursor
		var text strings.Builder
		gap := f.Lossy
		for _, chunk := range f.Chunks {
			end := chunk.At + int64(len(chunk.Text))
			if chunk.At < 0 || end < chunk.At || end > f.Next {
				return errors.New("invalid job output chunk")
			}
			if end <= cursor {
				continue
			}
			if chunk.At < cursor {
				return errors.New("overlapping job output chunk")
			}
			gap = gap || chunk.Gap || chunk.At > cursor
			text.WriteString(chunk.Text)
			cursor = end
		}
		if cursor != f.Next {
			gap = true
		}
		next.Text += clean(text.String())
		if len(next.Text) > toolOutputLimit {
			next.Text = strings.ToValidUTF8(next.Text[len(next.Text)-toolOutputLimit:], "")
			gap = true
		}
		next.Cursor, next.Gap, next.Stale = f.Next, next.Gap || gap, false
	case "status":
		if !o.Opened || f.Job.ID != jobID || f.Job.Owner != sessionID || (f.Job.Status != "completed" && f.Job.Status != "failed" && f.Job.Status != "killed") {
			return errors.New("invalid job output settlement")
		}
		next.Done, next.Stale = true, false
	default:
		return errors.New("unsupported job output frame")
	}
	*o = next
	return nil
}

type toolOutputObserver struct {
	cancel context.CancelFunc
	ctx    context.Context
	inbox  chan toolOutput
	value  toolOutput
}
type toolOutputUpdate struct {
	epoch    int
	jobID    string
	observer *toolOutputObserver
	value    toolOutput
}

func (s *chatState) registerToolJob(raw json.RawMessage) error {
	var job toolJob
	if json.Unmarshal(raw, &job) != nil {
		return errors.New("неподдерживаемая связь задания с инструментом")
	}
	// Copied history never transfers ownership of a running execution.
	if job.Owner != s.id || job.ToolCall.ID == "" {
		return nil
	}
	if job.ID == "" || len(job.ID) > 1024 || len(job.ToolCall.ID) > 1024 || job.ToolCall.RootID == "" || len(job.ToolCall.RootID) > 1024 {
		return errors.New("неверная связь задания с инструментом")
	}
	if s.jobs == nil {
		s.jobs = make(map[string]toolJob)
	}
	if len(s.jobs) < 128 {
		s.jobs[job.ToolCall.ID] = job
	}
	return nil
}

func (m *model) syncToolOutputs() tea.Cmd {
	s := m.chat
	if s == nil || m.browserPreview {
		return nil
	}
	if s.observers == nil {
		s.observers = make(map[string]*toolOutputObserver)
	}
	wanted := make(map[string]toolJob)
	retained := make(map[string]toolJob)
	for _, message := range s.messages {
		t := message.tool
		if t == nil {
			continue
		}
		job, ok := s.jobs[t.ID]
		root := t.ID
		if t.Nested {
			root = t.RootID
		}
		if !ok || job.ToolCall.RootID != root {
			continue
		}
		retained[t.ID] = job
		if t.State == "running" && len(wanted) < toolObserverLimit {
			wanted[job.ID] = job
		}
	}
	s.jobs = retained
	for id, observer := range s.observers {
		if _, ok := wanted[id]; !ok {
			observer.cancel()
			delete(s.observers, id)
		}
	}
	var cmds []tea.Cmd
	for id := range wanted {
		if s.observers[id] != nil {
			continue
		}
		ctx, cancel := context.WithCancel(s.ctx)
		observer := &toolOutputObserver{ctx: ctx, cancel: cancel, inbox: make(chan toolOutput, 8)}
		s.observers[id] = observer
		go followToolOutput(ctx, m.settingsLogin, s.id, id, observer.inbox)
		cmds = append(cmds, waitToolOutput(s.epoch, id, observer))
	}
	return tea.Batch(cmds...)
}

func waitToolOutput(epoch int, id string, observer *toolOutputObserver) tea.Cmd {
	return func() tea.Msg {
		select {
		case value := <-observer.inbox:
			return toolOutputUpdate{epoch, id, observer, value}
		case <-observer.ctx.Done():
			return nil
		}
	}
}

func (m *model) handleToolOutput(msg toolOutputUpdate) tea.Cmd {
	s := m.chat
	if s == nil || s.epoch != msg.epoch || s.ctx.Err() != nil || s.observers[msg.jobID] != msg.observer {
		return nil
	}
	bottom := m.viewport.AtBottom()
	msg.observer.value = msg.value
	m.rebuild()
	if bottom && m.page == chatPage {
		m.viewport.GotoBottom()
	}
	if msg.value.Done {
		return nil
	}
	return waitToolOutput(s.epoch, msg.jobID, msg.observer)
}

func (m *model) liveToolOutput(t *chatTool) (string, string) {
	if m.chat == nil || t.State != "running" {
		return "", ""
	}
	job, ok := m.chat.jobs[t.ID]
	if !ok {
		return "", ""
	}
	observer := m.chat.observers[job.ID]
	if observer == nil {
		return "", ""
	}
	o := observer.value
	label := "Вывод команды · stdout/stderr"
	if o.Done {
		label += " · поток завершён"
	}
	if o.Stale {
		label += " · нет связи, вывод может быть неполным"
	}
	if o.Gap {
		label += "\n[Часть вывода недоступна или обрезана]"
	}
	if o.Text == "" && !o.Stale && !o.Gap {
		return "", ""
	}
	return o.Text, label
}
