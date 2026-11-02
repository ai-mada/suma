package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/ai-mada/suma/internal/strictjson"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type poolPairingListQuery struct{ status, q, limit, cursor string }
type poolPairingCursor struct {
	User    string `json:"user"`
	Owner   string `json:"owner"`
	Pool    string `json:"pool"`
	Filter  string `json:"filter"`
	AsOf    string `json:"as_of"`
	Created string `json:"created"`
	ID      string `json:"id"`
}

func poolPairingReadCursor(raw string, p Principal, pool, filter string) (out poolPairingCursor, err error) {
	if raw == "" {
		return out, nil
	}
	if len(raw) > 4096 {
		return out, poolPairingError(400, "invalid")
	}
	data, e := base64.RawURLEncoding.DecodeString(raw)
	if e != nil || strictjson.Decode(data, &out, 4096) != nil || out.User != p.ID || out.Owner != p.TenantID || out.Pool != pool || out.Filter != filter || !executionPoolID.MatchString(out.ID) {
		return out, poolPairingError(400, "invalid")
	}
	asof, e := time.Parse(time.RFC3339Nano, out.AsOf)
	if e != nil || poolPairingStamp(asof) != out.AsOf {
		return out, poolPairingError(400, "invalid")
	}
	created, e := time.Parse(time.RFC3339Nano, out.Created)
	if e != nil || poolPairingStamp(created) != out.Created || created.After(asof) {
		return out, poolPairingError(400, "invalid")
	}
	return out, nil
}

// Lists only the owner Pool's indexed inbox. Current authorization is checked
// on every page; the first as_of excludes later creations, not later revocation.
func (s *Service) listPoolRunnerPairings(parent context.Context, actor Actor, poolID string, read func() (poolPairingListQuery, error), control string) (out PoolRunnerPairingPage, err error) {
	if !executionPoolID.MatchString(poolID) {
		return out, poolPairingError(400, "invalid")
	}
	ctx, tx, done, e := s.beginAuthorityTx(parent)
	if e != nil {
		return out, e
	}
	defer done()
	defer tx.Rollback()
	auth, pool, e := s.poolPairingAdminTx(ctx, tx, actor, poolID)
	if e != nil {
		return out, e
	}
	request, e := read()
	if e != nil {
		return out, e
	}
	status := request.status
	if status == "" {
		status = "active"
	}
	switch status {
	case "all", "active", "waiting", "proposed", "approved", "rejected", "expired":
	default:
		return out, poolPairingError(400, "invalid")
	}
	query := strings.TrimSpace(request.q)
	if !utf8.ValidString(request.q) || len(request.q) > 120 {
		return out, poolPairingError(400, "invalid")
	}
	limit := 20
	if request.limit != "" {
		n, e := strconv.Atoi(request.limit)
		if e != nil || n < 1 || n > 100 || strconv.Itoa(n) != request.limit {
			return out, poolPairingError(400, "invalid")
		}
		limit = n
	}
	filter := digest(stringify([]any{status, query, limit}))
	cursor, e := poolPairingReadCursor(request.cursor, auth.currentPrincipal, poolID, filter)
	if e != nil {
		return out, e
	}
	clock, e := s.authorityNow(ctx, tx)
	if e != nil {
		return out, e
	}
	asof := poolPairingStamp(clock)
	if request.cursor != "" {
		asof = cursor.AsOf
		if asof > poolPairingStamp(clock) {
			return out, poolPairingError(400, "invalid")
		}
	}
	out = PoolRunnerPairingPage{OwnerTenantID: pool.OwnerTenantID, PoolID: poolID, AsOf: asof, Items: []PoolRunnerPairing{}}
	where := `p.owner_tenant_id=? AND p.pool_id=? AND p.created_at<=?`
	args := []any{pool.OwnerTenantID, poolID, asof}
	now := poolPairingStamp(clock)
	switch status {
	case "active":
		where += ` AND p.state IN ('waiting','proposed') AND p.expires_at>?`
		args = append(args, now)
	case "waiting", "proposed":
		where += ` AND p.state=? AND p.expires_at>?`
		args = append(args, status, now)
	case "expired":
		where += ` AND p.state IN ('waiting','proposed') AND p.expires_at<=?`
		args = append(args, now)
	case "approved", "rejected":
		where += ` AND p.state=?`
		args = append(args, status)
	}
	if query != "" {
		escape := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
		pattern := "%" + escape.Replace(strings.ToLower(query)) + "%"
		where += ` AND (LOWER(p.id) LIKE ? ESCAPE '!' OR LOWER(COALESCE(` + s.runnerPairingInboxField("name") + `,'')) LIKE ? ESCAPE '!' OR LOWER(COALESCE(` + s.runnerPairingInboxField("device_name") + `,'')) LIKE ? ESCAPE '!' OR LOWER(COALESCE(` + s.runnerPairingInboxField("worker_id") + `,'')) LIKE ? ESCAPE '!')`
		args = append(args, pattern, pattern, pattern, pattern)
	}
	if e = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_pool_runner_pairings p WHERE `+where), args...).Scan(&out.Total); e != nil {
		return PoolRunnerPairingPage{}, e
	}
	if request.cursor != "" {
		where += ` AND (p.created_at<? OR (p.created_at=? AND p.id<?))`
		args = append(args, cursor.Created, cursor.Created, cursor.ID)
	}
	args = append(args, limit+1)
	rows, e := tx.QueryContext(ctx, s.q(`SELECT `+poolPairingColumns+` FROM suma_pool_runner_pairings p WHERE `+where+` ORDER BY p.created_at DESC,p.id DESC LIMIT ?`), args...)
	if e != nil {
		return PoolRunnerPairingPage{}, e
	}
	records := []poolPairingRecord{}
	for rows.Next() {
		row, e := scanPoolPairing(rows)
		if e != nil {
			rows.Close()
			return PoolRunnerPairingPage{}, e
		}
		records = append(records, row)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return PoolRunnerPairingPage{}, e
	}
	if len(records) > limit {
		records = records[:limit]
		last := records[limit-1]
		raw, _ := json.Marshal(poolPairingCursor{User: auth.currentPrincipal.ID, Owner: pool.OwnerTenantID, Pool: poolID, Filter: filter, AsOf: asof, Created: last.CreatedAt, ID: last.ID})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	for _, row := range records {
		view, e := s.poolPairingViewTx(ctx, tx, row, pool, clock, true, control)
		if e != nil {
			return PoolRunnerPairingPage{}, e
		}
		out.Items = append(out.Items, view)
	}
	return out, nil
}
