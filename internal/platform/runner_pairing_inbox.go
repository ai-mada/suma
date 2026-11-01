package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type runnerPairingInboxQuery struct{ status, q, limit, cursor string }
type runnerPairingInboxCursor struct {
	User    string `json:"user"`
	Tenant  string `json:"tenant"`
	Project string `json:"project"`
	Scope   string `json:"scope"`
	Created string `json:"created"`
	ID      string `json:"id"`
}

// Only the three code-owned pairing payload fields enter SQL expressions.
func (s *Service) runnerPairingInboxField(field string) string {
	switch s.Dialect {
	case "postgres":
		return `(CAST(p.payload AS JSONB)->>'` + field + `')`
	case "oceanbase":
		return `JSON_UNQUOTE(JSON_EXTRACT(p.payload,'$.` + field + `'))`
	default:
		return `CAST(json_extract(p.payload,'$.` + field + `') AS VARCHAR(100))`
	}
}

func runnerPairingInboxReadCursor(value string, p Principal, project, scope string) (runnerPairingInboxCursor, error) {
	var out runnerPairingInboxCursor
	if value == "" {
		return out, nil
	}
	if len(value) > 4096 {
		return out, runnerPairingError(400, "invalid")
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || RejectDuplicateReleaseJSON(raw) != nil {
		return out, runnerPairingError(400, "invalid")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || out.User != p.ID || out.Tenant != p.TenantID || out.Project != project || out.Scope != scope || !desktopWorkspaceHex(out.ID, 32) {
		return out, runnerPairingError(400, "invalid")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return out, runnerPairingError(400, "invalid")
	}
	at, err := time.Parse(time.RFC3339, out.Created)
	if err != nil || automationStamp(at) != out.Created {
		return out, runnerPairingError(400, "invalid")
	}
	return out, nil
}

func (s *Service) listRunnerPairings(parent context.Context, actor Actor, project string, request runnerPairingInboxQuery, control string) (RunnerPairingPage, error) {
	var out RunnerPairingPage
	if !handoffID(project, false) {
		return out, runnerPairingError(400, "invalid")
	}
	ctx, tx, close, err := s.beginAuthorityTx(parent)
	if err != nil {
		return out, err
	}
	defer close()
	defer tx.Rollback()
	auth, err := s.artifactAdminTx(ctx, tx, actor, project)
	if err != nil {
		return out, err
	}
	// Decode filters and cursor only after current project administration.
	status := request.status
	if status == "" {
		status = "active"
	}
	switch status {
	case "active", "waiting", "proposed", "approved", "rejected", "expired", "all":
	default:
		return out, runnerPairingError(400, "invalid")
	}
	q := strings.TrimSpace(request.q)
	if !utf8.ValidString(q) || len(q) > 200 {
		return out, runnerPairingError(400, "invalid")
	}
	limit := 20
	if request.limit != "" {
		parsed, e := strconv.ParseInt(request.limit, 10, 32)
		if e != nil || parsed < 1 || parsed > 100 {
			return out, runnerPairingError(400, "invalid")
		}
		limit = int(parsed)
	}
	scope := digest(stringify([]any{"runner-pairings", status, q, limit}))
	cursor, err := runnerPairingInboxReadCursor(request.cursor, auth.currentPrincipal, project, scope)
	if err != nil {
		return out, err
	}
	clock, err := s.authorityNow(ctx, tx)
	if err != nil {
		return out, err
	}
	asOf := automationStamp(clock)
	out = RunnerPairingPage{ProjectID: project, Status: status, Q: q, Items: []RunnerPairing{}, Limit: limit, AsOf: asOf}
	where := `p.tenant_id=? AND p.project_id=?`
	args := []any{auth.target.tenantID, project}
	switch status {
	case "active":
		where += ` AND p.state IN ('waiting','proposed') AND p.expires_at>?`
		args = append(args, asOf)
	case "waiting", "proposed":
		where += ` AND p.state=? AND p.expires_at>?`
		args = append(args, status, asOf)
	case "expired":
		where += ` AND (p.state='expired' OR (p.state IN ('waiting','proposed') AND p.expires_at<=?))`
		args = append(args, asOf)
	case "approved", "rejected":
		where += ` AND p.state=?`
		args = append(args, status)
	}
	if q != "" {
		escape := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
		pattern := "%" + escape.Replace(strings.ToLower(q)) + "%"
		where += ` AND (LOWER(p.id) LIKE ? ESCAPE '!' OR LOWER(COALESCE(` + s.runnerPairingInboxField("device_name") + `,'')) LIKE ? ESCAPE '!' OR LOWER(COALESCE(` + s.runnerPairingInboxField("worker_id") + `,'')) LIKE ? ESCAPE '!')`
		args = append(args, pattern, pattern, pattern)
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_runner_pairings p WHERE `+where), args...).Scan(&out.Total); err != nil {
		return RunnerPairingPage{}, err
	}
	created := s.runnerPairingInboxField("created_at")
	if request.cursor != "" {
		where += ` AND (` + created + `<? OR (` + created + `=? AND p.id<?))`
		args = append(args, cursor.Created, cursor.Created, cursor.ID)
	}
	args = append(args, limit+1)
	rows, err := tx.QueryContext(ctx, s.q(`SELECT `+runnerPairingColumns+` FROM suma_runner_pairings p WHERE `+where+` ORDER BY `+created+` DESC,p.id DESC LIMIT ?`), args...)
	if err != nil {
		return RunnerPairingPage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		record, e := scanRunnerPairing(rows)
		if e != nil {
			return RunnerPairingPage{}, e
		}
		at, e := time.Parse(time.RFC3339, record.CreatedAt)
		if e != nil || automationStamp(at) != record.CreatedAt {
			return RunnerPairingPage{}, runnerPairingError(409, "changed")
		}
		view, e := runnerPairingView(record, clock, true, control)
		if e != nil {
			return RunnerPairingPage{}, e
		}
		out.Items = append(out.Items, view)
	}
	if err = rows.Err(); err != nil {
		return RunnerPairingPage{}, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		raw, _ := json.Marshal(runnerPairingInboxCursor{User: auth.currentPrincipal.ID, Tenant: auth.target.tenantID, Project: project, Scope: scope, Created: last.CreatedAt, ID: last.ID})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return out, nil
}
