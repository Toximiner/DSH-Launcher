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

type historySearch struct {
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
		editor.Placeholder = "Текст сообщения…"
		editor.CharLimit = 256
		h.search = &historySearch{editor: editor, hitSeq: -1, anchor: -1}
	}
	h.search.editor.SetValue(h.search.query)
	h.search.editor.CursorEnd()
	h.search.editor.Width = max(1, h.viewport.Width-7)
	h.search.editing = true
	h.note = "Поиск текста сообщений без учёта регистра. Enter — найти; Esc — назад."
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
	return func() tea.Msg {
		defer cancel()
		result := historySearchResult{reader: h, serial: serial, query: query, cut: cut, before: before, nextBefore: before, hitSeq: -1}
		api, err := openAPI(ctx, login)
		if err == nil {
			defer api.close()
			result = scanHistory(ctx, result, id, func(ctx context.Context, before int) (historyPage, error) {
				var page historyPage
				err := api.rpc(ctx, "session/events", map[string]any{"request": map[string]any{
					"address":    map[string]any{"kind": "session", "sessionId": id},
					"throughSeq": cut, "beforeSeq": before, "maxMessages": historyPageMessages,
				}}, &page)
				return page, err
			})
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
		projection, first, err := projectHistory(page, id, result.cut, before)
		if err != nil {
			result.err = err
			return result
		}
		result.pages++
		var hits []int
		for _, message := range projection.messages {
			// Tool arguments/results, permission records and hidden service context
			// are not conversation text and are intentionally outside this search.
			if message.tool == nil && message.approvalID == "" && message.text != "" && strings.Contains(strings.ToLower(message.text), query) {
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
	m.handleHistoryResult(historyResult{reader: h, serial: h.serial, direction: "search", cut: h.cut, before: before, page: page})
	s.hitSeq = hit
	h.selectedSeq = -1
	for _, message := range h.projection.messages {
		if message.seq == hit && historyForkable(message) {
			h.selectedSeq = hit
			break
		}
	}
	h.note = "Совпадение найдено. Ctrl+B — ветка; F3 — раньше; Ctrl+F — новый запрос."
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
