package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var settingsTabs = []string{"Общие", "Модели", "Встроенные плагины", "Пресеты агента", "Маркет плагинов"}

type settingsHubRow struct{ label, value, description, action string }
type settingsHubHit struct {
	area
	action string
	index  int
}

func (m *model) settingsHubGroupRows(group int) []settingsHubRow {
	open := tr("Открыть")
	switch group {
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

type settingsHubEntry struct {
	settingsHubRow
	group int
}

func (m *model) settingsHubEntries() []settingsHubEntry {
	var entries []settingsHubEntry
	query := strings.ToLower(strings.TrimSpace(m.chrome.settingsQuery))
	for group := range settingsTabs {
		if m.chrome.settingsTab >= 0 && m.chrome.settingsTab != group {
			continue
		}
		for _, row := range m.settingsHubGroupRows(group) {
			text := tr(settingsTabs[group]) + " " + row.label + " " + row.value + " " + row.description
			if query == "" || strings.Contains(strings.ToLower(text), query) {
				entries = append(entries, settingsHubEntry{row, group})
			}
		}
	}
	return entries
}

func (m *model) settingsHubRows() []settingsHubRow {
	var rows []settingsHubRow
	for _, entry := range m.settingsHubEntries() {
		rows = append(rows, entry.settingsHubRow)
	}
	return rows
}

func (m *model) settingsHubQueryKey(msg tea.KeyMsg) tea.Cmd {
	c := &m.chrome
	switch msg.Type {
	case tea.KeyCtrlU:
		c.settingsQuery = ""
	case tea.KeyBackspace:
		r := []rune(c.settingsQuery)
		if len(r) > 0 {
			c.settingsQuery = string(r[:len(r)-1])
		}
	case tea.KeySpace:
		if len(c.settingsQuery) < 256 {
			c.settingsQuery += " "
		}
	case tea.KeyRunes:
		text := singleLine(clean(string(msg.Runes)))
		if len(c.settingsQuery)+len(text) <= 256 {
			c.settingsQuery += text
		}
	}
	c.settingsRow, c.settingsOffset = 0, 0
	return nil
}

func (m *model) settingsHubKey(key string) tea.Cmd {
	c := &m.chrome
	rows := m.settingsHubRows()
	switch key {
	case "tab", "shift+tab":
		delta := 1
		if key == "shift+tab" {
			delta = -1
		}
		c.settingsTab = (c.settingsTab+1+delta+len(settingsTabs)+1)%(len(settingsTabs)+1) - 1
		c.settingsRow, c.settingsOffset = 0, 0
	case "up", "down", "home", "end", "pgup", "pgdown":
		if len(rows) == 0 {
			return nil
		}
		switch key {
		case "up":
			c.settingsRow = (c.settingsRow + len(rows) - 1) % len(rows)
		case "down":
			c.settingsRow = (c.settingsRow + 1) % len(rows)
		case "home":
			c.settingsRow = 0
		case "end":
			c.settingsRow = len(rows) - 1
		case "pgup":
			c.settingsRow = max(0, c.settingsRow-5)
		case "pgdown":
			c.settingsRow = min(len(rows)-1, c.settingsRow+5)
		}
	case "enter", "left", "right":
		if len(rows) == 0 {
			return nil
		}
		action := rows[max(0, min(c.settingsRow, len(rows)-1))].action
		if key != "enter" && action != "theme" && action != "language" && action != "mouse" && action != "input-history" {
			return nil
		}
		return m.settingsHubAction(action)
	case "f5":
		return m.loadSettings()
	}
	return nil
}

// Rendering and mouse input share the same physical rows, including group headings.
func (m *model) settingsHubLayout() (area, []string, []settingsHubHit) {
	geometry := m.chooserLayout(chooserSpec{Width: 60, Height: 26})
	box := geometry.Box
	inner := box.width - 4
	x, y := box.x+2, box.y+1
	c := &m.chrome
	entries := m.settingsHubEntries()
	c.settingsRow = max(0, min(c.settingsRow, len(entries)-1))
	group := tr("Все разделы")
	if c.settingsTab >= 0 && c.settingsTab < len(settingsTabs) {
		group = tr(settingsTabs[c.settingsTab])
	}
	close := "esc"
	lines := []string{bar(heading().Render(tr("Настройки TUI")), muted().Render(close), inner),
		ansi.Truncate(tr("Поиск: ")+c.settingsQuery, inner, "…"),
		muted().Render(ansi.Truncate(tr("Раздел: ")+group, inner, "…")), ""}
	hits := []settingsHubHit{{area{x + inner - lipgloss.Width(close), y, lipgloss.Width(close), 1}, "close", 0},
		{area{x, y + 2, inner, 1}, "group", 0}}
	type line struct {
		text  string
		index int
	}
	var content []line
	previous, selected := -1, 0
	for i, entry := range entries {
		if entry.group != previous {
			content = append(content, line{chooserGroup(tr(settingsTabs[entry.group])), -1})
			previous = entry.group
		}
		if i == c.settingsRow {
			selected = len(content)
		}
		value := ansi.Truncate(singleLine(entry.value), max(10, inner/2), "…")
		content = append(content, line{chooserItem(entry.label, value, i == c.settingsRow, true, inner), i})
	}
	count := max(1, box.height-10)
	c.settingsOffset = max(0, min(c.settingsOffset, max(0, len(content)-count)))
	if selected < c.settingsOffset {
		c.settingsOffset = selected
	}
	if selected >= c.settingsOffset+count {
		c.settingsOffset = selected - count + 1
	}
	for n := 0; n < count; n++ {
		index := c.settingsOffset + n
		text := ""
		if index < len(content) {
			text = content[index].text
			if content[index].index >= 0 {
				hits = append(hits, settingsHubHit{area{x, y + 4 + n, inner, 1}, "row", content[index].index})
			}
		} else if len(content) == 0 && n == 0 {
			text = muted().Render(tr("Ничего не найдено"))
		}
		lines = append(lines, text)
	}
	description := ""
	if len(entries) > 0 {
		description = entries[c.settingsRow].description
	}
	note := m.preferencesNote
	if m.settingsLoading {
		note = tr("Загрузка настроек…")
	}
	if m.settingsError != "" {
		note = tr("Не удалось прочитать настройки dsh; F5 — повторить.")
	}
	lines = append(lines, muted().Render(ansi.Truncate(description, inner, "…")),
		muted().Render(ansi.Truncate(note, inner, "…")),
		muted().Render(ansi.Truncate(tr("↑/↓ выбор · Enter открыть · ←/→ изменить"), inner, "…")),
		muted().Render(ansi.Truncate(tr("Tab раздел · F1 клавиши · F5 обновить · Esc закрыть"), inner, "…")))
	return box, lines, hits
}
func (m *model) settingsHubView() (area, string) {
	box, lines, _ := m.settingsHubLayout()
	return box, flatDialogSurface(box, lines)
}
func (m *model) settingsHubMouse(msg tea.MouseMsg) tea.Cmd {
	box, _, hits := m.settingsHubLayout()
	if !box.contains(msg.X, msg.Y) {
		return nil
	}
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
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
		case "group":
			return m.settingsHubKey("tab")
		case "row":
			rows := m.settingsHubRows()
			if hit.index >= 0 && hit.index < len(rows) {
				m.chrome.settingsRow = hit.index
				return m.settingsHubAction(rows[hit.index].action)
			}
		default:
			return m.settingsHubAction(hit.action)
		}
	}
	return nil
}
