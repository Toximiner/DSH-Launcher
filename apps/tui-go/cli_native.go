//go:build !js

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
	"io"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func run(args []string, out io.Writer) error {
	activeEnglish.Store(false)
	if path, err := clientPreferencesPath(); err == nil {
		if prefs, err := readClientPreferences(path); err == nil {
			activeEnglish.Store(prefs.Language == "en")
		}
	}

	f := flag.NewFlagSet("dsh-launcher-tui-go", flag.ContinueOnError)
	f.SetOutput(out)
	helpFlag := f.Bool("help", false, tr("показать справку"))
	loginFlag := f.String("url", os.Getenv("DSH_TUI_URL"), tr("URL входа dsh для настроек и сессий"))
	sessionFlag := f.String("session", "", tr("идентификатор сессии dsh"))
	newFlag := f.Bool("new", false, tr("создать и подключить новую сессию dsh"))
	startFlag := f.Bool("start", false, tr("запустить локальный dsh с предложением установить маркет"))
	binFlag := f.String("dsh", "dsh", tr("путь к dsh"))
	profileFlag := f.String("profile", "web", tr("профиль запуска dsh"))
	portFlag := f.Int("port", 3080, tr("порт локального dsh"))
	cwdFlag := f.String("cwd", "", tr("рабочая папка dsh"))
	marketFlag := f.String("market", "ask", tr("предустановка маркета: ask, install или skip"))
	noMouseFlag := f.Bool("no-mouse", false, tr("отключить мышь для обычного выделения текста"))
	rollbackFlag := f.Bool("rollback-tui", false, tr("восстановить предыдущий TUI; работает также из .previous"))
	versionFlag := f.Bool("version", false, tr("показать версию"))
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *helpFlag {
		fmt.Fprintln(out, tr("dsh-launcher-tui-go [--help] [--version] [--no-mouse] [--url URL]\nGo-прототип: F10 — меню, F4 — настройки TUI, Tab — разделы, Esc — чат. /help, /clear, /exit.\nНастройки dsh доступны через --url или DSH_TUI_URL.\nПодключение: --session ID или --new. Режимы: /mode default|plan|goal.\nЛокальный запуск: --start [--dsh PATH] [--profile web] [--port 3080] [--market ask|install|skip].\nС сессией dsh: Enter — отправить, F8 — остановить. Без подключения — локальный черновик.\n/runtime — установка/процесс · /updates — обновление/откат · /diagnostics — отчёт.\nNode.js нужен только бэкенду dsh."))
		return nil
	}
	if *versionFlag {
		fmt.Fprintln(out, version)
		return nil
	}
	if *rollbackFlag {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		exe, err = filepath.EvalSymlinks(exe)
		if err != nil {
			return err
		}
		c := &nativeRuntime{executable: strings.TrimSuffix(exe, ".previous")}
		if err = c.rollbackTUI(context.Background()); err != nil {
			return err
		}
		fmt.Fprintln(out, tr("Предыдущий TUI восстановлен. Запустите его снова."))
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
	cwd := *cwdFlag
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	cwd, _ = filepath.Abs(cwd)
	control, controlErr := newNativeRuntime(backendOptions{Bin: *binFlag, Profile: *profileFlag, Port: *portFlag, CWD: cwd})
	if controlErr == nil {
		// Explicit CLI arguments override saved launch defaults.
		f.Visit(func(flag *flag.Flag) {
			switch flag.Name {
			case "dsh":
				control.config.Bin = *binFlag
			case "profile":
				control.config.Profile = *profileFlag
			case "port":
				control.config.Port = *portFlag
			case "cwd":
				control.config.CWD = cwd
			}
		})
		if err := validateRuntimeConfig(control.config); err != nil && *startFlag {
			return err
		}
		defer func() { cancel(); control.Close() }()
	}
	if controlErr == nil {
		port := control.config.Port
		if *loginFlag != "" {
			u, _ := url.Parse(*loginFlag)
			port = 0
			if u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1" {
				port, _ = strconv.Atoi(u.Port())
			}
		}
		if port > 0 {
			if err := control.trackBackend(port); err != nil {
				return err
			}
		}
	}
	if *startFlag {
		if controlErr != nil {
			return controlErr
		}
		if err := checkBackendNode(ctx); err != nil {
			return err
		}
		if *loginFlag != "" {
			return fmt.Errorf("--start cannot be combined with a login URL")
		}
		if _, err := exec.LookPath(control.selectedBin()); err != nil {
			reader := bufio.NewReader(os.Stdin)
			for {
				fmt.Fprint(out, tr("dsh не найден. [i] Установить в каталог TUI · [q] Выйти: "))
				type reply struct {
					line string
					err  error
				}
				response := make(chan reply, 1)
				go func() { line, e := reader.ReadString('\n'); response <- reply{line, e} }()
				var answer string
				select {
				case <-ctx.Done():
					return ctx.Err()
				case r := <-response:
					if r.err != nil {
						return r.err
					}
					answer = r.line
				}
				if strings.TrimSpace(answer) != "i" {
					return fmt.Errorf("%s", tr("установка отменена; запустите без --start для /runtime"))
				}
				control.live = out
				err = control.Execute(ctx, "install", "latest")
				control.live = nil
				if err == nil {
					break
				}
				fmt.Fprintln(out, control.logSnapshot())
				fmt.Fprintln(out, redactDiagnostic(err.Error()))
			}
		}
		options := backendOptions{Bin: control.selectedBin(), Profile: control.config.Profile, Port: control.config.Port, CWD: control.config.CWD, Market: *marketFlag}
		env, err := prepareMarket(ctx, options, os.Stdin, out)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, tr("Запуск dsh…"))
		owner, err := lockRuntime(control.root)
		if err != nil {
			return err
		}
		control.owner = owner
		login, stop, done, err := startBackendLogged(ctx, options, env, control)
		if err != nil {
			return err
		}
		control.adopt(login, stop, done)
		*loginFlag = login
		if *sessionFlag == "" {
			*newFlag = true
		}
	}
	if (*sessionFlag != "" || *newFlag) && *loginFlag == "" {
		return fmt.Errorf("a login URL or --start is required for sessions")
	}
	m := newModel()
	if controlErr == nil {
		m.runtime = control
		control.setStatus(tr("Управление dsh готово."), "")
		m.runtimeState = control.Snapshot()
	} else {
		m.events = append(m.events, tr("Управление установкой недоступно: ")+redactDiagnostic(controlErr.Error()))
	}
	defer m.stopChat()
	m.mouseForcedOff = *noMouseFlag
	if path, prefErr := clientPreferencesPath(); prefErr == nil {
		m.prefs, prefErr = readClientPreferences(path)
		m.savePreferences = func(prefs clientPreferences) error { return writeClientPreferences(path, prefs) }
		if prefErr != nil {
			m.preferencesNote = tr("Ошибка чтения настроек; используются стандартные.")
		}
	} else {
		m.preferencesNote = tr("Папка настроек недоступна; выбор не сохраняется.")
	}
	m.applyClientAppearance()
	m.resize()
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
