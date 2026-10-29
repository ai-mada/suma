package main

import (
	"github.com/ai-mada/suma/internal/platform"
)

func applicationReleaseRolloutModels() []any {
	return []any{platform.ApplicationReleaseRollout{}, platform.ApplicationReleaseRolloutMember{}, platform.ApplicationReleaseRolloutPreviewRequest{}, platform.ApplicationReleaseRolloutPreview{}, platform.ApplicationReleaseRolloutStartRequest{}, platform.ApplicationReleaseRolloutItem{}, platform.ApplicationReleaseRolloutView{}, platform.ApplicationReleaseRolloutPage{}, platform.ApplicationReleaseRolloutResumePreviewRequest{}, platform.ApplicationReleaseRolloutResumePreview{}, platform.ApplicationReleaseRolloutCommandRequest{}}
}
func applicationReleaseRolloutContracts(add operationAdder, api map[string]any) {
	for _, r := range []struct{ path, method, input, output, status string }{
		{"/application-release-sets/{set_id}/rollouts/preview", "post", "ApplicationReleaseRolloutPreviewRequest", "ApplicationReleaseRolloutPreview", "200"},
		{"/application-release-sets/{set_id}/rollouts", "post", "ApplicationReleaseRolloutStartRequest", "ApplicationReleaseRolloutView", "201"},
		{"/application-release-sets/{set_id}/rollouts", "get", "", "ApplicationReleaseRolloutPage", "200"},
		{"/application-release-sets/{set_id}/rollouts/{id}", "get", "", "ApplicationReleaseRolloutView", "200"},
		{"/application-release-sets/{set_id}/rollouts/{id}/resume-preview", "post", "ApplicationReleaseRolloutResumePreviewRequest", "ApplicationReleaseRolloutResumePreview", "200"},
		{"/application-release-sets/{set_id}/rollouts/{id}/commands", "post", "ApplicationReleaseRolloutCommandRequest", "ApplicationReleaseRolloutView", "200"},
	} {
		read := r.method == "get" || r.input == "ApplicationReleaseRolloutPreviewRequest" || r.input == "ApplicationReleaseRolloutResumePreviewRequest"
		op := add(r.path, r.method, r.output, false, !read)
		successStatus(op, r.status)
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Protected Go-DAG child execution over exact original approved release plans. Only a service-owned opaque binding may resolve a reserved child run. Dependency success requires the original DAG and actual healthy deployment. Pause stops new dispatch while active children drain; unknown effects require original-operation reconciliation and explicit reviewed resume. No group approval, blind retry or automatic rollback is granted. Current identity, set, group, plan and artifact facts are rechecked."
		if r.input != "" {
			request(op, r.input)
		}
		if read {
			op["x-read-only"] = true
		}
		if r.method != "get" {
			browserOnly(op)
		}
		if r.path == "/application-release-sets/{set_id}/rollouts" && r.method == "get" {
			appendQuery(op, queryParameter("plan_group_id", "Optional exact plan group.", map[string]any{"type": "string", "maxLength": 64}))
			appendQuery(op, queryParameter("limit", "Default20, maximum100.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("before", "Current-set page cursor.", map[string]any{"type": "string", "maxLength": 64}))
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"ApplicationReleaseRollout", "ApplicationReleaseRolloutMember", "ApplicationReleaseRolloutPreviewRequest", "ApplicationReleaseRolloutPreview", "ApplicationReleaseRolloutStartRequest", "ApplicationReleaseRolloutItem", "ApplicationReleaseRolloutView", "ApplicationReleaseRolloutPage", "ApplicationReleaseRolloutResumePreviewRequest", "ApplicationReleaseRolloutResumePreview", "ApplicationReleaseRolloutCommandRequest"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	field := func(model, name string, attrs map[string]any) {
		v := toolContractMap(schemas, model, "properties", name)
		for k, a := range attrs {
			v[k] = a
		}
	}
	field("ApplicationReleaseRollout", "members", map[string]any{"minItems": 1, "maxItems": 16})
	field("ApplicationReleaseRolloutMember", "depends_on", map[string]any{"maxItems": 15})
	field("ApplicationReleaseRolloutCommandRequest", "action", map[string]any{"enum": []string{"pause", "resume", "cancel"}})
	for _, name := range []string{"ApplicationReleaseRolloutStartRequest", "ApplicationReleaseRolloutCommandRequest"} {
		field(name, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	for _, name := range []string{"ApplicationReleaseRolloutResumePreviewRequest", "ApplicationReleaseRolloutCommandRequest"} {
		field(name, "expected_run_version", map[string]any{"minimum": 1, "maximum": 9007199254740990})
		field(name, "expected_rollout_sha256", map[string]any{"pattern": "^[a-f0-9]{64}$"})
	}
	field("ApplicationReleaseRolloutCommandRequest", "expected_preview_sha256", map[string]any{"description": "Required exact current resume preview for action=resume; must be empty for pause/cancel."})
	field("ApplicationReleaseRolloutPreview", "preview_sha256", map[string]any{"description": "Go canonical SHA256 of {protocol:suma-specific preview marker, rollout:public draft}; generated IDs/parent/binding IDs, final hash, started-preview hash and created identity/time are empty. Exact original plan revision/version/SHA and reserved child run/node remain included. See the contract and Go-derived golden."})
	field("ApplicationReleaseRolloutView", "plan_group", map[string]any{"description": "Exact immutable original group, also returned for a historical rollout outside the bounded set history window. Never inferred from a group identifier alone."})
}
