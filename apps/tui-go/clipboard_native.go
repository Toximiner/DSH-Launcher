//go:build !js

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aymanbagabas/go-osc52/v2"
)

func writeClipboard(ctx context.Context, value string) (string, error) {
	return writeNativeClipboard(ctx, value, os.Getenv, exec.LookPath, os.Stdout)
}
func writeNativeClipboard(ctx context.Context, value string, env func(string) string, lookPath func(string) (string, error), out io.Writer) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if value == "" || len(value) > clipboardTextLimit {
		return "", errors.New("Недопустимый размер фрагмента.")
	}
	if env("SSH_CONNECTION") == "" && env("SSH_TTY") == "" {
		var candidates [][]string
		if env("WAYLAND_DISPLAY") != "" {
			candidates = append(candidates, []string{"wl-copy", "--type", "text/plain;charset=utf-8"})
		}
		if env("DISPLAY") != "" {
			candidates = append(candidates, []string{"xclip", "-selection", "clipboard", "-in"}, []string{"xsel", "--clipboard", "--input"})
		}
		for _, candidate := range candidates {
			path, err := lookPath(candidate[0])
			if err != nil {
				continue
			}
			runCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			command := exec.CommandContext(runCtx, path, candidate[1:]...)
			command.Stdin = strings.NewReader(value)
			err = command.Run()
			cancel()
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if err == nil {
				return "скопировано в системный буфер.", nil
			}
		}
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	sequence := osc52.New(value).Limit(clipboardTextLimit)
	if env("TMUX") != "" {
		sequence = sequence.Tmux()
	} else if strings.HasPrefix(env("TERM"), "screen") {
		sequence = sequence.Screen()
	}
	encoded := sequence.String()
	n, err := io.WriteString(out, encoded)
	if err != nil || n != len(encoded) {
		return "", errors.New("Не удалось отправить запрос буферу терминала.")
	}
	return "запрос OSC 52 отправлен; запись терминал не подтверждает.", nil
}
