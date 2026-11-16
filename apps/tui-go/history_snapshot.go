package main

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"sort"
)

const historyIndexEvents = 100000
const historyIndexBytes = 64 << 20

type historyReplacement struct {
	Op    string `json:"op"`
	Start int    `json:"startSeq"`
	End   int    `json:"endSeq"`
}
type historyIndexedTool struct {
	chatTool
	seq     int
	result  bool
	visible bool
}
type historyMetadata struct {
	seq         int
	surface     bool
	replacement *historyReplacement
	tool        *historyIndexedTool
}
type historySnapshot struct {
	cut       int
	visible   map[int]bool
	rank      map[int]int
	tools     map[string]*historyIndexedTool
	positions []int
}

func validateHistoryPage(page historyPage, cut, before int) (int, error) {
	if page.Cursor != cut || before < 0 || before > cut+1 || len(page.Records) > 4096 {
		return 0, errors.New("Некорректный снимок истории.")
	}
	first, last := -1, -1
	for _, record := range page.Records {
		seq := record.Event.Seq
		if record.Type != "event" || seq < 0 || seq >= before || last >= 0 && seq != last+1 {
			return 0, errors.New("Пропуск или неверный порядок событий истории.")
		}
		if first < 0 {
			first = seq
		}
		last = seq
	}
	if before > 0 && last != before-1 || before == 0 && len(page.Records) != 0 || page.HasMore != (first > 0) {
		return 0, errors.New("Неполная страница истории.")
	}
	return first, nil
}

// Build one immutable metadata index. Text bodies stay in the requested page only.
// Folding in forward surface order also handles numerically reversed endpoints.
func buildHistorySnapshot(ctx context.Context, cut int, read func(context.Context, int) (historyPage, error)) (*historySnapshot, historyPage, error) {
	var latest historyPage
	var metadata []historyMetadata
	before, bytes := cut+1, 0
	for {
		if ctx.Err() != nil {
			return nil, latest, errors.New("Индекс истории: чтение прервано или истекло время.")
		}
		page, err := read(ctx, before)
		if err != nil {
			return nil, latest, errors.New("Не удалось прочитать индекс истории. F5 — повторить.")
		}
		first, err := validateHistoryPage(page, cut, before)
		if err != nil {
			return nil, latest, err
		}
		if before == cut+1 {
			latest = page
		}
		for i := len(page.Records) - 1; i >= 0; i-- {
			event := page.Records[i].Event
			bytes += len(event.Data) + len(event.SurfaceOp)
			if len(metadata) >= historyIndexEvents || bytes > historyIndexBytes {
				return nil, latest, errors.New("Индекс истории превышает 100 000 событий / 64 МиБ. Текущая страница сохранена.")
			}
			meta := historyMetadata{seq: event.Seq}
			if len(event.SurfaceOp) > 0 {
				meta.surface = true
				if string(event.SurfaceOp) != `"append"` {
					op := &historyReplacement{}
					if json.Unmarshal(event.SurfaceOp, op) != nil || op.Op != "replace" || op.Start < 0 || op.End < 0 || op.Start >= event.Seq || op.End >= event.Seq {
						return nil, latest, errors.New("Некорректная замена истории.")
					}
					meta.replacement = op
				}
			}
			switch event.Type {
			case "tool/call", "tool/result":
				var data struct {
					CallID, Name, Arguments string
					Turn                    int
					Message                 struct {
						ToolCallID string
						Source     struct{ CallID string }
					}
				}
				if json.Unmarshal(event.Data, &data) != nil {
					return nil, latest, errors.New("Некорректные данные инструмента в истории.")
				}
				id := data.CallID
				if event.Type == "tool/result" {
					id = data.Message.Source.CallID
					if id == "" {
						id = data.Message.ToolCallID
					}
				}
				if id == "" || len(id) > 1024 {
					return nil, latest, errors.New("Некорректный идентификатор инструмента.")
				}
				meta.tool = &historyIndexedTool{chatTool: chatTool{ID: id, Name: boundedToolText(data.Name, 256), Arguments: boundedToolText(data.Arguments, toolArgumentsLimit), Turn: data.Turn, StartSeq: event.Seq}, seq: event.Seq, result: event.Type == "tool/result"}
			case "tool/ptc-dispatch-start", "tool/ptc-dispatch":
				var data nestedToolEvent
				if json.Unmarshal(event.Data, &data) != nil || data.ID == "" || len(data.ID) > 1024 || data.ParentID == "" || len(data.ParentID) > 1024 || data.RootID == "" || len(data.RootID) > 1024 {
					return nil, latest, errors.New("Некорректная связь вложенного инструмента.")
				}
				meta.tool = &historyIndexedTool{chatTool: chatTool{ID: data.ID, Name: boundedToolText(data.Name, 256), Arguments: boundedToolText(string(data.Arguments), toolArgumentsLimit), ParentID: data.ParentID, RootID: data.RootID, Nested: true, StartSeq: event.Seq}, seq: event.Seq, result: event.Type == "tool/ptc-dispatch"}
			}
			metadata = append(metadata, meta)
		}
		if !page.HasMore {
			break
		}
		before = first
	}
	index := &historySnapshot{cut: cut, visible: map[int]bool{}, rank: map[int]int{}, tools: map[string]*historyIndexedTool{}}
	surface := list.New()
	nodes := map[int]*list.Element{}
	surfaceTools := map[int]string{}
	retiredAt := map[string]int{}
	for i := len(metadata) - 1; i >= 0; i-- {
		if ctx.Err() != nil {
			return nil, latest, errors.New("Индекс истории: чтение прервано.")
		}
		meta := metadata[i]
		if meta.surface {
			if op := meta.replacement; op != nil {
				start, end := nodes[op.Start], nodes[op.End]
				if start == nil || end == nil {
					return nil, latest, errors.New("Замена ссылается на отсутствующее сообщение истории.")
				}
				// Check ordering before changing the surface.
				found := false
				for e := start; e != nil; e = e.Next() {
					if e == end {
						found = true
						break
					}
				}
				if !found {
					return nil, latest, errors.New("Обратный порядок границ замены истории.")
				}
				node := surface.InsertBefore(meta.seq, start)
				for e := start; ; {
					next := e.Next()
					removedSeq := e.Value.(int)
					if id := surfaceTools[removedSeq]; id != "" {
						retiredAt[id] = meta.seq
					}
					delete(nodes, removedSeq)
					surface.Remove(e)
					if e == end {
						break
					}
					e = next
				}
				nodes[meta.seq] = node
			} else {
				nodes[meta.seq] = surface.PushBack(meta.seq)
			}
		}
		if tool := meta.tool; tool != nil {
			if previous := index.tools[tool.ID]; previous != nil {
				tool.StartSeq = previous.StartSeq
				if tool.Name == "" {
					tool.Name = previous.Name
				}
				if tool.Arguments == "" {
					tool.Arguments = previous.Arguments
				}
			}
			index.tools[tool.ID] = tool
			if meta.surface && tool.result {
				surfaceTools[meta.seq] = tool.ID
			}
		}
	}
	rank := 0
	for e := surface.Front(); e != nil; e = e.Next() {
		seq := e.Value.(int)
		index.visible[seq] = true
		index.rank[seq] = rank
		rank++
	}
	for _, tool := range index.tools {
		tool.visible = tool.Nested || !tool.result || index.visible[tool.seq]
		if tool.Nested {
			for _, id := range []string{tool.ParentID, tool.RootID} {
				if seq, ok := retiredAt[id]; ok && tool.StartSeq <= seq {
					tool.visible = false
					break
				}
			}
		}
	}
	// Retire nested descendants without repeated walks of deep chains.
	children := map[string][]string{}
	var removed []string
	for id, tool := range index.tools {
		if !tool.visible {
			removed = append(removed, id)
		}
		if tool.Nested {
			children[tool.ParentID] = append(children[tool.ParentID], id)
			children[tool.RootID] = append(children[tool.RootID], id)
		}
	}
	for i := 0; i < len(removed); i++ {
		for _, id := range children[removed[i]] {
			if tool := index.tools[id]; tool.visible {
				tool.visible = false
				removed = append(removed, id)
			}
		}
	}
	for seq := range index.rank {
		index.positions = append(index.positions, seq)
	}
	sort.Ints(index.positions)

	return index, latest, nil
}

