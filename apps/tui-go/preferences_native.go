//go:build !js

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func clientPreferencesPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "dsh-launcher-tui", "preferences.json"), nil
}

func readClientPreferences(path string) (clientPreferences, error) {
	prefs := defaultClientPreferences()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return prefs, nil
	}
	if err != nil {
		return prefs, err
	}
	if err = json.Unmarshal(data, &prefs); err != nil {
		return defaultClientPreferences(), err
	}
	return prefs, nil
}

func writeClientPreferences(path string, prefs clientPreferences) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".preferences-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = json.NewEncoder(file).Encode(prefs); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}
