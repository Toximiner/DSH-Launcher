//go:build !js

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func runtimeFixture(t *testing.T) *nativeRuntime {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root)
	t.Setenv("HOME", root)
	bin := filepath.Join(root, "bin")
	os.MkdirAll(bin, 0700)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\necho v24.22.1\n"), 0700)
	npm := `#!/bin/sh
if [ "$1" = view ]; then printf '"0.2.2-rc.1"'; exit 0; fi
if [ -f "$HOME/fail" ]; then echo 'password=fixture-secret' >&2; exit 1; fi
if [ -f "$HOME/hold" ]; then exec /bin/sleep 60; fi
/bin/mkdir -p "$3/node_modules/.bin"
/bin/mkdir -p "$3/node_modules/@deepseek-ai/dsh"
printf '{"version":"0.2.2-rc.1"}' > "$3/node_modules/@deepseek-ai/dsh/package.json"
printf '#!/bin/sh\necho dsh 0.2.2-rc.1\n' > "$3/node_modules/.bin/dsh"
/bin/chmod 700 "$3/node_modules/.bin/dsh"
echo 'installed fixture'
`
	os.WriteFile(filepath.Join(bin, "npm"), []byte(npm), 0700)
	c, e := newNativeRuntime(backendOptions{Bin: "dsh", Profile: "web", CWD: root, Port: 3205})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(c.Close)
	return c
}
func TestManagedDshTransactionsAndCancelledInstall(t *testing.T) {
	c := runtimeFixture(t)
	ctx := context.Background()
	if e := c.Execute(ctx, "install", "latest"); e != nil {
		t.Fatal(e)
	}
	first := c.install.Current
	if first == "" || c.install.Previous != "" {
		t.Fatal("first slot not activated")
	}
	c2, e := newNativeRuntime(backendOptions{Bin: "dsh", Profile: "web", CWD: c.config.CWD, Port: 3205})
	if e != nil || c2.install.Current != first {
		t.Fatal("installation was not durable", e)
	}
	c2.Close()
	os.WriteFile(filepath.Join(c.config.CWD, "fail"), nil, 0600)
	if e = c.Execute(ctx, "update-dsh", "0.2.2-rc.1"); e == nil || c.install.Current != first {
		t.Fatal("failed install changed current slot")
	}
	if strings.Contains(c.Snapshot().Log, "fixture-secret") {
		t.Fatal("install secret leaked")
	}
	os.Remove(filepath.Join(c.config.CWD, "fail"))
	os.WriteFile(filepath.Join(c.config.CWD, "hold"), nil, 0600)
	cancelled, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if e = c.Execute(cancelled, "update-dsh", "0.2.2-rc.1"); e == nil || c.install.Current != first {
		t.Fatal("cancelled install changed current slot")
	}
	os.Remove(filepath.Join(c.config.CWD, "hold"))
	if e = c.Execute(ctx, "update-dsh", "0.2.2-rc.1"); e != nil {
		t.Fatal(e)
	}
	second := c.install.Current
	if c.install.Previous != first || second == first {
		t.Fatal("update did not retain previous slot")
	}
	if e = c.Execute(ctx, "rollback-dsh", ""); e != nil || c.install.Current != first || c.install.Previous != second {
		t.Fatal("rollback failed", e)
	}
	slots, _ := os.ReadDir(filepath.Join(c.root, "versions"))
	if len(slots) != 2 {
		t.Fatal("failed installations were not removed")
	}
	if e = c.Execute(ctx, "install", "--global"); e == nil {
		t.Fatal("option injection accepted")
	}
}
func TestRuntimeOwnershipAndExit(t *testing.T) {
	c := runtimeFixture(t)
	bin := filepath.Join(c.config.CWD, "backend")
	script := `#!/bin/sh
echo 'stderr secret: apiKey=private-owned-key' >&2
echo 'dsh web: http://127.0.0.1:3205/?token=private-login'
exec /bin/sleep 30
`
	os.WriteFile(bin, []byte(script), 0700)
	c.config.Bin = bin
	ctx := context.Background()
	if e := c.Execute(ctx, "stop", ""); e == nil {
		t.Fatal("unowned stop accepted")
	}
	if e := c.Execute(ctx, "start", ""); e != nil {
		t.Fatal(e)
	}
	snapshot := c.Snapshot()
	if !snapshot.Owned || snapshot.Login == "" || strings.Contains(snapshot.Log, "private-login") || strings.Contains(snapshot.Log, "private-owned-key") {
		t.Fatal("owned process state or redaction invalid")
	}
	if e := c.Execute(ctx, "update-dsh", "latest"); e == nil {
		t.Fatal("running installation replaced")
	}
	other, e := newNativeRuntime(backendOptions{Bin: bin, Profile: "web", CWD: c.config.CWD, Port: 3205})
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	if e = other.Execute(ctx, "start", ""); e == nil {
		t.Fatal("second owner acquired runtime lock")
	}
	if e = c.Execute(ctx, "restart", ""); e != nil || !c.Snapshot().Owned {
		t.Fatal("restart failed", e)
	}
	if e = c.Execute(ctx, "stop", ""); e != nil || c.Snapshot().Owned {
		t.Fatal("stop failed", e)
	}
	os.WriteFile(bin, []byte("#!/bin/sh\necho 'startup failed' >&2\nexit 1\n"), 0700)
	if e = c.Execute(ctx, "start", ""); e == nil || !strings.Contains(c.Snapshot().Log, "startup failed") {
		t.Fatal("startup failure missing from log")
	}
}
func TestRuntimeReportMasksAcrossWritesAndExcludesProfile(t *testing.T) {
	c := runtimeFixture(t)
	c.Write([]byte("https://user:private-pass@host/?token=private-token\nAuthorization: Bea"))
	c.Write([]byte("rer private-bearer\n{\"apiKey\":\"private-key\"}\nCookie: private-cookie\npassword=private-password\n"))
	path, e := c.report("")
	if e != nil {
		t.Fatal(e)
	}
	f, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	z, e := gzip.NewReader(f)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	tr := tar.NewReader(z)
	count := 0
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		b, _ := io.ReadAll(tr)
		if bytes.Contains(b, []byte("private-")) {
			t.Fatalf("report secret leaked in %s", h.Name)
		}
		if strings.Contains(h.Name, "profile") || h.Mode != 0600 {
			t.Fatal("unexpected report entry")
		}
		count++
	}
	if count != 4 {
		t.Fatal("missing diagnostic entries")
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal("report not private")
	}
}

