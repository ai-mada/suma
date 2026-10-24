package dag

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/toolcontract"
)

// AttemptToolEventPage is one bounded, durable window of original tool facts.
type AttemptToolEventPage struct {
	SchemaVersion  int     `json:"schema_version"`
	ProjectID      string  `json:"project_id"`
	RunID          string  `json:"run_id"`
	NodeID         string  `json:"node_id"`
	Attempt        int     `json:"attempt"`
	Fence          int64   `json:"fence"`
	WorkerID       string  `json:"worker_id"`
	Items          []Event `json:"items"`
	Watermark      int64   `json:"watermark"`
	ScannedThrough int64   `json:"scanned_through"`
	HasMore        bool    `json:"has_more"`
	PageCursor     string  `json:"page_cursor"`
	NextCursor     string  `json:"next_cursor,omitempty"`
}

const attemptSafeInteger int64 = 9007199254740991
const attemptCandidateQuery = `SELECT sequence,run_id,node_id,event_type,at_time,data FROM dag_events WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND event_type='tool_progress' AND sequence>? AND sequence<=? ORDER BY sequence ASC LIMIT 1001`

type attemptToolCursor struct {
	Version     int    `json:"version"`
	Tenant      string `json:"tenant"`
	Project     string `json:"project"`
	Run         string `json:"run"`
	Node        string `json:"node"`
	Attempt     int    `json:"attempt"`
	Fence       int64  `json:"fence"`
	Worker      string `json:"worker"`
	LastScanned int64  `json:"last_scanned"`
	Watermark   int64  `json:"watermark"`
}

func attemptReadError(key string, cause error) error {
	return dagDiagnostic(cause, "attempt_history."+key, nil)
}

// rawObject rejects duplicate keys before Go's map/struct decoder can erase them.
func rawObject(raw []byte) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return nil, fmt.Errorf("expected JSON object")
	}
	out := map[string]json.RawMessage{}
	for d.More() {
		k, err := d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := k.(string)
		if !ok {
			return nil, fmt.Errorf("invalid key")
		}
		if _, ok = out[key]; ok {
			return nil, fmt.Errorf("duplicate JSON key")
		}
		var v json.RawMessage
		if err = d.Decode(&v); err != nil {
			return nil, err
		}
		out[key] = v
	}
	if _, err = d.Token(); err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing JSON")
	}
	return out, nil
}

