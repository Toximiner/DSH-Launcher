package main

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme")

type clientPreferences struct {
	Mouse        bool   `json:"mouse"`
	InputHistory bool   `json:"inputHistory"`
	Theme        string `json:"theme,omitempty"`
	Language     string `json:"language,omitempty"`
	HideSidebar  bool   `json:"hideSidebar,omitempty"`
}

func defaultClientPreferences() clientPreferences {
	return clientPreferences{Mouse: true, InputHistory: true}
}

type chromeState struct {
	settingsTab, settingsRow, settingsOffset int
	settingsNav                              bool
	helpFromSettings                         bool
	menuOpen                                 bool
	menu, item                               int
	dialog                                   string
	selection                                int
	helpOffset                               int
	helpGroup                                int
	helpQuery                                string
	inputFocused, searchFocused              bool
}

var menuNames = []string{"Файл", "Вид", "Справка"}

type menuEntry struct {
	label, action string
}

var menuEntries = [][]menuEntry{
	{{"Настройки TUI…   F4", "settings"}, {"Установка и dsh…", "runtime"}, {"Выйти        Ctrl+D", "quit"}},
	{{"Настройки dsh", "dsh-settings"}},
	{{"Горячие клавиши", "help"}, {"Обновления и откат…", "updates"}, {"Журнал клиента", "log"}, {"Диагностика…", "diagnostics"}, {"О программе", "about"}},
}

func (m *model) mouseEnabled() bool { return m.prefs.Mouse && !m.mouseForcedOff }

func (m *model) beginChrome() {
	if m.chrome.menuOpen || m.chrome.dialog != "" {
		return
	}
	m.chrome.inputFocused = m.input.Focused()
	m.chrome.searchFocused = m.marketSearch.Focused()
	m.input.Blur()
	m.marketSearch.Blur()
}

func (m *model) openDialog(kind string) tea.Cmd {
	m.beginChrome()
	m.chrome.menuOpen = false
	m.chrome.dialog = kind
	m.chrome.selection = 0
	m.chrome.helpOffset = 0
	m.chrome.helpQuery = ""
	m.chrome.helpGroup = 0
	if kind == "settings" {
		m.chrome.settingsTab, m.chrome.settingsRow, m.chrome.settingsOffset = 0, 0, 0
		m.chrome.settingsNav = true
		if m.settings == nil {
			return m.loadSettings()
		}
	}
	return nil
}

func (m *model) closeChrome() tea.Cmd {
	if m.chrome.dialog == "help" && m.chrome.helpFromSettings {
		m.chrome.dialog = "settings"
		m.chrome.helpFromSettings = false
		return nil
	}
	input, search := m.chrome.inputFocused, m.chrome.searchFocused
	m.chrome.menuOpen = false
	m.chrome.dialog = ""
	m.chrome.helpFromSettings = false
	if search {
		return m.marketSearch.Focus()
	}
	if input {
		return m.input.Focus()
	}
	return nil
}

func (m *model) togglePreference(index int) tea.Cmd {
	next := m.prefs
	if index == 0 {
		if m.mouseForcedOff {
			m.preferencesNote = tr("Мышь отключена флагом --no-mouse.")
			return nil
		}
		next.Mouse = !next.Mouse
	} else if index == 1 {
		next.InputHistory = !next.InputHistory
	} else if index == 4 {
		if next.Theme == "light" {
			next.Theme = ""
		} else {
			next.Theme = "light"
		}
	} else if index == 5 {
		next.HideSidebar = !next.HideSidebar
	} else if index == 6 {
		if next.Language == "en" {
			next.Language = ""
		} else {
			next.Language = "en"
		}
	}
	if m.savePreferences != nil {
		if err := m.savePreferences(next); err != nil {
			m.preferencesNote = tr("Не удалось сохранить. Параметры не изменены.")
			return nil
		}
		m.preferencesNote = tr("Сохранено.")
	} else {
		m.preferencesNote = tr("Применено на время этого сеанса.")
	}
	m.prefs = next
	m.applyClientAppearance()
	if index == 6 {
		if m.savePreferences != nil {
			m.preferencesNote = tr("Сохранено.")
		} else {
			m.preferencesNote = tr("Применено на время этого сеанса.")
		}
	}
	if index == 5 {
		if next.HideSidebar {
			m.navFocused = false
			if m.chrome.dialog != "" {
				m.chrome.inputFocused = m.page == chatPage
			} else if m.page == chatPage {
				m.input.Focus()
			}
		}
		m.resize()
	}
	m.rebuild()
	if !next.InputHistory {
		m.history = nil
		m.historyIndex = 0
		m.draft = ""
	}
	if index == 0 {
		if m.mouseEnabled() {
			return tea.EnableMouseCellMotion
		}
		return tea.DisableMouse
	}
	return nil
}

