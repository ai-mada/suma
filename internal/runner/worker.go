package runner

import (
	"context"
	"errors"
	"fmt"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ti "github.com/ai-mada/suma/internal/toolinspectioncontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
	wc "github.com/ai-mada/suma/internal/workspaceoperationcontract"
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
	inventoryOpMu                   sync.Mutex
	inventory                       []tc.Observation
	inventoryReady                  bool
	inventoryInterval               time.Duration
	inventoryNext                   time.Time
	maintenance                     *ti.ToolMaintenanceCapability
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
	if err := w.enableToolMaintenance(); err != nil {
		return err
	}
	defer func() { w.maintenance = nil }()
	initialErr := w.tickToolInventory(ctx)
	if initialErr != nil && permanentInventoryError(initialErr) {
		return initialErr
	}
	inventoryDone := make(chan struct{})
	go func() {
		defer close(inventoryDone)
		// The existing inventory owner also consumes maintenance requests while
		// the business executor is busy. Each pass uses the same Journal owner.
		for wait(ctx, w.inventoryPollInterval()) {
			_ = w.tickToolInventory(ctx)
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
		for _, a := range w.Journal.pendingCloudExecutions() {
			if a.WorkerID != w.ID {
				return fmt.Errorf("cloud checkpoint belongs to another worker")
			}
			if err := w.runAssignment(ctx, a, beat); err != nil {
				return err
			}
		}
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
			if errors.Is(err, errToolPermissionReconciliation) {
				return err
			}
			if !wait(ctx, poll) {
				break
			}
			continue
		}
		_ = ReplayWorkspaceObservations(ctx, w.Client, w.Journal, nil)
		_ = ReplayWorkspaceWrites(ctx, w.Client, w.Journal, nil)
		if !w.canClaimInventory() {
			if !wait(ctx, poll) {
				break
			}
			continue
		}
		capabilities := runnerCapabilities(w.Exec, ToolCapabilities(w.currentInventory()))
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
	cfg.TerminalClient = w.Client
	cfg.CloudClient = w.Client
	cfg.ToolInputClient = w.Client
	cfg.SteeringClient = w.Client
	cfg.ToolInventory = w.currentInventory()
	if cfg.Artifacts == nil {
		cfg.Artifacts = w.Client
	}
	cfg.Journal = w.Journal
	cfg.ToolPermission = func(ctx context.Context, request pc.Request) (pc.Decision, error) {
		return w.waitToolPermission(ctx, a, request, cfg.Secrets)
	}
	cfg.ToolPermissionPoll = func(ctx context.Context, id string) (pc.Permission, error) {
		return w.Client.PollToolPermission(ctx, a, id)
	}
	if a.WorkBuddyCloud == nil {
		registry := NewWorkspaceObservationRegistry(w.Client.ProjectID)
		cfg.WorkspaceObservations = registry
		observationDone := make(chan struct{})
		go func() {
			defer close(observationDone)
			runWorkspaceObservationBroker(runCtx, w.Client, w.Journal, a, registry, beat)
		}()
		defer func() { cancel(); <-observationDone }()
		writeDone := make(chan struct{})
		go func() {
			defer close(writeDone)
			runWorkspaceWriteBroker(runCtx, w.Client, w.Journal, a, registry, beat)
		}()
		defer func() { cancel(); <-writeDone }()
	}
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
	cfg.WorkspaceProgress = func(event wc.Event) error {
		if err := w.Journal.RecordWorkspaceProgress(a, event); err != nil {
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
			_ = ReplayToolPermissionObservations(runCtx, w.Client, w.Journal)
			_ = ReplayToolInputObservations(runCtx, w.Client, w.Journal)
			_ = ReplaySteering(runCtx, w.Client, w.Journal)
			_ = ReplayProgress(runCtx, w.Client, w.Journal)
		}
	}()
	defer func() { cancel(); <-senderDone }()
	timer := time.NewTicker(beat)
	defer timer.Stop()
	for {
		select {
		case r := <-results:
			if a.WorkBuddyCloud != nil && r.WorkBuddyCloudWait == nil {
				return fmt.Errorf("cloud checkpoint unavailable; original assignment retained")
			}
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
