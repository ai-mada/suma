package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

const maxStreamSequence int64 = 9007199254740991

// The connection budget and bounded synchronous writes avoid an unbounded
// goroutine/event queue for slow or abandoned clients. Keys disappear on close.
var runStreams = struct {
	sync.Mutex
	total  int
	actors map[string]int
}{actors: map[string]int{}}

func acquireRunStream(actor string) (func(), bool) {
	runStreams.Lock()
	defer runStreams.Unlock()
	if runStreams.total >= 64 || runStreams.actors[actor] >= 4 {
		return nil, false
	}
	runStreams.total++
	runStreams.actors[actor]++
	return func() {
		runStreams.Lock()
		defer runStreams.Unlock()
		runStreams.total--
		runStreams.actors[actor]--
		if runStreams.actors[actor] == 0 {
			delete(runStreams.actors, actor)
		}
	}, true
}

func parseStreamSequence(raw string) (int64, error) {
	if raw == "" {
		return 0, invalid("事件游标必须为非负安全整数")
	}
	for _, c := range raw {
		if c < '0' || c > '9' {
			return 0, invalid("事件游标必须为非负安全整数")
		}
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n > maxStreamSequence {
		return 0, invalid("事件游标必须为非负安全整数")
	}
	return n, nil
}

func runStreamCursor(r *http.Request) (int64, error) {
	q, err := httpQuery(r)
	if err != nil {
		return 0, err
	}
	var cursor int64
	hasAfter := false
	if values, ok := q["after"]; ok {
		if len(values) != 1 {
			return 0, invalid("事件游标不可重复")
		}
		hasAfter = true
		cursor, err = parseStreamSequence(values[0])
		if err != nil {
			return 0, err
		}
	}
	headers := r.Header.Values("Last-Event-ID")
	if len(headers) > 1 {
		return 0, invalid("事件游标不可重复")
	}
	if len(headers) == 1 {
		last, e := parseStreamSequence(headers[0])
		if e != nil {
			return 0, e
		}
		if hasAfter && last != cursor {
			return 0, invalid("after 与 Last-Event-ID 冲突")
		}
		cursor = last
	}
	return cursor, nil
}

func httpQuery(r *http.Request) (map[string][]string, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, invalid("查询参数格式错误")
	}
	for key := range q {
		if key != "project_id" && key != "after" {
			return nil, invalid("事件连接只接受 project_id 和 after")
		}
	}
	if ids, ok := q["project_id"]; ok && len(ids) != 1 {
		return nil, invalid("项目参数不可重复")
	}
	return q, nil
}

