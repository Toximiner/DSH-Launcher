package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const historySearchPages = 10
const (
	historySearchAll = iota
	historySearchMessages
	historySearchTools
)

func historyScopeLabel(scope int) string {
	switch scope {
	case historySearchMessages:
		return "сообщения"
	case historySearchTools:
		return "инструменты"
	default:
		return "всё"
	}
}

type historySearch struct {
	scope, editScope           int
	editor                     textinput.Model
	editing, running, finished bool
	query                      string
	nextBefore, hitSeq, anchor int
	cancel                     context.CancelFunc
	remaining                  []int
	page                       historyPage
	before                     int
}
type historySearchResult struct {
	snapshot                               *historySnapshot
	scope                                  int
	reader                                 *historyReader
	serial                                 int
	query                                  string
	cut, before, nextBefore, hitSeq, pages int
	page                                   historyPage
	hits                                   []int
	finished                               bool
	err                                    error
}

func (m *model) editHistorySearch() tea.Cmd {
	h := m.historyReader
	if h == nil {
		return nil
	}
	if h.search != nil && h.search.running {
		m.cancelHistorySearch()
	}
	if h.busy {
		return nil
	}
	if h.search == nil {
		editor := textinput.New()
		editor.Prompt = "Поиск: "
		editor.Placeholder = "Текст сообщения или инструмента…"
		editor.CharLimit = 256
		h.search = &historySearch{editor: editor, hitSeq: -1, anchor: -1}
	}
	h.search.editor.SetValue(h.search.query)
	h.search.editor.CursorEnd()
	h.search.editor.Width = max(1, h.viewport.Width-7)
	h.search.editing = true
	h.search.editScope = h.search.scope
	h.note = "Область: " + historyScopeLabel(h.search.editScope) + " · Ctrl+T — сменить · Enter — найти · Esc — назад."
	return h.search.editor.Focus()
}

func (m *model) cancelHistorySearch() {
	h := m.historyReader
	if h == nil || h.search == nil || !h.search.running {
		return
	}
	h.search.cancel()
	h.serial++
	h.search.running = false
	h.busy = false
	h.note = "Поиск отменён. F3 — повторить; Esc — вернуться в чат."
}

func (m *model) startHistorySearch(fresh bool) tea.Cmd {
	h := m.historyReader
	if h == nil || h.search == nil || h.busy {
		return nil
	}
	s := h.search
	if fresh {
		query := strings.TrimSpace(clean(s.editor.Value()))
		if query == "" {
			h.note = "Введите текст для поиска."
			return nil
		}
		s.query, s.nextBefore, s.hitSeq, s.finished = query, h.cut+1, -1, false
		s.scope = s.editScope
		s.remaining, s.page = nil, historyPage{}
		s.editing = false
		s.editor.Blur()
		m.resizeHistory()
	} else if s.query == "" {
		return m.editHistorySearch()
	}
	if len(s.remaining) > 0 {
		hit := s.remaining[0]
		s.remaining = s.remaining[1:]
		m.showHistorySearchHit(s.page, s.before, hit)
		return nil
	}
	if s.finished {
		h.note = "Поиск дошёл до начала истории. Ctrl+F — новый запрос."
		return nil
	}
	ctx, cancel := context.WithTimeout(h.ctx, 20*time.Second)
	s.cancel, s.running = cancel, true
	h.busy, h.note = true, "Поиск по истории… Esc — отменить."
	h.serial++
	serial, cut, before, query, login, id := h.serial, h.cut, s.nextBefore, s.query, m.settingsLogin, h.sessionID
	snapshot, scope := h.snapshot, s.scope
	address := h.address.wire(id)
	return func() tea.Msg {
		defer cancel()
		result := historySearchResult{reader: h, serial: serial, query: query, cut: cut, before: before, nextBefore: before, hitSeq: -1, snapshot: snapshot, scope: scope}
		api, err := openAPI(ctx, login)
		if err == nil {
			defer api.close()
			read := func(ctx context.Context, before int) (historyPage, error) {
				var page historyPage
				err := api.rpc(ctx, "session/events", map[string]any{"request": map[string]any{"address": address, "throughSeq": cut, "beforeSeq": before, "maxMessages": historyPageMessages}}, &page)
				return page, err
			}
			if result.snapshot == nil {
				result.snapshot, _, result.err = buildHistorySnapshot(ctx, cut, read)
			}
			if result.err == nil {
				result = scanHistory(ctx, result, id, read)
			}
		} else {
			result.err = errors.New("Поиск недоступен. Проверьте подключение и нажмите F3.")
		}
		return result
	}
}

