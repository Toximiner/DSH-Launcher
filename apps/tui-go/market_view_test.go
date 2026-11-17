package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func marketViewFixture(english bool, size tea.WindowSizeMsg) *model {
	m := newModel()
	activeEnglish.Store(english)
	m.page = marketPage
	if english {
		m.prefs.Language = "en"
	}
	m.market = &marketSnapshot{Profile: "fixture", Installed: map[string]string{"package-09": "2.7.0"}, Present: []string{"package-09"}}
	for i := range 30 {
		m.market.Plugins = append(m.market.Plugins, marketPlugin{Name: fmt.Sprintf("Package %02d 世界", i), NPM: fmt.Sprintf("package-%02d", i), Version: "3.2.0", URL: fmt.Sprintf("https://example.invalid/package-%02d", i), Description: map[string]string{"ru": fmt.Sprintf("Описание %02d ", i) + strings.Repeat("подробности ", 20), "en": fmt.Sprintf("Description %02d ", i) + strings.Repeat("details ", 20)}})
	}
	m.Update(size)
	return m
}

func TestMarketFlatRowsKeepMeasuredActionsAndSelectedDetails(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, english := range []bool{false, true} {
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 160, Height: 40}} {
			m := marketViewFixture(english, size)
			for _, selected := range []int{0, 9, 29} {
				m.marketIndex = selected
				m.rebuild()
				view := m.View()
				physical := strings.Split(clean(view), "\n")
				box := m.pluginsPageBox()
				if lipgloss.Width(view) > size.Width-1 || lipgloss.Height(view) > size.Height {
					t.Fatalf("market overflow at %+v", size)
				}
				visible, foundSelected := 0, false
				for _, hit := range m.marketHits {
					if hit.box.y < 0 || hit.box.y >= m.viewport.Height || hit.box.x < 0 || hit.box.x+hit.box.width > m.viewport.Width {
						t.Fatalf("market hit exceeds actual frame: %+v", hit)
					}
					if hit.kind != "row" {
						continue
					}
					visible++
					if hit.box.height != 1 {
						t.Fatal("package still consumes a multiline card hit area")
					}
					row := physical[box.y+hit.box.y]
					if !strings.Contains(row, fmt.Sprintf("Package %02d", hit.index)) {
						t.Fatal("package row hit differs from render")
					}
					if hit.index == selected {
						foundSelected = true
						version := "3.2.0"
						if selected == 9 {
							version = "2.7.0"
						}
						if !strings.Contains(row, version) {
							t.Fatal("actual row version is not visible on the right")
						}
					}
				}
				if !foundSelected || visible < 5 {
					t.Fatal("compact list lost selection or still uses sparse cards")
				}
				prefix := "Описание"
				if english {
					prefix = "Description"
				}
				selectedDescription := fmt.Sprintf("%s %02d", prefix, selected)
				if !strings.Contains(clean(view), selectedDescription) || !strings.Contains(clean(view), fmt.Sprintf("https://example.invalid/package-%02d", selected)) {
					t.Fatalf("selected description/source lost English=%v size=%+v index=%d:\n%s", english, size, selected, clean(view))
				}
				for _, other := range []int{0, 9, 29} {
					if other != selected && strings.Contains(clean(view), fmt.Sprintf("%s %02d", prefix, other)) {
						t.Fatal("unselected descriptions clutter compact package list")
					}
				}
				m.confirmMarket("install")
				physical = strings.Split(clean(m.View()), "\n")
				no, yes := -1, -1
				for _, hit := range m.marketHits {
					if hit.kind == "no" {
						no = hit.box.x
					}
					if hit.kind == "yes" {
						yes = hit.box.x
					}
					if hit.kind == "no" || hit.kind == "yes" {
						label := tr("[n Отмена]")
						if hit.kind == "yes" {
							label = tr("[y Подтвердить]")
						}
						if !strings.Contains(physical[box.y+hit.box.y], label) || hit.box.height != 1 {
							t.Fatal("confirmation hit differs from rendered control")
						}
					}
				}
				if no < 0 || yes <= no {
					t.Fatal("confirmation must keep cancellation before explicit approval")
				}
				m.marketKey(tea.KeyMsg{Type: tea.KeyEsc})
				if m.marketConfirm != nil || m.marketIndex != selected {
					t.Fatal("cancellation changed selected package")
				}
			}
		}
	}
}

func TestMarketFlatStatusReadonlyAndMissingArtifactsStayVisible(t *testing.T) {
	defer activeEnglish.Store(false)
	for _, english := range []bool{false, true} {
		m := marketViewFixture(english, tea.WindowSizeMsg{Width: 80, Height: 24})
		m.marketIndex = 9
		m.market.Present = []string{}
		m.rebuild()
		if !strings.Contains(clean(m.View()), tr("файлы не найдены: ")+"2.7.0") {
			t.Fatalf("profile-only package was displayed as physically installed:\n%s", clean(m.View()))
		}
		m.market.Status = marketStatus{Active: true, Phase: "packages", Done: 3, Total: 8}
		m.rebuild()
		if !strings.Contains(clean(m.View()), "3/8") {
			t.Fatal("server progress disappeared")
		}
		m.marketError = "fixture operation failed"
		m.rebuild()
		if !strings.Contains(clean(m.View()), "! fixture operation failed") {
			t.Fatal("failure was hidden behind progress")
		}
		m.marketError = ""
		m.market.Status = marketStatus{}
		m.market.Plugins = []marketPlugin{{Name: "Git source", URL: "https://example.invalid/read-only", Description: map[string]string{"en": "Read-only fixture"}}}
		m.marketIndex = 0
		m.rebuild()
		if !strings.Contains(clean(m.View()), tr("Только чтение")) || !strings.Contains(clean(m.View()), tr("Git-only источник пока доступен только для просмотра.")) {
			t.Fatal("read-only source was presented as installable")
		}
		m.confirmMarket("install")
		if m.marketConfirm != nil {
			t.Fatal("render change bypassed read-only install guard")
		}
	}
}
