package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Match the built-in cards served by the pinned dsh GUI, not Loader module names.
var builtinPluginCards = []struct{ ns, title, description string }{
	{"subagent", "Подагенты", "Глубина рекурсии и число подагентов."},
	{"web-search-deepseek", "Веб-поиск", "Настройки поискового провайдера DeepSeek."},
	{"agent-loop", "Цикл агента", "Управление выполнением вызовов инструментов."},
	{"bash-sandbox", "Оболочка", "Время выполнения команд и ограничения вывода."},
}

var hiddenPluginBundles = map[string]bool{
	"@deepseek-ai/dsh-base": true, "@deepseek-ai/dsh-web-app": true,
	"@deepseek-ai/dsh-headless": true, "@deepseek-ai/dsh-sdk-app": true,
	"@deepseek-ai/dsh-acp-app": true, "@deepseek-ai/dsh-sdk-minimal": true,
	"@deepseek-ai/dsh-experimental-terminal-bundle": true, "@deepseek-ai/dsh-experimental-tool-worktree": true,
}

// The GUI's optional bundle ordering and its secondary More list.
var pinnedPluginBundles = []string{
	"@deepseek-ai/dsh-experimental-agent-team-profile", "@deepseek-ai/dsh-experimental-voice-input-bundle",
	"@deepseek-ai/dsh-experimental-plugin-market-bundle", "@deepseek-ai/dsh-experimental-auto-review",
}
var morePluginBundles = map[string]bool{
	"@deepseek-ai/dsh-personalized-recommendations": true, "@deepseek-ai/dsh-experimental-inspector-profile": true,
	"@deepseek-ai/dsh-cua-bundle": true, "@deepseek-ai/dsh-experimental-cot-translation-bundle": true,
	"@deepseek-ai/dsh-experimental-session-search": true, "@deepseek-ai/dsh-experimental-session-titles-bundle": true,
	"@deepseek-ai/dsh-experimental-badge-skill-bundle": true, "@deepseek-ai/dsh-experimental-ralph-bundle": true,
	"@deepseek-ai/dsh-subagent-codex": true, "@deepseek-ai/dsh-subagent-claude-code": true,
}

func pluginBundleRank(name string) int {
	for i, pinned := range pinnedPluginBundles {
		if pinned == name {
			return i
		}
	}
	if morePluginBundles[name] {
		return len(pinnedPluginBundles) + 1
	}
	return len(pinnedPluginBundles)
}

func pluginDisplayText(value any) string {
	if text, ok := value.(string); ok {
		return clean(text)
	}
	texts := objectValue(value)
	if !activeEnglish.Load() {
		if text := stringValue(texts, "ru"); text != "" {
			return clean(text)
		}
	}
	return clean(stringValue(texts, "en"))
}

func pluginTogglePreflight(ctx context.Context, api *dshAPI, name string, baseline bool) error {
	var bundles []map[string]any
	if err := api.rpc(ctx, "pluginManager/listBundles", map[string]any{}, &bundles); err != nil {
		return err
	}
	for _, bundle := range bundles {
		if stringValue(bundle, "name") == name {
			if enabled, ok := bundle["enabled"].(bool); ok && enabled == baseline && bundle["readOnlyReason"] == nil {
				return nil
			}
			break
		}
	}
	return errors.New("plugin selection changed; refresh before writing")
}
func readPlugins(ctx context.Context, api *dshAPI, d *workbench) ([]benchRow, string, bool, error) {
	var inventory struct {
		Management bool             `json:"managementAvailable"`
		Entries    []map[string]any `json:"entries"`
	}
	if err := api.rpc(ctx, "pluginInventory/list", map[string]any{}, &inventory); err != nil {
		return nil, "", false, err
	}
	var settings settingsSnapshot
	if err := api.rpc(ctx, "settings/describe", map[string]any{}, &settings); err != nil {
		return nil, "", false, err
	}
	d.pluginSettings = &settings
	d.pluginManagement = inventory.Management
	rows := []benchRow{}
	namespaces := map[string]bool{}
	for _, ns := range settings.Namespaces {
		namespaces[ns.NS] = true
	}
	for _, card := range builtinPluginCards {
		ns := card.ns
		if ns == "bash-sandbox" && namespaces["pwsh-sandbox"] {
			ns = "pwsh-sandbox"
		}
		if !namespaces[ns] {
			continue
		}
		rows = append(rows, benchRow{ID: "builtin:" + ns, Title: tr(card.title), Body: tr(card.description), Data: map[string]any{"pluginGroup": "builtin", "namespace": ns}})
	}
	if inventory.Management {
		var bundles []map[string]any
		if err := api.rpc(ctx, "pluginManager/listBundles", map[string]any{}, &bundles); err != nil {
			return nil, "", false, err
		}
		groups := map[string][]benchRow{}
		for _, bundle := range bundles {
			name := stringValue(bundle, "name")
			if name == "" || hiddenPluginBundles[name] {
				continue
			}
			if bundle["listedByDefault"] == false && bundle["enabled"] != true && bundle["installed"] != true {
				continue
			}
			if bundle["official"] != true && bundle["installed"] != true && bundle["error"] == nil {
				continue
			}
			meta := objectValue(bundle["meta"])
			title := pluginDisplayText(meta["title"])
			if title == "" {
				title = name
			}
			description := pluginDisplayText(meta["description"])
			if description == "" {
				description = clean(stringValue(bundle, "description"))
			}
			group := "installed"
			if bundle["official"] == true {
				group = "experimental"
			}
			bundle["pluginGroup"] = group
			groups[group] = append(groups[group], benchRow{ID: name, Title: title, Body: description, Data: bundle})
		}
		for _, group := range []string{"experimental", "installed"} {
			sort.SliceStable(groups[group], func(i, j int) bool {
				if group == "experimental" {
					return pluginBundleRank(groups[group][i].ID) < pluginBundleRank(groups[group][j].ID)
				}
				return strings.ToLower(groups[group][i].Title) < strings.ToLower(groups[group][j].Title)
			})
			rows = append(rows, groups[group]...)
		}
	}
	for _, entry := range inventory.Entries {
		meta := objectValue(entry["meta"])
		title := pluginDisplayText(meta["title"])
		if title == "" {
			title = stringValue(entry, "moduleName")
		}
		entry["pluginGroup"] = "components"
		rows = append(rows, benchRow{ID: "component:" + stringValue(entry, "entryId"), Title: title, Body: pluginDisplayText(meta["description"]), Data: entry})
	}
	return rows, "", false, nil
}