func (m *model) activateMenu() tea.Cmd {
	action := menuEntries[m.chrome.menu][m.chrome.item].action
	switch action {
	case "settings", "help", "about":
		return m.openDialog(action)
	case "runtime", "updates", "diagnostics":
		m.closeChrome()
		return m.openRuntime(action)
	case "quit":
		return tea.Quit
	}
	m.closeChrome()
	switch action {
	case "dsh-settings":
		m.page = dshSettingsPage
	case "log":
		m.page = logPage
	}
	m.navFocused = false
	m.input.Blur()
	m.marketEditing = false
	m.marketSearch.Blur()
	m.resize()
	m.viewport.GotoTop()
	return nil
}

func (m *model) chromeKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	key := msg.String()
	active := m.chrome.menuOpen || m.chrome.dialog != ""
	if m.chrome.dialog == "help" && (msg.Type == tea.KeyRunes || msg.Type == tea.KeyBackspace || msg.Type == tea.KeyCtrlU) && !msg.Alt {
		if msg.Type == tea.KeyCtrlU {
			m.chrome.helpQuery = ""
		} else if msg.Type == tea.KeyBackspace {
			r := []rune(m.chrome.helpQuery)
			if len(r) > 0 {
				m.chrome.helpQuery = string(r[:len(r)-1])
			}
		} else if len(m.chrome.helpQuery)+len(string(msg.Runes)) <= 128 {
			m.chrome.helpQuery += clean(string(msg.Runes))
		}
		m.chrome.helpOffset = 0
		return nil, true
	}
	if msg.Paste {
		return nil, active
	}
	if msg.Type == tea.KeyF8 {
		return nil, false
	}
	if msg.Type == tea.KeyRunes && !msg.Alt && key != " " {
		return nil, active
	}
	if key == "ctrl+c" || key == "ctrl+d" {
		return tea.Quit, true
	}
	if key == "f4" || key == "alt+t" {
		if m.chrome.dialog == "settings" {
			return m.closeChrome(), true
		}
		return m.openDialog("settings"), true
	}
	if key == "f1" {
		if m.chrome.dialog == "settings" {
			return m.settingsHelp(), true
		}
		return m.openDialog("help"), true
	}
	if key == "f10" || key == "alt+f" || key == "alt+v" || key == "alt+h" {
		if key == "f10" && active {
			return m.closeChrome(), true
		}
		m.beginChrome()
		m.chrome.dialog = ""
		m.chrome.menuOpen = true
		m.chrome.menu = 0
		if key == "alt+v" {
			m.chrome.menu = 1
		}
		if key == "alt+h" {
			m.chrome.menu = 2
		}
		m.chrome.item = 0
		return nil, true
	}
	if !active {
		return nil, false
	}
	if key == "esc" {
		return m.closeChrome(), true
	}
	if m.chrome.dialog != "" {
		if m.chrome.dialog == "help" {
			switch key {
			case "tab", "right":
				m.changeHelpGroup(1)
			case "shift+tab", "left":
				m.changeHelpGroup(-1)
			case "up":
				m.chrome.helpOffset--
			case "down":
				m.chrome.helpOffset++
			case "pgup":
				m.chrome.helpOffset -= m.shortcutPageSize()
			case "pgdown":
				m.chrome.helpOffset += m.shortcutPageSize()
			case "home":
				m.chrome.helpOffset = 0
			case "end":
				m.chrome.helpOffset = 1 << 30
			case "enter":
				return m.closeChrome(), true
			}
			return nil, true
		}
		if entries := sidebarDialogEntries(m.chrome.dialog); len(entries) > 0 {
			switch key {
			case "up", "shift+tab":
				m.chrome.selection = (m.chrome.selection + len(entries) - 1) % len(entries)
			case "down", "tab":
				m.chrome.selection = (m.chrome.selection + 1) % len(entries)
			case "enter", " ":
				return m.activateSidebarDialog(), true
			}
			return nil, true
		}
		if m.chrome.dialog != "settings" {
			if key == "enter" {
				return m.closeChrome(), true
			}
			return nil, true
		}
		return m.settingsHubKey(key), true
	}
	switch key {
	case "left":
		m.chrome.menu = (m.chrome.menu + 2) % 3
		m.chrome.item = 0
	case "right", "tab":
		m.chrome.menu = (m.chrome.menu + 1) % 3
		m.chrome.item = 0
	case "up":
		m.chrome.item = (m.chrome.item + len(menuEntries[m.chrome.menu]) - 1) % len(menuEntries[m.chrome.menu])
	case "down":
		m.chrome.item = (m.chrome.item + 1) % len(menuEntries[m.chrome.menu])
	case "enter":
		return m.activateMenu(), true
	}
	return nil, true
}

func menuStart(index int) int {
	x := 1
	for i := 0; i < index; i++ {
		x += lipgloss.Width(tr(menuNames[i])) + 2
	}
	return x
}

func (m *model) menuBar(width int) string {
	parts := []string{}
	for i, label := range menuNames {
		label = tr(label)
		if m.chrome.menuOpen && m.chrome.menu == i {
			label = lipgloss.NewStyle().Bold(true).Foreground(theme.Color(theme.Bg)).Background(theme.Color(theme.Highlight)).Render(label)
		} else {
			label = heading().Render(tr(label))
		}
		parts = append(parts, label)
	}
	return ansi.Truncate(" "+strings.Join(parts, "  ")+muted().Render("  │ "+filepath.Base(m.workspace)+" / "+tr(pages[m.page])), width, "…")
}

