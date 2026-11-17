//go:build !js

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const savedSessionTabsLimit = 256 << 10

// Login credentials, query parameters and URL paths never enter the scope key.
// Relative DSH_HOME follows the working directory used to launch the backend.
func tabPersistencePath(login, profile, cwd string) (string, error) {
	u, err := url.Parse(login)
	if err != nil || u == nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", errors.New("invalid tab persistence origin")
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	origin := strings.ToLower(u.Scheme) + "://" + net.JoinHostPort(host, port)
	if profile == "" {
		profile = "web"
	}
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	home := os.Getenv("DSH_HOME")
	if home == "" {
		base, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		home = filepath.Join(base, ".dsh")
	}
	if !filepath.IsAbs(home) {
		home = filepath.Join(cwd, home)
	}
	home = filepath.Clean(home)
	if resolved, e := filepath.EvalSymlinks(home); e == nil {
		home = resolved
	}
	scope := sha256.Sum256([]byte("dsh-tui-tabs-scope-v1\x00" + origin + "\x00" + profile + "\x00" + home))
	prefs, err := clientPreferencesPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(prefs), "tabs", hex.EncodeToString(scope[:])+".json"), nil
}

func validateStoredSessionTabs(value savedSessionTabs) error {
	if value.Version != 1 {
		return errors.New("unsupported saved session tabs file version")
	}
	return validateSavedSessionTabs(value)
}

func readSavedSessionTabs(path string) (savedSessionTabs, error) {
	var value savedSessionTabs
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, savedSessionTabsLimit+1))
	if err != nil {
		return value, err
	}
	if len(data) > savedSessionTabsLimit || !utf8.Valid(data) {
		return value, errors.New("saved session tabs exceed the file limit or contain invalid text")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&value); err != nil {
		return savedSessionTabs{}, err
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return savedSessionTabs{}, errors.New("unexpected data after saved session tabs")
	}
	if err = validateStoredSessionTabs(value); err != nil {
		return savedSessionTabs{}, err
	}
	return value, nil
}

func writeSavedSessionTabs(path string, value savedSessionTabs) error {
	if err := validateStoredSessionTabs(value); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data)+1 > savedSessionTabsLimit {
		return errors.New("saved session tabs exceed the file limit")
	}
	directory := filepath.Dir(path)
	if err = os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	if err = os.Chmod(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".tabs-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(append(data, '\n'))
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(file.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr = dir.Close()
	if syncErr != nil {
		return fmt.Errorf("sync saved session tab directory: %w", syncErr)
	}
	return closeErr
}
