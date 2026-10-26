package platform

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
)

type handoffFixture struct {
	f                                  *fixture
	receiver                           Actor
	resource, owner, task, requirement Record
}

func newHandoffFixture(t *testing.T) *handoffFixture {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	ownerCookie := f.cookie
	inv := governanceInvite(f, "handoff@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "developer"}})
	accepted := governanceAccept(f, inv, "handoff@example.test")
	user := accepted["user"].(map[string]any)["id"].(string)
	receiver, err := f.s.AuthenticateActor(ctx, f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	f.cookie = ownerCookie
	f.call("GET", "/auth/session", nil, 200)
	create := func(kind string, body map[string]any) Record {
		r, e := f.s.Create(ctx, f.actor, f.project, kind, f.body(body))
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	owner := create("members", map[string]any{"title": "Original owner", "kind": "human", "data": map[string]any{"user_id": f.user.ID}})
	resource := create("members", map[string]any{"title": "Receiving human", "kind": "human", "data": map[string]any{"user_id": user}})
	requirement := create("requirements", map[string]any{"title": "Fixed handoff scope"})
	task := create("tasks", map[string]any{"title": "Transfer actual responsibility", "owner_id": owner.ID, "requirement_id": requirement.ID, "acceptance_criteria": []any{"Independent validation remains required"}})
	return &handoffFixture{f: f, receiver: receiver, resource: resource, owner: owner, task: task, requirement: requirement}
}

func (h *handoffFixture) current(t *testing.T) Record {
	t.Helper()
	task, err := h.f.s.Get(context.Background(), h.f.actor, h.f.project, "tasks", h.task.ID)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func (h *handoffFixture) request(t *testing.T) TaskHandoffCreateRequest {
	t.Helper()
	return TaskHandoffCreateRequest{ProjectID: h.f.project, TaskID: h.task.ID, ExpectedTaskVersion: h.current(t).Version, ToResourceID: h.resource.ID, Note: "Transfer the fixed task and preserve its original delivery evidence", IdempotencyKey: newID()}
}

func (h *handoffFixture) send(t *testing.T) TaskHandoff {
	t.Helper()
	out, err := h.f.s.CreateTaskHandoff(context.Background(), h.f.actor, h.f.project, h.request(t))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func handoffCommand(h *handoffFixture, action string, version int) TaskHandoffCommandRequest {
	return TaskHandoffCommandRequest{ProjectID: h.f.project, ExpectedVersion: version, Action: action, Reason: "Explicit account decision for the fixed handoff", AcknowledgeInvalidation: true, IdempotencyKey: newID()}
}

func handoffWantStatus(t *testing.T, err error, status int) {
	t.Helper()
	var api *APIError
	if !errors.As(err, &api) || api.Status != status {
		t.Fatalf("want status %d, got %v", status, err)
	}
}

func (h *handoffFixture) bindRun(t *testing.T, task Record, complete bool) Record {
	t.Helper()
	f := h.f
	workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "Handoff independent test", "nodes": []any{map[string]any{"id": "test", "type": "test", "command": []string{"fixture-go-test"}, "idempotent": true}}, "edges": []any{}}), 201)
	f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow["id"], "task_id": task.ID, "expected_version": task.Version}), 201)
	if complete {
		a, err := f.e.Claim(context.Background(), f.user.TenantID, f.project, "handoff-test-worker", toolFixtureCapabilities(t, f))
		if err != nil || a == nil {
			t.Fatalf("claim %v %v", a, err)
		}
		if err = f.e.Complete(context.Background(), f.user.TenantID, f.project, dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: 0, OperationID: newID(), Output: "independent fixed test"}); err != nil {
			t.Fatal(err)
		}
	}
	out, err := f.s.Get(context.Background(), f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTaskHandoffAcceptInvalidatesEvidenceWithoutNewExecution(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	task := h.bindRun(t, h.task, true)
	accepted, err := f.s.Review(ctx, f.actor, f.project, task.ID, f.body(map[string]any{"expected_version": task.Version, "run_id": task.RunID, "decision": "accepted", "reason": "Independent test result explicitly reviewed"}), f.e)
	if err != nil {
		t.Fatal(err)
	}
	downstream, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Dependent accepted delivery", "owner_id": h.owner.ID, "depends_on": []any{h.task.ID}}))
	if err != nil {
		t.Fatal(err)
	}
	downstream = h.bindRun(t, downstream, true)
	if _, err = f.s.Review(ctx, f.actor, f.project, downstream.ID, f.body(map[string]any{"expected_version": downstream.Version, "run_id": downstream.RunID, "decision": "accepted", "reason": "Reviewed with the current dependency basis"}), f.e); err != nil {
		t.Fatal(err)
	}
	proposal := h.send(t)
	request := handoffCommand(h, "accept", proposal.Version)
	request.AcknowledgeInvalidation = false
	_, err = f.s.CommandTaskHandoff(ctx, h.receiver, f.project, proposal.ID, request)
	handoffWantStatus(t, err, 400)
	request.AcknowledgeInvalidation = true
	out, err := f.s.CommandTaskHandoff(ctx, h.receiver, f.project, proposal.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	current := h.current(t)
	if out.State != "accepted" || out.ResultTaskVersion != accepted.Version+1 || current.OwnerID != h.resource.ID || current.Status != "in_progress" || current.RunID != accepted.RunID || current.Data["verified_run_id"] != nil || current.Data["run_task_basis"] != accepted.Data["run_task_basis"] {
		t.Fatalf("incomplete responsibility transfer: %+v task=%+v", out, current)
	}
	dependent, err := f.s.Get(ctx, f.actor, f.project, "tasks", downstream.ID)
	if err != nil || dependent.Status != "in_progress" || dependent.Data["verified_run_id"] != nil {
		t.Fatalf("dependent acceptance remained valid: %+v %v", dependent, err)
	}
	again, err := f.s.CommandTaskHandoff(ctx, h.receiver, f.project, proposal.ID, request)
	if err != nil || again.ID != out.ID || again.Version != out.Version {
		t.Fatalf("lost accept ACK did not replay: %+v %v", again, err)
	}
	var runs, reviews, pending int
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&runs)
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_reviews`).Scan(&reviews)
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_task_handoff_pending`).Scan(&pending)
	if runs != 2 || reviews != 2 || pending != 0 {
		t.Fatalf("handoff fabricated execution/review or left slot: runs=%d reviews=%d pending=%d", runs, reviews, pending)
	}
}

