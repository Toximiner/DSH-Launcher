package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

var settingsTabs = []string{"Общие", "Модели", "Встроенные плагины", "Пресеты агента", "Маркет плагинов"}

type settingsHubRow struct{ label, value, description, action string }
type settingsHubHit struct {
	area
	action string
	index  int
}

func (m *model) settingsHubRows() []settingsHubRow {
	open := tr("Открыть")
	switch m.chrome.settingsTab {
	case 1:
		return []settingsHubRow{
			{tr("Модель сессии"), m.modelButton(), tr("Модель и уровень рассуждения текущего диалога"), "models"},
			{tr("Провайдеры"), open, tr("Подключения, модели и API-ключи dsh"), "providers"},
		}
	case 2:
		return []settingsHubRow{
			{tr("Встроенные плагины"), open, tr("Состав и состояние плагинов работающего dsh"), "plugins"},
			{tr("Параметры плагинов"), open, tr("Настройки dsh по схемам установленных плагинов"), "config"},
			{tr("Команды"), open, tr("Команды, доступные агенту в текущей сессии"), "commands"},
		}
	case 3:
		return []settingsHubRow{{tr("Пресеты агента"), m.agentPresetLabel(), tr("Просмотр пресетов и создание сессии с выбранным агентом"), "presets"}}
	case 4:
		return []settingsHubRow{
			{tr("Маркет плагинов"), open, tr("Каталог, установка и обновление плагинов"), "market"},
			{tr("Установленные пакеты"), open, tr("Версии, разрешения установки и удаление пакетов"), "packages"},
		}
	}
	permission := tr("По умолчанию dsh")
	if m.settings != nil {
		for _, ns := range m.settings.Namespaces {
			if ns.NS == "permission" {
				if value, _ := valueAt(ns.Value, []string{"defaultPreset"}); value != nil && value != "" {
					if name, ok := value.(string); ok {
						permission = permissionLabel(name)
					}
				}
			}
		}
	}
	mark := func(on bool) string {
		if on {
			return tr("Включено")
		}
		return tr("Выключено")
	}
	language, theme := "Русский", tr("тёмная")
	if m.prefs.Language == "en" {
		language = "English"
	}
	if m.prefs.Theme == "light" {
		theme = tr("светлая")
	}
	mouse := mark(m.mouseEnabled())
	if m.mouseForcedOff {
		mouse = "--no-mouse"
	}
	return []settingsHubRow{
		{tr("Доступ по умолчанию"), permission, tr("Общий параметр dsh для новых сессий; F5 сохраняет форму"), "permission-default"},
		{tr("Язык"), language, tr("Язык интерфейса TUI"), "language"},
		{tr("Оформление"), theme, tr("Тема интерфейса TUI"), "theme"},
		{tr("Размер шрифта"), tr("Терминал"), tr("Шрифт и его размер задаются в настройках терминала"), "font"},
		{tr("Горячие клавиши"), open, tr("Поиск сочетаний клавиш и действий интерфейса"), "shortcuts"},
		{tr("Мышь в интерфейсе"), mouse, tr("Выбор, кнопки и прокрутка мышью"), "mouse"},
		{tr("История ввода"), mark(m.prefs.InputHistory), tr("Повтор предыдущих сообщений стрелками ↑/↓"), "input-history"},
	}
}

func (m *model) settingsHelp() tea.Cmd {
	cmd := m.openDialog("help")
	m.chrome.helpFromSettings = true
	return cmd
}

