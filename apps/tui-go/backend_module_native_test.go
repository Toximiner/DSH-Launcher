//go:build !js

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestBundledBackendModuleIsStandaloneAtomicAndDoesNotEditProfile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data with spaces"))
	t.Setenv("DSH_HOME", filepath.Join(root, "profile"))
	const count = 8
	paths := make([]string, count)
	errs := make([]error, count)
	var workers sync.WaitGroup
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func(i int) { defer workers.Done(); paths[i], errs[i] = prepareBackendModule() }(i)
	}
	workers.Wait()
	for i, err := range errs {
		if err != nil || paths[i] != paths[0] {
			t.Fatalf("concurrent preparation %d: %v, %q", i, err, paths[i])
		}
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var patch []struct{ Insert []struct{ ID, Name string } }
	if json.Unmarshal(data, &patch) != nil || len(patch) != 1 || len(patch[0].Insert) != 1 {
		t.Fatal("invalid overlay")
	}
	module := patch[0].Insert[0].Name
	if !filepath.IsAbs(module) || filepath.Dir(module) != filepath.Dir(paths[0]) {
		t.Fatal("module requires repository or profile resolution")
	}
	if data, err := os.ReadFile(module); err != nil || len(data) == 0 {
		t.Fatal("missing bundled module")
	}
	if _, err := os.Stat(os.Getenv("DSH_HOME")); !os.IsNotExist(err) {
		t.Fatal("preparing module modified profile")
	}
	if err := os.WriteFile(module, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareBackendModule(); err == nil {
		t.Fatal("cached module mismatch accepted")
	}
}

func TestBundledBackendModuleRejectsAssetSymlink(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	patch, err := prepareBackendModule()
	if err != nil {
		t.Fatal(err)
	}
	module := filepath.Join(filepath.Dir(patch), "index.mjs")
	external := filepath.Join(t.TempDir(), "outside")
	if err := os.Rename(module, external); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, module); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareBackendModule(); err == nil || !strings.Contains(err.Error(), "invalid backend module asset") {
		t.Fatal("symlinked module accepted", err)
	}
}
