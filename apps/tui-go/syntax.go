package main

import (
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

const syntaxBlockLimit = 16 << 10
const syntaxRenderLimit = 64 << 10
const syntaxTokenLimit = 8192

// Match declared names/aliases only; never guess a language from source or filenames.
var syntaxLexers = func() map[string]chroma.Lexer {
	result := make(map[string]chroma.Lexer)
	for _, lexer := range lexers.GlobalLexerRegistry.Lexers {
		config := lexer.Config()
		result[strings.ToLower(config.Name)] = lexer
		for _, alias := range config.Aliases {
			result[strings.ToLower(alias)] = lexer
		}
	}
	return result
}()

type syntaxBudget struct {
	remaining int
	deadline  time.Time
}

func newSyntaxBudget() *syntaxBudget {
	return &syntaxBudget{remaining: syntaxRenderLimit, deadline: time.Now().Add(150 * time.Millisecond)}
}

// The deadline is checked between tokens; a single library match may take longer.
// Errors, panics, changed token text, or an exhausted budget retain the plain code.
func highlightCode(source, language string, budget *syntaxBudget) (result string) {
	source = clean(source)
	result = source
	if len(source) == 0 || len(source) > syntaxBlockLimit || len(language) > 64 || budget == nil || len(source) > budget.remaining || time.Now().After(budget.deadline) {
		return
	}
	if lipgloss.ColorProfile() == termenv.Ascii {
		return
	}
	lexer := syntaxLexers[strings.ToLower(strings.TrimSpace(language))]
	if lexer == nil {
		return
	}
	budget.remaining -= len(source)
	defer func() {
		if recover() != nil {
			result = source
		}
	}()
	iterator, err := lexer.Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, source)
	if err != nil {
		return
	}
	var out strings.Builder
	offset := 0
	profile := lipgloss.ColorProfile()
	for count := 0; count < syntaxTokenLimit; count++ {
		if time.Now().After(budget.deadline) {
			return source
		}
		token := iterator()
		if token == chroma.EOF {
			if offset == len(source) {
				return out.String()
			}
			return source
		}
		if !strings.HasPrefix(source[offset:], token.Value) {
			return source
		}
		offset += len(token.Value)
		color := syntaxColor(token.Type)
		if color == "" {
			out.WriteString(token.Value)
		} else {
			out.WriteString(termenv.String(token.Value).Foreground(profile.Color(color)).String())
		}
		if out.Len() > 512<<10 {
			return source
		}
	}
	return source
}

func syntaxColor(kind chroma.TokenType) string {
	switch {
	case kind == chroma.GenericInserted:
		return "#7ee787"
	case kind == chroma.GenericDeleted:
		return "#ff7b72"
	case kind == chroma.GenericHeading || kind == chroma.GenericSubheading:
		return "#79c0ff"
	case kind.Category() == chroma.Comment:
		return "#8b949e"
	case kind.Category() == chroma.Keyword:
		return "#ff7b72"
	case kind.SubCategory() == chroma.LiteralString:
		return "#a5d6ff"
	case kind.SubCategory() == chroma.LiteralNumber:
		return "#79c0ff"
	case kind.SubCategory() == chroma.NameFunction || kind == chroma.NameClass:
		return "#d2a8ff"
	case kind.SubCategory() == chroma.NameBuiltin || kind == chroma.NameTag || kind == chroma.NameAttribute:
		return "#7ee787"
	case kind.Category() == chroma.Operator:
		return "#ff7b72"
	}
	return ""
}
