package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const settingsDraftLimit = 64 << 10

var credentialRefPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type settingsField struct {
	row             formRow
	editor          textinput.Model
	multiline       *textarea.Model
	add, credential bool
	ref             string
	info            credentialInfo
}
type credentialInfo struct {
	Configured bool   `json:"configured"`
	Writable   bool   `json:"writable"`
	Source     string `json:"source"`
}
type settingsForm struct {
	conflicts                                map[string]bool
	ctx                                      context.Context
	cancel                                   context.CancelFunc
	namespace                                settingsNamespace
	schema                                   *formSchema
	value                                    any
	path                                     []string
	index, serial                            int
	ops                                      []settingsPathOp
	field                                    *settingsField
	focused, busy, writing, blocked, discard bool
	note                                     string
}
type settingsFormResult struct {
	dialog          *settingsForm
	serial          int
	namespace       *settingsNamespace
	snapshot        *settingsSnapshot
	credential      *credentialInfo
	err             error
	refresh         bool
	credentialSaved bool
}

func (m *model) openSettingsForm(providers bool) tea.Cmd {
	if m.browserPreview || m.settings == nil || !m.settings.Writable || m.settingsLoading {
		m.append("Настройки недоступны для записи. Обновите подключение и снимок.")
		return nil
	}
	index := m.settingsIndex
	if providers {
		index = -1
		for i, ns := range m.settings.Namespaces {
			var schema formSchema
			if json.Unmarshal(ns.Schema, &schema) == nil {
				if _, ok := schema.fields(schema.node(schema.UID), 0)["providers"]; ok {
					index = i
					break
				}
			}
		}
	}
	if index < 0 || index >= len(m.settings.Namespaces) {
		m.append("В этом профиле нет формы провайдеров.")
		return nil
	}
	namespace := m.settings.Namespaces[index]
	schema, err := decodeFormSchema(namespace.Schema)
	if err != nil {
		m.append(err.Error())
		return nil
	}
	base := m.settingsContext
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	d := &settingsForm{ctx: ctx, cancel: cancel, namespace: namespace, schema: schema, focused: m.input.Focused()}
	d.resetValue()
	if providers {
		d.path = []string{"providers"}
	}
	m.settingsForm = d
	m.input.Blur()
	return nil
}
func (d *settingsForm) resetValue() {
	d.value = detached(d.namespace.Value)
	for _, secret := range d.namespace.Secrets {
		d.value = editValue(d.value, secret.Path, nil, true)
	}
	d.ops = nil
	d.conflicts = nil
	d.blocked = false
	d.field = nil
	d.discard = false
}
func (m *model) closeSettingsForm() tea.Cmd {
	d := m.settingsForm
	if d == nil {
		return nil
	}
	d.cancel()
	d.field = nil
	d.ops = nil
	d.value = nil
	m.settingsForm = nil
	if d.focused && m.page == chatPage && !m.navFocused {
		return m.input.Focus()
	}
	return nil
}
func (d *settingsForm) rows() []formRow { return d.schema.rows(d.value, d.path) }
func (d *settingsForm) selection() (formRow, bool) {
	rows := d.rows()
	if d.index < 0 || d.index >= len(rows) {
		return formRow{}, false
	}
	return rows[d.index], true
}
func (d *settingsForm) secretUnder(path []string) bool {
	for _, secret := range d.namespace.Secrets {
		if pathUnder(secret.Path, path) || pathUnder(path, secret.Path) {
			return true
		}
	}
	for i := 0; i < len(path); i++ {
		ancestor, _ := d.schema.at(path[:i])
		if nodeRole(ancestor) == "secret" {
			return true
		}
	}
	node, ok := d.schema.at(path)
	return ok && d.schema.containsSecret(node, 0)
}
func (s *formSchema) containsSecret(node formNode, depth int) bool {
	if depth > 32 {
		return true
	}
	if nodeRole(node) == "secret" {
		return true
	}
	for _, id := range node.Dict {
		if s.containsSecret(s.node(id), depth+1) {
			return true
		}
	}
	for _, id := range node.List {
		if s.containsSecret(s.node(id), depth+1) {
			return true
		}
	}
	if node.Type == "dict" || node.Type == "array" {
		return s.containsSecret(s.node(node.Inner), depth+1)
	}
	return false
}
func (d *settingsForm) stage(path []string, value any, unset bool) bool {
	if len(path) == 0 || len(path) > 32 {
		d.note = "Нельзя заменить весь раздел."
		return false
	}
	original := detached(d.value)
	originalOps := d.ops
	for i := 1; i < len(path); i++ {
		node, _ := d.schema.at(path[:i])
		if node.Type == "array" {
			if _, exists := valueAt(d.value, path[:i]); !exists {
				d.value = editValue(d.value, path[:i], []any{}, false)
			}
		}
	}
	if unset {
		if d.secretUnder(path) {
			d.value = original
			d.note = "Сброс контейнера с секретами недоступен."
			return false
		}
		inherited, exists := valueAt(d.namespace.Base, path)
		d.value = editValue(d.value, path, detached(inherited), !exists)
	} else {
		preview := value
		node, _ := d.schema.at(path)
		if nodeRole(node) == "secret" {
			preview = "[новый секрет]"
		}
		d.value = editValue(d.value, path, preview, false)
	}
	// An inherited array must be materialized as one array value in the user layer.
	op := settingsPathOp{Op: "set", Path: append([]string{}, path...), Value: detached(value)}
	if unset {
		op.Op = "unset"
	}
	for i := 1; i <= len(path); i++ {
		node, _ := d.schema.at(path[:i])
		if node.Type == "array" {
			if unset && i == len(path) {
				break
			}
			if d.secretUnder(path[:i]) {
				d.value = original
				d.note = "Редактирование массива с секретами недоступно."
				return false
			}
			op.Path = append([]string{}, path[:i]...)
			op.Op = "set"
			op.Value, _ = valueAt(d.value, op.Path)
			op.Value = detached(op.Value)
			break
		}
	}
	kept := make([]settingsPathOp, 0, len(d.ops)+1)
	for _, old := range d.ops {
		if !pathUnder(old.Path, op.Path) {
			kept = append(kept, old)
		}
	}
	d.ops = append(kept, op)
	raw, _ := json.Marshal(d.ops)
	if len(d.ops) > 256 || len(raw) > settingsDraftLimit {
		d.value = original
		d.ops = originalOps
		d.note = "Лимит изменений: 256 полей / 64 КиБ."
		return false
	}
	for key := range d.conflicts {
		var conflict []string
		_ = json.Unmarshal([]byte(key), &conflict)
		if pathUnder(conflict, op.Path) {
			delete(d.conflicts, key)
		}
	}
	d.note = fmt.Sprintf("Изменений: %d · F5 — сохранить · Esc — назад.", len(d.ops))
	return true
}
func (op settingsPathOp) MarshalJSON() ([]byte, error) {
	value := map[string]any{"op": op.Op, "path": op.Path}
	if op.Op == "set" {
		value["value"] = op.Value
	}
	return json.Marshal(value)
}
func (d *settingsForm) edit(row formRow, add bool) tea.Cmd {
	if !row.known && !add {
		d.note = "Поле отсутствует в поддерживаемой схеме; доступно только чтение."
		return nil
	}
	if !add && formContainer(row.node) {
		if len(d.path) >= 32 {
			d.note = "Достигнут предел глубины формы."
			return nil
		}
		d.path = row.path
		d.index = 0
		return nil
	}
	if !add && d.secretUnder(row.path) && nodeRole(row.node) != "secret" {
		d.note = "Поле с вложенными секретами недоступно для изменения целиком."
		return nil
	}
	if !add && row.node.Type == "string" {
		if value, ok := row.value.(string); ok && strings.ContainsAny(value, "\n\r") {
			return d.editMultiline(row)
		}
	}
	editor := textinput.New()
	editor.CharLimit = settingsDraftLimit
	editor.Prompt = "Значение: "
	if add {
		editor.Prompt = "Имя: "
	} else if nodeRole(row.node) == "secret" {
		editor.EchoMode = textinput.EchoPassword
		editor.EchoCharacter = '•'
	} else if row.node.Type == "string" {
		value, _ := row.value.(string)
		editor.SetValue(value)
		if editor.Value() != value {
			d.note = "Значение нельзя открыть без преобразования (многострочное или слишком большое)."
			return nil
		}
	} else if row.value != nil {
		raw, _ := json.Marshal(row.value)
		editor.SetValue(string(raw))
	}
	d.field = &settingsField{row: row, editor: editor, add: add}
	return d.field.editor.Focus()
}
func (m *model) formEnter() tea.Cmd {
	d := m.settingsForm
	if d.blocked {
		d.note = "Сначала перечитайте раздел через Ctrl+R."
		return nil
	}
	if row, ok := d.selection(); ok {
		return d.edit(row, false)
	}
	node, _ := d.schema.at(d.path)
	switch node.Type {
	case "dict":
		return d.edit(formRow{}, true)
	case "array":
		value, _ := valueAt(d.value, d.path)
		array, _ := value.([]any)
		path := append(append([]string{}, d.path...), strconv.Itoa(len(array)))
		if d.stage(path, formDefault(d.schema.node(node.Inner)), false) {
			return d.edit(formRow{path: path, node: d.schema.node(node.Inner), known: true}, false)
		}
	}
	return nil
}
func (m *model) acceptSettingsField() tea.Cmd {
	d := m.settingsForm
	field := d.field
	if field == nil {
		return nil
	}
	if field.credential {
		return m.writeCredential()
	}
	value := field.editor.Value()
	if field.multiline != nil {
		value = field.multiline.Value()
	}
	if field.add {
		if value == "" || len(value) > 256 || clean(value) != value {
			d.note = "Введите непустое имя до 256 символов."
			return nil
		}
		path := append(append([]string{}, d.path...), value)
		if _, exists := valueAt(d.value, path); exists {
			d.note = "Такое имя уже существует."
			return nil
		}
		node, _ := d.schema.at(d.path)
		child := d.schema.node(node.Inner)
		if d.stage(path, formDefault(child), false) {
			d.field = nil
			return d.edit(formRow{path: path, node: child, known: true}, false)
		}
		return nil
	}
	var parsed any = value
	if len(value) > settingsDraftLimit {
		d.note = "Значение превышает 64 КиБ."
		return nil
	}
	if field.row.node.Type != "string" {
		if len(value) > settingsDraftLimit || json.Unmarshal([]byte(value), &parsed) != nil {
			d.note = "Введите корректное значение JSON для этого типа."
			return nil
		}
		switch field.row.node.Type {
		case "number":
			if _, ok := parsed.(float64); !ok {
				d.note = "Ожидается число."
				return nil
			}
		case "boolean":
			if _, ok := parsed.(bool); !ok {
				d.note = "Ожидается true или false."
				return nil
			}
		}
	}
	if nodeRole(field.row.node) == "credential-ref" && value != "" && !credentialRefPattern.MatchString(value) {
		d.note = "Имя ключа: буквы, цифры и _, первый символ — буква или _."
		return nil
	}
	if choices := d.schema.enums(field.row.node); len(choices) > 0 {
		found := false
		for _, choice := range choices {
			a, _ := json.Marshal(choice)
			b, _ := json.Marshal(parsed)
			if string(a) == string(b) {
				found = true
				break
			}
		}
		if !found {
			d.note = "Выберите одно из значений схемы."
			return nil
		}
	}
	if err := d.schema.validate(field.row.node, parsed); err != nil {
		d.note = err.Error()
		return nil
	}
	if d.stage(field.row.path, parsed, false) {
		field.editor.SetValue("")
		d.field = nil
	}
	return nil
}
func (m *model) saveSettingsForm() tea.Cmd {
	d := m.settingsForm
	if d == nil || d.busy || d.blocked || d.field != nil || len(d.ops) == 0 || len(d.conflicts) > 0 {
		return nil
	}
	for _, op := range d.ops {
		if op.Op == "unset" {
			continue
		}
		node, known := d.schema.at(op.Path)
		value, _ := valueAt(d.value, op.Path)
		if known && !d.secretUnder(op.Path) {
			if err := d.schema.validate(node, value); err != nil {
				d.note = clean(strings.Join(op.Path, "/")) + ": " + err.Error()
				return nil
			}
		}
	}
	raw, err := json.Marshal(d.ops)
	if err != nil || len(raw) > settingsDraftLimit {
		d.note = "Изменения превышают лимит."
		return nil
	}
	var ops []settingsPathOp
	_ = json.Unmarshal(raw, &ops)
	d.busy, d.writing = true, true
	d.serial++
	d.note = "Сохранение в dsh…"
	serial, revision, namespace, login := d.serial, d.namespace.Revision, d.namespace.NS, m.settingsLogin
	return func() tea.Msg {
		result := settingsFormResult{dialog: d, serial: serial}
		ctx, cancel := context.WithTimeout(d.ctx, 20*time.Second)
		defer cancel()
		api, err := openAPI(ctx, login)
		if err == nil {
			defer api.close()
			var saved settingsNamespace
			err = api.rpc(ctx, "settings/mutate", map[string]any{"ns": namespace, "ops": ops, "expectedRevision": revision}, &saved)
			if err == nil && (saved.NS != namespace || saved.Revision <= revision) {
				err = errors.New("unconfirmed settings write")
			}
			if err == nil {
				result.namespace = &saved
			}
		}
		if err != nil {
			var rejected *rpcRejected
			if errors.As(err, &rejected) && rejected.code == "settings/conflict" {
				result.err = errors.New("Раздел изменён другим клиентом. Ctrl+R — перечитать; изменения сохранены в форме.")
			} else if errors.As(err, &rejected) && rejected.code == "settings/rejected" {
				result.err = errors.New("dsh отклонил настройки. Проверьте значения; Ctrl+R — перечитать.")
			} else {
				result.err = errors.New("Сохранение не подтверждено. Ctrl+R — перечитать; повтора нет.")
			}
		}
		return result
	}
}
func (m *model) refreshSettingsForm() tea.Cmd {
	d := m.settingsForm
	if d == nil || d.busy {
		return nil
	}
	d.busy = true
	d.serial++
	serial, login, namespace := d.serial, m.settingsLogin, d.namespace.NS
	return func() tea.Msg {
		result := settingsFormResult{dialog: d, serial: serial, refresh: true}
		ctx, cancel := context.WithTimeout(d.ctx, 20*time.Second)
		defer cancel()
		snapshot, err := readSettings(ctx, login)
		if err == nil {
			for _, ns := range snapshot.Namespaces {
				if ns.NS == namespace {
					copy := ns
					result.namespace = &copy
					break
				}
			}
			if result.namespace == nil {
				err = errors.New("namespace missing")
			}
			if !snapshot.Writable {
				err = errors.New("settings readonly")
			}
			result.snapshot = snapshot
		}
		if err != nil {
			result.err = errors.New("Раздел недоступен для записи. Предыдущий снимок сохранён.")
		}
		return result
	}
}
func (m *model) handleSettingsForm(result settingsFormResult) {
	d := m.settingsForm
	if d == nil || d != result.dialog || d.ctx.Err() != nil || d.serial != result.serial {
		return
	}
	writing := d.writing
	d.busy, d.writing = false, false
	if result.err != nil {
		d.note = result.err.Error()
		if writing || result.refresh {
			d.blocked = true
			d.clearSecrets()
		}
		return
	}
	if result.credential != nil {
		if d.field != nil {
			d.field.info = *result.credential
		}
		d.note = "Существующий ключ не читается. Enter — сохранить новое значение."
		return
	}
	if result.credentialSaved {
		d.field = nil
		d.note = "Новый ключ записан; доступ к провайдеру проверяется при следующем запросе."
		return
	}
	if result.namespace != nil {
		schema, err := decodeFormSchema(result.namespace.Schema)
		if err != nil {
			d.note = err.Error()
			d.blocked = true
			return
		}
		oldOps := d.ops
		oldNamespace := d.namespace
		oldPath := append([]string{}, d.path...)
		d.namespace = *result.namespace
		d.schema = schema
		d.resetValue()
		d.path = oldPath
		if result.refresh {
			for _, op := range oldOps {
				d.stage(op.Path, op.Value, op.Op == "unset")
				previous, hadPrevious := valueAt(oldNamespace.Value, op.Path)
				fresh, hasFresh := valueAt(d.namespace.Value, op.Path)
				a, _ := json.Marshal(previous)
				b, _ := json.Marshal(fresh)
				if hadPrevious != hasFresh || string(a) != string(b) {
					if d.conflicts == nil {
						d.conflicts = map[string]bool{}
					}
					d.conflicts[pathKey(op.Path)] = true
				}
			}
			d.note = "Перечитана свежая ревизия; изменения оставлены для проверки. F5 — сохранить."
			if len(d.conflicts) > 0 {
				d.note = "Конфликт полей: откройте и проверьте правку либо Ctrl+X отмените её. F5 заблокирован."
			}
		} else {
			d.note = "Настройки сохранены. Условия применения не указаны dsh."
			if d.namespace.Applies == "live" {
				d.note = "Настройки сохранены. Применяются dsh без перезапуска."
			} else if d.namespace.Applies != "" {
				d.note = "Настройки сохранены. Применение: " + clean(d.namespace.Applies)
			}
		}
		if m.settings != nil {
			for i, ns := range m.settings.Namespaces {
				if ns.NS == d.namespace.NS {
					m.settings.Namespaces[i] = d.namespace
					break
				}
			}
		}
		if result.snapshot != nil {
			m.settings = result.snapshot
		}
	}
}
func (d *settingsForm) clearSecrets() {
	if d.field != nil {
		d.field.editor.SetValue("")
	}
	var kept []settingsPathOp
	for _, op := range d.ops {
		node, _ := d.schema.at(op.Path)
		if nodeRole(node) != "secret" {
			kept = append(kept, op)
		} else {
			d.value = editValue(d.value, op.Path, nil, true)
		}
	}
	d.ops = kept
}

func (d *settingsForm) dropDraft(path []string) {
	var kept []settingsPathOp
	reset := path
	for _, op := range d.ops {
		if pathUnder(op.Path, path) || pathUnder(path, op.Path) {
			delete(d.conflicts, pathKey(op.Path))
			if len(op.Path) < len(reset) {
				reset = op.Path
			}
		} else {
			kept = append(kept, op)
		}
	}
	fresh, exists := valueAt(d.namespace.Value, reset)
	d.value = editValue(d.value, reset, detached(fresh), !exists)
	d.ops = kept
	d.note = "Правка отменена; показано перечитанное значение этого поля."
}
