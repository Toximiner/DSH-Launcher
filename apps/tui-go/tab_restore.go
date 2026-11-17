package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// Persist navigation identity only. No tokens, titles, drafts or conversation data.
// A zero version with no values means that no saved file exists yet.
type savedSessionTabs struct {
	Version  int      `json:"version"`
	IDs      []string `json:"ids"`
	ActiveID string   `json:"activeId"`
}

type tabPersistenceState struct {
	load                                       func() (savedSessionTabs, error)
	save                                       func(savedSessionTabs) error
	fallbackNew                                bool
	started, loading, ready, interacted, dirty bool
	last                                       savedSessionTabs
}

type tabRestoreResult struct {
	owner                      *tabPersistenceState
	login                      string
	sequence                   int
	saved                      savedSessionTabs
	items                      []sessionItem
	explicitAction, explicitID string
	err                        error
}

func validateSavedSessionTabs(saved savedSessionTabs) error {
	if saved.Version != 1 && !(saved.Version == 0 && len(saved.IDs) == 0 && saved.ActiveID == "") {
		return errors.New("unsupported saved session tabs")
	}
	if len(saved.IDs) > 512 {
		return errors.New("too many saved session tabs")
	}
	seen := map[string]bool{}
	for _, id := range saved.IDs {
		if id == "" || len(id) > 200 || !utf8.ValidString(id) || strings.IndexFunc(id, unicode.IsControl) >= 0 || strings.TrimSpace(id) != id || strings.ContainsAny(id, "\x00\r\n\x1b") || seen[id] {
			return errors.New("invalid saved session identity")
		}
		seen[id] = true
	}
	if saved.ActiveID != "" && !seen[saved.ActiveID] {
		return errors.New("saved active session is absent from tabs")
	}
	return nil
}

func (m *model) configureTabPersistence(load func() (savedSessionTabs, error), save func(savedSessionTabs) error, fallbackNew bool) {
	m.tabPersistence = &tabPersistenceState{load: load, save: save, fallbackNew: fallbackNew}
}

// Called once the actual backend login is known, including runtime startup.
func (m *model) startSessionTabs() tea.Cmd {
	p := m.tabPersistence
	if p == nil {
		create, id := m.createSession, m.initialSession
		m.createSession, m.initialSession = false, ""
		if create {
			return m.sessionOperation("new", "")
		}
		if id != "" {
			return m.sessionOperation("connect", id)
		}
		return nil
	}
	if p.started || m.settingsLogin == "" {
		return nil
	}
	p.started, p.loading = true, true
	login, ctx, sequence := m.settingsLogin, m.settingsContext, m.sessionSequence
	action, id := "", ""
	if m.createSession {
		action = "new"
	} else if m.initialSession != "" {
		action, id = "connect", m.initialSession
	}
	m.createSession, m.initialSession = false, ""
	return func() tea.Msg {
		result := tabRestoreResult{owner: p, login: login, sequence: sequence, explicitAction: action, explicitID: id}
		if p.load == nil {
			result.err = errors.New("saved session tab reader is unavailable")
			return result
		}
		result.saved, result.err = p.load()
		if result.err == nil {
			result.err = validateSavedSessionTabs(result.saved)
		}
		if result.err != nil {
			return result
		}
		if ctx == nil {
			ctx = context.Background()
		}
		readCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		api, err := openAPI(readCtx, login)
		if err != nil {
			result.err = err
			return result
		}
		defer api.close()
		var catalog struct {
			Items []sessionItem `json:"items"`
		}
		result.err = api.rpc(readCtx, "session/list", map[string]any{"_request": map[string]any{}}, &catalog)
		result.items = catalog.Items
		return result
	}
}

func (m *model) interruptTabRestore() {
	if p := m.tabPersistence; p != nil && p.loading {
		p.interacted = true
	}
}

func (m *model) persistedTabs() savedSessionTabs {
	snapshot := savedSessionTabs{Version: 1, IDs: []string{}, ActiveID: m.activeTabID()}
	for _, tab := range m.tabList() {
		snapshot.IDs = append(snapshot.IDs, tab.ID)
	}
	return snapshot
}

// Small atomic native writes run in model order so quit and older write completions
// cannot lose or replace the final tab snapshot. No callback exists in pure models.
func (m *model) persistSessionTabs() {
	p := m.tabPersistence
	if p == nil {
		return
	}
	p.dirty = true
	if !p.ready || p.save == nil {
		return
	}
	snapshot := m.persistedTabs()
	if p.last.Version == snapshot.Version && p.last.ActiveID == snapshot.ActiveID && slices.Equal(p.last.IDs, snapshot.IDs) {
		p.dirty = false
		return
	}
	if err := p.save(snapshot); err != nil {
		m.tabPersistenceNotice("Не удалось сохранить открытые вкладки.")
		return
	}
	p.last, p.dirty = snapshot, false
}

func (m *model) handleTabRestore(result tabRestoreResult) tea.Cmd {
	p := m.tabPersistence
	if p == nil || result.owner != p || result.login != m.settingsLogin || !p.loading {
		return nil
	}
	p.loading = false
	cancelled := p.interacted || result.sequence != m.sessionSequence
	if result.err != nil {
		// Failed reads never overwrite a previous good snapshot or auto-create a session.
		m.tabPersistenceNotice("Не удалось восстановить вкладки. Сохранённый список не изменён.")
		if !cancelled && result.explicitAction != "" {
			return m.sessionOperation(result.explicitAction, result.explicitID)
		}
		return nil
	}
	p.ready = true
	p.last = result.saved
	if cancelled {
		if p.dirty {
			m.persistSessionTabs()
		}
		return nil
	}
	catalog := make(map[string]sessionItem, len(result.items))
	for _, item := range result.items {
		catalog[item.ID] = item
	}
	restored := make([]sessionItem, 0, len(result.saved.IDs))
	for _, id := range result.saved.IDs {
		if item, exists := catalog[id]; exists {
			restored = append(restored, item)
		}
	}
	m.tabState.items = restored
	m.sessionItems = result.items
	if result.explicitAction != "" {
		return m.sessionOperation(result.explicitAction, result.explicitID)
	}
	target := ""
	if _, exists := catalog[result.saved.ActiveID]; exists {
		target = result.saved.ActiveID
	}
	if target == "" && len(restored) > 0 {
		target = restored[0].ID
	}
	if target != "" {
		m.rebuild()
		return m.sessionOperation("connect", target)
	}
	if result.saved.Version == 1 {
		m.persistSessionTabs()
	} // Prune only an existing snapshot after a confirmed catalog.
	m.rebuild()
	if p.fallbackNew && result.saved.Version == 0 {
		return m.sessionOperation("new", "")
	}
	return m.input.Focus()
}

func (m *model) tabPersistenceNotice(message string) {
	m.preferencesNote = tr(message)
	m.events = append(m.events, m.preferencesNote)
	if len(m.events) > 200 {
		m.events = m.events[len(m.events)-200:]
	}
}
