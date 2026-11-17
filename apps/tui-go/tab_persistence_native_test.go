//go:build !js

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestTabPersistenceScopeIgnoresLoginSecretsAndSeparatesStores(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("DSH_HOME", "relative-store")
	cwd := filepath.Join(root, "backend-cwd")
	original, err := tabPersistencePath("http://LOCALHOST:80/?token=first&private=secret", "web", cwd)
	if err != nil {
		t.Fatal(err)
	}
	equivalent, err := tabPersistencePath("http://localhost/path/ignored?token=rotated#fragment", "web", filepath.Join(cwd, "."))
	if err != nil || equivalent != original {
		t.Fatalf("credentials/path/default port changed scope: %q %q %v", original, equivalent, err)
	}
	if filepath.Dir(original) != filepath.Join(root, "config", "dsh-launcher-tui", "tabs") || len(filepath.Base(original)) != 64+len(".json") {
		t.Fatalf("scope path is not an opaque private hash: %s", original)
	}
	for _, value := range []string{"first", "secret", "relative-store", "localhost", "backend-cwd"} {
		if strings.Contains(filepath.Base(original), value) {
			t.Fatal("scope filename exposes identity data")
		}
	}
	for _, tc := range []struct{ login, profile, cwd string }{
		{"http://localhost:81/?token=x", "web", cwd},
		{"https://localhost:80/?token=x", "web", cwd},
		{"http://127.0.0.1/?token=x", "web", cwd},
		{"http://localhost/?token=x", "other", cwd},
		{"http://localhost/?token=x", "web", filepath.Join(root, "other-cwd")},
	} {
		other, e := tabPersistencePath(tc.login, tc.profile, tc.cwd)
		if e != nil || other == original {
			t.Fatalf("distinct store reused scope: %+v %v", tc, e)
		}
	}
	t.Setenv("DSH_HOME", filepath.Join(cwd, "relative-store"))
	absolute, err := tabPersistencePath("http://localhost/?token=last", "web", root)
	if err != nil || absolute != original {
		t.Fatal("relative DSH_HOME did not resolve against backend cwd")
	}
	for _, login := range []string{"", "://invalid", "file:///tmp/store", "http://user:password@localhost", "http://localhost:bad"} {
		if _, err := tabPersistencePath(login, "web", cwd); err == nil {
			t.Fatalf("invalid origin accepted: %s", login)
		}
	}
	first, err := tabPersistencePath("https://[::1]/?token=one", "web", cwd)
	second, e := tabPersistencePath("https://[::1]:443/?token=two", "web", cwd)
	if err != nil || e != nil || first != second {
		t.Fatal("IPv6/default HTTPS port not canonical")
	}
}

func TestTabPersistenceDefaultHomeAndSymlinkScope(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("DSH_HOME", "")
	path, err := tabPersistencePath("http://localhost:3080/?token=x", "", root)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DSH_HOME", filepath.Join(root, ".dsh"))
	explicit, err := tabPersistencePath("http://localhost:3080/?token=y", "web", root)
	if err != nil || explicit != path {
		t.Fatal("default profile/home differs from explicit identity")
	}
	target := filepath.Join(root, "store")
	if err = os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err = os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DSH_HOME", target)
	first, err := tabPersistencePath("http://localhost/", "web", root)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DSH_HOME", alias)
	second, err := tabPersistencePath("http://localhost/", "web", root)
	if err != nil || first != second {
		t.Fatal("same existing store symlink produced another scope")
	}
}

