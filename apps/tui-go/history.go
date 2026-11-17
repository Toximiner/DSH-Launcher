package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

const historyPageMessages = 20
const historyBackLimit = 128

type historyPage struct {
	Cursor  int          `json:"cursor"`
	Records []chatRecord `json:"records"`
	HasMore bool         `json:"hasMore"`
}
type historyLocation struct{ before, offset, number int }
type historyMessageRow struct{ seq, start, end int }

type historyReader struct {
	address                    sessionReadAddress
	snapshot                   *historySnapshot
	toolFocused                bool
	toolRows                   []chatToolHit
	expandedTools              map[historyToolKey]bool
	expandedOrder              []historyToolKey
	codeDialog                 *historyCodeDialog
	copyBusy                   bool
	copyOperation              int
	rawMarkdown                bool
	selectedSeq                int
	messageRows                []historyMessageRow
	ctx                        context.Context
	cancel                     context.CancelFunc
	sessionID                  string
	epoch                      int
	serial                     int
	busy                       bool
	focused                    bool
	cut, before, first, number int
	hasMore                    bool
	back                       []historyLocation
	projection                 *chatState
	viewport                   viewport.Model
	note                       string
	search                     *historySearch
}
type historyResult struct {
	snapshot    *historySnapshot
	reader      *historyReader
	serial      int
	direction   string
	cut, before int
	page        historyPage
	err         error
}

func (m *model) openHistory() tea.Cmd {
	if m.historyReader != nil {
		return m.closeHistory()
	}
	if m.chat == nil || !m.chat.ready || m.session == nil || m.chatSessionChanging() {
		m.append(tr("Для истории подключитесь к сессии dsh и дождитесь чата."))
		return nil
	}
	ctx, cancel := context.WithCancel(m.chat.ctx)
	h := &historyReader{address: m.session.Address.wire(m.chat.id), selectedSeq: -1, ctx: ctx, cancel: cancel, sessionID: m.chat.id, epoch: m.chat.epoch,
		cut: m.chat.cursor, before: m.chat.cursor + 1, first: -1, focused: m.input.Focused(), viewport: viewport.New(1, 1)}
	m.historyReader = h
	m.input.Blur()
	m.resizeHistory()
	return m.requestHistory("latest", h.cut, h.before)
}

func (m *model) closeHistory() tea.Cmd {
	h := m.historyReader
	if h == nil {
		return nil
	}
	h.cancel()
	m.historyReader = nil
	if h.focused && m.page == chatPage && !m.navFocused {
		return m.input.Focus()
	}
	return nil
}

func (m *model) requestHistory(direction string, cut, before int) tea.Cmd {
	h := m.historyReader
	if h == nil || h.busy {
		return nil
	}
	h.busy, h.note = true, tr("Чтение истории…")
	h.serial++
	serial, login := h.serial, m.settingsLogin
	snapshot := h.snapshot
	if snapshot != nil && snapshot.cut != cut {
		snapshot = nil
	}
	return func() tea.Msg {
		result := historyResult{reader: h, serial: serial, direction: direction, cut: cut, before: before}
		ctx, cancel := context.WithTimeout(h.ctx, 20*time.Second)
		defer cancel()
		api, err := openAPI(ctx, login)
		if err == nil {
			defer api.close()
			read := func(ctx context.Context, before int) (historyPage, error) {
				var page historyPage
				err := api.rpc(ctx, "session/events", map[string]any{"request": map[string]any{"address": h.address.wire(h.sessionID), "throughSeq": cut, "beforeSeq": before, "maxMessages": historyPageMessages}}, &page)
				if err != nil {
					err = errors.New(tr("История недоступна. Проверьте подключение; ←/→ или F5 повторят чтение."))
				}
				return page, err
			}
			if snapshot == nil || direction == "latest" {
				var latest historyPage
				snapshot, latest, err = buildHistorySnapshot(ctx, cut, read)
				if err == nil && before == cut+1 {
					result.page = latest
				} else if err == nil {
					result.page, err = read(ctx, before)
				}
			} else {
				result.page, err = read(ctx, before)
			}
			result.snapshot = snapshot
			result.err = err
		} else {
			result.err = errors.New(tr("История недоступна. Проверьте подключение; ←/→ или F5 повторят чтение."))
		}
		return result
	}
}

