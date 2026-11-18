//go:build !js

package main

import (
	"context"
	"encoding/json"
	"time"
)

func followArchive(ctx context.Context, login string, state *archiveState) {
	post := func(ids map[string]bool, err error) bool {
		select {
		case state.inbox <- archiveUpdate{state: state, ids: ids, err: err}:
			return true
		case <-ctx.Done():
			return false
		}
	}
	delay := time.Second
	for ctx.Err() == nil {
		api, err := openAPI(ctx, login)
		if err == nil {
			baseline := false
			var frameErr error
			err = readMuxStream(ctx, api, "workspace/follow", map[string]any{}, func(raw json.RawMessage) bool {
				ids, changed, parseErr := parseArchiveFrame(raw, baseline)
				if parseErr != nil {
					frameErr = parseErr
					return false
				}
				baseline = true
				delay = time.Second
				return !changed || post(ids, nil)
			})
			if frameErr != nil {
				err = frameErr
			}
			api.close()
		}
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = context.Canceled
		}
		if !post(nil, err) {
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
