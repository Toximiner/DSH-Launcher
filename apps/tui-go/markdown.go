package main

import (
	"fmt"
	"html"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

"github.com/Toximiner/DSH-Launcher/apps/tui-go/internal/theme")

const markdownInputLimit = 256 << 10
const markdownCacheLimit = 8 << 20

type markdownKey struct {
	text  string
	width int
}
type markdownCache struct {
	entries map[markdownKey]string
	order   []markdownKey
	bytes   int
}

func (m *model) messageText(message chatMessage, width int, raw bool) string {
	if raw || !message.assistant && message.role != tr("АГЕНТ") || message.tool != nil || message.approvalID != "" {
		return message.text
	}
	if m.markdown == nil {
		m.markdown = &markdownCache{entries: make(map[markdownKey]string)}
	}
	key := markdownKey{message.text, max(1, min(512, width))}
	if rendered, ok := m.markdown.entries[key]; ok {
		return rendered
	}
	rendered := renderMarkdown(message.text, key.width)
	size := len(message.text) + len(rendered)
	if size <= markdownCacheLimit {
		for len(m.markdown.order) >= 128 || m.markdown.bytes+size > markdownCacheLimit {
			old := m.markdown.order[0]
			m.markdown.order = m.markdown.order[1:]
			m.markdown.bytes -= len(old.text) + len(m.markdown.entries[old])
			delete(m.markdown.entries, old)
		}
		m.markdown.entries[key] = rendered
		m.markdown.order = append(m.markdown.order, key)
		m.markdown.bytes += size
	}
	return rendered
}

type terminalMarkdown struct {
	source []byte
	failed bool
	syntax *syntaxBudget
}

func renderMarkdown(source string, width int) string {
	source = clean(source)
	plain := func() string { return ansi.Hardwrap(source, max(1, width), true) }
	if len(source) > markdownInputLimit {
		return plain()
	}
	parser := goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough))
	document := parser.Parser().Parse(text.NewReader([]byte(source)))
	nodes, depth := 0, 0
	valid := true
	ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			nodes++
			depth++
		} else {
			depth--
		}
		if nodes > 20000 || depth > 64 {
			valid = false
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	if !valid {
		return plain()
	}
	renderer := terminalMarkdown{source: []byte(source)}
	result := renderer.blocks(document, max(1, width), 0)
	if renderer.failed || len(result) > 2<<20 {
		return plain()
	}
	return strings.TrimRight(result, "\n")
}

func (r *terminalMarkdown) inline(parent ast.Node, depth int) string {
	if depth > 64 {
		r.failed = true
		return ""
	}
	var out strings.Builder
	for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
		switch n := node.(type) {
		case *ast.Text:
			value := n.Value(r.source)
			if !n.IsRaw() {
				value = util.UnescapePunctuations(value)
				out.WriteString(clean(html.UnescapeString(string(value))))
			} else {
				out.Write(value)
			}
			if n.HardLineBreak() {
				out.WriteByte('\n')
			} else if n.SoftLineBreak() {
				out.WriteByte(' ')
			}
		case *ast.String:
			out.Write(n.Value)
		case *ast.CodeSpan:
			value := strings.ReplaceAll(string(n.Text(r.source)), "\n", " ")
			if strings.HasPrefix(value, " ") && strings.HasSuffix(value, " ") && strings.TrimSpace(value) != "" {
				value = value[1 : len(value)-1]
			}
			out.WriteString(lipgloss.NewStyle().Foreground(theme.Color(theme.Code)).Render(value))
		case *ast.Emphasis:
			style := lipgloss.NewStyle().Italic(true)
			if n.Level == 2 {
				style = lipgloss.NewStyle().Bold(true)
			}
			out.WriteString(style.Render(r.inline(n, depth+1)))
		case *extast.Strikethrough:
			out.WriteString(lipgloss.NewStyle().Strikethrough(true).Render(r.inline(n, depth+1)))
		case *ast.Link:
			label := r.inline(n, depth+1)
			destination := clean(html.UnescapeString(string(n.Destination)))
			out.WriteString(label)
			if destination != "" && clean(label) != destination {
				out.WriteString(" (" + destination + ")")
			}
		case *ast.AutoLink:
			out.Write(n.URL(r.source))
		case *ast.Image:
			out.WriteString(tr("[Изображение: ") + r.inline(n, depth+1) + "] (" + clean(html.UnescapeString(string(n.Destination))) + ")")
		case *ast.RawHTML:
			out.Write(n.Text(r.source))
		default:
			out.WriteString(r.inline(node, depth+1))
		}
	}
	return out.String()
}

