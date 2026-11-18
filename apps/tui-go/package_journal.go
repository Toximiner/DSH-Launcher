package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type packageReceipt struct {
	ID, Origin, Method, Target, State, Outcome string
	Updated                                    time.Time
}

func packageJournalDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "dsh-launcher-tui", "package-operations"), nil
}
func savePackageReceipt(r packageReceipt) error {
	if len(r.ID) != 32 || strings.ContainsAny(r.ID, "/\\.") {
		return errors.New("invalid operation id")
	}
	dir, err := packageJournalDir()
	if err != nil {
		return err
	}
	r.Updated = time.Now().UTC()
	return atomicJSON(filepath.Join(dir, r.ID+".json"), r)
}
func packageReceipts(origin string) ([]benchRow, error) {
	dir, err := packageJournalDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var receipts []packageReceipt
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Size() > 16<<10 {
			continue
		}
		var r packageReceipt
		if readJSON(filepath.Join(dir, e.Name()), &r) == nil && r.Origin == origin {
			receipts = append(receipts, r)
		}
	}
	sort.Slice(receipts, func(i, j int) bool { return receipts[i].Updated.After(receipts[j].Updated) })
	var rows []benchRow
	for _, r := range receipts[:min(len(receipts), 100)] {
		body := tr("Операция: ") + r.Method + tr("\nЗапрос: ") + r.ID + tr("\nПоследнее состояние: ") + r.State + "\n" + r.Outcome
		if r.State != "confirmed" {
			body += tr("\nРезультат неизвестен. Проверьте /packages и /runtime. Запрос автоматически не повторяется.")
		}
		rows = append(rows, benchRow{ID: r.ID, Title: clean(r.Target) + " · " + r.State, Body: clean(body)})
	}
	return rows, nil
}
