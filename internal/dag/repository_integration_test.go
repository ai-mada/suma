package dag

import (
	"database/sql"
	"errors"
	release "github.com/ai-mada/suma/internal/releasecontract"
	rc "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	"testing"
)

func TestRepositoryManagedObservationLeaseRollbackAndProvenance(t *testing.T) {
	e, run, a, result := managedObservationFixture(t)
	result.ReleaseEvidence = nil
	result.IntegrationEvidence = &rc.RepositoryResult{OperationID: "repository-test-op", RequestSHA256: hashText("request"), ObservationSHA256: hashText("repository proof"), Version: 1}
	// This Core fixture authenticates no Git conclusion. The mandatory service
	// callback verifies provider facts; Core only governs the expired attempt.
	before := func(*sql.Tx) error { return nil }
	commitCalls := 0
	commit := func(*sql.Tx, string) error { commitCalls++; return nil }
	if err := e.CompleteManagedObservedBound(ctx, "t", "p", result, before, commit); !errors.Is(err, ErrConflict) || commitCalls != 0 {
		t.Fatal("live lease overwritten", err)
	}
	unknown := Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "repository-unknown", OutcomeUnknown: true, ExitCode: -1}
	if err := e.Complete(ctx, "t", "p", unknown); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, run.ID, a.NodeID); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("proof commit rolled back")
	if err := e.CompleteManagedObservedBound(ctx, "t", "p", result, before, func(*sql.Tx, string) error { return denied }); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	var count int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM dag_managed_resolutions WHERE run_id=?`, run.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed proof left resolution", count, err)
	}
	for range 2 {
		if err := e.CompleteManagedObservedBound(ctx, "t", "p", result, before, commit); err != nil {
			t.Fatal(err)
		}
	}
	if commitCalls != 1 {
		t.Fatal("receipt replay committed twice", commitCalls)
	}
	proof, err := e.GetAttempt(ctx, "t", "p", run.ID, a.NodeID, a.Attempt)
	if err != nil || proof.ResultSource != "managed_observation" || proof.Result == nil || proof.Result.IntegrationEvidence == nil {
		t.Fatal("repository proof provenance", proof, err)
	}
	both := result
	both.ReleaseEvidence = &release.ReleaseResult{}
	if err = e.CompleteManagedObservedBound(ctx, "t", "p", both, before, commit); !errors.Is(err, ErrInvalid) {
		t.Fatal("ambiguous provider evidence accepted", err)
	}
	if noCompletionEffects(result) {
		t.Fatal("Git evidence entered read-only completion path")
	}
}