// Replay in an isolated model: reading old events must never change live receipts,
// agent state, pending questions, tool observers, or the composer.
func projectHistory(page historyPage, sessionID string, cut, before int) (*chatState, int, error) {
	if page.Cursor != cut || before < 0 || before > cut+1 || len(page.Records) > 4096 {
		return nil, 0, errors.New(tr("Некорректный снимок истории."))
	}
	s := &chatState{id: sessionID, ready: true, cursor: cut, clearedThrough: -1, expandedTools: make(map[string]bool)}
	shadow := &model{chat: s}
	last, first := -1, -1
	for _, record := range page.Records {
		e := record.Event
		if record.Type != "event" || e.Seq < 0 || e.Seq >= before || last >= 0 && e.Seq != last+1 {
			return nil, 0, errors.New(tr("Пропуск или неверный порядок событий истории."))
		}
		if first < 0 {
			first = e.Seq
		}
		last = e.Seq
		if err := shadow.applyChatEvent(s, e); err != nil {
			return nil, 0, errors.New(tr("Страница содержит неизвестные данные или замену вне её границ."))
		}
		if s.hasMore {
			return nil, 0, errors.New(tr("Страница слишком велика: 100 записей / 4 МиБ. Чат сохранён."))
		}
	}
	if before > 0 && last != before-1 || before == 0 && len(page.Records) != 0 || page.HasMore != (first > 0) {
		return nil, 0, errors.New(tr("Неполная страница истории."))
	}
	for _, message := range s.messages {
		if message.tool != nil {
			if message.tool.State == "running" {
				message.tool.State = "unknown"
			}
		}
	}
	return s, first, nil
}

func (m *model) handleHistoryResult(result historyResult) {
	h := m.historyReader
	if h == nil || h != result.reader || h.ctx.Err() != nil || h.serial != result.serial || m.chat == nil || m.chat.epoch != h.epoch || m.chat.id != h.sessionID {
		return
	}
	h.busy = false
	if result.err != nil {
		h.note = result.err.Error()
		return
	}
	snapshot := result.snapshot
	if snapshot == nil && h.snapshot != nil && h.snapshot.cut == result.cut {
		snapshot = h.snapshot
	}
	projection, first, err := projectIndexedHistory(result.page, h.sessionID, result.cut, result.before, snapshot)
	if err != nil {
		h.note = err.Error()
		return
	}
	offset := 0
	switch result.direction {
	case "older", "search":
		h.back = append(h.back, historyLocation{h.before, h.viewport.YOffset, h.number})
		if len(h.back) > historyBackLimit {
			h.back = h.back[len(h.back)-historyBackLimit:]
		}
		h.number++
	case "newer":
		if len(h.back) == 0 {
			h.note = tr("Нет сохранённого перехода к более новой странице.")
			return
		}
		location := h.back[len(h.back)-1]
		h.back = h.back[:len(h.back)-1]
		offset, h.number = location.offset, location.number
	default:
		h.back = nil
		h.number = 1
		if h.search != nil {
			h.search.hitSeq = -1
			h.search.nextBefore = result.cut + 1
			h.search.finished = false
			h.search.remaining, h.search.page = nil, historyPage{}
		}
	}
	h.cut, h.before, h.first = result.cut, result.before, first
	h.hasMore, h.projection = result.page.HasMore, projection
	h.snapshot = snapshot
	h.selectedSeq = -1
	m.restoreHistoryTools(result.direction != "older" && result.direction != "newer" && result.direction != "search")
	h.note = tr("Tab/Shift+Tab — выбор сообщения · Ctrl+B — ветка · Ctrl+Y — копия · Ctrl+K — код · F6 — инструменты.")
	m.resizeHistory()
	if result.direction == "newer" {
		h.viewport.SetYOffset(offset)
	} else {
		h.viewport.GotoBottom()
	}
}

