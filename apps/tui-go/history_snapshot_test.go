package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func snapshotReader(page historyPage, size int) func(context.Context, int) (historyPage, error) {
	return func(_ context.Context, before int) (historyPage, error) {
		first := max(0, before-size)
		return historyPage{Cursor: page.Cursor, HasMore: first > 0, Records: page.Records[first:before]}, nil
	}
}

func TestSnapshotReplacementsAcrossPagesAndReversedNumericEndpoints(t *testing.T) {
	page := historyFixture(0, 8, 8)
	page.Records[6].Event.SurfaceOp = json.RawMessage(`{"op":"replace","startSeq":0,"endSeq":1}`)
	page.Records[8].Event.SurfaceOp = json.RawMessage(`{"op":"replace","startSeq":6,"endSeq":4}`)
	read := snapshotReader(page, 3)
	index, latest, err := buildHistorySnapshot(context.Background(), 8, read)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.visible) != 3 || !index.visible[8] || !index.visible[5] || !index.visible[7] {
		t.Fatal("snapshot folded numeric intervals instead of actual surface order", index.visible)
	}
	projection, _, err := projectIndexedHistory(latest, "s", 8, 9, index)
	if err != nil || len(projection.messages) != 2 || projection.messages[0].seq != 8 || projection.messages[1].seq != 7 {
		t.Fatal("replacement page lost authoritative ordering", err)
	}
	older, _ := read(context.Background(), 3)
	projection, _, err = projectIndexedHistory(older, "s", 8, 3, index)
	if err != nil || len(projection.messages) != 0 {
		t.Fatal("older page resurrected replaced messages", err)
	}
	result := scanHistory(context.Background(), historySearchResult{cut: 8, nextBefore: 9, query: "message 006", hitSeq: -1, snapshot: index}, "s", read)
	if result.err != nil || result.hitSeq != -1 || !result.finished {
		t.Fatal("search resurrected a replaced replacement", result.err)
	}
	result = scanHistory(context.Background(), historySearchResult{cut: 8, nextBefore: 9, query: "message 008", hitSeq: -1, snapshot: index}, "s", read)
	if result.err != nil || result.hitSeq != 8 {
		t.Fatal("search missed replacement outside the referenced page", result.err)
	}
}

func TestSnapshotRestoresToolContextAndHidesReplacedNestedDescendants(t *testing.T) {
	page := historyToolsFixture(t, 0, 8)
	extra := historyFixture(8, 8, 8).Records[0]
	extra.Event.SurfaceOp = json.RawMessage(`{"op":"replace","startSeq":4,"endSeq":4}`)
	page.Cursor = 8
	page.Records = append(page.Records, extra)
	read := snapshotReader(page, 4)
	index, _, err := buildHistorySnapshot(context.Background(), 8, read)
	if err != nil {
		t.Fatal(err)
	}
	if index.tools["root"].visible || index.tools["child"].visible || !index.tools["failed"].visible {
		t.Fatal("replacing root result retained obsolete nested tools")
	}
	chunk, _ := read(context.Background(), 8)
	projection, _, err := projectIndexedHistory(chunk, "s", 8, 8, index)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, message := range projection.messages {
		if tool := message.tool; tool != nil {
			if tool.ID != "failed" {
				t.Fatal("replaced tool was restored")
			}
			found = tool.Name == "bash" && strings.Contains(tool.Arguments, "notes.txt") && tool.StartSeq == 5
		}
	}
	if !found {
		t.Fatal("tool context outside result page was not restored")
	}
	result := scanHistory(context.Background(), historySearchResult{cut: 8, nextBefore: 9, query: "nested output", hitSeq: -1, snapshot: index}, "s", read)
	if result.err != nil || result.hitSeq != -1 {
		t.Fatal("search found a retired nested result")
	}
	result = scanHistory(context.Background(), historySearchResult{cut: 8, nextBefore: 9, query: "notes.txt", hitSeq: -1, snapshot: index, scope: historySearchTools}, "s", read)
	if result.err != nil || result.hitSeq != 6 || len(result.hits) != 1 {
		t.Fatal("tool arguments were missing or duplicated across page boundaries", result.err, result.hits)
	}
}

