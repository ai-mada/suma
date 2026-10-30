package platform

import (
	"context"

	"strings"
	"time"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/workbuddy"
)

func (w *WorkBuddyConnections) Refresh(ctx context.Context, actor Actor, project, id string, req WorkBuddyRefreshRequest) (WorkBuddyConnection, error) {
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || !workBuddyConnectionExpectedVersion(req.ExpectedVersion) || !planningKey(req.IdempotencyKey) {
		return WorkBuddyConnection{}, invalid(workBuddyInvalid)
	}
	original := ctx
	var row workBuddyConnectionRow
	var op workBuddyOAuthOperation
	var old workbuddy.TokenGrant
	var client *workbuddy.OAuthClient
	invoke := false
	err := func() error {
		ctx, auth, close, err := w.workBuddyCommandAuthority(original, actor, project, id)
		if err != nil {
			return err
		}
		defer close()
		row, err = w.rowTx(ctx, auth.tx, auth.target.tenantID, project, id, true)
		if err != nil {
			return err
		}
		if err = w.workBuddyCommandOwner(auth, row); err != nil {
			return err
		}
		body := struct {
			Kind, ID string
			Request  WorkBuddyRefreshRequest
		}{"workbuddy_refresh", id, req}
		hash, replay, err := w.s.commandReceipt(ctx, auth, req.IdempotencyKey, body)
		if err != nil {
			return err
		}
		if replay {
			return nil
		}
		if err = w.workBuddyCommandApp(row); err != nil {
			return err
		}
		if row.Version != req.ExpectedVersion || !workBuddyConnectionExternalCapacity(row) {
			return conflict(workBuddyVersion)
		}
		if row.State != "connected" || row.ActiveOperationID != "" {
			return conflict(workBuddyReauthorize)
		}
		old, err = w.open(row)
		if err != nil || old.RefreshToken == "" {
			return conflict(workBuddyReauthorize)
		}
		client, err = w.oauth(row.Scopes)
		if err != nil {
			return invalid(workBuddyUnavailable)
		}
		clock, err := w.s.authorityNow(ctx, auth.tx)
		if err != nil {
			return err
		}
		before := row.Version
		row.Version++
		row.State = "refreshing"
		row.UpdatedAt = automationStamp(clock)
		row.LastError = ""
		op = workBuddyOAuthOperation{ID: newID(), TenantID: row.TenantID, ProjectID: project, ConnectionID: row.ID, OwnerID: row.OwnerID, Kind: "refresh", State: "exchanging", RequestHash: hash, CredentialDigest: actor.credential.digest, AppDigest: w.appDigest, ScopesJSON: stringify(row.Scopes), Version: 1, ConnectionVersion: row.Version, GrantGeneration: row.GrantGeneration, TokenGeneration: row.TokenGeneration, ExpiresAt: automationStamp(clock.Add(5 * time.Minute)), CreatedAt: row.UpdatedAt, UpdatedAt: row.UpdatedAt}
		row.ActiveOperationID = op.ID
		if err = w.saveRowTx(ctx, auth, row, before); err != nil {
			return err
		}
		if err = w.saveOpTx(ctx, auth, op, 0); err != nil {
			return err
		}
		if err = w.s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, workBuddyCommandReceipt{op.ID, row.ID}); err != nil {
			return err
		}
		if err = w.s.commandActivity(ctx, auth, "workbuddy_refresh_prepared", row.ID, "WorkBuddy token refresh prepared"); err != nil {
			return err
		}
		if err = auth.tx.Commit(); err != nil {
			return err
		}
		invoke = true
		return nil
	}()
	if err != nil {
		return WorkBuddyConnection{}, err
	}
	if !invoke {
		return w.connectionView(row, true), nil
	}
	grant, refreshErr := client.Refresh(original, old.RefreshToken)
	return w.workBuddyCommandObserved(original, actor, project, row, op, grant, old, refreshErr)
}

