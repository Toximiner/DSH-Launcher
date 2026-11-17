// Package backendassets shares the bundled backend source with the native client.
package backendassets

import "embed"

// Files contains the same dependency-free modules shipped with the desktop launcher.
//
//go:embed index.mjs session-delete.mjs
var Files embed.FS
