package platform

import (
	"context"
	"encoding/json"
	"errors"

	"time"

	"github.com/ai-mada/suma/internal/workbuddy"
)

type workBuddyCommandReceipt struct {
	AuthorizationID string `json:"authorization_id,omitempty"`
	ConnectionID    string `json:"connection_id"`
}

func workBuddyCommandOpaque(value string, max int) bool {
	if len(value) == 0 || len(value) > max {
		return false
	}
	for _, r := range value {
		if r < 33 || r > 126 {
			return false
		}
	}
	return true
}
func (w *WorkBuddyConnections) workBuddyCommandAuthority(ctx context.Context, actor Actor, project, id string) (context.Context, *authorizedCommand, func(), error) {
	if w == nil || w.s == nil {
		return ctx, nil, func() {}, invalid(workBuddyUnavailable)
	}
	return w.s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectWrite)
}
func (w *WorkBuddyConnections) workBuddyCommandOwner(auth *authorizedCommand, row workBuddyConnectionRow) error {
	if row.OwnerID != auth.currentPrincipal.ID {
		return missing()
	}
	return nil
}
func (w *WorkBuddyConnections) workBuddyCommandApp(row workBuddyConnectionRow) error {
	if !w.configured || row.AppDigest != w.appDigest || row.ClientID != w.app.ClientID {
		return conflict(workBuddyReauthorize)
	}
	return nil
}
func (w *WorkBuddyConnections) workBuddyCommandAuthorizationView(ctx context.Context, auth *authorizedCommand, row workBuddyConnectionRow, op workBuddyOAuthOperation) (WorkBuddyAuthorization, error) {
	out := WorkBuddyAuthorization{ID: op.ID, Version: op.Version, Connection: w.connectionView(row, true), ExpiresAt: op.ExpiresAt, State: op.State}
	if op.CredentialDigest != auth.actor.credential.digest || op.OwnerID != auth.currentPrincipal.ID {
		return out, forbidden()
	}
	clock, err := w.s.authorityNow(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	deadline, err := time.Parse(time.RFC3339Nano, op.ExpiresAt)
	if err != nil {
		return out, invalid(workBuddyInvalid)
	}
	if op.State == "pending" && !clock.Before(deadline) {
		out.State = "expired"
		return out, nil
	}
	if op.State != "pending" || row.ActiveOperationID != op.ID || row.Version != op.ConnectionVersion || row.State != "authorizing" {
		return out, nil
	}
	if err = w.workBuddyCommandApp(row); err != nil {
		return out, err
	}
	var scopes []string
	if json.Unmarshal([]byte(op.ScopesJSON), &scopes) != nil {
		return out, invalid(workBuddyInvalid)
	}
	client, err := w.oauth(scopes)
	if err != nil {
		return out, invalid(workBuddyUnavailable)
	}
	out.AuthorizationURL, err = client.AuthorizationURL(w.stateToken(op))
	return out, err
}

func (w *WorkBuddyConnections) StartAuthorization(ctx context.Context, actor Actor, project string, req WorkBuddyAuthorizationRequest) (out WorkBuddyAuthorization, err error) {
	if req.ProjectID != project || !handoffID(project, false) || !planningKey(req.IdempotencyKey) || req.ConnectionID != "" && !handoffID(req.ConnectionID, false) || req.ConnectionID == "" && req.ExpectedVersion != 0 || req.ConnectionID != "" && !workBuddyConnectionExpectedVersion(req.ExpectedVersion) {
		return out, invalid(workBuddyInvalid)
	}
	req.Scopes, err = workBuddyCanonicalScopes(req.Scopes, false)
	if err != nil {
		return out, invalid(workBuddyInvalid)
	}
	ctx, auth, close, err := w.workBuddyCommandAuthority(ctx, actor, project, req.ConnectionID)
	if err != nil {
		return out, err
	}
	defer close()
	if _, err = w.oauth(req.Scopes); err != nil {
		return out, invalid(workBuddyUnavailable)
	}
	body := struct {
		Kind    string
		Request WorkBuddyAuthorizationRequest
	}{"workbuddy_authorize", req}
	hash, replay, err := w.s.commandReceipt(ctx, auth, req.IdempotencyKey, body)
	if err != nil {
		return out, err
	}
	if replay {
		var receipt workBuddyCommandReceipt
		if json.Unmarshal([]byte(hash), &receipt) != nil {
			return out, invalid(workBuddyInvalid)
		}
		row, e := w.rowTx(ctx, auth.tx, auth.target.tenantID, project, receipt.ConnectionID, true)
		if e != nil {
			return out, e
		}
		if e = w.workBuddyCommandOwner(auth, row); e != nil {
			return out, e
		}
		op, e := w.opTx(ctx, auth.tx, auth.target.tenantID, project, receipt.AuthorizationID, true)
		if e != nil {
			return out, e
		}
		return w.workBuddyCommandAuthorizationView(ctx, auth, row, op)
	}
	clock, err := w.s.authorityNow(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	stamp := automationStamp(clock)
	var row workBuddyConnectionRow
	before := 0
	if req.ConnectionID != "" {
		row, err = w.rowTx(ctx, auth.tx, auth.target.tenantID, project, req.ConnectionID, true)
		if err != nil {
			return out, err
		}
		if err = w.workBuddyCommandOwner(auth, row); err != nil {
			return out, err
		}
		if row.Version != req.ExpectedVersion || !workBuddyConnectionAuthorizationCapacity(row) {
			return out, conflict(workBuddyVersion)
		}
		if row.State == "authorizing" || row.State == "refreshing" {
			return out, conflict(workBuddyEffectUnknown)
		}
		if row.ClientID != w.app.ClientID || row.AppDigest != w.appDigest {
			return out, conflict(workBuddyReauthorize)
		}
		before = row.Version
		row.GrantGeneration++
	} else {
		row = workBuddyConnectionRow{WorkBuddyConnection: WorkBuddyConnection{ID: newID(), ProjectID: project, OwnerID: auth.currentPrincipal.ID, ClientID: w.app.ClientID, GrantGeneration: 1, CreatedAt: stamp}, TenantID: auth.target.tenantID, AppDigest: w.appDigest}
	}
	row.Version = before + 1
	row.State = "authorizing"
	row.Ciphertext = ""
	row.ExpiresAt = ""
	row.LastError = ""
	row.UpdatedAt = stamp
	// Requested scope lives in the attempt; pending UI must not mistake it for a
	// newly granted permission. Preserve prior granted scopes until completion.
	if row.Scopes == nil {
		row.Scopes = []string{}
	}
	op := workBuddyOAuthOperation{ID: newID(), TenantID: row.TenantID, ProjectID: project, ConnectionID: row.ID, OwnerID: row.OwnerID, Kind: "authorization", State: "pending", RequestHash: hash, CredentialDigest: actor.credential.digest, AppDigest: w.appDigest, ScopesJSON: stringify(req.Scopes), Version: 1, ConnectionVersion: row.Version, GrantGeneration: row.GrantGeneration, TokenGeneration: row.TokenGeneration, ExpiresAt: automationStamp(clock.Add(5 * time.Minute)), CreatedAt: stamp, UpdatedAt: stamp}
	op.StateHash = digest(w.stateToken(op))
	row.ActiveOperationID = op.ID
	if err = w.saveRowTx(ctx, auth, row, before); err != nil {
		return out, err
	}
	if err = w.saveOpTx(ctx, auth, op, 0); err != nil {
		return out, err
	}
	if err = w.s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, workBuddyCommandReceipt{op.ID, row.ID}); err != nil {
		return out, err
	}
	if err = w.s.commandActivity(ctx, auth, "workbuddy_authorization_started", row.ID, "WorkBuddy authorization requested"); err != nil {
		return out, err
	}
	out, err = w.workBuddyCommandAuthorizationView(ctx, auth, row, op)
	if err != nil {
		return out, err
	}
	err = auth.tx.Commit()
	return out, err
}

func (w *WorkBuddyConnections) CompleteAuthorization(ctx context.Context, actor Actor, project, id string, req WorkBuddyAuthorizationComplete) (WorkBuddyConnection, error) {
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || !workBuddyConnectionExpectedVersion(req.ExpectedVersion) || !planningKey(req.IdempotencyKey) || !workBuddyCommandOpaque(req.State, 256) || !workBuddyCommandOpaque(req.Code, 8192) {
		return WorkBuddyConnection{}, invalid(workBuddyInvalid)
	}
	original := ctx
	var row workBuddyConnectionRow
	var op workBuddyOAuthOperation
	var client *workbuddy.OAuthClient
	invoke := false
	err := func() error {
		ctx, auth, close, err := w.workBuddyCommandAuthority(original, actor, project, id)
		if err != nil {
			return err
		}
		defer close()
		op, err = w.opTx(ctx, auth.tx, auth.target.tenantID, project, id, true)
		if err != nil {
			return err
		}
		row, err = w.rowTx(ctx, auth.tx, auth.target.tenantID, project, op.ConnectionID, true)
		if err != nil {
			return err
		}
		if err = w.workBuddyCommandOwner(auth, row); err != nil {
			return err
		}
		if op.OwnerID != auth.currentPrincipal.ID || op.Kind != "authorization" || op.CredentialDigest != actor.credential.digest || !w.verifyState(op, req.State) {
			return forbidden()
		}
		body := struct {
			Kind, ID, CodeHash, StateHash, Key string
			ExpectedVersion                    int
		}{"workbuddy_complete", id, digest(req.Code), digest(req.State), req.IdempotencyKey, req.ExpectedVersion}
		requestHash := digest(stringify(body))
		hash, replay, err := w.s.commandReceipt(ctx, auth, req.IdempotencyKey, body)
		if err != nil {
			return err
		}
		if replay {
			if op.CodeHash != digest(req.Code) || op.RequestHash != requestHash {
				return conflict(workBuddyVersion)
			}
			return nil
		}
		if op.State != "pending" || op.Version != req.ExpectedVersion || row.ActiveOperationID != op.ID || row.Version != op.ConnectionVersion || row.GrantGeneration != op.GrantGeneration || row.State != "authorizing" || !workBuddyConnectionExternalCapacity(row) || !workBuddyVersionRoom(op.Version, 2) {
			return conflict(workBuddyVersion)
		}
		if err = w.workBuddyCommandApp(row); err != nil {
			return err
		}
		clock, err := w.s.authorityNow(ctx, auth.tx)
		if err != nil {
			return err
		}
		deadline, err := time.Parse(time.RFC3339Nano, op.ExpiresAt)
		if err != nil || !clock.Before(deadline) {
			return conflict(workBuddyReauthorize)
		}
		var scopes []string
		if json.Unmarshal([]byte(op.ScopesJSON), &scopes) != nil {
			return invalid(workBuddyInvalid)
		}
		client, err = w.oauth(scopes)
		if err != nil {
			return invalid(workBuddyUnavailable)
		}
		rowBefore, opBefore := row.Version, op.Version
		row.Version++
		row.UpdatedAt = automationStamp(clock)
		op.Version++
		op.State = "exchanging"
		op.CodeHash = digest(req.Code)
		op.RequestHash = requestHash
		op.ConnectionVersion = row.Version
		op.UpdatedAt = row.UpdatedAt
		if err = w.saveRowTx(ctx, auth, row, rowBefore); err != nil {
			return err
		}
		if err = w.saveOpTx(ctx, auth, op, opBefore); err != nil {
			return err
		}
		if err = w.s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, workBuddyCommandReceipt{op.ID, row.ID}); err != nil {
			return err
		}
		if err = w.s.commandActivity(ctx, auth, "workbuddy_authorization_exchanging", row.ID, "WorkBuddy code exchange prepared"); err != nil {
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
	grant, exchangeErr := client.ExchangeCode(original, req.Code)
	return w.workBuddyCommandObserved(original, actor, project, row, op, grant, workbuddy.TokenGrant{}, exchangeErr)
}

// The external call is over. Persist its observed outcome under a fresh bounded
// authorization transaction even if the initiating HTTP request was cancelled.
// This detached context never authorizes another network request.
func (w *WorkBuddyConnections) workBuddyCommandFinish(ctx context.Context, actor Actor, project string, prepared workBuddyConnectionRow, permit workBuddyOAuthOperation, grant, old workbuddy.TokenGrant, callErr error) (WorkBuddyConnection, error) {
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), authorityTransactionBudget)
	defer cancel()
	ctx, auth, close, err := w.workBuddyCommandAuthority(persist, actor, project, prepared.ID)
	if err != nil {
		return WorkBuddyConnection{}, err
	}
	defer close()
	row, err := w.rowTx(ctx, auth.tx, auth.target.tenantID, project, prepared.ID, true)
	if err != nil {
		return WorkBuddyConnection{}, err
	}
	if err = w.workBuddyCommandOwner(auth, row); err != nil {
		return WorkBuddyConnection{}, err
	}
	op, err := w.opTx(ctx, auth.tx, auth.target.tenantID, project, permit.ID, true)
	if err != nil {
		return WorkBuddyConnection{}, err
	}
	if row.State == "disconnected" || op.State == "cancelled" {
		return w.connectionView(row, true), nil
	}
	if err = w.workBuddyCommandApp(row); err != nil {
		return WorkBuddyConnection{}, err
	}
	if row.ActiveOperationID != permit.ID || row.Version != prepared.Version || op.Version != permit.Version || op.State != "exchanging" || row.GrantGeneration != permit.GrantGeneration || row.TokenGeneration != permit.TokenGeneration || op.CredentialDigest != actor.credential.digest {
		return WorkBuddyConnection{}, conflict(workBuddyVersion)
	}
	clock, err := w.s.authorityNow(ctx, auth.tx)
	if err != nil {
		return WorkBuddyConnection{}, err
	}
	rowBefore, opBefore := row.Version, op.Version
	row.Version++
	op.Version++
	row.UpdatedAt = automationStamp(clock)
	op.UpdatedAt = row.UpdatedAt
	row.ActiveOperationID = ""
	row.LastError = ""
	op.LastError = ""
	row.Ciphertext = ""
	row.ExpiresAt = ""
	if callErr != nil {
		row.State = "authorization_unknown"
		if permit.Kind == "refresh" {
			row.State = "refresh_unknown"
		}
		op.State = "unknown"
		row.LastError = workBuddyEffectUnknown
		op.LastError = workBuddyEffectUnknown
	} else {
		scopes, scopeErr := workBuddyCanonicalScopes(grant.Scopes, false)
		identityChanged := row.OpenID != "" && row.OpenID != grant.OpenID
		scopeChanged := permit.Kind == "refresh" && stringify(scopes) != stringify(row.Scopes)
		if scopeErr != nil || identityChanged || scopeChanged || grant.OpenID == "" || !clock.Before(grant.ExpiresAt) {
			row.State = "reauthorization_required"
			row.GrantGeneration++
			row.LastError = workBuddyReauthorize
			op.State = "failed"
			op.LastError = workBuddyReauthorize
		} else {
			if permit.Kind == "refresh" && !grant.RefreshTokenPresent {
				grant.RefreshToken = old.RefreshToken
				grant.RefreshTokenPresent = old.RefreshTokenPresent
			}
			row.OpenID = grant.OpenID
			row.Scopes = scopes
			row.ExpiresAt = grant.ExpiresAt.UTC().Format(time.RFC3339Nano)
			row.TokenGeneration++
			row.State = "connected"
			op.State = "succeeded"
			row.Ciphertext, err = w.seal(row, grant)
			if err != nil {
				return WorkBuddyConnection{}, invalid(workBuddyUnavailable)
			}
		}
	}
	if err = w.saveRowTx(ctx, auth, row, rowBefore); err != nil {
		return WorkBuddyConnection{}, err
	}
	if err = w.saveOpTx(ctx, auth, op, opBefore); err != nil {
		return WorkBuddyConnection{}, err
	}
	if err = w.s.commandActivity(ctx, auth, "workbuddy_"+permit.Kind+"_observed", row.ID, "WorkBuddy authorization outcome recorded"); err != nil {
		return WorkBuddyConnection{}, err
	}
	if err = auth.tx.Commit(); err != nil {
		return WorkBuddyConnection{}, err
	}
	return w.connectionView(row, true), nil
}

