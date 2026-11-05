package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/platform"
)

func mapAt(value any, keys ...string) map[string]any {
	for _, key := range keys {
		value = value.(map[string]any)[key]
	}
	return value.(map[string]any)
}
func TestEmbeddedTaskAndNullablePlanningWire(t *testing.T) {
	api, types := generate()
	schemas := mapAt(api, "components", "schemas")
	properties := mapAt(schemas, "PlannedTask", "properties")
	actual, err := json.Marshal(platform.PlannedTask{Record: platform.Record{ID: "real-task"}})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(actual, &fields); err != nil {
		t.Fatal(err)
	}
	for key := range fields {
		if _, ok := properties[key]; !ok {
			t.Fatalf("actual JSON key %q missing from schema", key)
		}
	}
	if _, exists := properties["Record"]; exists {
		t.Fatal("embedded record was represented as a nested object")
	}
	for _, pair := range [][2]string{{"TaskPlan", "estimated_hours"}, {"CapacityEntry", "available_hours"}, {"PlanningMetrics", "acceptance_percent"}} {
		value := mapAt(schemas, pair[0], "properties", pair[1])
		choices := value["anyOf"].([]any)
		if choices[1].(map[string]any)["type"] != "null" {
			t.Fatalf("%v lost null", pair)
		}
	}
	if !strings.Contains(types, "estimated_hours: number | null;") || !strings.Contains(types, "available_hours: number | null;") {
		t.Fatal("generated client type lost nullability")
	}
	for _, name := range []string{"idempotent", "compensatable", "workspace_evidence"} {
		if _, ok := mapAt(schemas, "NodeRun", "properties")[name]; !ok {
			t.Fatalf("NodeRun missing %s", name)
		}
	}
}
func TestGovernanceAndRunnerSecurityAndSecretBoundaries(t *testing.T) {
	api, _ := generate()
	paths := mapAt(api, "paths")
	accept := mapAt(paths, "/auth/invitations/accept", "post")
	if len(accept["security"].([]any)) != 0 {
		t.Fatal("public invitation acceptance requires authentication")
	}
	for _, path := range []string{"/organizations", "/organization/members", "/organization/invitations", "/teams", "/teams/{id}/members", "/project-access"} {
		op := mapAt(paths, path, "get")
		security := op["security"].([]any)
		if len(security) != 1 || security[0].(map[string]any)["cookieAuth"] == nil {
			t.Fatalf("%s permits scoped bearer credentials", path)
		}
		if path != "/project-access" {
			for _, raw := range op["parameters"].([]any) {
				if raw.(map[string]any)["name"] == "project_id" {
					t.Fatalf("%s incorrectly requires project scope", path)
				}
			}
		}
	}
	progress := mapAt(paths, "/runner/progress", "post")
	if progress["x-required-token-scope"] != "runner" {
		t.Fatal("progress does not require Runner scope")
	}
	if progress["security"].([]any)[0].(map[string]any)["runnerAuth"] == nil {
		t.Fatal("progress security scheme is not Runner-only")
	}
	schemas := mapAt(api, "components", "schemas")
	metadata := mapAt(schemas, "InvitationMetadata", "properties")
	for _, secret := range []string{"token", "token_hash", "password"} {
		if _, exists := metadata[secret]; exists {
			t.Fatalf("listing schema exposes %s", secret)
		}
	}
	for _, secret := range []string{"token", "password"} {
		if mapAt(schemas, "InvitationAcceptRequest", "properties", secret)["writeOnly"] != true {
			t.Fatalf("%s is not body-only secret input", secret)
		}
	}
	if mapAt(schemas, "InvitationCreation")["additionalProperties"] != false {
		t.Fatal("secret receipt schema does not declare its audited field boundary")
	}
	for _, key := range mapAt(schemas, "InvitationCreation")["required"].([]string) {
		if key == "token" {
			t.Fatal("replay incorrectly requires plaintext token")
		}
	}
	for _, raw := range accept["parameters"].([]any) {
		if raw.(map[string]any)["name"] == "token" {
			t.Fatal("invitation token exposed in query")
		}
	}
}
func TestPlanningRoutesPagingAndCoverage(t *testing.T) {
	api, _ := generate()
	paths := mapAt(api, "paths")
	for _, path := range []string{"/planning", "/iterations", "/iterations/{id}", "/milestones", "/milestones/{id}", "/planning/schedule", "/planning/capacity", "/planning/baselines", "/planning/baselines/{id}", "/auth/switch", "/organization/members/{user_id}", "/organization/invitations/{id}/revoke"} {
		if _, exists := paths[path]; !exists {
			t.Fatalf("actual route %s missing", path)
		}
	}
	for _, path := range []string{"/planning", "/planning/baselines/{id}"} {
		op := mapAt(paths, path, "get")
		params := map[string]map[string]any{}
		for _, raw := range op["parameters"].([]any) {
			p := raw.(map[string]any)
			params[p["name"].(string)] = p
		}
		if mapAt(params["limit"], "schema")["default"] != 200 || params["cursor"] == nil {
			t.Fatalf("%s lacks exact paging contract", path)
		}
		if path == "/planning" && (params["task_id"] == nil || params["week_start"] == nil) {
			t.Fatal("planning exact task/week selectors missing")
		}
	}
	schemas := mapAt(api, "components", "schemas")
	if mapAt(schemas, "PlanningResponse", "properties", "coverage")["$ref"] != "#/components/schemas/PlanningCoverage" {
		t.Fatal("task coverage metadata missing")
	}
	if mapAt(schemas, "BaselineDetail", "properties", "snapshot")["$ref"] != "#/components/schemas/BaselineSnapshot" {
		t.Fatal("immutable snapshot paging missing")
	}
	if mapAt(schemas, "BaselineDetail", "properties", "comparison")["$ref"] != "#/components/schemas/BaselineComparison" {
		t.Fatal("full-scope comparison missing")
	}
	for _, key := range []string{"expected_task_version", "expected_version", "estimated_hours"} {
		if _, ok := mapAt(schemas, "ScheduleChange", "properties")[key]; !ok {
			t.Fatalf("schedule missing %s", key)
		}
	}
}
func TestModelClosureAndDeterministicGeneration(t *testing.T) {
	first, firstTypes := generate()
	second, secondTypes := generate()
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) || firstTypes != secondTypes {
		t.Fatal("contract generation is not deterministic")
	}
	schemas := mapAt(first, "components", "schemas")
	var visit func(any)
	visit = func(value any) {
		switch item := value.(type) {
		case map[string]any:
			if ref, ok := item["$ref"].(string); ok {
				name := strings.TrimPrefix(ref, "#/components/schemas/")
				if _, exists := schemas[name]; !exists {
					t.Fatalf("unresolved reference %s", ref)
				}
			}
			for _, child := range item {
				visit(child)
			}
		case []any:
			for _, child := range item {
				visit(child)
			}
		}
	}
	visit(first)
	if !strings.Contains(firstTypes, "export interface PlanningCoverage") || !strings.Contains(firstTypes, "export interface InvitationGrant") {
		t.Fatal("nested models omitted during map iteration")
	}
	if schema(reflect.TypeOf((*float64)(nil)))["anyOf"] == nil {
		t.Fatal("pointer schema is not nullable")
	}
}
func TestCheckModeReportsDriftWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contract.ts")
	if err := emit(path, []byte("original"), false); err != nil {
		t.Fatal(err)
	}
	if err := emit(path, []byte("original"), true); err != nil {
		t.Fatal(err)
	}
	if err := emit(path, []byte("changed"), true); err == nil {
		t.Fatal("contract drift was accepted")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original" {
		t.Fatal("check mode rewrote the delivery artifact")
	}
}

