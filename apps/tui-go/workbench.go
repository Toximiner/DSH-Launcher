package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type benchRow struct {
	ID, Title, Body string
	Data            map[string]any
}
type benchAction struct {
	Method string
	Args   map[string]any
	Label  string
}
type workbench struct {
	lastResult                               string
	documentOwner                            string
	fileBody                                 string
	ctx                                      context.Context
	cancel                                   context.CancelFunc
	id                                       string
	epoch, serial, index                     int
	kind, location, document, note           string
	rows                                     []benchRow
	search                                   textinput.Model
	view                                     viewport.Model
	busy, writing, blocked, focused, editing bool
	editor                                   textarea.Model
	action                                   *benchAction
	fileOffset                               int
	fileEOF                                  bool
	confirm                                  bool
}
type benchTerminalDone struct{ err error }
type benchResult struct {
	feedback string
	dialog   *workbench
	serial   int
	rows     []benchRow
	document string
	fileEOF  bool
	err      error
	action   bool
	connect  string
}

func prettyValue(v any) string                      { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }
func stringValue(m map[string]any, k string) string { s, _ := m[k].(string); return s }
func objectValue(v any) map[string]any              { m, _ := v.(map[string]any); return m }
func arrayValue(v any) []any                        { a, _ := v.([]any); return a }
func projectionValues(ctx context.Context, api *dshAPI, id string) (map[string]any, error) {
	var p struct {
		Kind   string         `json:"kind"`
		Values map[string]any `json:"values"`
	}
	err := api.rpc(ctx, "session/projections", map[string]any{"request": map[string]any{"sessionId": id}}, &p)
	if err != nil || p.Kind != "sequenced" {
		return nil, errors.New("Не удалось прочитать сессию.")
	}
	return p.Values, nil
}
func (m *model) openWorkbench(kind string) tea.Cmd {
	if m.chat == nil || m.session == nil || !m.chat.ready || m.chatSessionChanging() {
		m.append("Подключите сессию dsh.")
		return nil
	}
	ctx, cancel := context.WithCancel(m.chat.ctx)
	search := textinput.New()
	search.Prompt = "Поиск: "
	search.CharLimit = 128
	d := &workbench{ctx: ctx, cancel: cancel, id: m.session.ID, epoch: m.chat.epoch, kind: kind, editor: textarea.New(), focused: m.input.Focused(), search: search, view: viewport.New(1, 1), fileOffset: 1}
	m.workbench = d
	m.input.Blur()
	return tea.Batch(d.search.Focus(), m.loadWorkbench())
}
func (m *model) closeWorkbench() tea.Cmd {
	d := m.workbench
	if d == nil {
		return nil
	}
	d.cancel()
	d.editor.SetValue("")
	m.workbench = nil
	if d.focused && m.page == chatPage && !m.navFocused {
		return m.input.Focus()
	}
	return nil
}
func (d *workbench) visible() []benchRow {
	q := strings.ToLower(strings.TrimSpace(d.search.Value()))
	var out []benchRow
	for _, r := range d.rows {
		if q == "" || strings.Contains(strings.ToLower(r.ID+" "+r.Title), q) {
			out = append(out, r)
		}
	}
	return out
}
func (d *workbench) selected() (benchRow, bool) {
	rows := d.visible()
	if d.index < 0 || d.index >= len(rows) {
		return benchRow{}, false
	}
	return rows[d.index], true
}
func readBench(ctx context.Context, api *dshAPI, d *workbench) ([]benchRow, string, bool, error) {
	var rows []benchRow
	var value any
	var err error
	add := func(id, title, body string, data map[string]any) {
		rows = append(rows, benchRow{id, clean(title), clean(body), data})
	}
	switch d.kind {
	case "workspaces":
		raw, e := firstBenchFrame(ctx, api, "workspace/follow", map[string]any{})
		err = e
		if err == nil {
			if json.Unmarshal(raw, &value) != nil {
				err = errors.New("invalid workspaces")
			} else {
				for _, v := range arrayValue(objectValue(objectValue(value)["value"])["items"]) {
					r := objectValue(v)
					add(stringValue(r, "workspaceId"), stringValue(r, "title"), fmt.Sprintf("Папка: %v\nСессии: %v\nCtrl+N — новая сессия здесь. F3 — новая сессия в другой папке.", r["path"], r["sessionIds"]), r)
				}
			}
		}
	case "packages":
		err = api.rpc(ctx, "pluginManager/listBundles", map[string]any{}, &value)
		if err == nil {
			for _, v := range arrayValue(value) {
				r := objectValue(v)
				add(stringValue(r, "name"), stringValue(r, "name")+" · "+stringValue(r, "version"), prettyValue(map[string]any{"name": r["name"], "version": r["version"], "enabled": r["enabled"], "installed": r["installed"], "availability": r["availability"], "removable": r["removable"], "updatable": r["updatable"], "readOnlyReason": r["readOnlyReason"], "description": r["description"]}), r)
			}
		}
	case "presets":
		err = api.rpc(ctx, "agentPresets/list", map[string]any{}, &value)
		if err == nil {
			for _, v := range arrayValue(objectValue(value)["presets"]) {
				r := objectValue(v)
				id := stringValue(r, "id")
				title := id + " · " + stringValue(r, "name")
				if r["isDefault"] == true {
					title += " · по умолчанию"
				}
				if stringValue(r, "broken") != "" {
					title += " · недоступен"
				}
				add(id, title, stringValue(r, "description"), r)
			}
		}
		if err == nil {
			p, e := projectionValues(ctx, api, d.id)
			err = e
			if e == nil {
				return rows, "Текущий пресет: " + fmt.Sprint(p["agentPreset"]) + "\nВыбор для текущей сессии разрешён dsh только до первого хода.\nCtrl+N — новая сессия с выбранным пресетом.", false, nil
			}
		}
	case "commands":
		err = api.rpc(ctx, "commands/list", map[string]any{"agentId": d.id}, &value)
		if err == nil {
			for _, v := range arrayValue(value) {
				r := objectValue(v)
				id := stringValue(r, "name")
				add(id, "/"+id, stringValue(r, "description"), r)
			}
		}
	case "plugins":
		err = api.rpc(ctx, "pluginInventory/list", map[string]any{}, &value)
		if err == nil {
			for _, v := range arrayValue(objectValue(value)["entries"]) {
				r := objectValue(v)
				id := stringValue(r, "entryId")
				add(id, stringValue(r, "moduleName")+" · "+fmt.Sprint(r["fiberPhase"]), fmt.Sprintf("ID: %s\nВключён: %v\nСостояние: %v\n\nF4 — настройки dsh по схемам.\n/commands — доступные команды плагинов.\n/market — установка и обновление.\nВеб-экраны требуют отдельного адаптера.", id, r["enabled"], r["fiberPhase"]), r)
			}
		}
	case "files":
		if d.document != "" {
			var file struct {
				Text          string `json:"text"`
				Offset, Lines int
				EOF           bool   `json:"eof"`
				Version       string `json:"version"`
			}
			err = api.rpc(ctx, "workspaceFiles/read", map[string]any{"workspaceFileScopeId": d.id, "path": d.document, "range": map[string]any{"offset": d.fileOffset, "limit": 200}}, &file)
			if err == nil {
				return nil, fmt.Sprintf("%s · строки %d–%d · версия %s\n\n%s", d.document, file.Offset, file.Offset+file.Lines-1, file.Version, file.Text), file.EOF, nil
			}
		} else {
			err = api.rpc(ctx, "workspaceFiles/list", map[string]any{"workspaceFileScopeId": d.id, "path": path.Join(".", d.location)}, &value)
			if err == nil {
				r := objectValue(value)
				if r["truncated"] == true {
					return nil, "", false, errors.New("Каталог превышает лимит dsh; сузьте путь.")
				}
				for _, v := range arrayValue(r["entries"]) {
					entry := objectValue(v)
					name := stringValue(entry, "name")
					if name == "" || path.Base(name) != name || name == "." || name == ".." {
						return nil, "", false, errors.New("Неподдерживаемое имя файла.")
					}
					label := name
					if entry["type"] == "directory" {
						label += "/"
					}
					add(path.Join(d.location, name), label, fmt.Sprintf("Тип: %v · байты: %v", entry["type"], entry["size"]), entry)
				}
			}
		}
	case "jobs":
		if d.document != "" {
			text, e := readBenchJob(ctx, api, d.documentOwner, d.document)
			return nil, text, true, e
		}
		raw, e := firstBenchFrame(ctx, api, "job/list", map[string]any{"request": map[string]any{"sessionId": d.id}})
		err = e
		if err == nil {
			if json.Unmarshal(raw, &value) != nil {
				err = errors.New("Некорректный список заданий.")
			} else {
				for _, v := range arrayValue(objectValue(value)["jobs"]) {
					r := objectValue(v)
					owner := stringValue(r, "owner")
					if owner != "" && owner != d.id {
						return nil, "", false, errors.New("Список содержит задание другой сессии; F5 перечитать.")
					}
					if owner == "" {
						owner = "Общее задание Host"
					}
					add(stringValue(r, "id"), stringValue(r, "label")+" · "+stringValue(r, "status"), fmt.Sprintf("Тип: %s\nСостояние: %s\nВладелец: %s\nПрогресс: %s\n\nCtrl+X — запросить остановку задания.", stringValue(r, "kind"), stringValue(r, "status"), owner, stringValue(r, "progress")), r)
				}
			}
		}
	case "schedules":
		if d.document != "" {
			err = api.rpc(ctx, "schedule/history", map[string]any{"request": map[string]any{"id": d.document, "limit": 100}}, &value)
			if err != nil || stringValue(objectValue(value), "code") != "" || objectValue(value)["records"] == nil {
				return nil, "", false, errors.New("История расписания недоступна; F5 обновить.")
			}
			return nil, "История расписания · последние 100 записей\n" + clean(prettyValue(value)), true, nil
		}
		err = api.rpc(ctx, "schedule/list", map[string]any{"request": map[string]any{"limit": 200, "offset": 0}}, &value)
		if err == nil {
			r := objectValue(value)
			if total, ok := r["total"].(float64); ok && total > 200 {
				return nil, "", false, errors.New("Более 200 расписаний; используйте фильтр на сервере.")
			}
			for _, v := range arrayValue(r["items"]) {
				r := objectValue(v)
				add(stringValue(r, "id"), stringValue(r, "title")+" · "+stringValue(r, "status"), prettyValue(map[string]any{"id": r["id"], "title": r["title"], "prompt": r["prompt"], "status": r["status"], "target": r["target"], "scheduledAt": r["scheduledAt"], "updatedAt": r["updatedAt"]}), r)
			}
		}
	case "agents", "tasks":
		var p map[string]any
		p, err = projectionValues(ctx, api, d.id)
		if err == nil {
			if d.kind == "agents" {
				for _, v := range arrayValue(p["subagentCatalog"]) {
					r := objectValue(v)
					add(stringValue(r, "id"), stringValue(r, "name")+" · "+stringValue(r, "label")+" · "+stringValue(r, "mode"), fmt.Sprintf("Сессия: %v\nРежим: %v\nEnter — подключить дочернюю сессию.\nCtrl+X — запросить остановку.", r["id"], r["mode"]), r)
				}
			} else {
				for _, v := range arrayValue(objectValue(p["agentTeam"])["tasks"]) {
					r := objectValue(v)
					add(fmt.Sprint(r["id"]), fmt.Sprint(r["id"])+" · "+stringValue(r, "subject")+" · "+stringValue(r, "status"), prettyValue(r), r)
				}
			}
		}
	case "terminals":
		err = api.rpc(ctx, "terminal/list", map[string]any{"sessionId": d.id}, &value)
		if err == nil {
			for _, v := range arrayValue(value) {
				r := objectValue(v)
				add(stringValue(r, "id"), stringValue(r, "title")+" · "+stringValue(r, "state"), fmt.Sprintf("Папка: %v\nОболочка: %v\nСостояние: %v\nCtrl+X — закрыть терминал.\nEnter — открыть интерактивный терминал. Ctrl+] — вернуться в TUI.", r["cwd"], objectValue(r["shell"])["path"], r["state"]), r)
			}
		}
	default:
		err = errors.New("Раздел не поддержан.")
	}
	if err != nil {
		return nil, "", false, errors.New("Не удалось прочитать раздел dsh. Старый снимок сохранён; F5 — обновить.")
	}
	if len(rows) > 2000 {
		return nil, "", false, errors.New("Слишком большой список (лимит 2000).")
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if r.ID == "" || seen[r.ID] {
			return nil, "", false, errors.New("Некорректные идентификаторы списка dsh.")
		}
		seen[r.ID] = true
	}
	if d.kind == "plugins" || d.kind == "files" {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Title < rows[j].Title })
	}
	return rows, "", false, nil
}
func (m *model) loadWorkbench() tea.Cmd {
	d := m.workbench
	if d == nil || d.busy {
		return nil
	}
	d.busy = true
	d.serial++
	d.note = "Чтение снимка dsh…"
	serial, login := d.serial, m.settingsLogin
	// The worker receives a detached navigation snapshot.
	snapshot := *d
	return func() tea.Msg {
		result := benchResult{dialog: d, serial: serial}
		ctx, cancel := context.WithTimeout(d.ctx, 20*time.Second)
		defer cancel()
		api, err := openAPI(ctx, login)
		if err == nil {
			defer api.close()
			result.rows, result.document, result.fileEOF, result.err = readBench(ctx, api, &snapshot)
		} else {
			result.err = errors.New("Раздел dsh недоступен.")
		}
		return result
	}
}
func (m *model) handleBench(result benchResult) tea.Cmd {
	d := m.workbench
	if d == nil || d != result.dialog || d.ctx.Err() != nil || d.serial != result.serial || m.chat == nil || m.chat.epoch != d.epoch || m.session == nil || m.session.ID != d.id {
		return nil
	}
	writing := d.writing
	d.busy, d.writing = false, false
	if result.err != nil {
		d.note = result.err.Error()
		d.blocked = true
		return nil
	}
	if result.connect != "" {
		m.closeWorkbench()
		return m.sessionOperation("connect", result.connect)
	}
	if result.action {
		d.action = nil
		d.confirm = false
		d.editing = false
		d.editor.SetValue("")
		d.lastResult = result.feedback
		d.note = "Ответ dsh получен. F5 — перечитать состояние."
		if result.feedback != "" {
			d.note = result.feedback + " · F5 перечитать"
		}
		d.blocked = true
		return nil
	}
	d.rows = result.rows
	d.fileEOF = result.fileEOF
	d.note = "Снимок dsh · F5 обновить · Esc назад"
	d.blocked = false
	d.action = nil
	d.confirm = false
	if (d.kind == "files" || d.kind == "jobs" || d.kind == "schedules") && d.document != "" {
		d.fileBody = result.document
		d.view.SetContent(ansi.Hardwrap(clean(result.document), max(1, d.view.Width), true))
	} else if d.kind == "presets" {
		d.note = result.document
	}
	d.index = min(d.index, max(0, len(d.visible())-1))
	if !writing {
		d.view.GotoTop()
	}
	return nil
}
func (m *model) benchExecute() tea.Cmd {
	d := m.workbench
	if d == nil || d.busy || d.blocked || d.action == nil || !d.confirm || m.isSessionArchived(d.id) {
		return nil
	}
	action := *d.action
	d.busy, d.writing = true, true
	d.serial++
	serial, login := d.serial, m.settingsLogin
	return func() tea.Msg {
		result := benchResult{dialog: d, serial: serial, action: true}
		duration := 20 * time.Second
		var value any
		if strings.HasPrefix(action.Method, "pluginManager/") {
			duration = 10 * time.Minute
		}
		ctx, cancel := context.WithTimeout(d.ctx, duration)
		defer cancel()
		api, err := openAPI(ctx, login)
		if err == nil {
			defer api.close()
			if strings.HasPrefix(action.Method, "pluginManager/") {
				api.client.Timeout = 10 * time.Minute
			}
			target := any(&value)
			if action.Method == "terminal/close" {
				target = nil
			}
			err = api.rpc(ctx, action.Method, action.Args, target)
			if action.Method == "job/kill" || action.Method == "terminal/close" {
				if err == nil {
					result.action = true
				}
			}
			if err == nil {
				if code := stringValue(objectValue(value), "code"); code != "" {
					err = errors.New("action refused")
				}
				if action.Method == "commands/execute" && stringValue(objectValue(objectValue(value)["result"]), "kind") != "success" {
					err = errors.New("command failed")
				}
				r := objectValue(value)
				switch action.Method {
				case "subagents/interruptByParent":
					if r["accepted"] != true {
						err = errors.New("interruption unconfirmed")
					}
				case "subagents/prompt":
					if stringValue(r, "messageId") == "" {
						err = errors.New("message unconfirmed")
					}
				case "terminal/create":
					if r["id"] != objectValue(action.Args["request"])["id"] {
						err = errors.New("terminal unconfirmed")
					}
				case "job/kill":
					if r["outcome"] != "requested" && r["outcome"] != "already-finished" {
						err = errors.New("stop unconfirmed")
					}
				case "schedule/create", "schedule/edit", "schedule/run":
					request := objectValue(action.Args["request"])
					if request["action"] == "delete" {
						if r["deleted"] != true {
							err = errors.New("deletion unconfirmed")
						}
					} else if stringValue(r, "id") == "" || stringValue(r, "updatedAt") == "" {
						err = errors.New("schedule unconfirmed")
					}
				}
				if action.Method == "agentPresets/select" && value != action.Args["agentPreset"] {
					err = errors.New("selection unconfirmed")
				}
				if action.Method == "session/create" {
					result.connect = stringValue(objectValue(value), "sessionId")
					if result.connect == "" {
						err = errors.New("session unconfirmed")
					}
				}
			}
		}
		if err == nil {
			if action.Method == "commands/execute" {
				result.feedback = shortText(stringValue(objectValue(objectValue(value)["result"]), "text"), 65536)
			}
			if strings.HasPrefix(action.Method, "pluginManager/") {
				r := objectValue(value)
				if action.Method == "pluginManager/updateBundle" {
					r = objectValue(r["result"])
				}
				application := stringValue(r, "application")
				switch application {
				case "applied", "restart-required", "overridden", "failed", "cancelled":
					result.feedback = "Применение: " + application + " · причина перезапуска: " + stringValue(r, "restartReason")
				default:
					err = errors.New("unknown application")
				}
				if restore := objectValue(objectValue(value)["restore"]); restore != nil {
					result.feedback += " · откат: " + stringValue(restore, "application")
				}
				if code := stringValue(objectValue(r["error"]), "code"); code != "" {
					result.feedback += " · отказ: " + code
				}
			}
		}
		if err != nil {
			result.err = errors.New("Действие не подтверждено. Автоповтора нет; черновик сохранён. Esc — отменить, F5 — перечитать.")
		}
		return result
	}
}
func (m *model) benchPrepare(key tea.KeyType) tea.Cmd {
	d := m.workbench
	if d == nil || d.busy || d.blocked {
		return nil
	}
	r, ok := d.selected()
	if d.kind == "files" && ok && key == tea.KeyEnter {
		if r.Data["type"] == "directory" {
			d.location = r.ID
			d.search.SetValue("")
			d.index = 0
		} else if r.Data["type"] == "file" {
			d.document = r.ID
			d.fileOffset = 1
		} else {
			return nil
		}
		return m.loadWorkbench()
	}
	if d.kind == "jobs" && ok && key == tea.KeyEnter {
		d.documentOwner = stringValue(r.Data, "owner")
		if d.documentOwner != "" && d.documentOwner != d.id {
			d.note = "Задание принадлежит другой сессии; F5 перечитать."
			return nil
		}
		d.document = r.ID
		return m.loadWorkbench()
	}
	if d.kind == "agents" && ok && key == tea.KeyEnter {
		if r.Data["mode"] == "external" || r.Data["mode"] == "unknown" {
			d.note = "У этого агента нет доступной локальной сессии."
			return nil
		}
		m.closeWorkbench()
		return m.sessionOperation("connect", r.ID)
	}
	if d.kind == "terminals" && ok && key == tea.KeyEnter {
		d.note = "Ctrl+] — вернуться в TUI; ввод не повторяется при разрыве."
		return startBenchTerminal(d.ctx, m.settingsLogin, d.id, r.ID)
	}
	if !ok && !((d.kind == "schedules" || d.kind == "terminals") && key == tea.KeyCtrlN) && !(d.kind == "workspaces" && key == tea.KeyF3) {
		return nil
	}
	var action *benchAction
	switch d.kind {
	case "workspaces":
		if key == tea.KeyCtrlN || key == tea.KeyEnter {
			action = &benchAction{"session/create", map[string]any{"request": map[string]any{"workspaceId": r.ID}}, "Создать сессию в рабочей области " + r.Title}
		}
		if key == tea.KeyF3 {
			action = &benchAction{"session/create", map[string]any{}, "Создать новую сессию в указанной папке"}
			d.startEditor(prettyValue(map[string]any{"cwd": m.workspace}), action)
			return d.editor.Focus()
		}
	case "packages":
		if r.Data["readOnlyReason"] != nil {
			d.note = "Этот пакет доступен только для чтения."
			return nil
		}
		if key == tea.KeySpace {
			action = &benchAction{"pluginManager/setBundleEnabled", map[string]any{"name": r.ID, "enabled": r.Data["enabled"] != true}, "Переключить пакет " + r.ID + "; может потребоваться перезапуск"}
		}
		if key == tea.KeyCtrlX && r.Data["removable"] == true {
			action = &benchAction{"pluginManager/removeBundle", map[string]any{"name": r.ID}, "Удалить пакет " + r.ID}
		}
		if key == tea.KeyF3 && r.Data["updatable"] == true {
			action = &benchAction{"pluginManager/updateBundle", map[string]any{"name": r.ID}, "Обновить пакет " + r.ID + "; совместимость и откат проверяет dsh"}
			d.startEditor("", action)
			return d.editor.Focus()
		}
	case "presets":
		if stringValue(r.Data, "broken") != "" {
			d.note = "Пресет недоступен."
			return nil
		}
		if key == tea.KeyCtrlN {
			action = &benchAction{"session/create", map[string]any{"request": map[string]any{"agentPreset": r.ID}}, "Создать новую сессию с пресетом " + r.ID}
		} else if key == tea.KeyEnter {
			action = &benchAction{"agentPresets/select", map[string]any{"agentId": d.id, "agentPreset": r.ID}, "Выбрать пресет " + r.ID + " для текущей сессии"}
		}
	case "commands":
		if key == tea.KeyEnter {
			action = &benchAction{"commands/execute", map[string]any{"agentId": d.id, "line": "/" + r.ID, "submittedAttachments": []any{}}, "Выполнить команду /" + r.ID}
			if r.Data["input"] != nil {
				d.startEditor("/"+r.ID+" ", action)
				return d.editor.Focus()
			}
		}
	case "schedules":
		if key == tea.KeyCtrlN {
			action = &benchAction{"schedule/create", map[string]any{}, "Создать расписание: будущие срабатывания могут запускать агента"}
			d.startEditor(prettyValue(map[string]any{"title": "Новая задача", "prompt": "Описание задачи", "timing": map[string]any{"kind": "after", "after_seconds": 3600}}), action)
			return d.editor.Focus()
		}
		request := map[string]any{"id": r.ID, "action": "update", "expectedUpdatedAt": r.Data["updatedAt"]}
		if r.Data["updatedAt"] == nil {
			d.note = "У расписания нет ревизии updatedAt; изменение недоступно."
			return nil
		}
		switch key {
		case tea.KeyEnter:
			d.document = r.ID
			return m.loadWorkbench()
		case tea.KeyCtrlG:
			action = &benchAction{"schedule/run", map[string]any{"request": map[string]any{"id": r.ID}}, "Запустить расписание сейчас: агент получит задание " + r.Title}
		case tea.KeyF3:
			action = &benchAction{"schedule/edit", map[string]any{"request": request}, "Изменить расписание " + r.Title}
			d.startEditor(prettyValue(map[string]any{"title": r.Data["title"], "prompt": r.Data["prompt"]}), action)
			return d.editor.Focus()
		case tea.KeyCtrlX:
			request["action"] = "delete"
			action = &benchAction{"schedule/edit", map[string]any{"request": request}, "Удалить расписание и его историю: " + r.Title}
		case tea.KeySpace:
			request["patch"] = map[string]any{"active": r.Data["status"] != "active"}
			action = &benchAction{"schedule/edit", map[string]any{"request": request}, "Переключить паузу расписания " + r.Title}
		}
	case "jobs":
		if key == tea.KeyCtrlX {
			action = &benchAction{"job/kill", map[string]any{"request": map[string]any{"sessionId": d.id, "jobId": r.ID}}, "Запросить остановку задания " + r.Title}
			if stringValue(r.Data, "owner") == "" {
				action.Label = "Остановить общее задание Host: " + r.Title
			}
		}
	case "agents":
		if key == tea.KeyF3 && r.Data["mode"] == "continuable" {
			requestID, e := newRequestID()
			if e != nil {
				return nil
			}
			action = &benchAction{"subagents/prompt", map[string]any{"request": map[string]any{"requestId": requestID, "parentSessionId": d.id, "childSessionId": r.ID, "mode": "continuable", "delivery": "queue"}}, "Отправить сообщение в очередь подагента " + r.Title}
			d.startEditor("", action)
			return d.editor.Focus()
		}
		if key == tea.KeyCtrlX && r.Data["mode"] == "continuable" {
			action = &benchAction{"subagents/interruptByParent", map[string]any{"childSessionId": r.ID, "parentSessionId": d.id, "mode": "continuable"}, "Запросить остановку подагента " + r.Title}
		}
	case "terminals":
		if key == tea.KeyCtrlN {
			id, err := newRequestID()
			if err != nil {
				return nil
			}
			action = &benchAction{"terminal/create", map[string]any{"agentId": d.id, "request": map[string]any{"id": "tui-" + id, "cols": m.width, "rows": m.height}}, "Запустить пользовательскую оболочку в рабочей папке сессии"}
		}
		if key == tea.KeyCtrlX {
			action = &benchAction{"terminal/close", map[string]any{"agentId": d.id, "id": r.ID}, "Закрыть терминал и его процесс " + r.Title}
		}
	}
	if action != nil {
		if m.isSessionArchived(d.id) {
			d.note = "Сессия в архиве; действия недоступны."
			return nil
		}
		d.action = action
		d.confirm = true
		d.note = action.Label + ". Enter подтвердить · Esc отменить."
	}
	return nil
}
func (d *workbench) startEditor(text string, action *benchAction) {
	d.editor = textarea.New()
	d.editor.CharLimit = 65536
	d.editor.ShowLineNumbers = false
	d.editor.Prompt = "> "
	d.editor.SetValue(text)
	d.action = action
	d.editing = true
	d.note = "F5 — проверить и перейти к подтверждению · Esc отменить"
}
func (m *model) benchKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	d := m.workbench
	if d == nil {
		return nil, false
	}
	if !msg.Paste {
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlD, tea.KeyF7, tea.KeyF8:
			return nil, false
		}
	}
	if msg.Paste && msg.Type != tea.KeyRunes {
		return nil, true
	}
	if d.busy {
		if msg.Type == tea.KeyEsc && !d.writing {
			return m.closeWorkbench(), true
		}
		return nil, true
	}
	if d.confirm {
		if msg.Type == tea.KeyEsc {
			d.confirm = false
			d.note = "Подтверждение отменено."
			if !d.editing {
				d.action = nil
			}
			return nil, true
		}
		if msg.Type == tea.KeyEnter && !msg.Paste {
			return m.benchExecute(), true
		}
		return nil, true
	}
	if d.editing {
		if msg.Type == tea.KeyEsc && !msg.Paste {
			d.editing = false
			d.action = nil
			d.editor.SetValue("")
			return nil, true
		}
		if msg.Type == tea.KeyF5 && !msg.Paste {
			text := d.editor.Value()
			if len(text) > 65536 || strings.TrimSpace(text) == "" {
				d.note = "Введите значение до 64 КиБ."
				return nil, true
			}
			if d.kind == "commands" {
				if strings.ContainsAny(text, "\r\n") {
					d.note = "Команда должна занимать одну строку."
					return nil, true
				}
				prefix := "/"
				if r, ok := d.selected(); ok {
					prefix += r.ID
				}
				if text != prefix && !strings.HasPrefix(text, prefix+" ") {
					d.note = "Нельзя подменить выбранную команду."
					return nil, true
				}
				d.action.Args["line"] = text
			} else if d.kind == "agents" {
				objectValue(d.action.Args["request"])["content"] = []any{map[string]any{"type": "text", "text": text}}
			} else if d.kind == "packages" {
				version := strings.TrimSpace(text)
				if strings.ContainsAny(version, " \r\n\t") || len(version) > 128 {
					d.note = "Введите точную версию пакета."
					return nil, true
				}
				d.action.Args["version"] = version
			} else {
				var data map[string]any
				if json.Unmarshal([]byte(text), &data) != nil || len(data) == 0 {
					d.note = "Введите объект JSON."
					return nil, true
				}
				if d.action.Method == "schedule/create" || d.action.Method == "session/create" {
					d.action.Args["request"] = data
				} else {
					objectValue(d.action.Args["request"])["patch"] = data
				}
			}
			d.confirm = true
			d.note = d.action.Label + ". Enter подтвердить · Esc назад."
			return nil, true
		}
		if msg.Type == tea.KeyRunes && (len(d.editor.Value())+len(string(msg.Runes)) > 65536 || strings.ContainsAny(string(msg.Runes), "\r\t")) {
			d.note = "Вставка не принята: лимит 64 КиБ, без табуляции/CR."
			return nil, true
		}
		var cmd tea.Cmd
		d.editor, cmd = d.editor.Update(msg)
		return cmd, true
	}
	if !msg.Paste {
		switch msg.Type {
		case tea.KeyEsc, tea.KeyLeft:
			if d.kind == "files" || d.kind == "jobs" || d.kind == "schedules" {
				if d.document != "" {
					d.document = ""
					d.fileOffset = 1
					return m.loadWorkbench(), true
				}
				if d.location != "" {
					d.location = path.Dir(d.location)
					if d.location == "." {
						d.location = ""
					}
					d.index = 0
					return m.loadWorkbench(), true
				}
			}
			return m.closeWorkbench(), true
		case tea.KeyF5:
			return m.loadWorkbench(), true
		case tea.KeyF4:
			if d.kind == "plugins" {
				m.closeWorkbench()
				m.page = dshSettingsPage
				m.navFocused = false
				m.rebuild()
				return nil, true
			}
		case tea.KeyUp, tea.KeyDown:
			if d.document != "" {
				var cmd tea.Cmd
				d.view, cmd = d.view.Update(msg)
				return cmd, true
			}
			delta := 1
			if msg.Type == tea.KeyUp {
				delta = -1
			}
			if len(d.visible()) > 0 {
				d.index = (d.index + delta + len(d.visible())) % len(d.visible())
				d.view.GotoTop()
			}
			return nil, true
		case tea.KeyPgDown, tea.KeyPgUp:
			if d.kind == "files" && d.document != "" {
				if msg.Type == tea.KeyPgDown && !d.fileEOF && d.view.AtBottom() {
					d.fileOffset += 200
					return m.loadWorkbench(), true
				}
				if msg.Type == tea.KeyPgUp && d.fileOffset > 1 && d.view.AtTop() {
					d.fileOffset = max(1, d.fileOffset-200)
					return m.loadWorkbench(), true
				}
			}
			var cmd tea.Cmd
			d.view, cmd = d.view.Update(msg)
			return cmd, true
		case tea.KeySpace:
			if d.kind == "packages" || d.kind == "schedules" {
				return m.benchPrepare(msg.Type), true
			}
			msg.Type, msg.Runes = tea.KeyRunes, []rune{' '}
		case tea.KeyEnter, tea.KeyCtrlN, tea.KeyCtrlX, tea.KeyF3, tea.KeyCtrlG:
			return m.benchPrepare(msg.Type), true
		}
	}
	if d.document != "" {
		return nil, true
	}
	var cmd tea.Cmd
	d.search, cmd = d.search.Update(msg)
	d.index = 0
	return cmd, true
}
func (m *model) benchBox() area {
	w, h := min(110, m.width-3), min(36, m.height-2)
	return area{(m.width - 1 - w) / 2, (m.height - h) / 2, w, h}
}
func (m *model) benchView(base string) string {
	d := m.workbench
	if d == nil {
		return base
	}
	box := m.benchBox()
	w := box.width - 4
	h := box.height - 10
	title := map[string]string{"workspaces": "Рабочие области", "packages": "Управление пакетами dsh", "presets": "Пресеты агента", "files": "Файлы рабочей области", "commands": "Команды плагинов", "plugins": "Плагины dsh", "jobs": "Фоновые задания", "schedules": "Расписания · весь Host", "agents": "Подагенты сессии", "tasks": "Задачи команды", "terminals": "Терминалы сессии"}[d.kind]
	trim := func(s string) string { return ansi.Truncate(clean(s), w, "…") }
	d.search.Width = max(1, w-8)
	lines := []string{heading.Render(title), trim(d.id + " / " + d.location), d.search.View()}
	if d.editing {
		d.editor.SetWidth(w)
		d.editor.SetHeight(max(1, h))
		lines = append(lines, strings.Split(d.editor.View(), "\n")...)
	} else if (d.kind == "files" || d.kind == "jobs" || d.kind == "schedules") && d.document != "" {
		d.view.Width = w
		d.view.Height = h
		d.view.SetContent(ansi.Hardwrap(clean(d.fileBody), w, true))
		lines = append(lines, strings.Split(d.view.View(), "\n")...)
	} else {
		rows := d.visible()
		listHeight := max(2, h/2)
		start := max(0, min(d.index-listHeight/2, len(rows)-listHeight))
		for i := start; i < min(len(rows), start+listHeight); i++ {
			prefix := "  "
			if i == d.index {
				prefix = "› "
			}
			lines = append(lines, trim(prefix+rows[i].Title))
		}
		for len(lines) < 3+listHeight {
			lines = append(lines, "")
		}
		body := "Нет записей."
		if r, ok := d.selected(); ok {
			body = r.Body
			if d.lastResult != "" {
				body += "\n\nПоследний результат:\n" + d.lastResult
			}
		}
		d.view.Width = w
		d.view.Height = max(1, h-listHeight)
		d.view.SetContent(ansi.Hardwrap(body, w, true))
		lines = append(lines, strings.Split(d.view.View(), "\n")...)
	}
	note := d.note
	if d.busy {
		note = "Запрос dsh…"
	}
	notes := strings.Split(ansi.Hardwrap(clean(note), w, true), "\n")
	for len(notes) < 3 {
		notes = append(notes, "")
	}
	lines = append(lines, muted.Render(notes[0]), muted.Render(notes[1]), muted.Render(notes[2]))
	hint := "↑/↓ выбор · Enter открыть · F5 обновить · Esc назад"
	hint2 := "Снимок · PgUp/PgDn детали · поиск по имени"
	switch d.kind {
	case "workspaces":
		hint2 = "Ctrl+N новая сессия здесь · F3 другая папка"
	case "packages":
		hint2 = "Space включить · F3 точная версия/обновить · Ctrl+X удалить"
	case "presets":
		hint2 = "Enter выбрать · Ctrl+N новая сессия · после первого хода пресет закреплён"
	case "schedules":
		hint2 = "Enter история · Ctrl+N создать · F3 правка · Space пауза · Ctrl+G запуск · Ctrl+X удалить"
	case "terminals":
		hint2 = "Enter терминал · Ctrl+N оболочка · Ctrl+X закрыть · Ctrl+] вернуться"
	case "jobs":
		hint2 = "Enter снимок вывода · Ctrl+X остановить · F5 перечитать"
	case "agents":
		hint2 = "Enter сессия · F3 сообщение в очередь · Ctrl+X остановить"
	case "plugins":
		hint2 = "F4 настройки · /commands команды · /market пакеты"
	case "files":
		hint2 = "PgUp/PgDn страницы файла · Left назад · снимок без записи"
	}
	if d.editing {
		hint = "Enter новая строка · F5 проверить · Esc отменить"
	}
	if d.confirm {
		hint = "Enter — подтвердить действие · Esc — отменить"
		hint2 = "Запрос не повторяется автоматически."
	}
	lines = append(lines, trim(hint), trim(hint2))
	return overlay(base, panel(box.width, box.height, strings.Join(lines, "\n"), "#58a6ff"), box.x, box.y, m.width-1)
}

func (m *model) benchMouse(msg tea.MouseMsg) tea.Cmd {
	d := m.workbench
	if d == nil || d.busy || d.confirm || d.editing {
		return nil
	}
	box := m.benchBox()
	if !box.contains(msg.X, msg.Y) {
		return nil
	}
	listHeight := max(2, (box.height-10)/2)
	listTop := box.y + 4
	if d.document == "" && msg.Y >= listTop && msg.Y < listTop+listHeight {
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			key := tea.KeyDown
			if msg.Button == tea.MouseButtonWheelUp {
				key = tea.KeyUp
			}
			cmd, _ := m.benchKey(tea.KeyMsg{Type: key})
			return cmd
		}
		if msg.Button == tea.MouseButtonLeft {
			rows := d.visible()
			start := max(0, min(d.index-listHeight/2, len(rows)-listHeight))
			index := start + msg.Y - listTop
			if index < len(rows) {
				d.index = index
				d.view.GotoTop()
			}
		}
		return nil
	}
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		var cmd tea.Cmd
		d.view, cmd = d.view.Update(msg)
		return cmd
	}
	return nil
}
