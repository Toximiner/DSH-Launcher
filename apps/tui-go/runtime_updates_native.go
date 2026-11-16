//go:build !js

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type updateArtifact struct{ URL, Path, SHA256, Version string }
type updateCatalog struct {
	TUI  updateArtifact
	Text string
}
type tuiTransaction struct{ PreviousHash, CandidateHash, PreviousVersion, CandidateVersion string }

func lockTUIExecutable(path string) (*os.File, error) {
	f, e := os.OpenFile(path+".update.lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New(tr("другая операция уже обновляет этот TUI"))
	}
	return f, nil
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, limit+1))
	if e == nil && int64(len(b)) > limit {
		e = errors.New(tr("превышен допустимый размер загрузки"))
	}
	return b, e
}
func httpsBytes(ctx context.Context, address string, limit int64) ([]byte, error) {
	u, e := url.Parse(address)
	if e != nil || u.Scheme != "https" || u.User != nil {
		return nil, errors.New(tr("для загрузки нужен HTTPS"))
	}
	client := &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || len(via) > 5 {
			return errors.New(tr("небезопасное перенаправление"))
		}
		return nil
	}}
	req, e := http.NewRequestWithContext(ctx, "GET", address, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", "DSH-Launcher-TUI/"+version)
	res, e := client.Do(req)
	if e != nil {
		return nil, errors.New(tr("не удалось загрузить данные обновления"))
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(tr("сервер обновлений: HTTP %d"), res.StatusCode)
	}
	return readLimited(res.Body, limit)
}
func fetchUpdateCatalog(ctx context.Context) (updateCatalog, error) {
	catalog := updateCatalog{}
	var latest struct{ Version string }
	var notes []struct {
		Tag   string `json:"tag_name"`
		Body  string
		Draft bool
	}
	b, e := httpsBytes(ctx, "https://registry.npmjs.org/@deepseek-ai%2fdsh/latest", 4<<20)
	if e != nil {
		return catalog, e
	}
	if json.Unmarshal(b, &latest) != nil || !packageVersion.MatchString(latest.Version) {
		return catalog, errors.New(tr("неверный ответ npm"))
	}
	catalog.Text = "dsh npm latest: " + latest.Version + tr("\nУстановка из раздела обновления dsh.\n")
	b, e = httpsBytes(ctx, "https://api.github.com/repos/deepseek-ai/deepseek-harness/releases?per_page=10", 4<<20)
	if e == nil && json.Unmarshal(b, &notes) == nil {
		for _, r := range notes {
			if !r.Draft && r.Tag == "dsh-v"+latest.Version {
				catalog.Text += tr("\nИзменения dsh:\n") + r.Body + "\n"
				break
			}
		}
	}
	var releases []struct {
		Tag    string `json:"tag_name"`
		Body   string
		Draft  bool
		Assets []struct {
			Name, Digest string
			URL          string `json:"browser_download_url"`
		}
	}
	b, e = httpsBytes(ctx, "https://api.github.com/repos/Toximiner/DSH-Launcher/releases?per_page=30", 8<<20)
	if e != nil {
		catalog.Text += tr("\nРелизы TUI недоступны: ") + e.Error()
		return catalog, nil
	}
	if json.Unmarshal(b, &releases) != nil {
		return catalog, errors.New(tr("неверный каталог релизов TUI"))
	}
	name := "dsh-launcher-tui-go_linux_" + runtime.GOARCH
	for _, r := range releases {
		if r.Draft || !strings.HasPrefix(r.Tag, "tui-v") {
			continue
		}
		for _, a := range r.Assets {
			if a.Name != name || !strings.HasPrefix(a.Digest, "sha256:") {
				continue
			}
			catalog.TUI = updateArtifact{URL: a.URL, SHA256: strings.TrimPrefix(a.Digest, "sha256:"), Version: strings.TrimPrefix(r.Tag, "tui-v")}
			catalog.Text += tr("\nРелиз TUI: ") + r.Tag + "\n" + r.Body
			return catalog, nil
		}
	}
	catalog.Text += tr("\nВ релизах пока нет TUI-бинарника для ") + runtime.GOARCH + tr(" с SHA-256.\nМожно установить проверенный локальный бинарник. GUI .deb не подходит.")
	return catalog, nil
}
func hashBytes(b []byte) string { v := sha256.Sum256(b); return hex.EncodeToString(v[:]) }
func binaryVersion(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, path, "--version").Output()
	v := strings.TrimSpace(string(b))
	if e != nil || !packageVersion.MatchString(v) {
		return "", errors.New(tr("новый TUI не прошёл проверку --version"))
	}
	return v, nil
}
func (c *nativeRuntime) updateTUI(ctx context.Context, a updateArtifact) error {
	lock, e := lockTUIExecutable(c.executable)
	if e != nil {
		return e
	}
	defer lock.Close()
	digest, e := hex.DecodeString(a.SHA256)
	if e != nil || len(digest) != sha256.Size {
		return errors.New(tr("нужен SHA-256: 64 шестнадцатеричных символа"))
	}
	var b []byte
	if a.Path != "" {
		if !filepath.IsAbs(a.Path) {
			return errors.New(tr("нужен абсолютный путь к бинарнику"))
		}
		f, err := os.Open(a.Path)
		if err != nil {
			return err
		}
		b, e = readLimited(f, 128<<20)
		f.Close()
	} else {
		b, e = httpsBytes(ctx, a.URL, 128<<20)
	}
	if e != nil {
		return e
	}
	if !strings.EqualFold(hashBytes(b), a.SHA256) {
		return errors.New(tr("SHA-256 не совпадает; TUI не изменён"))
	}
	// Stage on the same filesystem as the executable; rename is atomic even
	// though the old inode is currently executing. Never truncate that inode.
	candidate, e := os.CreateTemp(filepath.Dir(c.executable), ".tui-candidate-*")
	if e != nil {
		return errors.New(tr("нет права записи рядом с TUI; установите бинарник в пользовательский каталог"))
	}
	path := candidate.Name()
	defer os.Remove(path)
	if e = candidate.Chmod(0700); e == nil {
		_, e = candidate.Write(b)
	}
	if e == nil {
		e = candidate.Sync()
	}
	closeErr := candidate.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	v, e := binaryVersion(ctx, path)
	if e != nil {
		return e
	}
	if a.Version != "" && v != a.Version {
		return errors.New(tr("версия бинарника отличается от версии релиза"))
	}
	old, e := os.ReadFile(c.executable)
	if e != nil {
		return e
	}
	if hashBytes(old) == hashBytes(b) {
		return errors.New(tr("эта сборка TUI уже установлена; резервная версия сохранена"))
	}
	oldV, e := binaryVersion(ctx, c.executable)
	if e != nil {
		return e
	}
	// Both the backup and its checksummed journal are durable before promotion.
	if e = atomicFile(c.executable+".previous", old, 0700); e != nil {
		return e
	}
	txn := tuiTransaction{hashBytes(old), hashBytes(b), oldV, v}
	if e = atomicJSON(c.executable+".update.json", txn); e != nil {
		return e
	}
	if e = os.Rename(path, c.executable); e != nil {
		return e
	}
	if _, e = binaryVersion(ctx, c.executable); e != nil {
		if restore := atomicFile(c.executable, old, 0700); restore != nil {
			return fmt.Errorf(tr("проверка обновления и восстановление не удались: %w"), restore)
		}
		return errors.New(tr("обновление не прошло проверку; прежний TUI восстановлен"))
	}
	fmt.Fprintln(c, "TUI "+v+tr(" установлен. Перезапустите клиент; предыдущая версия сохранена."))
	return nil
}
func (c *nativeRuntime) rollbackTUI(ctx context.Context) error {
	lock, e := lockTUIExecutable(c.executable)
	if e != nil {
		return e
	}
	defer lock.Close()
	var txn tuiTransaction
	if e := readJSON(c.executable+".update.json", &txn); e != nil {
		return errors.New(tr("нет сохранённой транзакции обновления TUI"))
	}
	old, e := os.ReadFile(c.executable + ".previous")
	if e != nil {
		return e
	}
	if hashBytes(old) != txn.PreviousHash {
		return errors.New(tr("резервный TUI повреждён; восстановление отменено"))
	}
	if _, e = binaryVersion(ctx, c.executable+".previous"); e != nil {
		return e
	}
	if e = atomicFile(c.executable, old, 0700); e != nil {
		return e
	}
	fmt.Fprintln(c, tr("Восстановлен TUI ")+txn.PreviousVersion+tr(". Перезапустите клиент."))
	return nil
}
