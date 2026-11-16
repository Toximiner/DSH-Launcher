package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type shortcutGroup struct {
	title    string
	contexts []string
}

var shortcutGroups = []shortcutGroup{
	{"Основное и навигация", []string{"Основной экран и настройки TUI", "Основной экран", "Весь интерфейс", "Окна и разделы", "Боковая панель", "Каталоги и панели"}},
	{"Чат и ввод", []string{"Чат", "Поле сообщения; включается в настройках", "Чат и списки", "Команда чата", "Команды чата", "Чат и настройки dsh", "Чат и история", "Редактор /compose", "Очередь сообщений"}},
	{"Сессии и история", []string{"Сессии", "История сессии", "Поиск в истории"}},
	{"Настройки и редакторы", []string{"Настройки TUI", "Форма настроек dsh", "Выбор модели", "Цель и редакторы", "Конфликт редактора цели"}},
	{"Вопросы и подтверждения", []string{"Вопросы агента", "Разрешения инструментов", "Подтверждение действия"}},
	{"Файлы, плагины и аккаунт", []string{"Файлы", "Плагины", "Пакеты", "Установка пакетов", "Аккаунт DeepSeek", "Декларативная панель плагина"}},
	{"Фоновая работа", []string{"Расписания, терминалы, пресеты, рабочие области", "Расписания", "Продолжаемые подагенты", "Задания и терминалы", "Интерактивный терминал"}},
}

type shortcutHelpLine struct {
	text    string
	group   int
	heading bool
}
type shortcutHelpHit struct {
	area
	action string
}

