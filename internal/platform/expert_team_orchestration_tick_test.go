package platform

import (
	"context"
	"encoding/json"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	"strings"
	"sync"
	"testing"
)

func orchestrationFixture(t *testing.T) (*expertFixture, ExpertTeamPlan) {
	t.Helper()
	h := newExpertFixture(t, false)
	f := h.f
	dependent, err := f.s.Update(context.Background(), f.actor, f.project, "tasks", h.tasks[1].ID, f.body(map[string]any{"expected_version": h.tasks[1].Version, "depends_on": []any{h.tasks[0].ID}}))
	if err != nil {
		t.Fatal(err)
	}
	h.tasks[1] = dependent
	p := h.preview(t)
	p, err = f.s.StartExpertTeamPlan(context.Background(), f.e, f.actor, f.project, p.ID, h.start(p))
	if err != nil {
		t.Fatal(err)
	}
	return h, p
}
func orchestrationCurrent(t *testing.T, h *expertFixture, id string) ExpertTeamPlan {
	t.Helper()
	p, err := h.f.s.GetExpertTeamPlan(context.Background(), h.f.e, h.f.actor, h.f.project, id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func orchestrationDue(t *testing.T, h *expertFixture) {
	t.Helper()
	if _, err := h.f.s.DB.Exec(`UPDATE suma_expert_team_plan_activations SET next_check_at='2000-01-01T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
}

// Actual Core completions and the formal Review API consume typed boundary
// fixtures here; these checks make no claim of a Git process or model execution.
func orchestrationCompleteRoot(t *testing.T, h *expertFixture, p ExpertTeamPlan) string {
	t.Helper()
	f, ctx := h.f, context.Background()
	profile, err := f.s.GetToolProfile(ctx, f.actor, f.project, h.role.ToolProfileID)
	if err != nil {
		t.Fatal(err)
	}
	caps := []string{"command:" + profile.Profile.CommandID, "command:independent-tests"}
	runID := p.Items[0].RunID
	for range 2 {
		a, err := f.e.Claim(ctx, f.user.TenantID, f.project, "tool-worker", caps)
		if err != nil || a == nil || a.RunID != runID {
			t.Fatal("missing original root execution", a, err)
		}
		head, base := strings.Repeat("b", 40), strings.Repeat("a", 40)
		proof := map[string]any{"head_commit": head, "base_commit": base, "evidence_sha256": strings.Repeat("c", 64), "state": "completed", "clean": true, "outcome_unknown": false, "exit_code": 0, "test_command": a.Type == "test"}
		r := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: proof}
		if a.Type == "agent" {
			r.ToolEvidence = &tc.Evidence{Profile: profile.Profile, Terminal: "completed", EventCount: 1, UsageState: tc.UsageUnreported}
		} else {
			proof["base_commit"] = a.CandidateCommit
			proof["head_commit"] = a.CandidateCommit
			proof["candidate_from"] = a.CandidateFrom
			proof["candidate_attempt"] = a.CandidateAttempt
			proof["candidate_fence"] = a.CandidateFence
			proof["candidate_evidence_sha256"] = a.CandidateEvidenceSHA256
		}
		if err = f.e.Complete(ctx, f.user.TenantID, f.project, r); err != nil {
			t.Fatal(err)
		}
	}
	return runID
}
func orchestrationAcceptRoot(t *testing.T, h *expertFixture, p ExpertTeamPlan) {
	t.Helper()
	f := h.f
	task, err := f.s.Get(context.Background(), f.actor, f.project, "tasks", p.Items[0].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Review(context.Background(), f.actor, f.project, task.ID, f.body(map[string]any{"expected_version": task.Version, "run_id": p.Items[0].RunID, "decision": "accepted", "reason": "Explicit test-owner acceptance of typed fixture evidence"}), f.e); err != nil {
		t.Fatal(err)
	}
}

func TestExpertTeamOrchestrationFormalAcceptanceAndAtomicTick(t *testing.T) {
	h, p := orchestrationFixture(t)
	f, ctx := h.f, context.Background()
	if p.Items[0].RunID == "" || p.Items[1].RunID != "" || expertCount(t, f, "dag_runs") != 1 {
		t.Fatal("unready dependent was dispatched", p)
	}
	orchestrationCompleteRoot(t, h, p)
	orchestrationDue(t, h)
	if err := f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
		t.Fatal(err)
	}
	current := orchestrationCurrent(t, h, p.ID)
	if current.Items[0].State != "awaiting_review" || current.Items[1].RunID != "" {
		t.Fatal("run success substituted formal acceptance", current)
	}
	orchestrationAcceptRoot(t, h, p)
	orchestrationDue(t, h)
	var group sync.WaitGroup
	failures := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() { defer group.Done(); failures <- f.s.TickExpertTeamPlans(ctx, f.e) }()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	current = orchestrationCurrent(t, h, p.ID)
	if current.Items[0].State != "accepted" || current.Items[1].RunID == "" || expertCount(t, f, "dag_runs") != 2 {
		t.Fatal("formal accepted successor not created exactly once", current)
	}
	original := current.Items[1].RunID
	orchestrationDue(t, h)
	if err := f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
		t.Fatal(err)
	}
	current = orchestrationCurrent(t, h, p.ID)
	if current.Items[1].RunID != original || expertCount(t, f, "dag_runs") != 2 {
		t.Fatal("tick duplicated original run")
	}
	raw, _ := json.Marshal(current)
	if strings.Contains(string(raw), f.actor.credential.digest) || strings.Contains(string(raw), "credential_reference") {
		t.Fatal("private activation credential leaked")
	}
}

func TestExpertTeamOrchestrationCredentialPauseAndSourceBoundaries(t *testing.T) {
	for _, mode := range []string{"credential", "paused", "stopped", "source"} {
		t.Run(mode, func(t *testing.T) {
			h, p := orchestrationFixture(t)
			f, ctx := h.f, context.Background()
			orchestrationCompleteRoot(t, h, p)
			orchestrationAcceptRoot(t, h, p)
			switch mode {
			case "credential":
				if _, err := f.s.DB.Exec(`UPDATE suma_credential_slots SET revoked=1 WHERE id=?`, f.actor.credential.slotID); err != nil {
					t.Fatal(err)
				}
			case "paused", "stopped":
				state := "paused"
				action := "pause"
				if mode == "stopped" {
					state, action = "stopped", "stop"
				}
				current := orchestrationCurrent(t, h, p.ID)
				if _, err := f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, ExpertTeamPlanCommandRequest{ProjectID: f.project, ExpectedVersion: current.Version, Action: action, Reason: "Explicit boundary fixture", Acknowledge: true, IdempotencyKey: newID()}); err != nil {
					t.Fatal(err)
				}
				_ = state
			case "source":
				if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET description='changed pending input',version=version+1 WHERE id=?`, h.tasks[1].ID); err != nil {
					t.Fatal(err)
				}
			}
			orchestrationDue(t, h)
			if err := f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
				t.Fatal(err)
			}
			if expertCount(t, f, "dag_runs") != 1 {
				t.Fatal("boundary created successor", mode)
			}
			var raw string
			if err := f.s.DB.QueryRow(`SELECT payload FROM suma_expert_team_plans WHERE id=?`, p.ID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var facts ExpertTeamPlan
			_ = json.Unmarshal([]byte(raw), &facts)
			if facts.Items[0].State != "accepted" {
				t.Fatal("dispatch denial erased old accepted facts", facts)
			}
		})
	}
}
