package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme"
)

type marketPlugin struct {
	Name        string            `json:"name"`
	NPM         string            `json:"npm"`
	URL         string            `json:"url"`
	Version     string            `json:"version"`
	Description map[string]string `json:"description"`
}

func (p marketPlugin) pkg() string {
	if p.NPM != "" {
		return p.NPM
	}
	return p.Name
}
func (p marketPlugin) description() string {
	langs := []string{"ru", "en", "zh"}
	if activeEnglish.Load() {
		langs = []string{"en", "ru", "zh"}
	}
	for _, lang := range langs {
		if p.Description[lang] != "" {
			return p.Description[lang]
		}
	}
	return ""
}

type marketStatus struct {
	Busy   bool   `json:"busy"`
	Active bool   `json:"active"`
	Phase  string `json:"phase"`
	Done   int    `json:"done"`
	Total  int    `json:"total"`
}
type marketSnapshot struct {
	Profile      string            `json:"profile"`
	Version      string            `json:"version"`
	Plugins      []marketPlugin    `json:"plugins"`
	Installed    map[string]string `json:"installed"`
	Present      []string          `json:"present"`
	Status       marketStatus      `json:"status"`
	CatalogError bool              `json:"catalogError"`
}
type marketIntent struct{ Action, Package, URL, Profile string }
type marketResult struct {
	sequence  int
	snapshot  *marketSnapshot
	err       error
	operation bool
}
type marketPulse struct{ sequence int }
type marketProgress struct {
	sequence int
	status   marketStatus
	err      error
}
type marketHit struct {
	kind  string
	index int
	box   area
}

