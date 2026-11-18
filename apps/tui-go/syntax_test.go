package main

import (
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func syntaxTestProfile(t *testing.T) {
	t.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
}

func TestSyntaxPreservesCodeAndColorsDeclaredLanguage(t *testing.T) {
	syntaxTestProfile(t)
	for _, example := range [][2]string{
		{"Go", "package main\n\nfunc main() {\n    fmt.Println(\"世界 🙂\", 42) // comment\n}\n\n"},
		{"py", "def hello():\n    print(\"Привет\", 42) # comment\n\n"},
		{"js", "const value = 42; // comment\nconsole.log(\"hello\");\n"},
		{"ts", "const value: number = 42;\n"},
		{"bash", "# comment\nprintf '%s\\n' \"hello\"\n"},
		{"json", "{\"value\": 42, \"name\": \"世界\"}\n"},
		{"yaml", "name: \"世界\"\ncount: 42\n"},
		{"rust", "fn main() { println!(\"hello\"); }\n"},
	} {
		t.Run(example[0], func(t *testing.T) {
			got := highlightCode(example[1], example[0], newSyntaxBudget())
			if ansi.Strip(got) != example[1] {
				t.Fatalf("syntax changed code bytes: %q", ansi.Strip(got))
			}
			if !strings.Contains(got, "\x1b[") {
				t.Fatal("declared language was not colored")
			}
		})
	}
	// A monochrome terminal keeps identical readable text.
	lipgloss.SetColorProfile(termenv.Ascii)
	source := "func main() {}\n"
	if got := highlightCode(source, "go", newSyntaxBudget()); got != source {
		t.Fatal("monochrome terminal received styling", got)
	}
}

func TestSyntaxFallbackRetainsAllContentAndStopsAtBudget(t *testing.T) {
	syntaxTestProfile(t)
	for _, language := range []string{"", "not-a-language", "main.go", strings.Repeat("a", 65)} {
		source := "const answer = 42;\n"
		if got := highlightCode(source, language, newSyntaxBudget()); got != source {
			t.Fatal("unknown/unlabelled language was guessed", language)
		}
	}
	for _, source := range []string{strings.Repeat("x", syntaxBlockLimit+1), strings.Repeat("1,", 4200)} {
		if got := highlightCode(source, "json", newSyntaxBudget()); got != source {
			t.Fatal("large or token-heavy source did not fall back intact")
		}
	}
	source := "func main() {}"
	for _, budget := range []*syntaxBudget{nil, {remaining: len(source) - 1, deadline: time.Now().Add(time.Second)}, {remaining: syntaxRenderLimit, deadline: time.Now().Add(-time.Second)}} {
		if got := highlightCode(source, "go", budget); got != source {
			t.Fatal("exhausted syntax budget did not fall back")
		}
	}
}

func TestSyntaxCannotInjectTerminalActionsAndWrappingRetainsRows(t *testing.T) {
	syntaxTestProfile(t)
	source := "func main() {\n    println(\"世界 🙂\") // " + strings.Repeat("long ", 25) + "\n}\n\n"
	for _, width := range []int{24, 50, 74, 110} {
		rendered := renderMarkdown("```go\n"+source+"```", width)
		if !strings.Contains(rendered, "\x1b[") {
			t.Fatal("formatted code has no styling")
		}
		for _, line := range strings.Split(rendered, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatal("syntax wrapping overflowed", width, line)
			}
		}
		// Colors must not change row boundaries, blank lines, or wrapping.
		plain := renderMarkdown("```unknown-language\n"+source+"```", width)
		coloredRows := strings.Split(ansi.Strip(rendered), "\n")
		plainRows := strings.Split(ansi.Strip(plain), "\n")
		if strings.Join(coloredRows[1:], "\n") != strings.Join(plainRows[1:], "\n") {
			t.Fatal("syntax changed code layout")
		}
	}
	hostile := "// \x1b]52;c;YQ==\x07\nprintln(\"\x1b[2J\")\n"
	got := highlightCode(hostile, "go", newSyntaxBudget())
	if strings.Contains(got, "\x1b]") || strings.Contains(got, "\x1b[2J") || strings.Contains(got, "\x07") || ansi.Strip(got) != clean(hostile) {
		t.Fatal("code exposed terminal actions")
	}
}

func TestSyntaxLexerErrorsAndPanicsFallBackWithoutPartialOutput(t *testing.T) {
	syntaxTestProfile(t)
	for _, rules := range []func() chroma.Rules{
		func() chroma.Rules { return chroma.Rules{"root": {{Pattern: "[", Type: chroma.Text}}} },
		func() chroma.Rules { panic("broken lexer") },
	} {
		name := "test-broken-lexer"
		syntaxLexers[name] = chroma.MustNewLexer(&chroma.Config{Name: name}, rules)
		source := "unchanged source\n"
		got := highlightCode(source, name, newSyntaxBudget())
		delete(syntaxLexers, name)
		if got != source {
			t.Fatal("lexer failure changed text or retained partial colors")
		}
	}
}
