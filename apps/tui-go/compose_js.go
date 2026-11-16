//go:build js && wasm

package main

import "errors"

func readComposeFile(name string) ([]byte, error) {
	return nil, errors.New("native file access unavailable")
}
