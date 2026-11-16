//go:build !js

package main

import (
	"context"
	"encoding/json"
	"time"
)

func followToolOutput(ctx context.Context, login, sessionID, jobID string, out chan<- toolOutput) {
	state := toolOutput{}
	post := func() bool {
		select {
		case out <- state:
			return true
		case <-ctx.Done():
			return false
		}
	}
	delay := time.Second
	for ctx.Err() == nil {
		api, err := openAPI(ctx, login)
		if err == nil {
			request := map[string]any{"sessionId": sessionID, "jobId": jobID}
			if state.Opened {
				request["from"] = state.Cursor
			}
			_ = readMuxStream(ctx, api, "job/follow", map[string]any{"request": request}, func(raw json.RawMessage) bool {
				var frame jobOutputFrame
				if json.Unmarshal(raw, &frame) != nil || state.accept(frame, sessionID, jobID) != nil {
					return false
				}
				delay = time.Second
				return post() && !state.Done
			})
			api.close()
		}
		if state.Done || ctx.Err() != nil {
			return
		}
		state.Stale = true
		if !post() {
			return
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(15*time.Second, delay*2)
	}
}
