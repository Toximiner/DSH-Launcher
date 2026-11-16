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
	menuOpen                    bool
	menu, item                  int
	dialog                      string
	selection                   int
	helpOffset                  int
	helpQuery                   string
	inputFocused, searchFocused bool
}

var menuNames = []string{"Файл", "Вид", "Справка"}
var menuEntries = [][]string{
	{"Настройки TUI…   F4", "Выйти        Ctrl+D", "Установка и dsh…", "Обновления и откат…"},
	{"Настройки dsh", "Журнал клиента"},
	{"Горячие клавиши", "О программе", "Диагностика…"},
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
	if index == 5 {
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
	menu, item := m.chrome.menu, m.chrome.item
	if menu == 0 {
		if item == 0 {
			return m.openDialog("settings")
		}

		if item >= 2 {
			m.closeChrome()
			return m.openRuntime([]string{"runtime", "updates"}[item-2])
		}
		return tea.Quit
	}
	if menu == 2 {
		if item == 0 {
			return m.openDialog("help")
		}
		if item == 2 {
			m.closeChrome()
			return m.openRuntime("diagnostics")
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
			case "up":
				m.chrome.helpOffset--
			case "down":
				m.chrome.helpOffset++
			case "pgup":
				m.chrome.helpOffset -= max(1, m.height-12)
			case "pgdown":
				m.chrome.helpOffset += max(1, m.height-12)
			case "home":
				m.chrome.helpOffset = 0
			case "end":
				m.chrome.helpOffset = len(shortcutLines(m.chrome.helpQuery))
			case "enter":
				return m.closeChrome(), true
			}
			return nil, true
		}
		if m.chrome.dialog != "settings" {
			if key == "enter" {
				return m.closeChrome(), true
			}
			return nil, true
		}
		switch key {
		case "up", "shift+tab":
			m.chrome.selection = (m.chrome.selection + 6) % 7
		case "down", "tab":
			m.chrome.selection = (m.chrome.selection + 1) % 7
		case "enter", " ":
			if m.chrome.selection == 2 {
				return m.closeChrome(), true
			}
			if m.chrome.selection == 3 {
				return m.openDialog("help"), true
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
			label = lipgloss.NewStyle().Bold(true).Foreground(uiColor("#101318")).Background(uiColor("#79c0ff")).Render(label)
		} else {
			label = heading().Render(tr(label))
		}
		parts = append(parts, label)
	}
	return ansi.Truncate(" "+strings.Join(parts, "  ")+muted().Render("  │ "+filepath.Base(m.workspace)+" / "+tr(pages[m.page])), width, "…")
}

func selectedRow(text string, selected bool, width int) string {
	text = ansi.Truncate(text, width, "…")
	if selected {
		return lipgloss.NewStyle().Width(width).Bold(true).Foreground(uiColor("#79c0ff")).Background(uiColor("#233044")).Render(text)
	}
	return text
}

func (m *model) dialogBox() (area, string) {
	width := min(64, m.width-3)
	inner := width - 4
	lines := []string{}
	switch m.chrome.dialog {
	case "settings":
		lines = append(lines, heading().Render(tr("Настройки TUI")), muted().Render(tr("Применяются сразу · только этот клиент")), "")
		for i, label := range []string{tr("Мышь в интерфейсе"), tr("История ввода")} {
			on := m.mouseEnabled()
			if i == 1 {
				on = m.prefs.InputHistory
			}
			mark := "[ ]"
			if on {
				mark = "[✓]"
			}
			if i == 0 && m.mouseForcedOff {
				label = tr("Мышь (--no-mouse)")
			}
			lines = append(lines, selectedRow(mark+" "+label, m.chrome.selection == i, inner))
		}
		lines = append(lines, "", muted().Render(tr("F4 / Alt+T — открыть · Esc — закрыть")), muted().Render(tr("↑/↓ — выбор · Enter/Пробел — изменить")), "", ansi.Truncate(m.preferencesNote, inner, "…"), "", selectedRow(tr("[ Закрыть ]"), m.chrome.selection == 2, inner), selectedRow(tr("[ Горячие клавиши ]"), m.chrome.selection == 3, inner), selectedRow(tr("Тема: ")+map[bool]string{true: tr("светлая"), false: tr("тёмная")}[m.prefs.Theme == "light"], m.chrome.selection == 4, inner), selectedRow(tr("Боковая панель: ")+map[bool]string{true: tr("скрыта"), false: tr("видна")}[m.prefs.HideSidebar], m.chrome.selection == 5, inner), selectedRow("Language / Язык: "+map[bool]string{true: "English", false: "Русский"}[m.prefs.Language == "en"], m.chrome.selection == 6, inner))
	case "help":
		all := shortcutLines(m.chrome.helpQuery)
		count := max(1, min(24, m.height-11))
		offset := max(0, min(m.chrome.helpOffset, max(0, len(all)-count)))
		m.chrome.helpOffset = offset
		lines = []string{heading().Render(tr("Горячие клавиши")), tr("Поиск: ") + m.chrome.helpQuery, ""}
		lines = append(lines, all[offset:min(len(all), offset+count)]...)
		lines = append(lines, "", fmt.Sprintf(tr("%d–%d / %d · ↑/↓ PgUp/PgDn · Ctrl+U сброс"), offset+1, min(len(all), offset+count), len(all)), tr("[ Закрыть · Enter / Esc ]"))
	default:
		lines = []string{heading().Render(tr("О программе")), "", "DSH Launcher TUI · " + version, "Go + Bubble Tea · Linux", tr("Интерфейс: русский · тема из настроек"), fmt.Sprintf(tr("Терминал: %d × %d"), m.width, m.height), "", tr("Рабочая папка:"), clean(m.workspace), "", tr("[ Закрыть · Enter / Esc ]")}
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
			lines = append(lines, selectedRow(tr(item), m.chrome.item == i, 24))
		}
		return overlay(base, panel(28, len(lines)+2, strings.Join(lines, "\n"), "#79c0ff"), min(menuStart(m.chrome.menu), m.width-29), 2, m.width-1)
	}
	return base
}

func (m *model) chromeMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if m.chrome.dialog != "" {
		if m.chrome.dialog == "help" && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
			if msg.Button == tea.MouseButtonWheelUp {
				m.chrome.helpOffset -= 3
			} else {
				m.chrome.helpOffset += 3
			}
			return nil, true
		}
		if msg.Button == tea.MouseButtonLeft {
			box, _ := m.dialogBox()
			if box.contains(msg.X, msg.Y) {
				row := msg.Y - box.y - 1
				if m.chrome.dialog == "settings" {
					if row == 3 || row == 4 {
						m.chrome.selection = row - 3
						return m.togglePreference(row - 3), true
					}
					if row == 13 || row == 14 || row == 15 {
						m.chrome.selection = row - 9
						return m.togglePreference(m.chrome.selection), true
					}
					if row == 12 {
						return m.openDialog("help"), true
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