func TestTaskHandoffDriftCanReturnOrWithdrawWithoutTaskMutation(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	proposal := h.send(t)
	changed, err := f.s.Update(ctx, f.actor, f.project, "tasks", h.task.ID, f.body(map[string]any{"expected_version": h.task.Version, "description": "Scope changed after sending"}))
	if err != nil {
		t.Fatal(err)
	}
	page, err := f.s.TaskHandoffs(ctx, h.receiver, f.project, h.task.ID, 20, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].BasisCurrent {
		t.Fatalf("stale basis not displayed: %+v %v", page, err)
	}
	_, err = f.s.CommandTaskHandoff(ctx, h.receiver, f.project, proposal.ID, handoffCommand(h, "accept", 1))
	handoffWantStatus(t, err, 409)
	out, err := f.s.CommandTaskHandoff(ctx, h.receiver, f.project, proposal.ID, handoffCommand(h, "return", 1))
	if err != nil || out.State != "returned" || out.BasisCurrent || h.current(t).Version != changed.Version {
		t.Fatalf("return modified task: %+v %v", out, err)
	}
	proposal = h.send(t)
	if _, err = f.s.Update(ctx, f.actor, f.project, "requirements", h.requirement.ID, f.body(map[string]any{"expected_version": h.requirement.Version, "description": "New requirement basis"})); err != nil {
		t.Fatal(err)
	}
	before := h.current(t)
	out, err = f.s.CommandTaskHandoff(ctx, f.actor, f.project, proposal.ID, handoffCommand(h, "withdraw", 1))
	if err != nil || out.State != "withdrawn" || out.BasisCurrent || h.current(t).Version != before.Version {
		t.Fatalf("withdraw modified task: %+v %v", out, err)
	}
}

