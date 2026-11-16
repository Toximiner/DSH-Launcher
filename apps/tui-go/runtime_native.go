//go:build !js

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const dshPackage = "@deepseek-ai/dsh"

var packageVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

type managedInstall struct{ Current, Previous string }
type runtimeConfig struct {
	Bin, Profile, CWD string
	Port              int
	Bell              bool
}
type nativeRuntime struct {
	owner            *os.File
	live             io.Writer
	livePending      string
	mu               sync.Mutex
	op               sync.Mutex
	state            runtimeSnapshot
	config           runtimeConfig
	root, executable string
	install          managedInstall
	stop             func()
	done             <-chan struct{}
	releases         updateCatalog
}

func runtimeRoot() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		h, _ := os.UserHomeDir()
		base = filepath.Join(h, ".local", "share")
	}
	return filepath.Join(base, "dsh-launcher-tui")
}
func newNativeRuntime(options backendOptions) (*nativeRuntime, error) {
	root := runtimeRoot()
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	c := &nativeRuntime{root: root, config: runtimeConfig{options.Bin, options.Profile, options.CWD, options.Port, false}}
	c.executable, _ = os.Executable()
	c.executable, _ = filepath.EvalSymlinks(c.executable)
	if err := readJSON(filepath.Join(root, "runtime.json"), &c.config); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New(tr("повреждены параметры запуска runtime.json"))
	}
	if err := readJSON(filepath.Join(root, "dsh-install.json"), &c.install); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf(tr("повреждено состояние установки dsh: %w"), err)
	}
	for _, slot := range []string{c.install.Current, c.install.Previous} {
		if slot != "" && !validSlot(slot) {
			return nil, errors.New("invalid installation slot")
		}
	}
	if previous, e := os.ReadFile(filepath.Join(root, "last-runtime.log")); e == nil {
		if len(previous) > 128<<10 {
			previous = previous[len(previous)-(128<<10):]
		}
		c.state.Log = tr("Предыдущий запуск:\n") + redactDiagnostic(string(previous)) + tr("\nТекущий запуск:\n")
	}
	c.state.Bell = c.config.Bell
	c.setStatus(tr("Управление dsh готово."), "")
	return c, nil
}
func validSlot(s string) bool {
	return filepath.Base(s) == s && strings.HasPrefix(s, "dsh-") && !strings.Contains(s, "..")
}
func readJSON(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func atomicJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return atomicFile(path, append(b, '\n'), 0600)
}
func atomicFile(path string, b []byte, mode os.FileMode) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".transaction-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e == nil {
		defer d.Close()
		e = d.Sync()
	}
	return e
}
func validateRuntimeConfig(c runtimeConfig) error {
	if c.Bin == "" || !profileName.MatchString(c.Profile) || c.Port < 1024 || c.Port > 65535 {
		return errors.New(tr("неверный путь, профиль или порт dsh"))
	}
	if !filepath.IsAbs(c.CWD) {
		return errors.New(tr("рабочая папка должна быть абсолютной"))
	}
	st, e := os.Stat(c.CWD)
	if e != nil || !st.IsDir() {
		return errors.New(tr("рабочая папка недоступна"))
	}
	return nil
}
func (c *nativeRuntime) selectedBin() string {
	if c.install.Current != "" {
		return filepath.Join(c.root, "versions", c.install.Current, "node_modules", ".bin", "dsh")
	}
	return c.config.Bin
}
func (c *nativeRuntime) setStatus(status, note string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.Status = redactDiagnostic(status)
	c.state.Note = redactDiagnostic(note)
	c.state.Revision++
	c.state.Detail = fmt.Sprintf(tr("Профиль: %s · порт: %d\nПапка: %s\ndsh: %s\nTUI: %s · %s/%s\nЗвуковые уведомления: %t"), c.config.Profile, c.config.Port, c.config.CWD, c.selectedBin(), version, runtime.GOOS, runtime.GOARCH, c.config.Bell) + "\n" + c.releases.Text
}
func (c *nativeRuntime) Snapshot() runtimeSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.state
	s.Log = redactDiagnostic(s.Log)
	s.Detail = redactDiagnostic(s.Detail)
	if s.Owned && c.done != nil {
		select {
		case <-c.done:
			s.Owned = false
			s.Exited = true
			s.Login = ""
			s.Status = tr("Процесс dsh завершился.")
		default:
		}
	}
	return s
}
func (c *nativeRuntime) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.Log += string(p)
	if c.live != nil {
		c.livePending += string(p)
		for {
			i := strings.IndexByte(c.livePending, '\n')
			if i < 0 {
				break
			}
			fmt.Fprintln(c.live, redactDiagnostic(c.livePending[:i]))
			c.livePending = c.livePending[i+1:]
		}
	}

	if len(c.state.Log) > 256<<10 {
		c.state.Log = c.state.Log[len(c.state.Log)-(256<<10):]
	}
	// Redaction happens on complete snapshots, not write boundaries.
	return len(p), nil
}
func (c *nativeRuntime) persistLog() {
	if c.logSnapshot() == "" {
		return
	}
	atomicFile(filepath.Join(c.root, "last-runtime.log"), []byte(c.logSnapshot()), 0600)
}
func (c *nativeRuntime) Notice(message string) {
	fmt.Fprintln(c, tr("[Уведомление] ")+message)
	c.mu.Lock()
	bell := c.state.Bell
	c.mu.Unlock()
	if bell {
		os.Stdout.Write([]byte{'\a'})
	}
}
func (c *nativeRuntime) logSnapshot() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return redactDiagnostic(c.state.Log)
}
func (c *nativeRuntime) Close() {
	c.op.Lock()
	defer c.op.Unlock()
	c.persistLog()
	c.stopOwned()
	if c.owner != nil {
		c.owner.Close()
		c.owner = nil
	}
}
func (c *nativeRuntime) stopOwned() {
	c.mu.Lock()
	stop := c.stop
	c.stop = nil
	c.state.Owned = false
	c.state.Login = ""
	c.done = nil
	c.mu.Unlock()
	if stop != nil {
		stop()
	}
}
func (c *nativeRuntime) adopt(login string, stop func(), done <-chan struct{}) {
	c.mu.Lock()
	c.stop = stop
	c.done = done
	c.state.Owned = true
	c.state.Exited = false
	c.state.Login = login
	c.mu.Unlock()
}
func (c *nativeRuntime) start(ctx context.Context) error {
	if c.Snapshot().Owned {
		return errors.New(tr("свой dsh уже работает"))
	}
	if err := checkBackendNode(ctx); err != nil {
		return err
	}
	if err := validateRuntimeConfig(c.config); err != nil {
		return err
	}
	options := backendOptions{Bin: c.selectedBin(), Profile: c.config.Profile, CWD: c.config.CWD, Port: c.config.Port, Market: "skip"}
	login, stop, done, err := startBackendLogged(ctx, options, os.Environ(), c)
	if err != nil {
		return err
	}
	c.adopt(login, stop, done)
	return nil
}
func (c *nativeRuntime) Execute(ctx context.Context, action, value string) (err error) {
	if !c.op.TryLock() {
		return errors.New(tr("операция уже выполняется"))
	}
	defer c.op.Unlock()
	if c.owner == nil {
		c.owner, err = lockRuntime(c.root)
		if err != nil {
			return err
		}
	}
	defer func() {
		if !c.Snapshot().Owned && c.owner != nil {
			c.owner.Close()
			c.owner = nil
		}
	}()

	var saved managedInstall
	if e := readJSON(filepath.Join(c.root, "dsh-install.json"), &saved); e == nil {
		if (saved.Current != "" && !validSlot(saved.Current)) || (saved.Previous != "" && !validSlot(saved.Previous)) {
			return errors.New("invalid installation state")
		}
		c.install = saved
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	c.mu.Lock()
	c.state.Busy = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.state.Busy = false
		c.mu.Unlock()
		if err != nil {
			c.setStatus(tr("Ошибка операции · можно повторить"), err.Error())
		} else {
			c.setStatus(tr("Готово · ")+runtimeActionLabel(action), tr("Операция завершена. Подробности — в журнале."))
		}
		c.Notice(c.Snapshot().Status)
		c.persistLog()
	}()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	fmt.Fprintf(c, "\n[%s] %s\n", time.Now().UTC().Format(time.RFC3339), action)
	switch action {
	case "probe":
		if err = checkBackendNode(ctx); err != nil {
			return err
		}
		b, e := exec.CommandContext(ctx, c.selectedBin(), "--version").CombinedOutput()
		c.Write(b)
		if e != nil {
			return errors.New(tr("dsh не найден или не запускается; выберите установку"))
		}
	case "install", "update-dsh":
		return c.installDsh(ctx, value)
	case "start":
		return c.start(ctx)
	case "stop":
		if c.stop == nil {
			return errors.New(tr("нет своего процесса; внешний dsh не останавливается"))
		}
		c.stopOwned()
	case "restart":
		if c.stop == nil {
			return errors.New(tr("нет своего процесса для перезапуска"))
		}
		c.stopOwned()
		return c.start(ctx)
	case "rollback-dsh":
		if c.Snapshot().Owned {
			return errors.New(tr("сначала остановите свой dsh"))
		}
		if c.install.Previous == "" {
			if c.install.Current == "" {
				return errors.New(tr("предыдущая установка отсутствует"))
			}
			if e := verifyDsh(ctx, c.config.Bin); e != nil {
				return errors.New(tr("до первой установки не было доступного dsh; возвращать нечего"))
			}
			next := managedInstall{Previous: c.install.Current}
			if e := atomicJSON(filepath.Join(c.root, "dsh-install.json"), next); e != nil {
				return e
			}
			c.install = next
			return nil
		}
		next := managedInstall{c.install.Previous, c.install.Current}
		if err = verifyDsh(ctx, filepath.Join(c.root, "versions", next.Current, "node_modules", ".bin", "dsh")); err != nil {
			return err
		}
		if err = atomicJSON(filepath.Join(c.root, "dsh-install.json"), next); err == nil {
			c.install = next
		}
		return err
	case "bin", "profile", "port", "cwd":
		if c.Snapshot().Owned {
			return errors.New(tr("сначала остановите свой dsh"))
		}
		next := c.config
		switch action {
		case "bin":
			next.Bin = strings.TrimSpace(value)
		case "profile":
			next.Profile = strings.TrimSpace(value)
		case "port":
			next.Port, _ = strconv.Atoi(value)
		case "cwd":
			next.CWD = value
		}
		if err = validateRuntimeConfig(next); err != nil {
			return err
		}
		if err = atomicJSON(filepath.Join(c.root, "runtime.json"), next); err == nil {
			c.config = next
		}
		return err
	case "check":
		if b, e := exec.CommandContext(ctx, c.selectedBin(), "--version").CombinedOutput(); e == nil {
			fmt.Fprintf(c, tr("Установлен dsh: %s\n"), strings.TrimSpace(string(b)))
		}
		c.releases, err = fetchUpdateCatalog(ctx)
		if err == nil {
			fmt.Fprintln(c, c.releases.Text)
		}
		return err
	case "update-tui":
		if c.releases.TUI.URL == "" {
			return errors.New(tr("сначала проверьте версии; в релизе нужен TUI-бинарник с SHA-256"))
		}
		return c.updateTUI(ctx, c.releases.TUI)
	case "local-tui":
		parts := strings.Fields(value)
		if len(parts) < 2 {
			return errors.New(tr("укажите абсолютный путь и SHA-256 через пробел"))
		}
		return c.updateTUI(ctx, updateArtifact{Path: strings.Join(parts[:len(parts)-1], " "), SHA256: parts[len(parts)-1]})
	case "rollback-tui":
		return c.rollbackTUI(ctx)
	case "report":
		dest, e := c.report(value)
		if e != nil {
			return e
		}
		fmt.Fprintln(c, tr("Отчёт сохранён: ")+dest)
	case "logs":
		return nil
	case "bell":
		next := c.config
		next.Bell = !next.Bell
		if err = atomicJSON(filepath.Join(c.root, "runtime.json"), next); err == nil {
			c.config = next
			c.mu.Lock()
			c.state.Bell = next.Bell
			c.mu.Unlock()
		}
		return err
	default:
		return errors.New(tr("неизвестная операция"))
	}
	return nil
}
func runtimeActionLabel(action string) string {
	for _, group := range [][]runtimeAction{runtimeActions, updateActions, diagnosticActions} {
		for _, a := range group {
			if a.id == action {
				return a.label
			}
		}
	}
	return action
}
func verifyDsh(ctx context.Context, bin string) error {
	output, e := exec.CommandContext(ctx, bin, "--version").CombinedOutput()
	if e != nil || !regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`).Match(output) {
		return errors.New(tr("проверка установленного dsh не прошла"))
	}
	return nil
}
func (c *nativeRuntime) installDsh(ctx context.Context, v string) error {
	if c.Snapshot().Owned {
		return errors.New(tr("сначала остановите свой dsh; исполняющиеся задачи не переживают перезапуск"))
	}
	if e := checkBackendNode(ctx); e != nil {
		return e
	}
	if v != "latest" && !packageVersion.MatchString(v) {
		return errors.New(tr("нужна точная версия npm или latest"))
	}
	// Resolve tags before installation so the log and immutable slot identify the
	// exact candidate. A failed or interrupted install never changes the pointer.
	if v == "latest" {
		b, e := exec.CommandContext(ctx, "npm", "view", dshPackage+"@latest", "version", "--json").Output()
		if e != nil {
			return errors.New(tr("не удалось получить версию из npm"))
		}
		if json.Unmarshal(b, &v) != nil || !packageVersion.MatchString(v) {
			return errors.New(tr("неверная версия npm"))
		}
	}
	dir := filepath.Join(c.root, "versions")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	slot, e := os.MkdirTemp(dir, "dsh-"+v+"-")
	if e != nil {
		return e
	}
	committed := false
	defer func() {
		if !committed {
			os.RemoveAll(slot)
		}
	}()
	fmt.Fprintln(c, tr("Установка dsh ")+v+"…")
	if e = runLocal(ctx, "npm", []string{"install", "--prefix", slot, "--no-audit", "--no-fund", "--ignore-scripts", dshPackage + "@" + v}, c.config.CWD, os.Environ(), c); e != nil {
		return errors.New(tr("установка npm не завершена; прежняя версия сохранена (см. журнал)"))
	}
	if e = verifyDsh(ctx, filepath.Join(slot, "node_modules", ".bin", "dsh")); e != nil {
		return e
	}
	var metadata struct {
		Version string `json:"version"`
	}
	if e = readJSON(filepath.Join(slot, "node_modules", "@deepseek-ai", "dsh", "package.json"), &metadata); e != nil || metadata.Version != v {
		return errors.New(tr("версия установленного пакета не совпадает с запрошенной; прежняя версия сохранена"))
	}
	next := managedInstall{filepath.Base(slot), c.install.Current}
	if e = atomicJSON(filepath.Join(c.root, "dsh-install.json"), next); e != nil {
		return e
	}
	c.install = next
	committed = true
	fmt.Fprintln(c, tr("Активирована управляемая версия ")+v+tr(". Профиль не изменён."))
	return nil
}

// A filesystem lock excludes concurrent installers and process owners across
// clients. The descriptor stays open for the lifetime of the native controller.
func lockRuntime(root string) (*os.File, error) {
	f, e := os.OpenFile(filepath.Join(root, "owner.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New(tr("другой TUI уже управляет установкой dsh"))
	}
	return f, nil
}

var _ io.Writer = (*nativeRuntime)(nil)
