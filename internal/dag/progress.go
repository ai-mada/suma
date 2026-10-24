package dag

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var progressANSI = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var progressCredential = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*bearer\s+|api[_-]?key\s*[:=]\s*|token\s*[:=]\s*)[^\s]+`)

// Progress records actual execution output without changing the final output or
// state. Its receipt and event commit under the same scheduler transaction.
func (e *Engine) Progress(ctx context.Context, tenant, project string, a Assignment, operationID, message string) error {
	if err := scopeValid(tenant, project); err != nil {
		return err
	}
	if a.RunID == "" || a.NodeID == "" || a.WorkerID == "" || a.Attempt < 1 || a.Fence < 1 || strings.TrimSpace(message) == "" || len(message) > 8192 || !utf8.ValidString(message) {
		return fmt.Errorf("%w: progress identity and 1–8192 UTF-8 bytes required", ErrInvalid)
	}
	// Fingerprint the original request so a reused key cannot hide changed text
	// behind redaction. Only the hash and sanitized text are stored.
	hash := fingerprint(struct {
		RunID, NodeID, WorkerID string
		Attempt                 int
		Fence                   int64
		Message                 string
	}{a.RunID, a.NodeID, a.WorkerID, a.Attempt, a.Fence, message})
	message = progressANSI.ReplaceAllString(message, "")
	message = progressCredential.ReplaceAllString(message, "[REDACTED]")
	message = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, message)
	return e.write(ctx, func(t *transaction) error {
		scope := "progress:" + a.RunID
		previous, err := t.operation(tenant, project, scope, operationID, hash)
		if err != nil {
			return err
		}
		if previous != "" {
			return nil
		}
		r, _, err := t.assigned(tenant, project, a)
		if err != nil {
			return err
		}
		if err = t.event(r, a.NodeID, "node_progress", map[string]any{"operation_id": operationID, "message": message, "attempt": a.Attempt, "fence": a.Fence, "worker_id": a.WorkerID}); err != nil {
			return err
		}
		return t.remember(tenant, project, scope, operationID, hash, r.ID)
	})
}
