package dag

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func appendHistoryData(t *testing.T, e *Engine, r Run, node string, data ...string) []int64 {
	t.Helper()
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var seq int64
	if err = tx.QueryRow(`SELECT COALESCE(MAX(sequence),0) FROM dag_events`).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	out := []int64{}
	for _, raw := range data {
		seq++
		if _, err = tx.Exec(`INSERT INTO dag_events(sequence,tenant_id,project_id,run_id,node_id,event_type,at_time,data)VALUES(?,?,?,?,?,'tool_progress','original-time',?)`, seq, "t", "p", r.ID, node, raw); err != nil {
			t.Fatal(err)
		}
		out = append(out, seq)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return out
}

func historyRaw(a *Assignment, payload string) string {
	return fmt.Sprintf(`{"attempt":%d,"fence":%d,"worker_id":%q,"tool_event":%s}`, a.Attempt, a.Fence, a.WorkerID, payload)
}

func historySnapshot(t *testing.T, e *Engine) string {
	t.Helper()
	var all []byte
	for _, table := range []string{"dag_runs", "dag_nodes", "dag_attempts", "dag_events", "dag_operations", "dag_resource_ledger", "dag_resource_limits", "dag_reservations"} {
		rows, err := e.db.Query(`SELECT * FROM ` + table)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		lines := []string{}
		for rows.Next() {
			v := make([]any, len(cols))
			p := make([]any, len(cols))
			for i := range v {
				p[i] = &v[i]
			}
			if err = rows.Scan(p...); err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(v)
			lines = append(lines, string(b))
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		b, _ := json.Marshal(lines)
		all = append(all, b...)
	}
	return fmt.Sprintf("%x", sha256.Sum256(all))
}

func TestAttemptToolHistoryProgressRetriesAllPagesAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	e := openEngine(t, path)
	n := toolNode()
	n.Idempotent = true
	n.MaxRetries = 1
	if _, err := e.Publish(ctx, "t", "p", Workflow{ID: "wf", Name: "wf", Nodes: []Node{n}}); err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRun(ctx, "t", "p", "wf", "create")
	if err != nil {
		t.Fatal(err)
	}
	event := tc.Event{SchemaVersion: 1, CommandID: "native-agent", ProfileSHA256: strings.Repeat("a", 64), Protocol: "suma-jsonl-v1", Kind: "message", Message: "raw tool text"}
	one := claim(t, e)
	if one == nil {
		t.Fatal("missing first attempt")
	}
	for i := 0; i < 140; i++ {
		if err = e.Progress(ctx, "t", "p", *one, fmt.Sprintf("earlier-%d", i), "", &event); err != nil {
			t.Fatal(err)
		}
	}
	if err = e.Complete(ctx, "t", "p", Result{RunID: one.RunID, NodeID: one.NodeID, Attempt: one.Attempt, Fence: one.Fence, WorkerID: one.WorkerID, ExitCode: 1, FailureClass: "transient", OperationID: "fail"}); err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Exec(`UPDATE dag_nodes SET wake_at=1 WHERE run_id=?`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Exec(`UPDATE dag_wakeups SET due_at=1 WHERE run_id=?`, r.ID); err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	two := claim(t, e)
	if two == nil || two.Attempt != 2 {
		t.Fatalf("retry %#v", two)
	}
	for i := 0; i < 65; i++ {
		if err = e.Progress(ctx, "t", "p", *two, fmt.Sprintf("later-%d", i), "", &event); err != nil {
			t.Fatal(err)
		}
	}
	before := historySnapshot(t, e)
	page, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, n.ID, 2, 50, "")
	if err != nil || len(page.Items) != 50 || !page.HasMore || page.PageCursor == "" {
		t.Fatalf("first %+v %v", page, err)
	}
	first := page
	var last int64
	count := 0
	for _, v := range page.Items {
		if v.Sequence <= last {
			t.Fatal("unordered")
		}
		last = v.Sequence
		count++
	}
	// Append after the frozen window and restart the engine before continuing.
	if err = e.Progress(ctx, "t", "p", *two, "append-after-window", "", &event); err != nil {
		t.Fatal(err)
	}
	afterAppend := historySnapshot(t, e)
	if err = e.db.Close(); err != nil {
		t.Fatal(err)
	}
	e = openEngine(t, path)
	next, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, n.ID, 2, 50, first.NextCursor)
	if err != nil || next.Watermark != first.Watermark || len(next.Items) != 15 || next.HasMore {
		t.Fatalf("next %+v %v", next, err)
	}
	for _, v := range next.Items {
		if v.Sequence <= last || v.Sequence > first.Watermark {
			t.Fatal("gap/append leak")
		}
		last = v.Sequence
		count++
	}
	if count != 65 {
		t.Fatal(count)
	}
	back, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, n.ID, 2, 50, first.PageCursor)
	if err != nil || len(back.Items) != 50 || back.Watermark != first.Watermark || back.PageCursor != first.PageCursor {
		t.Fatalf("back %+v %v", back, err)
	}
	old, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, n.ID, 1, 100, "")
	if err != nil || len(old.Items) != 100 || !old.HasMore {
		t.Fatal(old, err)
	}
	old2, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, n.ID, 1, 100, old.NextCursor)
	if err != nil || len(old2.Items) != 40 || old2.HasMore {
		t.Fatal(old2, err)
	}
	if before == afterAppend || historySnapshot(t, e) != afterAppend {
		t.Fatal("read/restart changed persisted domain state")
	}
	fresh, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, n.ID, 2, 100, "")
	if err != nil || len(fresh.Items) != 66 || fresh.Watermark <= first.Watermark {
		t.Fatal(fresh, err)
	}
}

