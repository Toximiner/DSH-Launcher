//go:build !js

package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

func (c *nativeRuntime) report(destination string) (string, error) {
	if destination == "" {
		destination = filepath.Join(c.root, "reports")
	}
	if !filepath.IsAbs(destination) {
		return "", fmt.Errorf("нужен абсолютный путь к папке отчётов")
	}
	if err := os.MkdirAll(destination, 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(destination, "dsh-tui-report-*.tar.gz")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(f.Name())
		}
	}()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	add := func(name, text string) error {
		b := []byte(redactDiagnostic(text))
		if e := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(b)), ModTime: time.Now().UTC()}); e != nil {
			return e
		}
		_, e := tw.Write(b)
		return e
	}
	s := c.Snapshot()
	facts := fmt.Sprintf("TUI %s\nGo %s\nOS %s/%s\nCreated %s\n%s\n%s\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH, time.Now().UTC().Format(time.RFC3339), s.Status, s.Detail)
	config, _ := json.MarshalIndent(c.config, "", "  ")
	for _, entry := range [][2]string{{"info.txt", facts}, {"runtime.json", string(config)}, {"dsh-and-operations.log", c.logSnapshot()}, {"README.txt", "Report contains launcher runtime options and the bounded output of its own dsh/npm processes. No dsh profile, credentials, environment, session history or conversation is collected. Review logs before sharing: arbitrary application output can contain sensitive text not recognizable as a credential.\n"}} {
		if err = add(entry[0], entry[1]); err != nil {
			return "", err
		}
	}
	if err = tw.Close(); err != nil {
		return "", err
	}
	if err = gz.Close(); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	ok = true
	return f.Name(), nil
}
