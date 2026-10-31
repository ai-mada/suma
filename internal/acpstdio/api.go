// Package acpstdio owns one strict ACP SDK session over caller-owned process
// pipes. It never starts a process, retries a prompt or supplies filesystem I/O.
package acpstdio

import (
	"context"
	"fmt"
	"io"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type Options struct {
	Cwd, Goal, Model, ConfigSHA string
	Secrets                     []string
	Emit                        func(Event) error
	Permission                  func(context.Context, ac.PermissionRequest) (ac.PermissionDecision, error)
}
type Event struct{ Kind, SessionID, Text, ToolCallID, ToolState string }
type Result struct {
	Evidence             ac.Evidence
	Terminal, UsageState string
	Usage                *tc.Usage
	Error                error
}

func (Options) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "acpstdio.Options{redacted}") }
func (Event) Format(s fmt.State, _ rune)   { _, _ = io.WriteString(s, "acpstdio.Event{redacted}") }
func (Result) Format(s fmt.State, _ rune)  { _, _ = io.WriteString(s, "acpstdio.Result{redacted}") }
func (*Driver) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "acpstdio.Driver{redacted}") }

// New returns before any peer bytes are needed. CopyInput pumps SDK output to
// process stdin; Write receives process stdout. Finish is called after real
// stdout EOF/process Wait. Close affects local pipes, never sends Cancel.
func New(ctx context.Context, options Options) (*Driver, error) { return newDriver(ctx, options) }
