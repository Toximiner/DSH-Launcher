//go:build !js

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackendClientsInteropBothCloseOrders(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js not installed")
	}
	module, err := filepath.Abs("../../src/backend-clients.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, nodeOwns := range []bool{false, true} {
		t.Run(fmt.Sprint("nodeOwner=", nodeOwns), func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("XDG_DATA_HOME", root)
			bin := filepath.Join(root, "backend")
			os.WriteFile(bin, []byte("#!/bin/sh\nprintf 'dsh web: http://127.0.0.1:32101/?token=fixture\\n'\nwhile :; do printf 'still alive\\n'; /bin/sleep 0.05; done\n"), 0700)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var stop func()
			var done <-chan struct{}
			if !nodeOwns {
				_, stop, done, err = startBackendLogged(ctx, backendOptions{Bin: bin, CWD: root, Profile: "web", Port: 32101}, os.Environ(), io.Discard)
				if err != nil {
					t.Fatal(err)
				}
				defer stop()
			}
			// This separate process uses the actual GUI lease implementation. It also
			// exercises inherited regular-file output when the starting client exits.
			script := `const fs=require('fs'),cp=require('child_process'); const m=require(process.argv[1]);
const client=m.joinBackend(32101); let backend;
if(process.argv[3]==='true') { const fd=fs.openSync(process.argv[2]+'.log','a',0o600); backend=cp.spawn(process.argv[2],[],{stdio:['ignore',fd,fd],detached:true}); backend.unref(); backend.once('spawn',()=>{m.rememberBackend(32101,backend.pid);console.log('ready')}); }
else console.log('ready');
process.stdin.once('data',()=>{ const last=client.release(); const pid=client.target(); if(last&&pid)process.kill(-pid,'SIGTERM'); process.exit(0); });`
			peer := exec.Command(node, "-e", script, module, bin, fmt.Sprint(nodeOwns))
			input, _ := peer.StdinPipe()
			output, _ := peer.StdoutPipe()
			peer.Stderr = os.Stderr
			if err := peer.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { peer.Process.Kill(); peer.Wait() }()
			scanner := bufio.NewScanner(output)
			if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "ready" {
				t.Fatal("peer failed to join")
			}
			client, err := joinBackend(32101)
			if err != nil {
				t.Fatal(err)
			}
			var backend backendIdentity
			if err := readJSON(filepath.Join(client.dir, "managed.json"), &backend); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if identityAlive(backend) {
					exec.Command("/bin/kill", "-KILL", fmt.Sprint(-backend.PID)).Run()
				}
			}()
			if nodeOwns {
				fmt.Fprintln(input, "close")
				if err := peer.Wait(); err != nil {
					t.Fatal(err)
				}
				if !identityAlive(backend) {
					t.Fatal("closing GUI lease killed backend used by TUI")
				}
				if !client.release() {
					t.Fatal("last TUI client not recognized")
				}
				client.stopManaged()
			} else {
				if client.release() {
					t.Fatal("live GUI client was ignored")
				}
				cancel() // Stop log collection as when the TUI process exits.
				time.Sleep(150 * time.Millisecond)
				if !identityAlive(backend) {
					t.Fatal("backend lost its output when owner detached")
				}
				fmt.Fprintln(input, "close")
				if err := peer.Wait(); err != nil {
					t.Fatal(err)
				}
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("last GUI client did not stop backend")
				}
			}
			if identityAlive(backend) {
				t.Fatal("last client left managed backend running")
			}
		})
	}
}
