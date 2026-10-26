package dag

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	rc "github.com/ai-mada/suma/internal/releasecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type liveReconciliationTestHook func(context.Context, *sql.Tx, LiveReadOnlyCompletion, Result, Result) (bool, error)

func (f liveReconciliationTestHook) CheckLiveReadOnlyCompletionTx(ctx context.Context, tx *sql.Tx, scope LiveReadOnlyCompletion, previous, final Result) (bool, error) {
	return f(ctx, tx, scope, previous, final)
}

func liveReconciliationFixture(t *testing.T, pool bool) (*Engine, Run, *Assignment, Result, Result) {
	t.Helper()
	var e *Engine
	var r Run
	var acquire func() (*Assignment, error)
	if pool {
		var claim func(string) (*Assignment, error)
		e, r, claim = accountingPoolFixture(t)
		acquire = func() (*Assignment, error) { return claim("live-unknown-claim") }
	} else {
		e, r = fixture(t, Workflow{Nodes: []Node{toolNode(), {ID: "after", Type: "command", Command: []string{"test-command"}, DependsOn: []string{"agent"}}}})
		acquire = func() (*Assignment, error) {
			return e.ClaimWithOperation(ctx, "t", "p", "worker", "live-unknown-claim", []string{"command:native-agent"}, nil, nil)
		}
	}
	e.LeaseSeconds = 3600
	a, err := acquire()
	if err != nil || a == nil {
		t.Fatal("missing real leased attempt", a, err)
	}
	profile := tc.Profile{CommandID: "native-agent", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "fixture", ReadOnly: true, KnowledgeExtraction: &tc.KnowledgeExtractionCapability{Protocol: tc.KnowledgeExtractionProtocol, Model: "fixed", EndpointOrigin: "https://model.example"}}
	previous := Result{RunID: r.ID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "live-previous-unknown", ExitCode: -1, Error: "final response was not yet received", Output: "received fragment", OutcomeUnknown: true, ToolEvidence: &tc.Evidence{Profile: tc.CloneProfile(profile), Terminal: "unknown", EventCount: 1, UsageState: tc.UsageUnreported}}
	if err = e.Complete(ctx, "t", "p", previous); err != nil {
		t.Fatal(err)
	}
	in, out := int64(23), int64(17)
	final := Result{RunID: r.ID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "live-formal-final", Output: "formal raw output; domain schema is a separate decision", ToolEvidence: &tc.Evidence{Profile: profile, Terminal: "completed", EventCount: 4, Usage: &tc.Usage{InputTokens: &in, OutputTokens: &out}, UsageState: tc.UsageComplete}}
	return e, r, a, previous, final
}

