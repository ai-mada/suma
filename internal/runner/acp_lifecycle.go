package runner

import (
	"context"
	"time"
)

type acpHostCompletion struct {
	waitErr      error
	waitObserved bool
	stopErr      error
	afterRPC     bool
	interrupted  bool
}

// Called only for Cursor after its sole stdin writer drained the SDK stack.
// The protocol/source gate is checked before and after the real tree stop.
// A Stop error is retained even if a later Finish sees no remaining process.
func stopCursorHostAfterRPC(ctx context.Context, controller interface{ Stop() error }, done <-chan error, safe func() bool) acpHostCompletion {
	grace := time.NewTimer(2 * time.Second)
	defer grace.Stop()
	var r acpHostCompletion
	waitStopped := func() {
		if probe, ok := controller.(*probeProcessController); ok {
			r.waitErr = waitStoppedProbeProcess(probe, done)
			r.waitObserved, _ = probe.waitFact()
			return
		}
		r.waitErr, r.waitObserved = waitForNativeCompletion(done, processCancellationWait)
	}
	select {
	case r.waitErr = <-done:
		r.waitObserved = true
		r.interrupted = ctx.Err() != nil
		return r
	case <-grace.C:
		eligible := ctx.Err() == nil && safe()
		r.stopErr = controller.Stop()
		waitStopped()
		r.interrupted = ctx.Err() != nil
		r.afterRPC = r.waitObserved && eligible && r.stopErr == nil && !r.interrupted && safe()
		return r
	case <-ctx.Done():
		r.interrupted = true
		r.stopErr = controller.Stop()
		waitStopped()
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