func (m *model) settingsHubAction(action string) tea.Cmd {
	switch action {
	case "close":
		return m.closeChrome()
	case "shortcuts":
		return m.settingsHelp()
	case "mouse":
		return m.togglePreference(0)
	case "input-history":
		return m.togglePreference(1)
	case "theme":
		return m.togglePreference(4)
	case "sidebar":
		return m.togglePreference(5)
	case "language":
		return m.togglePreference(6)
	case "font":
		m.preferencesNote = tr("Шрифт и его размер задаются в настройках терминала")
		return nil
	case "config":
		m.closeChrome()
		m.page, m.navFocused = dshSettingsPage, false
		m.input.Blur()
		m.resize()
		m.viewport.GotoTop()
		return m.loadSettings()
	case "market":
		m.closeChrome()
		m.page, m.navFocused = marketPage, false
		m.input.Blur()
		m.resize()
		return m.loadMarket()
	}
	if m.settingsLogin == "" {
		m.preferencesNote = tr("Для этого раздела подключитесь к dsh.")
		return nil
	}
	switch action {
	case "permission-default", "providers":
		if m.settingsLoading || m.settings == nil {
			m.preferencesNote = tr("Дождитесь снимка настроек; F5 — обновить.")
			return m.loadSettings()
		}
		if !m.settings.Writable {
			m.preferencesNote = tr("Настройки dsh доступны только для чтения.")
			return nil
		}
		if action == "providers" {
			return m.openSettingsForm(true)
		}
		for i, ns := range m.settings.Namespaces {
			if ns.NS != "permission" {
				continue
			}
			m.settingsIndex = i
			m.openSettingsForm(false)
			if d := m.settingsForm; d != nil {
				for index, row := range d.rows() {
					if len(row.path) == 1 && row.path[0] == "defaultPreset" {
						d.index = index
						return d.edit(row, false)
					}
				}
			}
			return nil
		}
		m.preferencesNote = tr("В профиле dsh нет настройки доступа по умолчанию.")
	case "models", "plugins", "presets", "packages", "commands":
		if m.chat == nil || !m.chat.ready || m.session == nil || m.chatSessionChanging() {
			m.preferencesNote = tr("Для этого раздела подключитесь к сессии dsh.")
			return nil
		}
		if action == "models" {
			return m.openModels()
		}
		return m.openWorkbench(action)
	}
	return nil
}

func (m *model) settingsHubKey(key string) tea.Cmd {
	c := &m.chrome
	rows := m.settingsHubRows()
	switch key {
	case "tab", "shift+tab":
		c.settingsNav = !c.settingsNav
	case "left":
		c.settingsNav = true
	case "right":
		c.settingsNav = false
	case "up", "down", "home", "end", "pgup", "pgdown":
		index, count := &c.settingsRow, len(rows)
		if c.settingsNav {
			index, count = &c.settingsTab, len(settingsTabs)
		}
		switch key {
		case "up":
			*index = (*index + count - 1) % count
		case "down":
			*index = (*index + 1) % count
		case "home", "pgup":
			*index = 0
		case "end", "pgdown":
			*index = count - 1
		}
		if c.settingsNav {
			c.settingsRow, c.settingsOffset = 0, 0
		}
	case "enter", " ":
		if c.settingsNav {
			c.settingsNav = false
			return nil
		}
		return m.settingsHubAction(rows[min(c.settingsRow, len(rows)-1)].action)
	case "f5":
		return m.loadSettings()
	}
	return nil
}

