//go:build !js

package main

import (
	"errors"
	"io"
	"os"
	"syscall"
)

func readComposeFile(name string) ([]byte, error) {
	before, err := os.Lstat(name)
	if err != nil || !before.Mode().IsRegular() {
		return nil, errors.New("regular file required")
	}
	f, err := os.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 2<<20 {
		return nil, errors.New("regular file up to 2 MiB required")
	}
	data, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if len(data) > 2<<20 {
		return nil, errors.New("file exceeds cap")
	}
	return data, err
}