func safeLiteral(raw []byte, positive bool) (int64, bool) {
	s := string(raw)
	if s == "" || len(s) > 16 || len(s) > 1 && s[0] == '0' {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil && n <= attemptSafeInteger && (!positive || n > 0)
}

func provesAttempt(raw []byte, out AttemptToolEventPage) bool {
	m, err := rawObject(raw)
	if err != nil {
		return false
	}
	for k := range m {
		for _, name := range []string{"attempt", "fence", "worker_id"} {
			if strings.EqualFold(k, name) && k != name {
				return false
			}
		}
	}
	a, ok := safeLiteral(m["attempt"], true)
	if !ok || a != int64(out.Attempt) {
		return false
	}
	f, ok := safeLiteral(m["fence"], true)
	if !ok || f != out.Fence {
		return false
	}
	return exactJSONText(m["worker_id"], out.WorkerID)
}

func exactJSONText(raw json.RawMessage, expected string) bool {
	var value string
	return len(raw) > 0 && raw[0] == '"' && toolcontract.ValidJSONUnicode(raw) && json.Unmarshal(raw, &value) == nil && value == expected
}

func decodeAttemptToolCursor(raw string) (attemptToolCursor, error) {
	var c attemptToolCursor
	bad := func() (attemptToolCursor, error) {
		return c, attemptReadError("cursor", fmt.Errorf("%w: invalid attempt history cursor", ErrInvalid))
	}
	if len(raw) > 4096 {
		return bad()
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || base64.RawURLEncoding.EncodeToString(b) != raw {
		return bad()
	}
	m, err := rawObject(b)
	if err != nil {
		return bad()
	}
	for _, k := range []string{"version", "tenant", "project", "run", "node", "attempt", "fence", "worker", "last_scanned", "watermark"} {
		if _, ok := m[k]; !ok {
			return bad()
		}
	}
	// Exactly ten named fields: no unknowns or duplicate aliases.
	if len(m) != 10 {
		return bad()
	}
	for _, k := range []string{"tenant", "project", "run", "node", "worker"} {
		if len(m[k]) == 0 || m[k][0] != '"' || !toolcontract.ValidJSONUnicode(m[k]) {
			return bad()
		}
	}
	for _, k := range []string{"version", "attempt", "fence", "last_scanned", "watermark"} {
		if _, ok := safeLiteral(m[k], k == "version" || k == "attempt"); !ok {
			return bad()
		}
	}
	if json.Unmarshal(b, &c) != nil || c.Version != 1 || c.LastScanned > c.Watermark {
		return bad()
	}
	return c, nil
}

func (e *Engine) AttemptToolEventsPage(ctx context.Context, tenant, project, run, node string, attempt, limit int, cursor string) (AttemptToolEventPage, error) {
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return AttemptToolEventPage{}, err
	}
	defer tx.Rollback()
	out, err := e.AttemptToolEventsPageTx(ctx, tx, tenant, project, run, node, attempt, limit, cursor)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// AttemptToolEventsPageTx accepts the caller's authorization transaction. It
// never loads the run graph/result or changes persisted execution facts.
func (e *Engine) AttemptToolEventsPageTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string, attempt, limit int, cursor string) (AttemptToolEventPage, error) {
	out := AttemptToolEventPage{SchemaVersion: 1, ProjectID: project, RunID: run, NodeID: node, Attempt: attempt, Items: []Event{}}
	if err := scopeValid(tenant, project); err != nil {
		return out, err
	}
	if attempt < 1 || int64(attempt) > attemptSafeInteger || limit < 0 || limit > 100 {
		return out, attemptReadError("query", fmt.Errorf("%w: invalid attempt history query", ErrInvalid))
	}
	if limit == 0 {
		limit = 50
	}
	err := tx.QueryRowContext(ctx, e.q(`SELECT a.fence,a.worker_id FROM dag_attempts a JOIN dag_runs r ON r.id=a.run_id WHERE r.tenant_id=? AND r.project_id=? AND a.run_id=? AND a.node_id=? AND a.attempt=?`), tenant, project, run, node, attempt).Scan(&out.Fence, &out.WorkerID)
	if err == sql.ErrNoRows {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if out.Fence < 0 || out.Fence > attemptSafeInteger {
		return out, attemptReadError("identity", fmt.Errorf("invalid durable attempt identity"))
	}
	if cursor == "" {
		err = tx.QueryRowContext(ctx, e.q(`SELECT COALESCE(MAX(sequence),0) FROM dag_events WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND event_type='tool_progress'`), tenant, project, run, node).Scan(&out.Watermark)
		if err != nil {
			return out, err
		}
	} else {
		c, err := decodeAttemptToolCursor(cursor)
		if err != nil {
			return out, err
		}
		if c.Tenant != tenant || c.Project != project || c.Run != run || c.Node != node || c.Attempt != attempt || c.Fence != out.Fence || c.Worker != out.WorkerID {
			return out, attemptReadError("cursor", fmt.Errorf("%w: invalid attempt history cursor", ErrInvalid))
		}
		for _, seq := range []int64{c.LastScanned, c.Watermark} {
			if seq == 0 {
				continue
			}
			var marker int64
			err = tx.QueryRowContext(ctx, e.q(`SELECT sequence FROM dag_events WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND event_type='tool_progress' AND sequence=?`), tenant, project, run, node, seq).Scan(&marker)
			if err == sql.ErrNoRows {
				return out, attemptReadError("cursor", fmt.Errorf("%w: invalid attempt history cursor", ErrInvalid))
			}
			if err != nil {
				return out, err
			}
		}
		out.Watermark = c.Watermark
		out.ScannedThrough = c.LastScanned
	}
	out.PageCursor = cursor
	if cursor == "" {
		c := attemptToolCursor{1, tenant, project, run, node, attempt, out.Fence, out.WorkerID, 0, out.Watermark}
		b, err := json.Marshal(c)
		if err != nil {
			return out, err
		}
		out.PageCursor = base64.RawURLEncoding.EncodeToString(b)
		if len(out.PageCursor) > 4096 {
			return out, attemptReadError("cursor", fmt.Errorf("%w: invalid attempt history cursor", ErrInvalid))
		}
	}
	if out.Watermark < 0 || out.Watermark > attemptSafeInteger {
		return out, attemptReadError("identity", fmt.Errorf("invalid durable attempt identity"))
	}
	rows, err := tx.QueryContext(ctx, e.q(attemptCandidateQuery), tenant, project, run, node, out.ScannedThrough, out.Watermark)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	examined, used := 0, 2
	for rows.Next() {
		if examined == 1000 || len(out.Items) == limit {
			out.HasMore = true
			break
		}
		var ev Event
		var raw string
		if err = rows.Scan(&ev.Sequence, &ev.RunID, &ev.NodeID, &ev.Type, &ev.At, &raw); err != nil {
			return out, err
		}
		if ev.Sequence <= out.ScannedThrough || ev.Sequence > attemptSafeInteger {
			return out, attemptReadError("identity", fmt.Errorf("invalid durable attempt identity"))
		}
		if provesAttempt([]byte(raw), out) {
			if !utf8.ValidString(raw) {
				return out, attemptReadError("raw_encoding", fmt.Errorf("selected tool event is not valid UTF-8"))
			}
			ev.RawData = raw
			d := json.NewDecoder(strings.NewReader(raw))
			d.UseNumber()
			if err = d.Decode(&ev.Data); err != nil {
				return out, err
			}
			b, err := json.Marshal(ev)
			if err != nil {
				return out, err
			}
			if len(b) > 2*1024*1024 {
				return out, attemptReadError("event_size", fmt.Errorf("selected tool event exceeds 2 MiB"))
			}
			cost := len(b)
			if len(out.Items) > 0 {
				cost++
			}
			if used+cost > 4*1024*1024 {
				out.HasMore = true
				break
			}
			used += cost
			out.Items = append(out.Items, ev)
		}
		examined++
		out.ScannedThrough = ev.Sequence
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if out.HasMore {
		c := attemptToolCursor{1, tenant, project, run, node, attempt, out.Fence, out.WorkerID, out.ScannedThrough, out.Watermark}
		b, err := json.Marshal(c)
		if err != nil {
			return out, err
		}
		out.NextCursor = base64.RawURLEncoding.EncodeToString(b)
		if len(out.NextCursor) > 4096 {
			return out, attemptReadError("cursor", fmt.Errorf("%w: invalid attempt history cursor", ErrInvalid))
		}
	}
	return out, nil
}