// runStreamRead gives authorization and persisted-event selection one SQL
// linearization point under the same organization lock as governance writes.
// It never holds a transaction while waiting on the network. A second guard
// immediately before each write prevents a replay batch bypassing revocation.
func (s *Server) runStreamRead(ctx context.Context, token string, expected Principal, scope, project, run string, after int64, events bool) ([]dag.Event, error) {
	tx, err := s.Service.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = s.Service.authorizeProjectTx(ctx, tx, expected, project, false, false); err != nil {
		return nil, err
	}
	var user, tenant, expiry string
	err = tx.QueryRowContext(ctx, s.Service.q(`SELECT user_id,tenant_id,expires_at FROM suma_sessions WHERE token_hash=?`), digest(token)).Scan(&user, &tenant, &expiry)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &APIError{401, "session_expired", "登录已失效", nil}
	}
	if err != nil {
		return nil, err
	}
	expires, parseErr := time.Parse(time.RFC3339Nano, expiry)
	if parseErr != nil || !time.Now().Before(expires) || user != expected.ID || tenant != expected.TenantID {
		return nil, &APIError{401, "session_expired", "登录已失效", nil}
	}
	var kind, bound string
	err = tx.QueryRowContext(ctx, s.Service.q(`SELECT kind,project_id FROM suma_token_scopes WHERE token_hash=?`), digest(token)).Scan(&kind, &bound)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if kind != scope || kind != "" && (kind != "api" || bound != project) {
		return nil, forbidden()
	}
	var exists string
	if err = tx.QueryRowContext(ctx, s.Service.q(`SELECT id FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), expected.TenantID, project, run).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return nil, missing()
	}
	if err != nil {
		return nil, err
	}
	out := []dag.Event{}
	if events {
		if after > 0 {
			var cursor int64
			if err = tx.QueryRowContext(ctx, s.Service.q(`SELECT sequence FROM dag_events WHERE tenant_id=? AND project_id=? AND run_id=? AND sequence=?`), expected.TenantID, project, run, after).Scan(&cursor); errors.Is(err, sql.ErrNoRows) {
				return nil, &APIError{400, "cursor_scope_mismatch", "事件游标不属于当前运行", nil}
			}
			if err != nil {
				return nil, err
			}
		}
		rows, e := tx.QueryContext(ctx, s.Service.q(`SELECT sequence,run_id,node_id,event_type,at_time,data FROM dag_events WHERE tenant_id=? AND project_id=? AND run_id=? AND sequence>? ORDER BY sequence LIMIT 128`), expected.TenantID, project, run, after)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var event dag.Event
			var data string
			if err = rows.Scan(&event.Sequence, &event.RunID, &event.NodeID, &event.Type, &event.At, &data); err != nil {
				rows.Close()
				return nil, err
			}
			if event.Sequence > maxStreamSequence {
				rows.Close()
				return nil, invalid("持久事件序号超过客户端安全范围")
			}
			if err = json.Unmarshal([]byte(data), &event.Data); err != nil {
				rows.Close()
				return nil, err
			}
			if event.Data == nil {
				event.Data = map[string]any{}
			}
			out = append(out, event)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) runEventStream(w http.ResponseWriter, r *http.Request, p Principal, scope, project, runID string) {
	after, err := runStreamCursor(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	token := tokenOf(r)
	release, ok := acquireRunStream(p.TenantID + ":" + p.ID)
	if !ok {
		w.Header().Set("Retry-After", "3")
		writeError(w, &APIError{429, "stream_limit", "实时连接已达上限，请稍后重连", nil})
		return
	}
	defer release()
	events, err := s.runStreamRead(ctx, token, p, scope, project, runID, after, true)
	if err != nil {
		writeError(w, err)
		return
	}
	controller := http.NewResponseController(w)
	if _, ok := w.(http.Flusher); !ok {
		writeError(w, &APIError{503, "stream_unavailable", "当前连接不支持实时事件", nil})
		return
	}
	rotationDeadline, _ := ctx.Deadline()
	idleDeadline := rotationDeadline.Add(2 * time.Second)
	if err := controller.SetWriteDeadline(idleDeadline); err != nil {
		writeError(w, &APIError{503, "stream_unavailable", "当前连接不支持有界实时事件写入", nil})
		return
	}
	writeFailed := false
	defer func() {
		// net/http writes the terminating chunk after this handler returns.
		// Give that final flush a bounded grace period, without reopening a
		// connection whose actual write already failed its backpressure limit.
		if !writeFailed {
			deadline := time.Now().Add(2 * time.Second)
			if idleDeadline.Before(deadline) {
				deadline = idleDeadline
			}
			_ = controller.SetWriteDeadline(deadline)
		}
	}()
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func(data string) bool {
		if ctx.Err() != nil {
			return false
		}
		deadline := time.Now().Add(2 * time.Second)
		if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
			deadline = end
		}
		if e := controller.SetWriteDeadline(deadline); e != nil {
			writeFailed = true
			return false
		}
		if _, e := fmt.Fprint(w, data); e != nil {
			writeFailed = true
			return false
		}
		if e := controller.Flush(); e != nil {
			writeFailed = true
			return false
		}
		// A 2s write deadline must not expire during the 5s heartbeat interval.
		// Restore the bounded idle deadline immediately after a successful flush.
		if e := controller.SetWriteDeadline(idleDeadline); e != nil {
			writeFailed = true
			return false
		}
		return true
	}
	streamError := func(e error) {
		status := 500
		code := "stream_unavailable"
		var api *APIError
		if errors.As(e, &api) {
			status = api.Status
			code = api.Code
		}
		data, _ := json.Marshal(map[string]any{"status": status, "code": code})
		send("event: stream_error\ndata: " + string(data) + "\n\n")
	}
	if !send(": connected\n\n") {
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	heartbeat := time.Now()
	for {
		for _, event := range events {
			if _, err = s.runStreamRead(ctx, token, p, scope, project, runID, after, false); err != nil {
				streamError(err)
				return
			}
			data, e := json.Marshal(event)
			if e != nil {
				return
			}
			if !send("id: " + strconv.FormatInt(event.Sequence, 10) + "\nevent: run_event\ndata: " + string(data) + "\n\n") {
				return
			}
			after = event.Sequence
		}
		// Drain another bounded persisted page immediately, including terminal runs.
		// A terminal snapshot does not imply that all final events have arrived.
		if len(events) == 128 {
			events, err = s.runStreamRead(ctx, token, p, scope, project, runID, after, true)
			if err != nil {
				streamError(err)
				return
			}
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		events, err = s.runStreamRead(ctx, token, p, scope, project, runID, after, true)
		if err != nil {
			streamError(err)
			return
		}
		if len(events) == 0 && time.Since(heartbeat) >= 5*time.Second {
			if !send(": heartbeat\n\n") {
				return
			}
			heartbeat = time.Now()
		}
	}
}
