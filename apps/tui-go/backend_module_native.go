//go:build !js

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	backendassets "github.com/Toximiner/DSH-Launcher/packages/launcher-backend"
)

// A content-addressed, private directory keeps an owned backend's modules valid
// after the frontend exits or a newer launcher is installed. No profile is edited.
func prepareBackendModule() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	entries, err := fs.ReadDir(backendassets.Files, ".")
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	for _, entry := range entries {
		data, err := backendassets.Files.ReadFile(entry.Name())
		if err != nil {
			return "", err
		}
		hash.Write([]byte(entry.Name() + "\x00"))
		hash.Write(data)
	}
	parent := filepath.Join(base, "dsh-launcher", "backend")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	destination := filepath.Join(parent, hex.EncodeToString(hash.Sum(nil)))
	// Absolute names avoid profile resolution ambiguity. JSON is also valid YAML.
	patch := []any{map[string]any{"insert": []any{map[string]any{"id": "dsh-launcher-session-management", "name": filepath.Join(destination, "index.mjs")}}}}
	data, err := json.Marshal(patch)
	if err != nil {
		return "", err
	}
	path := filepath.Join(destination, "launcher.patch.json")
	validate := func() error {
		info, err := os.Lstat(destination)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("invalid backend module directory")
		}
		for _, entry := range entries {
			path := filepath.Join(destination, entry.Name())
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return errors.New("invalid backend module asset")
			}
			actual, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			expected, _ := backendassets.Files.ReadFile(entry.Name())
			if !bytes.Equal(actual, expected) {
				return errors.New("backend module asset differs from bundled source")
			}
		}
		info, err = os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("invalid backend module overlay")
		}
		actual, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(actual, data) {
			return errors.New("backend module overlay differs from bundled source")
		}
		return nil
	}
	if _, err := os.Lstat(destination); os.IsNotExist(err) {
		stage, err := os.MkdirTemp(parent, ".module-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(stage)
		for _, entry := range entries {
			data, _ := backendassets.Files.ReadFile(entry.Name())
			if err := os.WriteFile(filepath.Join(stage, entry.Name()), data, 0600); err != nil {
				return "", err
			}
		}
		if err := os.WriteFile(filepath.Join(stage, "launcher.patch.json"), data, 0600); err != nil {
			return "", err
		}
		if err := os.Rename(stage, destination); err != nil {
			if verify := validate(); verify != nil {
				return "", err
			}
		}
	}
	if err := validate(); err != nil {
		return "", err
	}
	return path, nil
}