type runtimeMock struct {
	calls []string
	state runtimeSnapshot
}

func (c *runtimeMock) Snapshot() runtimeSnapshot { return c.state }
func (c *runtimeMock) Execute(_ context.Context, a, v string) error {
	c.calls = append(c.calls, a+":"+v)
	return nil
}
func (c *runtimeMock) Close() {}
func TestRuntimeDialogConfirmationGeometryAndExternalOwnership(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 30}, {Width: 80, Height: 24}} {
		m := newModel()
		m.Update(size)
		r := &runtimeMock{}
		m.runtime = r
		m.input.SetValue("unsent draft")
		for _, kind := range []string{"runtime", "updates", "diagnostics"} {
			m.openRuntime(kind)
			view := m.View()
			if lipgloss.Height(view) > size.Height {
				t.Fatal("runtime dialog too tall")
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size.Width {
					t.Fatal("runtime dialog too wide")
				}
			}
			key(m, tea.KeyEsc)
		}
		m.openRuntime("runtime")
		m.runtimeDialog.index = 1
		key(m, tea.KeyEnter)
		key(m, tea.KeyEnter)
		if !m.runtimeDialog.confirm || len(r.calls) != 0 {
			t.Fatal("install was not staged for confirmation")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter"), Paste: true})
		if !m.runtimeDialog.confirm || len(r.calls) != 0 {
			t.Fatal("pasted key name confirmed installation")
		}
		key(m, tea.KeyEsc)
		if len(r.calls) != 0 || m.input.Value() != "unsent draft" {
			t.Fatal("cancelled action leaked")
		}
		m.settingsLogin = "http://external/?token=private"
		m.runtimeDialog.index = 2
		key(m, tea.KeyEnter)
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd != nil || len(r.calls) != 0 {
			t.Fatal("attach adopted external ownership")
		}
		key(m, tea.KeyEsc)
		if !m.input.Focused() || m.input.Value() != "unsent draft" {
			t.Fatal("modal lost draft or focus")
		}
	}
}
func TestTuiUpdateValidationAndAtomicRollback(t *testing.T) {
	c := runtimeFixture(t)
	dir := t.TempDir()
	c.executable = filepath.Join(dir, "tui")
	old := []byte("#!/bin/sh\necho 0.2.0-preview.36\n")
	candidate := []byte("#!/bin/sh\necho 0.2.0-preview.37\n")
	os.WriteFile(c.executable, old, 0700)
	path := filepath.Join(dir, "candidate")
	os.WriteFile(path, candidate, 0700)
	a := updateArtifact{Path: path, SHA256: strings.Repeat("0", 64)}
	ctx := context.Background()
	if e := c.updateTUI(ctx, a); e == nil {
		t.Fatal("wrong checksum accepted")
	}
	b, _ := os.ReadFile(c.executable)
	if !bytes.Equal(old, b) {
		t.Fatal("checksum failure modified executable")
	}
	a.SHA256 = hashBytes(candidate)
	if e := c.updateTUI(ctx, a); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(c.executable)
	if !bytes.Equal(candidate, b) {
		t.Fatal("candidate not promoted")
	}
	if e := c.rollbackTUI(ctx); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(c.executable)
	if !bytes.Equal(old, b) {
		t.Fatal("rollback did not restore original")
	}
	os.WriteFile(c.executable+".previous", []byte("corrupt"), 0700)
	if e := c.rollbackTUI(ctx); e == nil {
		t.Fatal("corrupt backup accepted")
	}
	os.WriteFile(path, []byte("not executable"), 0700)
	a.SHA256 = hashBytes([]byte("not executable"))
	if e := c.updateTUI(ctx, a); e == nil {
		t.Fatal("unstartable candidate accepted")
	}
	// The promoted path fails its post-install check; the staged path succeeds.
	failing := []byte("#!/bin/sh\ncase \"$0\" in *.tui-candidate-*) echo 0.2.0-preview.38;; *) exit 1;; esac\n")
	os.WriteFile(path, failing, 0700)
	a.SHA256 = hashBytes(failing)
	if e := c.updateTUI(ctx, a); e == nil {
		t.Fatal("post-install failure accepted")
	}
	b, _ = os.ReadFile(c.executable)
	if !bytes.Equal(old, b) {
		t.Fatal("post-install failure was not restored")
	}
}
func TestRuntimeInvalidSavedConfigDoesNotSilentlyReset(t *testing.T) {
	c := runtimeFixture(t)
	os.WriteFile(filepath.Join(c.root, "runtime.json"), []byte("broken"), 0600)
	_, e := newNativeRuntime(backendOptions{Bin: "dsh", Profile: "web", CWD: c.config.CWD, Port: 3205})
	if e == nil {
		t.Fatal("corrupt saved options silently reset")
	}
}

