//go:build !js

package main

import (
	"context"
	"encoding/json"
	"errors"
)

func firstBenchFrame(ctx context.Context, api *dshAPI, method string, args map[string]any) (json.RawMessage, error) {
	var frame json.RawMessage
	err := readMuxStream(ctx, api, method, args, func(raw json.RawMessage) bool { frame = append(json.RawMessage(nil), raw...); return false })
	if len(frame) > 0 {
		return frame, nil
	}
	if err == nil {
		err = errors.New("stream ended without snapshot")
	}
	return nil, err
}

// Read a bounded output snapshot through the existing byte-cursor validator.
func readBenchJob(ctx context.Context, api *dshAPI, session, id string) (string, error) {
	state := toolOutput{}
	total := int64(-1)
	var invalid error
	request := map[string]any{"jobId": id}
	if session != "" {
		request["sessionId"] = session
	}
	err := readMuxStream(ctx, api, "job/follow", map[string]any{"request": request}, func(raw json.RawMessage) bool {
		var frame jobOutputFrame
		if json.Unmarshal(raw, &frame) != nil {
			invalid = errors.New("invalid output")
			return false
		}
		if invalid = state.accept(frame, session, id); invalid != nil {
			return false
		}
		if frame.Type == "opened" {
			total = frame.Job.Output.Total
		}
		return !state.Done && state.Cursor < total
	})
	if invalid != nil || err != nil || !state.Opened {
		return "", errors.New("Не удалось прочитать вывод задания; F5 обновить.")
	}
	label := "Вывод задания · снимок · F5 обновить\n"
	if state.Gap {
		label += "Часть вывода недоступна (разрыв/лимит).\n"
	}
	if state.Text == "" {
		label += "Пока нет вывода. F5 — перечитать.\n"
	}
	return label + state.Text, nil
}