func TestAttemptToolHistoryBoundedForeignScanAndZeroWindow(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	a := claim(t, e)
	empty, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 50, "")
	if err != nil || empty.Watermark != 0 || empty.HasMore {
		t.Fatal(empty, err)
	}
	foreign := make([]string, 1001)
	for i := range foreign {
		foreign[i] = `{"attempt":2,"fence":2,"worker_id":"other"}`
	}
	seqs := appendHistoryData(t, e, r, "a", foreign...)
	match := appendHistoryData(t, e, r, "a", historyRaw(a, `{"future":true}`))[0]
	old, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 50, empty.PageCursor)
	if err != nil || old.Watermark != 0 || len(old.Items) != 0 || old.HasMore {
		t.Fatal(old, err)
	}
	first, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 50, "")
	if err != nil || len(first.Items) != 0 || !first.HasMore || first.ScannedThrough != seqs[999] || first.Watermark != match {
		t.Fatal(first, err)
	}
	next, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 50, first.NextCursor)
	if err != nil || len(next.Items) != 1 || next.Items[0].Sequence != match || next.HasMore {
		t.Fatal(next, err)
	}
}

func TestAttemptToolHistoryStrictRawAttributionAndInnerFallback(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	a := claim(t, e)
	valid := historyRaw(a, `{"future_schema":99,"usage":1.2300e+5}`)
	bad := []string{`null`, `{`, `[]`, strings.Replace(valid, `"attempt":1`, `"attempt":1.0`, 1), strings.Replace(valid, `"attempt":1`, `"attempt":1e0`, 1), strings.Replace(valid, `"attempt":1`, `"attempt":"1"`, 1), strings.Replace(valid, `"attempt":1`, `"attempt":null`, 1), strings.Replace(valid, `"attempt":1`, `"attempt":9007199254740992`, 1), strings.Replace(valid, `"attempt":1`, `"attempt":1,"Attempt":1`, 1), strings.Replace(valid, `"attempt":1`, `"attempt":1,"attempt":1`, 1), strings.Replace(valid, `"fence":1`, `"fence":1.0`, 1), strings.Replace(valid, `"worker_id":"worker"`, `"worker_id":null`, 1), strings.Replace(valid, `"worker_id":"worker"`, `"worker_id":"Worker"`, 1)}
	appendHistoryData(t, e, r, "a", bad...)
	want := appendHistoryData(t, e, r, "a", valid, historyRaw(a, `null`), fmt.Sprintf(`{"attempt":1,"fence":%d,"worker_id":"worker"}`, a.Fence))
	before := historySnapshot(t, e)
	page, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 100, "")
	if err != nil || len(page.Items) != 3 {
		t.Fatalf("items %d %v %+v", len(page.Items), err, page)
	}
	for i, v := range page.Items {
		if v.Sequence != want[i] || v.At != "original-time" {
			t.Fatal(v)
		}
	}
	raw, _ := json.Marshal(page.Items[0])
	if !strings.Contains(string(raw), `1.2300e+5`) {
		t.Fatalf("number lexeme lost: %s", raw)
	}
	if before != historySnapshot(t, e) {
		t.Fatal("read rewrote raw data")
	}
}

