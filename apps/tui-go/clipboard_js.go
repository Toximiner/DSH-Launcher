//go:build js && wasm

package main

import (
	"context"
	"errors"
)

func writeClipboard(context.Context, string) (string, error) {
	return "", errors.New("В браузерном демо используйте выделение и копирование браузера.")
}
