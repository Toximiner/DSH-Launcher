//go:build !js

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

type backendOptions struct {
	Bin, Profile, CWD, Market string
	Port                      int
}

var profileName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func marketInstalled(profile string) bool {
	home := os.Getenv("DSH_HOME")
	if home == "" {
		base, _ := os.UserHomeDir()
		home = filepath.Join(base, ".dsh")
	}
	data, err := os.ReadFile(filepath.Join(home, "profiles", profile, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
		Dsh          struct {
			Profile struct {
				Bundles []string `json:"bundles"`
			} `json:"profile"`
		} `json:"dsh"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return false
	}
	if pkg.Dependencies["dshmarket"] != "" {
		return true
	}
	for _, name := range pkg.Dsh.Profile.Bundles {
		if name == "dshmarket" {
			return true
		}
	}
	return false
}
func marketPreferences() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "dsh-launcher-tui", "market-prompt.json")
}
func marketNever() bool {
	data, _ := os.ReadFile(marketPreferences())
	var prefs struct {
		DontAsk bool `json:"dontAsk"`
	}
	json.Unmarshal(data, &prefs)
	return prefs.DontAsk
}
func rememberMarketNever() error {
	path := marketPreferences()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("{\"dontAsk\":true}\n"), 0600)
}

func runLocal(ctx context.Context, bin string, args []string, cwd string, env []string, out io.Writer) error {
	command := exec.CommandContext(ctx, bin, args...)
	command.Dir = cwd
	command.Env = env
	command.Stdout = out
	command.Stderr = out
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process != nil {
			return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	command.WaitDelay = 3 * time.Second
	return command.Run()
}
func prepareMarket(ctx context.Context, options backendOptions, input io.Reader, out io.Writer) ([]string, error) {
	env := os.Environ()
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	prefix := filepath.Join(base, "dsh-launcher-tui", "tools")
	toolsBin := filepath.Join(prefix, "node_modules", ".bin")
	_, pnpmErr := exec.LookPath("pnpm")
	pnpmReady := pnpmErr == nil
	if info, err := os.Stat(filepath.Join(toolsBin, "pnpm")); err == nil && !info.IsDir() {
		pnpmReady = true
		env = append(env, "PATH="+toolsBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	if marketInstalled(options.Profile) || options.Market == "skip" || (options.Market == "ask" && marketNever()) {
		return env, nil
	}
	reader := bufio.NewReader(input)
	for {
		choice := "install"
		if options.Market != "install" {
			fmt.Fprintf(out, tr("\nВ профиле %s нет dshmarket.\n[i] Установить  [s] Не сейчас  [n] Не спрашивать больше: "), options.Profile)
			type answer struct {
				line string
				err  error
			}
			answers := make(chan answer, 1)
			go func() { line, err := reader.ReadString('\n'); answers <- answer{line, err} }()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case value := <-answers:
				if value.err != nil && strings.TrimSpace(value.line) == "" {
					return nil, errors.New(tr("не получен ответ об установке маркета"))
				}
				choice = strings.ToLower(strings.TrimSpace(value.line))
			}
		}
		switch choice {
		case "s", "skip":
			return env, nil
		case "n", "never":
			if err := rememberMarketNever(); err != nil {
				return nil, err
			}
			return env, nil
		case "i", "install":
		default:
			fmt.Fprintln(out, tr("Введите i, s или n."))
			continue
		}
		if !pnpmReady {
			npm, err := exec.LookPath("npm")
			if err != nil {
				return nil, errors.New(tr("для установки маркета нужны npm и pnpm"))
			}
			fmt.Fprintln(out, tr("Установка pnpm для текущего пользователя…"))
			if err = runLocal(ctx, npm, []string{"install", "--prefix", prefix, "--no-audit", "--no-fund", "--ignore-scripts", "pnpm@10"}, options.CWD, env, out); err != nil {
				if options.Market == "install" || ctx.Err() != nil {
					return nil, errors.New(tr("не удалось установить pnpm"))
				}
				fmt.Fprintln(out, tr("Ошибка установки pnpm; можно повторить или пропустить маркет."))
				continue
			}
			if _, err := os.Stat(filepath.Join(toolsBin, "pnpm")); err != nil {
				return nil, errors.New(tr("pnpm не найден после установки"))
			}
			pnpmReady = true
			env = append(env, "PATH="+toolsBin+string(os.PathListSeparator)+os.Getenv("PATH"))
		}
		fmt.Fprintf(out, tr("Установка dshmarket в профиль %s…\n"), options.Profile)
		err := runLocal(ctx, options.Bin, []string{"plugin", "--profile", options.Profile, "add", "dshmarket"}, options.CWD, env, out)
		if err == nil && marketInstalled(options.Profile) {
			fmt.Fprintln(out, tr("dshmarket установлен."))
			return env, nil
		}
		if options.Market == "install" || ctx.Err() != nil {
			return nil, errors.New(tr("предустановка dshmarket не завершена"))
		}
		fmt.Fprintln(out, tr("Не удалось установить dshmarket; можно повторить или продолжить без него."))
	}
}

func startBackend(ctx context.Context, options backendOptions, env []string) (string, func(), error) {
	login, stop, _, err := startBackendLogged(ctx, options, env, io.Discard)
	return login, stop, err
}

type backendLogWriter struct {
	mu      sync.Mutex
	pending string
	out     io.Writer
	port    int
	ready   chan string
}

func (w *backendLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.out.Write(p)
	w.pending += string(p)
	for {
		at := strings.IndexByte(w.pending, '\n')
		if at < 0 {
			break
		}
		line := clean(w.pending[:at])
		w.pending = w.pending[at+1:]
		at = strings.Index(line, "dsh web: ")
		if at < 0 {
			continue
		}
		fields := strings.Fields(line[at+len("dsh web: "):])
		if len(fields) == 0 {
			continue
		}
		u, e := url.Parse(fields[0])
		if e == nil && u.Scheme == "http" && u.Host == fmt.Sprintf("127.0.0.1:%d", w.port) && u.Query().Get("token") != "" {
			select {
			case w.ready <- u.String():
			default:
			}
		}
	}
	if len(w.pending) > 1<<20 {
		w.pending = w.pending[len(w.pending)-(1<<20):]
	}
	return len(p), nil
}
func startBackendLogged(ctx context.Context, options backendOptions, env []string, out io.Writer) (string, func(), <-chan struct{}, error) {
	patch, err := prepareBackendModule()
	if err != nil {
		return "", nil, nil, fmt.Errorf(tr("Не удалось подготовить встроенный модуль dsh: %s"), clean(err.Error()))
	}
	command := exec.Command(options.Bin, "--profile", options.Profile, "--patch", patch, "--no-open", "--port", fmt.Sprint(options.Port))
	command.Dir = options.CWD
	command.Env = env
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	ready := make(chan string, 1)
	writer := &backendLogWriter{out: out, port: options.Port, ready: ready}
	// The child owns a regular file descriptor, so exiting this client cannot
	// break a stdout pipe while another frontend still uses the backend.
	spool, err := os.CreateTemp("", "dsh-backend-*.log")
	if err != nil {
		return "", nil, nil, err
	}
	defer os.Remove(spool.Name())
	command.Stdout = spool
	command.Stderr = spool
	if err := command.Start(); err != nil {
		spool.Close()
		return "", nil, nil, errors.New(tr("не удалось запустить dsh; проверьте путь и Node.js 24.9+"))
	}
	done := make(chan struct{})
	go func() { command.Wait(); close(done) }()
	logsDone := make(chan struct{})
	go func() {
		defer close(logsDone)
		defer spool.Close()
		var offset int64
		buffer := make([]byte, 32<<10)
		poll := func() {
			for {
				n, _ := spool.ReadAt(buffer, offset)
				if n == 0 {
					return
				}
				offset += int64(n)
				writer.Write(buffer[:n])
			}
		}
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				poll()
			case <-done:
				poll()
				return
			}
		}
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			// Never signal a recycled process group after the child has been reaped.
			select {
			case <-done:
				return
			default:
			}
			syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
			select {
			case <-done:
			case <-time.After(4 * time.Second):
				syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
				<-done
			}
		})
	}
	select {
	case value := <-ready:
		if err := rememberBackend(options.Port, command.Process.Pid); err != nil {
			stop()
			return "", nil, nil, err
		}
		return value, stop, done, nil
	case <-ctx.Done():
		stop()
		return "", nil, nil, ctx.Err()
	case <-done:
		<-logsDone
		return "", nil, nil, errors.New(tr("dsh завершился до запуска; откройте журнал"))
	case <-time.After(120 * time.Second):
		stop()
		return "", nil, nil, errors.New(tr("истекло время запуска dsh"))
	}
}

func checkBackendNode(ctx context.Context) error {
	output, err := exec.CommandContext(ctx, "node", "--version").Output()
	major, minor := 0, 0
	if err != nil {
		return errors.New(tr("для dsh нужен Node.js 24.9+; node не найден"))
	}
	if n, _ := fmt.Sscanf(strings.TrimSpace(string(output)), "v%d.%d", &major, &minor); n != 2 || major < 24 || (major == 24 && minor < 9) {
		return errors.New(tr("для dsh нужен Node.js 24.9+; обновите Node.js перед --start"))
	}
	return nil
}
