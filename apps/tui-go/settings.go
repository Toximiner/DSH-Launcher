package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type settingsNamespace struct {
	NS       string `json:"ns"`
	Value    any    `json:"value"`
	Revision int    `json:"revision"`
	Secrets  []struct {
		Path []string `json:"path"`
		Set  bool     `json:"set"`
	} `json:"secrets"`
}
type settingsSnapshot struct {
	Namespaces []settingsNamespace `json:"namespaces"`
}
type settingsResult struct {
	snapshot *settingsSnapshot
	err      error
}

func settingsURL(value string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil || u == nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") || u.Fragment != "" {
		return nil, errors.New("ожидается корневой HTTP(S) URL входа dsh с ?token=")
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
	default:
		return nil, errors.New("используйте localhost или SSH-туннель для доступа к dsh")
	}
	if u.Query().Get("token") == "" {
		return nil, errors.New("в URL входа dsh отсутствует ?token=")
	}
	return u, nil
}

// Settings remain read-only; session control uses separate, explicit operations.
func readSettings(ctx context.Context, login string) (*settingsSnapshot, error) {
	api, err := openAPI(ctx, login)
	if err != nil {
		return nil, err
	}
	defer api.close()
	var snapshot settingsSnapshot
	if err = api.rpc(ctx, "settings/describe", map[string]any{}, &snapshot); err != nil {
		return nil, err
	}
	if snapshot.Namespaces == nil {
		return nil, errors.New("в ответе dsh отсутствуют разделы настроек")
	}
	sort.SliceStable(snapshot.Namespaces, func(i, j int) bool { return snapshot.Namespaces[i].NS < snapshot.Namespaces[j].NS })
	return &snapshot, nil
}

func (m *model) loadSettings() tea.Cmd {
	if m.settingsLogin == "" || m.settingsLoading {
		return nil
	}
	m.settingsLoading = true
	m.settingsError = ""
	m.rebuild()
	return func() tea.Msg {
		snapshot, err := readSettings(m.settingsContext, m.settingsLogin)
		return settingsResult{snapshot, err}
	}
}

func (m *model) settingsView() string {
	if m.settingsLogin == "" {
		return heading.Render("Настройки dsh · только чтение") + "\n\n" + muted.Render("Для чтения настроек работающего dsh\nукажите --url или DSH_TUI_URL.\nНужен URL входа с ?token=.")
	}
	lines := []string{heading.Render("Настройки dsh · только чтение"), muted.Render("←/→ — раздел · r — обновить · колесо — прокрутка")}
	if m.settingsLoading {
		lines = append(lines, "", "Загрузка настроек…")
	}
	if m.settingsError != "" {
		lines = append(lines, "", "Ошибка: "+m.settingsError, "r — повторить чтение")
	}
	if m.settings == nil {
		return strings.Join(lines, "\n")
	}
	u, _ := settingsURL(m.settingsLogin)
	lines = append(lines, muted.Render(u.Host+" · снимок "+m.settingsUpdated.Format("15:04:05")))
	if m.settingsError != "" || m.settingsLoading {
		lines = append(lines, "Показан предыдущий снимок.")
	}
	if len(m.settings.Namespaces) == 0 {
		return strings.Join(append(lines, "", "dsh не вернул разделов настроек."), "\n")
	}
	ns := m.settings.Namespaces[m.settingsIndex]
	lines = append(lines, "", accent.Bold(true).Render(fmt.Sprintf("‹ %s ›  [%d/%d]", clean(ns.NS), m.settingsIndex+1, len(m.settings.Namespaces))), muted.Render(fmt.Sprintf("Ревизия %d · действующие значения", ns.Revision)), "")
	lines = append(lines, settingsFields(ns)...)
	lines = append(lines, "", muted.Render("Секреты скрыты. Изменение параметров пока недоступно."))
	return strings.Join(lines, "\n")
}

func settingsFields(ns settingsNamespace) []string {
	var lines []string
	hidden := map[string]bool{}
	pathKey := func(path []string) string { b, _ := json.Marshal(append([]string{}, path...)); return string(b) }
	for _, secret := range ns.Secrets {
		hidden[pathKey(secret.Path)] = true
	}
	var walk func(any, []string, int)
	walk = func(value any, path []string, depth int) {
		if len(lines) >= 1000 {
			return
		}
		if hidden[pathKey(path)] {
			return
		}
		label := strings.Join(path, ".")
		if label == "" {
			label = "значение"
		}
		if depth > 32 {
			lines = append(lines, clean(label)+" = [слишком глубокая структура]")
			return
		}
		switch v := value.(type) {
		case map[string]any:
			if len(v) > 0 {
				keys := make([]string, 0, len(v))
				for key := range v {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					walk(v[key], append(append([]string{}, path...), key), depth+1)
				}
				return
			}
		case []any:
			if len(v) > 0 {
				for i, item := range v {
					walk(item, append(append([]string{}, path...), fmt.Sprint(i)), depth+1)
				}
				return
			}
		}
		encoded, _ := json.Marshal(value)
		lines = append(lines, clean(label)+" = "+clean(string(encoded)))
	}
	walk(ns.Value, nil, 0)
	for _, secret := range ns.Secrets {
		state := "не задан"
		if secret.Set {
			state = "задан"
		}
		lines = append(lines, clean(strings.Join(secret.Path, "."))+" = [секрет: "+state+"]")
	}
	if len(lines) == 0 {
		lines = append(lines, "Нет заданных значений.")
	}
	if len(lines) >= 1000 {
		lines = append(lines, "… Вывод ограничен 1000 строками.")
	}
	return lines
}