func TestTaskHandoffReceiptsRequireCurrentAuthorityAndExactBody(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	request := h.request(t)
	first, err := f.s.CreateTaskHandoff(ctx, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.s.CreateTaskHandoff(ctx, f.actor, f.project, request)
	if err != nil || again.ID != first.ID {
		t.Fatal("create ACK replay changed proposal", err)
	}
	request.Note = "Different payload with the original operation"
	_, err = f.s.CreateTaskHandoff(ctx, f.actor, f.project, request)
	handoffWantStatus(t, err, 409)
	_, err = f.s.CommandTaskHandoff(ctx, f.actor, f.project, first.ID, handoffCommand(h, "accept", 1))
	handoffWantStatus(t, err, 403)
	command := handoffCommand(h, "accept", 1)
	if _, err = f.s.CommandTaskHandoff(ctx, h.receiver, f.project, first.ID, command); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.SaveProjectAccess(ctx, f.actor, map[string]any{"project_id": f.project, "user_id": first.ToUserID, "action": "remove", "expected_version": float64(1), "idempotency_key": newID()}); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CommandTaskHandoff(ctx, h.receiver, f.project, first.ID, command)
	handoffWantStatus(t, err, 403)
}

func TestTaskHandoffRejectsChangedBasisAndUnsafeRuns(t *testing.T) {
	for _, scenario := range []string{"requirement", "owner", "run_version", "live_node", "unknown_attempt"} {
		t.Run(scenario, func(t *testing.T) {
			h := newHandoffFixture(t)
			f := h.f
			ctx := context.Background()
			if scenario == "run_version" || scenario == "live_node" || scenario == "unknown_attempt" {
				h.task = h.bindRun(t, h.task, true)
			}
			proposal := h.send(t)
			switch scenario {
			case "requirement":
				_, err := f.s.Update(ctx, f.actor, f.project, "requirements", h.requirement.ID, f.body(map[string]any{"expected_version": 1, "description": "Changed requirement"}))
				if err != nil {
					t.Fatal(err)
				}
			case "owner":
				_, err := f.s.Update(ctx, f.actor, f.project, "tasks", h.task.ID, f.body(map[string]any{"expected_version": h.task.Version, "owner_id": ""}))
				if err != nil {
					t.Fatal(err)
				}
			case "run_version":
				if _, err := f.s.DB.Exec(`UPDATE dag_runs SET version=version+1 WHERE id=?`, h.task.RunID); err != nil {
					t.Fatal(err)
				}
			case "live_node":
				if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET state='running' WHERE run_id=?`, h.task.RunID); err != nil {
					t.Fatal(err)
				}
			case "unknown_attempt":
				var raw string
				if err := f.s.DB.QueryRow(`SELECT result FROM dag_attempts WHERE run_id=?`, h.task.RunID).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var result dag.Result
				if json.Unmarshal([]byte(raw), &result) != nil {
					t.Fatal("invalid fixture result")
				}
				result.OutcomeUnknown = true
				if _, err := f.s.DB.Exec(`UPDATE dag_attempts SET result=? WHERE run_id=?`, stringify(result), h.task.RunID); err != nil {
					t.Fatal(err)
				}
			}
			_, err := f.s.CommandTaskHandoff(ctx, h.receiver, f.project, proposal.ID, handoffCommand(h, "accept", 1))
			handoffWantStatus(t, err, 409)
			if h.current(t).OwnerID == h.resource.ID {
				t.Fatal("stale or unsafe handoff transferred ownership")
			}
			if _, err = f.s.CommandTaskHandoff(ctx, f.actor, f.project, proposal.ID, handoffCommand(h, "withdraw", 1)); err != nil {
				t.Fatal(err)
			}
			if scenario == "live_node" || scenario == "unknown_attempt" {
				_, err = f.s.CreateTaskHandoff(ctx, f.actor, f.project, h.request(t))
				handoffWantStatus(t, err, 409)
			}
		})
	}
}

func TestTaskHandoffRecipientIdentityTeamAndReadProjection(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	agent, err := f.s.Create(ctx, f.actor, f.project, "members", f.body(map[string]any{"title": "Agent is not a human receiver", "kind": "agent"}))
	if err != nil {
		t.Fatal(err)
	}
	request := h.request(t)
	request.ToResourceID = agent.ID
	_, err = f.s.CreateTaskHandoff(ctx, f.actor, f.project, request)
	handoffWantStatus(t, err, 400)
	request = h.request(t)
	request.ToResourceID = h.owner.ID
	_, err = f.s.CreateTaskHandoff(ctx, f.actor, f.project, request)
	handoffWantStatus(t, err, 400)
	team, err := f.s.SaveTeam(ctx, f.actor, "", map[string]any{"name": "Receiving team", "expected_version": float64(0), "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	member := map[string]any{"resource_kind": "human", "resource_id": h.resource.Data["user_id"], "project_id": "", "role": "observer", "expected_version": float64(0), "idempotency_key": newID()}
	if _, err = f.s.SaveTeamMember(ctx, f.actor, team["id"].(string), member); err != nil {
		t.Fatal(err)
	}
	request = h.request(t)
	request.ToTeamID = team["id"].(string)
	_, err = f.s.CreateTaskHandoff(ctx, f.actor, f.project, request)
	handoffWantStatus(t, err, 400)
	member["role"], member["expected_version"], member["idempotency_key"] = "member", float64(1), newID()
	if _, err = f.s.SaveTeamMember(ctx, f.actor, team["id"].(string), member); err != nil {
		t.Fatal(err)
	}
	proposal, err := f.s.CreateTaskHandoff(ctx, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	page, err := f.s.TaskHandoffs(ctx, h.receiver, f.project, h.task.ID, 20, "")
	if err != nil || len(page.Items) != 1 || !page.Items[0].BasisCurrent || len(page.Items[0].AllowedActions) != 2 || page.CanCreate {
		t.Fatalf("recipient projection: %+v %v", page, err)
	}
	member["role"], member["expected_version"], member["idempotency_key"] = "observer", float64(2), newID()
	if _, err = f.s.SaveTeamMember(ctx, f.actor, team["id"].(string), member); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CommandTaskHandoff(ctx, h.receiver, f.project, proposal.ID, handoffCommand(h, "accept", 1))
	handoffWantStatus(t, err, 403)
	if _, err = f.s.CommandTaskHandoff(ctx, f.actor, f.project, proposal.ID, handoffCommand(h, "withdraw", 1)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Update(ctx, f.actor, f.project, "members", h.resource.ID, f.body(map[string]any{"expected_version": h.resource.Version, "status": "archived"})); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CreateTaskHandoff(ctx, f.actor, f.project, h.request(t))
	handoffWantStatus(t, err, 400)
}

func TestTaskHandoffTwoConnectionCASAndStableTaskPaging(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	var seq int
	var name, path string
	if err := f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	other := &Service{DB: db, Dialect: "sqlite"}
	type outcome struct {
		h   TaskHandoff
		err error
	}
	results := make(chan outcome, 2)
	var group sync.WaitGroup
	for _, service := range []*Service{f.s, other} {
		request := h.request(t)
		group.Add(1)
		go func(s *Service, req TaskHandoffCreateRequest) {
			defer group.Done()
			out, e := s.CreateTaskHandoff(ctx, f.actor, f.project, req)
			results <- outcome{out, e}
		}(service, request)
	}
	group.Wait()
	close(results)
	var proposal TaskHandoff
	success, conflicts := 0, 0
	for result := range results {
		if result.err == nil {
			success++
			proposal = result.h
		} else {
			handoffWantStatus(t, result.err, 409)
			conflicts++
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("multiple pending handoffs: %d/%d", success, conflicts)
	}
	results = make(chan outcome, 2)
	for i, service := range []*Service{f.s, other} {
		actor, action := h.receiver, "accept"
		if i == 1 {
			actor, action = f.actor, "withdraw"
		}
		group.Add(1)
		go func(s *Service, a Actor, action string) {
			defer group.Done()
			out, e := s.CommandTaskHandoff(ctx, a, f.project, proposal.ID, handoffCommand(h, action, 1))
			results <- outcome{out, e}
		}(service, actor, action)
	}
	group.Wait()
	close(results)
	success = 0
	for result := range results {
		if result.err == nil {
			success++
			if result.h.State == "accepted" && h.current(t).OwnerID != h.resource.ID {
				t.Fatal("partial owner transfer")
			}
		} else {
			handoffWantStatus(t, result.err, 409)
		}
	}
	if success != 1 {
		t.Fatal("decision CAS applied twice")
	}
	// Keep paging on the original owner if accept won, using a new source task.
	newTask, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Paging history", "owner_id": h.owner.ID}))
	if err != nil {
		t.Fatal(err)
	}
	h.task = newTask
	for i := 0; i < 3; i++ {
		sent := h.send(t)
		if _, err = f.s.CommandTaskHandoff(ctx, f.actor, f.project, sent.ID, handoffCommand(h, "withdraw", 1)); err != nil {
			t.Fatal(err)
		}
	}
	first, err := f.s.TaskHandoffs(ctx, f.actor, f.project, h.task.ID, 2, "")
	if err != nil || first.Total != 3 || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page: %+v %v", first, err)
	}
	second, err := f.s.TaskHandoffs(ctx, f.actor, f.project, h.task.ID, 2, first.NextCursor)
	if err != nil || second.Total != 3 || len(second.Items) != 1 || second.NextCursor != "" || second.Items[0].ID == first.Items[0].ID {
		t.Fatalf("second page: %+v %v", second, err)
	}
	_, err = f.s.TaskHandoffs(ctx, f.actor, f.project, h.task.ID, 2, proposal.ID)
	handoffWantStatus(t, err, 400)
}