func TestReleaseRoutesWireAndAuthorityBoundaries(t *testing.T) {
	api, types := generate()
	paths := mapAt(api, "paths")
	for _, route := range []struct{ path, method, model string }{{"/artifacts", "get", "ReleaseArtifact"}, {"/release-candidates", "post", "ReleaseCandidateView"}, {"/release-workers", "get", "ReleaseWorkerList"}, {"/release-workers", "post", "ReleaseWorkerCreation"}, {"/release-workers/{id}", "patch", "ReleaseWorker"}, {"/environments", "get", "ReleaseEnvironmentList"}, {"/release-plans/preview", "post", "ReleasePlanPreview"}, {"/release-plans/{id}/execute", "post", "ReleaseDeployment"}, {"/deployments/{id}/rollback-preview", "post", "ReleaseRollbackPreview"}, {"/runner/artifact-uploads", "post", "ArtifactUploadState"}, {"/runner/artifacts/resolve", "post", "ArtifactBinding"}, {"/runner/release-authorizations", "post", "ReleaseAuthorization"}, {"/provider/deployment-authorizations/consume", "post", "DeploymentAuthorizationGrant"}} {
		op := mapAt(paths, route.path, route.method)
		status := "200"
		if _, ok := mapAt(op, "responses")["201"]; ok {
			status = "201"
		}
		response := responseSchema(op, status)
		if route.path == "/artifacts" {
			response = mapAt(response, "properties", "items", "items")
		}
		if response["$ref"] != "#/components/schemas/"+route.model {
			t.Fatalf("%s response does not reflect %s: %#v", route.path, route.model, response)
		}
	}
	for _, path := range []string{"/release-workers", "/environments", "/release-plans/{id}/approve", "/release-plans/{id}/execute", "/deployments/{id}/reconcile"} {
		op := mapAt(paths, path, "post")
		security := op["security"].([]any)
		if len(security) != 1 || security[0].(map[string]any)["cookieAuth"] == nil {
			t.Fatalf("%s permits non-browser deployment authority", path)
		}
	}
	if mapAt(paths, "/runner/release-authorizations", "post")["x-required-token-scope"] != "runner" {
		t.Fatal("release ticket is not Runner-only")
	}
	provider := mapAt(paths, "/provider/deployment-authorizations/consume", "post")
	if provider["security"].([]any)[0].(map[string]any)["providerAuth"] == nil {
		t.Fatal("provider request uses browser or generic Runner authority")
	}
	for _, param := range provider["parameters"].([]any) {
		if param.(map[string]any)["name"] == "project_id" {
			t.Fatal("provider authority trusts caller-selected project")
		}
	}
	if mapAt(api, "components", "securitySchemes")["providerAuth"] == nil {
		t.Fatal("independent provider scheme missing")
	}
	schemas := mapAt(api, "components", "schemas")
	for _, pair := range [][2]string{{"Assignment", "release_execution"}, {"Result", "release_evidence"}, {"Result", "artifact_evidence"}, {"NodeRun", "artifact_evidence"}} {
		if mapAt(schemas, pair[0], "properties")[pair[1]] == nil {
			t.Fatalf("actual wire field missing: %v", pair)
		}
	}
	capability := mapAt(schemas, "ReleaseWorkerList", "properties", "credential_management_allowed")
	if capability["type"] != "boolean" || capability["readOnly"] != true {
		t.Fatalf("worker list must report the current read-only management capability: %#v", capability)
	}
	for _, model := range []string{"Assignment", "ReleaseExecution", "NonSecretDeploymentRequest", "ReleaseDeployment", "ReleaseWorker", "ReleaseWorkerList"} {
		for _, key := range []string{"authorization_ticket", "token", "token_hash"} {
			if mapAt(schemas, model, "properties")[key] != nil {
				t.Fatalf("%s leaks %s", model, key)
			}
		}
	}
	if mapAt(schemas, "ReleaseWorkerCreation", "properties", "token")["readOnly"] != true || mapAt(schemas, "ConsumeDeploymentAuthorization", "properties", "authorization_ticket")["writeOnly"] != true {
		t.Fatal("once-only secret direction omitted")
	}
	for _, key := range mapAt(schemas, "ReleaseWorkerRegisterRequest")["required"].([]string) {
		if key == "deployment" || key == "profiles" {
			t.Fatal("worker profile/deployment alternatives incorrectly both required")
		}
	}
	properties := mapAt(schemas, "ReleaseCandidateCreateRequest", "properties")
	for _, key := range []string{"build_attestation_id", "package_test_attestation_id", "tasks"} {
		if properties[key] == nil {
			t.Fatalf("candidate final body lacks %s", key)
		}
	}
	for _, fake := range []string{"passed", "build_run_id", "outcome", "source_commit"} {
		if properties[fake] != nil {
			t.Fatalf("candidate permits invented proof input %s", fake)
		}
	}
	if mapAt(schemas, "ProjectVersionedOperation")["additionalProperties"] != false {
		t.Fatal("reconcile accepts arbitrary outcome fields")
	}
	for _, name := range []string{"ReleaseExecution", "ReleaseResult", "ReleaseCandidateView", "ArtifactManifest", "ReleaseWorkerCreation"} {
		if !strings.Contains(types, "export interface "+name+" {") {
			t.Fatalf("client type omitted %s", name)
		}
	}
}

