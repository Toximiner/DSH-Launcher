package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const chatTextLimit = 1 << 20

type chatEvent struct {
	Type      string          `json:"type"`
	Seq       int             `json:"seq"`
	Data      json.RawMessage `json:"data"`
	SurfaceOp json.RawMessage `json:"surfaceOp"`
}
type chatRecord struct {
	Type  string    `json:"type"`
	Event chatEvent `json:"event"`
}
type chatChunk struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Text  string `json:"text"`
	Block struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"block"`
	Reason struct {
		Kind    string `json:"kind"`
		Failure struct {
			Code string `json:"code"`
		} `json:"failure"`
	} `json:"reason"`
}
type compactChatRecord struct {
	Type  string    `json:"type"`
	Index int       `json:"index"`
	Texts []string  `json:"texts"`
	Chunk chatChunk `json:"chunk"`
}
type chatAttempt struct {
	ID           string              `json:"attemptId"`
	Turn         int                 `json:"turn"`
	Step         int                 `json:"step"`
	Next         int                 `json:"nextIndex"`
	Stream       []compactChatRecord `json:"stream"`
	blocks       map[int]string
	committedSeq *int
}
type assistantChatFrame struct {
	Type    string    `json:"type"`
	ID      string    `json:"attemptId"`
	Turn    int       `json:"turn"`
	Step    int       `json:"step"`
	Index   int       `json:"index"`
	Chunk   chatChunk `json:"chunk"`
	Outcome struct {
		Kind string `json:"kind"`
		Seq  int    `json:"seq"`
	} `json:"outcome"`
}
type chatFrame struct {
	Type   string `json:"type"`
	Header struct {
		ID  string `json:"id"`
		CWD string `json:"cwd"`
	} `json:"header"`
	Cursor          int          `json:"cursor"`
	HasMore         bool         `json:"hasMore"`
	Records         []chatRecord `json:"records"`
	Event           chatEvent    `json:"event"`
	AssistantStream *struct {
		Active *chatAttempt `json:"activeAttempt"`
	} `json:"assistantStream"`
	Frame assistantChatFrame `json:"frame"`
}
type chatNotice struct {
	frame *chatFrame
	err   error
}
type chatUpdate struct {
	epoch  int
	notice chatNotice
}
type chatMessage struct {
	seq        int
	role, text string
	tool       *chatTool
}
type pendingPrompt struct{ ID, Text, Status string }
type chatState struct {
	id             string
	epoch          int
	ctx            context.Context
	cancel         context.CancelFunc
	inbox          chan chatNotice
	ready          bool
	cursor         int
	hasMore        bool
	messages       []chatMessage
	surface        []int
	live           *chatAttempt
	note           string
	streamError    string
	clearedThrough int
	expandedTools  map[string]bool
	selectedTool   string
}
type promptResult struct {
	sessionID, requestID string
	err                  error
	rejected             bool
}
type cancelResult struct {
	sessionID string
	err       error
}

func newRequestID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (m *model) startChat(id string) tea.Cmd {
	if m.browserPreview || (m.chat != nil && m.chat.id == id) {
		return nil
	}
	m.stopChat()
	m.toolFocused = false
	m.chatToolHits = nil
	ctx := m.settingsContext
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	m.chatEpoch++
	s := &chatState{id: id, epoch: m.chatEpoch, ctx: ctx, cancel: cancel, inbox: make(chan chatNotice, 32), cursor: -1, clearedThrough: -1}
	m.chat = s
	login := m.settingsLogin
	return func() tea.Msg { go followChat(ctx, login, id, s.inbox); return waitChat(s)() }
}

func (m *model) stopChat() {
	if m.chat != nil {
		m.chat.cancel()
	}
}

func waitChat(s *chatState) tea.Cmd {
	return func() tea.Msg {
		select {
		case notice := <-s.inbox:
			return chatUpdate{epoch: s.epoch, notice: notice}
		case <-s.ctx.Done():
			return nil
		}
	}
}

func (m *model) submitText(text string) tea.Cmd {
	if m.settingsLogin == "" || m.browserPreview {
		m.append("Введено: " + text)
		return nil
	}
	if m.chatSessionChanging() {
		m.input.SetValue(text)
		m.input.CursorEnd()
		m.append("Дождитесь переключения сессии; сообщение оставлено в поле ввода.")
		return nil
	}
	if m.session == nil || m.chat == nil || !m.chat.ready {
		m.input.SetValue(text)
		m.input.CursorEnd()
		m.append("Чат ещё не подключён. Создайте /new или подключите /session <id>; дождитесь подключения.")
		return nil
	}
	if pending := m.prompts[m.session.ID]; pending != nil && pending.Status != "rejected" {
		m.input.SetValue(text)
		m.input.CursorEnd()
		m.chat.note = "Предыдущее сообщение ещё не подтверждено. При неизвестном результате: /retry-send."
		m.rebuild()
		return nil
	}
	if m.session.Running || m.chat.live != nil {
		m.input.SetValue(text)
		m.input.CursorEnd()
		m.chat.note = "Агент ещё выполняется. F8 — остановить; новое сообщение сохранено в поле ввода."
		m.rebuild()
		return nil
	}
	id, err := newRequestID()
	if err != nil {
		m.input.SetValue(text)
		m.append("Не удалось подготовить сообщение.")
		return nil
	}
	p := &pendingPrompt{ID: id, Text: text, Status: "sending"}
	m.prompts[m.session.ID] = p
	return m.sendPrompt(m.session.ID, p)
}

func (m *model) sendPrompt(id string, p *pendingPrompt) tea.Cmd {
	p.Status = "sending"
	if m.chat != nil && m.chat.id == id {
		m.chat.note = ""
	}
	text, requestID, login, ctx := p.Text, p.ID, m.settingsLogin, m.settingsContext
	if ctx == nil {
		ctx = context.Background()
	}
	m.rebuild()
	m.viewport.GotoBottom()
	return func() tea.Msg {
		result := promptResult{sessionID: id, requestID: requestID}
		api, err := openAPI(ctx, login)
		if err != nil {
			result.err = err
			result.rejected = true
			return result
		}
		defer api.close()
		var receipt struct {
			Accepted bool `json:"accepted"`
		}
		result.err = api.rpc(ctx, "session/prompt", map[string]any{"request": map[string]any{
			"sessionId": id, "requestId": requestID, "mode": "queue", "content": []any{map[string]any{"type": "text", "text": text}},
		}}, &receipt)
		var rejected *rpcRejected
		result.rejected = errors.As(result.err, &rejected)
		if result.err == nil && !receipt.Accepted {
			result.err = errors.New("неподдерживаемое подтверждение отправки")
		}
		return result
	}
}

func (m *model) retryPrompt() tea.Cmd {
	if m.chatSessionChanging() {
		m.append("Дождитесь переключения сессии.")
		return nil
	}
	if m.session == nil || m.chat == nil || !m.chat.ready {
		m.append("Дождитесь подключения чата.")
		return nil
	}
	p := m.prompts[m.session.ID]
	if p == nil || p.Status != "unknown" {
		m.chat.note = "Нет отправки с неизвестным результатом."
		m.rebuild()
		return nil
	}
	// dsh deduplicates requestId. Manual recovery reuses the original immutable request.
	return m.sendPrompt(m.session.ID, p)
}

func (m *model) cancelChat() tea.Cmd {
	if m.chatSessionChanging() {
		m.append("Дождитесь переключения сессии.")
		return nil
	}
	if m.session == nil || m.browserPreview {
		m.append("Нет подключённого агента для отмены.")
		return nil
	}
	id, login, ctx := m.session.ID, m.settingsLogin, m.settingsContext
	if ctx == nil {
		ctx = context.Background()
	}
	if m.chat != nil {
		m.chat.note = "Запрошена отмена; ожидается подтверждение dsh."
		m.rebuild()
	}
	return func() tea.Msg {
		api, err := openAPI(ctx, login)
		if err == nil {
			defer api.close()
			var receipt struct {
				Accepted bool `json:"accepted"`
			}
			err = api.rpc(ctx, "session/cancel", map[string]any{"request": map[string]any{"sessionId": id}}, &receipt)
			if err == nil && !receipt.Accepted {
				err = errors.New("неподдерживаемое подтверждение отмены")
			}
		}
		return cancelResult{sessionID: id, err: err}
	}
}

func (m *model) chatSessionChanging() bool {
	if m.sessionBusy && (m.sessionAction == "new" || m.sessionAction == "connect") {
		return true
	}
	for _, action := range m.sessionQueue {
		if action.action == "new" || action.action == "connect" {
			return true
		}
	}
	return false
}

func failureNote(code string) string {
	switch code {
	case "MISSING_CREDENTIAL", "INVALID_CREDENTIAL":
		return "Нет действующего доступа к модели. Настройте провайдера в dsh."
	case "NO_ADAPTER", "MODEL_NOT_FOUND":
		return "Выбранная модель недоступна. Проверьте провайдера и модель в dsh."
	case "QUOTA", "ACCOUNT_QUOTA":
		return "Квота провайдера исчерпана."
	case "RATE_LIMIT":
		return "Провайдер ограничил частоту запросов."
	case "CONTEXT_WINDOW_EXCEEDED":
		return "Превышен размер контекста модели."
	default:
		return "Ошибка выполнения агента. Проверьте настройки модели и журнал dsh."
	}
}

func contentText(content []struct {
	Type string `json:"type"`
	Text string `json:"text"`
}) string {
	parts := []string{}
	for _, part := range content {
		if part.Type == "text" {
			parts = append(parts, part.Text)
		} else if part.Type == "reasoning" || part.Type == "tool-call" {
			continue
		} else {
			parts = append(parts, "[Нетекстовое содержимое]")
		}
	}
	return clean(strings.Join(parts, "\n"))
}

func (a *chatAttempt) chunk(chunk chatChunk) string {
	if a.blocks == nil {
		a.blocks = make(map[int]string)
	}
	switch chunk.Type {
	case "text-delta":
		a.blocks[chunk.Index] += chunk.Text
	case "block-end":
		if chunk.Block.Type == "text" {
			a.blocks[chunk.Index] = chunk.Block.Text
		}
	case "finish":
		if chunk.Reason.Kind == "error" {
			return failureNote(chunk.Reason.Failure.Code)
		}
	}
	return ""
}

func (a *chatAttempt) text() string {
	indices := make([]int, 0, len(a.blocks))
	for index := range a.blocks {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	parts := []string{}
	for _, index := range indices {
		parts = append(parts, a.blocks[index])
	}
	return clean(strings.Join(parts, "\n"))
}

func (a *chatAttempt) restore() string {
	note := ""
	for _, record := range a.Stream {
		if record.Type == "text-chunks" {
			for _, text := range record.Texts {
				a.chunk(chatChunk{Type: "text-delta", Index: record.Index, Text: text})
			}
		} else if record.Type == "chunk" {
			if n := a.chunk(record.Chunk); n != "" {
				note = n
			}
		}
	}
	a.Stream = nil
	return note
}

func (m *model) applyChatEvent(s *chatState, event chatEvent) error {
	var data struct {
		CallID    string `json:"callId"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		Error     *struct {
			Code string `json:"code"`
		} `json:"error"`
		Turn    int `json:"turn"`
		Step    int `json:"step"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Source struct {
			RPCID string `json:"rpcId"`
			Kind  string `json:"kind"`
		} `json:"source"`
		Message struct {
			ToolCallID string `json:"toolCallId"`
			Source     struct {
				CallID string `json:"callId"`
			} `json:"source"`
			IsError bool `json:"isError"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
		Stream []compactChatRecord `json:"stream"`
		Reason struct {
			Kind  string `json:"kind"`
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		} `json:"reason"`
	}
	switch event.Type {
	case "user/message", "assistant/message", "assistant/attempt", "turn/start", "turn/end", "tool/call", "tool/result":
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return errors.New("неподдерживаемые данные истории")
		}
	}
	text, role := "", ""
	var tool *chatTool
	switch event.Type {
	case "user/message":
		if data.Source.Kind == "user" || data.Source.Kind == "" {
			role = "ВЫ"
			if data.Source.Kind == "" {
				role = "СООБЩЕНИЕ DSH"
			}
			text = contentText(data.Content)
		}
		if p := m.prompts[s.id]; p != nil && p.ID == data.Source.RPCID {
			delete(m.prompts, s.id)
		}
	case "assistant/message":
		role = "АГЕНТ"
		text = contentText(data.Message.Content)
	case "assistant/attempt":
		a := &chatAttempt{Stream: data.Stream}
		if note := a.restore(); note != "" {
			s.note = note
		}
		role = "АГЕНТ · НЕЗАВЕРШЁННЫЙ ОТВЕТ"
		text = a.text()
	case "tool/call":
		if data.CallID == "" {
			return errors.New("вызов инструмента без идентификатора")
		}
		tool = &chatTool{ID: data.CallID, Name: boundedToolText(data.Name, 256), Arguments: boundedToolText(data.Arguments, toolArgumentsLimit), State: "running", Turn: data.Turn}
	case "tool/result":
		id := data.Message.Source.CallID
		if id == "" {
			id = data.Message.ToolCallID
		}
		if id == "" {
			return errors.New("результат инструмента без идентификатора")
		}
		tool = &chatTool{ID: id, State: "done", Turn: data.Turn}
		for _, previous := range s.messages {
			if previous.tool != nil && previous.tool.ID == id {
				*tool = *previous.tool
				break
			}
		}
		tool.State = "done"
		tool.Result = boundedToolText(contentText(data.Message.Content), chatTextLimit)
		if data.Message.IsError || data.Error != nil {
			tool.State = "error"
			if data.Error != nil {
				tool.ErrorCode = boundedToolText(data.Error.Code, 128)
				if data.Error.Code == "ABORTED" {
					tool.State = "stopped"
				}
			}
		}
	case "turn/start":
		if m.session != nil && m.session.ID == s.id {
			m.session.Running = true
		}
		s.note = ""
	case "turn/end":
		for i := range s.messages {
			previous := s.messages[i].tool
			if previous == nil || previous.Turn != data.Turn || previous.State != "running" {
				continue
			}
			next := *previous
			next.State = "unknown"
			if data.Reason.Kind == "aborted" {
				next.State = "stopped"
			}
			s.messages[i].tool = &next
		}
		if m.session != nil && m.session.ID == s.id {
			m.session.Running = false
		}
		if data.Reason.Kind == "error" {
			s.note = failureNote(data.Reason.Error.Code)
		}
		if data.Reason.Kind == "aborted" {
			s.note = "Выполнение остановлено."
		}
	}
	if len(text) > chatTextLimit {
		return errors.New("сообщение превышает размер, поддерживаемый TUI")
	}
	removed := make(map[int]bool)
	if len(event.SurfaceOp) > 0 && string(event.SurfaceOp) != "\"append\"" {
		var op struct {
			Op    string `json:"op"`
			Start int    `json:"startSeq"`
			End   int    `json:"endSeq"`
		}
		if json.Unmarshal(event.SurfaceOp, &op) != nil || op.Op != "replace" {
			return errors.New("неподдерживаемая замена истории")
		}
		start, end := -1, -1
		for i, seq := range s.surface {
			if seq == op.Start {
				start = i
			}
			if seq == op.End {
				end = i
			}
		}
		if start < 0 || end < start {
			return errors.New("замена затрагивает недоступную часть истории; используйте новую сессию")
		}
		for _, seq := range s.surface[start : end+1] {
			removed[seq] = true
		}
		s.surface = append(append(append([]int{}, s.surface[:start]...), event.Seq), s.surface[end+1:]...)
	} else if len(event.SurfaceOp) > 0 || role != "" || tool != nil {
		s.surface = append(s.surface, event.Seq)
	}
	kept := s.messages[:0]
	for _, message := range s.messages {
		if !removed[message.seq] && !(tool != nil && message.tool != nil && tool.ID == message.tool.ID) {
			kept = append(kept, message)
		}
	}
	s.messages = kept
	if text != "" || tool != nil {
		message := chatMessage{seq: event.Seq, role: role, text: text, tool: tool}
		s.messages = append(s.messages, message)
	}
	positions := make(map[int]int, len(s.surface))
	for i, seq := range s.surface {
		positions[seq] = i
	}
	sort.SliceStable(s.messages, func(i, j int) bool { return positions[s.messages[i].seq] < positions[s.messages[j].seq] })
	bytes := 0
	for _, message := range s.messages {
		bytes += message.byteSize()
	}
	for len(s.messages) > 100 || bytes > 4*chatTextLimit {
		bytes -= s.messages[0].byteSize()
		s.messages = s.messages[1:]
		s.hasMore = true
	}
	if len(s.surface) > 2048 {
		s.surface = s.surface[len(s.surface)-2048:]
		s.hasMore = true
		present := make(map[int]bool, len(s.surface))
		for _, seq := range s.surface {
			present[seq] = true
		}
		kept := s.messages[:0]
		for _, message := range s.messages {
			if present[message.seq] {
				kept = append(kept, message)
			}
		}
		s.messages = kept
	}
	if s.live != nil && (event.Type == "assistant/message" || event.Type == "assistant/attempt") && (s.live.committedSeq != nil && *s.live.committedSeq == event.Seq || s.live.Turn == data.Turn && s.live.Step == data.Step) {
		s.live = nil
	}
	return nil
}

