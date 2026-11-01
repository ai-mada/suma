package runner

import (
	"context"
	"time"
)

type acpHostCompletion struct {
	waitErr     error
	stopErr     error
	afterRPC    bool
	interrupted bool
}

// Called only for Cursor after its sole stdin writer drained the SDK stack.
// The protocol/source gate is checked before and after the real tree stop.
// A Stop error is retained even if a later Finish sees no remaining process.
func stopCursorHostAfterRPC(ctx context.Context, controller *processController, done <-chan error, safe func() bool) acpHostCompletion {
	grace := time.NewTimer(2 * time.Second)
	defer grace.Stop()
	var r acpHostCompletion
	select {
	case r.waitErr = <-done:
		r.interrupted = ctx.Err() != nil
		return r
	case <-grace.C:
		eligible := ctx.Err() == nil && safe()
		r.stopErr = controller.Stop()
		r.waitErr = <-done
		r.interrupted = ctx.Err() != nil
		r.afterRPC = eligible && r.stopErr == nil && !r.interrupted && safe()
		return r
	case <-ctx.Done():
		r.interrupted = true
		r.stopErr = controller.Stop()
		r.waitErr = <-done
		return r
	}
}

func currentACPCommand(ctx context.Context, cfg ExecConfig, spec CommandSpec, executable, profileSHA string) bool {
	if ctx.Err() != nil {
		return false
	}
	current, sha, err := commandExecutable(spec)
	if err != nil || current != executable {
		return false
	}
	currentSHA, err := effectiveCommandProfileDigest(cfg, spec, sha)
	return err == nil && currentSHA == profileSHA && ctx.Err() == nil
}

func completeACPOutput(out *output) bool {
	out.mu.Lock()
	defer out.mu.Unlock()
	return !out.truncated
}
