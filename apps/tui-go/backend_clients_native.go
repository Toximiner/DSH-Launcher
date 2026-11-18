//go:build !js

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// This on-disk protocol is shared with src/backend-clients.js. Start ticks
// distinguish a live client or backend from a reused Linux PID.
type backendIdentity struct {
	PID   int    `json:"pid"`
	Start string `json:"start"`
	Group int    `json:"group"`
}

func processIdentity(pid int) (backendIdentity, bool) {
	if pid <= 1 {
		return backendIdentity{}, false
	}
	b, e := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if e != nil {
		return backendIdentity{}, false
	}
	fields := strings.Fields(string(b)[strings.LastIndexByte(string(b), ')')+1:])
	if len(fields) < 20 || fields[0] == "Z" || fields[0] == "X" {
		return backendIdentity{}, false
	}
	group, e := strconv.Atoi(fields[2])
	if e != nil {
		return backendIdentity{}, false
	}
	return backendIdentity{pid, fields[19], group}, true
}
func identityAlive(value backendIdentity) bool {
	current, ok := processIdentity(value.PID)
	return ok && current == value
}
func sharedBackendDirectory(port int) string {
	return filepath.Join(filepath.Dir(runtimeRoot()), "dsh-launcher", "backends", strconv.Itoa(port))
}
func rememberBackend(port, pid int) error {
	value, ok := processIdentity(pid)
	if !ok || value.Group != pid {
		return fmt.Errorf("backend process identity unavailable")
	}
	return atomicJSON(filepath.Join(sharedBackendDirectory(port), "managed.json"), value)
}

type backendClient struct{ dir, lease string }

func joinBackend(port int) (*backendClient, error) {
	dir := sharedBackendDirectory(port)
	value, ok := processIdentity(os.Getpid())
	if !ok {
		return nil, fmt.Errorf("client process identity unavailable")
	}
	lease := filepath.Join(dir, "clients", fmt.Sprintf("%d-%s-%d.json", value.PID, value.Start, time.Now().UnixNano()))
	if e := atomicJSON(lease, value); e != nil {
		return nil, e
	}
	return &backendClient{dir, lease}, nil
}
func (c *backendClient) release() bool {
	if e := os.Remove(c.lease); e != nil && !os.IsNotExist(e) {
		return false
	}
	entries, e := os.ReadDir(filepath.Join(c.dir, "clients"))
	if e != nil {
		return false
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(c.dir, "clients", entry.Name())
		var value backendIdentity
		if e := readJSON(path, &value); e != nil && !os.IsNotExist(e) {
			return false
		}
		if identityAlive(value) {
			return false
		}
		if e := os.Remove(path); e != nil && !os.IsNotExist(e) {
			return false
		}
	}
	return true
}
func (c *backendClient) stopManaged() {
	var record struct {
		backendIdentity
		Previous []backendIdentity `json:"previous"`
	}
	if readJSON(filepath.Join(c.dir, "managed.json"), &record) != nil {
		return
	}
	own, _ := processIdentity(os.Getpid())
	targets := append(record.Previous, record.backendIdentity)
	for _, value := range targets {
		if !identityAlive(value) || value.Group <= 1 || value.Group == own.Group {
			continue
		}
		syscall.Kill(-value.Group, syscall.SIGTERM)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		active := false
		for _, value := range targets {
			if identityAlive(value) && value.Group > 1 && value.Group != own.Group {
				active = true
			}
		}
		if !active {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, value := range targets {
		if identityAlive(value) && value.Group > 1 && value.Group != own.Group {
			syscall.Kill(-value.Group, syscall.SIGKILL)
		}
	}
}
