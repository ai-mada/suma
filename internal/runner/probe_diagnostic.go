package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
)

// The internal cause supports errors.Is without making stderr, paths or
// credentials part of Error() or the public installation observation.
type toolProbeFailure struct {
	stage, code string
	cause       error
}

func (e *toolProbeFailure) Error() string { return "read-only CLI probe: " + e.stage + "/" + e.code }
func (e *toolProbeFailure) Unwrap() error { return e.cause }
func newProbeFailure(stage, code string, cause error) error {
	var prior *toolProbeFailure
	if errors.As(cause, &prior) {
		return cause
	}
	if code != "stop_unverified" && errors.Is(cause, os.ErrPermission) {
		code = "permission_denied"
	}
	d := ii.ProbeDiagnostic{Stage: stage, Code: code, NextAction: ii.ProbeDiagnosticNextAction(code)}
	if ii.ValidateProbeDiagnostic(d) != nil {
		return &toolProbeFailure{stage: "protocol", code: "probe_failed", cause: cause}
	}
	return &toolProbeFailure{stage: stage, code: code, cause: cause}
}
func probeDiagnostic(err error) *ii.ProbeDiagnostic {
	var p *toolProbeFailure
	if !errors.As(err, &p) {
		return nil
	}
	d := ii.ProbeDiagnostic{Stage: p.stage, Code: p.code, NextAction: ii.ProbeDiagnosticNextAction(p.code)}
	if ii.ValidateProbeDiagnostic(d) != nil {
		return nil
	}
	return &d
}
func cloneProbeDiagnostic(d *ii.ProbeDiagnostic) *ii.ProbeDiagnostic {
	if d == nil {
		return nil
	}
	copy := *d
	return &copy
}
func incompleteLocalResolution(paths []string, binary, packageName string) *ii.ProbeDiagnostic {
	for _, dir := range paths {
		name := binary
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		fixed := []string{dir, filepath.Join(dir, name)}
		if packageName != "" {
			fixed = append(fixed, filepath.Join(dir, "node_modules", filepath.FromSlash(packageName)))
		}
		for _, path := range fixed {
			if _, err := os.Lstat(path); err != nil && !os.IsNotExist(err) {
				return probeDiagnostic(newProbeFailure("resolve", "source_unavailable", err))
			}
		}
	}
	return nil
}
func probeStage(args []string) string {
	for _, a := range args {
		if a == "--version" {
			return "version"
		}
		if a == "--help" {
			return "help"
		}
	}
	return "protocol"
}
func probeProcessFailure(ctx context.Context, stage string, waitErr, stopErr error, truncated bool) error {
	// Stop uncertainty always takes precedence over a timeout/retry suggestion.
	if stopErr != nil {
		// A typed prelaunch/source failure did not acquire a process. A real
		// Stop/Finish failure is never downgraded to a retryable permission hint.
		var beforeStart *toolProbeFailure
		if errors.As(stopErr, &beforeStart) {
			return stopErr
		}
		return newProbeFailure(stage, "stop_unverified", stopErr)
	}
	if ctx.Err() != nil {
		return newProbeFailure(stage, "probe_timeout", ctx.Err())
	}
	if waitErr != nil {
		return newProbeFailure(stage, "probe_failed", waitErr)
	}
	if truncated {
		return newProbeFailure(stage, "output_limit_exceeded", nil)
	}
	return nil
}