func (m *model) openPluginSettings(row benchRow) tea.Cmd {
	d := m.workbench
	if d == nil || d.pluginSettings == nil || !d.pluginSettings.Writable {
		if d != nil {
			d.note = tr("Настройки dsh доступны только для чтения.")
		}
		return nil
	}
	for i, ns := range d.pluginSettings.Namespaces {
		if ns.NS == stringValue(row.Data, "namespace") {
			m.settings = d.pluginSettings
			m.settingsIndex = i
			return m.openSettingsForm(false)
		}
	}
	d.note = tr("Настройки плагина недоступны; F5 — обновить.")
	return nil
}

func pluginGroupTitle(group string) string {
	switch group {
	case "builtin":
		return tr("Настройки встроенных плагинов")
	case "experimental":
		return tr("Экспериментальные плагины")
	case "installed":
		return tr("Установленные плагины")
	default:
		return tr("Компоненты dsh · только чтение")
	}
}

type pluginHit struct {
	area
	action string
	index  int
}

func (m *model) pluginsPageBox() area {
	w, h := m.contentSize()
	x := 0
	if m.sidebarVisible() {
		x = sidebarWidth + sidebarGap
	}
	width := min(w, 110)
	return area{x + (w-width)/2, bodyTop, width, h}
}
func (m *model) pluginsBox() area {
	if m.workbench.pageView {
		return m.pluginsPageBox()
	}
	box := m.benchBox()
	return area{box.x + 2, box.y + 1, box.width - 4, box.height - 2}
}
func (m *model) pluginsLayout(box area) ([]string, []pluginHit) {
	d := m.workbench
	w := box.width
	refresh, add := "["+tr("Обновить")+"]", "[+ "+tr("Добавить плагин")+"]"
	toolbar := refresh + " " + add
	lines := []string{bar(heading().Render(tr("Плагины dsh")), accent().Render(toolbar), w), muted().Render(tr("Установка, включение и настройка плагинов"))}
	installed, market, components := "["+tr("Установленные")+"]", "["+tr("Маркет плагинов")+"]", "["+tr("Компоненты · F2")+"]"
	lines = append(lines, accent().Bold(true).Render(installed)+" "+accent().Render(market)+" "+muted().Render(components))
	d.search.Width = max(1, w-10)
	lines = append(lines, d.search.View(), muted().Render(strings.Repeat("─", w)))
	hits := []pluginHit{
		{area{box.x, box.y + 2, lipgloss.Width(installed), 1}, "installed", 0},
		{area{box.x + w - lipgloss.Width(toolbar), box.y, lipgloss.Width(refresh), 1}, "refresh", 0},
		{area{box.x + w - lipgloss.Width(add), box.y, lipgloss.Width(add), 1}, "add", 0},
		{area{box.x + lipgloss.Width(installed) + 1, box.y + 2, lipgloss.Width(market), 1}, "market", 0},
		{area{box.x + lipgloss.Width(installed) + lipgloss.Width(market) + 2, box.y + 2, lipgloss.Width(components), 1}, "components", 0},
	}
	rows := d.visible()
	d.index = max(0, min(d.index, max(0, len(rows)-1)))
	body := []string{}
	cardHits := []pluginHit{}
	toggleHits := []pluginHit{}
	moreHits := []pluginHit{}
	group := ""
	for i, row := range rows {
		current := stringValue(row.Data, "pluginGroup")
		if current != group || i == 0 {
			if len(body) > 0 {
				body = append(body, "")
			}
			header := heading().Render(pluginGroupTitle(current))
			if current == "experimental" {
				more := "[" + tr("Ещё · Ctrl+O") + "]"
				if d.pluginMore {
					more = "[" + tr("Свернуть · Ctrl+O") + "]"
				}
				moreHits = append(moreHits, pluginHit{area{w - lipgloss.Width(more), len(body), lipgloss.Width(more), 1}, "more", -1})
				header = bar(header, accent().Render(more), w)
			}
			body = append(body, header)
			group = current
		}
		start := len(body)
		state := "[" + tr("Выкл") + "]"
		if row.Data["enabled"] == true {
			state = "[" + tr("Вкл") + "]"
		}
		if row.Data["error"] != nil {
			state = "[" + tr("Ошибка") + "]"
		}
		if current == "builtin" {
			state = "[" + tr("Настроить") + "]"
		}
		if current == "components" {
			state = "[" + tr("Только чтение") + "]"
		}
		text := bar("  "+singleLine(row.Title), accent().Render(state), w)
		if i == d.index {
			text = selectedRow(singleLine(row.Title), true, max(1, w-lipgloss.Width(state)-1)) + " " + accent().Render(state)
		}
		body = append(body, text)
		description := singleLine(row.Body)
		if description == "" {
			description = tr("Описание отсутствует.")
		}
		body = append(body, "  "+muted().Render(ansi.Truncate(description, max(1, w-2), "…")), "")
		cardHits = append(cardHits, pluginHit{area{0, start, w, 2}, "card", i})
		if current == "experimental" || current == "installed" {
			toggleHits = append(toggleHits, pluginHit{area{w - lipgloss.Width(state), start, lipgloss.Width(state), 1}, "toggle", i})
		}
	}
	if len(body) == 0 {
		body = append(body, muted().Render(tr("Нет плагинов по этому запросу.")))
	}
	count := max(1, box.height-8)
	d.pluginOffset = max(0, min(d.pluginOffset, max(0, len(body)-count)))
	for _, hit := range cardHits {
		if hit.index == d.index {
			if hit.y < d.pluginOffset {
				d.pluginOffset = max(0, hit.y-1)
			}
			if hit.y+hit.height > d.pluginOffset+count {
				d.pluginOffset = hit.y + hit.height - count
			}
		}
	}
	for row := 0; row < count; row++ {
		index := d.pluginOffset + row
		line := ""
		if index < len(body) {
			line = body[index]
		}
		lines = append(lines, line)
	}
	for _, hit := range append(append(toggleHits, cardHits...), moreHits...) {
		y := box.y + 5 + hit.y - d.pluginOffset
		if y >= box.y+5 && y+hit.height <= box.y+5+count {
			hit.x, hit.y = box.x+hit.x, y
			hits = append(hits, hit)
		}
	}
	note := d.note
	if d.busy {
		note = tr("Запрос dsh…")
	}
	lines = append(lines, muted().Render(ansi.Truncate(clean(note), w, "…")), muted().Render(tr("↑/↓ выбор · Enter настройки/детали · Space вкл/выкл")), muted().Render(tr("F5 обновить · Ctrl+N добавить · F3 обновление · Ctrl+X удалить · Esc назад")))
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, w, "…")
	}
	return lines, hits
}
func (m *model) pluginsContent(box area) string {
	d := m.workbench
	if d.document == "plugin-details" {
		if row, ok := d.selected(); ok {
			body := heading().Render(singleLine(row.Title)) + "\n\n" + clean(row.Body) + "\n\n" + clean(row.ID)
			for _, field := range []struct{ key, label string }{{"version", tr("Версия: ")}, {"availability", tr("Доступность: ")}, {"moduleName", tr("Модуль: ")}, {"fiberPhase", tr("Состояние: ")}} {
				if value := stringValue(row.Data, field.key); value != "" {
					body += "\n" + field.label + clean(value)
				}
			}
			if enabled, ok := row.Data["enabled"].(bool); ok {
				body += "\n" + tr("Включён: ") + fmt.Sprint(enabled)
			}
			if code := stringValue(objectValue(row.Data["error"]), "code"); code != "" {
				body += "\n" + tr("Ошибка: ") + clean(code)
			}
			d.view.Width, d.view.Height = box.width, max(1, box.height-3)
			d.view.SetContent(ansi.Wrap(body, box.width, ""))
			return lipgloss.NewStyle().Width(box.width).Height(box.height).Render(d.view.View() + "\n" + muted().Render(ansi.Truncate(clean(d.note), box.width, "…")) + "\n" + muted().Render(ansi.Truncate(tr("Esc назад · Space вкл/выкл · F3 обновление · Ctrl+X удалить"), box.width, "…")))
		}
	}
	lines, _ := m.pluginsLayout(box)
	return lipgloss.NewStyle().Width(box.width).Height(box.height).Render(strings.Join(lines, "\n"))
}
func (m *model) pluginsKey(msg tea.KeyMsg) tea.Cmd {
	d := m.workbench
	if !msg.Paste {
		switch msg.Type {
		case tea.KeyEsc, tea.KeyLeft:
			if d.document != "" {
				d.document = ""
				return nil
			}
			return m.closeWorkbench()
		case tea.KeyF5:
			return m.loadWorkbench()
		case tea.KeyF2:
			d.pluginTechnical = !d.pluginTechnical
			d.search.SetValue("")
			d.index, d.pluginOffset = 0, 0
			return nil
		case tea.KeyCtrlO:
			selected := ""
			if row, ok := d.selected(); ok {
				selected = row.ID
			}
			d.pluginMore = !d.pluginMore
			for i, row := range d.visible() {
				if row.ID == selected {
					d.index = i
					break
				}
			}
			return nil
		case tea.KeyRight, tea.KeyF6:
			m.closeWorkbench()
			m.closeChrome()
			m.page = marketPage
			m.input.Blur()
			m.resize()
			return m.loadMarket()
		case tea.KeyUp, tea.KeyDown, tea.KeyHome, tea.KeyEnd, tea.KeyPgUp, tea.KeyPgDown:
			if d.document != "" {
				var cmd tea.Cmd
				d.view, cmd = d.view.Update(msg)
				return cmd
			}
			delta := 1
			if msg.Type == tea.KeyUp || msg.Type == tea.KeyPgUp {
				delta = -1
			}
			if msg.Type == tea.KeyPgUp || msg.Type == tea.KeyPgDown {
				delta *= max(1, (m.pluginsBox().height-8)/3)
			}
			d.index = max(0, min(max(0, len(d.visible())-1), d.index+delta))
			if msg.Type == tea.KeyHome {
				d.index = 0
			}
			if msg.Type == tea.KeyEnd {
				d.index = max(0, len(d.visible())-1)
			}
			return nil
		case tea.KeyCtrlN, tea.KeyF3, tea.KeyCtrlX, tea.KeySpace, tea.KeyEnter:
			return m.benchPrepare(msg.Type)
		}
	}
	if d.document != "" {
		return nil
	}
	var cmd tea.Cmd
	d.search, cmd = d.search.Update(msg)
	d.index, d.pluginOffset = 0, 0
	return cmd
}
func (m *model) pluginsMouse(msg tea.MouseMsg) tea.Cmd {
	box := m.pluginsBox()
	if !box.contains(msg.X, msg.Y) {
		return nil
	}
	m.navFocused = false
	if msg.Button == tea.MouseButtonWheelUp {
		return m.pluginsKey(tea.KeyMsg{Type: tea.KeyUp})
	}
	if msg.Button == tea.MouseButtonWheelDown {
		return m.pluginsKey(tea.KeyMsg{Type: tea.KeyDown})
	}
	if msg.Button != tea.MouseButtonLeft || m.workbench.document != "" {
		return nil
	}
	_, hits := m.pluginsLayout(box)
	for _, hit := range hits {
		if !hit.contains(msg.X, msg.Y) {
			continue
		}
		switch hit.action {
		case "refresh":
			return m.loadWorkbench()
		case "add":
			return m.benchPrepare(tea.KeyCtrlN)
		case "market":
			return m.pluginsKey(tea.KeyMsg{Type: tea.KeyRight})
		case "components":
			return m.pluginsKey(tea.KeyMsg{Type: tea.KeyF2})
		case "installed":
			if m.workbench.pluginTechnical {
				return m.pluginsKey(tea.KeyMsg{Type: tea.KeyF2})
			}
			return nil
		case "more":
			return m.pluginsKey(tea.KeyMsg{Type: tea.KeyCtrlO})
		case "card":
			m.workbench.index = hit.index
			return m.benchPrepare(tea.KeyEnter)
		case "toggle":
			m.workbench.index = hit.index
			return m.benchPrepare(tea.KeySpace)
		}
	}
	return nil
}
