package main

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"strings"
	"testing"
)

func TestSchedulePageFiltersCardsAndFormsFit(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, language := range []string{"ru", "en"} {
		for _, sidebar := range []bool{true, false} {
			m := benchTestModel(t, "schedules")
			m.prefs.Language = language
			m.prefs.HideSidebar = !sidebar
			d := m.workbench
			d.rows = nil
			for i, status := range []string{"active", "paused", "ended"} {
				data := map[string]any{"title": fmt.Sprintf("Task %d 世界", i), "prompt": strings.Repeat("Instruction ", 25), "status": status, "kind": "every", "everySeconds": float64(60), "updatedAt": "rev", "scheduledAt": "2026-11-16T10:00:00Z"}
				d.rows = append(d.rows, benchRow{ID: status, Title: status, Body: scheduleDetails(data), Data: data})
			}
			for _, size := range [][2]int{{80, 24}, {100, 30}, {160, 42}} {
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				check := func() {
					t.Helper()
					v := m.View()
					if lipgloss.Width(v) > m.width-1 || lipgloss.Height(v) > m.height {
						t.Fatalf("overflow %s sidebar=%v size=%v: %dx%d", language, sidebar, size, lipgloss.Width(v), lipgloss.Height(v))
					}
				}
				check()
				m.benchKey(tea.KeyMsg{Type: tea.KeyF2})
				if len(d.visible()) != 1 || d.visible()[0].ID != "active" {
					t.Fatal("status filter lost")
				}
				m.benchPrepare(tea.KeyF3)
				check()
				m.scheduleFormKey(tea.KeyMsg{Type: tea.KeyEsc})
				m.benchPrepare(tea.KeyCtrlN)
				check()
				m.scheduleFormKey(tea.KeyMsg{Type: tea.KeyEsc})
				d.scheduleFilter = 0
				d.document = "active"
				d.fileBody = "Delivery history"
				check()
				d.document = ""
			}
		}
	}
}
func TestScheduleFormReviewCancellationAndRevision(t *testing.T) {
	m := benchTestModel(t, "schedules")
	d := m.workbench
	m.benchPrepare(tea.KeyCtrlN)
	f := d.scheduleForm
	f.title.SetValue("  Daily report  ")
	f.prompt.SetValue("Summarize changes")
	f.minutes.SetValue("0")
	m.scheduleFormKey(tea.KeyMsg{Type: tea.KeyF5})
	if d.confirm {
		t.Fatal("invalid interval was accepted")
	}
	f.minutes.SetValue("15")
	f.every = true
	m.scheduleFormKey(tea.KeyMsg{Type: tea.KeyF5, Paste: true})
	if d.confirm {
		t.Fatal("pasted F5 submitted form")
	}
	m.scheduleFormKey(tea.KeyMsg{Type: tea.KeyF5})
	req := objectValue(d.action.Args["request"])
	if !d.confirm || req["title"] != "Daily report" || objectValue(req["timing"])["every_seconds"] != 900 {
		t.Fatal("reviewed request is wrong")
	}
	m.benchKey(tea.KeyMsg{Type: tea.KeyEsc})
	if d.scheduleForm != f || !d.editing || d.confirm {
		t.Fatal("confirmation cancellation lost draft")
	}
	m.scheduleFormKey(tea.KeyMsg{Type: tea.KeyEsc})
	if d.scheduleForm != nil || d.action != nil {
		t.Fatal("cancel did not discard draft")
	}
	m.benchPrepare(tea.KeyF3)
	d.scheduleForm.title.SetValue("Updated title")
	m.scheduleFormKey(tea.KeyMsg{Type: tea.KeyF5})
	req = objectValue(d.action.Args["request"])
	patch := objectValue(req["patch"])
	if req["expectedUpdatedAt"] != "revision-1" || patch["title"] != "Updated title" || patch["timing"] != nil {
		t.Fatal("edit lost revision or changed timing")
	}
}
func TestScheduleHistoryAndMouseAreReadOnlyUntilConfirmed(t *testing.T) {
	m := benchTestModel(t, "schedules")
	d := m.workbench
	box := m.pluginsPageBox()
	_, hits := m.schedulesLayout(box)
	for _, hit := range hits {
		if hit.action == "select" {
			m.schedulesMouse(tea.MouseMsg{X: hit.x, Y: hit.y + 1, Button: tea.MouseButtonLeft})
			if d.confirm || d.action != nil {
				t.Fatal("selecting card wrote state")
			}
		}
	}
	for _, hit := range hits {
		if hit.action == "toggle" {
			m.schedulesMouse(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft})
			if !d.confirm || d.action.Method != "schedule/edit" {
				t.Fatal("switch skipped confirmation")
			}
		}
	}
	d.confirm = false
	d.action = nil
	d.document = "r"
	m.handleBench(benchResult{dialog: d, serial: d.serial, document: "History", fileEOF: true})
	if len(d.rows) != 1 || d.rows[0].ID != "r" {
		t.Fatal("history discarded task snapshot")
	}
	body := scheduleHistory(map[string]any{"records": []any{map[string]any{"targetSessionId": "target", "deliveredAt": "2026-11-16T10:00:00Z", "prompt": "Instruction"}}, "earlierRecordsPruned": true})
	if !strings.Contains(body, "target") || !strings.Contains(body, "Instruction") || strings.Contains(body, "targetSessionId") {
		t.Fatal("history is not human-readable")
	}
}

func TestScheduleHistoryDoesNotRequireWritableRevision(t *testing.T) {
	m := benchTestModel(t, "schedules")
	delete(m.workbench.rows[0].Data, "updatedAt")
	if m.benchPrepare(tea.KeyEnter) == nil || m.workbench.document != "r" || m.workbench.confirm {
		t.Fatal("read-only history was blocked by missing revision")
	}
}