func TestSnapshotRejectsInvalidTransitionsAndCancelledReads(t *testing.T) {
	for _, raw := range []string{`{"op":"replace","startSeq":99,"endSeq":0}`, `{"op":"replace","startSeq":1,"endSeq":0}`, `{"op":"other","startSeq":0,"endSeq":0}`} {
		page := historyFixture(0, 2, 2)
		page.Records[2].Event.SurfaceOp = json.RawMessage(raw)
		if _, _, err := buildHistorySnapshot(context.Background(), 2, snapshotReader(page, 2)); err == nil {
			t.Fatal("invalid replacement accepted", raw)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := buildHistorySnapshot(ctx, 2, func(context.Context, int) (historyPage, error) {
		t.Fatal("cancelled index read another page")
		return historyPage{}, nil
	})
	if err == nil {
		t.Fatal("cancelled index claimed success")
	}
	_, _, err = buildHistorySnapshot(context.Background(), 2, func(context.Context, int) (historyPage, error) {
		return historyPage{}, errors.New("private backend detail")
	})
	if err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("index leaked backend error details")
	}
}

func TestToolSearchScopesExpandExactCardWithoutSelectingMessage(t *testing.T) {
	m := chatModel(t)
	h := historyTestReader(t, m)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	page := historyToolsFixture(t, 0, 7)
	index, _, err := buildHistorySnapshot(context.Background(), 7, snapshotReader(page, 3))
	if err != nil {
		t.Fatal(err)
	}
	m.handleHistoryResult(historyResult{reader: h, cut: 7, before: 8, page: page, snapshot: index})
	m.input.SetValue("retained draft")
	m.editHistorySearch()
	if h.search.editScope != historySearchAll {
		t.Fatal("new search is not all-fields")
	}
	m.historyKey(tea.KeyMsg{Type: tea.KeyCtrlT})
	m.historyKey(tea.KeyMsg{Type: tea.KeyCtrlT})
	if h.search.editScope != historySearchTools || h.search.scope != historySearchAll {
		t.Fatal("scope editing was committed before Enter")
	}
	h.search.editor.SetValue("TAIL_ONLY")
	// Use the pure scanner instead of network; the UI still receives a serial-bound result.
	h.search.scope = h.search.editScope
	h.search.query = "TAIL_ONLY"
	h.search.editing = false
	h.search.running = true
	h.busy = true
	result := scanHistory(context.Background(), historySearchResult{reader: h, serial: h.serial, cut: 7, nextBefore: 8, query: "TAIL_ONLY", hitSeq: -1, snapshot: index, scope: historySearchTools}, "s", snapshotReader(page, 8))
	m.handleHistorySearch(result)
	if !h.toolFocused || h.selectedSeq != -1 || h.projection.selectedTool != "root" || !h.projection.expandedTools["root"] || !strings.Contains(h.viewport.View(), "TAIL_ONLY") {
		t.Fatal("tool search did not reveal the exact matching card and result")
	}
	m.forkHistorySelection()
	if m.forkDialog != nil || m.copyHistoryMessage() != nil || m.input.Value() != "retained draft" {
		t.Fatal("tool search changed draft or selected a message action")
	}
	for _, scope := range []int{historySearchAll, historySearchMessages, historySearchTools} {
		found := scanHistory(context.Background(), historySearchResult{cut: 7, nextBefore: 8, query: "notes.txt", hitSeq: -1, snapshot: index, scope: scope}, "s", snapshotReader(page, 3))
		if found.err != nil || (found.hitSeq >= 0) != (scope != historySearchMessages) {
			t.Fatal("tool scope returned incorrect matches", scope, found.err)
		}
	}
}

func TestSnapshotRootResultRewriteRetiresOldNestedCalls(t *testing.T) {
	page := historyToolsFixture(t, 0, 8)
	data, _ := json.Marshal(toolResult("root", "shortened result", false))
	page.Cursor = 8
	page.Records = append(page.Records, chatRecord{Type: "event", Event: chatEvent{Seq: 8, Type: "tool/result", Data: data, SurfaceOp: json.RawMessage(`{"op":"replace","startSeq":4,"endSeq":4}`)}})
	index, _, err := buildHistorySnapshot(context.Background(), 8, snapshotReader(page, 3))
	if err != nil {
		t.Fatal(err)
	}
	if !index.tools["root"].visible || index.tools["child"].visible {
		t.Fatal("rewriting the root result resurrected old nested context")
	}
	result := scanHistory(context.Background(), historySearchResult{cut: 8, nextBefore: 9, query: "nested output", hitSeq: -1, snapshot: index}, "s", snapshotReader(page, 3))
	if result.err != nil || result.hitSeq != -1 {
		t.Fatal("search resurrected a child of a rewritten root result")
	}
}
