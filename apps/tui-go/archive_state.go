package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/charmbracelet/x/ansi"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type archiveState struct {
	ctx    context.Context
	cancel context.CancelFunc
	inbox  chan archiveUpdate
	ids    map[string]bool
	ready  bool
}
type archiveUpdate struct {
	state *archiveState
	ids   map[string]bool
	err   error
}

func archiveIDs(ids []string) (map[string]bool, error) {
	if ids == nil || len(ids) > 100000 {
		return nil, errors.New("неподдерживаемый список архива")
	}
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || result[id] {
			return nil, errors.New("некорректный список архива")
		}
		result[id] = true
	}
	return result, nil
}

func parseArchiveFrame(raw json.RawMessage, baseline bool) (map[string]bool, bool, error) {
	var frame struct {
		Type  string `json:"type"`
		Value struct {
			IDs []string `json:"archivedSessionIds"`
		} `json:"value"`
		IDs []string `json:"archivedSessionIds"`
	}
	if json.Unmarshal(raw, &frame) != nil {
		return nil, false, errors.New("некорректное состояние архива")
	}
	if !baseline && frame.Type != "baseline" {
		return nil, false, errors.New("нет начального состояния архива")
	}
	switch frame.Type {
	case "baseline", "replace":
		ids, err := archiveIDs(frame.Value.IDs)
		return ids, true, err
	case "archived":
		ids, err := archiveIDs(frame.IDs)
		return ids, true, err
	case "upsert", "remove", "order", "pinned":
		return nil, false, nil
	default:
		return nil, false, errors.New("неизвестное состояние архива")
	}
}

func (m *model) ensureArchive() tea.Cmd {
	if m.archive != nil || m.settingsLogin == "" || m.browserPreview {
		return nil
	}
	return m.restartArchive(nil)
}
func (m *model) restartArchive(ids map[string]bool) tea.Cmd {
	if m.archive != nil {
		m.archive.cancel()
	}
	ctx := m.settingsContext
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	state := &archiveState{ctx: ctx, cancel: cancel, inbox: make(chan archiveUpdate, 16), ids: ids, ready: ids != nil}
	m.archive = state
	login := m.settingsLogin
	return func() tea.Msg { go followArchive(ctx, login, state); return waitArchive(state)() }
}
func waitArchive(state *archiveState) tea.Cmd {
	return func() tea.Msg {
		select {
		case update := <-state.inbox:
			return update
		case <-state.ctx.Done():
			return nil
		}
	}
}
func (m *model) handleArchiveUpdate(update archiveUpdate) tea.Cmd {
	if m.archive != update.state || update.state.ctx.Err() != nil {
		return nil
	}
	m.archive.ready = update.err == nil
	if update.err == nil {
		m.archive.ids = update.ids
		m.sessionListIndex = 0
	}
	m.rebuild()
	m.revealSelectedSession()
	return waitArchive(m.archive)
}
func (m *model) isSessionArchived(id string) bool { return m.archive != nil && m.archive.ids[id] }
func (m *model) visibleSessions() []sessionItem {
	if m.archive == nil {
		return m.sessionItems
	}
	var items []sessionItem
	for _, item := range m.sessionItems {
		if m.isSessionArchived(item.ID) == m.showArchived {
			items = append(items, item)
		}
	}
	return items
}

func (m *model) revealSelectedSession() {
	if m.page != sessionsPage {
		return
	}
	for row, line := range strings.Split(ansi.Hardwrap(clean(m.sessionInfo()), max(1, m.viewport.Width), true), "\n") {
		if strings.HasPrefix(line, "› ") {
			if row < m.viewport.YOffset {
				m.viewport.SetYOffset(row)
			} else if row >= m.viewport.YOffset+m.viewport.Height {
				m.viewport.SetYOffset(row - m.viewport.Height + 1)
			}
			break
		}
	}
}
