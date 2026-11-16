//go:build !js

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAttachBackendHelper(t *testing.T) {
	if os.Getenv("DSH_ATTACH_TEST_HELPER") != "1" {
		return
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "fixture", Path: "/"})
			w.WriteHeader(302)
			return
		}
		if r.URL.Path != "/api/session/list" {
			w.WriteHeader(404)
			return
		}
		var request struct {
			ID string `json:"rpcId"`
		}
		json.NewDecoder(r.Body).Decode(&request)
		json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": request.ID, "result": map[string]any{"ok": true, "value": map[string]any{"items": []any{}}}})
	}))
	fmt.Println("dsh web: " + server.URL + "/?token=attach-fixture-token")
	select {}
}
func attachBackendFixture(t *testing.T) (int, *exec.Cmd) {
	t.Helper()
	outputPath := filepath.Join(t.TempDir(), "backend.log")
	output, err := os.OpenFile(outputPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { output.Close() })
	process := exec.Command(os.Args[0], "-test.run=^TestAttachBackendHelper$")
	process.Env = append(os.Environ(), "DSH_ATTACH_TEST_HELPER=1")
	process.Stdout = output
	process.Stderr = output
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { process.Process.Kill(); process.Wait() })
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(outputPath)
		if line, _, ok := strings.Cut(string(data), "\n"); ok && strings.HasPrefix(line, "dsh web: ") {
			login, err := url.Parse(strings.TrimPrefix(line, "dsh web: "))
			if err != nil {
				t.Fatal(err)
			}
			port, _ := strconv.Atoi(login.Port())
			return port, process
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("backend fixture did not become ready")
	return 0, nil
}
func TestStartAttachesWithoutLaunchingOrClaimingExternalBackend(t *testing.T) {
	c := runtimeFixture(t)
	port, process := attachBackendFixture(t)
	c.config.Port = port
	c.config.Bin = "/missing/dsh"
	// A running backend does not require a local Node/dsh installation.
	t.Setenv("PATH", t.TempDir())
	if err := c.Execute(context.Background(), "start", ""); err != nil {
		t.Fatal(err)
	}
	state := c.Snapshot()
	if state.Owned || !strings.Contains(state.Login, "attach-fixture-token") || c.stop != nil {
		t.Fatal("attachment claimed process ownership")
	}
	if _, err := os.Stat(filepath.Join(sharedBackendDirectory(port), "managed.json")); !os.IsNotExist(err) {
		t.Fatal("external backend was marked as managed")
	}
	c.Close()
	if err := process.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatal("external backend was stopped on client close")
	}
}
func TestOccupiedPortWithoutLoginDoesNotStartASecondProcess(t *testing.T) {
	c := runtimeFixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	c.config.Port = listener.Addr().(*net.TCPAddr).Port
	c.config.Bin = "/missing/dsh"
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	err = c.start(ctx)
	if err == nil || !strings.Contains(err.Error(), strconv.Itoa(c.config.Port)) || c.Snapshot().Owned {
		t.Fatal("occupied port did not produce a useful attachment refusal", err)
	}
}
func TestBackendLoginOutputRequiresExactLocalOrigin(t *testing.T) {
	for _, value := range []string{"http://example.com:3205/?token=secret", "http://127.0.0.1:3206/?token=secret", "http://user@127.0.0.1:3205/?token=secret", "http://127.0.0.1:3205/no?token=secret", "http://127.0.0.1:3205/"} {
		if backendLoginFromOutput("dsh web: "+value, 3205) != "" {
			t.Fatal("unrelated or invalid login URL accepted")
		}
	}
	if backendLoginFromOutput("dsh web: http://127.0.0.1:3205/?token=first\ndsh web: http://127.0.0.1:3205/?token=last\n", 3205) != "http://127.0.0.1:3205/?token=last" {
		t.Fatal("latest matching URL was not selected")
	}
}