func (m *model) applyChatFrame(s *chatState, frame *chatFrame) error {
	switch frame.Type {
	case "snapshot":
		if frame.Header.ID != s.id || frame.Cursor < -1 {
			return errors.New("снимок другой сессии или неверный курсор")
		}
		next := *s
		next.messages = nil
		next.surface = nil
		next.live = nil
		next.cursor = frame.Cursor
		next.hasMore = frame.HasMore
		next.note = ""
		last := -1
		for _, record := range frame.Records {
			if record.Type != "event" || record.Event.Seq > frame.Cursor || record.Event.Seq <= last {
				return errors.New("неподдерживаемый снимок истории")
			}
			last = record.Event.Seq
			if err := m.applyChatEvent(&next, record.Event); err != nil {
				return err
			}
		}
		if frame.AssistantStream != nil && frame.AssistantStream.Active != nil {
			next.live = frame.AssistantStream.Active
			if next.live.ID == "" || next.live.Next < 0 {
				return errors.New("неподдерживаемый снимок ответа")
			}
			if note := next.live.restore(); note != "" {
				next.note = note
			}
			if len(next.live.text()) > chatTextLimit {
				return errors.New("снимок ответа превышает ограничение TUI")
			}
		}
		next.ready = true
		next.streamError = ""
		*s = next
		if frame.Header.CWD != "" {
			m.workspace = clean(frame.Header.CWD)
		}
	case "event":
		if !s.ready {
			return errors.New("история пришла до снимка")
		}
		if frame.Event.Seq <= s.cursor {
			return nil
		}
		if frame.Event.Seq != s.cursor+1 {
			return errors.New("пропуск в истории; требуется новый снимок")
		}
		if err := m.applyChatEvent(s, frame.Event); err != nil {
			return err
		}
		s.cursor = frame.Event.Seq
	case "assistant-stream":
		if !s.ready {
			return errors.New("ответ пришёл до снимка")
		}
		f := frame.Frame
		if f.ID == "" || f.Index < 0 {
			return errors.New("неверная попытка ответа")
		}
		switch f.Type {
		case "start":
			s.live = &chatAttempt{ID: f.ID, Turn: f.Turn, Step: f.Step}
		case "chunk":
			if s.live == nil || s.live.ID != f.ID {
				return errors.New("неизвестная попытка ответа")
			}
			if f.Index < s.live.Next {
				return nil
			}
			if f.Index != s.live.Next {
				return errors.New("пропуск фрагмента ответа")
			}
			next := *s.live
			next.blocks = make(map[int]string, len(s.live.blocks))
			for index, text := range s.live.blocks {
				next.blocks[index] = text
			}
			note := next.chunk(f.Chunk)
			if len(next.text()) > chatTextLimit {
				return errors.New("ответ превышает размер, поддерживаемый TUI")
			}
			next.Next++
			s.live = &next
			if note != "" {
				s.note = note
			}
		case "end":
			if s.live == nil {
				return nil
			}
			if s.live.ID != f.ID || s.live.Next != f.Index {
				return errors.New("неполный ответ")
			}
			if f.Outcome.Kind == "committed" {
				s.live.committedSeq = &f.Outcome.Seq
				if f.Outcome.Seq <= s.cursor {
					s.live = nil
				}
			} else if f.Outcome.Kind == "abandoned" {
				s.live = nil
			} else {
				return errors.New("неподдерживаемое завершение ответа")
			}
		default:
			return errors.New("неподдерживаемый фрагмент ответа")
		}
	default:
		return errors.New("неподдерживаемый поток чата")
	}
	pruneToolExpansion(s)
	return nil
}

