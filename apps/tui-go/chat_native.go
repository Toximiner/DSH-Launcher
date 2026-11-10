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
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if conn.WriteJSON(map[string]any{"type": "open", "streamId": streamID, "endpoint": "session/follow", "payload": map[string]any{"args": map[string]any{"request": map[string]any{
		"address": map[string]any{"kind": "session", "sessionId": id}, "maxMessages": 50, "assistantStream": true,
	}}}}) != nil {
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
		if json.Unmarshal(data, &envelope) != nil || envelope.ID != streamID {
			return errors.New("неподдерживаемый протокол потока dsh")
		}
		if envelope.Type != "item" {
			return errors.New("подписка чата остановлена сервером; проверьте доступ и версию dsh")
		}
		var frame chatFrame
		if json.Unmarshal(envelope.Value, &frame) != nil {
			return errors.New("неподдерживаемые данные потока dsh")
		}
		if !receive(&frame) {
			return nil
		}
	}
}