func (r *terminalMarkdown) blocks(parent ast.Node, width, depth int) string {
	if depth > 64 {
		r.failed = true
		return ""
	}
	var parts []string
	for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
		parts = append(parts, r.block(node, width, depth+1))
	}
	return strings.Join(parts, "\n\n")
}
func prefixMarkdown(value, prefix string) string {
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}
func (r *terminalMarkdown) code(node ast.Node, language string, width int) string {
	label := tr("Код")
	if language != "" {
		label += " · " + language
	}
	content := strings.TrimSuffix(string(node.Lines().Value(r.source)), "\n")
	content = strings.ReplaceAll(content, "\t", "    ")
	if r.syntax == nil {
		r.syntax = newSyntaxBudget()
	}
	content = highlightCode(content, language, r.syntax)
	return muted().Render(ansi.Truncate("┌─ "+label, width, "…")) + "\n" + prefixMarkdown(ansi.Hardwrap(content, max(1, width-2), true), "│ ") + "\n" + muted().Render(strings.Repeat("─", min(width, 16)))
}
func (r *terminalMarkdown) block(node ast.Node, width, depth int) string {
	if depth > 64 {
		r.failed = true
		return ""
	}
	switch n := node.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		return ansi.Wrap(r.inline(node, depth), width, "")
	case *ast.Heading:
		return heading().Render(ansi.Wrap(r.inline(n, depth), width, ""))
	case *ast.FencedCodeBlock:
		return r.code(n, string(n.Language(r.source)), width)
	case *ast.CodeBlock:
		return r.code(n, "", width)
	case *ast.Blockquote:
		return prefixMarkdown(r.blocks(n, max(1, width-2), depth), "│ ")
	case *ast.List:
		var items []string
		number := n.Start
		for item := n.FirstChild(); item != nil; item = item.NextSibling() {
			marker := "• "
			if n.IsOrdered() {
				marker = fmt.Sprintf("%d. ", number)
				number++
			}
			indent := ansi.StringWidth(marker)
			content := r.blocks(item, max(1, width-indent), depth)
			lines := strings.Split(content, "\n")
			for i := range lines {
				if i == 0 {
					lines[i] = marker + lines[i]
				} else {
					lines[i] = strings.Repeat(" ", indent) + lines[i]
				}
			}
			items = append(items, strings.Join(lines, "\n"))
		}
		gap := "\n"
		if !n.IsTight {
			gap = "\n\n"
		}
		return strings.Join(items, gap)
	case *ast.ThematicBreak:
		return muted().Render(strings.Repeat("─", width))
	case *ast.HTMLBlock:
		return ansi.Hardwrap(strings.TrimSuffix(string(n.Text(r.source)), "\n"), width, true)
	case *extast.Table:
		return r.table(n, width, depth)
	default:
		return r.blocks(node, width, depth)
	}
}

func (r *terminalMarkdown) table(table *extast.Table, width, depth int) string {
	columns := len(table.Alignments)
	if columns == 0 || columns > 32 {
		r.failed = true
		return ""
	}
	var rows [][]string
	widths := make([]int, columns)
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		if (len(rows)+1)*columns > 2048 {
			r.failed = true
			return ""
		}
		cells := make([]string, columns)
		index := 0
		for cell := row.FirstChild(); cell != nil && index < columns; cell = cell.NextSibling() {
			cells[index] = strings.ReplaceAll(r.inline(cell, depth), "\n", " ")
			widths[index] = max(widths[index], ansi.StringWidth(cells[index]))
			index++
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return ""
	}
	required := 1
	for _, w := range widths {
		required += w + 3
	}
	if required > width {
		bytes := 0
		for _, row := range rows[1:] {
			for i, value := range row {
				bytes += len(rows[0][i]) + len(value) + 4
				if bytes > 2<<20 {
					r.failed = true
					return ""
				}
			}
		}
		var records []string
		for _, row := range rows[1:] {
			var fields []string
			for i, value := range row {
				fields = append(fields, ansi.Wrap(clean(rows[0][i])+": "+value, width, ""))
			}
			records = append(records, strings.Join(fields, "\n"))
		}
		if len(records) == 0 {
			return ansi.Wrap(strings.Join(rows[0], " · "), width, "")
		}
		return strings.Join(records, "\n\n")
	}
	border := func(left, middle, right string) string {
		parts := make([]string, columns)
		for i, w := range widths {
			parts[i] = strings.Repeat("─", w+2)
		}
		return left + strings.Join(parts, middle) + right
	}
	lines := []string{border("┌", "┬", "┐")}
	for index, row := range rows {
		var cells []string
		for i, value := range row {
			padding := widths[i] - ansi.StringWidth(value)
			left, right := 0, padding
			if table.Alignments[i] == extast.AlignRight {
				left, right = padding, 0
			} else if table.Alignments[i] == extast.AlignCenter {
				left = padding / 2
				right = padding - left
			}
			if index == 0 {
				value = heading().Render(value)
			}
			cells = append(cells, " "+strings.Repeat(" ", left)+value+strings.Repeat(" ", right)+" ")
		}
		lines = append(lines, "│"+strings.Join(cells, "│")+"│")
		if index == 0 {
			lines = append(lines, border("├", "┼", "┤"))
		}
	}
	return strings.Join(append(lines, border("└", "┴", "┘")), "\n")
}
