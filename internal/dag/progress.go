package dag

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ai-mada/suma/internal/toolcontract"
	"regexp"
	"strings"
	"unicode/utf8"
)

var progressANSI = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var progressCredential = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*bearer\s+|api[_-]?key\s*[:=]\s*|token\s*[:=]\s*)[^\s]+`)

// Progress records actual execution output without changing the final output or
// state. Its receipt and event commit under the same scheduler transaction.
func (e *Engine) Progress(ctx context.Context, tenant, project string, a Assignment, operationID, message string, toolEvents ...*toolcontract.Event) error {
	var toolEvent *toolcontract.Event
	if len(toolEvents) > 1 {
		return dagDiagnostic(fmt.Errorf("%w: one tool event permitted", ErrInvalid), "dag.progress.event_count", nil)
	}
	if len(toolEvents) == 1 {
		toolEvent = toolEvents[0]
	}
	return e.ProgressWithAuthority(ctx, tenant, project, a, operationID, message, toolEvent, nil)
}

func (e *Engine) ProgressWithAuthority(ctx context.Context, tenant, project string, a Assignment, operationID, message string, toolEvent *toolcontract.Event, before func(*sql.Tx) error) error {
	if err := scopeValid(tenant, project); err != nil {
		return err
	}
	if a.RunID == "" || a.NodeID == "" || a.WorkerID == "" || a.Attempt < 1 || a.Fence < 1 || strings.TrimSpace(message) == "" && toolEvent == nil || len(message) > 8192 || !utf8.ValidString(message) {
		return dagDiagnostic(fmt.Errorf("%w: progress identity and 1–8192 UTF-8 bytes required", ErrInvalid), "dag.progress.identity", nil)
	}
	if toolEvent != nil {
		if err := toolcontract.ValidateEvent(*toolEvent); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalid, err)
		}
	}
	// Fingerprint the original request so a reused key cannot hide changed text
	// behind redaction. Only the hash and sanitized text are stored.
	hash := fingerprint(struct {
		RunID, NodeID, WorkerID string
		Attempt                 int
		Fence                   int64
		Message                 string
		ToolEvent               *toolcontract.Event
	}{a.RunID, a.NodeID, a.WorkerID, a.Attempt, a.Fence, message, toolEvent})
	message = progressANSI.ReplaceAllString(message, "")
	message = progressCredential.ReplaceAllString(message, "[REDACTED]")
	message = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, message)
	return e.writeBefore(ctx, before, func(t *transaction) error {
		scope := "progress:" + a.RunID
		previous, err := t.operation(tenant, project, scope, operationID, hash)
		if err != nil {
			return err
		}
		if previous != "" {
			return nil
		}
		r, n, err := t.assigned(tenant, project, a)
		if err != nil {
			return err
		}
		if toolEvent != nil {
			if err = validateToolEvent(n.spec, *toolEvent); err != nil {
				return err
			}
			clean := *toolEvent
			clean.Message = progressCredential.ReplaceAllString(progressANSI.ReplaceAllString(clean.Message, ""), "[REDACTED]")
			if err = t.event(r, a.NodeID, "tool_progress", map[string]any{"operation_id": operationID, "attempt": a.Attempt, "fence": a.Fence, "worker_id": a.WorkerID, "tool_event": clean}); err != nil {
				return err
			}
		}
		if strings.TrimSpace(message) != "" {
			if err = t.event(r, a.NodeID, "node_progress", map[string]any{"operation_id": operationID, "message": message, "attempt": a.Attempt, "fence": a.Fence, "worker_id": a.WorkerID}); err != nil {
				return err
			}
		}
		return t.remember(tenant, project, scope, operationID, hash, r.ID)
	})
}