func (w *WorkBuddyConnections) Disconnect(ctx context.Context, actor Actor, project, id string, req WorkBuddyDisconnectRequest) (out WorkBuddyConnection, err error) {
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || !workBuddyConnectionExpectedVersion(req.ExpectedVersion) || !planningKey(req.IdempotencyKey) || !workBuddyConnectionReason(req.Reason) {
		return out, invalid(workBuddyInvalid)
	}
	ctx, auth, close, err := w.workBuddyCommandAuthority(ctx, actor, project, id)
	if err != nil {
		return out, err
	}
	defer close()
	row, err := w.rowTx(ctx, auth.tx, auth.target.tenantID, project, id, true)
	if err != nil {
		return out, err
	}
	if err = w.workBuddyCommandOwner(auth, row); err != nil {
		return out, err
	}
	body := struct {
		Kind, ID string
		Request  WorkBuddyDisconnectRequest
	}{"workbuddy_disconnect", id, req}
	hash, replay, err := w.s.commandReceipt(ctx, auth, req.IdempotencyKey, body)
	if err != nil {
		return out, err
	}
	if replay {
		return w.connectionView(row, true), nil
	}
	if row.Version != req.ExpectedVersion {
		return out, conflict(workBuddyVersion)
	}
	clock, err := w.s.authorityNow(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	if row.ActiveOperationID != "" {
		op, e := w.opTx(ctx, auth.tx, auth.target.tenantID, project, row.ActiveOperationID, true)
		if e != nil {
			return out, e
		}
		if op.State == "pending" || op.State == "exchanging" {
			before := op.Version
			op.Version++
			op.State = "cancelled"
			op.UpdatedAt = automationStamp(clock)
			if err = w.saveOpTx(ctx, auth, op, before); err != nil {
				return out, err
			}
		}
	}
	before := row.Version
	row.Version++
	row.GrantGeneration++
	row.State = "disconnected"
	row.Ciphertext = ""
	row.ExpiresAt = ""
	row.ActiveOperationID = ""
	row.LastError = ""
	row.UpdatedAt = automationStamp(clock)
	if err = w.saveRowTx(ctx, auth, row, before); err != nil {
		return out, err
	}
	if err = w.s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, workBuddyCommandReceipt{ConnectionID: row.ID}); err != nil {
		return out, err
	}
	if err = w.s.commandActivity(ctx, auth, "workbuddy_connection_disconnected", row.ID, "WorkBuddy connection disabled locally"); err != nil {
		return out, err
	}
	if err = auth.tx.Commit(); err != nil {
		return out, err
	}
	return w.connectionView(row, true), nil
}

func workBuddyConnectionExpectedVersion(version int) bool {
	return workBuddyVersionRoom(version, 1)
}
func workBuddyConnectionReason(reason string) bool {
	return len(reason) <= 8000 && utf8.ValidString(reason) && utf8.RuneCountInString(reason) <= 2000 && strings.TrimSpace(reason) != "" && !strings.ContainsRune(reason, 0)
}

// Exchange and refresh consume one CAS for their durable permit and another
// for the observed result. Reserve both before any provider request, including
// capacity for token rotation or invalidation of a changed grant.
func workBuddyConnectionExternalCapacity(row workBuddyConnectionRow) bool {
	return workBuddyVersionRoom(row.Version, 2) && row.TokenGeneration >= 0 && row.TokenGeneration < workBuddyRecordMaxVersion && workBuddyVersionRoom(row.GrantGeneration, 1)
}

// Reauthorization must leave room for Start plus both exchange transactions,
// and for either accepting or invalidating the subsequently observed grant.
func workBuddyConnectionAuthorizationCapacity(row workBuddyConnectionRow) bool {
	return workBuddyVersionRoom(row.Version, 3) && workBuddyVersionRoom(row.GrantGeneration, 2) && row.TokenGeneration >= 0 && row.TokenGeneration < workBuddyRecordMaxVersion
}
