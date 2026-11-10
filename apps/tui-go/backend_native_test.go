//go:build !js

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func marketFixture(t *testing.T) (backendOptions, string) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	os.MkdirAll(bin, 0700)
	t.Setenv("HOME", root)
	t.Setenv("DSH_HOME", filepath.Join(root, "dsh"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("PATH", bin)
	dsh := filepath.Join(bin, "dsh")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$HOME/calls"
/bin/mkdir -p "$DSH_HOME/profiles/$3"
printf '{"dependencies":{"dshmarket":"1.0.0"}}' > "$DSH_HOME/profiles/$3/package.json"
`
	os.WriteFile(dsh, []byte(script), 0700)
	return backendOptions{Bin: dsh, Profile: "web", CWD: root, Market: "ask"}, root
}
func TestMarketChoicesAndInstalledDetection(t *testing.T) {
	options, root := marketFixture(t)
	var out bytes.Buffer
	if _, err := prepareMarket(context.Background(), options, strings.NewReader("s\n"), &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "calls")); !os.IsNotExist(err) {
		t.Fatal("market installed without consent")
	}
	if _, err := prepareMarket(context.Background(), options, strings.NewReader("n\n"), &out); err != nil || !marketNever() {
		t.Fatal("never choice was not saved", err)
	}
	out.Reset()
	if _, err := prepareMarket(context.Background(), options, strings.NewReader(""), &out); err != nil || out.Len() != 0 {
		t.Fatal("disabled market prompt returned", err)
	}
	manifest := filepath.Join(root, "dsh/profiles/web/package.json")
	os.MkdirAll(filepath.Dir(manifest), 0700)
	os.WriteFile(manifest, []byte(`{"dsh":{"profile":{"bundles":["dshmarket"]}}}`), 0600)
	if !marketInstalled("web") {
		t.Fatal("bundled marketplace was not detected")
	}
}
func TestMarketInstallUsesProfileAndUserLocalPnpm(t *testing.T) {
	options, root := marketFixture(t)
	bin := filepath.Join(root, "bin")
	npm := `#!/bin/sh
printf '%s\n' "$*" > "$HOME/npm-calls"
/bin/mkdir -p "$3/node_modules/.bin"
printf '#!/bin/sh\nexit 0\n' > "$3/node_modules/.bin/pnpm"
/bin/chmod 700 "$3/node_modules/.bin/pnpm"
`
	os.WriteFile(filepath.Join(bin, "npm"), []byte(npm), 0700)
	var out bytes.Buffer
	env, err := prepareMarket(context.Background(), options, strings.NewReader("i\n"), &out)
	if err != nil || !marketInstalled("web") {
		t.Fatal("market install failed", err)
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls"))
	if strings.TrimSpace(string(calls)) != "plugin --profile web add dshmarket" {
		t.Fatal("wrong plugin install command")
	}
	npmCalls, _ := os.ReadFile(filepath.Join(root, "npm-calls"))
	if strings.Contains(string(npmCalls), " -g ") || !strings.Contains(string(npmCalls), "pnpm@10") {
		t.Fatal("pnpm was not installed locally")
	}
	if !strings.Contains(strings.Join(env, "\n"), "node_modules/.bin") {
		t.Fatal("local pnpm was not supplied to dsh")
	}
	out.Reset()
	if _, err = prepareMarket(context.Background(), options, strings.NewReader(""), &out); err != nil || out.Len() != 0 {
		t.Fatal("installed market was prompted again")
	}
}
func TestMarketFailureCanBeSkipped(t *testing.T) {
	options, root := marketFixture(t)
	os.WriteFile(filepath.Join(root, "bin/pnpm"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	os.WriteFile(options.Bin, []byte("#!/bin/sh\nexit 1\n"), 0700)
	var out bytes.Buffer
	if _, err := prepareMarket(context.Background(), options, strings.NewReader("i\ns\n"), &out); err != nil {
		t.Fatal("failed install could not be skipped", err)
	}
	if marketInstalled("web") {
		t.Fatal("failed install was treated as success")
	}
}

func TestOwnedBackendStartsAndStopsWithoutPrintingToken(t *testing.T) {
	options, root := marketFixture(t)
	script := `#!/bin/sh
printf 'dsh web: http://127.0.0.1:3205/?token=private-backend-token\n'
exec /bin/sleep 30
`
	os.WriteFile(options.Bin, []byte(script), 0700)
	options.Port = 3205
	login, stop, err := startBackend(context.Background(), options, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(login, "?token=") {
		stop()
		t.Fatal("backend login URL was not discovered")
	}
	stop()
	if _, err := os.Stat(filepath.Join(root, "calls")); !os.IsNotExist(err) {
		t.Fatal("backend launch unexpectedly installed a plugin")
	}
}
