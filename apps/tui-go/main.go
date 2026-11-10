// A desktop-shaped terminal shell, without backend actions at this checkpoint.
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

const version = "0.2.0-preview.3"
const help = "/help — справка\n/clear — очистить экран\n/exit — выйти\n↑/↓ — история ввода · PgUp/PgDn — прокрутка\nTab — боковая панель · Esc — вернуться в чат\nМышь: клик — раздел или поле ввода · колесо — прокрутка"

var pages = []string{"Чат", "Сессии", "Настройки", "Журнал"}

type quitMsg struct{}

type model struct {
	input         textinput.Model
	viewport      viewport.Model
	width, height int
	lines         []string
	history       []string
	historyIndex  int
	draft         string
	workspace     string
	page          int
	navFocused    bool
	events        []string
}

func newModel() *model {
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "Сообщение или /help…"
	input.CharLimit = 65536
	input.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e6edf3"))
	input.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b949e"))
	input.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#58a6ff"))
	input.Focus()
	cwd, _ := os.Getwd()
	m := &model{input: input, viewport: viewport.New(78, 17), width: 80, height: 24, workspace: clean(cwd), events: []string{"Открыт интерфейс клиента. Подключение к dsh отсутствует."}}
	m.resize()
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
		label := "СПРАВКА"
		text := line
		if strings.HasPrefix(line, "Введено: ") {
			label = "ВЫ · ЛОКАЛЬНЫЙ ЧЕРНОВИК"
			text = strings.TrimPrefix(line, "Введено: ")
		}
		lines = append(lines, accent.Bold(true).Render(label)+"\n"+ansi.Hardwrap(clean(text), max(1, m.viewport.Width), true)+"\n")
	}
	content := strings.Join(lines, "\n")
	if len(m.lines) == 0 {
		welcome := heading.Render("С чего начнём?") + "\n\n" + muted.Render("Нет подключения к dsh.") + "\n" +
			"Здесь появится разговор с агентом.\n\n" + muted.Render("Пока можно проверить ввод и навигацию.") + "\n\n" + accent.Render("F1 — справка   Tab — разделы")
		content = lipgloss.Place(m.viewport.Width, m.viewport.Height, lipgloss.Center, lipgloss.Center, welcome)
	}
	switch m.page {
	case 1:
		content = heading.Render("Локальный черновик") + "\n\nТекст в этом окне сохраняется только в памяти.\n\n" + muted.Render("Сессии dsh недоступны без подключения.") + "\n\nEsc — вернуться в чат"
	case 2:
		content = heading.Render("Данные этого окна") + "\n\nВерсия     " + version + "\nИнтерфейс  Русский · тёмная тема\nТерминал   " + fmt.Sprintf("%d × %d", m.width, m.height) + "\n\nРабочая папка\n" + m.workspace + "\n\n" + muted.Render("Подключение к dsh отсутствует.")
	case 3:
		content = heading.Render("Журнал клиента · локально") + "\n\n" + strings.Join(m.events, "\n")
	}
	m.viewport.SetContent(ansi.Hardwrap(content, max(1, m.viewport.Width), true))
}

func (m *model) append(line string) {
	m.lines = append(m.lines, clean(line))
	if len(m.lines) > 1000 {
		m.lines = m.lines[len(m.lines)-1000:]
	}
	m.rebuild()
	m.viewport.GotoBottom()
}

func (m *model) resize() {
	w, h := m.contentSize()
	if m.page == 0 {
		h -= 6
	}
	m.viewport.Width = max(1, w-4)
	m.viewport.Height = max(1, h-4)
	m.input.Width = max(1, w-6)
	m.rebuild()
}

func (m *model) contentSize() (int, int) {
	w := max(1, m.width-1)
	if w >= 78 {
		w -= sidebarWidth + sidebarGap
	}
	return w, max(1, m.height-3)
}

func (m *model) showChat() tea.Cmd {
	m.page = 0
	m.navFocused = false
	m.resize()
	return m.input.Focus()
}

func (m *model) Init() tea.Cmd { return textinput.Blink }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case quitMsg:
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
	case tea.MouseMsg:
		return m, m.mouse(msg)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+d":
			return m, tea.Quit
		case "tab":
			m.navFocused = !m.navFocused
			if m.navFocused || m.page != 0 {
				m.input.Blur()
				return m, nil
			}
			return m, m.input.Focus()
		case "esc":
			return m, m.showChat()
		case "f1":
			m.showChat()
			m.append(help)
			return m, nil
		case "enter":
			if m.navFocused {
				m.navFocused = false
				if m.page == 0 {
					return m, m.input.Focus()
				}
				return m, nil
			}
			if m.page != 0 {
				return m, nil
			}
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
				m.events = append(m.events, "Сохранён локальный черновик: "+clean(line))
				if len(m.events) > 50 {
					m.events = m.events[len(m.events)-50:]
				}
			}
			return m, nil
		case "up", "down":
			if m.navFocused {
				delta := 1
				if msg.String() == "up" {
					delta = -1
				}
				m.page = (m.page + delta + len(pages)) % len(pages)
				m.resize()
				return m, nil
			}
			if m.page != 0 {
				m.viewport, _ = m.viewport.Update(msg)
				return m, nil
			}
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
	if m.navFocused || m.page != 0 {
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func run(args []string, out io.Writer) error {
	f := flag.NewFlagSet("dsh-launcher-tui-go", flag.ContinueOnError)
	f.SetOutput(out)
	helpFlag := f.Bool("help", false, "показать справку")
	noMouseFlag := f.Bool("no-mouse", false, "отключить мышь для обычного выделения текста")
	versionFlag := f.Bool("version", false, "показать версию")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *helpFlag {
		fmt.Fprintln(out, "dsh-launcher-tui-go [--help] [--version] [--no-mouse]\nGo-прототип: панели, Tab — разделы, Esc — чат, /help, /clear, /exit.\nПодключение к dsh пока не реализовано. Node.js для запуска не нужен.")
		return nil
	}
	if *versionFlag {
		fmt.Fprintln(out, version)
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("run this application in an interactive terminal")
	}
	options := []tea.ProgramOption{tea.WithAltScreen(), tea.WithoutSignalHandler()}
	if !*noMouseFlag {
		options = append(options, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(newModel(), options...)
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
