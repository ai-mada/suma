package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/store"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func runUsageEvidenceCLI(args []string, command ...string) (int, string, string) {
	var out, stderr bytes.Buffer
	all := append(append([]string(nil), args...), "usage")
	code := Execute(context.Background(), append(all, command...), nil, &out, &stderr)
	return code, out.String(), stderr.String()
}

func TestUsageEvidenceCLIReadsActualLedgerPages(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open("sqlite", filepath.Join(dir, "usage-cli.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := &platform.Service{DB: db, Dialect: "sqlite", DataDir: dir}
	if err := service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	engine, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	user, token, err := service.Setup(ctx, map[string]any{
		"name": "Usage operator", "email": "usage@example.test",
		"password": "fixture-usage-password", "project": "Usage ledger fixture",
	})
	if err != nil {
		t.Fatal(err)
	}
	var project string
	if err := db.QueryRowContext(ctx, "SELECT id FROM suma_projects WHERE tenant_id=?", user.TenantID).Scan(&project); err != nil {
		t.Fatal(err)
	}

	const (
		maxUsage    = int64(9007199254740991)
		originalAt  = "2026-11-06T02:00:00Z"
		lateAt      = "2026-11-06T02:00:00.1Z"
		earlyAt     = "2026-11-06T01:00:00Z"
		privateText = "private-ledger-fixture-text"
	)
	originalSHA := strings.Repeat("a", 64)
	zero, measured := int64(0), maxUsage
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range []struct{ id, month, state, stamp string }{
		{"attempt-new", "2026-11", "settled", originalAt},
		{"attempt-old", "2026-11", "settled", earlyAt},
		{"attempt-unpriced", "2026-11", "unpriced", lateAt},
		{"attempt-prior-month", "2026-10", "settled", "2026-10-06T02:00:00Z"},
	} {
		attempt := platform.UsageAttempt{
			ID: entry.id, ProjectID: project, Version: 2, RunID: "run-" + entry.id, NodeID: "node-one",
			Attempt: 1, Fence: 1, WorkerID: "worker-one", ToolProfileID: "profile-one",
			ProfileSHA256: strings.Repeat("b", 64), Model: "fixture-model", EndpointOrigin: "https://example.test", Currency: "USD",
			State: entry.state, CreatedAt: entry.stamp, UpdatedAt: entry.stamp,
			UsageState: "unreported", UsageSource: "none",
		}
		if entry.state == "settled" {
			attempt.UsageState, attempt.UsageSource = "complete", "tool_evidence"
			attempt.Usage = &toolcontract.Usage{InputTokens: &measured, OutputTokens: &zero}
			attempt.ReportedTokens, attempt.EstimatedCostMicros = &measured, &zero
			attempt.SourceResultSHA256 = originalSHA
		}
		raw, err := json.Marshal(map[string]any{"attempt": attempt})
		if err != nil {
			t.Fatal(err)
		}
		exec("INSERT INTO suma_usage_attempts(id,tenant_id,project_id,period,version,state,unresolved,run_id,node_id,attempt,fence,created_at,payload) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)",
			attempt.ID, user.TenantID, project, entry.month, attempt.Version, attempt.State, 0, attempt.RunID, attempt.NodeID, attempt.Attempt, attempt.Fence, attempt.CreatedAt, string(raw))
	}
	for _, fact := range []struct{ id, kind, digest, stamp, payload string }{
		{"fact-original", "result", originalSHA, originalAt, fmt.Sprintf(`{"stdout":%q,"tool_evidence":{"usage_state":"complete","usage":{"input_tokens":%d,"output_tokens":0}}}`, privateText, maxUsage)},
		{"fact-late", "result", strings.Repeat("c", 64), lateAt, fmt.Sprintf(`{"stdout":%q}`, privateText)},
		{"fact-resolution", "resolution", strings.Repeat("d", 64), earlyAt, fmt.Sprintf(`{"reason":%q}`, privateText)},
		{"fact-confirmation", "confirmation", strings.Repeat("e", 64), earlyAt, fmt.Sprintf(`{"source_ref":%q,"usage":{"input_tokens":12,"output_tokens":0}}`, privateText)},
	} {
		exec("INSERT INTO suma_usage_facts(id,tenant_id,project_id,attempt_id,kind,identity_sha256,created_at,payload) VALUES(?,?,?,?,?,?,?,?)",
			fact.id, user.TenantID, project, "attempt-new", fact.kind, fact.digest, fact.stamp, fact.payload)
	}
	// A write to any accounting table during these reads would fail the request.
	for _, table := range []string{"attempts", "facts", "accounts", "account_guards", "policies", "rates"} {
		for _, operation := range []string{"INSERT", "UPDATE", "DELETE"} {
			exec(fmt.Sprintf("CREATE TRIGGER no_write_%s_%s BEFORE %s ON suma_usage_%s BEGIN SELECT RAISE(ABORT, 'usage CLI must read only'); END",
				table, operation, operation, table))
		}
	}

	handler := (&platform.Server{Service: service, Engine: engine, WebDir: dir}).Handler()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.ContentLength != 0 || !strings.HasPrefix(r.URL.Path, "/api/v1/usage/attempts") {
			t.Error("unexpected usage request", r.Method, r.URL.Path, r.ContentLength)
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	args := []string{"--control", server.URL, "--project", project, "--token", token, "--expected-user", user.ID, "--expected-organization", user.TenantID}
	read := func(target any, command ...string) string {
		t.Helper()
		code, out, stderr := runUsageEvidenceCLI(args, command...)
		if code != 0 || stderr != "" || json.Unmarshal([]byte(out), target) != nil {
			t.Fatalf("%v failed: exit %d, %q, %q", command, code, out, stderr)
		}
		if strings.Contains(out, token) || strings.Contains(out, privateText) {
			t.Fatal("private source escaped usage read")
		}
		return out
	}

	var attempts platform.UsageAttemptPage
	raw := read(&attempts, "attempts", "--month", "2026-11", "--state", "settled", "--limit", "1")
	if attempts.Total != 2 || attempts.NextCursor != "attempt-new" || len(attempts.Items) != 1 {
		t.Fatal("unexpected attempt page", attempts)
	}
	attempt := attempts.Items[0]
	var wire struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		t.Fatal(err)
	}
	if attempt.ID != "attempt-new" || attempt.ProjectID != project || attempt.EstimatedCostMicros == nil || *attempt.EstimatedCostMicros != 0 ||
		attempt.ReservedCostMicros != nil || string(wire.Items[0]["reserved_cost_micros"]) != "null" ||
		string(wire.Items[0]["reported_tokens"]) != "9007199254740991" {
		t.Fatal("attempt scope, zero, unknown or exact counter changed", raw)
	}
	read(&attempts, "attempts", "--month", "2026-11", "--state", "settled", "--limit", "1", "--before", attempts.NextCursor)
	if attempts.Total != 2 || attempts.NextCursor != "" || len(attempts.Items) != 1 || attempts.Items[0].ID != "attempt-old" {
		t.Fatal("unexpected next attempt page", attempts)
	}

	var facts platform.UsageAttemptFactPage
	read(&facts, "evidence", "attempt-new", "--limit", "2")
	if facts.ProjectID != project || facts.AttemptID != "attempt-new" || facts.NextCursor != "fact-original" || len(facts.Items) != 2 {
		t.Fatal("unexpected evidence page", facts)
	}
	late, original := facts.Items[0], facts.Items[1]
	if late.ID != "fact-late" || late.CreatedAt != lateAt || late.OriginalResult || late.Usage != nil || late.UsageState != "unreported" ||
		original.ID != "fact-original" || original.CreatedAt != originalAt || !original.OriginalResult || original.IdentitySHA256 != originalSHA ||
		original.Usage == nil || original.Usage.InputTokens == nil || *original.Usage.InputTokens != maxUsage ||
		original.Usage.OutputTokens == nil || *original.Usage.OutputTokens != 0 {
		t.Fatal("evidence order, source or measured counters changed", facts)
	}
	// Use a fresh decode target so omitted fields cannot inherit the prior page.
	cursor := facts.NextCursor
	facts = platform.UsageAttemptFactPage{}
	read(&facts, "evidence", "attempt-new", "--limit", "2", "--before", cursor)
	if facts.NextCursor != "" || len(facts.Items) != 2 || facts.Items[0].ID != "fact-resolution" ||
		facts.Items[0].OriginalResult || facts.Items[0].Usage != nil || facts.Items[0].UsageState != "" ||
		facts.Items[1].ID != "fact-confirmation" || facts.Items[1].Kind != "confirmation" ||
		facts.Items[1].OriginalResult || facts.Items[1].UsageState != "complete" {
		t.Fatal("unexpected next evidence page", facts)
	}
	read(&facts, "evidence", "attempt-old")
	if facts.AttemptID != "attempt-old" || facts.NextCursor != "" || facts.Items == nil || len(facts.Items) != 0 {
		t.Fatal("empty evidence history changed", facts)
	}
	if code, out, _ := runUsageEvidenceCLI(args, "evidence", "attempt-old", "--before", "fact-original"); code != 1 || out != "" {
		t.Fatal("cursor from another attempt was accepted", code, out)
	}
	wrongUser := append(append([]string(nil), args...), "--expected-user", strings.Repeat("f", 32))
	if code, out, stderr := runUsageEvidenceCLI(wrongUser, "evidence", "attempt-new"); code != 1 || out != "" || !strings.Contains(stderr, "view_context_changed") {
		t.Fatal("account mismatch was not reported", code, out, stderr)
	}
}

func TestUsageEvidenceCLIRejectsUnverifiedPages(t *testing.T) {
	const (
		token       = "fixture-evidence-session"
		privateText = "private-model-prompt"
	)
	result := fmt.Sprintf(`{"id":"fact-one","kind":"result","created_at":"2026-11-06T02:00:00Z","identity_sha256":%q,"original_result":true,"usage_state":"partial","usage":{"input_tokens":0}}`, strings.Repeat("a", 64))
	page := fmt.Sprintf(`{"project_id":"project-one","attempt_id":"attempt-one","items":[%s],"next_cursor":""}`, result)
	for _, tc := range []struct {
		name, response, wantCode string
		status                   int
	}{
		{"trusted page", page, "", http.StatusOK},
		{"wrong project", strings.Replace(page, "project-one", "other-project", 1), "usage_evidence_invalid", http.StatusOK},
		{"wrong attempt", strings.Replace(page, "attempt-one", "other-attempt", 1), "usage_evidence_invalid", http.StatusOK},
		{"unexpected prompt", strings.Replace(page, `"original_result":true`, `"original_result":true,"raw_prompt":"`+privateText+`"`, 1), "usage_evidence_invalid", http.StatusOK},
		{"duplicate attempt scope", strings.Replace(page, `"attempt_id":`, `"attempt_id":"other-attempt","attempt_id":`, 1), "usage_evidence_invalid", http.StatusOK},
		{"unbound cursor", strings.Replace(page, `"next_cursor":""`, `"next_cursor":"other-fact"`, 1), "usage_evidence_invalid", http.StatusOK},
		{"unsafe counter", strings.Replace(page, `"input_tokens":0`, `"input_tokens":9007199254740992`, 1), "usage_evidence_invalid", http.StatusOK},
		{"unrecognized usage field", strings.Replace(page, `"input_tokens":0`, `"input_tokens":0,"completion":"`+privateText+`"`, 1), "usage_evidence_invalid", http.StatusOK},
		{"server error details", fmt.Sprintf(`{"error":{"code":"error","message":%q,"details":{"credential":%q}}}`, privateText, token), "forbidden", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query()
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/usage/attempts/attempt-one/facts" ||
					query.Get("project_id") != "project-one" || query.Get("limit") != "1" || len(query) != 2 ||
					r.Header.Get("Authorization") != "Bearer "+token ||
					r.Header.Get("X-Suma-Expected-User") != strings.Repeat("b", 32) {
					t.Error("unexpected scoped evidence request", r.Method, r.URL)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.response)
			}))
			defer server.Close()
			args := []string{"--control", server.URL, "--project", "project-one", "--token", token, "--expected-user", strings.Repeat("b", 32), "--expected-organization", ""}
			code, out, stderr := runUsageEvidenceCLI(args, "evidence", "attempt-one", "--limit", "1")
			if tc.wantCode == "" {
				var got platform.UsageAttemptFactPage
				if code != 0 || stderr != "" || json.Unmarshal([]byte(out), &got) != nil || len(got.Items) != 1 || got.Items[0].ID != "fact-one" {
					t.Fatalf("trusted page failed: exit %d, %q, %q", code, out, stderr)
				}
				return
			}
			var diagnostic struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if code != 1 || out != "" || json.Unmarshal([]byte(stderr), &diagnostic) != nil || diagnostic.Error.Code != tc.wantCode ||
				strings.Contains(stderr, token) || strings.Contains(stderr, privateText) {
				t.Fatalf("unverified page was not safely rejected: exit %d, %q, %q", code, out, stderr)
			}
		})
	}
}