func TestAttemptToolHistoryNullWorkerCannotProveEmptyDurableWorker(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	a := claim(t, e)
	if _, err := e.db.Exec(`UPDATE dag_attempts SET worker_id='' WHERE run_id=?`, r.ID); err != nil {
		t.Fatal(err)
	}
	appendHistoryData(t, e, r, "a", fmt.Sprintf(`{"attempt":1,"fence":%d,"worker_id":null}`, a.Fence))
	page, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 50, "")
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("null is not an exact worker string: %+v %v", page, err)
	}
	b, _ := base64.RawURLEncoding.DecodeString(page.PageCursor)
	raw := strings.Replace(string(b), `"worker":""`, `"worker":null`, 1)
	if _, err = e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 50, base64.RawURLEncoding.EncodeToString([]byte(raw))); !errors.Is(err, ErrInvalid) {
		t.Fatalf("null cursor worker accepted: %v", err)
	}
}

func TestAttemptToolHistoryRawDataIsOriginalAndLegacyReadsStayUnchanged(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	a := claim(t, e)
	raw := fmt.Sprintf("{ \"attempt\": 1, \"fence\": %d, \"worker_id\": \"worker\", \"tool_event\": {\"message\":\"\\ud800\",\"x\":1,\"x\":2,\"number\":1.2300e+5} }", a.Fence)
	appendHistoryData(t, e, r, "a", raw)
	page, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 50, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	b, _ := json.Marshal(page.Items[0])
	var envelope map[string]json.RawMessage
	if err = json.Unmarshal(b, &envelope); err != nil {
		t.Fatal(err)
	}
	var original string
	if err = json.Unmarshal(envelope["raw_data"], &original); err != nil || original != raw {
		t.Fatalf("raw disclosure changed original SQL text: %q != %q (%v)", original, raw, err)
	}
	legacy, err := e.Events(ctx, "t", "p", r.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(legacy)
	if strings.Contains(string(b), `"raw_data"`) {
		t.Fatal("legacy Event output gained new raw field")
	}
}

func TestAttemptToolHistoryInvalidSelectedUnicodeNeverBecomesWorkerIdentity(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	a := claim(t, e)
	if _, err := e.db.Exec(`UPDATE dag_attempts SET worker_id=? WHERE run_id=?`, "�", r.ID); err != nil {
		t.Fatal(err)
	}
	appendHistoryData(t, e, r, "a", fmt.Sprintf(`{"attempt":1,"fence":%d,"worker_id":"\ud800"}`, a.Fence))
	page, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 50, "")
	if err != nil || len(page.Items) != 0 {
		t.Fatal("invalid unicode acquired worker identity", page, err)
	}
	b, _ := base64.RawURLEncoding.DecodeString(page.PageCursor)
	raw := strings.Replace(string(b), `"worker":"�"`, `"worker":"\ud800"`, 1)
	if _, err = e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 50, base64.RawURLEncoding.EncodeToString([]byte(raw))); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid cursor unicode acquired worker identity", err)
	}
}

func TestAttemptToolHistoryCursorStrictScopeAndMarkers(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	a := claim(t, e)
	appendHistoryData(t, e, r, "a", historyRaw(a, `null`), historyRaw(a, `null`))
	page, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := base64.RawURLEncoding.DecodeString(page.NextCursor)
	original := string(b)
	mutants := []string{"!", page.NextCursor + "=", strings.Repeat("a", 4097)}
	for _, change := range []string{strings.Replace(original, `"version":1`, `"version":2`, 1), strings.Replace(original, `"attempt":1`, `"attempt":1e0`, 1), strings.Replace(original, `"attempt":1`, `"attempt":1,"attempt":1`, 1), strings.Replace(original, `"attempt":1`, `"attempt":1,"extra":0`, 1), strings.Replace(original, `"project":"p"`, `"project":"other"`, 1), strings.Replace(original, `"worker":"worker"`, `"worker":"other"`, 1), strings.Replace(original, `"last_scanned":`, `"last_scanned":9007199254740992,"old":`, 1), strings.Replace(original, fmt.Sprintf(`"watermark":%d`, page.Watermark), `"watermark":900`, 1), strings.Replace(original, fmt.Sprintf(`"last_scanned":%d`, page.ScannedThrough), `"last_scanned":1`, 1)} {
		mutants = append(mutants, base64.RawURLEncoding.EncodeToString([]byte(change)))
	}
	for _, cursor := range mutants {
		if _, err = e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 1, cursor); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted invalid cursor %s: %v", cursor, err)
		}
	}
	for _, scope := range []struct {
		tenant, project, run, node string
		attempt                    int
	}{{"foreign", "p", r.ID, "a", 1}, {"t", "other", r.ID, "a", 1}, {"t", "p", r.ID, "missing", 1}, {"t", "p", r.ID, "a", 2}} {
		if _, err = e.AttemptToolEventsPage(ctx, scope.tenant, scope.project, scope.run, scope.node, scope.attempt, 50, ""); !errors.Is(err, ErrNotFound) {
			t.Fatal(scope, err)
		}
	}
}

