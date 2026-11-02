package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func dispatchTestBindingsFixture(t *testing.T) (*fixture, TaskDispatchPreviewRequest, ToolProfile) {
	t.Helper()
	f, _, body, _ := taskDispatchFixture(t)
	var request TaskDispatchPreviewRequest
	if err := json.Unmarshal([]byte(stringify(body)), &request); err != nil {
		t.Fatal(err)
	}
	profile := dispatchTestBindingsProfile(t, f, "independent-worker", "independent-report", tr.Protocol)
	request.TestBindings = []TaskDispatchTestSelection{{NodeID: "test", ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version}}
	return f, request, profile
}

func dispatchTestBindingsProfile(t *testing.T, f *fixture, worker, command, report string) ToolProfile {
	t.Helper()
	ctx := context.Background()
	token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": worker, "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	definition := toolTestProfile()
	definition.Adapter, definition.Protocol, definition.CommandID = "command", "text-v1", command
	definition.ProfileSHA256, definition.TestReportProtocol = digest(worker+command+report), report
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: worker, Name: "Independent report command", Profile: definition, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: []tc.Observation{{Profile: definition, Available: true}}})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func dispatchTestBindingStatus(t *testing.T, err error, status int) {
	t.Helper()
	if status == 400 && errors.Is(err, dag.ErrInvalid) {
		return // Original compiler diagnostics map to the same HTTP input boundary.
	}
	var api *APIError
	if !errors.As(err, &api) || api.Status != status {
		t.Fatalf("expected status %d, got %v", status, err)
	}
}

