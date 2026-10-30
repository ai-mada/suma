package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
)

// List deliberately has no exact total. Each page is a fresh authorized read,
// ordered by stable ID, rather than a claimed cross-request database snapshot.
func (w *WorkBuddyConnections) List(ctx context.Context, actor Actor, project string, limit int, cursor string) (out WorkBuddyConnectionPage, err error) {
	if w == nil || w.s == nil || !handoffID(project, false) || limit < 1 || limit > 50 || len(cursor) > 4096 {
		return out, invalid(workBuddyInvalid)
	}
	scope := digest(stringify([]string{actor.credential.tenantID, project, actor.credential.subjectID}))
	after := ""
	if cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(cursor)
		var value []string
		if e != nil || base64.RawURLEncoding.EncodeToString(raw) != cursor || json.Unmarshal(raw, &value) != nil || len(value) != 2 || value[0] != scope || !handoffID(value[1], false) {
			return out, invalid(workBuddyInvalid)
		}
		after = value[1]
	}
	err = w.s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		write, e := w.currentWrite(auth)
		if e != nil {
			return e
		}
		out = WorkBuddyConnectionPage{ProjectID: project, Configured: w.configured, AvailableScopes: append([]string{}, w.app.Scopes...), CanAuthorize: write && w.configured && len(w.key) == 32, Items: []WorkBuddyConnection{}, Limit: limit}
		rows, e := auth.tx.QueryContext(auth.ctx, w.s.q(`SELECT id FROM suma_workbuddy_connections WHERE tenant_id=? AND project_id=? AND owner_id=? AND id>? ORDER BY id ASC LIMIT ?`), auth.target.tenantID, project, auth.currentPrincipal.ID, after, limit+1)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		closeErr := rows.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		if len(ids) > limit {
			ids = ids[:limit]
			raw, _ := json.Marshal([]string{scope, ids[len(ids)-1]})
			out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
		}
		// Close the first cursor before issuing exact-ID reads on SQLite's one
		// transaction connection. No secret ciphertext is projected to the client.
		for _, id := range ids {
			row, e := w.rowTx(auth.ctx, auth.tx, auth.target.tenantID, project, id, false)
			if e != nil {
				return e
			}
			if row.OwnerID != auth.currentPrincipal.ID {
				return missing()
			}
			out.Items = append(out.Items, w.connectionView(row, write))
		}
		return nil
	})
	if err != nil {
		return WorkBuddyConnectionPage{}, err
	}
	return out, nil
}

func (w *WorkBuddyConnections) Get(ctx context.Context, actor Actor, project, id string) (out WorkBuddyConnection, err error) {
	if w == nil || w.s == nil || !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(workBuddyInvalid)
	}
	err = w.s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		row, e := w.rowTx(auth.ctx, auth.tx, auth.target.tenantID, project, id, false)
		if e != nil {
			return e
		}
		if row.OwnerID != auth.currentPrincipal.ID {
			return missing()
		}
		write, e := w.currentWrite(auth)
		if e == nil {
			out = w.connectionView(row, write)
		}
		return e
	})
	if err != nil {
		return WorkBuddyConnection{}, err
	}
	return out, nil
}

// Authorization returns an ephemeral URL only to the exact initiating browser
// credential and a current writer. It never returns a code or provider token.
func (w *WorkBuddyConnections) Authorization(ctx context.Context, actor Actor, project, id string) (out WorkBuddyAuthorization, err error) {
	if w == nil || w.s == nil || !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(workBuddyInvalid)
	}
	err = w.s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeBrowserProjectWrite, func(auth *authorizedCommand) error {
		op, e := w.opTx(auth.ctx, auth.tx, auth.target.tenantID, project, id, false)
		if e != nil {
			return e
		}
		if op.Kind != "authorization" || op.OwnerID != auth.currentPrincipal.ID {
			return missing()
		}
		row, e := w.rowTx(auth.ctx, auth.tx, auth.target.tenantID, project, op.ConnectionID, false)
		if e != nil {
			return e
		}
		if row.OwnerID != auth.currentPrincipal.ID {
			return missing()
		}
		out, e = w.workBuddyCommandAuthorizationView(auth.ctx, auth, row, op)
		return e
	})
	if err != nil {
		return WorkBuddyAuthorization{}, err
	}
	return out, nil
}

func (w *WorkBuddyConnections) currentWrite(read *authorizedCommand) (bool, error) {
	_, err := w.s.authorizeTx(read.ctx, read.tx, read.actor, read.target, purposeBrowserProjectWrite)
	if err == nil {
		return true, nil
	}
	var failure *APIError
	if errors.As(err, &failure) && failure.Status == 403 {
		return false, nil
	}
	return false, err
}