func TestAttemptToolHistoryByteBudgetDoesNotSkipAndOversizeFails(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	a := claim(t, e)
	payload := `{"message":"` + strings.Repeat("x", 700000) + `"}`
	seq := appendHistoryData(t, e, r, "a", historyRaw(a, payload), historyRaw(a, payload), historyRaw(a, payload))
	first, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 50, "")
	if err != nil || len(first.Items) != 2 || first.ScannedThrough != seq[1] || !first.HasMore {
		t.Fatalf("byte first %d %d %v", len(first.Items), first.ScannedThrough, err)
	}
	second, err := e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 50, first.NextCursor)
	if err != nil || len(second.Items) != 1 || second.Items[0].Sequence != seq[2] || second.HasMore {
		t.Fatal(second, err)
	}
	appendHistoryData(t, e, r, "a", historyRaw(a, `{"message":"`+strings.Repeat("x", 2*1024*1024)+`"}`))
	if _, err = e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 50, ""); err != nil {
		t.Fatal("earlier page must remain readable", err)
	}
	first, err = e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.AttemptToolEventsPage(ctx, "t", "p", r.ID, "a", 1, 50, first.NextCursor); err == nil {
		t.Fatal("oversized selected event silently dropped")
	}
}

func TestAttemptToolHistorySavedResultExactIdentity(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	a := claim(t, e)
	done(t, e, a, 0)
	var original string
	if err := e.db.QueryRow(`SELECT result FROM dag_attempts WHERE run_id=?`, r.ID).Scan(&original); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"run_id", "node_id", "attempt", "fence", "worker_id"} {
		var m map[string]any
		json.Unmarshal([]byte(original), &m)
		if field == "attempt" || field == "fence" {
			m[field] = 99
		} else {
			m[field] = "foreign"
		}
		b, _ := json.Marshal(m)
		if _, err := e.db.Exec(`UPDATE dag_attempts SET result=? WHERE run_id=?`, string(b), r.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := e.GetAttempt(ctx, "t", "p", r.ID, "a", 1); err == nil {
			t.Fatal("promoted mismatched receipt", field)
		}
		var saved string
		e.db.QueryRow(`SELECT result FROM dag_attempts WHERE run_id=?`, r.ID).Scan(&saved)
		if saved != string(b) {
			t.Fatal("read rewrote receipt")
		}
	}
	for _, raw := range []string{`{"Run_ID":"foreign","Node_ID":"a","attempt":1,"fence":1,"worker_id":"worker"}`, `{"attempt":1,"fence":1,"worker_id":"worker","exit_code":0}`} {
		if _, err := e.db.Exec(`UPDATE dag_attempts SET result=? WHERE run_id=?`, raw, r.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := e.GetAttempt(ctx, "t", "p", r.ID, "a", 1); err == nil {
			t.Fatal("malformed runner-shaped receipt silently became unreported", raw)
		}
	}
	for _, tc := range []struct{ raw, source string }{{original, "runner_result"}, {`{"outcome":"failed","reason":"raw manual reason"}`, "manual_reconciliation"}, {`{"child_run_id":"child"}`, "child_workflow"}, {"", "unreported"}} {
		e.db.Exec(`UPDATE dag_attempts SET result=? WHERE run_id=?`, tc.raw, r.ID)
		got, err := e.GetAttempt(ctx, "t", "p", r.ID, "a", 1)
		if err != nil || got.ResultSource != tc.source {
			t.Fatal(got, err)
		}
		if (got.Result != nil) != (tc.source == "runner_result") {
			t.Fatal("borrowed runner result", got)
		}
	}
}

func TestAttemptToolHistorySavedReceiptRejectsIdentityAliasesInEitherOrder(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	a := claim(t, e)
	done(t, e, a, 0)
	canonical := fmt.Sprintf(`"run_id":%q,"node_id":%q,"attempt":%d,"fence":%d,"worker_id":%q`, r.ID, a.NodeID, a.Attempt, a.Fence, a.WorkerID)
	for _, field := range []struct{ alias, matching, foreign string }{
		{"Run_ID", fmt.Sprintf("%q", r.ID), `"foreign"`},
		{"Node_ID", fmt.Sprintf("%q", a.NodeID), `"foreign"`},
		{"Attempt", fmt.Sprint(a.Attempt), "99"},
		{"Fence", fmt.Sprint(a.Fence), "99"},
		{"Worker_ID", fmt.Sprintf("%q", a.WorkerID), `"foreign"`},
	} {
		for _, value := range []string{field.matching, field.foreign} {
			for _, aliasFirst := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/first=%t", field.alias, value, aliasFirst), func(t *testing.T) {
					alias := fmt.Sprintf("%q:%s", field.alias, value)
					body := canonical + "," + alias
					if aliasFirst {
						body = alias + "," + canonical
					}
					raw := "{" + body + "}"
					if _, err := e.db.Exec(`UPDATE dag_attempts SET result=? WHERE run_id=?`, raw, r.ID); err != nil {
						t.Fatal(err)
					}
					got, err := e.GetAttempt(ctx, "t", "p", r.ID, a.NodeID, a.Attempt)
					if err == nil || got.Result != nil || got.ResultSource == "runner_result" {
						t.Fatalf("aliased receipt promoted: %+v %v", got, err)
					}
					var saved string
					if err = e.db.QueryRow(`SELECT result FROM dag_attempts WHERE run_id=?`, r.ID).Scan(&saved); err != nil || saved != raw {
						t.Fatal("read changed original aliased receipt", err)
					}
				})
			}
		}
	}
}