func (m *model) chatView() string {
	m.chatToolHits = nil
	s := m.chat
	if s == nil {
		return "Ожидание подключения чата…"
	}
	lines := []string{}
	if !s.ready {
		lines = append(lines, "◌ Подключение чата…")
	}
	if s.hasMore {
		lines = append(lines, muted.Render("Показан последний фрагмент истории; полная навигация появится позже."))
	}
	row := strings.Count(ansi.Hardwrap(strings.Join(lines, "\n"), max(1, m.viewport.Width), true), "\n")
	for _, message := range s.messages {
		if message.seq <= s.clearedThrough {
			continue
		}
		if len(lines) > 0 {
			row++
		}
		part := ""
		if message.tool != nil {
			card := m.toolView(message.tool)
			m.chatToolHits = append(m.chatToolHits, chatToolHit{id: message.tool.ID, row: row, end: row + strings.Count(ansi.Hardwrap(card, max(1, m.viewport.Width), true), "\n") + 1})
			part = card + "\n"
		} else {
			part = accent.Bold(true).Render(message.role) + "\n" + message.text + "\n"
		}
		lines = append(lines, part)
		row += strings.Count(ansi.Hardwrap(part, max(1, m.viewport.Width), true), "\n")
	}
	if p := m.prompts[s.id]; p != nil {
		label := map[string]string{"sending": "ОТПРАВЛЯЕТСЯ", "accepted": "ПРИНЯТО DSH · ОЖИДАНИЕ ИСТОРИИ", "unknown": "РЕЗУЛЬТАТ ОТПРАВКИ НЕИЗВЕСТЕН · /retry-send", "rejected": "НЕ ОТПРАВЛЕНО"}[p.Status]
		lines = append(lines, heading.Render("ВЫ · "+label)+"\n"+clean(p.Text)+"\n")
	}
	if s.live != nil {
		lines = append(lines, accent.Bold(true).Render("АГЕНТ · ПОТОКОВЫЙ ОТВЕТ")+"\n"+s.live.text()+"\n")
	}
	if len(s.messages) == 0 && s.live == nil && s.ready {
		lines = append(lines, "Введите сообщение. Enter — отправить в dsh, F8 — остановить агента.")
	}
	if s.note != "" {
		lines = append(lines, "! "+s.note)
	}
	if s.streamError != "" {
		lines = append(lines, "! "+s.streamError+" · чтение восстанавливается автоматически")
	}
	return strings.Join(lines, "\n")
}

