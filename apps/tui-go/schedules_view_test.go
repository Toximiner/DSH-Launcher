package main

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
			m.schedulesMouse(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft})
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

func TestScheduleCompactRowsHaveExactVisibleTargets(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, english := range []bool{false, true} {
		activeEnglish.Store(english)
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 40}} {
			m := benchTestModel(t, "schedules")
			if english {
				m.prefs.Language = "en"
			}
			m.Update(size)
			d := m.workbench
			d.rows = nil
			for i := 0; i < 18; i++ {
				status := scheduleStatuses[i%3+1]
				data := map[string]any{"title": fmt.Sprintf("Task %02d 世界 %s", i, strings.Repeat("long ", 25)), "prompt": fmt.Sprintf("Selected prompt %02d", i), "status": status, "kind": "every", "everySeconds": float64(60), "updatedAt": "revision", "scheduledAt": "2026-11-16T10:00:00Z"}
				d.rows = append(d.rows, benchRow{ID: fmt.Sprint(i), Title: fmt.Sprint(i), Body: scheduleDetails(data), Data: data})
			}
			d.index = 15
			box := m.pluginsPageBox()
			lines, hits := m.schedulesLayout(box)
			if len(lines) != box.height {
				t.Fatalf("schedule rows overflow measured box: %d vs %d", len(lines), box.height)
			}
			if strings.ContainsAny(clean(strings.Join(lines, "\n")), "╭╮╰╯─") {
				t.Fatal("decorative border remained in compact list")
			}
			selects := 0
			selectedVisible := false
			for _, hit := range hits {
				if hit.x < box.x || hit.y < box.y || hit.x+hit.width > box.x+box.width || hit.y+hit.height > box.y+box.height {
					t.Fatalf("hit escapes frame: %+v", hit)
				}
				visible := ansi.Cut(clean(lines[hit.y-box.y]), hit.x-box.x, hit.x-box.x+hit.width)
				switch hit.action {
				case "select":
					selects++
					if hit.height != 1 {
						t.Fatal("schedule card still occupies multiple rows")
					}
					if hit.index == 15 {
						selectedVisible = true
					}
					if !strings.Contains(visible, fmt.Sprintf("Task %02d", hit.index)) {
						t.Fatalf("selection hit misses its row: %+v %q", hit, visible)
					}
				case "toggle":
					want := scheduleStatus(stringValue(d.visible()[hit.index].Data, "status"))
					if visible != want {
						t.Fatalf("toggle hit does not match visible status: %q vs %q", visible, want)
					}
				case "filter":
					if visible == "" {
						t.Fatal("empty filter target")
					}
				case "refresh":
					if visible != tr("Обновить")+" F5" {
						t.Fatalf("refresh target shifted: %q", visible)
					}
				case "create":
					if visible != "+ "+tr("Новая задача")+" Ctrl+N" {
						t.Fatalf("create target shifted: %q", visible)
					}
				}
			}
			if selects < 5 || !selectedVisible {
				t.Fatal("compact list failed to expose multiple rows and current selection")
			}
			if !strings.Contains(clean(strings.Join(lines, "\n")), "Selected prompt 15") {
				t.Fatal("selected detail is missing")
			}
			// Search and the status filter must still select original records, not
			// synthetic headings or a neighboring task. Neither operation may write.
			d.search.SetValue("Selected prompt 15")
			d.scheduleFilter = 1
			lines, hits = m.schedulesLayout(box)
			if len(d.visible()) != 1 || d.visible()[0].ID != "15" {
				t.Fatal("search/filter changed record identity")
			}
			for _, hit := range hits {
				if hit.action == "select" {
					m.schedulesMouse(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft})
					if d.action != nil || d.confirm {
						t.Fatal("row selection mutated a task")
					}
				}
			}
		}
	}
}
