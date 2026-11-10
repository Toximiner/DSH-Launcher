package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type marketAPIFixture struct {
	mu                sync.Mutex
	installed         map[string]string
	writes            []string
	profile           string
	version           int
	catalogFail, busy bool
}

func fakeMarket(t *testing.T) (*marketAPIFixture, *httptest.Server) {
	t.Helper()
	state := &marketAPIFixture{installed: map[string]string{"alpha-plugin": "1.0.0"}, profile: "test", version: 1}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "test", Path: "/"})
			w.WriteHeader(302)
			return
		}
		if r.Header.Get("Cookie") != "dsh=test" || r.Header.Get("Origin") != "http://"+r.Host {
			t.Error("market authentication headers missing")
		}
		state.mu.Lock()
		defer state.mu.Unlock()
		var value any
		switch r.URL.Path {
		case "/dsh-market/api/v1/capabilities":
			value = map[string]any{"apiVersion": state.version, "profile": state.profile, "marketVersion": "1.73.17"}
		case "/dsh-market/installed":
			present := []string{}
			for name := range state.installed {
				present = append(present, name)
			}
			value = map[string]any{"installed": state.installed, "present": present}
		case "/dsh-market/status":
			value = map[string]any{"busy": state.busy, "active": state.busy, "phase": "packages", "done": 2, "total": 4}
		case "/dsh-market/registry":
			if state.catalogFail {
				w.WriteHeader(502)
				w.Write([]byte("private-token"))
				return
			}
			value = map[string]any{"registry": map[string]any{"plugins": []any{
				marketPlugin{Name: "Alpha", NPM: "alpha-plugin", URL: "https://github.com/test/alpha", Description: map[string]string{"ru": "Первый плагин"}},
				marketPlugin{Name: "Beta", NPM: "beta-plugin", URL: "https://github.com/test/beta", Description: map[string]string{"en": "Second plugin"}},
			}}}
		case "/dsh-market/install", "/dsh-market/update", "/dsh-market/uninstall":
			if r.Method != "POST" {
				t.Error("mutation used wrong method")
			}
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if len(body) != 1 {
				t.Error("unexpected mutation fields", body)
			}
			state.writes = append(state.writes, r.URL.Path)
			switch r.URL.Path {
			case "/dsh-market/install":
				if body["url"] != "https://github.com/test/beta" {
					t.Error("install source differs from selected entry")
				}
				state.installed["beta-plugin"] = "1.0.0"
			case "/dsh-market/update":
				if body["name"] != "beta-plugin" {
					t.Error("wrong update package")
				}
				state.installed["beta-plugin"] = "2.0.0"
			case "/dsh-market/uninstall":
				if body["name"] != "beta-plugin" {
					t.Error("wrong remove package")
				}
				delete(state.installed, "beta-plugin")
			}
			value = map[string]any{"ok": true}
		default:
			t.Error("unexpected market route", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(value)
	}))
	t.Cleanup(server.Close)
	return state, server
}
func settleMarket(t *testing.T, m *model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("missing market command")
	}
	message := cmd()
	if batch, ok := message.(tea.BatchMsg); ok {
		message = batch[0]()
	}
	m.Update(message)
}
func TestMarketConfirmationAndBackendState(t *testing.T) {
	state, server := fakeMarket(t)
	m := newModel()
	m.page = marketPage
	m.settingsLogin = server.URL + "/?token=private-token"
	m.settingsContext = context.Background()
	m.resize()
	settleMarket(t, m, m.loadMarket())
	if len(m.marketRows()) != 2 {
		t.Fatal("catalog not loaded")
	}
	m.marketSearch.SetValue("second")
	if len(m.marketRows()) != 1 || m.marketRows()[0].NPM != "beta-plugin" {
		t.Fatal("catalog search failed")
	}
	m.confirmMarket("install")
	state.mu.Lock()
	before := len(state.writes)
	state.mu.Unlock()
	if before != 0 {
		t.Fatal("operation started before confirmation")
	}
	m.marketKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if m.marketConfirm != nil {
		t.Fatal("cancel did not close confirmation")
	}
	m.confirmMarket("install")
	if cmd, _ := m.marketKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y"), Paste: true}); cmd != nil {
		t.Fatal("pasted text confirmed mutation")
	}
	settleMarket(t, m, m.executeMarket())
	if m.market.Installed["beta-plugin"] != "1.0.0" {
		t.Fatal("install did not reconcile installed state")
	}
	m.confirmMarket("update")
	settleMarket(t, m, m.executeMarket())
	if m.market.Installed["beta-plugin"] != "2.0.0" {
		t.Fatal("update did not reconcile installed state")
	}
	m.confirmMarket("uninstall")
	settleMarket(t, m, m.executeMarket())
	if _, ok := m.market.Installed["beta-plugin"]; ok {
		t.Fatal("removed package still displayed as installed")
	}
	state.mu.Lock()
	writes := len(state.writes)
	state.mu.Unlock()
	if writes != 3 {
		t.Fatal("unexpected mutation count", writes)
	}
}
func TestMarketReadFailurePreservesInstalledAndRejectsProfileChange(t *testing.T) {
	state, server := fakeMarket(t)
	api, err := openAPI(context.Background(), server.URL+"/?token=test")
	if err != nil {
		t.Fatal(err)
	}
	defer api.close()
	state.mu.Lock()
	state.catalogFail = true
	state.mu.Unlock()
	snapshot, err := readMarket(context.Background(), api, true)
	if err == nil || snapshot == nil || snapshot.Installed["alpha-plugin"] == "" {
		t.Fatal("catalog failure lost installed data")
	}
	if strings.Contains(err.Error(), "private-token") {
		t.Fatal("market error leaked response body")
	}
	_, err = runMarketIntent(context.Background(), api, marketIntent{Action: "uninstall", Package: "alpha-plugin", Profile: "old-profile"})
	if err == nil {
		t.Fatal("changed profile accepted")
	}
	state.mu.Lock()
	writes := len(state.writes)
	state.version = 2
	state.mu.Unlock()
	if writes != 0 {
		t.Fatal("write sent after profile changed")
	}
	if _, err = readMarket(context.Background(), api, true); err == nil {
		t.Fatal("unsupported API version accepted")
	}
}
func TestBrowserMarketIsReadOnlyAndFitsPhones(t *testing.T) {
	m := newModel()
	m.page = marketPage
	m.browserPreview = true
	m.market = &marketSnapshot{Profile: "demo", Installed: map[string]string{}, Plugins: []marketPlugin{{Name: "Beta", NPM: "beta-plugin", Description: map[string]string{"ru": "Плагин 世界"}}}}
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 30}, {Width: 44, Height: 18}} {
		m.Update(size)
		m.confirmMarket("install")
		if lipgloss.Height(m.View()) > size.Height || lipgloss.Width(m.View()) > size.Width {
			t.Fatal("market view does not fit terminal")
		}
		if m.executeMarket() != nil || m.marketBusy || m.marketOperating {
			t.Fatal("browser demo tried to write to backend")
		}
		if !strings.Contains(m.marketError, "Демо") {
			t.Fatal("demo limitation missing")
		}
	}
}