func TestTaskDispatchExplicitTestsFreezeOnlySelectedNodesAndRecoverOriginalReceipt(t *testing.T) {
	f, request, profile := dispatchTestBindingsFixture(t)
	ctx := context.Background()
	var original string
	if err := f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE id=? AND version=?`, request.WorkflowID, request.WorkflowVersion).Scan(&original); err != nil {
		t.Fatal(err)
	}
	var template dag.Workflow
	if err := json.Unmarshal([]byte(original), &template); err != nil {
		t.Fatal(err)
	}
	template.Version++
	template.Nodes[1].Config["capability"] = "command:" + template.Nodes[1].Command[0]
	publishedTemplate, err := f.e.Publish(ctx, f.user.TenantID, f.project, template)
	if err != nil {
		t.Fatal(err)
	}
	request.WorkflowVersion = publishedTemplate.Version
	if err := f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE id=? AND version=?`, request.WorkflowID, request.WorkflowVersion).Scan(&original); err != nil {
		t.Fatal(err)
	}
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	want := TaskDispatchTestBinding{NodeID: "test", ToolProfileID: profile.ID, ToolProfileVersion: profile.Version, ToolProfileSHA256: profile.Profile.ProfileSHA256, WorkerID: profile.WorkerID, CommandID: profile.Profile.CommandID}
	if len(plan.TestBindings) != 1 || plan.TestBindings[0] != want {
		t.Fatal("explicit test snapshot was not frozen", plan.TestBindings)
	}
	var published dag.Workflow
	if err = json.Unmarshal([]byte(original), &published); err != nil {
		t.Fatal(err)
	}
	test := plan.Workflow.Nodes[1]
	if test.ID != "test" || test.Type != "test" || test.Command[0] != want.CommandID || test.Config["tool_profile_id"] != want.ToolProfileID || test.Config["tool_profile_sha256"] != want.ToolProfileSHA256 || test.Config["candidate_from"] != "develop" || test.Config["capability"] != "command:"+want.CommandID || stringify(test.DependsOn) != stringify(published.Nodes[1].DependsOn) {
		t.Fatal("test binding changed the wrong node or graph", test)
	}
	var unchanged string
	if err = f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE id=? AND version=?`, request.WorkflowID, request.WorkflowVersion).Scan(&unchanged); err != nil || unchanged != original {
		t.Fatal("preview changed the published definition", err)
	}
	replay, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil || replay.ID != plan.ID || stringify(replay.TestBindings) != stringify(plan.TestBindings) {
		t.Fatal("preview did not recover the original test bindings", replay, err)
	}
	changed := request
	changed.TestBindings = nil
	_, err = f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, changed)
	dispatchTestBindingStatus(t, err, 409)
	start := TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
	if err != nil || run.ID != plan.RunID {
		t.Fatal("fixed test plan did not start atomically", err)
	}
	_, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "pause", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
	if err != nil || recovered.ID != run.ID || dispatchOptionsCount(t, f, "dag_runs") != 1 {
		t.Fatal("uncertain start recovery reapplied current eligibility", recovered, err)
	}
}

func TestTaskDispatchExplicitTestsRejectInvalidTargetsAndCurrentQualification(t *testing.T) {
	for _, name := range []string{"non_test", "missing", "duplicate", "too_many", "version", "same_worker", "no_report", "paused", "stale_inventory", "different_producer", "artifact", "extra_capability"} {
		t.Run(name, func(t *testing.T) {
			f, request, profile := dispatchTestBindingsFixture(t)
			ctx := context.Background()
			status := 400
			switch name {
			case "non_test":
				request.TestBindings[0].NodeID = "develop"
			case "missing":
				request.TestBindings[0].NodeID = "absent"
			case "duplicate":
				request.TestBindings = append(request.TestBindings, request.TestBindings[0])
			case "too_many":
				request.TestBindings = make([]TaskDispatchTestSelection, 65)
			case "version":
				request.TestBindings[0].ExpectedToolVersion++
				status = 409
			case "same_worker":
				if _, err := f.s.DB.Exec(`UPDATE suma_tool_profiles SET worker_id='tool-worker' WHERE id=?`, profile.ID); err != nil {
					t.Fatal(err)
				}
			case "no_report":
				other := dispatchTestBindingsProfile(t, f, "no-report-worker", "not-a-report", "")
				request.TestBindings[0].ToolProfileID, request.TestBindings[0].ExpectedToolVersion = other.ID, other.Version
			case "paused":
				if _, err := f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "pause", IdempotencyKey: newID()}); err != nil {
					t.Fatal(err)
				}
				status = 409
			case "stale_inventory":
				if _, err := f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at='2000-01-01T00:00:00Z' WHERE worker_id=?`, profile.WorkerID); err != nil {
					t.Fatal(err)
				}
				status = 409
			default:
				var raw string
				if err := f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE id=? AND version=?`, request.WorkflowID, request.WorkflowVersion).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var workflow dag.Workflow
				if err := json.Unmarshal([]byte(raw), &workflow); err != nil {
					t.Fatal(err)
				}
				if name == "different_producer" {
					workflow.Nodes = append(workflow.Nodes, dag.Node{ID: "other", Type: "agent", Command: []string{"unused"}})
					workflow.Nodes[1].Config["candidate_from"] = "other"
				} else if name == "artifact" {
					workflow.Nodes[1].Config["artifact_from"] = "develop"
				} else {
					workflow.Nodes[1].Config["capability"] = "custom:privileged"
				}
				if _, err := f.s.DB.Exec(`UPDATE dag_workflows SET spec=? WHERE id=? AND version=?`, stringify(workflow), request.WorkflowID, request.WorkflowVersion); err != nil {
					t.Fatal(err)
				}
			}
			_, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, request)
			dispatchTestBindingStatus(t, err, status)
			if dispatchOptionsCount(t, f, "suma_task_dispatch_plans") != 0 || dispatchOptionsCount(t, f, "dag_runs") != 0 {
				t.Fatal("rejected test override left admission effects")
			}
		})
	}
}

func TestTaskDispatchExplicitTestsRecheckFrozenWorkflowAndCurrentSourceBeforeStart(t *testing.T) {
	for _, name := range []string{"command", "profile_hash", "worker_metadata", "candidate_source", "paused", "membership"} {
		t.Run(name, func(t *testing.T) {
			f, request, profile := dispatchTestBindingsFixture(t)
			ctx := context.Background()
			plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, request)
			if err != nil {
				t.Fatal(err)
			}
			status := 409
			switch name {
			case "paused":
				_, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "pause", IdempotencyKey: newID()})
			case "membership":
				_, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID)
				if err == nil {
					_, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, f.user.ID)
				}
				status = 403
			default:
				switch name {
				case "command":
					plan.Workflow.Nodes[1].Command[0] = "another-test"
				case "profile_hash":
					plan.Workflow.Nodes[1].Config["tool_profile_sha256"] = strings.Repeat("c", 64)
				case "worker_metadata":
					plan.TestBindings[0].WorkerID = "unrelated-worker"
				case "candidate_source":
					// Keep a compile-valid alternate ancestor so the failure proves
					// our frozen source check, not merely the DAG's missing-ID gate.
					plan.Workflow.Nodes = append(plan.Workflow.Nodes, dag.Node{ID: "other", Type: "command", Command: []string{"never-executed"}})
					plan.Workflow.Nodes[1].DependsOn = append(plan.Workflow.Nodes[1].DependsOn, "other")
					plan.Workflow.Nodes[1].Config["candidate_from"] = "other"
				}
				_, err = f.s.DB.Exec(`UPDATE suma_task_dispatch_plans SET payload=? WHERE id=?`, stringify(plan), plan.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			view, readErr := f.s.GetTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID)
			if status == 403 {
				dispatchTestBindingStatus(t, readErr, status)
			} else if readErr != nil || view.Valid || view.Reason == "" {
				t.Fatal("GET concealed the changed test basis", view, readErr)
			}
			_, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
			dispatchTestBindingStatus(t, err, status)
			if dispatchOptionsCount(t, f, "dag_runs") != 0 {
				t.Fatal("stale or inconsistent test binding started a run")
			}
		})
	}
}
