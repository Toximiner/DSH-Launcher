package main

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func tabPresent(m *model, id string) bool {
	for _, tab := range m.tabList() {
		if tab.ID == id {
			return true
		}
	}
	return false
}

func TestSessionTabCloseInactiveKeepsBackendCatalogAndDraft(t *testing.T) {
	m := tabFixture(3)
	m.input.SetValue("current draft")
	m.tabState.drafts["session-00"] = "closed draft 世界"
	m.sessionItems = []sessionItem{{ID: "session-00"}, {ID: "session-02"}}
	if cmd := m.closeTab("session-00"); cmd != nil {
		t.Fatal("inactive close scheduled an operation")
	}
	if tabPresent(m, "session-00") || m.activeTabID() != "session-02" || m.input.Value() != "current draft" {
		t.Fatal("inactive close changed active session or draft")
	}
	if len(m.sessionItems) != 2 || m.tabState.drafts["session-00"] != "closed draft 世界" {
		t.Fatal("close deleted catalog entry or draft")
	}
	m.tabRefreshItems(m.sessionItems)
	if tabPresent(m, "session-00") {
		t.Fatal("catalog refresh reopened a closed tab")
	}
}

func TestSessionTabCloseActiveConnectsLeftAndRestoresDrafts(t *testing.T) {
	m := tabFixture(3)
	m.settingsLogin = "http://127.0.0.1:1"
	m.input.SetValue("third draft")
	m.tabState.drafts["session-01"] = "second draft"
	if m.closeTab("session-02") == nil || m.sessionAction != "connect" || !m.sessionBusy {
		t.Fatal("active close did not connect existing neighbor")
	}
	if m.session != nil || tabPresent(m, "session-02") || len(m.tabList()) != 2 {
		t.Fatal("closed active session remained in rail")
	}
	m.Update(sessionResult{sequence: m.sessionSequence, view: &sessionView{ID: "session-01", Title: "Second"}})
	defer m.stopChat()
	if m.activeTabID() != "session-01" || m.input.Value() != "second draft" || tabPresent(m, "session-02") {
		t.Fatal("accepted neighbor did not restore draft or reopened closed tab")
	}
	if m.tabState.drafts["session-02"] != "third draft" {
		t.Fatal("closed draft lost")
	}
}

func TestSessionTabCloseFinalIsRefusedAndHasNoCloseHit(t *testing.T) {
	m := tabFixture(1)
	m.width = 80
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.chat = &chatState{id: m.session.ID, ctx: ctx, cancel: cancel, epoch: m.chatEpoch}
	m.input.SetValue("unsent final draft")
	oldSequence := m.sessionSequence
	if cmd := m.closeTab("session-00"); cmd != nil {
		t.Fatal("last close scheduled an operation")
	}
	if ctx.Err() != nil || m.session == nil || m.chat == nil || len(m.tabList()) != 1 || m.sessionSequence != oldSequence || m.input.Value() != "unsent final draft" {
		t.Fatal("final close discarded session, readers or draft")
	}
	for _, hit := range m.sessionTabLayout().hits {
		if hit.close {
			t.Fatal("last tab exposes an unusable close button")
		}
	}
	// Permanent deletion remains separate and may legitimately leave home empty.
	m.tabForgetDeletedSession("session-00")
	if m.session != nil || len(m.tabList()) != 0 || ctx.Err() == nil {
		t.Fatal("explicit deletion could not remove final session")
	}
}

func TestSessionTabCloseRespectsPendingOperationOwnership(t *testing.T) {
	for _, action := range []string{"connect", "new", "plan", "list"} {
		m := tabFixture(2)
		m.sessionBusy, m.sessionAction = true, action
		m.closeTab("session-01")
		if m.activeTabID() != "session-01" || !tabPresent(m, "session-01") || !m.sessionBusy {
			t.Fatalf("close invalidated %s ownership", action)
		}
		m.closeTab("session-00")
		if tabPresent(m, "session-00") || !m.sessionBusy {
			t.Fatal("inactive close affected operation")
		}
	}
	m := tabFixture(1)
	m.sessionQueue = []sessionAction{{action: "new"}}
	m.closeTab("session-00")
	if m.session == nil || len(m.sessionQueue) != 1 {
		t.Fatal("close discarded queued action")
	}
}

func TestSessionTabCloseMouseUsesExactCellsAcrossOverflow(t *testing.T) {
	for _, width := range []int{80, 100, 160} {
		sample := tabFixture(20)
		sample.width = width
		closes := 0
		for _, hit := range sample.sessionTabLayout().hits {
			if !hit.close {
				continue
			}
			closes++
			for x := hit.x; x < hit.x+hit.width; x++ {
				m := tabFixture(20)
				m.width = width
				m.settingsLogin = "http://127.0.0.1:1"
				_, handled := m.tabRailMouse(tea.MouseMsg{X: x, Y: sessionTabRow, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				if !handled || tabPresent(m, hit.id) || len(m.tabList()) != 19 {
					t.Fatalf("close cell %d/%d did not close its exact visible tab", width, x)
				}
				if m.sessionAction == "new" {
					t.Fatal("close click created backend session")
				}
			}
		}
		if closes == 0 {
			t.Fatal("overflow rail has no close buttons")
		}
	}
	m := tabFixture(2)
	m.width = 80
	m.chrome.dialog = "settings"
	for _, hit := range m.sessionTabLayout().hits {
		if hit.close {
			_, handled := m.tabRailMouse(tea.MouseMsg{X: hit.x, Y: sessionTabRow, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if handled || !tabPresent(m, hit.id) {
				t.Fatal("close clicked through modal")
			}
		}
	}
}

func TestSessionTabForgetDeletedInvalidatesLocalState(t *testing.T) {
	m := tabFixture(2)
	m.tabState.drafts["session-01"] = "deleted draft"
	m.sessionBusy, m.sessionAction = true, "refresh"
	old := m.sessionSequence
	m.tabForgetDeletedSession("session-01")
	if m.session != nil || tabPresent(m, "session-01") || tabPresent(m, "missing") || len(m.tabList()) != 1 {
		t.Fatal("deletion did not detach active and forget tab")
	}
	if _, exists := m.tabState.drafts["session-01"]; exists {
		t.Fatal("deleted draft retained")
	}
	m.Update(sessionResult{sequence: old, view: &sessionView{ID: "session-01"}})
	if m.session != nil {
		t.Fatal("stale result reopened deleted session")
	}
}