func shortcutSection(s shortcut) int {
	for i, g := range shortcutGroups {
		for _, context := range g.contexts {
			if s.context == context {
				return i + 1
			}
		}
	}
	return 0
}
func shortcutHeading(group, width int) string {
	return accent().Bold(true).Render(ansi.Truncate("── "+tr(shortcutGroups[group-1].title)+" ──", width, "…"))
}
func shortcutContent(query string, group, width int) ([]shortcutHelpLine, int) {
	query = strings.ToLower(strings.TrimSpace(query))
	lines := []shortcutHelpLine{}
	matches := 0
	keyWidth := min(31, max(22, width*2/5))
	descriptionWidth := width - keyWidth - 2
	for i, g := range shortcutGroups {
		if group != 0 && group != i+1 {
			continue
		}
		items := []shortcut{}
		for _, s := range tuiShortcuts {
			if shortcutSection(s) != i+1 {
				continue
			}
			haystack := strings.ToLower(s.keys + " " + tr(s.action) + " " + tr(s.context) + " " + tr(g.title))
			if query == "" || strings.Contains(haystack, query) {
				items = append(items, s)
			}
		}
		if len(items) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, shortcutHelpLine{group: i + 1})
		}
		lines = append(lines, shortcutHelpLine{shortcutHeading(i+1, width), i + 1, true})
		for _, s := range items {
			keys := strings.Split(ansi.Wrap(s.keys, keyWidth, ""), "\n")
			actions := strings.Split(ansi.Wrap(tr(s.action), descriptionWidth, ""), "\n")
			for row := 0; row < max(len(keys), len(actions)); row++ {
				key, action := "", ""
				if row < len(keys) {
					key = accent().Bold(true).Render(keys[row])
				}
				if row < len(actions) {
					action = actions[row]
				}
				text := lipgloss.NewStyle().Width(keyWidth).Render(key) + "  " + action
				lines = append(lines, shortcutHelpLine{text, i + 1, false})
			}
			// Context is subordinate to the action, never a second undifferentiated entry.
			context := strings.Split(ansi.Wrap("· "+tr(s.context), descriptionWidth, ""), "\n")
			for _, text := range context {
				lines = append(lines, shortcutHelpLine{strings.Repeat(" ", keyWidth+2) + muted().Render(text), i + 1, false})
			}
			matches++
		}
	}
	if len(lines) == 0 {
		lines = append(lines, shortcutHelpLine{text: muted().Render(tr("Совпадений нет."))})
	}
	return lines, matches
}
func (m *model) shortcutBox() area {
	width, height := min(100, m.width-3), min(34, m.height-4)
	return area{(m.width - 1 - width) / 2, max(2, (m.height-height)/2), width, height}
}
func (m *model) shortcutPageSize() int { return max(1, m.shortcutBox().height-10) }
func (m *model) changeHelpGroup(delta int) {
	m.chrome.helpGroup = (m.chrome.helpGroup + len(shortcutGroups) + 1 + delta) % (len(shortcutGroups) + 1)
	m.chrome.helpOffset = 0
}
func (m *model) shortcutHelpLayout() (area, []string, []shortcutHelpHit) {
	box := m.shortcutBox()
	width, count := box.width-4, m.shortcutPageSize()
	c := &m.chrome
	group := tr("Все разделы")
	if c.helpGroup > 0 && c.helpGroup <= len(shortcutGroups) {
		group = tr(shortcutGroups[c.helpGroup-1].title)
	}
	close := "[×]"
	title := bar(heading().Render(tr("Горячие клавиши")), accent().Render(close), width)
	prev, next := "[‹]", "[›]"
	category := tr("Раздел: ") + accent().Render(prev) + " " + group + " " + accent().Render(next)
	keyWidth := min(31, max(22, width*2/5))
	columns := lipgloss.NewStyle().Width(keyWidth).Render(tr("Клавиши")) + "  " + tr("Действие")
	lines := []string{title, category, tr("Поиск: ") + c.helpQuery, muted().Render(columns), muted().Render(strings.Repeat("─", width))}
	x, y := box.x+2, box.y+1
	hits := []shortcutHelpHit{
		{area{x + width - lipgloss.Width(close), y, lipgloss.Width(close), 1}, "close"},
		{area{x + lipgloss.Width(tr("Раздел: ")), y + 1, 3, 1}, "previous"},
		{area{x + lipgloss.Width(tr("Раздел: ")) + 3 + 1 + lipgloss.Width(group) + 1, y + 1, 3, 1}, "next"},
	}
	all, matches := shortcutContent(c.helpQuery, c.helpGroup, width)
	maxOffset := 0
	if len(all) > count {
		maxOffset = len(all) - count + 1
	}
	offset := max(0, min(c.helpOffset, maxOffset))
	if all[offset].text == "" && offset < len(all)-1 {
		offset++
	}
	c.helpOffset = offset
	// Retain the section title when scrolling into the middle of a group.
	if !all[offset].heading && all[offset].group > 0 {
		lines = append(lines, shortcutHeading(all[offset].group, width))
		count--
	}
	end := min(len(all), offset+count)
	for _, line := range all[offset:end] {
		lines = append(lines, line.text)
	}
	for len(lines) < 5+m.shortcutPageSize() {
		lines = append(lines, "")
	}
	lines = append(lines, muted().Render(fmt.Sprintf(tr("Найдено: %d · строки %d–%d/%d"), matches, offset+1, end, len(all))),
		muted().Render(tr("Tab / ← → раздел · ↑/↓ PgUp/PgDn прокрутка · Ctrl+U сброс")), tr("[ Закрыть · Enter / Esc ]"))
	hits = append(hits, shortcutHelpHit{area{x, y + len(lines) - 1, lipgloss.Width(tr("[ Закрыть · Enter / Esc ]")), 1}, "close"})
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "…")
	}
	return box, lines, hits
}
func (m *model) shortcutHelpView() (area, string) {
	box, lines, _ := m.shortcutHelpLayout()
	return box, panel(box.width, box.height, strings.Join(lines, "\n"), "#79c0ff")
}
func (m *model) shortcutHelpMouse(msg tea.MouseMsg) tea.Cmd {
	box, _, hits := m.shortcutHelpLayout()
	if !box.contains(msg.X, msg.Y) {
		return nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.chrome.helpOffset = max(0, m.chrome.helpOffset-3)
	case tea.MouseButtonWheelDown:
		m.chrome.helpOffset += 3
	case tea.MouseButtonLeft:
		for _, hit := range hits {
			if hit.contains(msg.X, msg.Y) {
				switch hit.action {
				case "close":
					return m.closeChrome()
				case "previous":
					m.changeHelpGroup(-1)
				case "next":
					m.changeHelpGroup(1)
				}
			}
		}
	}
	return nil
}
