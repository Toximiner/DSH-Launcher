// The first Go migration checkpoint: a terminal shell, without backend actions.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

const version = "0.2.0-preview.1"
const help = "/help — команды\n/clear — очистить экран\n/exit — выйти\n↑/↓ — история ввода · PgUp/PgDn — прокрутка\nЭто первый Go-прототип. Подключение к dsh будет следующим этапом."

type quitMsg struct{}

type model struct {
	input         textinput.Model
	viewport      viewport.Model
	width, height int
	lines         []string
	history       []string
	historyIndex  int
	draft         string
}

func newModel() *model {
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "Введите текст или /help"
	input.CharLimit = 65536
	input.Focus()
	m := &model{input: input, viewport: viewport.New(78, 17), width: 80, height: 24}
	m.append("DSH Launcher TUI · Go + Bubble Tea\nТерминальный прототип: текст пока не отправляется в dsh.\nВведите /help, чтобы посмотреть команды.")
	return m
}

func clean(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, ansi.Strip(value))
}

func (m *model) rebuild() {
	var lines []string
	for _, line := range m.lines {
		lines = append(lines, ansi.Hardwrap(clean(line), max(1, m.viewport.Width), true))
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m *model) append(line string) {
	m.lines = append(m.lines, clean(line))
	if len(m.lines) > 1000 {
		m.lines = m.lines[len(m.lines)-1000:]
	}
	m.rebuild()
	m.viewport.GotoBottom()
}

func (m *model) Init() tea.Cmd { return textinput.Blink }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case quitMsg:
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.viewport.Width = max(1, msg.Width-2)
		m.viewport.Height = max(1, msg.Height-7)
		m.input.Width = max(1, msg.Width-5)
		m.rebuild()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+d":
			return m, tea.Quit
		case "enter":
			line := strings.TrimSpace(m.input.Value())
			m.input.SetValue("")
			if line == "" {
				return m, nil
			}
			if len(m.history) == 0 || m.history[len(m.history)-1] != line {
				m.history = append(m.history, line)
			}
			if len(m.history) > 50 {
				m.history = m.history[len(m.history)-50:]
			}
			m.historyIndex = len(m.history)
			switch line {
			case "/exit", "/quit":
				return m, tea.Quit
			case "/clear":
				m.lines = nil
				m.rebuild()
			case "/help":
				m.append(help)
			default:
				m.append("Введено: " + line)
			}
			return m, nil
		case "up", "down":
			if m.historyIndex == len(m.history) {
				m.draft = m.input.Value()
			}
			if msg.String() == "up" {
				m.historyIndex = max(0, m.historyIndex-1)
			} else {
				m.historyIndex = min(len(m.history), m.historyIndex+1)
			}
			if m.historyIndex == len(m.history) {
				m.input.SetValue(m.draft)
			} else {
				m.input.SetValue(m.history[m.historyIndex])
			}
			m.input.CursorEnd()
			return m, nil
		case "pgup", "pgdown", "ctrl+pgup", "ctrl+pgdown":
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *model) View() string {
	if m.width < 30 || m.height < 10 {
		return "Увеличьте терминал (от 30×10). Ctrl+D — выход."
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69")).Render("DSH Launcher TUI · Go")
	return title + "\n\n" + m.viewport.View() + "\n\n" +
		ansi.Truncate("Прототип · без подключения к dsh · PgUp/PgDn · Ctrl+D — выход", m.width-1, "…") +
		"\n" + m.input.View()
}

func run(args []string, out io.Writer) error {
	f := flag.NewFlagSet("dsh-launcher-tui-go", flag.ContinueOnError)
	f.SetOutput(out)
	helpFlag := f.Bool("help", false, "показать справку")
	versionFlag := f.Bool("version", false, "показать версию")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *helpFlag {
		fmt.Fprintln(out, "dsh-launcher-tui-go [--help] [--version]\nПервый Go-прототип: ввод, история, прокрутка, /help, /clear, /exit.\nПодключение к dsh пока не реализовано. Node.js для запуска не нужен.")
		return nil
	}
	if *versionFlag {
		fmt.Fprintln(out, version)
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("run this application in an interactive terminal")
	}
	p := tea.NewProgram(newModel(), tea.WithAltScreen(), tea.WithoutSignalHandler())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	done := make(chan struct{})
	defer close(done)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			p.Send(quitMsg{})
		case <-done:
		}
	}()
	_, err := p.Run()
	return err
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "dsh-launcher-tui-go:", err)
		os.Exit(1)
	}
}
