package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestMarkdownPreservesCodeLinksUnicodeAndTableContent(t *testing.T) {
	source := "# Heading **bold**\n\nA *small* paragraph with `a < b && c` and [docs](https://example.test/a).\n\n3. first\n4. second\n   - nested\n\n> quoted text\n\n```go\n\tfmt.Println(\"世界\")\n\n// a < b & c\n```\n\n| Name | Value |\n| :--- | ---: |\n| 世界 | 42 |\n| Кириллица | 🙂 |\n"
	rendered := renderMarkdown(source, 50)
	plain := ansi.Strip(rendered)
	for _, want := range []string{"Heading bold", "a < b && c", "docs", "(https://example.test/a)", "3. first", "4. second", "• nested", "│ quoted text", "Код · go", "    fmt.Println(\"世界\")", "// a < b & c", "世界", "Кириллица", "42", "🙂", "┬"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing %q in:\n%s", want, plain)
		}
	}
	for _, line := range strings.Split(rendered, "\n") {
		if ansi.StringWidth(line) > 50 {
			t.Fatal("rendered line overflowed", line)
		}
	}
	if strings.Contains(plain, "```go") || strings.Contains(plain, "**bold**") {
		t.Fatal("Markdown source leaked into formatted output")
	}
}

func TestMarkdownNarrowTablesStackWithoutLosingCells(t *testing.T) {
	source := "| Description | Result |\n| --- | --- |\n| " + strings.Repeat("Long content ", 8) + " | FINAL CELL |\n"
	formatted := ansi.Strip(renderMarkdown(source, 50))
	if !strings.Contains(formatted, "Description:") || !strings.Contains(formatted, "Result: FINAL CELL") || strings.Count(strings.Join(strings.Fields(formatted), " "), "Long content") != 8 {
		t.Fatal("narrow table lost content", formatted)
	}
	for _, line := range strings.Split(formatted, "\n") {
		if ansi.StringWidth(line) > 50 {
			t.Fatal("stacked table overflowed")
		}
	}
}

func TestMarkdownDecodedEntitiesCannotInjectTerminalControls(t *testing.T) {
	source := "Visible &#27;[2J &#x9b;31m &amp; \\*literal\\*\n\n[label](https://example.test/&#27;[2J)\n\n<script>still literal</script>"
	rendered := renderMarkdown(source, 80)
	plain := ansi.Strip(rendered)
	if strings.Contains(plain, "\x1b") || strings.Contains(plain, "\u009b") || !strings.Contains(plain, "& *literal*") || !strings.Contains(plain, "<script>still literal</script>") {
		t.Fatal("decoded controls or literal HTML were mishandled", plain)
	}
	// Strip would hide an injected CSI; inspect the actual renderer output too.
	if strings.Contains(rendered, "\x1b[2J") || strings.Contains(rendered, "\u009b") || strings.Contains(rendered, "\x1b]") {
		t.Fatal("renderer emitted a terminal action")
	}
}

func TestMarkdownSourceRemainsAuthoritativeAcrossHistoryToggle(t *testing.T) {
	m := chatModel(t)
	h := historyTestReader(t, m)
	page := historyFixture(0, 0, 0)
	page.Records[0].Event.Type = "assistant/message"
	page.Records[0].Event.Data = []byte(`{"message":{"content":[{"type":"text","text":"# Title\n\n**retained source**"}]}}`)
	m.handleHistoryResult(historyResult{reader: h, cut: 0, before: 1, page: page})
	source := h.projection.messages[0].text
	if strings.Contains(ansi.Strip(h.viewport.View()), "**retained source**") {
		t.Fatal("history did not format assistant text")
	}
	h.selectedSeq = 0
	m.historyKey(tea.KeyMsg{Type: tea.KeyCtrlR})
	if !h.rawMarkdown || !strings.Contains(ansi.Strip(h.viewport.View()), "**retained source**") || h.projection.messages[0].text != source || h.selectedSeq != 0 {
		t.Fatal("raw toggle changed source or selected event")
	}
	m.historyKey(tea.KeyMsg{Type: tea.KeyCtrlR})
	if h.rawMarkdown || h.projection.messages[0].text != source {
		t.Fatal("formatted toggle rewrote history")
	}
	user := chatMessage{role: "ВЫ", text: "**literal user text**"}
	if m.messageText(user, 50, false) != user.text {
		t.Fatal("user input was reformatted")
	}
}

func TestMarkdownLargeInputAndCacheStayBounded(t *testing.T) {
	large := strings.Repeat("x", markdownInputLimit+1)
	if got := renderMarkdown(large, 50); strings.ReplaceAll(got, "\n", "") != large {
		t.Fatal("large-input plain fallback lost content")
	}
	m := newModel()
	for i := 1; i <= 192; i++ {
		m.messageText(chatMessage{role: "АГЕНТ", text: strings.Repeat("x", i)}, 50, false)
	}
	if len(m.markdown.entries) > 128 || m.markdown.bytes > markdownCacheLimit {
		t.Fatal("Markdown cache grew without bound")
	}
}

func TestMarkdownHistorySelectionUsesRenderedRows(t *testing.T) {
	m := chatModel(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 42})
	h := historyTestReader(t, m)
	page := historyFixture(0, 1, 1)
	page.Records[0].Event.Type = "assistant/message"
	page.Records[0].Event.Data = []byte(`{"message":{"content":[{"type":"text","text":"# Title\n\n| A | B |\n| --- | --- |\n| x | y |"}]}}`)
	m.handleHistoryResult(historyResult{reader: h, cut: 1, before: 2, page: page})
	h.viewport.GotoTop()
	row := h.messageRows[1]
	box := m.historyBox()
	m.historyMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 4 + row.start, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if h.selectedSeq != 1 {
		t.Fatal("table layout changed clicked event identity")
	}
	m.forkHistorySelection()
	if m.forkDialog == nil || *m.forkDialog.atSeq != 1 {
		t.Fatal("formatted rows changed fork boundary")
	}
	m.forkDialog = nil
	m.editHistorySearch()
	m.historyKey(tea.KeyMsg{Type: tea.KeyCtrlR})
	if h.rawMarkdown {
		t.Fatal("query editing toggled history source mode")
	}
}
