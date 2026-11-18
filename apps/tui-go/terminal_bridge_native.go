//go:build !js

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

type terminalBridge struct {
	ctx                context.Context
	login, session, id string
	in                 io.Reader
	out, stderr        io.Writer
}

func (b *terminalBridge) SetStdin(r io.Reader)  { b.in = r }
func (b *terminalBridge) SetStdout(w io.Writer) { b.out = w }
func (b *terminalBridge) SetStderr(w io.Writer) { b.stderr = w }
func (b *terminalBridge) Run() error {
	file, ok := b.in.(*os.File)
	if !ok {
		return errors.New("terminal requires a native TTY")
	}
	fd := int(file.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer term.Restore(fd, state)
	ctx, cancel := context.WithCancel(b.ctx)
	defer cancel()
	api, err := openAPI(ctx, b.login)
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
	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, Jar: api.client.Jar, HandshakeTimeout: 10 * time.Second}
	conn, response, err := dialer.DialContext(ctx, u.String(), http.Header{"Origin": []string{api.origin}})
	if response != nil {
		response.Body.Close()
	}
	if err != nil {
		return errors.New("terminal connection unavailable")
	}
	defer conn.Close()
	conn.SetReadLimit(16 << 20)
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	id, err := newRequestID()
	if err != nil {
		return err
	}
	send := func(v any) error { conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); return conn.WriteJSON(v) }
	if err = send(map[string]any{"type": "open", "streamId": id, "endpoint": "terminal/attach", "payload": map[string]any{"args": map[string]any{"agentId": b.session, "id": b.id}}}); err != nil {
		return err
	}
	defer send(map[string]any{"type": "cancel", "streamId": id})
	frames := make(chan json.RawMessage, 32)
	fail := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, raw, e := conn.ReadMessage()
			if e != nil {
				select {
				case fail <- errors.New("terminal disconnected; input is not replayed"):
				default:
				}
				return
			}
			select {
			case frames <- raw:
			case <-ctx.Done():
				return
			}
		}
	}()
	defer func() { cancel(); conn.Close(); <-done }()
	io.WriteString(b.out, tr("\x1b[2J\x1b[HDSH terminal · Ctrl+] — вернуться в TUI (процесс остаётся на сервере)\r\n"))
	writable, opened := false, false
	sequence, pending := 0, 0
	lastOutput := -1
	cols, rows := 0, 0
	var buffered []byte
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return nil
		case e := <-fail:
			return e
		case raw := <-frames:
			var env struct {
				Type  string `json:"type"`
				ID    string `json:"streamId"`
				Value struct {
					Type     string `json:"type"`
					Screen   string `json:"screen"`
					Data     string `json:"data"`
					Writable bool   `json:"writable"`
					Sequence int    `json:"sequence"`
					Refusal  string `json:"refusal"`
					Info     struct {
						State string `json:"state"`
					} `json:"info"`
				} `json:"value"`
			}
			if json.Unmarshal(raw, &env) != nil || env.ID != id || env.Type != "item" {
				return errors.New("terminal stream ended")
			}
			f := env.Value
			switch f.Type {
			case "snapshot":
				if opened {
					return errors.New("duplicate terminal snapshot")
				}
				opened = true
				writable = f.Writable
				lastOutput = f.Sequence
				io.WriteString(b.out, f.Screen)
				if f.Info.State != "running" {
					return nil
				}
			case "output":
				if !opened || f.Sequence <= lastOutput {
					return errors.New("invalid terminal output sequence")
				}
				lastOutput = f.Sequence
				io.WriteString(b.out, f.Data)
			case "ack":
				if f.Sequence != pending {
					return errors.New("invalid terminal input acknowledgement")
				}
				pending = 0
				if f.Refusal != "" {
					return errors.New("terminal input refused")
				}
			case "state":
				writable = f.Writable
				if f.Info.State != "running" {
					return nil
				}
			default:
				return errors.New("unknown terminal frame")
			}
		default:
		}
		if opened && writable && pending == 0 {
			c, r, e := term.GetSize(fd)
			if e == nil && (c != cols || r != rows) {
				cols, rows = c, r
				sequence++
				pending = sequence
				if e = send(map[string]any{"type": "item", "streamId": id, "value": map[string]any{"type": "resize", "sequence": sequence, "cols": c, "rows": r}}); e != nil {
					return e
				}
			}
			if pending == 0 && len(buffered) > 0 {
				size := len(buffered)
				for size > 0 && !utf8.Valid(buffered[:size]) {
					size--
				}
				if size == 0 && len(buffered) > 4 {
					return errors.New("input is not UTF-8")
				}
				if size > 0 {
					data := string(buffered[:size])
					buffered = append([]byte(nil), buffered[size:]...)
					sequence++
					pending = sequence
					if e = send(map[string]any{"type": "item", "streamId": id, "value": map[string]any{"type": "write", "sequence": sequence, "data": data}}); e != nil {
						return e
					}
				}
			}
		}
		if len(buffered) < 4096 {
			poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			n, e := unix.Poll(poll, 20)
			if e != nil && e != unix.EINTR {
				return e
			}
			if n > 0 && poll[0].Revents&unix.POLLIN != 0 {
				buf := make([]byte, 1024)
				n, e := unix.Read(fd, buf)
				if e != nil {
					return e
				}
				if n == 0 {
					return nil
				}
				if bytes.IndexByte(buf[:n], 0x1d) >= 0 {
					return nil
				}
				if !opened || writable {
					buffered = append(buffered, buf[:n]...)
				}
			}
		} else {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	return nil
}
func startBenchTerminal(ctx context.Context, login, session, id string) tea.Cmd {
	return tea.Exec(&terminalBridge{ctx: ctx, login: login, session: session, id: id}, func(err error) tea.Msg { return benchTerminalDone{err: err} })
}
