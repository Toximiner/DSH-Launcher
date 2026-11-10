package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"time"

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

// Only the redacted describe endpoint is exposed; this client has no write API.
func readSettings(ctx context.Context, login string) (*settingsSnapshot, error) {
	u, err := settingsURL(login)
	if err != nil {
		return nil, err
	}
	origin := u.Scheme + "://" + u.Host
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	q := u.Query()
	q.Set("interactions", "answer")
	u.Path = "/connect"
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("не удалось подключиться к dsh")
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound || len(jar.Cookies(u)) == 0 {
		return nil, fmt.Errorf("вход в dsh не выполнен (HTTP %d); получите свежий URL входа", res.StatusCode)
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return nil, errors.New("не удалось создать идентификатор запроса")
	}
	rpcID := hex.EncodeToString(id)
	body, _ := json.Marshal(map[string]any{"type": "client-request", "rpcId": rpcID, "method": "settings/describe", "payload": map[string]any{"args": map[string]any{}}})
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, origin+"/api/settings/describe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	res, err = client.Do(req)
	if err != nil {
		return nil, errors.New("не удалось прочитать настройки dsh")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("настройки dsh недоступны (HTTP %d)", res.StatusCode)
	}
	const limit = 8 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil || len(data) > limit {
		return nil, errors.New("ответ настроек dsh слишком велик или не прочитан")
	}
	var envelope struct {
		Type   string `json:"type"`
		ID     string `json:"rpcId"`
		Result struct {
			OK    *bool            `json:"ok"`
			Value settingsSnapshot `json:"value"`
		} `json:"result"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Type != "server-response" || envelope.ID != rpcID || envelope.Result.OK == nil {
		return nil, errors.New("неподдерживаемый ответ API настроек dsh")
	}
	if !*envelope.Result.OK {
		return nil, errors.New("dsh отказал в чтении настроек; проверьте доступность settings/describe")
	}
	if envelope.Result.Value.Namespaces == nil {
		return nil, errors.New("в ответе dsh отсутствуют разделы настроек")
	}
	snapshot := envelope.Result.Value
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

func (m *model) tuiSettingsView() string {
	return heading.Render("Настройки TUI · сведения о клиенте") + "\n\nВерсия     " + version + "\nИнтерфейс  Русский · тёмная тема\nТерминал   " + fmt.Sprintf("%d × %d", m.width, m.height) + "\n\nРабочая папка\n" + m.workspace + "\n\n" + muted.Render("Параметры клиента показаны только для чтения.")
}