func TestLiveReadOnlyCompletionAtomicReplayAndCurrentAuthority(t *testing.T) {
	for _, pool := range []bool{false, true} {
		t.Run(map[bool]string{false: "private", true: "pool"}[pool], func(t *testing.T) {
			e, r, a, previous, final := liveReconciliationFixture(t, pool)
			checks, commits, accounts, beforeCalls := 0, 0, 0, 0
			allow := true
			denied := errors.New("current authority revoked")
			before := func(*sql.Tx) error {
				beforeCalls++
				if !allow {
					return denied
				}
				return nil
			}
			e.AttemptReconciliation = liveReconciliationTestHook(func(_ context.Context, tx *sql.Tx, scope LiveReadOnlyCompletion, old, known Result) (bool, error) {
				checks++
				if scope.TenantID != "t" || scope.ProjectID != "p" || scope.RunID != r.ID || scope.NodeID != a.NodeID || scope.WorkerID != a.WorkerID || scope.Attempt != a.Attempt || scope.Fence != a.Fence || !old.OutcomeUnknown || known.OutcomeUnknown || known.Output != final.Output {
					t.Fatal("hook received a different fixed attempt or completion")
				}
				var state string
				if err := tx.QueryRow(`SELECT state FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?`, r.ID, a.NodeID, a.Attempt).Scan(&state); err != nil || state != "reconciling" {
					t.Fatal("proof did not precede durable result in its own tx", state, err)
				}
				scope.Spec.Config["goal"] = "hook-local mutation"
				known.ToolEvidence.Profile.KnowledgeExtraction.Model = "hook-local mutation"
				return true, nil
			})
			commit := func(tx *sql.Tx, state string) error {
				commits++
				if state != "succeeded" {
					t.Fatal(state)
				}
				return nil
			}
			e.AttemptAccounting = &accountingTestHook{result: func(_ context.Context, tx *sql.Tx, scope AccountingAttempt, known Result) error {
				accounts++
				if commits != 1 || known.ToolEvidence.Profile.KnowledgeExtraction.Model != "fixed" || scope.Spec.Config["goal"] != "inspect" {
					t.Fatal("proof mutation escaped or accounting preceded platform commit")
				}
				return nil
			}}
			for range 2 {
				if err := e.CompleteBound(ctx, "t", "p", final, before, commit); err != nil {
					t.Fatal("same current leased result could not reconcile/replay", err)
				}
			}
			allow = false
			if err := e.CompleteBound(ctx, "t", "p", final, before, commit); !errors.Is(err, denied) {
				t.Fatal("receipt bypassed current authority", err)
			}
			allow = true
			changed := final
			changed.Output = "changed under same operation"
			if err := e.CompleteBound(ctx, "t", "p", changed, before, commit); !errors.Is(err, ErrConflict) {
				t.Fatal("replay changed final content", err)
			}
			if checks != 1 || commits != 1 || accounts != 1 || beforeCalls != 4 {
				t.Fatal("proof/accounting effects replayed", checks, commits, accounts, beforeCalls)
			}
			attempt, readErr := e.GetAttempt(ctx, "t", "p", r.ID, a.NodeID, a.Attempt)
			if readErr != nil || attempt.ResultSource != "runner_result" || attempt.Result == nil || attempt.Result.Output != final.Output {
				t.Fatal("formal live completion was mislabeled as a provider observation", attempt.ResultSource, readErr)
			}
			var oldRaw, finalRaw, attemptRaw string
			if err := e.db.QueryRow(`SELECT original_result,resolution_result FROM dag_managed_resolutions WHERE run_id=? AND node_id=? AND attempt=?`, r.ID, a.NodeID, a.Attempt).Scan(&oldRaw, &finalRaw); err != nil || oldRaw != encoded(previous) || finalRaw != encoded(final) {
				t.Fatal("original/final proof was lost or mutated", err)
			}
			if err := e.db.QueryRow(`SELECT result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?`, r.ID, a.NodeID, a.Attempt).Scan(&attemptRaw); err != nil || attemptRaw != encoded(final) {
				t.Fatal("formal attempt final differs", err)
			}
			var events, active int
			if err := e.db.QueryRow(`SELECT COUNT(*) FROM dag_events WHERE run_id=? AND event_type='live_read_only_completion_reconciled'`, r.ID).Scan(&events); err != nil || events != 1 {
				t.Fatal("reconciliation event duplicated", events, err)
			}
			if err := e.db.QueryRow(`SELECT COUNT(*) FROM dag_resource_ledger WHERE run_id=? AND state='active'`, r.ID).Scan(&active); err != nil || active != 0 {
				t.Fatal("live completion retained process resources", active, err)
			}
			if pool {
				if err := e.db.QueryRow(`SELECT COUNT(*) FROM dag_execution_pool_slots WHERE run_id=?`, r.ID).Scan(&active); err != nil || active != 0 {
					t.Fatal("pool slot not released once", active, err)
				}
			} else {
				if next := claim(t, e); next == nil || next.NodeID != "after" || next.Attempt != 1 {
					t.Fatal("downstream not activated after formal completion", next)
				}
				if next := claim(t, e); next != nil {
					t.Fatal("duplicate downstream claim", next)
				}
			}
		})
	}
}

