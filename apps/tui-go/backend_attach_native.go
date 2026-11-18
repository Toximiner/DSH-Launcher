//go:build !js

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Match the same local stdout discovery contract as src/system.js. Never read
// pipes/devices or another user's output, and never emit the discovered token.
var backendLoginLine = regexp.MustCompile(`dsh web:\s*(https?://\S+)`)

func backendLoginFromOutput(data string, port int) string {
	login := ""
	for _, match := range backendLoginLine.FindAllStringSubmatch(clean(data), -1) {
		u, err := settingsURL(match[1])
		if err == nil && u.Scheme == "http" && u.Host == net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) {
			login = u.String()
		}
	}
	return login
}
func backendListenerPID(port int) int {
	inodes := map[string]bool{}
	for _, name := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 10 || fields[3] != "0A" || !strings.HasSuffix(fields[1], fmt.Sprintf(":%04X", port)) {
				continue
			}
			address, _, _ := strings.Cut(fields[1], ":")
			if address == "0100007F" || address == "00000000" || address == "00000000000000000000000000000000" || address == "0000000000000000FFFF00000100007F" {
				inodes[fields[9]] = true
			}
		}
	}
	if len(inodes) == 0 {
		return 0
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		directory := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(directory)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(directory, fd.Name()))
			if err != nil {
				continue
			}
			if strings.HasPrefix(target, "socket:[") && strings.HasSuffix(target, "]") && inodes[strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")] {
				return pid
			}
		}
	}
	return 0
}
func backendLoginFromListener(port int) string {
	pid := backendListenerPID(port)
	before, ok := processIdentity(pid)
	if !ok {
		return ""
	}
	file, err := os.OpenFile(fmt.Sprintf("/proc/%d/fd/1", pid), os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOCTTY, 0)
	if err != nil {
		return ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return ""
	}
	const limit = 64 << 10
	buffer := make([]byte, min(info.Size(), limit))
	n, _ := file.ReadAt(buffer, 0)
	login := backendLoginFromOutput(string(buffer[:n]), port)
	if info.Size() > limit {
		n, _ = file.ReadAt(buffer, info.Size()-int64(len(buffer)))
		if value := backendLoginFromOutput(string(buffer[:n]), port); value != "" {
			login = value
		}
	}
	if !identityAlive(before) {
		return ""
	}
	return login
}
func runningBackend(ctx context.Context, port int) (string, bool, error) {
	connection, err := (&net.Dialer{Timeout: 400 * time.Millisecond}).DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		if ctx.Err() != nil {
			return "", false, ctx.Err()
		}
		if errors.Is(err, syscall.ECONNREFUSED) {
			return "", false, nil
		}
		return "", false, fmt.Errorf(tr("Не удалось проверить порт dsh %d."), port)
	}
	connection.Close()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		if login := backendLoginFromListener(port); login != "" {
			api, err := openAPI(ctx, login)
			if err == nil {
				var sessions struct {
					Items []any `json:"items"`
				}
				err = api.rpc(ctx, "session/list", map[string]any{"_request": map[string]any{}}, &sessions)
				api.close()
				if err == nil && sessions.Items != nil {
					return login, true, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", true, fmt.Errorf(tr("Порт %d занят. Не удалось войти в работающий dsh; используйте --url или DSH_TUI_URL со свежей ссылкой входа. Второй процесс не запущен."), port)
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func (c *nativeRuntime) attachExisting(ctx context.Context) (bool, error) {
	login, found, err := runningBackend(ctx, c.config.Port)
	if err != nil || !found {
		return found, err
	}
	c.mu.Lock()
	c.state.Login = login
	c.state.Owned = false
	c.state.Exited = false
	c.mu.Unlock()
	return true, nil
}
