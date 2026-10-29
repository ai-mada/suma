//go:build windows

package main

import (
	"github.com/ai-mada/suma/internal/terminalruntime"
	"os"
)

// The local combined service is also a PTY host. Keep the bootstrap in its own
// entrypoint companion so ordinary service startup and desktop lifecycle flags
// remain unchanged; no model or service credentials are loaded by this mode.
func init() {
	if handled, code := terminalruntime.HandleChildProcess(os.Args[1:]); handled {
		os.Exit(code)
	}
}
