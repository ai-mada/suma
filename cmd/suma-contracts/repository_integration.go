package main

import (
	"github.com/ai-mada/suma/internal/platform"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func repositoryIntegrationModels() []any {
	return []any{
		platform.PoolRepositoryAuthorizationRequest{}, platform.RepositoryIntegrationTarget{}, platform.RepositoryIntegrationBasis{}, platform.RepositoryIntegrationApproval{}, platform.RepositoryIntegrationPlan{}, platform.RepositoryIntegrationSourceChoice{}, platform.RepositoryIntegrationProviderChoice{}, platform.RepositoryIntegrationOptions{}, platform.RepositoryIntegrationTargetCreateRequest{}, platform.RepositoryIntegrationTargetUpdateRequest{}, platform.RepositoryIntegrationPlanCreateRequest{}, platform.RepositoryIntegrationPlanCommandRequest{}, platform.RepositoryIntegrationPlanPage{},
		ri.RepositoryExecution{}, ri.RepositoryRequest{}, ri.RepositoryNonSecretRequest{}, ri.RepositoryTargetObservation{}, ri.RepositoryFileChange{}, ri.RepositoryPreview{}, ri.RepositoryObservation{}, ri.RepositoryResult{}, ri.RepositoryAuthorizationRequest{}, ri.RepositoryAuthorization{}, ri.RepositoryConsumeAuthorization{}, ri.RepositoryInspectAuthorization{}, ri.RepositoryAuthorizationGrant{}, ri.RepositoryBeginEffect{}, ri.RepositoryEffectGrant{}, ri.RepositoryBundleRequest{}, ri.RepositoryReconcileRequest{}, ri.RepositoryConsumeReconciliation{}, ri.RepositoryReconciliationGrant{}, ri.RepositoryReceipt{},
	}
}

func repositoryIntegrationContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/repository-integrations/options", "get", "", "RepositoryIntegrationOptions", "200"},
		{"/repository-integration-targets", "get", "", "RepositoryIntegrationTarget", "200"},
		{"/repository-integration-targets", "post", "RepositoryIntegrationTargetCreateRequest", "RepositoryIntegrationTarget", "201"},
		{"/repository-integration-targets/{id}", "patch", "RepositoryIntegrationTargetUpdateRequest", "RepositoryIntegrationTarget", "200"},
		{"/repository-integrations/plans", "get", "", "RepositoryIntegrationPlanPage", "200"},
		{"/repository-integrations/plans", "post", "RepositoryIntegrationPlanCreateRequest", "RepositoryIntegrationPlan", "201"},
		{"/repository-integrations/plans/{id}", "get", "", "RepositoryIntegrationPlan", "200"},
		{"/repository-integrations/plans/{id}/commands", "post", "RepositoryIntegrationPlanCommandRequest", "RepositoryIntegrationPlan", "200"},
	} {
		op := add(route.path, route.method, route.output, route.path == "/repository-integration-targets" && route.method == "get", route.input != "")
		successStatus(op, route.status)
		if route.input != "" {
			request(op, route.input)
		}
		if route.input == "RepositoryIntegrationTargetCreateRequest" || route.input == "RepositoryIntegrationTargetUpdateRequest" {
			browserOnly(op)
		}
		op["description"] = "Governed accepted-candidate fast-forward integration. Fixed source/review/test/bundle, administrator target binding and actual bounded Git diff are independent of release artifacts. Approval binds the exact preview; explicit execution obtains a current effect grant. Git target CAS, operation receipt and fencing ref form one transaction. Unknown effects retain the target barrier and reconcile only the original provider operation."
		if route.path == "/repository-integrations/options" || route.path == "/repository-integrations/plans" && route.method == "get" {
			parameter := queryParameter("task_id", "Current project task; required for options.", map[string]any{"type": "string", "maxLength": 64})
			if route.path == "/repository-integrations/options" {
				parameter["required"] = true
			}
			appendQuery(op, parameter)
		}
		if route.path == "/repository-integrations/plans" && route.method == "get" {
			appendQuery(op, queryParameter("limit", "Exact bounded integer page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("before", "Opaque original plan boundary.", map[string]any{"type": "string", "maxLength": 4096}))
		}
		if route.input == "RepositoryIntegrationPlanCommandRequest" {
			op["x-authorization-conditions"] = "approve/reject require current browser project approver; execute/reconcile require current project approver, exact version and explicit acknowledgement; apply additionally requires a current browser approval."
		}
	}
	for _, route := range []struct{ path, input, output, auth string }{
		{"/runner/repository-integrations/authorize", "RepositoryAuthorizationRequest", "RepositoryAuthorization", "runnerAuth"},
		{"/pool-runner/repository-integrations/authorize", "PoolRepositoryAuthorizationRequest", "RepositoryAuthorization", "poolRunnerAuth"},
		{"/repository-provider/consume", "RepositoryConsumeAuthorization", "RepositoryAuthorizationGrant", "providerAuth"},
		{"/repository-provider/inspect-authorization", "RepositoryInspectAuthorization", "RepositoryAuthorizationGrant", "providerAuth"},
		{"/repository-provider/begin-effect", "RepositoryBeginEffect", "RepositoryEffectGrant", "providerAuth"},
		{"/repository-provider/bundle", "RepositoryBundleRequest", "", "providerAuth"},
		{"/repository-provider/observations", "RepositoryObservation", "RepositoryIntegrationPlan", "providerAuth"},
		{"/repository-provider/consume-reconciliation", "RepositoryConsumeReconciliation", "RepositoryReconciliationGrant", "providerAuth"},
	} {
		op := add(route.path, "post", route.output, false, true)
		successStatus(op, "200")
		request(op, route.input)
		op["security"] = []any{map[string]any{route.auth: []string{}}}
		op["description"] = "Dedicated current machine/provider capability; never a browser, API or model decision. Exact project/operation/request/assignment, lease and grants are validated. Tickets and credentials are ephemeral and must not be logged."
		if route.output == "" {
			op["responses"].(map[string]any)["200"] = map[string]any{"description": "Hash-verified sealed candidate bundle, max64MiB; requires current prepare authorization.", "content": map[string]any{"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}}
		}
	}
	field := func(model, name string, extra map[string]any) {
		p := toolContractMap(api, "components", "schemas", model, "properties", name)
		for k, v := range extra {
			p[k] = v
		}
	}
	for _, model := range []string{"RepositoryIntegrationTargetCreateRequest", "RepositoryIntegrationTargetUpdateRequest", "RepositoryIntegrationPlanCreateRequest", "RepositoryIntegrationPlanCommandRequest"} {
		toolContractMap(api, "components", "schemas", model)["additionalProperties"] = false
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	for _, pair := range [][2]string{{"RepositoryIntegrationTargetUpdateRequest", "expected_version"}, {"RepositoryIntegrationPlanCreateRequest", "expected_task_version"}, {"RepositoryIntegrationPlanCreateRequest", "expected_target_version"}, {"RepositoryIntegrationPlanCommandRequest", "expected_version"}} {
		field(pair[0], pair[1], map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	field("RepositoryIntegrationTargetUpdateRequest", "status", map[string]any{"enum": []string{"active", "paused", "retired"}})
	field("RepositoryIntegrationPlanCommandRequest", "action", map[string]any{"enum": []string{"approve", "reject", "execute", "reconcile"}})
	field("RepositoryIntegrationPlanCommandRequest", "reason", map[string]any{"minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 2000})
	field("RepositoryIntegrationPlanCommandRequest", "acknowledge", map[string]any{"description": "Explicit true for approval, execution and original-operation reconciliation. Never a manual outcome override."})
	field("RepositoryIntegrationPlan", "state", map[string]any{"enum": []string{"preparing", "ready", "approved", "rejected", "applying", "integrated", "blocked", "reconciling"}})
	field("RepositoryReconcileRequest", "request", map[string]any{"description": "Control-fixed original non-secret tuple, used only to establish a negatively fenced missing operation after a current reconciliation grant. Never authorizes apply."})
	field("RepositoryExecution", "protocol", map[string]any{"const": ri.RepositoryProtocol})
	field("RepositoryExecution", "phase", map[string]any{"enum": []string{"prepare", "apply"}})
	field("RepositoryPreview", "diff", map[string]any{"maxLength": ri.RepositoryMaxDiffBytes, "x-max-utf8-bytes": ri.RepositoryMaxDiffBytes, "description": "Actual complete bounded diff. Oversize or unsupported binary review blocks approval; no silent truncation."})
	field("RepositoryPreview", "files", map[string]any{"maxItems": ri.RepositoryMaxFiles})
	for _, pair := range [][2]string{{"RepositoryRequest", "authorization_ticket"}, {"RepositoryAuthorization", "authorization_ticket"}, {"RepositoryConsumeAuthorization", "authorization_ticket"}, {"RepositoryReconcileRequest", "reconciliation_ticket"}, {"RepositoryConsumeReconciliation", "reconciliation_ticket"}} {
		field(pair[0], pair[1], map[string]any{"format": "password", "x-sensitive": true, "description": "Ephemeral bound capability; never persist or log."})
	}
}
