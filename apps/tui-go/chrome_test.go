package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

func key(m *model, kind tea.KeyType) { m.Update(tea.KeyMsg{Type: kind}) }

func selectHubSetting(t *testing.T, m *model, action string) {
	t.Helper()
	m.chrome.settingsNav = false
	for i, row := range m.settingsHubRows() {
		if row.action == action {
			m.chrome.settingsRow = i
			m.settingsHubLayout()
			return
		}
	}
	t.Fatalf("settings action %s is absent", action)
}

func TestSettingsDialogPreservesContext(t *testing.T) {
	for _, text := range []string{"f4", "f1", "alt+t"} {
		m := newModel()
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
		if m.chrome.dialog != "" || m.input.Value() != text {
			t.Fatal("literal text was interpreted as a shortcut")
		}
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 120, Height: 42}, {Width: 100, Height: 30}, {Width: 80, Height: 24}} {
		m := newModel()
		m.Update(size)
		m.input.SetValue("draft 世界")
		m.append(strings.Repeat("scrollable\n", 60))
		m.viewport.GotoTop()
		offset := m.viewport.YOffset
		key(m, tea.KeyF4)
		if m.chrome.dialog != "settings" || m.input.Focused() {
			t.Fatal("shortcut did not open a modal settings dialog")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/goal unsafe\r"), Paste: true})
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}, Alt: true})
		if m.page != chatPage || m.input.Value() != "draft 世界" || m.viewport.YOffset != offset {
			t.Fatal("dialog input leaked into the underlying page")
		}
		if strings.Contains(m.sidebar(20), "Настройки TUI") {
			t.Fatal("client settings remain in the sidebar")
		}
		for _, dialog := range []string{"settings", "help", "about"} {
			m.openDialog(dialog)
			box, _ := m.dialogBox()
			if box.x < 0 || box.y < 2 || box.x+box.width > size.Width || box.y+box.height > size.Height {
				t.Fatalf("dialog %s does not fit %+v", dialog, size)
			}
			view := m.View()
			if lipgloss.Height(view) > size.Height {
				t.Fatal("dialog overflowed terminal height")
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size.Width {
					t.Fatal("dialog overflowed terminal width")
				}
			}
		}
		key(m, tea.KeyEsc)
		if m.chrome.dialog != "" || m.page != chatPage || !m.input.Focused() || m.input.Value() != "draft 世界" || m.viewport.YOffset != offset {
			t.Fatal("closing dialog failed to restore context")
		}
	}
	m := newModel()
	m.page = marketPage
	m.input.Blur()
	m.marketEditing = true
	m.marketSearch.SetValue("plugin")
	m.marketSearch.Focus()
	key(m, tea.KeyF4)
	key(m, tea.KeyEsc)
	if !m.marketSearch.Focused() || !m.marketEditing || m.marketSearch.Value() != "plugin" || m.input.Focused() {
		t.Fatal("market search focus was lost")
	}
}

func TestPreferencesApplyAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "preferences.json")
	m := newModel()
	m.savePreferences = func(p clientPreferences) error { return writeClientPreferences(path, p) }
	enter(m, "remembered")
	m.input.SetValue("unsent")
	key(m, tea.KeyF4)
	selectHubSetting(t, m, "input-history")
	key(m, tea.KeyEnter)
	if m.prefs.InputHistory || len(m.history) != 0 || m.input.Value() != "unsent" {
		t.Fatal("disabling history did not clear recall while retaining the draft")
	}
	key(m, tea.KeyEsc)
	enter(m, "not retained")
	m.input.SetValue("another draft")
	key(m, tea.KeyUp)
	if len(m.history) != 0 || m.input.Value() != "another draft" {
		t.Fatal("disabled history still records or recalls input")
	}
	key(m, tea.KeyF4)
	selectHubSetting(t, m, "mouse")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mouseEnabled() || cmd == nil {
		t.Fatal("mouse reporting was not disabled")
	}
	cmd()
	prefs, err := readClientPreferences(path)
	if err != nil || prefs.Mouse || prefs.InputHistory {
		t.Fatalf("preferences were not saved: %+v %v", prefs, err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("preferences file permissions are not private")
	}
	m.savePreferences = func(clientPreferences) error { return errors.New("private filesystem detail") }
	key(m, tea.KeyEnter)
	if m.mouseEnabled() || !strings.Contains(m.preferencesNote, "Не удалось") || strings.Contains(m.preferencesNote, "private") {
		t.Fatal("failed save changed the applied state or leaked an error")
	}
}

func TestPreferencesDefaultsAndMouseOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	prefs, err := readClientPreferences(path)
	if err != nil || prefs != defaultClientPreferences() {
		t.Fatal("missing preferences did not use defaults")
	}
	os.WriteFile(path, []byte(`{"mouse":false}`), 0600)
	prefs, err = readClientPreferences(path)
	if err != nil || prefs.Mouse || !prefs.InputHistory {
		t.Fatal("partial preferences lost defaults")
	}
	os.WriteFile(path, []byte(`broken`), 0600)
	prefs, err = readClientPreferences(path)
	if err == nil || prefs != defaultClientPreferences() {
		t.Fatal("corrupt preferences were not reported")
	}
	m := newModel()
	m.mouseForcedOff = true
	key(m, tea.KeyF4)
	selectHubSetting(t, m, "mouse")
	key(m, tea.KeyEnter)
	if m.mouseEnabled() || !m.prefs.Mouse || !strings.Contains(m.preferencesNote, "--no-mouse") {
		t.Fatal("CLI mouse override was ignored")
	}
	key(m, tea.KeyEsc)
	mouse(m, 2, 1, tea.MouseButtonLeft, tea.MouseActionPress)
	if m.chrome.menuOpen {
		t.Fatal("disabled mouse still opens menus")
	}
}

func TestMenuAndDialogMouseTargets(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 30}, {Width: 80, Height: 24}} {
		m := newModel()
		m.Update(size)
		m.input.SetValue("draft")
		key(m, tea.KeyF4)
		if m.chrome.dialog != "settings" {
			t.Fatal("mouse did not open settings")
		}
		selectHubSetting(t, m, "input-history")
		_, _, hits := m.settingsHubLayout()
		var target, close area
		for _, hit := range hits {
			if hit.action == "row" && hit.index == m.chrome.settingsRow {
				target = hit.area
			}
			if hit.action == "close" {
				close = hit.area
			}
		}
		if !strings.Contains(clean(strings.Split(m.View(), "\n")[target.y]), "История ввода") {
			t.Fatal("history hit area is misaligned")
		}
		mouse(m, target.x, target.y, tea.MouseButtonLeft, tea.MouseActionPress)
		if m.prefs.InputHistory {
			t.Fatal("history checkbox click failed")
		}
		mouse(m, close.x, close.y, tea.MouseButtonLeft, tea.MouseActionPress)
		if m.chrome.dialog != "" || !m.input.Focused() || m.input.Value() != "draft" {
			t.Fatal("close button failed to restore draft focus")
		}
		paletteTestSelect(t, m, "Настройки dsh")
		if m.page != dshSettingsPage || m.chrome.menuOpen {
			t.Fatal("palette failed to open backend settings")
		}
		paletteTestSelect(t, m, "О программе")
		if m.chrome.dialog != "about" || !strings.Contains(clean(m.View()), version) {
			t.Fatal("palette did not open About")
		}
		key(m, tea.KeyEsc)
		if m.page != dshSettingsPage || m.input.Focused() {
			t.Fatal("About lost page focus")
		}
	}
}

func TestShortcutReferenceSearchAndSettingsEntry(t *testing.T) {
	m := newModel()
	m.input.SetValue("retained draft")
	m.openDialog("settings")
	selectHubSetting(t, m, "shortcuts")
	key(m, tea.KeyEnter)
	if m.chrome.dialog != "help" {
		t.Fatal("settings did not open shortcuts")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("расписания"), Paste: true})
	_, view := m.dialogBox()
	if !strings.Contains(view, "Ctrl+G") || strings.Contains(view, "Ctrl+]") {
		t.Fatal("shortcut search did not filter by context")
	}
	key(m, tea.KeyCtrlU)
	key(m, tea.KeyEnd)
	m.dialogBox()
	if m.chrome.helpOffset == 0 {
		t.Fatal("shortcut reference did not scroll")
	}
	key(m, tea.KeyEsc)
	if m.chrome.dialog != "settings" {
		t.Fatal("shortcuts did not return to settings")
	}
	key(m, tea.KeyEsc)
	if m.input.Value() != "retained draft" || !m.input.Focused() {
		t.Fatal("shortcut reference lost composer context")
	}
}

func TestThemeAndSidebarPreferences(t *testing.T) {
	m := newModel()
	m.width, m.height = 100, 30
	before, _ := m.contentSize()
	m.togglePreference(5)
	after, _ := m.contentSize()
	if m.sidebarVisible() || after != before {
		t.Fatal("legacy sidebar preference changed the new shell geometry")
	}
	m.togglePreference(4)
	if m.prefs.Theme != "light" || theme.Color(theme.Panel) != "#f0eeea" {
		t.Fatal("light theme did not apply")
	}
	m.togglePreference(4)
	if m.prefs.Theme != "" || string(theme.Color(theme.Panel)) != "#242424" {
		t.Fatal("dark theme was not restored")
	}
}

func TestLegacyMenuCannotCaptureInvisibleMouseTargets(t *testing.T) {
	m := newModel()
	m.input.SetValue("draft")
	key(m, tea.KeyF10)
	if m.chrome.menuOpen {
		t.Fatal("retired top menu opened")
	}
	mouse(m, 3, 1, tea.MouseButtonLeft, tea.MouseActionPress)
	if m.chrome.menuOpen || m.chrome.dialog != "" || m.input.Value() != "draft" {
		t.Fatal("invisible menu captured click")
	}
}