func TestUsageEvidenceCLIRejectsInvalidInputsBeforeNetwork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Error("invalid usage input reached the control plane", r.URL)
	}))
	defer server.Close()
	args := []string{"--control", server.URL, "--project", "project-one", "--token", "fixture-token", "--expected-user", strings.Repeat("c", 32), "--expected-organization", ""}
	for _, tc := range []struct {
		name    string
		global  []string
		command []string
	}{
		{"project required", []string{"--project", ""}, []string{"attempts"}},
		{"browser token required", []string{"--token", ""}, []string{"attempts"}},
		{"original account required", []string{"--expected-user", ""}, []string{"attempts"}},
		{"strict month", nil, []string{"attempts", "--month", "2026-13"}},
		{"strict state", nil, []string{"attempts", "--state", "unknown"}},
		{"limit bounds", nil, []string{"attempts", "--limit", "101"}},
		{"duplicate options", nil, []string{"attempts", "--limit", "1", "--limit", "2"}},
		{"attempt required", nil, []string{"evidence"}},
		{"attempt path escape", nil, []string{"evidence", "../another"}},
		{"invalid cursor", nil, []string{"evidence", "attempt-one", "--before", "bad\nvalue"}},
		{"unsupported evidence filter", nil, []string{"evidence", "attempt-one", "--state", "settled"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			global := append(append([]string(nil), args...), tc.global...)
			code, out, stderr := runUsageEvidenceCLI(global, tc.command...)
			if code != 2 || out != "" || stderr == "" {
				t.Fatalf("invalid invocation accepted: exit %d, %q, %q", code, out, stderr)
			}
		})
	}
}
