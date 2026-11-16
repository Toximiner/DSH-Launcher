package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestHistoryCopyUsesSourceAndCodeWithoutDisplayChanges(t *testing.T) {
	m := chatModel(t)
	h := historyTestReader(t, m)
	page := historyFixture(0, 0, 0)
	page.Records[0].Event.Type = "assistant/message"
	page.Records[0].Event.Data = []byte(`{"message":{"content":[{"type":"text","text":"# Header\n\n**Unicode 世界 🙂**\n\n~~~go\n\tfmt.Println(\"世界\")\n\n~~~\n\n    indented <code>\n"}]}}`)
	m.handleHistoryResult(historyResult{reader: h, cut: 0, before: 1, page: page})
	h.selectedSeq = 0
	source := h.projection.messages[0].text
	m.input.SetValue("retained draft")
	var copied []string
	m.copyClipboard = func(_ context.Context, value string) (string, error) {
		copied = append(copied, value)
		return "confirmed", nil
	}
	cmd := m.copyHistoryMessage()
	if cmd == nil || !h.copyBusy || len(copied) != 0 {
		t.Fatal("copy should wait for the explicit command")
	}
	if m.copyHistoryMessage() != nil {
		t.Fatal("duplicate clipboard write accepted")
	}
	m.handleHistoryCopy(cmd().(historyCopyResult))
	if len(copied) != 1 || copied[0] != source || h.projection.messages[0].text != source {
		t.Fatal("message copy used rendered text or rewrote source")
	}
	for _, size := range [][2]int{{120, 42}, {100, 30}, {80, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.openHistoryCode()
		d := h.codeDialog
		if d == nil || len(d.blocks) != 2 || d.blocks[0].language != "go" {
			t.Fatal("code extraction lost a block")
		}
		if lipgloss.Width(m.View()) > m.width-1 || lipgloss.Height(m.View()) > m.height {
			t.Fatal("code dialog overflowed")
		}
		cmd, _ = m.historyCopyKey(tea.KeyMsg{Type: tea.KeyEnter})
		m.handleHistoryCopy(cmd().(historyCopyResult))
		if copied[len(copied)-1] != "\tfmt.Println(\"世界\")\n\n" {
			t.Fatal("code copy changed tabs, blank lines, or fence removal", copied[len(copied)-1])
		}
		m.historyCopyKey(tea.KeyMsg{Type: tea.KeyDown})
		cmd, _ = m.historyCopyKey(tea.KeyMsg{Type: tea.KeyEnter})
		m.handleHistoryCopy(cmd().(historyCopyResult))
		if copied[len(copied)-1] != "indented <code>\n" {
			t.Fatal("indented code was not copied as code content")
		}
		m.historyCopyKey(tea.KeyMsg{Type: tea.KeyEsc})
		if h.codeDialog != nil || h.selectedSeq != 0 || m.input.Value() != "retained draft" {
			t.Fatal("code dialog lost history selection or draft")
		}
	}
}

func TestHistoryCopyRejectsInvisibleLargeAndEmptyTargets(t *testing.T) {
	m := chatModel(t)
	h := historyTestReader(t, m)
	calls := 0
	m.copyClipboard = func(context.Context, string) (string, error) { calls++; return "", nil }
	if m.copyHistoryMessage() != nil {
		t.Fatal("missing selection copied")
	}
	if m.requestHistoryCopy("", "empty", nil) != nil || m.requestHistoryCopy(strings.Repeat("x", clipboardTextLimit+1), "large", nil) != nil || calls != 0 {
		t.Fatal("invalid target changed clipboard")
	}
	page := historyFixture(0, 0, 0)
	m.handleHistoryResult(historyResult{reader: h, cut: 0, before: 1, page: page})
	h.selectedSeq = 0
	m.historyKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ctrl+y")})
	if calls != 0 || h.copyBusy {
		t.Fatal("literal key name copied a message")
	}
	m.editHistorySearch()
	m.historyKey(tea.KeyMsg{Type: tea.KeyCtrlY})
	m.historyKey(tea.KeyMsg{Type: tea.KeyCtrlK})
	if h.codeDialog != nil || h.copyBusy {
		t.Fatal("search editor triggered copying")
	}
	h.search.editing = false
	h.busy = true
	if m.copyHistoryMessage() != nil {
		t.Fatal("busy reader copied")
	}
	h.busy = false
	m.chat.epoch++
	if m.copyHistoryMessage() != nil {
		t.Fatal("stale reader copied")
	}
}

func TestHistoryCopyFailureAndLateResultPreserveState(t *testing.T) {
	m := chatModel(t)
	h := historyTestReader(t, m)
	m.copyClipboard = func(context.Context, string) (string, error) { return "", errors.New("clipboard failed") }
	cmd := m.requestHistoryCopy("value", "message", nil)
	result := cmd().(historyCopyResult)
	m.handleHistoryCopy(result)
	if h.copyBusy || !strings.Contains(h.note, "не подтверждено") {
		t.Fatal("failed write claimed success")
	}
	cmd = m.requestHistoryCopy("next", "next", nil)
	result = cmd().(historyCopyResult)
	h.serial++
	h.note = "new page"
	m.handleHistoryCopy(result)
	if h.copyBusy || h.note != "new page" {
		t.Fatal("late receipt changed the newer page")
	}
	m.closeHistory()
	m.handleHistoryCopy(result)
	if m.historyReader != nil {
		t.Fatal("copy result reopened history")
	}
}

func TestHistoryCodeExtractionRejectsPathologicalInput(t *testing.T) {
	if _, err := extractHistoryCode(strings.Repeat("x", markdownInputLimit+1)); err == nil {
		t.Fatal("oversized code parse accepted")
	}
	if _, err := extractHistoryCode(strings.Repeat("```\nx\n```\n\n", 65)); err == nil {
		t.Fatal("too many blocks accepted")
	}
	blocks, err := extractHistoryCode("`inline only`\n\nNo code block.")
	if err != nil || len(blocks) != 0 {
		t.Fatal("inline text became a code block")
	}
}

func TestHistoryCodePickerScrollMouseAndPaste(t *testing.T) {
	m := chatModel(t)
	h := historyTestReader(t, m)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	blocks, err := extractHistoryCode(strings.Repeat("```go\nx\n```\n\n", 12))
	if err != nil {
		t.Fatal(err)
	}
	h.codeDialog = &historyCodeDialog{seq: 0, blocks: blocks}
	for i := 0; i < 9; i++ {
		m.historyCopyKey(tea.KeyMsg{Type: tea.KeyDown})
	}
	start, count := codeWindow(h.codeDialog)
	if start != 7 || count != 5 {
		t.Fatal("selected block is outside the scrolling window")
	}
	box := m.historyCodeBox()
	m.historyCodeMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 4, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if h.codeDialog.index != 7 {
		t.Fatal("mouse selected the wrong block after scrolling")
	}
	for _, key := range []tea.KeyType{tea.KeyEnter, tea.KeyEsc, tea.KeyDown, tea.KeyCtrlD} {
		cmd, handled := m.historyCopyKey(tea.KeyMsg{Type: key, Paste: true})
		if cmd != nil || !handled || h.copyBusy || h.codeDialog == nil || h.codeDialog.index != 7 {
			t.Fatal("paste triggered a hidden modal action")
		}
	}
	if lipgloss.Width(m.View()) > 79 || lipgloss.Height(m.View()) > 24 {
		t.Fatal("scrolling picker overflowed minimum size")
	}
}