// Rendering and mouse input share the same measured cells and scroll window.
func (m *model) settingsHubLayout() (area, []string, []settingsHubHit) {
	width, height := min(112, m.width-3), min(34, m.height-4)
	box := area{(m.width - 1 - width) / 2, max(2, (m.height-height)/2), width, height}
	inner, nav := width-4, 21
	right := inner - nav - 3
	x, y := box.x+2, box.y+1
	close, config := "[×]", "["+tr("Конфигурация dsh")+"]"
	top := bar(heading().Render(tr("Настройки TUI")), accent().Render(config+" "+close), inner)
	hits := []settingsHubHit{
		{area{x + inner - lipgloss.Width(close), y, lipgloss.Width(close), 1}, "close", 0},
		{area{x + inner - lipgloss.Width(close) - 1 - lipgloss.Width(config), y, lipgloss.Width(config), 1}, "config", 0},
	}
	rows := m.settingsHubRows()
	c := &m.chrome
	c.settingsRow = max(0, min(c.settingsRow, len(rows)-1))
	count := max(1, (height-7)/3)
	c.settingsOffset = max(0, min(c.settingsOffset, max(0, len(rows)-count)))
	if c.settingsRow < c.settingsOffset {
		c.settingsOffset = c.settingsRow
	}
	if c.settingsRow >= c.settingsOffset+count {
		c.settingsOffset = c.settingsRow - count + 1
	}
	rightLines := []string{}
	for i := c.settingsOffset; i < min(len(rows), c.settingsOffset+count); i++ {
		r := rows[i]
		value := "[" + ansi.Truncate(singleLine(r.value), max(8, right/2)-2, "…") + "]"
		label := ansi.Truncate(r.label, max(1, right-lipgloss.Width(value)-3), "…")
		line := bar(label, accent().Render(value), right)
		if !c.settingsNav && c.settingsRow == i {
			line = lipgloss.NewStyle().Width(right).Foreground(theme.Color(theme.Highlight)).Background(theme.Color(theme.Active)).Render(line)
		}
		rightLines = append(rightLines, line, muted().Render(ansi.Truncate(r.description, right, "…")), muted().Render(strings.Repeat("─", right)))
		hits = append(hits, settingsHubHit{area{x + nav + 3, y + 2 + (i-c.settingsOffset)*3, right, 2}, "row", i})
	}
	lines := []string{top, ""}
	for i := 0; i < height-6; i++ {
		left, content := "", ""
		if i < len(settingsTabs)*2 && i%2 == 0 {
			index := i / 2
			left = selectedRow(tr(settingsTabs[index]), c.settingsTab == index, nav)
			if c.settingsNav && c.settingsTab == index {
				left = heading().Render(left)
			}
			hits = append(hits, settingsHubHit{area{x, y + 2 + i, nav, 1}, "tab", index})
		}
		if i < len(rightLines) {
			content = rightLines[i]
		}
		lines = append(lines, lipgloss.NewStyle().Width(nav).Render(left)+" │ "+content)
	}
	note := m.preferencesNote
	if m.settingsLoading {
		note = tr("Загрузка настроек…")
	}
	if m.settingsError != "" {
		note = tr("Не удалось прочитать настройки dsh; F5 — повторить.")
	}
	lines = append(lines, muted().Render(ansi.Truncate(note, inner, "…")), muted().Render(ansi.Truncate(tr("Tab панели · ↑/↓ выбор · Enter изменить · F1 клавиши · Esc закрыть"), inner, "…")))
	return box, lines, hits
}
func (m *model) settingsHubView() (area, string) {
	box, lines, _ := m.settingsHubLayout()
	return box, panel(box.width, box.height, strings.Join(lines, "\n"), theme.Highlight)
}
func (m *model) settingsHubMouse(msg tea.MouseMsg) tea.Cmd {
	box, _, hits := m.settingsHubLayout()
	if !box.contains(msg.X, msg.Y) {
		return nil
	}
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		m.chrome.settingsNav = false
		delta := 1
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		m.chrome.settingsRow = max(0, min(len(m.settingsHubRows())-1, m.chrome.settingsRow+delta))
		return nil
	}
	if msg.Button != tea.MouseButtonLeft {
		return nil
	}
	for _, hit := range hits {
		if !hit.contains(msg.X, msg.Y) {
			continue
		}
		switch hit.action {
		case "tab":
			m.chrome.settingsTab, m.chrome.settingsRow, m.chrome.settingsOffset = hit.index, 0, 0
			m.chrome.settingsNav = true
			return nil
		case "row":
			m.chrome.settingsNav = false
			m.chrome.settingsRow = hit.index
			return m.settingsHubAction(m.settingsHubRows()[hit.index].action)
		default:
			return m.settingsHubAction(hit.action)
		}
	}
	return nil
}
