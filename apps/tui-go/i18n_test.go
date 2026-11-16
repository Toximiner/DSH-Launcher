package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestEnglishCatalogCoversApplicationStrings(t *testing.T) {
	format := regexp.MustCompile(`%(?:\[[0-9]+\])?[+#0 -]*(?:[0-9]+|\*)?(?:\.(?:[0-9]+|\*))?[a-zA-Z%]`)
	files, _ := filepath.Glob("*.go")
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(source, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			russian := strings.ContainsFunc(s, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) })
			if !russian {
				return true
			}
			translation, ok := englishCatalog[s]
			if !ok {
				t.Errorf("missing translation in %s: %q", file, s)
				return true
			}
			if strings.Join(format.FindAllString(s, -1), "|") != strings.Join(format.FindAllString(translation, -1), "|") {
				t.Errorf("format placeholders changed: %q", s)
			}
			return true
		})
	}
}
func TestLanguageSwitchPreservesUserContentAndGeometry(t *testing.T) {
	m := newModel()
	defer activeEnglish.Store(false)
	m.input.SetValue("О программе — пользовательский текст")
	m.openDialog("settings")
	m.togglePreference(6)
	if !activeEnglish.Load() || m.prefs.Language != "en" {
		t.Fatal("language did not switch")
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 30}} {
		m.Update(size)
		for _, dialog := range []string{"settings", "help", "about"} {
			m.openDialog(dialog)
			view := m.View()
			if lipgloss.Width(view) > size.Width-1 || lipgloss.Height(view) > size.Height {
				t.Fatalf("English %s overflowed", dialog)
			}
			if strings.Contains(view, "Горячие клавиши") || strings.Contains(view, "Настройки TUI") {
				t.Fatal("shell labels were not translated")
			}
		}
	}
	if m.input.Value() != "О программе — пользовательский текст" {
		t.Fatal("language switch translated user text")
	}
	m.togglePreference(6)
	if activeEnglish.Load() {
		t.Fatal("Russian language was not restored")
	}
}
