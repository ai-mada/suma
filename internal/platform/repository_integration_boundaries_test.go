package platform

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	"github.com/ai-mada/suma/internal/store"
)

func repositoryReady(t *testing.T, h *repositoryFixture) {
	t.Helper()
	h.create(t)
	a := h.claim(t)
	g := h.consume(t, a)
	h.complete(t, a, h.observation(t, g, "prepared", ""))
	h.current(t)
}
func repositoryApplying(t *testing.T, h *repositoryFixture) (dag.Assignment, ri.RepositoryAuthorizationGrant) {
	t.Helper()
	repositoryReady(t, h)
	h.command(t, "approve")
	h.command(t, "execute")
	a := h.claim(t)
	return a, h.consume(t, a)
}

func TestRepositoryIntegrationUnknownBeforeTicketClosesOriginalPrepareAndApply(t *testing.T) {
	for _, phase := range []string{"prepare", "apply"} {
		t.Run(phase, func(t *testing.T) {
			h := newRepositoryFixture(t)
			if phase == "prepare" {
				h.create(t)
			} else {
				repositoryReady(t, h)
				h.command(t, "approve")
				h.command(t, "execute")
			}
			a := h.claim(t)
			// The real claim has already saved the full identity; no ticket or
			// provider operation exists. The unknown is an actual Core callback.
			result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), ExitCode: -1, OutcomeUnknown: true, Error: repositoryUnknown}
			h.f.bearer("POST", "/runner/results", h.workerToken, map[string]any{"project_id": h.f.project, "result": result}, 200)
			p := h.current(t)
			if p.State != "reconciling" || !p.CanReconcile {
				t.Fatal(p)
			}
			p = h.command(t, "reconcile")
			if p.State != "blocked" {
				t.Fatal("explicit same-operation close did not settle", p)
			}
			if phase == "apply" && (p.Observation.Receipt == nil || p.Observation.State != "not_applied") {
				t.Fatal("missing actual negative provider proof", p)
			}
			if phase == "prepare" && (p.Observation.Receipt != nil || p.Observation.State != "blocked") {
				t.Fatal("read preparation invented a Git receipt", p)
			}
			var original string
			if err := h.f.s.DB.QueryRow(`SELECT original_result FROM dag_managed_resolutions WHERE run_id=?`, a.RunID).Scan(&original); err != nil {
				t.Fatal(err)
			}
			var kept dag.Result
			if json.Unmarshal([]byte(original), &kept) != nil || !kept.OutcomeUnknown {
				t.Fatal("original unknown was overwritten")
			}
			targets, err := h.f.s.ListRepositoryIntegrationTargets(context.Background(), h.f.actor, h.f.project)
			if err != nil || targets[0].ActiveOperationID != "" || phase == "prepare" && targets[0].Generation != 0 || phase == "apply" && targets[0].Generation != 1 {
				t.Fatal(targets, err)
			}
		})
	}
}

func TestRepositoryIntegrationCurrentCreatorBeforeArmAndGenuineFactsAfterGrant(t *testing.T) {
	for _, phase := range []string{"before-arm", "after-arm"} {
		t.Run(phase, func(t *testing.T) {
			h := newRepositoryFixture(t)
			f, ctx := h.f, context.Background()
			a, g := repositoryApplying(t, h)
			ownerToken := f.cookie.Value
			inv := governanceInvite(f, "repo-reviewer@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "reviewer"}})
			governanceAccept(f, inv, "repo-reviewer@example.test")
			reviewer := toolActorForTest(t, f, f.cookie.Value)
			request := ri.RepositoryBeginEffect{AuthorizationID: g.AuthorizationID, OperationID: g.OperationID, Assignment: g.Assignment, RequestSHA256: g.RequestSHA256, Generation: g.Execution.Generation}
			var grant ri.RepositoryEffectGrant
			var err error
			if phase == "after-arm" {
				grant, err = f.s.BeginRepositoryEffect(ctx, h.secret, request)
				if err != nil {
					t.Fatal(err)
				}
			}
			logout, err := f.s.ResolveLogoutTarget(ctx, ownerToken)
			if err != nil {
				t.Fatal(err)
			}
			if err = f.s.Logout(ctx, logout); err != nil {
				t.Fatal(err)
			}
			if phase == "before-arm" {
				if _, err = f.s.BeginRepositoryEffect(ctx, h.secret, request); err == nil {
					t.Fatal("creator logout permitted new effect")
				}
				return
			}
			// A valid already-granted provider fact and actual Runner result survive
			// creator logout; this never grants another write or human approval.
			o := h.observation(t, g, "applied", grant.EffectGrantID)
			h.complete(t, a, o)
			p, err := f.s.GetRepositoryIntegrationPlan(ctx, f.e, reviewer, f.project, h.plan.ID)
			if err != nil || p.State != "integrated" {
				t.Fatal("genuine Git fact was lost", p, err)
			}
		})
	}
}

func TestRepositoryIntegrationRejectsUnconsumedForgedAndMixedResults(t *testing.T) {
	h := newRepositoryFixture(t)
	h.create(t)
	a := h.claim(t)
	// An authenticated provider token cannot skip consumption of the actual
	// worker ticket, even if it can compute public packet/digest fields.
	g := ri.RepositoryAuthorizationGrant{OperationID: a.RepositoryIntegration.OperationID, Execution: *a.RepositoryIntegration, Assignment: repositoryTestRef(a)}
	g.RequestSHA256, _ = ri.RepositoryRequestDigest(ri.RepositoryRequest{RepositoryExecution: g.Execution, Assignment: g.Assignment})
	o := h.observation(t, g, "prepared", "")
	if _, err := h.f.s.RecordRepositoryObservation(context.Background(), h.secret, o); err == nil {
		t.Fatal("unconsumed authorization forged a prepared fact")
	}
	g = h.consume(t, a)
	o = h.observation(t, g, "prepared", "")
	if _, err := h.f.s.RecordRepositoryObservation(context.Background(), h.secret, o); err != nil {
		t.Fatal(err)
	}
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), IntegrationEvidence: &ri.RepositoryResult{OperationID: o.OperationID, RequestSHA256: o.RequestSHA256, ObservationSHA256: o.ObservationSHA256, Version: o.Version}, WorkspaceEvidence: map[string]any{"head_commit": strings.Repeat("a", 40)}}
	h.f.bearer("POST", "/runner/results", h.workerToken, map[string]any{"project_id": h.f.project, "result": result}, 409)
	result.WorkspaceEvidence = nil
	result.OperationID = newID()
	result.IntegrationEvidence.ObservationSHA256 = strings.Repeat("f", 64)
	h.f.bearer("POST", "/runner/results", h.workerToken, map[string]any{"project_id": h.f.project, "result": result}, 409)
	p := h.current(t)
	if p.State != "preparing" {
		t.Fatal("fake callback advanced readiness", p)
	}
}

