package runner

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func writeScopedClient(client *Client, assignment Assignment, project string) (*Client, error) {
	if client.Pool != nil {
		return client.forPoolAssignment(assignment)
	}
	if assignment.PoolID != "" || client.ProjectID != project {
		return nil, fmt.Errorf("workspace write belongs to a different fixed project")
	}
	return client, nil
}
func terminalWriteRPC(err error) bool {
	var status *runnerHTTPStatusError
	return errors.As(err, &status) && (status.Status == 400 || status.Status == 401 || status.Status == 403 || status.Status == 404 || status.Status == 409 || status.Status == 410)
}

// Claims have an original durable RPC identity before delivery. Retrying that
// RPC can recover the one lease; it never asks Control to execute a new write.
func recoverWorkspaceWriteClaim(ctx context.Context, client *Client, journal *Journal) error {
	claim := journal.pendingWorkspaceWriteClaim(client.BaseURL)
	if claim == nil {
		return nil
	}
	scoped, err := writeScopedClient(client, claim.Assignment, claim.Request.ProjectID)
	if err != nil {
		return err
	}
	rpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	lease, err := scoped.ClaimWorkspaceWrite(rpcCtx, claim.Request)
	cancel()
	if err != nil {
		if terminalWriteRPC(err) {
			return journal.retireWorkspaceWriteClaim(client.BaseURL, claim.Request.OperationID)
		}
		return err
	}
	return journal.recordWorkspaceWriteClaim(client.BaseURL, claim.Request.OperationID, lease)
}

// ProcessingStarted is durable before touching the filesystem and removes the
// delivery body. Recovery may inspect an original native receipt, never rerun
// that effect. A missing live handle remains unknown, without historical reopen.
func ReplayWorkspaceWrites(ctx context.Context, client *Client, journal *Journal, registry *WorkspaceObservationRegistry) error {
	if err := recoverWorkspaceWriteClaim(ctx, client, journal); err != nil {
		return err
	}
	for _, entry := range journal.pendingWorkspaceWrites(client.BaseURL) {
		scoped, err := writeScopedClient(client, entry.Assignment, entry.Lease.Command.ProjectID)
		if err != nil {
			continue
		}
		if entry.Result == nil {
			if !entry.ProcessingStarted {
				if err = journal.startWorkspaceWrite(client.BaseURL, entry.Lease); err != nil {
					return err
				}
			}
			result := executeWorkspaceWrite(ctx, registry, entry.Lease, !entry.ProcessingStarted)
			if err = journal.saveWorkspaceWriteResult(client.BaseURL, entry.Lease, result); err != nil {
				return err
			}
			entry.Result = &result
		}
		// A permit deadline controls admission to the effect, not delivery of an
		// already-known fact. Keep the original result immutable across lost ACKs.
		rpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		receipt, err := scoped.CompleteWorkspaceWrite(rpcCtx, *entry.Result)
		cancel()
		if err != nil {
			if terminalWriteRPC(err) {
				if err = journal.retireWorkspaceWrite(client.BaseURL, entry.Lease); err != nil {
					return err
				}
				continue
			}
			return err
		}
		if err = journal.ackWorkspaceWrite(client.BaseURL, receipt); err != nil {
			return err
		}
	}
	return nil
}
func runWorkspaceWriteBroker(ctx context.Context, client *Client, journal *Journal, a Assignment, registry *WorkspaceObservationRegistry, interval time.Duration) {
	if interval <= 0 || interval > time.Second {
		interval = time.Second
	}
	for ctx.Err() == nil {
		if ReplayWorkspaceWrites(ctx, client, journal, registry) == nil {
			if scope, policy, active := registry.active(); active {
				if _, err := journal.beginWorkspaceWriteClaim(client.BaseURL, a, scope, policy, client.ProjectID); err == nil {
					_ = ReplayWorkspaceWrites(ctx, client, journal, registry)
				}
			}
		}
		if !wait(ctx, interval) {
			return
		}
	}
}
