//go:build !js

package main

import (
	"context"
	"flag"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

func run(args []string, out io.Writer) error {
	f := flag.NewFlagSet("dsh-launcher-tui-go", flag.ContinueOnError)
	f.SetOutput(out)
	helpFlag := f.Bool("help", false, "показать справку")
	loginFlag := f.String("url", os.Getenv("DSH_TUI_URL"), "URL входа dsh для настроек и сессий")
	sessionFlag := f.String("session", "", "идентификатор сессии dsh")
	newFlag := f.Bool("new", false, "создать и подключить новую сессию dsh")
	startFlag := f.Bool("start", false, "запустить локальный dsh с предложением установить маркет")
	binFlag := f.String("dsh", "dsh", "путь к dsh")
	profileFlag := f.String("profile", "web", "профиль запуска dsh")
	portFlag := f.Int("port", 3080, "порт локального dsh")
	cwdFlag := f.String("cwd", "", "рабочая папка dsh")
	marketFlag := f.String("market", "ask", "предустановка маркета: ask, install или skip")
	noMouseFlag := f.Bool("no-mouse", false, "отключить мышь для обычного выделения текста")
	versionFlag := f.Bool("version", false, "показать версию")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *helpFlag {
		fmt.Fprintln(out, "dsh-launcher-tui-go [--help] [--version] [--no-mouse] [--url URL]\nGo-прототип: F10 — меню, F4 — настройки TUI, Tab — разделы, Esc — чат. /help, /clear, /exit.\nНастройки dsh доступны только для чтения через --url или DSH_TUI_URL.\nПодключение: --session ID или --new. Режимы: /mode default|plan|goal.\nЛокальный запуск: --start [--dsh PATH] [--profile web] [--port 3080] [--market ask|install|skip].\nС сессией dsh: Enter — отправить, F8 — остановить. Без подключения — локальный черновик.\nNode.js нужен только бэкенду dsh.")
		return nil
	}
	if *versionFlag {
		fmt.Fprintln(out, version)
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("run this application in an interactive terminal")
	}
	if *loginFlag != "" {
		if _, err := settingsURL(*loginFlag); err != nil {
			return err
		}
	}
	if *sessionFlag != "" && *newFlag {
		return fmt.Errorf("use --session or --new")
	}
	if !profileName.MatchString(*profileFlag) || *portFlag < 1024 || *portFlag > 65535 {
		return fmt.Errorf("invalid profile or port")
	}
	if *marketFlag != "ask" && *marketFlag != "install" && *marketFlag != "skip" {
		return fmt.Errorf("invalid --market; use ask, install, or skip")
	}
	if !*startFlag && *marketFlag != "ask" {
		return fmt.Errorf("--market applies only with --start")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	if *startFlag {
		if err := checkBackendNode(ctx); err != nil {
			return err
		}
		if *loginFlag != "" {
			return fmt.Errorf("--start cannot be combined with a login URL")
		}
		cwd := *cwdFlag
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		cwd, err := filepath.Abs(cwd)
		if err != nil {
			return err
		}
		options := backendOptions{Bin: *binFlag, Profile: *profileFlag, Port: *portFlag, CWD: cwd, Market: *marketFlag}
		env, err := prepareMarket(ctx, options, os.Stdin, out)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "Запуск dsh…")
		login, stop, err := startBackend(ctx, options, env)
		if err != nil {
			return err
		}
		defer stop()
		*loginFlag = login
		if *sessionFlag == "" {
			*newFlag = true
		}
	}
	if (*sessionFlag != "" || *newFlag) && *loginFlag == "" {
		return fmt.Errorf("a login URL or --start is required for sessions")
	}
	m := newModel()
	defer m.stopChat()
	m.mouseForcedOff = *noMouseFlag
	if path, prefErr := clientPreferencesPath(); prefErr == nil {
		m.prefs, prefErr = readClientPreferences(path)
		m.savePreferences = func(prefs clientPreferences) error { return writeClientPreferences(path, prefs) }
		if prefErr != nil {
			m.preferencesNote = "Ошибка чтения настроек; используются стандартные."
		}
	} else {
		m.preferencesNote = "Папка настроек недоступна; выбор не сохраняется."
	}
	m.settingsLogin = *loginFlag
	m.settingsContext = ctx
	m.initialSession = *sessionFlag
	m.createSession = *newFlag
	options := []tea.ProgramOption{tea.WithAltScreen(), tea.WithoutSignalHandler()}
	if m.mouseEnabled() {
		options = append(options, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(m, options...)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	done := make(chan struct{})
	defer close(done)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			cancel()
			p.Send(quitMsg{})
		case <-ctx.Done():
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
