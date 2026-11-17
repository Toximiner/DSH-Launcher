package main

import (
	"strings"
	"testing"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func openTestPalette(t *testing.T) *model {
	t.Helper()
	m := newModel()
	if _, handled := m.paletteKey(tea.KeyMsg{Type: tea.KeyCtrlP}); !handled || !m.palette.open {
		t.Fatal("palette did not open")
	}
	return m
}

func TestPaletteDisabledDoesNotExecuteNeighbor(t *testing.T) {
	m := openTestPalette(t)
	originalPage := m.page
	if cmd := m.paletteExecute(); cmd != nil {
		t.Fatal("disabled selection returned an action")
	}
	if !m.palette.open || m.page != originalPage || m.palette.note == "" {
		t.Fatal("disabled selection changed navigation or closed palette")
	}
}

func TestPaletteRestoresDraftAndFocus(t *testing.T) {
	m := newModel()
	m.input.SetValue("unsent draft")
	m.paletteKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.input.Focused() {
		t.Fatal("underlying input retained focus")
	}
	m.paletteKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.input.Focused() || m.input.Value() != "unsent draft" {
		t.Fatal("closing palette lost focus or draft")
	}
}

func TestPaletteSuppressesOpenOverModal(t *testing.T) {
	m := newModel()
	m.queueDialog.visible = true
	m.input.SetValue("draft")
	_, handled := m.paletteKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	if !handled || m.palette.open || m.input.Value() != "draft" || !m.input.Focused() {
		t.Fatal("modal focus was stolen")
	}
}

func TestPalettePasteNeverExecutes(t *testing.T) {
	m := openTestPalette(t)
	m.paletteKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/exit\nctrl+d"), Paste: true})
	if !m.palette.open || !strings.Contains(m.palette.search.Value(), "/exit") {
		t.Fatal("paste was interpreted as a command")
	}
}

func TestPaletteRuntimeLocaleSearch(t *testing.T) {
	m := openTestPalette(t)
	previous := activeEnglish.Load()
	defer activeEnglish.Store(previous)
	activeEnglish.Store(true)
	translated := tr("Новая сессия")
	if translated == "Новая сессия" {
		t.Fatal("missing English source translation")
	}
	m.palette.search.SetValue(translated)
	rows := m.paletteFiltered()
	found := false
	for _, r := range rows {
		if r.kind == "item" && paletteCatalog[r.index].label == "Новая сессия" {
			found = true
		}
	}
	if !found {
		t.Fatal("palette did not search translated labels")
	}
	view := ansi.Strip(m.paletteView(strings.Repeat("\n", m.height)))
	if !strings.Contains(view, translated) {
		t.Fatal("palette rendered startup locale")
	}
	activeEnglish.Store(false)
	m.palette.search.SetValue("Новая сессия")
	if !strings.Contains(ansi.Strip(m.paletteView(strings.Repeat("\n", m.height))), "Новая сессия") {
		t.Fatal("palette did not switch back to Russian")
	}
}

func TestPaletteGeometryAndMouse(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {100, 30}, {160, 42}} {
		m := openTestPalette(t)
		m.width, m.height = size[0], size[1]
		box := m.paletteBox()
		if box.x < 0 || box.y < 0 || box.x+box.width > m.width || box.y+box.height > m.height {
			t.Fatal("palette outside terminal")
		}
		base := strings.Repeat(strings.Repeat(" ", m.width-1)+"\n", m.height-1)
		rendered := m.paletteView(base)
		for _, line := range strings.Split(rendered, "\n") {
			if lipgloss.Width(line) > m.width {
				t.Fatalf("row wider than terminal: %d", lipgloss.Width(line))
			}
		}
		rows := m.paletteFiltered()
		first, _ := m.paletteWindow()
		clicked := false
		for i := first; i < len(rows); i++ {
			if rows[i].kind == "item" {
				_, handled := m.paletteMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 5 + i - first, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
				if !handled || m.palette.offset != i || !m.palette.open {
					t.Fatal("disabled mouse row did not match rendered row")
				}
				clicked = true
				break
			}
		}
		if !clicked {
			t.Fatal("missing item")
		}
		_, handled := m.paletteMouse(tea.MouseMsg{X: 0, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
		if !handled || !m.palette.open {
			t.Fatal("outside click escaped overlay")
		}
	}
}

func TestPaletteHomeEndAndShortcutSearch(t *testing.T) {
	m := openTestPalette(t)
	m.paletteKey(tea.KeyMsg{Type: tea.KeyEnd})
	items := paletteItemRows(m.paletteFiltered())
	if m.palette.offset != items[len(items)-1] {
		t.Fatal("end did not select last action")
	}
	m.paletteKey(tea.KeyMsg{Type: tea.KeyHome})
	if m.palette.offset != items[0] {
		t.Fatal("home did not select first action")
	}
	m.palette.search.SetValue("/models")
	if len(paletteItemRows(m.paletteFiltered())) != 1 {
		t.Fatal("shortcut search failed")
	}
}

func TestPaletteRestoresPageSearchFocus(t *testing.T) {
	m := newModel()
	m.input.Blur()
	m.marketSearch.SetValue("keep filter")
	m.marketSearch.Focus()
	m.marketEditing = true
	m.paletteKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.marketSearch.Focused() {
		t.Fatal("market search retained focus behind palette")
	}
	m.paletteKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.marketSearch.Focused() || m.marketSearch.Value() != "keep filter" || m.input.Focused() {
		t.Fatal("market search focus or text changed")
	}
}

func TestPaletteMouseRunsExactVisibleAction(t *testing.T) {
	m := openTestPalette(t)
	// A harmless action with no runtime dependency verifies that geometry and
	// selection refer to the same catalog item rather than a neighboring row.
	m.palette.search.SetValue("/clear")
	m.paletteClampOffset()
	m.lines = []string{"old output"}
	first, _ := m.paletteWindow()
	box := m.paletteBox()
	_, handled := m.paletteMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 5 + m.palette.offset - first, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !handled || m.palette.open || len(m.lines) != 0 {
		t.Fatal("visible action click did not execute exact item")
	}
	if !m.input.Focused() {
		t.Fatal("executed action did not regain input focus")
	}
}

func TestPaletteIntegrationCapturesKeysAboveIncomingApproval(t *testing.T) {
	m := approvalModel(t)
	m.input.SetValue("draft before permission")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.palette == nil || !m.palette.open {
		t.Fatal("Update did not open palette")
	}
	// A permission can arrive asynchronously after the palette has opened.
	// It remains pending behind the palette and must not receive its key events.
	requestApproval(m, "palette-hidden-permission", "s")
	m.approvals.selection = 1
	answers := 0
	m.approvals.answer = func(string, string) error { answers++; return nil }
	m.palette.search.SetValue("unmatched-palette-query")
	m.paletteClampOffset()
	for _, key := range []tea.KeyMsg{{Type: tea.KeyRight}, {Type: tea.KeyTab}, {Type: tea.KeyLeft}, {Type: tea.KeyEnter}, {Type: tea.KeyRunes, Runes: []rune("enter"), Paste: true}} {
		m.Update(key)
		if m.approvals.busy || answers != 0 || m.approvals.selection != 1 || len(m.approvals.requests) != 1 {
			t.Fatal("palette key reached pending permission")
		}
	}
	if m.input.Value() != "draft before permission" {
		t.Fatal("palette input changed composer draft")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.palette.open || !m.input.Focused() || m.input.Value() != "draft before permission" {
		t.Fatal("Update did not restore composer draft and focus")
	}
	if answers != 0 || m.approvals.busy || len(m.approvals.requests) != 1 {
		t.Fatal("closing palette answered pending permission")
	}
}

func TestPaletteIntegrationMouseCannotAnswerHiddenApproval(t *testing.T) {
	m := approvalModel(t)
	m.input.SetValue("unsent mouse draft")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	requestApproval(m, "mouse-hidden-permission", "s")
	answers := 0
	m.approvals.answer = func(string, string) error { answers++; return nil }
	m.approvals.selection = -1
	m.palette.search.SetValue("unmatched-palette-query")
	m.paletteClampOffset()
	box := m.approvalBox()
	// Exercise the exact two approval button targets, including the outside
	// palette region where a click previously passed through the overlay.
	for _, x := range []int{box.x + 3, box.x + 2 + (box.width-4)/2} {
		cmd := m.mouse(tea.MouseMsg{X: x, Y: box.y + box.height - 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if cmd != nil || m.approvals.busy || answers != 0 || m.approvals.selection != -1 {
			t.Fatal("mouse passed through palette to a permission")
		}
	}
	m.mouse(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.palette.open || len(m.approvals.requests) != 1 {
		t.Fatal("outside click dismissed a pending interaction")
	}
	paletteBox := m.paletteBox()
	m.mouse(tea.MouseMsg{X: paletteBox.x + paletteBox.width - 4, Y: paletteBox.y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.palette.open || !m.input.Focused() || m.input.Value() != "unsent mouse draft" || answers != 0 {
		t.Fatal("palette mouse close lost draft or answered permission")
	}
}

func TestPaletteIntegrationCannotOpenOverActiveApproval(t *testing.T) {
	m := approvalModel(t)
	requestApproval(m, "active-permission", "s")
	m.input.SetValue("protected draft")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if cmd != nil || (m.palette != nil && m.palette.open) || !m.approvals.visible || m.approvals.selection != -1 || m.input.Value() != "protected draft" {
		t.Fatal("Ctrl+P interfered with active permission")
	}
}

func TestPaletteBackdropRetainsCellsAndReplacesNestedStyles(t *testing.T) {
	syntaxTestProfile(t)
	defer theme.SetLight(false)
	for _, light := range []bool{false, true} {
		theme.SetLight(light)
		base := "\x1b[31m  bright 世界 👩🏽‍💻\x1b[0m   \n\x1b[48;2;255;255;255msecond\x1b[49m line\n"
		dimmed := dimBackdrop(base)
		if ansi.Strip(dimmed) != ansi.Strip(base) {
			t.Fatal("backdrop moved or removed visible characters")
		}
		originalLines := strings.Split(base, "\n")
		for i, line := range strings.Split(dimmed, "\n") {
			if lipgloss.Width(line) != lipgloss.Width(originalLines[i]) {
				t.Fatal("backdrop changed cell geometry")
			}
		}
		if strings.Contains(dimmed, "\x1b[31m") || strings.Contains(dimmed, "255;255;255") {
			t.Fatal("backdrop replayed a bright nested color")
		}
		fg := lipgloss.NewStyle().Foreground(theme.Color(theme.BackdropText)).Render("X")
		bg := lipgloss.NewStyle().Background(theme.Color(theme.Backdrop)).Render("X")
		for _, sample := range []string{fg, bg} {
			prefix := sample[:strings.IndexByte(sample, 'X')]
			if prefix == "" || !strings.Contains(dimmed, strings.TrimSuffix(strings.TrimPrefix(prefix, "\x1b["), "m")) {
				t.Fatal("backdrop did not apply its dim colors")
			}
		}
		if strings.Contains(dimBackdrop("before\x1b]52;c;c2VjcmV0\aafter"), "\x1b]") {
			t.Fatal("backdrop replayed terminal action")
		}
	}
}

func TestPaletteUsesTealSelectionDarkTextAndSeparateNeutralTabs(t *testing.T) {
	syntaxTestProfile(t)
	defer theme.SetLight(false)
	for _, light := range []bool{false, true} {
		m := openTestPalette(t)
		theme.SetLight(light)
		if theme.Color(theme.Selection) == theme.Color(theme.Active) {
			t.Fatal("list selection changed neutral tab surface")
		}
		base := strings.Repeat("background 世界"+strings.Repeat(" ", 60)+"\n", m.height)
		view := m.paletteView(base)
		box := m.paletteBox()
		first, _ := m.paletteWindow()
		selected := strings.Split(view, "\n")[box.y+5+m.palette.offset-first]
		for _, style := range []lipgloss.Style{
			lipgloss.NewStyle().Foreground(theme.Color(theme.SelectionText)),
			lipgloss.NewStyle().Background(theme.Color(theme.Selection)),
		} {
			sample := style.Render("X")
			prefix := sample[:strings.IndexByte(sample, 'X')]
			if !strings.Contains(selected, strings.TrimSuffix(strings.TrimPrefix(prefix, "\x1b["), "m")) {
				t.Fatal("selected row did not use contrast-safe teal styling")
			}
		}
		groupSample := lipgloss.NewStyle().Foreground(theme.Color(theme.Secondary)).Render("X")
		prefix := groupSample[:strings.IndexByte(groupSample, 'X')]
		if !strings.Contains(view, strings.TrimSuffix(strings.TrimPrefix(prefix, "\x1b["), "m")) {
			t.Fatal("palette section headings are missing teal styling")
		}
		m.paletteClose()
		if m.paletteView(base) != base {
			t.Fatal("closed palette dimmed the application")
		}
	}
}

func TestPaletteContextTogglePreservesDraftAndFocus(t *testing.T) {
	m := openTestPalette(t)
	m.input.SetValue("context draft 世界")
	m.palette.search.SetValue("Показать/скрыть контекст")
	m.palette.offset = 0
	m.paletteMove(1)
	rows := paletteItemRows(m.paletteFiltered())
	if len(rows) != 1 {
		t.Fatal("missing unique context action")
	}
	m.palette.offset = rows[0]
	m.paletteExecute()
	if !m.contextPanelHidden || m.input.Value() != "context draft 世界" || m.palette.open {
		t.Fatal("context action lost draft or stayed modal")
	}
	m.paletteKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	m.palette.search.SetValue("Показать/скрыть контекст")
	m.palette.offset = paletteItemRows(m.paletteFiltered())[0]
	m.paletteExecute()
	if m.contextPanelHidden || m.input.Value() != "context draft 世界" {
		t.Fatal("context action failed to restore panel")
	}
}