func (w *WorkBuddyConnections) workBuddyCommandObserved(ctx context.Context, actor Actor, project string, row workBuddyConnectionRow, op workBuddyOAuthOperation, grant, old workbuddy.TokenGrant, callErr error) (WorkBuddyConnection, error) {
	result, err := w.workBuddyCommandFinish(ctx, actor, project, row, op, grant, old, callErr)
	if err == nil {
		return result, nil
	}
	// A persistence failure cannot prove the provider did nothing. Try to retain
	// that fact in a new bounded transaction, but never bypass revoked authority.
	// If storage is unavailable, the consumed exchanging permit remains durable.
	w.workBuddyCommandMarkUnknown(ctx, actor, project, row, op)
	var api *APIError
	if errors.As(err, &api) && api.Status >= 400 && api.Status < 500 {
		return WorkBuddyConnection{}, err
	}
	return WorkBuddyConnection{}, &APIError{503, workBuddyEffectUnknown, workBuddyEffectUnknown, nil}
}
func (w *WorkBuddyConnections) workBuddyCommandMarkUnknown(ctx context.Context, actor Actor, project string, prepared workBuddyConnectionRow, permit workBuddyOAuthOperation) {
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), authorityTransactionBudget)
	defer cancel()
	ctx, auth, close, err := w.workBuddyCommandAuthority(persist, actor, project, prepared.ID)
	if err != nil {
		return
	}
	defer close()
	row, err := w.rowTx(ctx, auth.tx, auth.target.tenantID, project, prepared.ID, true)
	if err != nil || w.workBuddyCommandOwner(auth, row) != nil {
		return
	}
	op, err := w.opTx(ctx, auth.tx, auth.target.tenantID, project, permit.ID, true)
	if err != nil {
		return
	}
	if row.Version != prepared.Version || row.ActiveOperationID != permit.ID || op.Version != permit.Version || op.State != "exchanging" || row.GrantGeneration != permit.GrantGeneration || row.TokenGeneration != permit.TokenGeneration || op.CredentialDigest != actor.credential.digest {
		return
	}
	clock, err := w.s.authorityNow(ctx, auth.tx)
	if err != nil {
		return
	}
	before, opBefore := row.Version, op.Version
	row.Version++
	op.Version++
	row.State = "authorization_unknown"
	if permit.Kind == "refresh" {
		row.State = "refresh_unknown"
	}
	row.Ciphertext = ""
	row.ExpiresAt = ""
	row.ActiveOperationID = ""
	row.LastError = workBuddyEffectUnknown
	row.UpdatedAt = automationStamp(clock)
	op.State = "unknown"
	op.LastError = workBuddyEffectUnknown
	op.UpdatedAt = row.UpdatedAt
	if w.saveRowTx(ctx, auth, row, before) != nil || w.saveOpTx(ctx, auth, op, opBefore) != nil {
		return
	}
	if w.s.commandActivity(ctx, auth, "workbuddy_outcome_unknown", row.ID, "WorkBuddy outcome requires reconciliation") != nil {
		return
	}
	_ = auth.tx.Commit()
}