func TestLiveReadOnlyCompletionClosedGatesPreserveUnknown(t *testing.T) {
	for _, name := range []string{"nil_hook", "nil_authority", "nil_commit", "expired", "cancelling", "old_stopped", "old_known", "new_unknown", "new_failed", "new_error", "foreign_worker", "foreign_fence", "foreign_attempt", "released_execution_slot", "released_pool_slot", "not_read_only", "workspace_effect", "artifact_effect", "release_effect", "previous_effect", "invalid_tool_binding", "service_denied"} {
		t.Run(name, func(t *testing.T) {
			e, r, a, old, final := liveReconciliationFixture(t, name == "released_pool_slot")
			checks := 0
			e.AttemptReconciliation = liveReconciliationTestHook(func(context.Context, *sql.Tx, LiveReadOnlyCompletion, Result, Result) (bool, error) {
				checks++
				return name != "service_denied", nil
			})
			before := func(*sql.Tx) error { return nil }
			commit := func(*sql.Tx, string) error { t.Fatal("closed gate reached platform commit"); return nil }
			exec := func(q string, values ...any) {
				t.Helper()
				if _, err := e.db.Exec(q, values...); err != nil {
					t.Fatal(err)
				}
			}
			switch name {
			case "nil_hook":
				e.AttemptReconciliation = nil
			case "nil_authority":
				before = nil
			case "nil_commit":
				commit = nil
			case "expired":
				exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, r.ID, a.NodeID)
			case "cancelling":
				exec(`UPDATE dag_runs SET state='cancelling' WHERE id=?`, r.ID)
			case "old_stopped":
				old.Stopped = true
			case "old_known":
				old.OutcomeUnknown = false
			case "new_unknown":
				final.OutcomeUnknown = true
			case "new_failed":
				final.ExitCode = 1
			case "new_error":
				final.Error = "not a normal completion"
			case "foreign_worker":
				final.WorkerID = "foreign-worker"
			case "foreign_fence":
				final.Fence++
			case "foreign_attempt":
				final.Attempt++
			case "released_execution_slot":
				exec(`UPDATE dag_resource_ledger SET state='released' WHERE run_id=? AND name='execution_slots'`, r.ID)
			case "released_pool_slot":
				exec(`DELETE FROM dag_execution_pool_slots WHERE run_id=?`, r.ID)
			case "not_read_only":
				final.ToolEvidence.Profile.ReadOnly = false
			case "workspace_effect":
				final.WorkspaceEvidence = map[string]any{"effect": "must reconcile separately"}
			case "artifact_effect":
				final.ArtifactEvidence = &rc.ArtifactEvidence{}
			case "release_effect":
				final.ReleaseEvidence = &rc.ReleaseResult{}
			case "previous_effect":
				old.WorkspaceEvidence = map[string]any{"effect": "unknown write"}
			case "invalid_tool_binding":
				final.ToolEvidence.Profile.ProfileSHA256 = strings.Repeat("c", 64)
			}
			if name == "old_stopped" || name == "old_known" || name == "previous_effect" {
				exec(`UPDATE dag_attempts SET result=? WHERE run_id=? AND node_id=? AND attempt=?`, encoded(old), r.ID, a.NodeID, a.Attempt)
			}
			want := ErrStale
			if name == "invalid_tool_binding" {
				want = ErrInvalid
			}
			if err := e.CompleteBound(ctx, "t", "p", final, before, commit); !errors.Is(err, want) {
				t.Fatalf("closed gate got %v, want %v", err, want)
			}
			wantChecks := 0
			if name == "service_denied" {
				wantChecks = 1
			}
			if checks != wantChecks {
				t.Fatal("service proof preceded core/tool validation", checks, wantChecks)
			}
			var stored string
			if err := e.db.QueryRow(`SELECT result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?`, r.ID, a.NodeID, a.Attempt).Scan(&stored); err != nil || stored != encoded(old) {
				t.Fatal("closed gate replaced previous evidence", err)
			}
			for _, query := range []string{`SELECT COUNT(*) FROM dag_managed_resolutions WHERE run_id=?`, `SELECT COUNT(*) FROM dag_events WHERE run_id=? AND event_type='live_read_only_completion_reconciled'`, `SELECT COUNT(*) FROM dag_operations WHERE scope='result:'||? AND operation_id='live-formal-final'`} {
				var count int
				if err := e.db.QueryRow(query, r.ID).Scan(&count); err != nil || count != 0 {
					t.Fatal("closed gate persisted a resolution/event/receipt", count, err)
				}
			}
		})
	}
}

