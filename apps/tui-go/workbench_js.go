//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"errors"
)

func firstBenchFrame(ctx context.Context, api *dshAPI, method string, args map[string]any) (json.RawMessage, error) {
	return nil, errors.New("backend unavailable in browser demo")
}

func readBenchJob(ctx context.Context, api *dshAPI, session, id string) (string, error) {
	return "", errors.New("backend unavailable in browser demo")
}
