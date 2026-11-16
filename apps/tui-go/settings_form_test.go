package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const formFixture = `{"uid":0,"refs":{"0":{"type":"object","dict":{"providers":1,"hidden":9}},"1":{"type":"dict","inner":2},"2":{"type":"object","dict":{"baseURL":3,"apiKeyEnv":4,"models":5,"enabled":8}},"3":{"type":"string"},"4":{"type":"string","meta":{"role":"credential-ref"}},"5":{"type":"array","inner":6,"meta":{"default":[]}},"6":{"type":"object","dict":{"id":3}},"8":{"type":"boolean"},"9":{"type":"string","meta":{"role":"secret"}}}}`

func formTestModel(t *testing.T) *model {
	t.Helper()
	m := chatModel(t)
	var ns settingsNamespace
	_ = json.Unmarshal([]byte(`{"ns":"llm-pi-ai","revision":3,"value":{"providers":{"local":{"baseURL":"http://old","apiKeyEnv":"TEST_KEY","models":[{"id":"base"},{"id":"other"}],"enabled":false}},"hidden":"NEVER DISPLAY"},"base":{"providers":{"local":{"baseURL":"http://base"}}},"secrets":[{"path":["hidden"],"set":true}]}`), &ns)
	ns.Schema = json.RawMessage(formFixture)
	m.settings = &settingsSnapshot{Writable: true, Namespaces: []settingsNamespace{ns}}
	m.settingsIndex = 0
	m.input.SetValue("retained draft")
	m.openSettingsForm(true)
	if m.settingsForm == nil {
		t.Fatal("settings form did not open")
	}
	return m
}
func selectFormPath(t *testing.T, m *model, path []string) formRow {
	t.Helper()
	d := m.settingsForm
	d.path = append([]string{}, path[:len(path)-1]...)
	for i, row := range d.rows() {
		if row.key == path[len(path)-1] {
			d.index = i
			return row
		}
	}
	t.Fatal("field missing", path)
	return formRow{}
}
func TestSettingsFormStagesExactPathsAndPreservesSecretsAndArrays(t *testing.T) {
	m := formTestModel(t)
	d := m.settingsForm
	if strings.Contains(m.View(), "NEVER DISPLAY") {
		t.Fatal("secret reached view")
	}
	if !d.stage([]string{"providers", "local", "baseURL"}, "http://new", false) {
		t.Fatal(d.note)
	}
	if !d.stage([]string{"providers", "local", "models", "1", "id"}, "changed", false) {
		t.Fatal(d.note)
	}
	if len(d.ops) != 2 || pathKey(d.ops[1].Path) != `["providers","local","models"]` {
		t.Fatal("array edit did not materialize the inherited array")
	}
	array := d.ops[1].Value.([]any)
	if array[0].(map[string]any)["id"] != "base" || array[1].(map[string]any)["id"] != "changed" {
		t.Fatal("array edit discarded unrelated members")
	}
	raw, _ := json.Marshal(d.ops)
	if strings.Contains(string(raw), "hidden") || strings.Contains(string(raw), "NEVER DISPLAY") {
		t.Fatal("redacted namespace was rewritten wholesale")
	}
	old := string(raw)
	if d.stage([]string{"providers", "local", "baseURL"}, strings.Repeat("x", settingsDraftLimit+1), false) {
		t.Fatal("oversized mutation accepted")
	}
	after, _ := json.Marshal(d.ops)
	if string(after) != old {
		t.Fatal("failed staging discarded earlier changes")
	}
	d.stage([]string{"providers", "local", "enabled"}, false, false)
	raw, _ = json.Marshal(d.ops)
	if !strings.Contains(string(raw), `"value":false`) {
		t.Fatal("false setting value was omitted")
	}
	d.stage([]string{"providers", "local", "baseURL"}, nil, true)
	value, _ := valueAt(d.value, []string{"providers", "local", "baseURL"})
	if value != "http://base" {
		t.Fatal("unset did not show composition fallback")
	}
	d.stage([]string{"providers", "local", "models"}, nil, true)
	if d.ops[len(d.ops)-1].Op != "unset" {
		t.Fatal("resetting the whole array materialized an override instead of unsetting it")
	}
	for _, size := range [][2]int{{120, 42}, {100, 30}, {80, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if lipgloss.Width(m.View()) > m.width-1 || lipgloss.Height(m.View()) > m.height {
			t.Fatal("form overflowed minimum size", size)
		}
	}
}
func TestSettingsConflictRereadPreservesForeignFieldsAndRequiresReview(t *testing.T) {
	m := formTestModel(t)
	d := m.settingsForm
	path := []string{"providers", "local", "baseURL"}
	d.stage(path, "http://draft", false)
	d.writing = true
	d.busy = true
	m.handleSettingsForm(settingsFormResult{dialog: d, serial: d.serial, err: errors.New("conflict")})
	if !d.blocked || m.saveSettingsForm() != nil {
		t.Fatal("conflicting mutation retried")
	}
	fresh := d.namespace
	fresh.Revision++
	fresh.Value = detached(fresh.Value)
	fresh.Value = editValue(fresh.Value, path, "http://other-client", false)
	fresh.Value = editValue(fresh.Value, []string{"providers", "local", "otherSetting"}, "preserve", false)
	m.handleSettingsForm(settingsFormResult{dialog: d, serial: d.serial, namespace: &fresh, refresh: true})
	if d.blocked || len(d.conflicts) != 1 || m.saveSettingsForm() != nil {
		t.Fatal("reread silently resolved a same-field conflict")
	}
	value, _ := valueAt(d.value, path)
	if value != "http://draft" {
		t.Fatal("conflict lost user's draft")
	}
	value, _ = valueAt(d.value, []string{"providers", "local", "otherSetting"})
	if value != "preserve" {
		t.Fatal("reread overwrote another client's field")
	}
	d.stage(path, "http://reviewed", false)
	if len(d.conflicts) != 0 {
		t.Fatal("explicitly reviewed edit remained blocked")
	}
	d.dropDraft(path)
	value, _ = valueAt(d.value, path)
	if len(d.ops) != 0 || value != "http://other-client" {
		t.Fatal("dropping draft did not restore fresh state")
	}
}
func TestSettingsNewProviderCreatesArrayWithoutAliasingParentOperations(t *testing.T) {
	m := formTestModel(t)
	d := m.settingsForm
	provider := []string{"providers", "added"}
	if !d.stage(provider, map[string]any{}, false) || !d.stage(append(provider, "models", "0"), map[string]any{}, false) || !d.stage(append(provider, "models", "0", "id"), "new-model", false) {
		t.Fatal(d.note)
	}
	if len(d.ops) != 2 || len(d.ops[0].Value.(map[string]any)) != 0 {
		t.Fatal("child editing mutated an earlier parent operation")
	}
	value, _ := valueAt(d.value, append(provider, "models"))
	array, ok := value.([]any)
	if !ok || len(array) != 1 || array[0].(map[string]any)["id"] != "new-model" {
		t.Fatal("new model list was not an array")
	}
	d.namespace.Secrets = append(d.namespace.Secrets, settingsSecret{Path: []string{"providers", "local", "models", "0", "private"}, Set: true})
	before, _ := json.Marshal(d.ops)
	if d.stage([]string{"providers", "local", "models", "0", "id"}, "refused", false) {
		t.Fatal("array containing a redacted secret was rewritten")
	}
	after, _ := json.Marshal(d.ops)
	if string(before) != string(after) {
		t.Fatal("refused array edit changed the draft")
	}
}
func TestSettingsWritesUseRevisionAndCredentialIsWriteOnly(t *testing.T) {
	m := formTestModel(t)
	d := m.settingsForm
	var mu sync.Mutex
	writes, keys := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "test", Path: "/"})
			w.WriteHeader(302)
			return
		}
		var req struct {
			ID      string `json:"rpcId"`
			Payload struct {
				Args map[string]json.RawMessage `json:"args"`
			} `json:"payload"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		defer mu.Unlock()
		var value any
		switch r.URL.Path {
		case "/api/settings/mutate":
			writes++
			if string(req.Payload.Args["expectedRevision"]) != "3" || string(req.Payload.Args["ns"]) != `"llm-pi-ai"` {
				t.Error("settings write omitted expected revision or namespace")
			}
			var ops []settingsPathOp
			_ = json.Unmarshal(req.Payload.Args["ops"], &ops)
			if len(ops) != 1 || pathKey(ops[0].Path) != `["providers","local","baseURL"]` {
				t.Error("write touched fields outside the requested path")
			}
			saved := d.namespace
			saved.Revision = 4
			value = saved
		case "/api/credentials/describe":
			value = map[string]any{"TEST_KEY": map[string]any{"configured": true, "source": "file", "writable": true}}
		case "/api/credentials/set":
			keys++
			if string(req.Payload.Args["ref"]) != `"TEST_KEY"` || string(req.Payload.Args["value"]) != `"NEW PRIVATE KEY"` {
				t.Error("credential write changed reference or value")
			}
		default:
			t.Error("unexpected settings endpoint", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": req.ID, "result": map[string]any{"ok": true, "value": value}})
	}))
	defer server.Close()
	m.settingsLogin = server.URL + "/?token=test"
	d.stage([]string{"providers", "local", "baseURL"}, "http://new", false)
	cmd := m.saveSettingsForm()
	if cmd == nil || m.saveSettingsForm() != nil {
		t.Fatal("concurrent settings writes accepted")
	}
	m.handleSettingsForm(cmd().(settingsFormResult))
	if d.namespace.Revision != 4 || len(d.ops) != 0 {
		t.Fatal("confirmed revision did not replace staged state")
	}
	selectFormPath(t, m, []string{"providers", "local", "apiKeyEnv"})
	cmd = m.openCredential()
	if cmd == nil {
		t.Fatal(d.note)
	}
	m.handleSettingsForm(cmd().(settingsFormResult))
	d.field.editor.SetValue("NEW PRIVATE KEY")
	if strings.Contains(m.View(), "NEW PRIVATE KEY") {
		t.Fatal("new credential was displayed unmasked")
	}
	cmd = m.writeCredential()
	if cmd == nil || d.field.editor.Value() != "" {
		t.Fatal("secret input not cleared before sending")
	}
	m.handleSettingsForm(cmd().(settingsFormResult))
	mu.Lock()
	if writes != 1 || keys != 1 {
		t.Error("unexpected settings or key retries")
	}
	mu.Unlock()
	if d.field != nil || m.input.Value() != "retained draft" || strings.Contains(strings.Join(m.history, " "), "PRIVATE") {
		t.Fatal("credential leaked into composer/history or remained in editor")
	}
}
func TestSettingsFormPasteReadonlyAndMultilineValues(t *testing.T) {
	m := formTestModel(t)
	d := m.settingsForm
	row := selectFormPath(t, m, []string{"providers", "local", "baseURL"})
	d.edit(row, false)
	m.Update(tea.KeyMsg{Type: tea.KeyEnter, Paste: true})
	if len(d.ops) != 0 {
		t.Fatal("pasted Enter staged a hidden action")
	}
	row.value = "line one\nline two"
	d.field = nil
	d.edit(row, false)
	if d.field == nil || d.field.multiline == nil || d.field.multiline.Value() != row.value {
		t.Fatal("multiline setting was not preserved exactly")
	}
	m.closeSettingsForm()
	m.settings.Writable = false
	m.openSettingsForm(true)
	if m.settingsForm != nil {
		t.Fatal("readonly settings opened for writing")
	}
}
func TestSettingsFailureClearsSecretsAndLateResultsCannotChangeAnotherDialog(t *testing.T) {
	m := formTestModel(t)
	d := m.settingsForm
	d.stage([]string{"providers", "local", "baseURL"}, "http://retained", false)
	d.stage([]string{"hidden"}, "NEW SECRET", false)
	d.writing = true
	m.handleSettingsForm(settingsFormResult{dialog: d, serial: d.serial, err: errors.New("write unconfirmed")})
	raw, _ := json.Marshal(d.ops)
	if !d.blocked || len(d.ops) != 1 || strings.Contains(string(raw), "SECRET") || strings.Contains(m.View(), "NEW SECRET") {
		t.Fatal("uncertain settings write retained a secret or lost the ordinary draft")
	}
	m.closeSettingsForm()
	m.openSettingsForm(true)
	fresh := m.settingsForm
	m.handleSettingsForm(settingsFormResult{dialog: d, serial: d.serial, credentialSaved: true})
	if m.settingsForm != fresh || fresh.note != "" {
		t.Fatal("late write result changed a new dialog")
	}
}