func TestLiveReadOnlyCompletionProofCommitAndAccountingRollback(t *testing.T) {
	for _, failure := range []string{"proof", "commit", "accounting"} {
		t.Run(failure, func(t *testing.T) {
			e, r, a, previous, final := liveReconciliationFixture(t, false)
			if _, err := e.db.Exec(`CREATE TABLE live_proof_fixture(id INTEGER PRIMARY KEY)`); err != nil {
				t.Fatal(err)
			}
			refused, fail := errors.New("proof transaction rejected"), true
			mark := func(tx *sql.Tx, id int, point string) error {
				if _, err := tx.Exec(`INSERT INTO live_proof_fixture(id)VALUES(?)`, id); err != nil {
					return err
				}
				if fail && failure == point {
					return refused
				}
				return nil
			}
			e.AttemptReconciliation = liveReconciliationTestHook(func(_ context.Context, tx *sql.Tx, _ LiveReadOnlyCompletion, _, _ Result) (bool, error) {
				return true, mark(tx, 1, "proof")
			})
			before := func(*sql.Tx) error { return nil }
			commit := func(tx *sql.Tx, _ string) error { return mark(tx, 2, "commit") }
			e.AttemptAccounting = &accountingTestHook{result: func(_ context.Context, tx *sql.Tx, _ AccountingAttempt, _ Result) error {
				return mark(tx, 3, "accounting")
			}}
			if err := e.CompleteBound(ctx, "t", "p", final, before, commit); !errors.Is(err, refused) {
				t.Fatal("proof failure did not abort result", err)
			}
			var stored string
			if err := e.db.QueryRow(`SELECT result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?`, r.ID, a.NodeID, a.Attempt).Scan(&stored); err != nil || stored != encoded(previous) {
				t.Fatal("failed transaction replaced original unknown", err)
			}
			for _, query := range []string{`SELECT COUNT(*) FROM live_proof_fixture`, `SELECT COUNT(*) FROM dag_managed_resolutions`, `SELECT COUNT(*) FROM dag_events WHERE event_type='live_read_only_completion_reconciled'`, `SELECT COUNT(*) FROM dag_operations WHERE operation_id='live-formal-final'`} {
				var count int
				if err := e.db.QueryRow(query).Scan(&count); err != nil || count != 0 {
					t.Fatal("failed transaction left side effects", count, err)
				}
			}
			var held int
			if err := e.db.QueryRow(`SELECT COUNT(*) FROM dag_resource_ledger WHERE run_id=? AND state='active'`, r.ID).Scan(&held); err != nil || held != 1 {
				t.Fatal("failed proof released the current execution slot", held, err)
			}
			fail = false
			for range 2 {
				if err := e.CompleteBound(ctx, "t", "p", final, before, commit); err != nil {
					t.Fatal("same final receipt could not recover after rollback", err)
				}
			}
			if err := e.db.QueryRow(`SELECT COUNT(*) FROM live_proof_fixture`).Scan(&held); err != nil || held != 3 {
				t.Fatal("replay repeated proof effects", held, err)
			}
		})
	}
}

func TestLiveReadOnlyCompletionKeepsExistingStoppedAcknowledgement(t *testing.T) {
	e, r, a, _, final := liveReconciliationFixture(t, false)
	e.AttemptReconciliation = liveReconciliationTestHook(func(context.Context, *sql.Tx, LiveReadOnlyCompletion, Result, Result) (bool, error) {
		t.Fatal("stopped acknowledgement reached live completion proof")
		return true, nil
	})
	final.Stopped = true
	if err := e.CompleteBound(ctx, "t", "p", final, func(*sql.Tx) error { return nil }, func(_ *sql.Tx, state string) error {
		if state != "failed" {
			t.Fatal("stopped acknowledgement became a successful completion", state)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := e.db.QueryRow(`SELECT state FROM dag_nodes WHERE run_id=? AND id=?`, r.ID, a.NodeID).Scan(&state); err != nil || state != "failed" {
		t.Fatal(state, err)
	}
	var resolutions int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM dag_managed_resolutions WHERE run_id=?`, r.ID).Scan(&resolutions); err != nil || resolutions != 0 {
		t.Fatal("stopped acknowledgement wrote a live reconciliation", resolutions, err)
	}
}
