package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func tabFixture(count int) *model {
	m := newModel()
	for i := 0; i < count; i++ {
		next := &sessionView{ID: fmt.Sprintf("session-%02d", i), Title: fmt.Sprintf("Task %02d 世界", i), Running: i%2 == 0}
		m.tabSessionChanged(next)
		m.session = next
	}
	return m
}

func TestSessionTabsRevealActiveAndShareCellGeometry(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, english := range []bool{false, true} {
		activeEnglish.Store(english)
		for _, width := range []int{80, 100, 160} {
			m := tabFixture(20)
			m.width = width
			layout := m.sessionTabLayout()
			if lipgloss.Width(layout.view) > width {
				t.Fatalf("rail overflow at %d", width)
			}
			last := layout.hits[len(layout.hits)-1]
			if !last.new {
				t.Fatal("new session button must follow the tabs")
			}
			found := false
			end := 0
			for _, hit := range layout.hits {
				if hit.x < end || hit.y != sessionTabRow || hit.x+hit.width > width {
					t.Fatal("overlapping or misplaced hit")
				}
				end = hit.x + hit.width
				if hit.id == m.session.ID {
					found = true
				}
			}
			if !found {
				t.Fatalf("active tab hidden at width %d", width)
			}
			if !strings.Contains(clean(layout.view), "Task 19") {
				t.Fatal("active title absent")
			}
			// Every clickable cell, including running markers, dispatches its own tab.
			for _, hit := range layout.hits {
				if hit.new {
					continue
				}
				for x := hit.x; x < hit.x+hit.width; x++ {
					m.settingsLogin = "http://127.0.0.1:1"
					m.sessionBusy = true
					m.sessionQueue = nil
					_, handled := m.tabRailMouse(tea.MouseMsg{X: x, Y: sessionTabRow, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
					if !handled {
						t.Fatal("visible tab did not handle click")
					}
					if hit.id != m.activeTabID() && (len(m.sessionQueue) != 1 || m.sessionQueue[0].action != "connect" || m.sessionQueue[0].value != hit.id) {
						t.Fatal("tab click did not connect to its visible session")
					}
				}
			}
		}
	}
}

func TestSessionTabsCatalogDoesNotOpenReorderOrRemoveTabs(t *testing.T) {
	m := tabFixture(3)
	updated := sessionItem{ID: "session-00", Running: true}
	updated.Projections.Values.Title = "Updated"
	m.tabRefreshItems([]sessionItem{{ID: "unopened"}, updated})
	// A later empty/filtered page leaves existing metadata and ordering intact.
	m.tabRefreshItems(nil)
	tabs := m.tabList()
	if len(tabs) != 3 || tabs[0].ID != "session-00" || tabs[2].ID != "session-02" {
		t.Fatal("catalog changed open tabs")
	}
	if tabs[0].Projections.Values.Title != "Updated" || !tabs[0].Running {
		t.Fatal("catalog metadata not refreshed")
	}
	m.sessionItems = []sessionItem{{ID: "unopened"}}
	if len(m.tabList()) != 3 {
		t.Fatal("search results replaced tabs")
	}
	m.session.Title = "Live title"
	if m.tabList()[2].Projections.Values.Title != "Live title" {
		t.Fatal("stale catalog replaced active title")
	}
}

func TestSessionTabsKeepDraftsOnlyAcrossAcceptedSwitches(t *testing.T) {
	m := tabFixture(1)
	m.input.SetValue("draft A 世界")
	b := &sessionView{ID: "b", Title: "Second"}
	m.tabSessionChanged(b)
	m.session = b
	if m.input.Value() != "" {
		t.Fatal("new session inherited previous draft")
	}
	m.input.SetValue("draft B")
	a := &sessionView{ID: "session-00", Title: "First"}
	m.tabSessionChanged(a)
	m.session = a
	if m.input.Value() != "draft A 世界" {
		t.Fatal("previous draft not restored")
	}
	m.input.SetValue("edited A")
	m.tabSessionChanged(a)
	if m.input.Value() != "edited A" {
		t.Fatal("polling reset live draft")
	}
	m.tabSessionChanged(b)
	m.session = b
	if m.input.Value() != "draft B" {
		t.Fatal("second draft not restored")
	}
	m.tabSessionChanged(nil)
	if m.input.Value() != "draft B" {
		t.Fatal("failed result reset draft")
	}
}

func TestSessionTabsBootstrapRetainsActiveWithoutCatalog(t *testing.T) {
	m := newModel()
	m.session = &sessionView{ID: "existing", Title: "Current"}
	m.sessionItems = []sessionItem{{ID: "unopened"}}
	if tabs := m.tabList(); len(tabs) != 1 || tabs[0].ID != "existing" {
		t.Fatal("active missing without catalog")
	}
	next := &sessionView{ID: "next"}
	m.tabSessionChanged(next)
	m.session = next
	if len(m.tabList()) != 2 {
		t.Fatal("initial active was not retained")
	}
}

func TestSessionCreationShortcutRespectsEditorsAndPages(t *testing.T) {
	m := tabFixture(2)
	key := tea.KeyMsg{Type: tea.KeyCtrlN}
	m.page = sessionsPage
	if _, handled := m.dispatchKeyAction(key); handled {
		t.Fatal("list CtrlN intercepted")
	}
	m.page = chatPage
	m.composer = &composeDialog{visible: true}
	if _, handled := m.dispatchKeyAction(key); handled {
		t.Fatal("editor CtrlN intercepted")
	}
	m.composer = nil
	m.palette = &paletteState{open: true}
	if _, handled := m.dispatchKeyAction(key); handled {
		t.Fatal("palette CtrlN intercepted")
	}
	if _, handled := m.tabRailMouse(tea.MouseMsg{X: 1, Y: sessionTabRow, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}); handled {
		t.Fatal("rail accepted mouse behind overlay")
	}
	m.palette = nil
	m.approvals = &approvalState{visible: true}
	if _, handled := m.dispatchKeyAction(key); handled {
		t.Fatal("approval CtrlN intercepted")
	}
	m.approvals = nil
	m.settingsLogin = "http://127.0.0.1:1"
	cmd, handled := m.dispatchKeyAction(key)
	if !handled || cmd == nil || m.sessionAction != "new" {
		t.Fatal("base chat CtrlN unavailable")
	}
}

func TestSessionTabsNewButtonFollowsLastTab(t *testing.T) {
	for _, count := range []int{0, 1, 3} {
		m := tabFixture(count)
		m.width = 160
		layout := m.sessionTabLayout()
		last := layout.hits[len(layout.hits)-1]
		if !last.new {
			t.Fatal("new session button is not the last tab")
		}
		if count == 0 {
			if last.x != 0 {
				t.Fatal("empty tab rail has leading padding")
			}
		} else {
			previous := layout.hits[len(layout.hits)-2]
			if last.x != previous.x+previous.width {
				t.Fatal("new session button is separated from the last tab")
			}
		}
		if last.x+last.width >= m.width-1 {
			t.Fatal("short tab rail is stretched to the right edge")
		}
	}
}
