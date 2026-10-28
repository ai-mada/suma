package main

import (
	"github.com/ai-mada/suma/internal/platform"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func applicationReleaseSetModels() []any {
	return []any{platform.ApplicationReleaseSet{}, platform.ApplicationReleaseSetPreviewRequest{}, platform.ApplicationReleaseSetPreview{}, platform.ApplicationReleaseSetRegisterRequest{}, platform.ApplicationReleaseSetView{}, platform.ApplicationReleaseSetSummary{}, platform.ApplicationReleaseSetPage{}, platform.ApplicationReleaseSetOptions{}, platform.ApplicationReleaseSetDeploymentPreviewRequest{}, platform.ApplicationReleaseSetDeploymentPreview{}, platform.ApplicationReleaseSetDeploymentPrepareRequest{}, platform.ApplicationReleaseSetPlanGroupView{}, rc.ApplicationReleaseSetRef{}}
}
func applicationReleaseSetContracts(add operationAdder, api map[string]any) {
	for _, r := range []struct{ path, method, input, output, status string }{
		{"/application-release-sets", "get", "", "ApplicationReleaseSetPage", "200"},
		{"/application-release-sets/options", "get", "", "ApplicationReleaseSetOptions", "200"},
		{"/application-release-sets/preview", "post", "ApplicationReleaseSetPreviewRequest", "ApplicationReleaseSetPreview", "200"},
		{"/application-release-sets", "post", "ApplicationReleaseSetRegisterRequest", "ApplicationReleaseSetView", "201"},
		{"/application-release-sets/{id}", "get", "", "ApplicationReleaseSetView", "200"},
		{"/application-release-sets/{id}/deployment-preview", "post", "ApplicationReleaseSetDeploymentPreviewRequest", "ApplicationReleaseSetDeploymentPreview", "200"},
		{"/application-release-sets/{id}/deployment-plans", "post", "ApplicationReleaseSetDeploymentPrepareRequest", "ApplicationReleaseSetPlanGroupView", "201"},
	} {
		read := r.method == "get" || r.input == "ApplicationReleaseSetPreviewRequest" || r.input == "ApplicationReleaseSetDeploymentPreviewRequest"
		op := add(r.path, r.method, r.output, false, !read)
		successStatus(op, r.status)
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Current-authorized reviewed associations of exact immutable application code versions with independently proven artifact candidates, exact dependency closure and original per-member deployment plans. Member/code/build facts remain separate. Registration or plan preparation grants no approval, execution or atomic cross-environment success."
		if r.input != "" {
			request(op, r.input)
		}
		if read {
			op["x-read-only"] = true
		} else {
			op["x-write-actor"] = "browser"
		}
		if !read {
			browserOnly(op)
		}
		if r.method == "get" && r.path == "/application-release-sets" {
			appendQuery(op, queryParameter("limit", "Default20, maximum100.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("before", "Current-project page cursor.", map[string]any{"type": "string", "maxLength": 64}))
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"ApplicationReleaseSetMemberInput", "ApplicationReleaseSetMember", "ApplicationReleaseSetDependency", "ApplicationReleaseSetBlocker", "ApplicationReleaseSet", "ApplicationReleaseSetPreviewRequest", "ApplicationReleaseSetPreview", "ApplicationReleaseSetRegisterRequest", "ApplicationReleaseSetPlanBinding", "ApplicationReleaseSetPlanGroup", "ApplicationReleaseSetRolloutItem", "ApplicationReleaseSetPlanGroupView", "ApplicationReleaseSetView", "ApplicationReleaseSetSummary", "ApplicationReleaseSetPage", "ApplicationReleaseSetCandidateOption", "ApplicationReleaseSetApplicationOption", "ApplicationReleaseSetOptions", "ApplicationReleaseSetEnvironmentInput", "ApplicationReleaseSetDeploymentPreviewRequest", "ApplicationReleaseSetDeploymentPreviewItem", "ApplicationReleaseSetDeploymentPreview", "ApplicationReleaseSetDeploymentPrepareRequest", "ApplicationReleaseSetRef"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	field := func(model, name string, attrs map[string]any) {
		v := toolContractMap(schemas, model, "properties", name)
		for k, a := range attrs {
			v[k] = a
		}
	}
	for _, name := range []string{"ApplicationReleaseSetPreviewRequest", "ApplicationReleaseSetRegisterRequest", "ApplicationReleaseSet"} {
		field(name, "members", map[string]any{"minItems": 1, "maxItems": 16})
		field(name, "label", map[string]any{"minLength": 1, "maxLength": 64, "x-max-utf8-bytes": 64})
	}
	for _, name := range []string{"ApplicationReleaseSetDeploymentPreviewRequest", "ApplicationReleaseSetDeploymentPrepareRequest"} {
		field(name, "environments", map[string]any{"minItems": 1, "maxItems": 16})
	}
	for _, name := range []string{"ApplicationReleaseSetRegisterRequest", "ApplicationReleaseSetDeploymentPrepareRequest"} {
		field(name, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
		field(name, "expected_preview_sha256", map[string]any{"pattern": "^[a-f0-9]{64}$"})
	}
	field("ApplicationReleaseSetEnvironmentInput", "expected_environment_generation", map[string]any{"minimum": 0, "maximum": 9007199254740990})
	field("ApplicationReleaseSet", "unverified_compatibility_acknowledged", map[string]any{"description": "Explicit human acknowledgement of unverified declared ranges; never a test or compatibility proof."})
	field("ReleasePlanContent", "application_release_set", map[string]any{"description": "Optional exact immutable member reference included in the original plan digest and provider authorization. Current set, code, candidate and quality facts are checked at the original approval/execution/effect gates."})
}
