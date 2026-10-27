package runner

import (
	"context"
	"fmt"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	"sync"
	"time"
)

type Worker struct {
	Client                          *Client
	Journal                         *Journal
	ID                              string
	Exec                            ExecConfig
	PollInterval, HeartbeatInterval time.Duration
	inventoryMu                     sync.Mutex
	inventory                       []tc.Observation
	inventoryReady                  bool
	inventoryInterval               time.Duration
}

func (w *Worker) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if w.Client == nil || w.Journal == nil {
		return fmt.Errorf("Runner client and durable journal are required")
	}
	if err := ValidateToolConfiguration(w.Exec); err != nil {
		return err
	}
	initialErr := w.refreshInventory(ctx)
	if initialErr != nil && permanentInventoryError(initialErr) {
		return initialErr
	}
	interval := w.inventoryInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	inventoryDone := make(chan struct{})
	go func() {
		defer close(inventoryDone)
		delay := interval
		if initialErr != nil {
			delay = min(interval, 5*time.Second)
		}
		for wait(ctx, delay) {
			if err := w.refreshInventory(ctx); err != nil {
				w.setInventoryUnavailable()
				delay = min(interval, 5*time.Second)
			} else {
				delay = interval
			}
		}
	}()
	defer func() { cancel(); <-inventoryDone }()
	poll := w.PollInterval
	if poll <= 0 {
		poll = 2 * time.Second
	}
	beat := w.HeartbeatInterval
	if beat <= 0 {
		beat = 5 * time.Second
	}
	for ctx.Err() == nil {
		for _, a := range w.Journal.pendingReleases() {
			if a.WorkerID != w.ID {
				return fmt.Errorf("release checkpoint belongs to another worker")
			}
			if err := w.runAssignment(ctx, a, beat); err != nil {
				return err
			}
		}
		if err := w.resumeArtifacts(ctx, beat); err != nil {
			if !wait(ctx, poll) {
				break
			}
			continue
		}
		if err := Replay(ctx, w.Client, w.Journal); err != nil {
			if !wait(ctx, poll) {
				break
			}
			continue
		}
		_ = ReplayWorkspaceObservations(ctx, w.Client, w.Journal, nil)
		if !w.canClaimInventory() {
			if !wait(ctx, poll) {
				break
			}
			continue
		}
		capabilities := ToolCapabilities(w.currentInventory())
		capabilities = append(capabilities, ExternalCapabilities(w.Exec.External)...)
		request, err := w.Journal.BeginClaim(w.Client.BaseURL, w.Client.ProjectID, w.ID, capabilities)
		if err != nil {
			return err
		}
		a, err := w.Client.ClaimWithOperation(ctx, request.WorkerID, request.Capabilities, request.OperationID)
		if err != nil {
			if !wait(ctx, poll) {
				break
			}
			continue
		}
		if err = w.Journal.RecordClaimResponse(request.OperationID, a); err != nil {
			return err
		}
		if a == nil || a.DispatchState != "active" {
			if err = w.Journal.CompleteClaim(request.OperationID); err != nil {
				return err
			}
			if !wait(ctx, poll) {
				break
			}
			continue
		}
		if err = w.Journal.Accept(*a); err != nil {
			return err
		}
		if err = w.Journal.CompleteClaim(request.OperationID); err != nil {
			return err
		}
		if err = w.runAssignment(ctx, *a, beat); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (w *Worker) runAssignment(ctx context.Context, a Assignment, beat time.Duration) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan Result, 1)
	progress := make(chan struct{}, 1)
	cfg := w.Exec
	cfg.ToolInventory = w.currentInventory()
	if cfg.Artifacts == nil {
		cfg.Artifacts = w.Client
	}
	cfg.Journal = w.Journal
	registry := NewWorkspaceObservationRegistry(w.Client.ProjectID)
	cfg.WorkspaceObservations = registry
	observationDone := make(chan struct{})
	go func() {
		defer close(observationDone)
		runWorkspaceObservationBroker(runCtx, w.Client, w.Journal, a, registry, beat)
	}()
	defer func() { cancel(); <-observationDone }()
	cfg.Progress = func(message string) error {
		if err := w.Journal.RecordProgress(a, message); err != nil {
			return err
		}
		select {
		case progress <- struct{}{}:
		default:
		}
		return nil
	}
	cfg.ToolProgress = func(event tc.Event) error {
		if err := w.Journal.RecordToolProgress(a, event); err != nil {
			return err
		}
		select {
		case progress <- struct{}{}:
		default:
		}
		return nil
	}
	go func() { results <- Execute(runCtx, a, cfg) }()
	senderDone := make(chan struct{})
	go func() {
		defer close(senderDone)
		retry := time.NewTicker(beat)
		defer retry.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-progress:
			case <-retry.C:
			}
			_ = ReplayProgress(runCtx, w.Client, w.Journal)
		}
	}()
	defer func() { cancel(); <-senderDone }()
	timer := time.NewTicker(beat)
	defer timer.Stop()
	for {
		select {
		case r := <-results:
			if r.artifactPending || r.candidatePending {
				return nil
			}
			return w.Journal.Finish(r)
		case <-timer.C:
			requested, err := w.Client.Heartbeat(runCtx, a)
			if requested || err != nil {
				cancel()
			}
		}
	}
}

func (w *Worker) resumeArtifacts(ctx context.Context, beat time.Duration) error {
	for _, p := range w.Journal.pendingArtifacts() {
		if p.Assignment.WorkerID != w.ID {
			return fmt.Errorf("artifact checkpoint belongs to another worker")
		}
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- resumeArtifact(runCtx, w.Client, w.Journal, p) }()
		timer := time.NewTicker(beat)
		var err error
	loop:
		for {
			select {
			case err = <-done:
				break loop
			case <-timer.C:
				requested, beatErr := w.Client.Heartbeat(runCtx, p.Assignment)
				if requested || beatErr != nil {
					cancel()
				}
			case <-ctx.Done():
				cancel()
			}
		}
		timer.Stop()
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func wait(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
