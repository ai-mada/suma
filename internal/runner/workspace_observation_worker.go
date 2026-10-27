package runner

import (
	"context"
	"errors"
	"fmt"
	"time"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

func observationScopedClient(client *Client, entry workspaceObservationCheckpoint) (*Client, error) {
	if client.Pool != nil {
		return client.forPoolAssignment(entry.Assignment)
	}
	if entry.Assignment.PoolID != "" || client.ProjectID != entry.Lease.Observation.ProjectID {
		return nil, fmt.Errorf("workspace observation callback belongs to a different fixed project")
	}
	return client, nil
}

// Replay submits the original bounded receipt. A missing old host produces a
// closed-scope failure; recovery never opens historical filesystem paths.
func ReplayWorkspaceObservations(ctx context.Context, client *Client, journal *Journal, registry *WorkspaceObservationRegistry) error {
	for _, entry := range journal.pendingWorkspaceObservations(client.BaseURL) {
		lease := entry.Lease
		until, err := time.Parse(time.RFC3339Nano, lease.LeaseUntil)
		expires, expiresErr := time.Parse(time.RFC3339Nano, lease.Observation.ExpiresAt)
		if err != nil || expiresErr != nil || !until.After(time.Now()) || !expires.After(time.Now()) {
			if err = journal.retireWorkspaceObservation(client.BaseURL, lease); err != nil {
				return err
			}
			continue
		}
		scoped, err := observationScopedClient(client, entry)
		if err != nil {
			continue
		}
		if entry.Result == nil {
			result := registry.Observe(ctx, lease)
			if err = journal.saveWorkspaceObservationResult(client.BaseURL, lease, result); err != nil {
				return err
			}
			entry.Result = &result
		}
		callbackCtx, cancel := context.WithDeadline(ctx, until)
		receipt, sendErr := scoped.CompleteWorkspaceObservation(callbackCtx, *entry.Result)
		cancel()
		if sendErr != nil {
			var status *runnerHTTPStatusError
			if errors.As(sendErr, &status) && (status.Status == 400 || status.Status == 401 || status.Status == 403 || status.Status == 404 || status.Status == 409 || status.Status == 410) {
				if err = journal.retireWorkspaceObservation(client.BaseURL, lease); err != nil {
					return err
				}
				continue
			}
			return sendErr
		}
		if err = journal.ackWorkspaceObservation(client.BaseURL, receipt); err != nil {
			return err
		}
	}
	return nil
}

func runWorkspaceObservationBroker(ctx context.Context, client *Client, journal *Journal, a Assignment, registry *WorkspaceObservationRegistry, interval time.Duration) {
	if interval <= 0 || interval > time.Second {
		interval = time.Second
	}
	for ctx.Err() == nil {
		if err := ReplayWorkspaceObservations(ctx, client, journal, registry); err == nil {
			if scope, policy, active := registry.active(); active {
				request := woc.ClaimRequest{ProjectID: client.ProjectID, WorkerID: a.WorkerID, Scope: scope, WorkspacePolicySHA256: policy, OperationID: operationID()}
				claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				lease, claimErr := client.ClaimWorkspaceObservation(claimCtx, request)
				cancel()
				if claimErr == nil && lease != nil {
					if journal.checkpointWorkspaceObservation(client.BaseURL, a, *lease) == nil {
						_ = ReplayWorkspaceObservations(ctx, client, journal, registry)
					}
				}
			}
		}
		if !wait(ctx, interval) {
			return
		}
	}
}
