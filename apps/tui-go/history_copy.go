package main

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

const clipboardTextLimit = 64 << 10

type historyCodeBlock struct{ language, text string }
type historyCodeDialog struct {
	seq, index int
	blocks     []historyCodeBlock
	note       string
}
type historyCopyResult struct {
	reader            *historyReader
	dialog            *historyCodeDialog
	operation, serial int
	label, receipt    string
	err               error
}

func (m *model) selectedHistoryMessage() (chatMessage, bool) {
	h := m.historyReader
	if h == nil || h.busy || h.ctx.Err() != nil || h.projection == nil || m.chat == nil || m.chat.id != h.sessionID || m.chat.epoch != h.epoch {
		return chatMessage{}, false
	}
	for _, message := range h.projection.messages {
		if message.seq == h.selectedSeq && message.seq >= 0 && message.seq <= h.cut && historyForkable(message) {
			return message, true
		}
	}
	return chatMessage{}, false
}

func extractHistoryCode(source string) ([]historyCodeBlock, error) {
	if len(source) > markdownInputLimit {
		return nil, errors.New(tr("Сообщение больше 256 КиБ: список блоков недоступен. Выделите нужный фрагмент в терминале."))
	}
	bytes := []byte(source)
	document := goldmark.New().Parser().Parse(text.NewReader(bytes))
	var blocks []historyCodeBlock
	depth, nodes := 0, 0
	var parseErr error
	ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			depth--
			return ast.WalkContinue, nil
		}
		depth++
		nodes++
		if depth > 64 || nodes > 20000 {
			parseErr = errors.New(tr("Слишком сложное сообщение для разбора кода."))
			return ast.WalkStop, nil
		}
		language := ""
		switch n := node.(type) {
		case *ast.FencedCodeBlock:
			language = string(n.Language(bytes))
		case *ast.CodeBlock:
		default:
			return ast.WalkContinue, nil
		}
		if len(blocks) >= 64 {
			parseErr = errors.New(tr("В сообщении больше 64 блоков кода; уточните фрагмент."))
			return ast.WalkStop, nil
		}
		blocks = append(blocks, historyCodeBlock{language: language, text: string(node.Lines().Value(bytes))})
		return ast.WalkSkipChildren, nil
	})
	return blocks, parseErr
}

func (m *model) copyHistoryMessage() tea.Cmd {
	message, ok := m.selectedHistoryMessage()
	if !ok {
		if m.historyReader != nil {
			m.historyReader.note = tr("Tab/Shift+Tab или клик — выберите сообщение для копирования.")
		}
		return nil
	}
	return m.requestHistoryCopy(message.text, fmt.Sprintf(tr("Сообщение #%d"), message.seq), nil)
}
func (m *model) openHistoryCode() tea.Cmd {
	message, ok := m.selectedHistoryMessage()
	if !ok {
		if m.historyReader != nil {
			m.historyReader.note = tr("Сначала выберите сообщение: Tab/Shift+Tab или клик.")
		}
		return nil
	}
	h := m.historyReader
	if h.copyBusy {
		return nil
	}
	blocks, err := extractHistoryCode(message.text)
	if err != nil {
		h.note = err.Error()
		return nil
	}
	if len(blocks) == 0 {
		h.note = tr("В выбранном сообщении нет отдельных блоков кода.")
		return nil
	}
	h.codeDialog = &historyCodeDialog{seq: message.seq, blocks: blocks}
	return nil
}
func (m *model) requestHistoryCopy(value, label string, dialog *historyCodeDialog) tea.Cmd {
	h := m.historyReader
	if h == nil || h.copyBusy || h.ctx.Err() != nil {
		return nil
	}
	note := func(value string) {
		h.note = value
		if dialog != nil {
			dialog.note = value
		}
	}
	if value == "" {
		note(tr("Пустой фрагмент: буфер не изменён."))
		return nil
	}
	if len(value) > clipboardTextLimit {
		note(tr("Фрагмент больше 64 КиБ: буфер не изменён. Выберите меньший блок."))
		return nil
	}
	h.copyBusy = true
	h.copyOperation++
	operation, serial, ctx := h.copyOperation, h.serial, h.ctx
	write := m.copyClipboard
	if write == nil {
		write = writeClipboard
	}
	note(tr("Копирование…"))
	return func() tea.Msg {
		receipt, err := write(ctx, value)
		return historyCopyResult{reader: h, dialog: dialog, operation: operation, serial: serial, label: label, receipt: receipt, err: err}
	}
}
func (m *model) handleHistoryCopy(result historyCopyResult) {
	h := m.historyReader
	if h == nil || h != result.reader || h.ctx.Err() != nil || h.copyOperation != result.operation {
		return
	}
	h.copyBusy = false
	if h.serial != result.serial {
		return
	}
	note := result.label + ": " + result.receipt
	if result.err != nil {
		note = tr("Копирование не подтверждено. ") + result.err.Error()
	}
	h.note = note
	if h.codeDialog != nil && h.codeDialog == result.dialog {
		h.codeDialog.note = note
	}
}

