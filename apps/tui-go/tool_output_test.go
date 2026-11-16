package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func outputFrame(t *testing.T, raw string) jobOutputFrame {
	t.Helper()
	var frame jobOutputFrame
	if err := json.Unmarshal([]byte(raw), &frame); err != nil {
		t.Fatal(err)
	}
	return frame
}

func TestToolOutputResumeGapsAndBounds(t *testing.T) {
	o := toolOutput{}
	accept := func(raw string) {
		t.Helper()
		if err := o.accept(outputFrame(t, raw), "s", "j"); err != nil {
			t.Fatal(err)
		}
	}
	accept(`{"type":"opened","job":{"id":"j","owner":"s","output":{"earliest":0,"total":4}},"from":0}`)
	accept(`{"type":"output","chunks":[{"at":0,"text":"я\n"}],"next":3}`)
	accept(`{"type":"output","chunks":[{"at":0,"text":"я\n"}],"next":3}`)
	if o.Text != "я\n" {
		t.Fatal("duplicate output was appended")
	}
	// A reconnect resumes original UTF-8 byte offsets, not rune counts.
	accept(`{"type":"opened","job":{"id":"j","owner":"s","output":{"earliest":3,"total":10}},"from":3}`)
	accept(`{"type":"output","chunks":[{"at":6,"text":"tail","gapBefore":true}],"next":10,"lossy":true}`)
	if o.Text != "я\ntail" || !o.Gap || o.Cursor != 10 {
		t.Fatal("resumed output or loss marker is incorrect")
	}
	before := o
	if o.accept(outputFrame(t, `{"type":"output","chunks":[{"at":9,"text":"overlap"}],"next":16}`), "s", "j") == nil || o != before {
		t.Fatal("invalid frame partially changed output")
	}
	large, _ := json.Marshal(map[string]any{"type": "output", "chunks": []any{map[string]any{"at": 10, "text": strings.Repeat("я", toolOutputLimit) + "\u001b[2J"}}, "next": 10 + 2*toolOutputLimit + 5})
	accept(string(large))
	if len(o.Text) > toolOutputLimit || strings.Contains(o.Text, "\x1b") || !o.Gap {
		t.Fatal("live output was unbounded or retained terminal controls")
	}
	accept(`{"type":"status","job":{"id":"j","owner":"s","status":"completed"}}`)
	if !o.Done {
		t.Fatal("terminal output was not closed")
	}
}

func TestToolOutputRejectsWrongOwnershipAndOrdering(t *testing.T) {
	for _, raw := range []string{
		`{"type":"output","next":0}`,
		`{"type":"status","job":{"id":"j","owner":"s","status":"completed"}}`,
		`{"type":"opened","job":{"id":"j","owner":"other","output":{"total":0}},"from":0}`,
		`{"type":"opened","job":{"id":"wrong","owner":"s","output":{"total":0}},"from":0}`,
		`{"type":"opened","job":{"id":"j","owner":"s","output":{"total":0}},"from":1}`,
	} {
		o := toolOutput{}
		if o.accept(outputFrame(t, raw), "s", "j") == nil {
			t.Fatalf("accepted invalid output: %s", raw)
		}
	}
}

func toolJobData(call, root string) any {
	return map[string]any{"jobId": "j", "owner": "s", "toolCall": map[string]any{"callId": call, "rootCallId": root}}
}

func TestToolOutputCardsAndStaleUpdates(t *testing.T) {
	for _, size := range [][2]int{{120, 42}, {100, 30}, {80, 24}} {
		m := chatModel(t)
		applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
		toolEvent(t, m, 0, "tool/call", toolCall("root", "run_code", 1))
		toolEvent(t, m, 1, "tool/ptc-dispatch-start", nestedEvent("child", "root", "root", "bash", false))
		toolEvent(t, m, 2, "job/registered", toolJobData("child", "root"))
		ctx, cancel := context.WithCancel(m.chat.ctx)
		observer := &toolOutputObserver{ctx: ctx, cancel: cancel, inbox: make(chan toolOutput, 1)}
		m.chat.observers = map[string]*toolOutputObserver{"j": observer}
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.input.SetValue("preserved draft")
		m.handleToolOutput(toolOutputUpdate{m.chat.epoch, "j", observer, toolOutput{Text: "early output\n", Opened: true, Stale: true, Gap: true}})
		m.chat.expandedTools = map[string]bool{"child": true}
		view := clean(m.chatView())
		compact := strings.Join(strings.Fields(view), "")
		if !strings.Contains(view, "early output") || !strings.Contains(compact, "нетсвязи,выводможетбытьнеполным") || !strings.Contains(compact, "Частьвывода") {
			t.Fatalf("live preview or uncertainty was hidden at %v: %s", size, view)
		}
		if lipgloss.Width(m.View()) > m.width-1 || lipgloss.Height(m.View()) > m.height {
			t.Fatal("live output overflowed terminal")
		}
		m.handleToolOutput(toolOutputUpdate{m.chat.epoch - 1, "j", observer, toolOutput{Text: "old session"}})
		if observer.value.Text != "early output\n" {
			t.Fatal("old epoch changed output")
		}
		toolEvent(t, m, 3, "tool/ptc-dispatch", nestedEvent("child", "root", "root", "bash", false))
		m.syncToolOutputs()
		m.handleToolOutput(toolOutputUpdate{m.chat.epoch, "j", observer, toolOutput{Text: "late output"}})
		view = clean(m.chatView())
		if strings.Contains(view, "early output") || strings.Contains(view, "late output") || !strings.Contains(view, "nested output") || m.input.Value() != "preserved draft" {
			t.Fatal("final result did not supersede live preview safely")
		}
		if ctx.Err() == nil {
			t.Fatal("finished card kept output observer alive")
		}
	}
}

func TestToolJobBindingsRestoreAndFenceCopiedHistory(t *testing.T) {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":1,"records":[{"type":"event","event":{"seq":0,"type":"tool/call","data":{"callId":"a","name":"bash","arguments":"{}"}}},{"type":"event","event":{"seq":1,"type":"job/registered","data":{"jobId":"j","owner":"s","toolCall":{"callId":"a","rootCallId":"a"}}}}]}`)
	if m.chat.jobs["a"].ID != "j" {
		t.Fatal("snapshot lost durable job association")
	}
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":0,"records":[{"type":"event","event":{"seq":0,"type":"job/registered","data":{"jobId":"j","owner":"original-session","toolCall":{"callId":"a","rootCallId":"a"}}}}]}`)
	if len(m.chat.jobs) != 0 {
		t.Fatal("copied history transferred execution ownership")
	}
}