func TestMarketMouseTargetsAndProgress(t *testing.T) {
	m := newModel()
	m.page = marketPage
	m.browserPreview = true
	m.market = &marketSnapshot{Profile: "demo", Installed: map[string]string{}, Plugins: []marketPlugin{{Name: "Alpha", NPM: "alpha-plugin"}, {Name: "Beta", NPM: "beta-plugin"}}}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	click := func(kind string, index int) {
		t.Helper()
		for _, hit := range m.marketHits {
			if hit.kind == kind && (kind != "row" || hit.index == index) {
				mouse(m, 27+hit.box.x, 5+hit.box.y, tea.MouseButtonLeft, tea.MouseActionPress)
				return
			}
		}
		t.Fatal("missing mouse target", kind)
	}
	click("row", 1)
	if m.marketIndex != 1 {
		t.Fatal("mouse did not select plugin")
	}
	click("install", 0)
	if m.marketConfirm == nil || m.marketConfirm.Package != "beta-plugin" {
		t.Fatal("mouse did not prepare selected package")
	}
	click("no", 0)
	if m.marketConfirm != nil {
		t.Fatal("cancel mouse target confirmed operation")
	}
	m.marketOperating = true
	m.marketBusy = true
	m.marketSequence = 7
	m.Update(marketProgress{sequence: 7, status: marketStatus{Busy: true, Phase: "packages", Done: 2, Total: 4}})
	if !strings.Contains(clean(m.View()), "2/4") || !m.marketOperating {
		t.Fatal("server progress was not displayed honestly")
	}
	m.Update(marketResult{sequence: 7, snapshot: m.market, operation: true})
	m.Update(marketProgress{sequence: 7, status: marketStatus{Busy: true}})
	if m.marketOperating || m.marketBusy {
		t.Fatal("late progress revived completed operation")
	}
}