func projectIndexedHistory(page historyPage, id string, cut, before int, index *historySnapshot) (*chatState, int, error) {
	if index == nil {
		return projectHistory(page, id, cut, before)
	}
	if index.cut != cut {
		return nil, 0, errors.New("Индекс относится к другому снимку истории.")
	}
	first, err := validateHistoryPage(page, cut, before)
	if err != nil {
		return nil, 0, err
	}
	normalized := page
	normalized.Records = append([]chatRecord(nil), page.Records...)
	for i := range normalized.Records {
		if len(normalized.Records[i].Event.SurfaceOp) > 0 {
			normalized.Records[i].Event.SurfaceOp = json.RawMessage(`"append"`)
		}
	}
	projection, _, err := projectHistory(normalized, id, cut, before)
	if err != nil {
		return nil, 0, err
	}
	kept := projection.messages[:0]
	for _, message := range projection.messages {
		if tool := message.tool; tool != nil {
			info := index.tools[tool.ID]
			if info == nil || !info.visible || info.seq != message.seq {
				continue
			}
			tool.Name, tool.Arguments, tool.StartSeq = info.Name, info.Arguments, info.StartSeq
			tool.ParentID, tool.RootID, tool.Nested = info.ParentID, info.RootID, info.Nested
		} else {
			// Non-surface notices remain trace entries; replaced surface messages do not.
			isSurface := false
			for _, record := range page.Records {
				if record.Event.Seq == message.seq {
					isSurface = len(record.Event.SurfaceOp) > 0
					break
				}
			}
			if isSurface && !index.visible[message.seq] {
				continue
			}
		}
		kept = append(kept, message)
	}
	projection.messages = kept
	// Give trace entries a stable position after their preceding retained surface node.
	position := func(message chatMessage) int {
		seq := message.seq
		if message.tool != nil && message.tool.Nested {
			if root := index.tools[message.tool.RootID]; root != nil {
				seq = root.seq
			}
		}
		if rank, ok := index.rank[seq]; ok {
			return 2 * rank
		}
		previous := sort.SearchInts(index.positions, seq) - 1
		if previous < 0 {
			return -1
		}
		return 2*index.rank[index.positions[previous]] + 1
	}
	sort.SliceStable(projection.messages, func(i, j int) bool { return position(projection.messages[i]) < position(projection.messages[j]) })
	return projection, first, nil
}
