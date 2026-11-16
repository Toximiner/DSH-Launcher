package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func benchTestModel(t *testing.T, kind string) *model {
	t.Helper()
	m := chatModel(t)
	applyChat(t, m, `{"type":"snapshot","header":{"id":"s"},"cursor":-1}`)
	m.openWorkbench(kind)
	d := m.workbench
	m.handleBench(benchResult{dialog: d, serial: d.serial, rows: []benchRow{{ID: "r", Title: "Row", Body: "Details", Data: map[string]any{"updatedAt": "revision-1", "title": "Title", "prompt": "Prompt", "status": "active"}}}})
	return m
}
func TestWorkbenchGeometryDraftCancellationAndStaleResults(t *testing.T) {
	for _, kind := range []string{"workspaces", "packages", "presets", "files", "plugins", "commands", "jobs", "agents", "tasks", "schedules", "terminals"} {
		m := benchTestModel(t, kind)
		d := m.workbench
		for _, size := range [][2]int{{120, 42}, {100, 30}, {80, 24}} {
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			if lipgloss.Width(m.View()) > m.width-1 || lipgloss.Height(m.View()) > m.height {
				t.Fatalf("%s overflow at %v", kind, size)
			}
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if m.workbench != nil {
			t.Fatal("panel did not close")
		}
		m.handleBench(benchResult{dialog: d, serial: d.serial, rows: []benchRow{{ID: "late"}}})
		if m.workbench != nil {
			t.Fatal("late response reopened panel")
		}
	}
	m := benchTestModel(t, "schedules")
	m.Update(tea.KeyMsg{Type: tea.KeyF3})
	d := m.workbench
	original := d.editor.Value()
	m.Update(tea.KeyMsg{Type: tea.KeyEnter, Paste: true})
	if d.confirm || d.editor.Value() != original {
		t.Fatal("pasted key triggered action")
	}
	d.editor.SetValue(`{"title":"new"}`)
	m.Update(tea.KeyMsg{Type: tea.KeyF5})
	req := objectValue(d.action.Args["request"])
	if !d.confirm || req["expectedUpdatedAt"] != "revision-1" || objectValue(req["patch"])["title"] != "new" {
		t.Fatal("CAS or edit patch lost")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !d.editing || d.confirm {
		t.Fatal("cancel confirmation lost draft")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if d.editing || d.action != nil {
		t.Fatal("draft not discarded")
	}
}
func TestWorkbenchRefusalDoesNotRetryOrLeakBackendError(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" {
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "test", Path: "/"})
			w.WriteHeader(302)
			return
		}
		writes++
		var req struct {
			ID string `json:"rpcId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "server-response", "rpcId": req.ID, "result": map[string]any{"ok": true, "value": map[string]any{"code": "schedule_conflict", "diagnostic": "private-secret"}}})
	}))
	defer server.Close()
	m := benchTestModel(t, "schedules")
	m.settingsLogin = server.URL + "/?token=test"
	m.benchPrepare(tea.KeySpace)
	d := m.workbench
	result := m.benchExecute()().(benchResult)
	m.handleBench(result)
	if writes != 1 || !d.blocked || strings.Contains(m.View(), "private-secret") {
		t.Fatal("refusal not isolated")
	}
	if m.benchExecute() != nil || m.benchPrepare(tea.KeySpace) != nil || writes != 1 {
		t.Fatal("uncertain mutation retried")
	}
}
func TestComposePreservesMultilineAndRejectsPartialPaste(t *testing.T) {
	m := benchTestModel(t, "files")
	m.closeWorkbench()
	m.openCompose()
	d := m.composer
	d.editor.SetValue("")
	m.composeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("First\nПривет\nlast"), Paste: true})
	before := d.editor.Value()
	m.composeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.Repeat("x", 65537)), Paste: true})
	if d.editor.Value() != before {
		t.Fatal("oversized paste changed draft")
	}
	m.composeKey(tea.KeyMsg{Type: tea.KeyEsc})
	m.openCompose()
	if m.composer != d || d.editor.Value() != before {
		t.Fatal("hidden draft lost")
	}
	for _, size := range [][2]int{{100, 30}, {80, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if lipgloss.Width(m.View()) > m.width-1 || lipgloss.Height(m.View()) > m.height {
			t.Fatal("compose overflow")
		}
	}
	old := d.serial
	d.serial++
	m.handleComposeUpload(composeUpload{dialog: d, serial: old, attachment: &promptAttachment{Receipt: "late"}})
	if len(d.attachments) != 0 {
		t.Fatal("late upload attached")
	}
}
func TestComposeFileRejectsSymlinkAndOversize(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "input")
	if err := os.WriteFile(file, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := readComposeFile(file)
	if err != nil || string(data) != "hello" {
		t.Fatal("regular file unreadable", err)
	}
	link := filepath.Join(dir, "link")
	if err = os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if _, err = readComposeFile(link); err == nil {
		t.Fatal("symlink accepted")
	}
	if err = os.WriteFile(file, make([]byte, (2<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readComposeFile(file); err == nil {
		t.Fatal("oversize accepted")
	}
}

func TestComposeRejectedMessageRetainsOriginalTextAndReceipt(t *testing.T) {
	m := benchTestModel(t, "files")
	m.closeWorkbench()
	m.input.SetValue("")
	text := "First\nSecond привет"
	m.prompts["s"] = &pendingPrompt{ID: "request", Text: text, Status: "sending", Attachments: []promptAttachment{{Receipt: "receipt", Name: "sample.txt", Bytes: 5}}}
	m.handlePromptResult(promptResult{sessionID: "s", requestID: "request", err: errors.New("rejected"), rejected: true})
	if m.input.Value() != "" {
		t.Fatal("multiline draft was flattened into single-line input")
	}
	m.openCompose()
	if m.composer.editor.Value() != text || len(m.composer.attachments) != 1 || m.composer.attachments[0].Receipt != "receipt" {
		t.Fatal("rejected text or upload receipt lost")
	}
}
func TestWorkbenchMouseSelectsWithoutExecutingAndScrollsFile(t *testing.T) {
	m := benchTestModel(t, "files")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	d := m.workbench
	d.rows = append(d.rows, benchRow{ID: "second", Title: "Second"})
	box := m.benchBox()
	m.benchMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if d.index != 1 || d.busy {
		t.Fatal("row click executed an action or missed selection")
	}
	d.document = "file"
	d.fileBody = strings.Repeat("line\n", 100)
	_ = m.View()
	m.benchMouse(tea.MouseMsg{X: box.x + 3, Y: box.y + 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if d.view.YOffset == 0 {
		t.Fatal("file wheel did not scroll")
	}
}