func (m *model) historyBox() area {
	w, h := min(100, m.width-1), min(36, m.height-2)
	return area{max(0, (m.width-1-w)/2), max(0, (m.height-h)/2), w, h}
}
func (m *model) resizeHistory() {
	h := m.historyReader
	if h == nil {
		return
	}
	box := m.historyBox()
	h.viewport.Width, h.viewport.Height = max(1, box.width-4), max(1, box.height-8)
	if h.search != nil {
		h.search.editor.Width = max(1, h.viewport.Width-7)
		h.search.anchor = -1
	}
	h.messageRows = nil
	h.toolRows = nil
	content := tr("Ожидание истории…")
	if h.projection != nil {
		renderer := &model{chat: h.projection, viewport: h.viewport, toolFocused: h.toolFocused}
		var parts []string
		row := 0
		for _, display := range renderer.displayMessages() {
			message := display.message
			part := ""
			if message.tool != nil {
				part = renderer.displayTool(message.tool, display.depth)
			} else {
				part = accent().Bold(true).Render(message.role) + "\n" + m.messageText(message, h.viewport.Width, h.rawMarkdown)
			}
			if h.search != nil && message.seq == h.search.hitSeq {
				h.search.anchor = row
				label := tr("▶ Совпадение")
				if message.seq == h.selectedSeq {
					label += fmt.Sprintf(tr(" · точка #%d"), message.seq)
				}
				if message.tool != nil {
					label += tr(" · инструмент")
					plain := strings.ToLower(clean(part))
					if offset := strings.Index(plain, strings.ToLower(h.search.query)); offset >= 0 {
						h.search.anchor = row + 1 + strings.Count(ansi.Hardwrap(plain[:offset], h.viewport.Width, true), "\n")
					}
				}
				part = heading().Render(label) + "\n" + part
			}
			if message.seq == h.selectedSeq && (h.search == nil || message.seq != h.search.hitSeq) {
				part = heading().Render(fmt.Sprintf(tr("◆ Точка ветвления #%d"), message.seq)) + "\n" + part
			}
			if message.tool != nil {
				h.toolRows = append(h.toolRows, chatToolHit{message.tool.ID, row, row + strings.Count(ansi.Hardwrap(part, h.viewport.Width, true), "\n") + 1})
			}
			if historyForkable(message) {
				h.messageRows = append(h.messageRows, historyMessageRow{message.seq, row, row + strings.Count(ansi.Hardwrap(part, h.viewport.Width, true), "\n") + 1})
			}
			parts = append(parts, part)
			row += strings.Count(ansi.Hardwrap(part, h.viewport.Width, true), "\n") + 2
		}
		content = strings.Join(parts, "\n\n")
		if content == "" {
			content = tr("Нет сообщений на странице.")
		}
	}
	h.viewport.SetContent(ansi.Hardwrap(content, h.viewport.Width, true))
	h.viewport.SetYOffset(h.viewport.YOffset)
}

var historyButtons = []string{"[←]", "[→]", "[F5 Новые]", "[Поиск]", "[Esc]"}

func (m *model) historyView(base string) string {
	h := m.historyReader
	if h == nil || !m.terminalFits() {
		return base
	}
	box := m.historyBox()
	width := box.width - 4
	status := fmt.Sprintf(tr("Страница %d · снимок до события %d"), h.number, h.cut)
	if h.number == 0 {
		status = tr("Чтение первой страницы…")
	}
	if h.projection != nil && !h.hasMore {
		status += tr(" · начало")
	}
	if h.search != nil && h.search.anchor >= 0 {
		status = fmt.Sprintf(tr("Совпадение: %q · %s · снимок %d"), h.search.query, historyScopeLabel(h.search.scope), h.cut)
	}
	note := h.note
	if h.busy {
		note = tr("Чтение истории…")
		if h.search != nil && h.search.running {
			note = tr("Поиск по истории… Esc — отменить.")
		}
	}
	noteRows := strings.Split(ansi.Hardwrap(note, width, true), "\n")
	for len(noteRows) < 2 {
		noteRows = append(noteRows, "")
	}
	formatHint := tr("Ctrl+R исходник")
	if h.rawMarkdown {
		formatHint = "Ctrl+R Markdown"
	}
	buttons := make([]string, len(historyButtons))
	for i, label := range historyButtons {
		buttons[i] = tr(label)
	}
	toolbar, hint := strings.Join(buttons, " "), tr("PgUp/PgDn · Ctrl+F поиск · ")+formatHint+tr(" · F3 ещё")
	if h.search != nil && h.search.editing {
		toolbar = h.search.editor.View()
		hint = tr("Enter — найти · Ctrl+T область: ") + historyScopeLabel(h.search.editScope) + tr(" · Esc — назад")
	}
	title := tr("История сессии")
	if h.rawMarkdown {
		title += tr(" · исходник")
	}
	lines := []string{heading().Render(title), ansi.Truncate(status, width, "…"), toolbar, h.viewport.View(), muted().Render(noteRows[0]), muted().Render(noteRows[1]), muted().Render(hint)}
	return overlay(base, panel(box.width, box.height, strings.Join(lines, "\n"), theme.Accent), box.x, box.y, m.width-1)
}

