package main

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func tabRestoreFixture(saved savedSessionTabs) (*model, *[]savedSessionTabs, tabRestoreResult) {
	m := newModel()
	m.settingsLogin = "http://127.0.0.1:1"
	writes := []savedSessionTabs{}
	m.configureTabPersistence(func() (savedSessionTabs, error) { return saved, nil }, func(value savedSessionTabs) error { writes = append(writes, value); return nil }, true)
	m.tabPersistence.loading, m.tabPersistence.started = true, true
	return m, &writes, tabRestoreResult{owner: m.tabPersistence, login: m.settingsLogin, sequence: m.sessionSequence, saved: saved}
}
func tabCatalog(ids ...string) []sessionItem {
	var items []sessionItem
	for _, id := range ids {
		item := sessionItem{ID: id}
		item.Projections.Values.Title = "title " + id
		items = append(items, item)
	}
	return items
}
func TestTabRestoreValidatesFreshCatalogRetainsOrderAndAcceptedActive(t *testing.T) {
	saved := savedSessionTabs{Version: 1, IDs: []string{"b", "missing", "a"}, ActiveID: "b"}
	m, writes, result := tabRestoreFixture(saved)
	result.items = tabCatalog("a", "unopened", "b")
	cmd := m.handleTabRestore(result)
	if cmd == nil || m.sessionAction != "connect" || m.session != nil {
		t.Fatal("restore did not read existing active session")
	}
	if len(*writes) != 0 {
		t.Fatal("unaccepted active connection persisted")
	}
	if got := m.persistedTabs().IDs; !reflect.DeepEqual(got, []string{"b", "a"}) {
		t.Fatalf("restored order/catalog filter: %v", got)
	}
	m.Update(sessionResult{sequence: m.sessionSequence, view: &sessionView{ID: "b", Title: "secret title"}})
	defer m.stopChat()
	if len(*writes) != 1 || !reflect.DeepEqual((*writes)[0], savedSessionTabs{Version: 1, IDs: []string{"b", "a"}, ActiveID: "b"}) {
		t.Fatalf("accepted/pruned snapshot: %+v", *writes)
	}
	m.input.SetValue("private draft")
	m.session.Title = "changed title"
	m.Update(sessionResult{sequence: m.sessionSequence, view: m.session})
	if len(*writes) != 1 {
		t.Fatal("metadata poll rewrote unchanged IDs")
	}
	encoded, _ := json.Marshal((*writes)[0])
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "draft") || strings.Contains(string(encoded), "title") {
		t.Fatal("navigation snapshot contains private content")
	}
}
func TestTabRestoreMissingActiveFallsBackWithinSavedTabs(t *testing.T) {
	m, _, result := tabRestoreFixture(savedSessionTabs{Version: 1, IDs: []string{"gone", "a", "b"}, ActiveID: "gone"})
	result.items = tabCatalog("b", "a")
	m.handleTabRestore(result)
	if m.sessionAction != "connect" || len(m.tabList()) != 2 || m.tabList()[0].ID != "a" {
		t.Fatal("missing active did not retain saved order")
	}
}
func TestTabRestoreExplicitFlagsWinAndAreConsumedOnce(t *testing.T) {
	for _, action := range []string{"connect", "new"} {
		m, _, result := tabRestoreFixture(savedSessionTabs{Version: 1, IDs: []string{"saved"}, ActiveID: "saved"})
		result.items = tabCatalog("saved")
		result.explicitAction = action
		result.explicitID = "requested"
		m.handleTabRestore(result)
		if m.sessionAction != action {
			t.Fatal("saved active overrode explicit CLI request")
		}
	}
	m := newModel()
	m.settingsLogin = "http://127.0.0.1:1"
	m.createSession = true
	m.initialSession = "old"
	m.configureTabPersistence(func() (savedSessionTabs, error) { return savedSessionTabs{}, errors.New("offline") }, nil, true)
	cmd := m.startSessionTabs()
	if cmd == nil || m.createSession || m.initialSession != "" {
		t.Fatal("startup flags were not captured and consumed")
	}
	result := cmd().(tabRestoreResult)
	if result.explicitAction != "new" || result.err == nil {
		t.Fatal("explicit startup request missing on read failure")
	}
	m.handleTabRestore(result)
	if m.sessionAction != "new" || m.tabPersistence.ready {
		t.Fatal("explicit new did not retain precedence without saving failed read")
	}
}
func TestTabRestoreReadFailurePreservesDiskAndNeverAutoCreates(t *testing.T) {
	for _, saved := range []savedSessionTabs{{}, {Version: 1, IDs: []string{"saved"}, ActiveID: "saved"}} {
		m, writes, result := tabRestoreFixture(saved)
		result.err = errors.New("catalog connection failed")
		m.handleTabRestore(result)
		if m.sessionBusy || m.session != nil || len(*writes) != 0 || m.tabPersistence.ready {
			t.Fatal("failed restore mutated sessions or saved tabs")
		}
		if len(m.lines) != 0 || len(m.events) != 2 || m.preferencesNote == "" {
			t.Fatal("failure should be visible in status/log, not transcript")
		}
		m.tabSessionChanged(&sessionView{ID: "manual"})
		m.session = &sessionView{ID: "manual"}
		m.persistSessionTabs()
		if len(*writes) != 0 {
			t.Fatal("failed validation overwrote a previous good snapshot")
		}
	}
}
func TestTabRestoreLateResultCannotOverrideUserActionOrNamespace(t *testing.T) {
	saved := savedSessionTabs{Version: 1, IDs: []string{"old"}, ActiveID: "old"}
	m, writes, result := tabRestoreFixture(saved)
	result.items = tabCatalog("old", "manual")
	m.interruptTabRestore()
	m.tabSessionChanged(&sessionView{ID: "manual"})
	m.session = &sessionView{ID: "manual"}
	m.persistSessionTabs()
	if len(*writes) != 0 {
		t.Fatal("unvalidated startup state written")
	}
	m.handleTabRestore(result)
	if m.activeTabID() != "manual" || len(m.tabList()) != 1 || len(*writes) != 1 || (*writes)[0].ActiveID != "manual" {
		t.Fatal("late restore overrode user navigation or lost accepted save")
	}
	// A request issued without a UI event also owns a newer session sequence.
	queued, queuedWrites, late := tabRestoreFixture(saved)
	late.items = tabCatalog("old")
	queued.sessionSequence++
	queued.handleTabRestore(late)
	if queued.sessionBusy || len(queued.tabList()) != 0 || len(*queuedWrites) != 0 {
		t.Fatal("stale startup sequence initiated a connection")
	}
	newer := m.tabPersistence
	m.configureTabPersistence(func() (savedSessionTabs, error) { return saved, nil }, nil, false)
	m.tabPersistence.loading = true
	result.owner = newer
	m.handleTabRestore(result)
	if !m.tabPersistence.loading || m.activeTabID() != "manual" {
		t.Fatal("old namespace completion changed current state")
	}
	result.owner = m.tabPersistence
	result.login = "http://other-origin"
	m.handleTabRestore(result)
	if !m.tabPersistence.loading {
		t.Fatal("old login completion accepted")
	}
}
func TestTabRestoreUserDraftCancelsDelayedAutoConnectWithoutOverwritingSavedList(t *testing.T) {
	m, writes, result := tabRestoreFixture(savedSessionTabs{Version: 1, IDs: []string{"saved"}, ActiveID: "saved"})
	result.items = tabCatalog("saved")
	m.interruptTabRestore()
	m.input.SetValue("early draft")
	m.handleTabRestore(result)
	if m.sessionBusy || m.input.Value() != "early draft" || len(*writes) != 0 {
		t.Fatal("delayed startup overrode early user interaction")
	}
}
func TestTabRestoreEmptySnapshotAndMissingFileHaveDifferentStartupMeaning(t *testing.T) {
	for _, saved := range []savedSessionTabs{{}, {Version: 1, IDs: []string{}}, {Version: 1, IDs: []string{"gone"}, ActiveID: "gone"}} {
		m, writes, result := tabRestoreFixture(saved)
		result.items = tabCatalog("unopened")
		m.handleTabRestore(result)
		if saved.Version == 0 {
			if m.sessionAction != "new" || len(*writes) != 0 {
				t.Fatal("missing file did not use one initial fallback without premature save")
			}
		} else {
			if m.sessionBusy || m.session != nil {
				t.Fatal("intentional or confirmed-pruned empty tabs created backend session")
			}
			if len(saved.IDs) > 0 && (len(*writes) != 1 || len((*writes)[0].IDs) != 0) {
				t.Fatal("confirmed missing IDs were not pruned")
			}
		}
	}
}
func TestTabRestoreSavesAcceptedCloseAndDeleteButRetainsFinalTab(t *testing.T) {
	m := tabFixture(3)
	m.settingsLogin = "http://127.0.0.1:1"
	writes := []savedSessionTabs{}
	m.configureTabPersistence(nil, func(value savedSessionTabs) error { writes = append(writes, value); return nil }, false)
	m.tabPersistence.ready = true
	m.tabPersistence.last = m.persistedTabs()
	m.closeTab("session-00")
	if len(writes) != 1 || !reflect.DeepEqual(writes[0].IDs, []string{"session-01", "session-02"}) {
		t.Fatal("inactive close not saved")
	}
	m.tabForgetDeletedSession("session-01")
	if len(writes) != 2 || len(writes[1].IDs) != 1 {
		t.Fatal("permanent deletion not saved")
	}
	m.closeTab("session-02")
	if len(writes) != 2 || m.session == nil {
		t.Fatal("closing final tab was persisted or detached")
	}
	m.tabForgetDeletedSession("session-02")
	if len(writes) != 3 || len(writes[2].IDs) != 0 || writes[2].ActiveID != "" {
		t.Fatal("confirmed final deletion did not persist empty home")
	}
}
func TestTabRestoreRejectsMalformedPersistenceAndReportsSaveFailureOutsideTranscript(t *testing.T) {
	for _, saved := range []savedSessionTabs{{Version: 2}, {Version: 1, IDs: []string{"a", "a"}}, {Version: 1, IDs: []string{"a"}, ActiveID: "absent"}, {Version: 1, IDs: []string{"bad\tID"}}, {Version: 1, IDs: []string{strings.Repeat("x", 201)}}} {
		if validateSavedSessionTabs(saved) == nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
	m := tabFixture(2)
	m.configureTabPersistence(nil, func(savedSessionTabs) error { return errors.New("disk failed") }, false)
	m.tabPersistence.ready = true
	m.persistSessionTabs()
	if len(m.lines) != 0 || m.preferencesNote == "" || len(m.events) != 2 || !m.tabPersistence.dirty {
		t.Fatal("save failure lost retry state or polluted conversation")
	}
}
func TestTabRestoreInputDispatchInvalidatesOnlyUserInteraction(t *testing.T) {
	m, _, _ := tabRestoreFixture(savedSessionTabs{Version: 1})
	m.Update(tea.MouseMsg{Action: tea.MouseActionMotion})
	if m.tabPersistence.interacted {
		t.Fatal("hover interrupted startup")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !m.tabPersistence.interacted {
		t.Fatal("user input did not supersede pending startup")
	}
}
