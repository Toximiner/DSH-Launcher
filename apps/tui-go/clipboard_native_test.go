//go:build !js

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func noClipboardHelper(string) (string, error) { return "", errors.New("not installed") }
func TestClipboardOSC52PreservesExactPayloadAndDoesNotClaimAcknowledgement(t *testing.T) {
	value := "\t世界 🙂\nline two\n\x1b[2J"
	var out bytes.Buffer
	receipt, err := writeNativeClipboard(context.Background(), value, func(string) string { return "" }, noClipboardHelper, &out)
	if err != nil || !strings.Contains(receipt, "не подтверждает") {
		t.Fatal("OSC52 claimed confirmed clipboard state", receipt, err)
	}
	sequence := out.String()
	if !strings.HasPrefix(sequence, "\x1b]52;c;") || !strings.HasSuffix(sequence, "\x07") {
		t.Fatal("unexpected clipboard protocol")
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(strings.TrimPrefix(sequence, "\x1b]52;c;"), "\x07"))
	if err != nil || string(decoded) != value || strings.Contains(sequence, "\x1b[2J") {
		t.Fatal("clipboard changed text or exposed terminal controls")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out.Reset()
	if _, err := writeNativeClipboard(ctx, value, func(string) string { return "" }, noClipboardHelper, &out); err == nil || out.Len() != 0 {
		t.Fatal("cancelled write changed clipboard")
	}
	for _, invalid := range []string{"", strings.Repeat("x", clipboardTextLimit+1)} {
		if _, err := writeNativeClipboard(context.Background(), invalid, func(string) string { return "" }, noClipboardHelper, &out); err == nil || out.Len() != 0 {
			t.Fatal("invalid payload emitted clipboard sequence")
		}
	}
}

func TestClipboardHelperReceivesTextOnlyOnStdinAndSSHUsesTerminal(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "copied")
	helper := filepath.Join(directory, "wl-copy")
	t.Setenv("COPY_TEST_OUTPUT", target)
	if err := os.WriteFile(helper, []byte("#!/bin/sh\ncat > \"$COPY_TEST_OUTPUT\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	env := func(name string) string {
		if name == "WAYLAND_DISPLAY" {
			return "wayland-test"
		}
		return ""
	}
	look := func(name string) (string, error) {
		if name != "wl-copy" {
			t.Error("unexpected clipboard command", name)
		}
		return helper, nil
	}
	value := "$(touch should-not-exist)\n`printf unsafe`\n🙂\tend\n"
	var out bytes.Buffer
	receipt, err := writeNativeClipboard(context.Background(), value, env, look, &out)
	if err != nil || !strings.Contains(receipt, "скопировано") || out.Len() != 0 {
		t.Fatal("system clipboard receipt incorrect", receipt, err)
	}
	actual, err := os.ReadFile(target)
	if err != nil || string(actual) != value {
		t.Fatal("clipboard text became arguments or shell code")
	}
	remote := func(name string) string {
		if name == "SSH_CONNECTION" {
			return "test"
		}
		return env(name)
	}
	_, err = writeNativeClipboard(context.Background(), value, remote, func(string) (string, error) { t.Fatal("SSH copied to remote desktop"); return "", nil }, &out)
	if err != nil || !strings.HasPrefix(out.String(), "\x1b]52;c;") {
		t.Fatal("SSH did not use terminal clipboard")
	}
}

type failingClipboardWriter struct{}

func (failingClipboardWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestClipboardWriteFailureIsReported(t *testing.T) {
	if _, err := writeNativeClipboard(context.Background(), "value", func(string) string { return "" }, noClipboardHelper, failingClipboardWriter{}); err == nil {
		t.Fatal("failed terminal write claimed success")
	}
}

func TestClipboardFailedHelperFallsBackWithMultiplexerTransport(t *testing.T) {
	for _, transport := range []string{"TMUX", "TERM"} {
		t.Run(transport, func(t *testing.T) {
			helper := filepath.Join(t.TempDir(), "failed-helper")
			if err := os.WriteFile(helper, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
				t.Fatal(err)
			}
			env := func(name string) string {
				if name == "DISPLAY" {
					return ":test"
				}
				if name == transport {
					if transport == "TERM" {
						return "screen-256color"
					}
					return "tmux-test"
				}
				return ""
			}
			var out bytes.Buffer
			receipt, err := writeNativeClipboard(context.Background(), "payload", env, func(string) (string, error) { return helper, nil }, &out)
			if err != nil || !strings.Contains(receipt, "не подтверждает") || !strings.HasPrefix(out.String(), "\x1bP") || !strings.Contains(out.String(), "cGF5bG9hZA==") {
				t.Fatal("failed helper did not use wrapped OSC 52", receipt, err)
			}
		})
	}
}
