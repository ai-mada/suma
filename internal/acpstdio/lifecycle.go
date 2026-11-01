package acpstdio

import "bytes"

// CanStopAfterRPC is provisional protocol proof, not process containment.
// Only Cursor's foreground may be stopped after this gate; the caller must
// recheck it after real Wait, retain the actual exit and prove the whole tree
// stopped before publishing success. Finish intentionally cancels this gate.
func (d *Driver) CanStopAfterRPC() bool {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.options.Runtime != "cursor" || d.ctx.Err() != nil || d.localClosed || d.bad != nil || !d.inputDrained || len(bytes.TrimSpace(d.pending)) != 0 || len(d.requests) != 0 || len(d.output) != 0 {
		return false
	}
	select {
	case <-d.done:
	default:
		return false
	}
	for _, p := range d.remote {
		if !p.decided {
			return false
		}
	}
	if s := d.discovery; s != nil {
		e := s.evidence
		return s.complete && s.catalogue != nil && e.SessionCreated && e.SessionCancelRequested && e.SessionCancelWritten && e.PromptRequests == 0 && e.ConfigurationWrites == 0 && e.PermissionRequests == 0 && e.UnexpectedActivity == 0
	}
	e := d.result.Evidence
	return d.result.Terminal == "completed" && e.PromptCompleted && e.StopReason == "end_turn" && e.ToolsComplete && !e.CancelRequested && !d.incomplete && !d.toolInvalid && e.PermissionsDenied == 0 && d.cursorInputsCompleteLocked()
}

func (m *ModelDiscovery) CanStopAfterRPC() bool { return m.d.CanStopAfterRPC() }
