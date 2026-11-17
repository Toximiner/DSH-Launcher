package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestSessionPickerTitleOnceAndStatusPreserved(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, language := range []string{"ru", "en"} {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {160, 40}} {
			m := pickerFixture(t)
			m.prefs.Language = language
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			activeEnglish.Store(language == "en")
			d := m.sessionPicker
			d.busy = false
			spec := m.sessionPickerSpec()
			if spec.Detail != "" {
				t.Fatalf("idle subtitle repeats heading: %q", spec.Detail)
			}
			frame := ansi.Strip(m.View())
			if strings.Count(frame, spec.Title) != 1 {
				t.Fatalf("%s %v: heading %q count%d\n%s", language, size, spec.Title, strings.Count(frame, spec.Title), frame)
			}
			d.busy = true
			if m.sessionPickerSpec().Detail != tr("Загрузка…") {
				t.Fatal("loading status lost")
			}
			d.busy = false
			d.note = "fixture error"
			if m.sessionPickerSpec().Detail != d.note {
				t.Fatal("error status lost")
			}
			d.note = ""
			d.workspaceID = "w1"
			if m.sessionPickerSpec().Detail != "Project One · /tmp/alpha" {
				t.Fatal("workspace context lost")
			}
			m.closeSessionPicker()
		}
	}
}

func TestPageHeadingsAreNotRepeatedInViewport(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, language := range []string{"ru", "en"} {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {160, 40}} {
			for _, page := range []int{sessionsPage, dshSettingsPage, logPage} {
				m := newModel()
				m.prefs.Language = language
				m.page = page
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				activeEnglish.Store(language == "en")
				title := tr("Настройки dsh")
				if page == sessionsPage {
					title = tr("Сессии dsh")
				}
				if page == logPage {
					title = tr("Журнал клиента · локально")
				}
				frame := ansi.Strip(m.View())
				count := 0
				for _, line := range strings.Split(frame, "\n") {
					if strings.Trim(line, " │") == title {
						count++
					}
				}
				if count != 1 {
					t.Fatalf("%s %v page%d: expected one heading %q, got %d", language, size, page, title, count)
				}
				if page == dshSettingsPage && !strings.Contains(frame, "--url") {
					t.Fatal("connection guidance lost")
				}
			}
		}
	}
}