func TestReleasePagingAndExactDecisionSchemas(t *testing.T) {
	api, _ := generate()
	paths := mapAt(api, "paths")
	for _, path := range []string{"/artifacts", "/release-candidates"} {
		op := mapAt(paths, path, "get")
		params := map[string]map[string]any{}
		for _, raw := range op["parameters"].([]any) {
			p := raw.(map[string]any)
			params[p["name"].(string)] = p
		}
		if mapAt(params["limit"], "schema")["default"] != 20 || mapAt(params["limit"], "schema")["maximum"] != 100 || params["cursor"] == nil {
			t.Fatalf("%s page contract incorrect", path)
		}
	}
	for _, path := range []string{"/release-plans", "/deployments", "/release-workers", "/release-providers"} {
		op := mapAt(paths, path, "get")
		response := responseSchema(op, "200")
		if ref, ok := response["$ref"].(string); ok {
			response = mapAt(api, "components", "schemas", strings.TrimPrefix(ref, "#/components/schemas/"))
		}
		if mapAt(response, "properties")["next_cursor"] != nil {
			t.Fatalf("%s claims unsupported pagination", path)
		}
	}
	schema := mapAt(paths, "/release-plans/{id}/approve", "post", "requestBody", "content", "application/json", "schema")
	if schema["$ref"] != "#/components/schemas/ReleasePlanDecisionRequest" {
		t.Fatal("exact approval request not typed")
	}
	props := mapAt(api, "components", "schemas", "ReleasePlanDecisionRequest", "properties")
	for _, key := range []string{"project_id", "expected_version", "idempotency_key", "plan_revision", "plan_sha256", "decision", "reason"} {
		if props[key] == nil {
			t.Fatalf("decision lacks %s", key)
		}
	}
	binary := mapAt(paths, "/runner/artifact-uploads/{id}/blobs/{sha256}", "put", "requestBody", "content")
	if binary["application/octet-stream"] == nil || binary["application/json"] != nil {
		t.Fatal("actual bytes misrepresented as JSON upload")
	}
}
