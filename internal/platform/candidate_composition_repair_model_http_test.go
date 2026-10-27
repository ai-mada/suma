package platform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCandidateRepairModelHTTPSelectionGuardsAndRevision(t *testing.T) {
	h, original, req, _, _, deployments := repairModelFixture(t)
	f := h.f
	selection := repairModelSelection(deployments[0])
	req.ModelExecutionSelection = &selection
	path := "/candidate-compositions/" + original.ID + "/repairs"
	tasks := expertCount(t, f, "suma_tasks")
	for _, mutate := range []func(map[string]any){
		func(body map[string]any) { body["model_execution_selection"] = nil },
		func(body map[string]any) {
			body["model_execution_selection"].(map[string]any)["deployment_revision"] = json.Number("1e0")
		},
		func(body map[string]any) {
			body["model_execution_selection"].(map[string]any)["parameters"].(map[string]any)["max_steps"] = 17
		},
		func(body map[string]any) {
			body["model_execution_selection"].(map[string]any)["parameters"].(map[string]any)["temperature"] = 0
		},
		func(body map[string]any) {
			body["model_execution_selection"].(map[string]any)["budget"].(map[string]any)["token_limit"] = nil
		},
	} {
		body := candidateJSON(t, req)
		mutate(body)
		f.call("POST", path, body, 400)
	}
	if expertCount(t, f, "suma_tasks") != tasks || expertCount(t, f, "suma_candidate_composition_repair_sources") != 0 {
		t.Fatal("invalid HTTP selection created repair state")
	}
	var plan CandidateCompositionPlan
	if err := json.Unmarshal([]byte(stringify(f.call("POST", path, req, 201))), &plan); err != nil || plan.ModelExecution == nil || plan.ModelExecution.Deployment != deployments[0] || plan.RunID != "" {
		t.Fatal("HTTP create dropped the fixed model", plan, err)
	}
	var view CandidateCompositionRepairRevisionContext
	if err := json.Unmarshal([]byte(stringify(f.call("GET", "/candidate-compositions/"+plan.ID+"/repair-revision-context?project_id="+f.project, nil, 200))), &view); err != nil || !view.CanRevise || view.ConflictBasis == nil {
		t.Fatal(view, err)
	}
	choice := repairModelSelection(deployments[1])
	revision := CandidateCompositionRepairRevisionRequest{ProjectID: f.project, ExpectedVersion: plan.Version, ExpectedTaskVersion: view.TaskVersion, ExpectedConflictSHA256: view.ConflictBasis.SHA256, WorkflowID: req.WorkflowID, WorkflowVersion: req.WorkflowVersion, ProducerNodeID: req.ProducerNodeID, ToolProfileID: req.ToolProfileID, ExpectedToolVersion: req.ExpectedToolVersion, ModelExecutionSelection: &choice, Reason: "Review the replacement native model", Acknowledge: true, IdempotencyKey: newID()}
	revisionPath := "/candidate-compositions/" + plan.ID + "/repair-revisions"
	bad := candidateJSON(t, revision)
	bad["model_execution_selection"] = nil
	f.call("POST", revisionPath, bad, 400)
	var child CandidateCompositionPlan
	if err := json.Unmarshal([]byte(stringify(f.call("POST", revisionPath, revision, 201))), &child); err != nil || child.ModelExecution == nil || child.ModelExecution.Deployment != deployments[1] || child.RunID != "" || child.TaskID != plan.TaskID {
		t.Fatal("HTTP revision dropped explicit model selection", child, err)
	}
	if output := os.Getenv("SUMA_REPAIR_MODEL_DTO_OUTPUT"); output != "" {
		if err := os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
		for name, value := range map[string]any{"create-request": req, "create-plan": plan, "revision-context": view, "revision-request": revision, "revision-plan": child} {
			raw, err := json.MarshalIndent(value, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(output, name+".json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
