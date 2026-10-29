//go:build windows

package main

import (
	"github.com/ai-mada/suma/internal/terminalruntime"
	"os"
)

// Opt this executable into the isolated PTY bootstrap before the normal Runner
// flags or credentials are read. The parent has already attached this process
// to its Job; clearing console inheritance occurs only in this child process.
func init() {
	if handled, code := terminalruntime.HandleChildProcess(os.Args[1:]); handled {
		os.Exit(code)
	}
}
