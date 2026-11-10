package main

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type clientPreferences struct {
	Mouse        bool `json:"mouse"`
	InputHistory bool `json:"inputHistory"`
}

func defaultClientPreferences() clientPreferences {
	return clientPreferences{Mouse: true, InputHistory: true}
}

type chromeState struct {
	menuOpen                    bool
	menu, item                  int
	dialog                      string
	selection                   int
	inputFocused, searchFocused bool
}

var menuNames = []string{"Файл", "Вид", "Справка"}
var menuEntries = [][]string{
	{"Настройки TUI…   F4", "Выйти        Ctrl+D"},
	{"Настройки dsh", "Журнал клиента"},
	{"Горячие клавиши", "О программе"},
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
	return nil
}

func (m *model) closeChrome() tea.Cmd {
	input, search := m.chrome.inputFocused, m.chrome.searchFocused
	m.chrome.menuOpen = false
	m.chrome.dialog = ""
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
			m.preferencesNote = "Мышь отключена флагом --no-mouse."
			return nil
		}
		next.Mouse = !next.Mouse
	} else {
		next.InputHistory = !next.InputHistory
	}
	if m.savePreferences != nil {
		if err := m.savePreferences(next); err != nil {
			m.preferencesNote = "Не удалось сохранить. Параметры не изменены."
			return nil
		}
		m.preferencesNote = "Сохранено."
	} else {
		m.preferencesNote = "Применено на время этого сеанса."
	}
	m.prefs = next
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
	menu, item := m.chrome.menu, m.chrome.item
	if menu == 0 {
		if item == 0 {
			return m.openDialog("settings")
		}
		return tea.Quit
	}
	if menu == 2 {
		if item == 0 {
			return m.openDialog("help")
		}
		return m.openDialog("about")
	}
	m.closeChrome()
	m.page = []int{dshSettingsPage, logPage}[item]
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
	if msg.Paste {
		return nil, active
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
		if m.chrome.dialog != "settings" {
			if key == "enter" {
				return m.closeChrome(), true
			}
			return nil, true
		}
		switch key {
		case "up", "shift+tab":
			m.chrome.selection = (m.chrome.selection + 2) % 3
		case "down", "tab":
			m.chrome.selection = (m.chrome.selection + 1) % 3
		case "enter", " ":
			if m.chrome.selection == 2 {
				return m.closeChrome(), true
			}
			return m.togglePreference(m.chrome.selection), true
		}
		return nil, true
	}
	switch key {
	case "left":
		m.chrome.menu = (m.chrome.menu + 2) % 3
		m.chrome.item = 0
	case "right", "tab":
		m.chrome.menu = (m.chrome.menu + 1) % 3
		m.chrome.item = 0
	case "up", "down":
		m.chrome.item = 1 - m.chrome.item
	case "enter":
		return m.activateMenu(), true
	}
	return nil, true
}

func menuStart(index int) int {
	x := 1
	for i := 0; i < index; i++ {
		x += lipgloss.Width(menuNames[i]) + 2
	}
	return x
}

func (m *model) menuBar(width int) string {
	parts := []string{}
	for i, label := range menuNames {
		if m.chrome.menuOpen && m.chrome.menu == i {
			label = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#101318")).Background(lipgloss.Color("#79c0ff")).Render(label)
		} else {
			label = heading.Render(label)
		}
		parts = append(parts, label)
	}
	return ansi.Truncate(" "+strings.Join(parts, "  ")+muted.Render("  │ "+filepath.Base(m.workspace)+" / "+pages[m.page]), width, "…")
}

func selectedRow(text string, selected bool, width int) string {
	text = ansi.Truncate(text, width, "…")
	if selected {
		return lipgloss.NewStyle().Width(width).Bold(true).Foreground(lipgloss.Color("#79c0ff")).Background(lipgloss.Color("#233044")).Render(text)
	}
	return text
}

func (m *model) dialogBox() (area, string) {
	width := min(64, m.width-3)
	inner := width - 4
	lines := []string{}
	switch m.chrome.dialog {
	case "settings":
		lines = append(lines, heading.Render("Настройки TUI"), muted.Render("Применяются сразу · только этот клиент"), "")
		for i, label := range []string{"Мышь в интерфейсе", "История ввода"} {
			on := m.mouseEnabled()
			if i == 1 {
				on = m.prefs.InputHistory
			}
			mark := "[ ]"
			if on {
				mark = "[✓]"
			}
			if i == 0 && m.mouseForcedOff {
				label = "Мышь (--no-mouse)"
			}
			lines = append(lines, selectedRow(mark+" "+label, m.chrome.selection == i, inner))
		}
		lines = append(lines, "", muted.Render("F4 / Alt+T — открыть · Esc — закрыть"), muted.Render("↑/↓ — выбор · Enter/Пробел — изменить"), "", ansi.Truncate(m.preferencesNote, inner, "…"), "", selectedRow("[ Закрыть ]", m.chrome.selection == 2, inner))
	case "help":
		lines = []string{heading.Render("Горячие клавиши"), "", "F10 / Alt+F/V/H — верхнее меню", "F4 / Alt+T — настройки TUI", "Tab — разделы · Esc — чат", "Alt+C/S/D/L/M — разделы", "F2 / Alt+1/2/3 — режим агента", "↑/↓ — история · PgUp/PgDn — прокрутка", "Ctrl+D / Ctrl+C — выход", "", "Чат пока локальный. /help — команды.", "", "[ Закрыть · Enter / Esc ]"}
	default:
		lines = []string{heading.Render("О программе"), "", "DSH Launcher TUI · " + version, "Go + Bubble Tea · Linux", "Интерфейс: русский · тёмная тема", fmt.Sprintf("Терминал: %d × %d", m.width, m.height), "", "Рабочая папка:", clean(m.workspace), "", "[ Закрыть · Enter / Esc ]"}
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, inner, "…")
	}
	height := len(lines) + 2
	box := area{(m.width - 1 - width) / 2, max(2, (m.height-height)/2), width, height}
	return box, panel(width, height, strings.Join(lines, "\n"), "#79c0ff")
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
			lines = append(lines, selectedRow(item, m.chrome.item == i, 24))
		}
		return overlay(base, panel(28, 4, strings.Join(lines, "\n"), "#79c0ff"), min(menuStart(m.chrome.menu), m.width-29), 2, m.width-1)
	}
	return base
}

func (m *model) chromeMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if m.chrome.dialog != "" {
		if msg.Button == tea.MouseButtonLeft {
			box, _ := m.dialogBox()
			if box.contains(msg.X, msg.Y) {
				row := msg.Y - box.y - 1
				if m.chrome.dialog == "settings" {
					if row == 3 || row == 4 {
						m.chrome.selection = row - 3
						return m.togglePreference(row - 3), true
					}
					if row == 11 {
						return m.closeChrome(), true
					}
				} else if row == box.height-3 {
					return m.closeChrome(), true
				}
			}
		}
		return nil, true
	}
	if msg.Button == tea.MouseButtonLeft && msg.Y == 1 {
		for i, label := range menuNames {
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
			if (area{x + 1, 3, 26, 2}).contains(msg.X, msg.Y) {
				m.chrome.item = msg.Y - 3
				return m.activateMenu(), true
			}
			return m.closeChrome(), true
		}
		return nil, true
	}
	return nil, false
}