// Bound each batch and keep only one page. A continuation never means no matches.
func scanHistory(ctx context.Context, result historySearchResult, id string, read func(context.Context, int) (historyPage, error)) historySearchResult {
	query := strings.ToLower(result.query)
	before := result.nextBefore
	for result.pages < historySearchPages {
		if ctx.Err() != nil {
			result.err = errors.New("Поиск прерван или истекло время. F3 — повторить.")
			return result
		}
		page, err := read(ctx, before)
		if err != nil {
			result.err = errors.New("Не удалось прочитать историю для поиска. F3 — повторить.")
			return result
		}
		projection, first, err := projectIndexedHistory(page, id, result.cut, before, result.snapshot)
		if err != nil {
			result.err = err
			return result
		}
		result.pages++
		var hits []int
		for _, message := range projection.messages {
			if historyMatches(message, query, result.scope) {
				hits = append(hits, message.seq)
			}
		}
		if len(hits) > 0 {
			sort.Sort(sort.Reverse(sort.IntSlice(hits)))
			result.before, result.page, result.hitSeq, result.hits = before, page, hits[0], hits
			// Finish all matches in this validated page before moving its boundary.
			// Cutting at a hit could resurrect messages removed later in this page.
			result.nextBefore = first
			result.finished = !page.HasMore
			return result
		}
		if !page.HasMore {
			result.finished = true
			result.nextBefore = 0
			return result
		}
		if first < 0 || first >= before {
			result.err = errors.New("Поиск остановлен: страница истории не продвигается.")
			return result
		}
		before, result.nextBefore = first, first
	}
	return result
}

func (m *model) handleHistorySearch(result historySearchResult) {
	h := m.historyReader
	if h == nil || h != result.reader || h.search == nil || !h.search.running || h.ctx.Err() != nil || h.serial != result.serial || m.chat == nil || m.chat.epoch != h.epoch || m.chat.id != h.sessionID {
		return
	}
	h.busy, h.search.running = false, false
	if result.err != nil {
		h.note = result.err.Error()
		return
	}
	s := h.search
	if result.snapshot != nil {
		h.snapshot = result.snapshot
	}
	s.nextBefore, s.finished = result.nextBefore, result.finished
	if result.hitSeq < 0 {
		if result.finished {
			h.note = "Более ранних совпадений нет; поиск дошёл до начала истории."
		} else {
			h.note = fmt.Sprintf("Проверено %d страниц. F3 — продолжить поиск более ранних сообщений.", result.pages)
		}
		return
	}
	s.page, s.before = result.page, result.before
	s.remaining = append([]int(nil), result.hits...)
	if len(s.remaining) > 0 {
		s.remaining = s.remaining[1:]
	}
	m.showHistorySearchHit(result.page, result.before, result.hitSeq)
}

func (m *model) showHistorySearchHit(page historyPage, before, hit int) {
	h := m.historyReader
	s := h.search
	if _, _, err := projectIndexedHistory(page, h.sessionID, h.cut, before, h.snapshot); err != nil {
		h.note = err.Error()
		return
	}
	m.handleHistoryResult(historyResult{reader: h, serial: h.serial, direction: "search", cut: h.cut, before: before, page: page, snapshot: h.snapshot})
	if h.projection == nil || h.before != before {
		return
	}
	s.hitSeq = hit
	h.selectedSeq = -1
	h.toolFocused = false
	for _, message := range h.projection.messages {
		if message.seq == hit && message.tool != nil {
			h.toolFocused = true
			h.projection.selectedTool = message.tool.ID
			h.projection.expandedTools[message.tool.ID] = true
			h.rememberHistoryTool(historyToolKey{message.tool.ID, message.tool.StartSeq}, true)
			break
		}
		if message.seq == hit && historyForkable(message) {
			h.selectedSeq = hit
			break
		}
	}
	h.note = "Совпадение найдено. Ctrl+Y — копия · Ctrl+K — код · F3 — раньше."
	if h.toolFocused {
		h.note = "Совпадение в инструменте · Enter — детали · F3 — раньше · Tab — сообщение."
	}
	m.resizeHistory()
	if s.anchor >= 0 {
		h.viewport.SetYOffset(s.anchor)
	}
}

func (m *model) historySearchKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	h := m.historyReader
	if h == nil {
		return nil, false
	}
	control := !msg.Paste && msg.Type != tea.KeyRunes
	if control && msg.Type == tea.KeyF11 {
		return nil, false
	}
	if control && msg.Type == tea.KeyCtrlF {
		return m.editHistorySearch(), true
	}
	s := h.search
	if s == nil {
		if control && msg.Type == tea.KeyF3 {
			return m.editHistorySearch(), true
		}
		return nil, false
	}
	if control && msg.Type == tea.KeyEsc && s.running {
		m.cancelHistorySearch()
		return nil, true
	}
	if s.editing {
		if control {
			switch msg.String() {
			case "ctrl+t":
				s.editScope = (s.editScope + 1) % 3
				h.note = "Область: " + historyScopeLabel(s.editScope) + " · Ctrl+T — сменить · Enter — найти."
				return nil, true
			case "esc":
				s.editing = false
				s.editor.Blur()
				h.note = "Поиск закрыт. Ctrl+F — открыть; Esc — чат."
				return nil, true
			case "enter":
				return m.startHistorySearch(true), true
			}
		}
		var cmd tea.Cmd
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
			msg.Paste = true
		}
		s.editor, cmd = s.editor.Update(msg)
		return cmd, true
	}
	if control && msg.Type == tea.KeyF3 {
		return m.startHistorySearch(false), true
	}
	return nil, false
}

func historyMatches(message chatMessage, query string, scope int) bool {
	if message.tool != nil {
		if scope == historySearchMessages {
			return false
		}
		for _, value := range []string{message.tool.Name, message.tool.Arguments, message.tool.Result, message.tool.ErrorCode} {
			if strings.Contains(strings.ToLower(value), query) {
				return true
			}
		}
		return false
	}
	return scope != historySearchTools && message.approvalID == "" && message.text != "" && strings.Contains(strings.ToLower(message.text), query)
}