var npmPackage = regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`)

// The marketplace uses ordinary authenticated HTTP routes, not the dsh RPC envelope.
func (api *dshAPI) marketRequest(ctx context.Context, method, path string, body any, target any) error {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, api.origin+"/dsh-market/"+path, payload)
	if err != nil {
		return errors.New(tr("не удалось подготовить запрос маркета"))
	}
	req.Header.Set("Origin", api.origin)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := *api.client
	if method == http.MethodPost {
		client.Timeout = 10 * time.Minute
	}
	response, err := client.Do(req)
	if err != nil {
		return errors.New(tr("маркет не ответил; результат операции может быть неизвестен — обновите список"))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		switch response.StatusCode {
		case 404:
			return errors.New(tr("маркет недоступен: установите dshmarket и перезапустите dsh"))
		case 409:
			return errors.New(tr("маркет или агенты заняты; повторите после завершения работы"))
		case 401, 403:
			return errors.New(tr("маркет отказал в доступе; проверьте URL входа и права"))
		}
		return fmt.Errorf(tr("маркет отклонил запрос (HTTP %d)"), response.StatusCode)
	}
	limit := 8 << 20
	if path == "registry" {
		limit = 64 << 20
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, int64(limit+1)))
	if err != nil || len(data) > limit {
		return errors.New(tr("не удалось прочитать ответ маркета"))
	}
	if json.Unmarshal(data, target) != nil {
		return errors.New(tr("неподдерживаемый ответ маркета"))
	}
	return nil
}
func readMarket(ctx context.Context, api *dshAPI, catalog bool) (*marketSnapshot, error) {
	var caps struct {
		APIVersion    int    `json:"apiVersion"`
		MarketVersion string `json:"marketVersion"`
		Profile       string `json:"profile"`
	}
	if err := api.marketRequest(ctx, "GET", "api/v1/capabilities", nil, &caps); err != nil {
		return nil, err
	}
	if caps.APIVersion != 1 || caps.Profile == "" {
		return nil, errors.New(tr("неподдерживаемая версия API маркета"))
	}
	var installed struct {
		Installed map[string]string `json:"installed"`
		Present   []string          `json:"present"`
	}
	if err := api.marketRequest(ctx, "GET", "installed", nil, &installed); err != nil {
		return nil, err
	}
	if installed.Installed == nil {
		return nil, errors.New(tr("маркет не вернул список установленных пакетов"))
	}
	snapshot := &marketSnapshot{Profile: caps.Profile, Version: caps.MarketVersion, Installed: installed.Installed, Present: installed.Present}
	if err := api.marketRequest(ctx, "GET", "status", nil, &snapshot.Status); err != nil {
		return snapshot, err
	}
	if catalog {
		var registry struct {
			Registry struct {
				Plugins []marketPlugin `json:"plugins"`
			} `json:"registry"`
		}
		if err := api.marketRequest(ctx, "GET", "registry", nil, &registry); err != nil {
			snapshot.CatalogError = true
			return snapshot, err
		}
		if registry.Registry.Plugins == nil {
			snapshot.CatalogError = true
			return snapshot, errors.New(tr("маркет не вернул каталог"))
		}
		snapshot.Plugins = registry.Registry.Plugins
		sort.SliceStable(snapshot.Plugins, func(i, j int) bool { return snapshot.Plugins[i].Name < snapshot.Plugins[j].Name })
	}
	return snapshot, nil
}
func runMarketIntent(ctx context.Context, api *dshAPI, intent marketIntent) (*marketSnapshot, error) {
	before, err := readMarket(ctx, api, false)
	if err != nil {
		return before, err
	}
	if before.Profile != intent.Profile {
		return before, errors.New(tr("профиль маркета изменился; подтвердите операцию заново"))
	}
	if before.Status.Busy || before.Status.Active {
		return before, errors.New(tr("маркет занят другой операцией"))
	}
	if !npmPackage.MatchString(intent.Package) {
		return before, errors.New(tr("неподдерживаемое имя пакета"))
	}
	if intent.Package == "dshmarket" || intent.Package == "dsh-market" || intent.Package == "@deepseek-ai/dsh-base" || intent.Package == "@deepseek-ai/dsh-web-app" {
		return before, errors.New(tr("этот пакет обслуживается отдельно от TUI маркета"))
	}
	_, installed := before.Installed[intent.Package]
	var body map[string]any
	switch intent.Action {
	case "install":
		if installed {
			return before, errors.New(tr("пакет уже установлен"))
		}
		if !strings.HasPrefix(intent.URL, "https://") {
			return before, errors.New(tr("установка этого источника пока не поддерживается"))
		}
		body = map[string]any{"url": intent.URL}
	case "update", "uninstall":
		if !installed {
			return before, errors.New(tr("пакет больше не установлен"))
		}
		body = map[string]any{"name": intent.Package}
	default:
		return before, errors.New(tr("неизвестная операция маркета"))
	}
	var result struct {
		OK        *bool `json:"ok"`
		Cancelled bool  `json:"cancelled"`
	}
	err = api.marketRequest(ctx, "POST", intent.Action, body, &result)
	if err == nil && (result.OK == nil || !*result.OK) {
		err = errors.New(tr("маркет не подтвердил выполнение операции"))
	}
	after, readErr := readMarket(ctx, api, false)
	if err != nil {
		return after, err
	}
	if readErr != nil {
		return after, readErr
	}
	return after, nil
}

func (m *model) loadMarket() tea.Cmd {
	if m.marketBusy {
		return nil
	}
	if m.settingsLogin == "" {
		m.marketError = tr("Укажите URL входа dsh; для предустановки маркета используйте --start.")
		m.rebuild()
		return nil
	}
	m.marketBusy = true
	m.marketError = ""
	m.marketConfirm = nil
	m.marketSequence++
	sequence := m.marketSequence
	ctx, login := m.settingsContext, m.settingsLogin
	m.rebuild()
	return func() tea.Msg {
		api, err := openAPI(ctx, login)
		if err != nil {
			return marketResult{sequence: sequence, err: err}
		}
		defer api.close()
		snapshot, err := readMarket(ctx, api, true)
		return marketResult{sequence: sequence, snapshot: snapshot, err: err}
	}
}
func (m *model) marketRows() []marketPlugin {
	if m.market == nil {
		return nil
	}
	plugins := m.market.Plugins
	if m.marketInstalledOnly {
		plugins = nil
		byPackage := map[string]marketPlugin{}
		for _, p := range m.market.Plugins {
			byPackage[p.pkg()] = p
		}
		for name := range m.market.Installed {
			p, ok := byPackage[name]
			if !ok {
				p = marketPlugin{Name: name, NPM: name}
			}
			plugins = append(plugins, p)
		}
		sort.SliceStable(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })
	}
	query := strings.ToLower(strings.TrimSpace(m.marketSearch.Value()))
	rows := []marketPlugin{}
	for _, p := range plugins {
		if query == "" || strings.Contains(strings.ToLower(p.Name+" "+p.pkg()+" "+p.description()), query) {
			rows = append(rows, p)
		}
	}
	return rows
}
func (m *model) confirmMarket(action string) {
	if m.marketBusy || m.market == nil {
		return
	}
	rows := m.marketRows()
	if len(rows) == 0 {
		return
	}
	p := rows[min(m.marketIndex, len(rows)-1)]
	if action == "install" && p.NPM == "" {
		m.marketError = tr("Git-only источник пока доступен только для просмотра.")
		m.rebuild()
		return
	}
	m.marketConfirm = &marketIntent{Action: action, Package: p.pkg(), URL: p.URL, Profile: m.market.Profile}
	m.marketError = ""
	m.rebuild()
}
func (m *model) executeMarket() tea.Cmd {
	if m.marketConfirm == nil || m.marketBusy {
		return nil
	}
	intent := *m.marketConfirm
	m.marketConfirm = nil
	m.marketBusy = true
	m.marketOperating = true
	m.marketError = ""
	m.marketSequence++
	sequence := m.marketSequence
	ctx, login := m.settingsContext, m.settingsLogin
	m.rebuild()
	command := func() tea.Msg {
		api, err := openAPI(ctx, login)
		if err != nil {
			return marketResult{sequence: sequence, operation: true, err: err}
		}
		defer api.close()
		snapshot, err := runMarketIntent(ctx, api, intent)
		return marketResult{sequence: sequence, snapshot: snapshot, operation: true, err: err}
	}
	return tea.Batch(command, m.marketTimer())
}
func (m *model) marketTimer() tea.Cmd {
	sequence := m.marketSequence
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return marketPulse{sequence} })
}
func (m *model) marketKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if msg.Paste && !m.marketEditing {
		return nil, true
	}
	if !m.marketEditing && m.marketConfirm == nil && msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && msg.Runes[0] == '/' {
		m.marketEditing = true
		cmd := m.marketSearch.Focus()
		m.marketSearch.SetValue(strings.ReplaceAll(clean(string(msg.Runes[1:])), "\n", " "))
		m.marketIndex = 0
		m.rebuild()
		return cmd, true
	}
	if m.marketConfirm != nil {
		switch msg.String() {
		case "y", "enter":
			return m.executeMarket(), true
		case "n", "esc":
			m.marketConfirm = nil
			m.rebuild()
			return nil, true
		}
		return nil, true
	}
	if m.marketEditing {
		if msg.String() == "enter" || msg.String() == "esc" {
			m.marketEditing = false
			m.marketSearch.Blur()
			m.marketIndex = 0
			m.rebuild()
			return nil, true
		}
		var cmd tea.Cmd
		m.marketSearch, cmd = m.marketSearch.Update(msg)
		value := m.marketSearch.Value()
		flat := strings.ReplaceAll(clean(value), "\n", " ")
		if value != flat {
			m.marketSearch.SetValue(flat)
		}
		m.marketIndex = 0
		m.rebuild()
		return cmd, true
	}
	switch msg.String() {
	case "/":
		m.marketEditing = true
		cmd := m.marketSearch.Focus()
		m.rebuild()
		return cmd, true
	case "r":
		return m.loadMarket(), true
	case "t":
		m.marketInstalledOnly = !m.marketInstalledOnly
		m.marketIndex = 0
		m.rebuild()
		return nil, true
	case "up", "down", "pgup", "pgdown":
		delta := 1
		if msg.String() == "up" {
			delta = -1
		}
		if msg.String() == "pgup" {
			delta = -5
		}
		if msg.String() == "pgdown" {
			delta = 5
		}
		rows := m.marketRows()
		m.marketIndex = max(0, min(max(0, len(rows)-1), m.marketIndex+delta))
		m.rebuild()
		return nil, true
	case "i":
		m.confirmMarket("install")
		return nil, true
	case "u":
		m.confirmMarket("update")
		return nil, true
	case "d":
		m.confirmMarket("uninstall")
		return nil, true
	}
	return nil, false
}
func (m *model) marketView() string {
	w, h := max(1, m.viewport.Width), max(1, m.viewport.Height)
	inner := max(1, w-4)
	contentHeight := max(1, h-2)
	lines := []string{}
	m.marketHits = nil
	add := func(text string) {
		lines = append(lines, ansi.Truncate(strings.ReplaceAll(text, "\n", " "), inner, "…"))
	}
	hit := func(kind string, index, x, width int) {
		if x >= inner || width <= 0 {
			return
		}
		m.marketHits = append(m.marketHits, marketHit{kind, index, area{x + 2, len(lines) + 1, min(width, inner-x), 1}})
	}
	title := tr("Маркет плагинов")
	if m.market != nil {
		title += " · " + singleLine(m.market.Profile)
	}
	refresh := "[r " + tr("Обновить") + "]"
	hit("refresh", 0, max(0, inner-lipgloss.Width(refresh)), lipgloss.Width(refresh))
	add(bar(heading().Render(title), muted().Render(refresh), inner))
	catalog, installed := "["+tr("Каталог")+"]", "["+tr("Установленные")+"]"
	hit("catalog", 0, 0, lipgloss.Width(catalog))
	hit("installed", 0, lipgloss.Width(catalog)+1, lipgloss.Width(installed))
	filter := accent().Bold(true).Render(catalog) + " " + muted().Render(installed)
	if m.marketInstalledOnly {
		filter = muted().Render(catalog) + " " + accent().Bold(true).Render(installed)
	}
	rows := m.marketRows()
	m.marketIndex = max(0, min(m.marketIndex, max(0, len(rows)-1)))
	position := "0/0"
	if len(rows) > 0 {
		position = fmt.Sprintf("%d/%d", m.marketIndex+1, len(rows))
	}
	add(bar(filter, muted().Render(position), inner))
	hit("search", 0, 0, inner)
	m.marketSearch.Width = max(1, inner-lipgloss.Width(m.marketSearch.Prompt))
	if m.marketEditing {
		add(m.marketSearch.View())
	} else {
		add(muted().Render(tr("Поиск: ") + m.marketSearch.Value()))
	}
	status := m.marketMessage
	if m.marketBusy {
		status = tr("Запрос маркета…")
		if m.marketOperating {
			status = tr("Операция выполняется…")
		}
	}
	if m.market != nil && (m.market.Status.Busy || m.market.Status.Active) {
		status = tr("Маркет занят · ") + m.market.Status.Phase
		if m.market.Status.Total > 0 {
			status += fmt.Sprintf(" · %d/%d", m.market.Status.Done, m.market.Status.Total)
		}
	}
	if m.marketError != "" {
		status = "! " + m.marketError
		add(lipgloss.NewStyle().Foreground(theme.Color(theme.Warning)).Render(singleLine(status)))
	} else {
		add(muted().Render(singleLine(status)))
	}
	add("")
	listHeight := max(1, min(14, contentHeight-13))
	start := max(0, min(m.marketIndex-listHeight+1, max(0, len(rows)-listHeight)))
	for i := 0; i < listHeight; i++ {
		index := start + i
		if index >= len(rows) {
			if i == 0 {
				add(muted().Render(tr("Нет результатов.")))
			} else {
				add("")
			}
			continue
		}
		plugin := rows[index]
		state := tr("Доступен")
		if plugin.NPM == "" {
			state = tr("Только чтение")
		}
		version := singleLine(plugin.Version)
		if installedVersion, ok := m.market.Installed[plugin.pkg()]; ok {
			state = tr("Установлен")
			version = singleLine(installedVersion)
		}
		if version != "" {
			state += " · " + version
		}
		name := singleLine(plugin.Name)
		if name == "" {
			name = singleLine(plugin.pkg())
		}
		hit("row", index, 0, inner)
		add(chooserItem(name, state, index == m.marketIndex, true, inner))
	}
	add("")
	if len(rows) > 0 {
		plugin := rows[m.marketIndex]
		add(heading().Render(singleLine(plugin.pkg())))
		description := strings.Split(ansi.Hardwrap(singleLine(plugin.description()), inner, true), "\n")
		for i := 0; i < 2; i++ {
			line := ""
			if i < len(description) {
				line = description[i]
			}
			if i == 1 && len(description) > 2 {
				line = ansi.Truncate(line, max(1, inner-1), "") + "…"
			}
			add(muted().Render(line))
		}
		spec := tr("не установлен")
		if plugin.NPM == "" {
			spec = tr("Git-only источник пока доступен только для просмотра.")
		}
		if value, ok := m.market.Installed[plugin.pkg()]; ok {
			spec = tr("в профиле: ") + singleLine(value)
			if m.market.Present != nil {
				spec = tr("файлы не найдены: ") + singleLine(value)
				for _, name := range m.market.Present {
					if name == plugin.pkg() {
						spec = tr("установлен: ") + singleLine(value)
						break
					}
				}
			}
		}
		if plugin.Version != "" {
			spec += " · " + tr("Версия каталога: ") + singleLine(plugin.Version)
		}
		add(muted().Render(spec))
		add(muted().Render(singleLine(plugin.URL)))
	} else {
		for range 5 {
			add("")
		}
	}
	for len(lines) < contentHeight-2 {
		add("")
	}
	if m.marketConfirm != nil {
		intent := m.marketConfirm
		action := map[string]string{"install": tr("Установить"), "update": tr("Обновить"), "uninstall": tr("Удалить")}[intent.Action]
		add(lipgloss.NewStyle().Foreground(theme.Color(theme.Warning)).Render(fmt.Sprintf("%s: %s · %s", action, singleLine(intent.Package), singleLine(intent.Profile))))
		no, yes := tr("[n Отмена]"), tr("[y Подтвердить]")
		hit("no", 0, 0, lipgloss.Width(no))
		hit("yes", 0, lipgloss.Width(no)+1, lipgloss.Width(yes))
		add(no + " " + yes)
	} else {
		actions := []struct{ key, label string }{{"install", tr("[i Уст.]")}, {"update", tr("[u Обн.]")}, {"uninstall", tr("[d Удал.]")}}
		text := ""
		for _, action := range actions {
			if text != "" {
				text += " "
			}
			hit(action.key, 0, lipgloss.Width(text), lipgloss.Width(action.label))
			text += action.label
		}
		add(text)
		add(muted().Render(tr("Операции требуют подтверждения.")))
	}
	return flatDialogSurface(area{width: w, height: h}, lines)
}

func newMarketSearch() textinput.Model {
	input := textinput.New()
	input.Prompt = tr("Поиск: ")
	input.CharLimit = 200
	return input
}
