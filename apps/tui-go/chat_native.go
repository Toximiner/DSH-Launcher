//go:build !js

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

func followChat(ctx context.Context, login, id string, out chan<- chatNotice) {
	post := func(n chatNotice) bool {
		select {
		case out <- n:
			return true
		case <-ctx.Done():
			return false
		}
	}
	delay := time.Second
	for ctx.Err() == nil {
		err := readChatConnection(ctx, login, id, func(frame *chatFrame) bool { return post(chatNotice{frame: frame}) })
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = errors.New("поток чата завершён")
		}
		if !post(chatNotice{err: err}) {
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

func readChatConnection(ctx context.Context, login, id string, receive func(*chatFrame) bool) error {
	api, err := openAPI(ctx, login)
	if err != nil {
		return err
	}
	defer api.close()
	return readMuxStream(ctx, api, "session/follow", map[string]any{"request": map[string]any{
		"address": map[string]any{"kind": "session", "sessionId": id}, "maxMessages": 50, "assistantStream": true,
	}}, func(data json.RawMessage) bool {
		var frame chatFrame
		if json.Unmarshal(data, &frame) != nil {
			return false
		}
		return receive(&frame)
	})
}

type muxSubscriptions struct {
	open   func(string, map[string]any) (string, error)
	cancel func(string)
	closed func(string)
}

func readMuxStream(ctx context.Context, api *dshAPI, endpoint string, args map[string]any, receive func(json.RawMessage) bool, subscriptions ...*muxSubscriptions) error {
	u, _ := url.Parse(api.origin + "/api/remote.mux")
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: 10 * time.Second, Jar: api.client.Jar}
	conn, response, err := dialer.DialContext(ctx, u.String(), http.Header{"Origin": []string{api.origin}})
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		return errors.New("не удалось открыть поток dsh; проверьте URL входа и версию dsh")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	conn.SetReadLimit(16 << 20)
	conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPingHandler(func(data string) error {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(5*time.Second))
	})
	streamID, err := newRequestID()
	if err != nil {
		return errors.New("не удалось подготовить поток")
	}
	additional := make(map[string]bool)
	known := make(map[string]bool)
	if len(subscriptions) > 0 {
		hooks := subscriptions[0]
		hooks.open = func(method string, namedArgs map[string]any) (string, error) {
			if len(additional) >= 32 {
				return "", errors.New("слишком много потоков ожидания")
			}
			id, err := newRequestID()
			if err != nil {
				return "", err
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err = conn.WriteJSON(map[string]any{"type": "open", "streamId": id, "endpoint": method, "payload": map[string]any{"args": namedArgs}})
			if err == nil {
				additional[id] = true
				known[id] = true
			}
			return id, err
		}
		hooks.cancel = func(id string) {
			if !additional[id] {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_ = conn.WriteJSON(map[string]any{"type": "cancel", "streamId": id})
			delete(additional, id)
		}
	}
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if conn.WriteJSON(map[string]any{"type": "open", "streamId": streamID, "endpoint": endpoint, "payload": map[string]any{"args": args}}) != nil {
		return errors.New("не удалось подписаться на сессию")
	}
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return errors.New("соединение с чатом потеряно")
		}
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		var envelope struct {
			Type  string          `json:"type"`
			ID    string          `json:"streamId"`
			Value json.RawMessage `json:"value"`
		}
		if json.Unmarshal(data, &envelope) != nil {
			return errors.New("неподдерживаемый протокол потока dsh")
		}
		if envelope.ID != streamID {
			if !known[envelope.ID] {
				return errors.New("неизвестный поток ожидания dsh")
			}
			if envelope.Type == "end" || envelope.Type == "error" {
				delete(additional, envelope.ID)
				if len(subscriptions) > 0 && subscriptions[0].closed != nil {
					subscriptions[0].closed(envelope.ID)
				}
				// Preserve a bounded tombstone window for late cancellation frames.
				if len(known) > 256 {
					known = make(map[string]bool)
					for id := range additional {
						known[id] = true
					}
				}
			}
			continue
		}
		if envelope.Type != "item" {
			return errors.New("подписка чата остановлена сервером; проверьте доступ и версию dsh")
		}
		if !receive(envelope.Value) {
			return nil
		}
	}
}