func TestSavedSessionTabsAtomicRoundTripAndMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tabs", "scope.json")
	missing, err := readSavedSessionTabs(path)
	if err != nil || missing.Version != 0 || len(missing.IDs) != 0 || missing.ActiveID != "" {
		t.Fatal("missing file is not an empty first launch")
	}
	initial := savedSessionTabs{Version: 1, IDs: []string{"s-1", "session-世界"}, ActiveID: "s-1"}
	if err = writeSavedSessionTabs(path, initial); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]os.FileMode{path: 0600, filepath.Dir(path): 0700} {
		info, e := os.Stat(file)
		if e != nil || info.Mode().Perm() != want {
			t.Fatalf("private mode differs for %s: %v %v", file, info, e)
		}
	}
	actual, err := readSavedSessionTabs(path)
	if err != nil || !reflect.DeepEqual(actual, initial) {
		t.Fatalf("round trip: %+v %v", actual, err)
	}
	empty := savedSessionTabs{Version: 1, IDs: []string{}}
	if err = writeSavedSessionTabs(path, empty); err != nil {
		t.Fatal(err)
	}
	actual, err = readSavedSessionTabs(path)
	if err != nil || !reflect.DeepEqual(actual, empty) {
		t.Fatal("final deletion cannot persist an empty shell")
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(files) != 1 || files[0].Name() != "scope.json" {
		t.Fatal("atomic writer leaked temporary files")
	}
}

func TestSavedSessionTabsRejectCorruptionAndBoundsWithoutOverwriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tabs.json")
	valid := savedSessionTabs{Version: 1, IDs: []string{"keep"}, ActiveID: "keep"}
	if err := writeSavedSessionTabs(path, valid); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(path)
	invalid := []savedSessionTabs{
		{}, {Version: 2}, {Version: 1, IDs: []string{"same", "same"}},
		{Version: 1, IDs: []string{""}}, {Version: 1, IDs: []string{" "}},
		{Version: 1, IDs: []string{" x"}}, {Version: 1, IDs: []string{"x\t"}},
		{Version: 1, IDs: []string{"x\x7f"}}, {Version: 1, IDs: []string{"x\u0085"}},
		{Version: 1, IDs: []string{strings.Repeat("x", 201)}},
		{Version: 1, IDs: []string{"x"}, ActiveID: "absent"},
	}
	ids := make([]string, 513)
	for i := range ids {
		ids[i] = strings.Repeat("x", i%150+1) + string(rune(0x1000+i))
	}
	invalid = append(invalid, savedSessionTabs{Version: 1, IDs: ids})
	for _, value := range invalid {
		if err := writeSavedSessionTabs(path, value); err == nil {
			t.Fatalf("invalid navigation written: %+v", value)
		}
		current, _ := os.ReadFile(path)
		if string(current) != string(original) {
			t.Fatal("invalid write replaced prior state")
		}
	}
	for _, raw := range []string{`{`, `null`, `{}`, `{"version":0}`, `{"version":2}`, `{"version":1,"ids":["x","x"]}`, `{"version":1,"ids":["x"],"activeId":"y"}`, `{"version":1,"extra":true}`, `{"version":1} {"version":1}`, strings.Repeat(" ", savedSessionTabsLimit+1), "{\"version\":1,\"ids\":[\"\xff\"]}"} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		value, err := readSavedSessionTabs(path)
		if err == nil || value.Version != 0 {
			t.Fatalf("corrupt state accepted: %q %+v %v", raw[:min(80, len(raw))], value, err)
		}
	}
}

func TestSavedSessionTabsMaximumAndConcurrentWritersStayComplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tabs.json")
	ids := make([]string, 512)
	for i := range ids {
		ids[i] = strings.Repeat("x", 197) + string(rune(0x1000+i))
	}
	maximum := savedSessionTabs{Version: 1, IDs: ids, ActiveID: ids[511]}
	if err := writeSavedSessionTabs(path, maximum); err != nil {
		t.Fatal(err)
	}
	read, err := readSavedSessionTabs(path)
	if err != nil || !reflect.DeepEqual(read, maximum) {
		t.Fatal("valid maximum cannot be restored")
	}
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value := savedSessionTabs{Version: 1, IDs: []string{ids[i]}, ActiveID: ids[i]}
			if err := writeSavedSessionTabs(path, value); err != nil {
				t.Error(err)
			}
			if current, err := readSavedSessionTabs(path); err != nil || current.Version != 1 {
				t.Errorf("reader observed partial state: %+v %v", current, err)
			}
		}(i)
	}
	wg.Wait()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var final savedSessionTabs
	if json.Unmarshal(data, &final) != nil || len(final.IDs) != 1 || final.ActiveID != final.IDs[0] {
		t.Fatal("concurrent writers mixed snapshots")
	}
}