type runtimeTransport func(*http.Request) (*http.Response, error)

func (f runtimeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestUpdateCatalogSelectsOnlyMatchingTuiArtifact(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	http.DefaultTransport = runtimeTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Host {
		case "registry.npmjs.org":
			body = `{"version":"0.2.2-rc.1"}`
		case "api.github.com":
			if strings.Contains(r.URL.Path, "deepseek-harness") {
				body = `[{"tag_name":"dsh-v0.2.2-rc.1","body":"DSH_CHANGELOG"}]`
			} else {
				body = `[{"tag_name":"v9.0.0","assets":[{"name":"dsh-launcher.deb"}]},{"tag_name":"tui-v0.2.0-preview.38","body":"TUI_CHANGELOG","assets":[{"name":"dsh-launcher-tui-go_linux_` + runtime.GOARCH + `","digest":"sha256:` + strings.Repeat("a", 64) + `","browser_download_url":"https://github.com/fixture/tui"}]}]`
			}
		default:
			t.Fatal("unexpected update host")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})
	catalog, e := fetchUpdateCatalog(context.Background())
	if e != nil || catalog.TUI.Version != "0.2.0-preview.38" || !strings.Contains(catalog.Text, "DSH_CHANGELOG") || !strings.Contains(catalog.Text, "TUI_CHANGELOG") {
		t.Fatalf("incorrect release selection: %+v %v", catalog, e)
	}
	if _, e = httpsBytes(context.Background(), "http://github.com/fixture", 1024); e == nil {
		t.Fatal("unencrypted update accepted")
	}
	if _, e = readLimited(strings.NewReader("12345"), 4); e == nil {
		t.Fatal("oversized update accepted")
	}
}

func TestRuntimeReloadsInstallPointerAfterAcquiringLock(t *testing.T) {
	c := runtimeFixture(t)
	other, e := newNativeRuntime(backendOptions{Bin: "dsh", Profile: "web", CWD: c.config.CWD, Port: 3205})
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	if e = c.Execute(context.Background(), "install", "latest"); e != nil {
		t.Fatal(e)
	}
	first := c.install.Current
	if e = other.Execute(context.Background(), "update-dsh", "latest"); e != nil {
		t.Fatal(e)
	}
	if other.install.Previous != first {
		t.Fatal("stale client lost previous installation")
	}
}
