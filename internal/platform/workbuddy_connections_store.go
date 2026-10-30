package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const workBuddyConnectionColumns = "id,tenant_id,project_id,owner_id,client_id,open_id,version,scopes_json,state,grant_generation,token_generation,expires_at,created_at,updated_at,last_error,app_digest,ciphertext,active_operation_id"
const workBuddyOAuthOperationColumns = "id,tenant_id,project_id,connection_id,owner_id,kind,state,request_hash,credential_digest,state_hash,code_hash,app_digest,scopes_json,version,connection_version,grant_generation,token_generation,expires_at,created_at,updated_at,last_error"

func (w *WorkBuddyConnections) rowTx(ctx context.Context, q authorityQuery, tenant, project, id string, current bool) (out workBuddyConnectionRow, err error) {
	query := `SELECT ` + workBuddyConnectionColumns + ` FROM suma_workbuddy_connections WHERE tenant_id=? AND project_id=? AND id=?`
	if current {
		query = w.s.currentSQL(query)
	}
	var scopes string
	err = q.QueryRowContext(ctx, w.s.q(query), tenant, project, id).Scan(&out.ID, &out.TenantID, &out.ProjectID, &out.OwnerID, &out.ClientID, &out.OpenID, &out.Version, &scopes, &out.State, &out.GrantGeneration, &out.TokenGeneration, &out.ExpiresAt, &out.CreatedAt, &out.UpdatedAt, &out.LastError, &out.AppDigest, &out.Ciphertext, &out.ActiveOperationID)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if json.Unmarshal([]byte(scopes), &out.Scopes) != nil {
		return out, fmt.Errorf("%s", workBuddyUnavailable)
	}
	out.Scopes, err = workBuddyCanonicalScopes(out.Scopes, true)
	out.AllowedActions = []string{}
	if err != nil || !workBuddyStoredConnectionValid(out) {
		return workBuddyConnectionRow{}, fmt.Errorf("%s", workBuddyUnavailable)
	}
	return out, nil
}
func workBuddyStoredConnectionValid(r workBuddyConnectionRow) bool {
	if r.ID == "" || r.TenantID == "" || r.ProjectID == "" || r.OwnerID == "" || r.ClientID == "" || r.Version < 1 || r.Version > math.MaxInt32 || r.GrantGeneration < 1 || r.TokenGeneration < 0 || len(r.AppDigest) != 64 {
		return false
	}
	switch r.State {
	case "authorizing", "connected", "refreshing", "authorization_unknown", "refresh_unknown", "reauthorization_required", "disconnected":
	default:
		return false
	}
	if _, err := workBuddyCanonicalScopes(r.Scopes, true); err != nil {
		return false
	}
	if r.State == "connected" {
		if r.Ciphertext == "" || r.OpenID == "" || len(r.Scopes) == 0 || r.TokenGeneration < 1 {
			return false
		}
		if _, err := time.Parse(time.RFC3339Nano, r.ExpiresAt); err != nil {
			return false
		}
	}
	return true
}
func (w *WorkBuddyConnections) storageAuthority(auth *authorizedCommand, tenant, project, owner string) error {
	if w == nil || w.s == nil || auth == nil || auth.actor.kind != actorBrowser || auth.target.tenantID != tenant || auth.target.projectID != project || auth.currentPrincipal.ID != owner {
		return forbidden()
	}
	return auth.assertTx(auth.tx, auth.target, purposeBrowserProjectWrite)
}
func (w *WorkBuddyConnections) saveRowTx(ctx context.Context, auth *authorizedCommand, row workBuddyConnectionRow, beforeVersion int) error {
	if err := w.storageAuthority(auth, row.TenantID, row.ProjectID, row.OwnerID); err != nil {
		return err
	}
	if beforeVersion < 0 || row.Version != beforeVersion+1 || !workBuddyStoredConnectionValid(row) {
		return invalid(workBuddyInvalid)
	}
	scopes, _ := workBuddyCanonicalScopes(row.Scopes, true)
	values := []any{row.ID, row.TenantID, row.ProjectID, row.OwnerID, row.ClientID, row.OpenID, row.Version, stringify(scopes), row.State, row.GrantGeneration, row.TokenGeneration, row.ExpiresAt, row.CreatedAt, row.UpdatedAt, row.LastError, row.AppDigest, row.Ciphertext, row.ActiveOperationID}
	if beforeVersion == 0 {
		_, err := auth.tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_connections(`+workBuddyConnectionColumns+`) VALUES(`+strings.TrimRight(strings.Repeat("?,", len(values)), ",")+`)`), values...)
		return err
	}
	// Immutable audience/owner and version remain part of the actual CAS, not
	// only a preliminary read that could refer to another row.
	query := `UPDATE suma_workbuddy_connections SET client_id=?,open_id=?,version=?,scopes_json=?,state=?,grant_generation=?,token_generation=?,expires_at=?,created_at=?,updated_at=?,last_error=?,app_digest=?,ciphertext=?,active_operation_id=? WHERE tenant_id=? AND project_id=? AND id=? AND owner_id=? AND version=?`
	args := append([]any{}, values[4:]...)
	args = append(args, row.TenantID, row.ProjectID, row.ID, row.OwnerID, beforeVersion)
	result, err := auth.tx.ExecContext(ctx, w.s.q(query), args...)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return conflict(workBuddyVersion)
	}
	return nil
}
func (w *WorkBuddyConnections) opTx(ctx context.Context, q authorityQuery, tenant, project, id string, current bool) (out workBuddyOAuthOperation, err error) {
	query := `SELECT ` + workBuddyOAuthOperationColumns + ` FROM suma_workbuddy_oauth_operations WHERE tenant_id=? AND project_id=? AND id=?`
	if current {
		query = w.s.currentSQL(query)
	}
	err = q.QueryRowContext(ctx, w.s.q(query), tenant, project, id).Scan(&out.ID, &out.TenantID, &out.ProjectID, &out.ConnectionID, &out.OwnerID, &out.Kind, &out.State, &out.RequestHash, &out.CredentialDigest, &out.StateHash, &out.CodeHash, &out.AppDigest, &out.ScopesJSON, &out.Version, &out.ConnectionVersion, &out.GrantGeneration, &out.TokenGeneration, &out.ExpiresAt, &out.CreatedAt, &out.UpdatedAt, &out.LastError)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if !workBuddyStoredOperationValid(out) {
		return workBuddyOAuthOperation{}, fmt.Errorf("%s", workBuddyUnavailable)
	}
	return out, nil
}
func workBuddyStoredOperationValid(op workBuddyOAuthOperation) bool {
	if op.ID == "" || op.TenantID == "" || op.ProjectID == "" || op.ConnectionID == "" || op.OwnerID == "" || op.Version < 1 || op.Version > math.MaxInt32 || op.ConnectionVersion < 1 || op.GrantGeneration < 1 || op.TokenGeneration < 0 || len(op.AppDigest) != 64 || len(op.CredentialDigest) != 64 || len(op.RequestHash) != 64 {
		return false
	}
	if op.Kind != "authorization" && op.Kind != "refresh" {
		return false
	}
	switch op.State {
	case "pending", "exchanging", "succeeded", "unknown", "failed", "cancelled":
	default:
		return false
	}
	var scopes []string
	if json.Unmarshal([]byte(op.ScopesJSON), &scopes) != nil {
		return false
	}
	if _, err := workBuddyCanonicalScopes(scopes, false); err != nil {
		return false
	}
	return true
}
func (w *WorkBuddyConnections) saveOpTx(ctx context.Context, auth *authorizedCommand, op workBuddyOAuthOperation, beforeVersion int) error {
	if err := w.storageAuthority(auth, op.TenantID, op.ProjectID, op.OwnerID); err != nil {
		return err
	}
	if beforeVersion < 0 || op.Version != beforeVersion+1 || !workBuddyStoredOperationValid(op) {
		return invalid(workBuddyInvalid)
	}
	values := []any{op.ID, op.TenantID, op.ProjectID, op.ConnectionID, op.OwnerID, op.Kind, op.State, op.RequestHash, op.CredentialDigest, op.StateHash, op.CodeHash, op.AppDigest, op.ScopesJSON, op.Version, op.ConnectionVersion, op.GrantGeneration, op.TokenGeneration, op.ExpiresAt, op.CreatedAt, op.UpdatedAt, op.LastError}
	if beforeVersion == 0 {
		_, err := auth.tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_oauth_operations(`+workBuddyOAuthOperationColumns+`) VALUES(`+strings.TrimRight(strings.Repeat("?,", len(values)), ",")+`)`), values...)
		return err
	}
	columns := strings.Split(workBuddyOAuthOperationColumns, ",")[5:]
	assignments := []string{}
	for _, column := range columns {
		assignments = append(assignments, column+"=?")
	}
	query := `UPDATE suma_workbuddy_oauth_operations SET ` + strings.Join(assignments, ",") + ` WHERE tenant_id=? AND project_id=? AND id=? AND connection_id=? AND owner_id=? AND version=?`
	args := append([]any{}, values[5:]...)
	args = append(args, op.TenantID, op.ProjectID, op.ID, op.ConnectionID, op.OwnerID, beforeVersion)
	result, err := auth.tx.ExecContext(ctx, w.s.q(query), args...)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return conflict(workBuddyVersion)
	}
	return nil
}
func (w *WorkBuddyConnections) connectionView(row workBuddyConnectionRow, write bool) WorkBuddyConnection {
	out := row.WorkBuddyConnection
	out.Scopes = append([]string{}, row.Scopes...)
	out.AllowedActions = []string{}
	if !write {
		return out
	}
	// Local withdrawal must remain possible during configuration/key outages.
	// The caller has already proved the current owner and project write right.
	if row.State != "disconnected" && workBuddyVersionRoom(row.Version, 1) {
		out.AllowedActions = []string{"disconnect"}
	}
	if w == nil || !w.configured || len(w.key) != 32 || row.AppDigest != w.appDigest || row.ClientID != w.app.ClientID {
		return out
	}
	switch row.State {
	case "connected":
		if workBuddyStoredConnectionValid(row) {
			providerActions := []string{}
			if workBuddyConnectionAuthorizationCapacity(row) {
				providerActions = append(providerActions, "authorize")
			}
			allowed := map[string]bool{}
			for _, scope := range w.app.Scopes {
				allowed[scope] = true
			}
			within := true
			for _, scope := range row.Scopes {
				if !allowed[scope] {
					within = false
				}
			}
			if within && workBuddyConnectionExternalCapacity(row) {
				providerActions = append(providerActions, "refresh")
			}
			out.AllowedActions = append(providerActions, out.AllowedActions...)
		}
	case "authorization_unknown", "refresh_unknown", "reauthorization_required", "disconnected":
		if workBuddyConnectionAuthorizationCapacity(row) {
			out.AllowedActions = append([]string{"authorize"}, out.AllowedActions...)
		}
	}
	return out
}