func TestAttemptToolHistoryPortableIndexRestartDefinitionAndSeek(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	e := openEngine(t, path)
	if err := migrateEventIndexes(e.db, "sqlite"); err != nil {
		t.Fatal(err)
	}
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2010; i++ {
		node := "other"
		if i > 2000 {
			node = "n"
		}
		if _, err = tx.Exec(`INSERT INTO dag_events(sequence,tenant_id,project_id,run_id,node_id,event_type,at_time,data)VALUES(?,'t','p','r',?,'tool_progress','','{}')`, i, node); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Exec(`ANALYZE dag_events`); err != nil {
		t.Fatal(err)
	}
	rows, err := e.db.Query(`EXPLAIN QUERY PLAN `+attemptCandidateQuery, "t", "p", "r", "n", 0, 3000)
	if err != nil {
		t.Fatal(err)
	}
	plan := ""
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail
	}
	rows.Close()
	if !strings.Contains(plan, "dag_events_node_type_sequence") || strings.Contains(plan, "TEMP B-TREE") {
		t.Fatal(plan)
	}
	before := historySnapshot(t, e)
	if _, err = New(e.db, "sqlite"); err != nil {
		t.Fatal(err)
	}
	if historySnapshot(t, e) != before {
		t.Fatal("index startup/restart changed domain rows")
	}
	if _, err = New(e.db, "sqlite"); err != nil {
		t.Fatal(err)
	}
	for _, ddl := range []string{`CREATE INDEX dag_events_node_type_sequence ON dag_events(tenant_id,project_id,run_id,sequence)`, `CREATE INDEX dag_events_node_type_sequence ON dag_events(tenant_id,project_id,run_id,node_id,event_type,sequence) WHERE event_type='tool_progress'`} {
		if _, err = e.db.Exec(`DROP INDEX dag_events_node_type_sequence`); err != nil {
			t.Fatal(err)
		}
		if _, err = e.db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
		if err = migrateEventIndexes(e.db, "sqlite"); err == nil {
			t.Fatal("silently accepted incompatible index")
		}
	}
}
