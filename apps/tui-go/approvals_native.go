//go:build !js

package main

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

func followApprovalEvents(ctx context.Context, login, id string, out chan<- approvalNotice) {
	post := func(n approvalNotice) bool {
		select {
		case out <- n:
			return true
		case <-ctx.Done():
			return false
		}
	}
	delay := time.Second
	for generation := 1; ctx.Err() == nil; generation++ {
		connection, cancel := context.WithCancel(ctx)
		api, err := openAPI(connection, login)
		if err == nil {
			clientID := ""
			answer := func(eventID, outcome string) error {
				return api.rpc(connection, "$events/result", map[string]any{"clientId": clientID, "eventId": eventID, "outcome": map[string]any{"kind": "result", "value": outcome}}, nil)
			}
			err = readMuxStream(connection, api, "$events", map[string]any{}, func(raw json.RawMessage) bool {
				var frame approvalFrame
				if json.Unmarshal(raw, &frame) != nil {
					return false
				}
				switch frame.Type {
				case "ready":
					if frame.ClientID == "" || len(frame.ClientID) > 256 || clientID != "" {
						return false
					}
					clientID = frame.ClientID
					return post(approvalNotice{generation: generation, frame: &frame, answer: answer, disconnect: cancel})
				case "waterfall":
					if clientID == "" || frame.ID == "" || len(frame.ID) > 256 {
						return false
					}
					if frame.AgentID != id {
						return true
					}
					if frame.Event != "approval/request" {
						// Delegate unsupported interactions; never invent a question answer.
						return api.rpc(connection, "$events/result", map[string]any{"clientId": clientID, "eventId": frame.ID, "outcome": map[string]any{"kind": "next"}}, nil) == nil
					}
					return post(approvalNotice{generation: generation, frame: &frame})
				case "cancel":
					return post(approvalNotice{generation: generation, frame: &frame})
				case "emit":
					return true
				default:
					return false
				}
			})
			api.close()
		}
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = errors.New("канал разрешений закрыт")
		}
		if !post(approvalNotice{generation: generation, err: err}) {
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