func (m *model) historyCopyKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	h := m.historyReader
	if h == nil || h.codeDialog == nil {
		return nil, false
	}
	if !msg.Paste {
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlD, tea.KeyF7, tea.KeyF8, tea.KeyF11:
			return nil, false
		}
	}
	if msg.Paste {
		return nil, true
	}
	d := h.codeDialog
	switch msg.Type {
	case tea.KeyEsc:
		h.codeDialog = nil
	case tea.KeyUp:
		d.index = (d.index + len(d.blocks) - 1) % len(d.blocks)
	case tea.KeyDown:
		d.index = (d.index + 1) % len(d.blocks)
	case tea.KeyEnter:
		return m.requestHistoryCopy(d.blocks[d.index].text, fmt.Sprintf(tr("Код %d из сообщения #%d"), d.index+1, d.seq), d), true
	}
	return nil, true
}
func (m *model) historyCodeBox() area {
	width, height := min(88, m.width-3), min(26, m.height-2)
	return area{(m.width - 1 - width) / 2, (m.height - height) / 2, width, height}
}
func codeWindow(d *historyCodeDialog) (int, int) {
	start := max(0, min(d.index-2, len(d.blocks)-5))
	return start, min(5, len(d.blocks)-start)
}
func (m *model) historyCodeView(base string) string {
	h := m.historyReader
	if h == nil || h.codeDialog == nil {
		return base
	}
	d := h.codeDialog
	box := m.historyCodeBox()
	width := box.width - 4
	lines := []string{heading().Render(tr("Копирование блока кода")), fmt.Sprintf(tr("Сообщение #%d · блок %d/%d"), d.seq, d.index+1, len(d.blocks)), ""}
	start, count := codeWindow(d)
	for i := start; i < start+count; i++ {
		block := d.blocks[i]
		language := clean(block.language)
		if language == "" {
			language = tr("код")
		}
		prefix := "  "
		if i == d.index {
			prefix = "› "
		}
		label := fmt.Sprintf(tr("%s%d · %s · %d байт"), prefix, i+1, language, len(block.text))
		lines = append(lines, ansi.Truncate(label, width, "…"))
	}
	lines = append(lines, "", muted().Render(tr("Фрагмент:")))
	preview := strings.Split(ansi.Hardwrap(clean(strings.ReplaceAll(d.blocks[d.index].text, "\t", "    ")), width, true), "\n")
	available := max(1, box.height-len(lines)-6)
	if len(preview) > available {
		preview = append(preview[:max(0, available-1)], "…")
	}
	lines = append(lines, preview...)
	for len(lines) < box.height-5 {
		lines = append(lines, "")
	}
	note := d.note
	if h.copyBusy {
		note = tr("Копирование…")
	}
	notes := strings.Split(ansi.Hardwrap(note, width, true), "\n")
	for len(notes) < 2 {
		notes = append(notes, "")
	}
	lines = append(lines, muted().Render(notes[0]), muted().Render(notes[1]), tr("↑/↓ — блок · Enter — копировать · Esc — назад"))
	return overlay(base, panel(box.width, box.height, strings.Join(lines, "\n"), "#58a6ff"), box.x, box.y, m.width-1)
}
func (m *model) historyCodeMouse(msg tea.MouseMsg) tea.Cmd {
	h := m.historyReader
	if h == nil || h.codeDialog == nil {
		return nil
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		box := m.historyCodeBox()
		start, count := codeWindow(h.codeDialog)
		if (area{box.x + 2, box.y + 4, box.width - 4, count}).contains(msg.X, msg.Y) {
			h.codeDialog.index = start + msg.Y - (box.y + 4)
		}
	}
	return nil
}