func TestRepositoryIntegrationBasisAndActualTargetDriftBlockApproval(t *testing.T) {
	for _, kind := range []string{"task", "target"} {
		t.Run(kind, func(t *testing.T) {
			h := newRepositoryFixture(t)
			repositoryReady(t, h)
			p := h.current(t)
			if kind == "task" {
				if _, err := h.f.s.Update(context.Background(), h.f.actor, h.f.project, "tasks", h.task.ID, h.f.body(map[string]any{"expected_version": h.task.Version, "description": "Changed after original human review"})); err != nil {
					t.Fatal(err)
				}
			} else {
				h.mock.Target.HeadCommit = strings.Repeat("d", 40)
			}
			_, err := h.f.s.CommandRepositoryIntegrationPlan(context.Background(), h.f.e, h.f.actor, h.f.project, p.ID, RepositoryIntegrationPlanCommandRequest{ProjectID: h.f.project, ExpectedVersion: p.Version, Action: "approve", Reason: "Review the exact source and target", Acknowledge: true, IdempotencyKey: newID()})
			releaseWantStatus(t, err, 409)
			p = h.current(t)
			if p.Approval != nil || p.State != "ready" {
				t.Fatal("drift acquired approval", p)
			}
		})
	}
}

func TestRepositoryIntegrationPrepareConcurrentOperationAndUnchangedTaskBinding(t *testing.T) {
	h := newRepositoryFixture(t)
	f, ctx := h.f, context.Background()
	var path string
	if err := f.s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	other := &Service{DB: db, Dialect: "sqlite", RepositoryProviders: f.s.RepositoryProviders}
	req := RepositoryIntegrationPlanCreateRequest{ProjectID: f.project, TaskID: h.task.ID, ExpectedTaskVersion: h.task.Version, SourceNodeID: "code", TargetID: h.target.ID, ExpectedTargetVersion: h.target.Version, IdempotencyKey: newID()}
	results := make(chan RepositoryIntegrationPlan, 2)
	failures := make(chan error, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			s, e := f.s, f.e
			if i == 1 {
				s, e = other, engine
			}
			p, err := s.CreateRepositoryIntegrationPlan(ctx, e, f.actor, f.project, req)
			if err != nil {
				failures <- err
			} else {
				results <- p
			}
		}(i)
	}
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for p := range results {
		ids[p.ID] = true
	}
	if len(ids) != 1 {
		t.Fatal("concurrent original operation duplicated", ids)
	}
	var count int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_repository_operations`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", h.task.ID)
	if err != nil || task.RunID != h.task.RunID || task.Version != h.task.Version || task.Status != "accepted" {
		t.Fatal("integration replaced business delivery basis", task, err)
	}
}

func TestRepositoryIntegrationProviderConfigurationRejectsAliasAndSecretConfusion(t *testing.T) {
	getenv := func(key string) string {
		if key == "CALLBACK" {
			return strings.Repeat("a", 32)
		}
		if key == "INSPECT" {
			return strings.Repeat("b", 32)
		}
		return ""
	}
	valid := `[{"id":"repo","tenant_id":"tenant","project_id":"project","target_aliases":["main"],"origin":"http://127.0.0.1:12345","token_env":"CALLBACK","reconcile_token_env":"INSPECT"}]`
	if _, err := LoadRepositoryProviders(valid, getenv); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(valid, `["main"]`, `["main","main"]`, 1), strings.Replace(valid, "127.0.0.1", "example.invalid", 1), strings.Replace(valid, `"INSPECT"`, `"CALLBACK"`, 1), strings.Replace(valid, `"id":"repo"`, `"id":"repo","id":"other"`, 1)} {
		if _, err := LoadRepositoryProviders(bad, getenv); err == nil {
			t.Fatal("unsafe provider configuration accepted")
		}
	}
}