func (m *model) historyKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	h := m.historyReader
	if h != nil && !msg.Paste {
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlD, tea.KeyF7, tea.KeyF8:
			return nil, false
		}
	}
	if cmd, handled := m.historySearchKey(msg); handled {
		return cmd, true
	}
	if msg.Paste {
		return nil, h != nil
	}
	if msg.Type == tea.KeyF11 {
		return m.openHistory(), true
	}
	if h == nil {
		return nil, false
	}
	if m.historyToolKey(msg) {
		return nil, true
	}
	switch msg.String() {
	case "ctrl+c", "ctrl+d", "f7", "f8":
		return nil, false
	case "esc":
		return m.closeHistory(), true
	case "ctrl+y":
		if msg.Type == tea.KeyCtrlY {
			return m.copyHistoryMessage(), true
		}
		return nil, true
	case "ctrl+k":
		if msg.Type == tea.KeyCtrlK {
			return m.openHistoryCode(), true
		}
		return nil, true
	case "ctrl+r":
		if !h.busy {
			h.rawMarkdown = !h.rawMarkdown
			m.resizeHistory()
			if h.toolFocused {
				m.revealHistoryTool()
			}
			if h.selectedSeq >= 0 {
				m.selectHistoryMessage(h.selectedSeq, true)
			}
			h.note = tr("Ctrl+R — Markdown/исходник · Ctrl+F — поиск исходного текста.")
		}
		return nil, true
	case "tab", "shift+tab":
		if !h.busy {
			m.moveHistorySelection(msg.Type == tea.KeyShiftTab)
		}
		return nil, true
	case "ctrl+b":
		return m.forkHistorySelection(), true
	case "left":
		if h.hasMore && !h.busy {
			return m.requestHistory("older", h.cut, h.first), true
		}
	case "right":
		if len(h.back) > 0 && !h.busy {
			return m.requestHistory("newer", h.cut, h.back[len(h.back)-1].before), true
		}
	case "f5":
		if m.chat != nil && m.chat.ready {
			return m.requestHistory("latest", m.chat.cursor, m.chat.cursor+1), true
		}
	case "home":
		h.viewport.GotoTop()
	case "end":
		h.viewport.GotoBottom()
	case "up", "down", "pgup", "pgdown":
		var cmd tea.Cmd
		h.viewport, cmd = h.viewport.Update(msg)
		return cmd, true
	}
	return nil, true
}

func (m *model) historyMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	h := m.historyReader
	if h == nil {
		return nil, false
	}
	if msg.Action != tea.MouseActionPress {
		return nil, true
	}
	box := m.historyBox()
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		if (area{box.x + 2, box.y + 4, h.viewport.Width, h.viewport.Height}).contains(msg.X, msg.Y) {
			var cmd tea.Cmd
			h.viewport, cmd = h.viewport.Update(msg)
			return cmd, true
		}
		return nil, true
	}
	if msg.Button == tea.MouseButtonLeft && msg.Y == box.y+3 {
		if h.search != nil && h.search.editing {
			return nil, true
		}
		x := box.x + 2
		for i, label := range historyButtons {
			label = tr(label)
			width := ansi.StringWidth(label)
			if msg.X >= x && msg.X < x+width {
				return m.historyKey(tea.KeyMsg{Type: []tea.KeyType{tea.KeyLeft, tea.KeyRight, tea.KeyF5, tea.KeyCtrlF, tea.KeyEsc}[i]})
			}
			x += width + 1
		}
	}
	if msg.Button == tea.MouseButtonLeft && !h.busy && (area{box.x + 2, box.y + 4, h.viewport.Width, h.viewport.Height}).contains(msg.X, msg.Y) {
		row := msg.Y - (box.y + 4) + h.viewport.YOffset
		if h.search == nil || !h.search.editing {
			for _, hit := range h.toolRows {
				if row >= hit.row && row < hit.end {
					m.selectHistoryTool(hit.id, false)
					m.toggleHistoryTool(hit.id)
					return nil, true
				}
			}
		}
		for _, hit := range h.messageRows {
			if row >= hit.start && row < hit.end {
				m.selectHistoryMessage(hit.seq, false)
				break
			}
		}
	}
	return nil, true
}
