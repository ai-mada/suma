package platform

import (
	"context"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestAuthoritySchedulingCurrentBrowserPrecedesWritesAndReads(t *testing.T) {
	for _, invalidation := range []string{"logout", "expiry"} {
		t.Run(invalidation, func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			actor := f.actor
			request := SchedulingPolicyPreviewRequest{ProjectID: f.project, ExpectedVersion: 0, Policy: dag.SchedulingPolicyInput{ConcurrencyLimit: 2, ResourceLimits: map[string]*int64{}}, IdempotencyKey: "authority-scheduling-preview"}
			preview, err := f.s.CreateSchedulingPreview(ctx, f.e, actor, f.project, request)
			if err != nil {
				t.Fatal(err)
			}
			commit := SchedulingPolicyCommitRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: "authority-scheduling-commit"}
			policy, err := f.s.CommitSchedulingPreview(ctx, f.e, actor, f.project, preview.ID, commit)
			if err != nil {
				t.Fatal(err)
			}
			before := [3]int{domainACount(t, f.s, "suma_scheduling_previews"), domainACount(t, f.s, "suma_operations"), domainACount(t, f.s, "suma_activity")}
			var response string
			if err := f.s.DB.QueryRow(`SELECT response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`, f.user.TenantID, f.project, commit.IdempotencyKey).Scan(&response); err != nil {
				t.Fatal(err)
			}
			if invalidation == "logout" {
				if err := logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
					t.Fatal(err)
				}
			} else {
				tx, err := f.s.DB.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				for _, change := range []struct{ query, key string }{
					{`UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash=?`, actor.credential.digest},
					{`UPDATE suma_session_families SET expires_at='2000-01-01T00:00:00Z' WHERE id=?`, actor.credential.familyID},
					{`UPDATE suma_credential_slots SET expires_at='2000-01-01T00:00:00Z' WHERE id=?`, actor.credential.slotID},
				} {
					if _, err := tx.ExecContext(ctx, change.query, change.key); err != nil {
						t.Fatal(err)
					}
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
			}
			for _, operation := range []string{request.IdempotencyKey, "fresh-scheduling-preview"} {
				request.IdempotencyKey = operation
				_, err := f.s.CreateSchedulingPreview(ctx, f.e, actor, f.project, request)
				requireAuthorityStatus(t, err, 401)
			}
			for _, operation := range []string{commit.IdempotencyKey, "fresh-scheduling-commit"} {
				commit.IdempotencyKey = operation
				_, err := f.s.CommitSchedulingPreview(ctx, f.e, actor, f.project, preview.ID, commit)
				requireAuthorityStatus(t, err, 401)
			}
			_, err = f.s.GetSchedulingPreview(ctx, f.e, actor, f.project, preview.ID)
			requireAuthorityStatus(t, err, 401)
			_, err = f.s.SchedulingPolicy(ctx, f.e, actor, f.project, true)
			requireAuthorityStatus(t, err, 401)
			after := [3]int{domainACount(t, f.s, "suma_scheduling_previews"), domainACount(t, f.s, "suma_operations"), domainACount(t, f.s, "suma_activity")}
			if after != before {
				t.Fatalf("denial changed previews/receipts/activity: %v -> %v", before, after)
			}
			var stored string
			if err := f.s.DB.QueryRow(`SELECT response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id='authority-scheduling-commit'`, f.user.TenantID, f.project).Scan(&stored); err != nil || stored != response {
				t.Fatalf("denial changed original receipt: %v", err)
			}
			var version int
			if err := f.s.DB.QueryRow(`SELECT version FROM dag_scheduling_policies WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&version); err != nil || version != policy.Version {
				t.Fatalf("denial changed policy: version=%d err=%v", version, err)
			}
		})
	}
}

func TestAuthoritySchedulingScopedKindsCannotApproveThroughDisplayHint(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	request := SchedulingPolicyPreviewRequest{ProjectID: f.project, ExpectedVersion: 0, Policy: dag.SchedulingPolicyInput{ConcurrencyLimit: 2, ResourceLimits: map[string]*int64{}}, IdempotencyKey: "kind-scheduling-preview"}
	preview, err := f.s.CreateSchedulingPreview(ctx, f.e, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, Label: "browser", IdempotencyKey: "scheduling-kind-" + scope})
		if err != nil {
			t.Fatal(err)
		}
		actor := domainAActor(t, f.s, issued.Token)
		policy, err := f.s.SchedulingPolicy(ctx, f.e, actor, f.project, true)
		if scope == "agent" {
			requireAuthorityStatus(t, err, 403)
		} else if err != nil || len(policy.AllowedActions) != 0 {
			t.Fatalf("%s display hint granted browser approval: %+v %v", scope, policy, err)
		}
		_, err = f.s.CreateSchedulingPreview(ctx, f.e, actor, f.project, request)
		requireAuthorityStatus(t, err, 403)
		_, err = f.s.CommitSchedulingPreview(ctx, f.e, actor, f.project, preview.ID, SchedulingPolicyCommitRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: "kind-scheduling-commit"})
		requireAuthorityStatus(t, err, 403)
	}
}
