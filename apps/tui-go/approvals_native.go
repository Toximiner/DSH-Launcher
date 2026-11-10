//go:build !js

package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
			reply := func(eventID string, value any) error {
				if strings.HasPrefix(eventID, "continued:") {
					var accepted bool
					err := api.rpc(connection, "userQuestions/answer", map[string]any{"agentId": id, "callId": strings.TrimPrefix(eventID, "continued:"), "answer": value}, &accepted)
					if err == nil && !accepted {
						return errors.New("вопрос уже завершён")
					}
					return err
				}
				return api.rpc(connection, "$events/result", map[string]any{"clientId": clientID, "eventId": eventID, "outcome": map[string]any{"kind": "result", "value": value}}, nil)
			}
			answer := func(eventID, outcome string) error { return reply(eventID, outcome) }
			claims := make(map[string]string)
			hooks := &muxSubscriptions{}
			hooks.closed = func(streamID string) {
				for eventID, id := range claims {
					if id == streamID {
						delete(claims, eventID)
						break
					}
				}
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
					return post(approvalNotice{generation: generation, frame: &frame, answer: answer, reply: reply, disconnect: cancel})
				case "waterfall":
					if clientID == "" || frame.ID == "" || len(frame.ID) > 256 {
						return false
					}
					if frame.AgentID != id {
						return true
					}
					if frame.Event != "approval/request" && frame.Event != "user-questions/request" {
						// Delegate unsupported interactions without inventing an answer.
						return api.rpc(connection, "$events/result", map[string]any{"clientId": clientID, "eventId": frame.ID, "outcome": map[string]any{"kind": "next"}}, nil) == nil
					}
					if frame.Event == "user-questions/request" && frame.Request.Wait != nil && frame.Request.Wait.Timed && frame.Request.Wait.CallID != "" && len(frame.Request.Wait.CallID) <= 1024 {
						if _, exists := claims[frame.ID]; !exists {
							streamID, err := hooks.open("userQuestions/attachWait", map[string]any{"agentId": id, "callId": frame.Request.Wait.CallID})
							if err != nil {
								return false
							}
							claims[frame.ID] = streamID
						}
					}
					return post(approvalNotice{generation: generation, frame: &frame})
				case "cancel":
					if streamID, ok := claims[frame.ID]; ok {
						hooks.cancel(streamID)
						delete(claims, frame.ID)
					}
					return post(approvalNotice{generation: generation, frame: &frame})
				case "emit":
					return true
				default:
					return false
				}
			}, hooks)
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