func selectedRow(text string, selected bool, width int) string {
	if selected {
		text = "› " + text
	}
	text = ansi.Truncate(text, width, "…")
	if selected {
		return lipgloss.NewStyle().Width(width).Bold(true).Foreground(theme.Color(theme.Highlight)).Background(theme.Color(theme.Active)).Render(text)
	}
	return text
}

func (m *model) dialogBox() (area, string) {
	if m.chrome.dialog == "settings" {
		return m.settingsHubView()
	}
	if m.chrome.dialog == "help" {
		return m.shortcutHelpView()
	}
	width := min(64, m.width-3)
	inner := width - 4
	lines := []string{}
	switch m.chrome.dialog {
	case "navigation", "more", "conversation":
		title := tr("Инструменты")
		if m.chrome.dialog == "more" {
			title = tr("Ещё")
		}
		if m.chrome.dialog == "conversation" {
			title = tr("Сессия")
		}
		lines = []string{heading().Render(title), ""}
		for i, entry := range sidebarDialogEntries(m.chrome.dialog) {
			lines = append(lines, selectedRow(tr(entry.label), m.chrome.selection == i, inner))
		}
		lines = append(lines, "", muted().Render(tr("↑/↓ выбор · Enter открыть · Esc назад")))
	default:
		lines = []string{heading().Render(tr("О программе")), "", "DSH Launcher TUI · " + version, "Go + Bubble Tea · Linux", tr("Интерфейс: русский · тема из настроек"), fmt.Sprintf(tr("Терминал: %d × %d"), m.width, m.height), "", tr("Рабочая папка:"), clean(m.workspace), "", tr("[ Закрыть · Enter / Esc ]")}
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, inner, "…")
	}
	height := len(lines) + 2
	box := area{(m.width - 1 - width) / 2, max(2, (m.height-height)/2), width, height}
	return box, panel(width, height, strings.Join(lines, "\n"), theme.Highlight)
}

// Composite by terminal cells so ANSI styles and wide characters keep hit areas aligned.
func overlay(base, front string, x, y, width int) string {
	back := strings.Split(base, "\n")
	for i, line := range strings.Split(front, "\n") {
		row := y + i
		if row >= len(back) {
			break
		}
		padded := back[row] + strings.Repeat(" ", max(0, width-lipgloss.Width(back[row])))
		back[row] = ansi.Cut(padded, 0, x) + line + ansi.Cut(padded, x+lipgloss.Width(line), width)
	}
	return strings.Join(back, "\n")
}

func (m *model) chromeView(base string) string {
	if m.chrome.dialog != "" {
		box, view := m.dialogBox()
		return overlay(base, view, box.x, box.y, m.width-1)
	}
	if m.chrome.menuOpen {
		lines := []string{}
		for i, item := range menuEntries[m.chrome.menu] {
			lines = append(lines, selectedRow(tr(item.label), m.chrome.item == i, 24))
		}
		return overlay(base, panel(28, len(lines)+2, strings.Join(lines, "\n"), theme.Highlight), min(menuStart(m.chrome.menu), m.width-29), 2, m.width-1)
	}
	return base
}

func (m *model) chromeMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if m.chrome.dialog != "" {
		if m.chrome.dialog == "settings" {
			return m.settingsHubMouse(msg), true
		}
		if m.chrome.dialog == "help" {
			return m.shortcutHelpMouse(msg), true
		}
		if msg.Button == tea.MouseButtonLeft {
			box, _ := m.dialogBox()
			if box.contains(msg.X, msg.Y) {
				row := msg.Y - box.y - 1
				if entries := sidebarDialogEntries(m.chrome.dialog); len(entries) > 0 {
					if row >= 2 && row < 2+len(entries) {
						m.chrome.selection = row - 2
						return m.activateSidebarDialog(), true
					}
					return nil, true
				}
				if row == box.height-3 {
					return m.closeChrome(), true
				}
			}
		}
		return nil, true
	}
	if msg.Button == tea.MouseButtonLeft && msg.Y == 1 {
		for i, label := range menuNames {
			label = tr(label)
			if (area{menuStart(i), 1, lipgloss.Width(label), 1}).contains(msg.X, msg.Y) {
				if m.chrome.menuOpen && m.chrome.menu == i {
					return m.closeChrome(), true
				}
				m.beginChrome()
				m.chrome.menuOpen = true
				m.chrome.menu = i
				m.chrome.item = 0
				return nil, true
			}
		}
	}
	if m.chrome.menuOpen {
		if msg.Button == tea.MouseButtonLeft {
			x := min(menuStart(m.chrome.menu), m.width-29)
			if (area{x + 1, 3, 26, len(menuEntries[m.chrome.menu])}).contains(msg.X, msg.Y) {
				m.chrome.item = msg.Y - 3
				return m.activateMenu(), true
			}
			return m.closeChrome(), true
		}
		return nil, true
	}
	return nil, false
}
