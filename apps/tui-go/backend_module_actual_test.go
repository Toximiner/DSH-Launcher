//go:build !js

package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Opt-in integration: use an existing isolated dsh installation, never a personal
// profile or a network package install. CI also exercises deletion via Python.
func TestBundledBackendBootsActualDsh(t *testing.T) {
	bin := os.Getenv("DSH_TUI_TEST_DSH")
	if bin == "" {
		t.Skip("set DSH_TUI_TEST_DSH to an isolated actual dsh executable")
	}
	bin, err := filepath.EvalSymlinks(bin)
	if err != nil {
		t.Fatal(err)
	}
	modules := filepath.Dir(bin)
	for filepath.Base(modules) != "node_modules" && filepath.Dir(modules) != modules {
		modules = filepath.Dir(modules)
	}
	if filepath.Base(modules) != "node_modules" {
		t.Fatal("fixture dsh must belong to node_modules")
	}
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("DSH_HOME", filepath.Join(root, "dsh"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	profile := filepath.Join(root, "dsh", "profiles", "web")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(modules, filepath.Join(profile, "node_modules")); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"name":"isolated-launcher-module-test","private":true,"dependencies":{},"dsh":{"profile":{"bundles":["@deepseek-ai/dsh-base","@deepseek-ai/dsh-web-app"]}}}`)
	patch := []byte(`[{"id":"session-title-llm","disabled":true},{"id":"goal-round-driver","disabled":true}]`)
	for name, data := range map[string][]byte{"package.json": manifest, "cordis.patch.yml": patch, "cordis.yml": []byte("[]\n")} {
		if err := os.WriteFile(filepath.Join(profile, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	login, stop, err := startBackend(ctx, backendOptions{Bin: bin, Profile: "web", CWD: root, Port: port}, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	api, err := openAPI(ctx, login)
	if err != nil {
		t.Fatal(err)
	}
	defer api.close()
	var caps struct {
		ProtocolVersion int    `json:"protocolVersion"`
		Supported       bool   `json:"supportsPermanentDelete"`
		Reason          string `json:"reason"`
	}
	if err := api.rpc(ctx, "launcherSessions/capabilities", map[string]any{}, &caps); err != nil {
		t.Fatal(err)
	}
	if caps.ProtocolVersion != 1 || !caps.Supported {
		t.Fatalf("bundled module unavailable: %+v", caps)
	}
	after, err := os.ReadFile(filepath.Join(profile, "cordis.patch.yml"))
	if err != nil || string(after) != string(patch) {
		t.Fatal("launcher modified profile patch")
	}
	var pkg map[string]any
	data, _ := os.ReadFile(filepath.Join(profile, "package.json"))
	if json.Unmarshal(data, &pkg) != nil {
		t.Fatal("profile manifest unreadable")
	}
	dependencies, _ := pkg["dependencies"].(map[string]any)
	for name := range dependencies {
		if name == "@michengai/dsh-archive-manager" {
			t.Fatal("third-party deletion plugin was installed")
		}
	}
}
