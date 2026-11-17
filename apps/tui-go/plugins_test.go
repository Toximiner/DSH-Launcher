package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func pluginRowsFixture() []benchRow {
	rows := []benchRow{{ID: "builtin:agent-loop", Title: "Agent loop", Body: "Tool dispatch settings", Data: map[string]any{"pluginGroup": "builtin", "namespace": "agent-loop"}}}
	for _, name := range []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon", "Zeta", "Eta", "Theta"} {
		rows = append(rows, benchRow{ID: name, Title: name, Body: strings.Repeat("Long description 世界 ", 10), Data: map[string]any{"pluginGroup": "installed", "enabled": false, "installed": true, "removable": true, "updatable": true, "version": "1.0.0"}})
	}
	rows = append(rows, benchRow{ID: "component:internal", Title: "internal-module", Data: map[string]any{"pluginGroup": "components", "enabled": true}})
	return rows
}
func pluginTestModel(t *testing.T) *model {
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	m.input.SetValue("preserved draft")
	m.openWorkbench("plugins")
	d := m.workbench
	m.handleBench(benchResult{dialog: d, serial: d.serial, rows: pluginRowsFixture(), pluginManagement: true,
		pluginSettings: &settingsSnapshot{Writable: true, Namespaces: []settingsNamespace{{NS: "agent-loop", Revision: 3, Schema: json.RawMessage(`{"uid":0,"refs":{"0":{"type":"object","dict":{"limit":1}},"1":{"type":"number"}}}`), Value: map[string]any{"limit": float64(4)}}}}})
	return m
}
func TestPluginPageGeometryButtonsAndNestedContext(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, language := range []string{"", "en"} {
		for _, hidden := range []bool{false, true} {
			for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 42}} {
				m := pluginTestModel(t)
				m.prefs.Language, m.prefs.HideSidebar = language, hidden
				m.applyClientAppearance()
				m.Update(size)
				if m.page != pluginsPage || len(m.workbench.visible()) != 9 {
					t.Fatal("plugins did not open the package page with hidden components")
				}
				key(m, tea.KeyEnd)
				view := m.View()
				if lipgloss.Width(view) > size.Width-1 || lipgloss.Height(view) > size.Height || !strings.Contains(view, "Theta") {
					t.Fatal("plugin page overflowed or hid the selected final card")
				}
				_, hits := m.pluginsLayout(m.pluginsBox())
				lines := strings.Split(clean(m.View()), "\n")
				clicked := false
				for _, hit := range hits {
					if hit.action == "toggle" && hit.index == m.workbench.index {
						if []rune(lines[hit.y])[hit.x] != '[' {
							t.Fatal("toggle hit target missed the visible switch")
						}
						mouse(m, hit.x, hit.y, tea.MouseButtonLeft, tea.MouseActionPress)
						clicked = true
						break
					}
				}
				if !clicked || !m.workbench.confirm || m.workbench.action.Args["name"] != "Theta" || m.workbench.action.Args["enabled"] != true {
					t.Fatal("switch did not prepare the exact confirmed action")
				}
				key(m, tea.KeyEsc)
				if m.workbench.confirm || m.workbench.rows[8].Data["enabled"] != false {
					t.Fatal("cancel changed the package state")
				}
				key(m, tea.KeyHome)
				key(m, tea.KeySpace)
				if m.workbench.action != nil || m.workbench.confirm {
					t.Fatal("built-in settings card was treated as a bundle switch")
				}
				key(m, tea.KeyEnter)
				if m.settingsForm == nil || m.settingsForm.namespace.NS != "agent-loop" || m.settingsForm.namespace.Revision != 3 {
					t.Fatal("built-in card opened the wrong revision-checked form")
				}
				key(m, tea.KeyEsc)
				root := m.workbench
				key(m, tea.KeyF4)
				m.openWorkbench("presets")
				if m.workbench.parent != root {
					t.Fatal("nested settings discarded the plugin page")
				}
				m.closeWorkbench()
				if m.workbench != root {
					t.Fatal("nested panel did not restore the plugin page")
				}
				key(m, tea.KeyEsc)
				key(m, tea.KeyEsc)
				if m.workbench != nil || m.page != chatPage || !m.input.Focused() || m.input.Value() != "preserved draft" {
					t.Fatal("leaving plugins lost chat context")
				}
			}
		}
	}
}
func TestPluginPageTechnicalViewAndNavigation(t *testing.T) {
	m := pluginTestModel(t)
	key(m, tea.KeyF2)
	if rows := m.workbench.visible(); len(rows) != 1 || rows[0].ID != "component:internal" {
		t.Fatal("technical view mixed in user-facing package cards")
	}
	key(m, tea.KeySpace)
	if m.workbench.action != nil {
		t.Fatal("technical component prepared a write")
	}
	key(m, tea.KeyEnter)
	if m.workbench.document != "plugin-details" {
		t.Fatal("component details did not open")
	}
	key(m, tea.KeyEsc)
	key(m, tea.KeyF2)
	key(m, tea.KeyCtrlP)
	if m.palette == nil || !m.palette.open {
		t.Fatal("plugin page trapped command palette navigation")
	}
	key(m, tea.KeyEsc)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}, Alt: true})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	if m.page != chatPage || m.workbench != nil || m.input.Value() != "preserved draft!" {
		t.Fatal("global navigation trapped the page or lost the first typed key")
	}
}
func TestPluginSnapshotUsesBundleMetadataAndReadOnlyFallback(t *testing.T) {
	for _, management := range []bool{false, true} {
		writes := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/connect" {
				http.SetCookie(w, &http.Cookie{Name: "dsh", Value: "fixture", Path: "/"})
				w.Header().Set("Location", "/")
				w.WriteHeader(302)
				return
			}
			var request struct {
				RPCID string `json:"rpcId"`
			}
			json.NewDecoder(r.Body).Decode(&request)
			var value any
			switch r.URL.Path {
			case "/api/pluginInventory/list":
				value = map[string]any{"managementAvailable": management, "entries": []any{map[string]any{"entryId": "internal", "moduleName": "@test/internal"}}}
			case "/api/settings/describe":
				value = map[string]any{"writable": true, "namespaces": []any{map[string]any{"ns": "agent-loop", "schema": json.RawMessage(`{"uid":0,"refs":{"0":{"type":"object"}}}`)}}}
			case "/api/pluginManager/listBundles":
				if !management {
					t.Error("read-only host requested management data")
				}
				value = []any{
					map[string]any{"name": "@deepseek-ai/dsh-base", "official": true, "enabled": true},
					map[string]any{"name": "@test/package", "installed": true, "enabled": false, "meta": map[string]any{"title": map[string]any{"en": "Friendly plugin", "ru": "Понятный плагин"}, "description": "Actual package description"}},
					map[string]any{"name": pinnedPluginBundles[0], "official": true, "enabled": false},
					map[string]any{"name": "@deepseek-ai/dsh-subagent-codex", "official": true, "enabled": false},
				}
			default:
				writes++
				t.Errorf("unexpected endpoint: %s", r.URL.Path)
			}
			json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": request.RPCID, "result": map[string]any{"ok": true, "value": value}})
		}))
		api, err := openAPI(context.Background(), server.URL+"/?token=fixture")
		if err != nil {
			t.Fatal(err)
		}
		d := &workbench{kind: "plugins"}
		d.rows, _, _, err = readPlugins(context.Background(), api, d)
		if management {
			if err := pluginTogglePreflight(context.Background(), api, "@test/package", false); err != nil {
				t.Fatal("unchanged bundle refused its explicit target", err)
			}
			if err := pluginTogglePreflight(context.Background(), api, "@test/package", true); err == nil {
				t.Fatal("concurrent toggle did not require a new snapshot")
			}
			if err := pluginTogglePreflight(context.Background(), api, "@test/missing", false); err == nil {
				t.Fatal("removed package passed the preflight")
			}
		}
		api.close()
		server.Close()
		if err != nil || writes != 0 || d.pluginSettings == nil {
			t.Fatalf("invalid read-only snapshot: %v", err)
		}
		if management {
			if len(d.visible()) != 3 || d.visible()[2].Title != "Понятный плагин" || d.visible()[2].Body != "Actual package description" {
				t.Fatal("snapshot exposed internal modules, ignored metadata or lost built-in settings")
			}
			d.pluginMore = true
			if len(d.visible()) != 4 {
				t.Fatal("More did not expose secondary experimental bundles")
			}
		}
		d.pluginTechnical = true
		if len(d.visible()) != 1 || d.visible()[0].ID != "component:internal" {
			t.Fatal("component fallback lost the Loader inventory")
		}
	}
}