// CancelAuthorization records only a pre-exchange user denial. It does not
// revoke a provider grant or cancel any remote operation already in progress.
func (w *WorkBuddyConnections) CancelAuthorization(ctx context.Context, actor Actor, project, id string, req WorkBuddyAuthorizationCancel) (out WorkBuddyConnection, err error) {
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || !workBuddyConnectionExpectedVersion(req.ExpectedVersion) || !planningKey(req.IdempotencyKey) || !workBuddyCommandOpaque(req.State, 256) {
		return out, invalid(workBuddyInvalid)
	}
	ctx, auth, close, err := w.workBuddyCommandAuthority(ctx, actor, project, id)
	if err != nil {
		return out, err
	}
	defer close()
	op, err := w.opTx(ctx, auth.tx, auth.target.tenantID, project, id, true)
	if err != nil {
		return out, err
	}
	row, err := w.rowTx(ctx, auth.tx, auth.target.tenantID, project, op.ConnectionID, true)
	if err != nil {
		return out, err
	}
	if err = w.workBuddyCommandOwner(auth, row); err != nil {
		return out, err
	}
	if op.OwnerID != auth.currentPrincipal.ID || op.Kind != "authorization" || op.CredentialDigest != actor.credential.digest || !w.verifyState(op, req.State) {
		return out, forbidden()
	}
	body := struct {
		Kind, ID, StateHash string
		ExpectedVersion     int
	}{"workbuddy_authorization_denied", id, digest(req.State), req.ExpectedVersion}
	hash, replay, err := w.s.commandReceipt(ctx, auth, req.IdempotencyKey, body)
	if err != nil {
		return out, err
	}
	if replay {
		return w.connectionView(row, true), nil
	}
	if op.State != "pending" || op.Version != req.ExpectedVersion || row.ActiveOperationID != op.ID || row.Version != op.ConnectionVersion || row.State != "authorizing" {
		return out, conflict(workBuddyVersion)
	}
	clock, err := w.s.authorityNow(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	expiry, err := time.Parse(time.RFC3339Nano, op.ExpiresAt)
	if err != nil || !clock.Before(expiry) {
		return out, conflict(workBuddyReauthorize)
	}
	before, opBefore := row.Version, op.Version
	row.Version++
	op.Version++
	row.State = "disconnected"
	row.GrantGeneration++
	row.Ciphertext = ""
	row.ExpiresAt = ""
	row.ActiveOperationID = ""
	row.LastError = ""
	row.UpdatedAt = automationStamp(clock)
	op.State = "cancelled"
	op.RequestHash = hash
	op.UpdatedAt = row.UpdatedAt
	if err = w.saveRowTx(ctx, auth, row, before); err != nil {
		return out, err
	}
	if err = w.saveOpTx(ctx, auth, op, opBefore); err != nil {
		return out, err
	}
	if err = w.s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, workBuddyCommandReceipt{op.ID, row.ID}); err != nil {
		return out, err
	}
	if err = w.s.commandActivity(ctx, auth, "workbuddy_authorization_denied", row.ID, "WorkBuddy authorization declined before exchange"); err != nil {
		return out, err
	}
	if err = auth.tx.Commit(); err != nil {
		return out, err
	}
	return w.connectionView(row, true), nil
}