func (m *model) handleChatUpdate(msg chatUpdate) tea.Cmd {
	s := m.chat
	if s == nil || s.epoch != msg.epoch || s.ctx.Err() != nil {
		return nil
	}
	bottom := m.viewport.AtBottom()
	if msg.notice.err != nil {
		s.ready = false
		s.streamError = msg.notice.err.Error()
	} else if msg.notice.frame != nil {
		if err := m.applyChatFrame(s, msg.notice.frame); err != nil {
			// Reopen a fresh snapshot rather than guessing after a cursor/index gap.
			id := s.id
			m.stopChat()
			m.chat = nil
			cmd := m.startChat(id)
			m.chat.messages = s.messages
			m.chat.live = s.live
			m.chat.clearedThrough = s.clearedThrough
			m.chat.expandedTools = s.expandedTools
			m.chat.selectedTool = s.selectedTool
			m.chat.streamError = err.Error()
			m.rebuild()
			ctx := m.chat.ctx
			return func() tea.Msg {
				timer := time.NewTimer(time.Second)
				defer timer.Stop()
				select {
				case <-ctx.Done():
					return nil
				case <-timer.C:
					return cmd()
				}
			}
		}
	}
	m.rebuild()
	if bottom && m.page == chatPage {
		m.viewport.GotoBottom()
	}
	return waitChat(s)
}

func (m *model) handlePromptResult(msg promptResult) {
	p := m.prompts[msg.sessionID]
	if p == nil || p.ID != msg.requestID {
		return
	}
	if msg.err == nil {
		p.Status = "accepted"
	} else if msg.rejected {
		p.Status = "rejected"
	} else {
		p.Status = "unknown"
	}
	if m.chat != nil && m.chat.id == msg.sessionID {
		if msg.err != nil {
			m.chat.note = msg.err.Error()
			if msg.rejected && m.input.Value() == "" {
				m.input.SetValue(p.Text)
				m.input.CursorEnd()
			}
		}
		m.rebuild()
	}
}

func (m *model) composerText() (string, string) {
	if m.settingsLogin != "" && !m.browserPreview {
		if m.session == nil {
			return " · dsh", "Сначала /new или /session <id>"
		}
		return " · dsh", "Enter — отправить · [F8 Стоп]"
	}
	return " · локальный черновик", "Enter — сохранить · ↑/↓ — история"
}
