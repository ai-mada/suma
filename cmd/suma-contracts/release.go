package main

import (
	"github.com/ai-mada/suma/internal/platform"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

// Map-backed requests/receipts are audited against the actual HTTP handlers.
// Shared execution and domain models below remain reflected from source.
type ReleaseWorkerRegisterRequest struct {
	ProjectID      string                                `json:"project_id"`
	WorkerID       string                                `json:"worker_id"`
	Profiles       []platform.ReleaseProfileRegistration `json:"profiles,omitempty"`
	Deployment     bool                                  `json:"deployment,omitempty"`
	IdempotencyKey string                                `json:"idempotency_key"`
}
type ReleaseWorkerCreation struct {
	platform.ReleaseWorker
	Token string `json:"token,omitempty"`
}
type ReleaseWorkerRevokeRequest struct {
	ProjectVersionedOperation
	Status string `json:"status"`
}
type ProjectVersionedOperation struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type ReleaseCandidateTaskInput struct {
	ID              string `json:"id"`
	ExpectedVersion int    `json:"expected_version"`
}
type ReleaseCandidateCreateRequest struct {
	ProjectID                string                      `json:"project_id"`
	Name                     string                      `json:"name"`
	ArtifactID               string                      `json:"artifact_id"`
	BuildAttestationID       string                      `json:"build_attestation_id"`
	PackageTestAttestationID string                      `json:"package_test_attestation_id"`
	Tasks                    []ReleaseCandidateTaskInput `json:"tasks"`
	IdempotencyKey           string                      `json:"idempotency_key"`
}
type ReleaseProviderOption struct {
	ID              string   `json:"id"`
	SiteIDs         []string `json:"site_ids"`
	Kind            string   `json:"kind"`
	CapabilityState string   `json:"capability_state"`
}
type ReleaseEnvironmentList struct {
	Items          []platform.ReleaseEnvironment `json:"items"`
	Total          int                           `json:"total"`
	AllowedActions []string                      `json:"allowed_actions"`
}
type ReleaseEnvironmentCreateRequest struct {
	ProjectID      string           `json:"project_id"`
	Name           string           `json:"name"`
	ProviderID     string           `json:"provider_id"`
	SiteID         string           `json:"site_id"`
	Health         *rc.HealthPolicy `json:"health,omitempty"`
	IdempotencyKey string           `json:"idempotency_key"`
}
type ReleaseEnvironmentUpdateRequest struct {
	ProjectVersionedOperation
	Name   *string          `json:"name,omitempty"`
	Health *rc.HealthPolicy `json:"health,omitempty"`
	Status *string          `json:"status,omitempty"`
}
type ReleasePlanExecuteRequest struct {
	ProjectVersionedOperation
	PlanRevision int    `json:"plan_revision"`
	PlanSHA256   string `json:"plan_sha256"`
}
type ReleasePlanDecisionRequest struct {
	ReleasePlanExecuteRequest
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}
type ReleaseRollbackPreviewRequest struct {
	ProjectID                     string `json:"project_id"`
	ExpectedEnvironmentGeneration int64  `json:"expected_environment_generation"`
}
type ReleaseRollbackPreview struct {
	Input   platform.ReleasePlanInput   `json:"input"`
	Preview platform.ReleasePlanPreview `json:"preview"`
}

func releaseModels() []any {
	return []any{platform.ReleaseWorker{}, platform.ReleaseWorkerRotationRequest{}, platform.ReleaseWorkerRotationResult{}, platform.ReleaseWorkerUploadRecoveryItem{}, platform.ReleaseWorkerUploadRecoveryView{}, platform.ReleaseWorkerUploadAbandonRequest{}, platform.ReleaseWorkerUploadAbandonResult{}, platform.ReleaseWorkerCandidateRecoveryItem{}, platform.ReleaseWorkerCandidateRecoveryView{}, platform.ReleaseWorkerCandidateAbandonRequest{}, platform.ReleaseWorkerCandidateAbandonResult{}, platform.ReleaseCandidateView{}, platform.ReleaseArtifact{}, platform.ReleaseEnvironment{}, platform.ReleasePlan{}, platform.ReleasePlanInput{}, platform.ReleasePlanPreview{}, platform.ReleaseDeployment{}, ProjectVersionedOperation{}, ReleaseWorkerRegisterRequest{}, ReleaseWorkerCreation{}, ReleaseWorkerRevokeRequest{}, ReleaseCandidateCreateRequest{}, ReleaseProviderOption{}, ReleaseEnvironmentList{}, ReleaseEnvironmentCreateRequest{}, ReleaseEnvironmentUpdateRequest{}, ReleasePlanExecuteRequest{}, ReleasePlanDecisionRequest{}, ReleaseRollbackPreviewRequest{}, ReleaseRollbackPreview{}, rc.ArtifactUploadRequest{}, rc.ArtifactUploadState{}, rc.ArtifactSealRequest{}, rc.ArtifactSealReceipt{}, rc.ArtifactResolveRequest{}, rc.ArtifactBinding{}, rc.ReleaseAuthorizationRequest{}, rc.ReleaseAuthorization{}, rc.DeploymentRequest{}, rc.NonSecretDeploymentRequest{}, rc.ConsumeDeploymentAuthorization{}, rc.InspectDeploymentAuthorization{}, rc.DeploymentAuthorizationGrant{}, rc.BeginDeploymentEffect{}, rc.DeploymentEffectGrant{}}
}

func releaseContracts(add operationAdder) {
	single := func(path, method, model string, body bool, status string) map[string]any {
		op := add(path, method, model, false, body)
		successStatus(op, status)
		return op
	}
	completeList := func(path, model string) map[string]any {
		op := add(path, "get", model, true, false)
		successStatus(op, "200")
		s := responseSchema(op, "200")
		delete(s["properties"].(map[string]any), "next_cursor")
		op["description"] = "Complete current-project list; this route does not implement limit/cursor pagination."
		return op
	}
	pagedList := func(path, model string) {
		op := add(path, "get", model, true, false)
		successStatus(op, "200")
		appendQuery(op, queryParameter("limit", "Default 20; maximum 100.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
		appendQuery(op, queryParameter("cursor", "Opaque cursor scoped to organization, project and collection.", map[string]any{"type": "string"}))
	}
	runner := func(op map[string]any) {
		op["security"] = []any{map[string]any{"runnerAuth": []string{}}}
		op["x-required-token-scope"] = "runner"
		op["description"] = "Requires a live administrator-registered release worker credential and current assignment identity. A generic Runner token or graph approval does not grant release capability."
	}
	provider := func(op map[string]any) {
		op["security"] = []any{map[string]any{"providerAuth": []string{}}}
		op["x-required-token-scope"] = "independent_provider"
		op["description"] = "Requires administrator-configured independent provider credential. Browser, API, Agent and Runner credentials are not provider authority."
	}
	header := func(op map[string]any, name, description string) {
		op["parameters"] = append(op["parameters"].([]any), map[string]any{"in": "header", "name": name, "required": true, "description": description, "schema": map[string]any{"type": "string"}})
	}
	binary := func(op map[string]any) {
		response := op["responses"].(map[string]any)["200"].(map[string]any)
		response["content"] = map[string]any{"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}
	}

	completeList("/release-workers", "ReleaseWorker")
	op := single("/release-workers", "post", "ReleaseWorkerCreation", true, "201")
	browserOnly(op)
	request(op, "ReleaseWorkerRegisterRequest")
	op["description"] = "Live browser project administrator registers fixed build/package-test profile digests or deployment capability. Plaintext token is returned once, omitted from receipt replay and registry; never log or persist it in browser recovery storage."
	op = single("/release-workers/{id}", "patch", "ReleaseWorker", true, "200")
	browserOnly(op)
	request(op, "ReleaseWorkerRevokeRequest")
	op["description"] = "Revokes a registered worker with expected_version and stable idempotency_key; no credential is returned."
	op = single("/release-workers/{id}/rotate-credential", "post", "ReleaseWorkerRotationResult", true, "200")
	browserOnly(op)
	request(op, "ReleaseWorkerRotationRequest")
	op["description"] = "The current project administrator who owns the manually registered worker may rotate the same node after explicitly acknowledging a stop and passing authoritative drain checks. A fresh credential is shown once. Identical replay under current authority returns only the original receipt metadata. The new node must report current tools before use."
	op = single("/release-workers/{id}/artifact-uploads", "get", "ReleaseWorkerUploadRecoveryView", false, "200")
	browserOnly(op)
	appendQuery(op, queryParameter("cursor", "ID of the last upload on the preceding authorized page.", map[string]any{"type": "string", "pattern": "^[a-f0-9]{32}$"}))
	op["description"] = "Read at most 20 unsealed release uploads for the current manually registered node and credential. The original persisted attempt determines eligibility. No file paths, credentials or contents are returned."
	op = single("/release-workers/{id}/artifact-uploads/{uploadID}/abandon", "post", "ReleaseWorkerUploadAbandonResult", true, "200")
	browserOnly(op)
	request(op, "ReleaseWorkerUploadAbandonRequest")
	op["description"] = "Close an incomplete upload only after the exact original attempt is durably resolved. Preserve retained bytes and manifest, and never seal or publish a release. Exact replay requires current authority."
	op = single("/release-workers/{id}/candidate-transfers", "get", "ReleaseWorkerCandidateRecoveryView", false, "200")
	browserOnly(op)
	appendQuery(op, queryParameter("cursor", "Continue after the last examined candidate transfer. A page may omit sealed and ended transfers.", map[string]any{"type": "string", "pattern": "^[a-f0-9]{32}$"}))
	op["description"] = "Inspect scoped candidate transfers for the original manual node and current credential. Read a bounded set of records and show only safe evidence. No bundle or storage path is returned."
	op = single("/release-workers/{id}/candidate-transfers/{transferID}/abandon", "post", "ReleaseWorkerCandidateAbandonResult", true, "200")
	browserOnly(op)
	request(op, "ReleaseWorkerCandidateAbandonRequest")
	op["description"] = "End an unsealed transfer after its exact original attempt durably failed or was cancelled with a known outcome. Preserve its bundle, manifest and Git evidence, and keep the original operation receipt."
	pagedList("/artifacts", "ReleaseArtifact")
	single("/artifacts/{id}", "get", "ReleaseArtifact", false, "200")
	op = single("/artifacts/{id}/blobs/{sha256}", "get", "", false, "200")
	binary(op)
	op["description"] = "Reads actual authorized project artifact bytes by content digest."
	pagedList("/release-candidates", "ReleaseCandidateView")
	single("/release-candidates/{id}", "get", "ReleaseCandidateView", false, "200")
	op = single("/release-candidates", "post", "ReleaseCandidateView", true, "201")
	request(op, "ReleaseCandidateCreateRequest")
	op["description"] = "Atomically freezes sealed bytes, current successful build/package-test proofs for the same source/digest/run and current formally accepted task review basis. Immutable; current eligibility/blockers are returned separately. No browser passed/upload conclusion is accepted."
	completeList("/release-providers", "ReleaseProviderOption")
	single("/environments", "get", "ReleaseEnvironmentList", false, "200")
	single("/environments/{id}", "get", "ReleaseEnvironment", false, "200")
	op = single("/environments", "post", "ReleaseEnvironment", true, "201")
	browserOnly(op)
	request(op, "ReleaseEnvironmentCreateRequest")
	op["description"] = "Administrator chooses a preconfigured provider/site. Optional health defaults to index.html HTTP 200; request health must not supply expected_sha256. Configured is not verified deployment."
	op = single("/environments/{id}", "patch", "ReleaseEnvironment", true, "200")
	browserOnly(op)
	request(op, "ReleaseEnvironmentUpdateRequest")
	op["description"] = "Explicit versioned name/status/health change; provider/site identities cannot be rebound. An unknown operation barrier prevents changes."
	op = single("/release-plans/preview", "post", "ReleasePlanPreview", true, "200")
	browserOnly(op)
	request(op, "ReleasePlanInput")
	op["description"] = "Read-only preparation by an authorized developer or administrator. Uses highest reserved environment generation, not active_generation. Does not approve, reserve or deploy."
	completeList("/release-plans", "ReleasePlan")
	single("/release-plans/{id}", "get", "ReleasePlan", false, "200")
	op = single("/release-plans", "post", "ReleasePlan", true, "201")
	browserOnly(op)
	request(op, "ReleasePlanInput", "preview_sha256", "idempotency_key")
	op["description"] = "Persists immutable revision/hash after matching the exact preview and current basis. Content changes create another plan; decision version is distinct from content revision."
	op = single("/release-plans/{id}/approve", "post", "ReleasePlan", true, "200")
	browserOnly(op)
	request(op, "ReleasePlanDecisionRequest")
	op["description"] = "Live administrator approves, rejects or revokes the exact plan revision/hash with expected decision version. Revocation does not undo past external effects."
	op = single("/release-plans/{id}/execute", "post", "ReleaseDeployment", true, "201")
	browserOnly(op)
	request(op, "ReleasePlanExecuteRequest")
	op["description"] = "Rechecks live authority, exact approval, candidate/task basis, environment generation and barrier; durably creates one managed execution before any provider effect. Lost receipts replay only the original complete request/key."
	completeList("/deployments", "ReleaseDeployment")
	single("/deployments/{id}", "get", "ReleaseDeployment", false, "200")
	op = single("/deployments/{id}/reconcile", "post", "ReleaseDeployment", true, "200")
	browserOnly(op)
	request(op, "ProjectVersionedOperation")
	op["description"] = "Reads authenticated provider observation for the original operation and updates managed projection. Accepts no outcome/succeeded/health conclusion and does not redeploy. Unknown audit history is retained."
	op = single("/deployments/{id}/rollback-preview", "post", "ReleaseRollbackPreview", true, "200")
	browserOnly(op)
	request(op, "ReleaseRollbackPreviewRequest")
	op["description"] = "Returns input plus preview for a new independent rollback plan using current generation and an eligible historical candidate. Still requires create, exact approval and execute; failure never implies rollback."

	op = single("/runner/artifact-uploads", "post", "ArtifactUploadState", true, "201")
	runner(op)
	request(op, "ArtifactUploadRequest")
	op = single("/runner/artifact-uploads/{id}", "get", "ArtifactUploadState", false, "200")
	runner(op)
	op = single("/runner/artifact-uploads/{id}/blobs/{sha256}", "put", "Acknowledgement", false, "200")
	runner(op)
	header(op, "X-Suma-Operation-ID", "Stable upload operation identity; exact content digest and declared length are enforced.")
	op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary", "maxLength": rc.MaxFileBytes}}}}
	op["x-request-schema-coverage"] = "raw content-addressed bytes; server enforces digest, exact declared size and current assignment"
	op = single("/runner/artifact-uploads/{id}/seal", "post", "ArtifactSealReceipt", true, "200")
	runner(op)
	request(op, "ArtifactSealRequest")
	op = single("/runner/artifacts/resolve", "post", "ArtifactBinding", true, "200")
	runner(op)
	request(op, "ArtifactResolveRequest")
	op = single("/runner/artifacts/{id}/blobs/{sha256}", "get", "", false, "200")
	runner(op)
	header(op, "X-Suma-Assignment", "Base64url-encoded AssignmentRef for the current test attempt/fence/worker.")
	binary(op)
	op = single("/runner/release-authorizations", "post", "ReleaseAuthorization", true, "200")
	runner(op)
	request(op, "ReleaseAuthorizationRequest")
	op["description"] = "Obtains a short-lived opaque authorization ticket for the current managed assignment only. Ticket stays in memory and is excluded from Assignment, Config, journal, logs, browser and result evidence."
	for _, route := range []struct{ path, input, output string }{{"inspect", "InspectDeploymentAuthorization", "DeploymentAuthorizationGrant"}, {"consume", "ConsumeDeploymentAuthorization", "DeploymentAuthorizationGrant"}, {"begin-effect", "BeginDeploymentEffect", "DeploymentEffectGrant"}} {
		op = single("/provider/deployment-authorizations/"+route.path, "post", route.output, true, "200")
		provider(op)
		request(op, route.input)
	}
	op = single("/provider/deployment-observations", "post", "ReleaseDeployment", true, "200")
	provider(op)
	request(op, "DeploymentObservation")
	for _, path := range []string{"/provider/artifacts/{id}", "/provider/artifacts/{id}/blobs/{sha256}"} {
		model := "ReleaseArtifact"
		if path != "/provider/artifacts/{id}" {
			model = ""
		}
		op = single(path, "get", model, false, "200")
		provider(op)
		param := queryParameter("operation_id", "Original provider-authorized deployment operation; server derives its project and artifact binding.", map[string]any{"type": "string"})
		param["required"] = true
		appendQuery(op, param)
		if model == "" {
			binary(op)
		}
	}
}

func responseSchema(op map[string]any, status string) map[string]any {
	return op["responses"].(map[string]any)[status].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
}

func enhanceReleaseSchemas(schemas map[string]any) {
	field := func(model, name string, attrs map[string]any) {
		value := schemas[model].(map[string]any)["properties"].(map[string]any)[name].(map[string]any)
		for key, item := range attrs {
			value[key] = item
		}
	}
	for _, model := range []string{"ReleaseWorkerRegisterRequest", "ReleaseWorkerRevokeRequest", "ReleaseWorkerRotationRequest", "ReleaseWorkerRotationResult", "ReleaseWorkerUploadAbandonRequest", "ReleaseWorkerUploadAbandonResult", "ReleaseWorkerUploadRecoveryItem", "ReleaseWorkerUploadRecoveryView", "ReleaseWorkerCandidateAbandonRequest", "ReleaseWorkerCandidateAbandonResult", "ReleaseWorkerCandidateRecoveryItem", "ReleaseWorkerCandidateRecoveryView", "ReleaseCandidateCreateRequest", "ReleaseCandidateTaskInput", "ReleaseEnvironmentCreateRequest", "ReleaseEnvironmentUpdateRequest", "ReleasePlanInput", "ReleasePlanExecuteRequest", "ReleasePlanDecisionRequest", "ReleaseRollbackPreviewRequest", "ProjectVersionedOperation", "ArtifactUploadRequest", "ArtifactSealRequest", "ArtifactResolveRequest", "ReleaseAuthorizationRequest", "ConsumeDeploymentAuthorization", "InspectDeploymentAuthorization", "BeginDeploymentEffect", "DeploymentObservation"} {
		schemas[model].(map[string]any)["additionalProperties"] = false
	}
	field("ReleaseWorkerCreation", "token", map[string]any{"readOnly": true, "description": "Once-only plaintext credential; absent from registry and same-key replay. Memory only, no browser session/local storage."})
	field("ReleaseWorkerRevokeRequest", "status", map[string]any{"const": "revoked"})
	field("ReleaseWorker", "manual_rotation_allowed", map[string]any{"readOnly": true, "description": "The current browser administrator owns a manually registered eligible credential. This is not proof of stop, online state, or completed drain."})
	field("ReleaseWorkerRotationRequest", "acknowledge_stopped", map[string]any{"const": true, "description": "The operator has stopped the original worker. Server-side drain verification still applies."})
	field("ReleaseWorkerRotationRequest", "expected_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	field("ReleaseWorkerRotationRequest", "project_id", map[string]any{"minLength": 1, "maxLength": 64})
	field("ReleaseWorkerRotationRequest", "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	field("ReleaseWorkerRotationResult", "token", map[string]any{"readOnly": true, "pattern": "^[a-f0-9]{64}$", "description": "Once-only plaintext replacement credential. Absent from original operation replay. Keep it in memory only."})
	field("ReleaseWorkerUploadRecoveryItem", "disposition", map[string]any{"enum": []string{"eligible", "awaiting_attempt", "unverified"}})
	field("ReleaseWorkerUploadRecoveryItem", "fence", map[string]any{"pattern": "^[1-9][0-9]{0,18}$", "description": "Exact original fencing number rendered as a decimal string."})
	field("ReleaseWorkerUploadRecoveryView", "items", map[string]any{"maxItems": 20})
	field("ReleaseWorkerUploadAbandonRequest", "expected_worker_version", map[string]any{"minimum": 1, "maximum": 2147483647})
	field("ReleaseWorkerUploadAbandonRequest", "expected_manifest_sha256", map[string]any{"pattern": "^[a-f0-9]{64}$"})
	field("ReleaseWorkerUploadAbandonRequest", "acknowledge_incomplete", map[string]any{"const": true})
	field("ReleaseWorkerUploadAbandonResult", "state", map[string]any{"const": "abandoned"})
	field("ReleaseWorkerCandidateRecoveryItem", "disposition", map[string]any{"enum": []string{"eligible", "awaiting_attempt", "unverified"}})
	field("ReleaseWorkerCandidateRecoveryItem", "fence", map[string]any{"pattern": "^[1-9][0-9]{0,18}$", "description": "Exact original fencing number rendered as a decimal string."})
	field("ReleaseWorkerCandidateRecoveryView", "items", map[string]any{"maxItems": 20})
	field("ReleaseWorkerCandidateAbandonRequest", "expected_worker_version", map[string]any{"minimum": 1, "maximum": 2147483647})
	field("ReleaseWorkerCandidateAbandonRequest", "expected_bundle_sha256", map[string]any{"pattern": "^[a-f0-9]{64}$"})
	field("ReleaseWorkerCandidateAbandonRequest", "acknowledge_incomplete", map[string]any{"const": true})
	field("ReleaseWorkerCandidateAbandonResult", "state", map[string]any{"const": "abandoned"})
	field("ArtifactUploadState", "state", map[string]any{"enum": []string{"receiving", "sealed", "abandoned"}})
	field("ReleaseWorkerRegisterRequest", "profiles", map[string]any{"maxItems": 32, "description": "Unique administrator-owned profile IDs and real configuration digests. An empty list supports a general execution node without release capability."})
	field("ReleaseProfileRegistration", "kind", map[string]any{"enum": []string{"build", "package_test"}})
	field("ReleaseCandidateCreateRequest", "tasks", map[string]any{"minItems": 1, "maxItems": 100, "description": "Unique current accepted tasks actually delivered by the same run as both selected proofs; versions and real review basis are validated atomically."})
	field("ReleaseCandidateTaskInput", "expected_version", map[string]any{"minimum": 1})
	field("ReleaseCandidateCreateRequest", "name", map[string]any{"minLength": 1, "maxLength": 200, "x-max-utf8-bytes": 200})
	field("ReleaseEnvironment", "generation", map[string]any{"minimum": 0, "description": "Highest reserved fencing generation. May exceed active_generation after failed/unknown attempts."})
	field("ReleaseEnvironment", "active_generation", map[string]any{"minimum": 0, "description": "Last observed active pointer; not the CAS basis for another deployment."})
	field("ReleaseProviderOption", "kind", map[string]any{"const": "local_static_site_v1"})
	field("ReleaseProviderOption", "capability_state", map[string]any{"const": "configured"})
	for _, model := range []string{"ReleasePlanInput", "ReleasePlanContent"} {
		field(model, "action", map[string]any{"enum": []string{"deploy", "rollback"}})
	}
	field("ReleasePlanInput", "expected_environment_generation", map[string]any{"minimum": 0})
	field("ReleasePlanDecisionRequest", "decision", map[string]any{"enum": []string{"approved", "rejected", "revoked"}})
	field("ReleasePlanDecisionRequest", "reason", map[string]any{"minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 2000})
	field("ReleasePlan", "state", map[string]any{"enum": []string{"pending_approval", "approved", "rejected", "revoked", "executing", "completed"}, "description": "completed means execution ended, not necessarily deployed or healthy; read ReleaseDeployment effect/health separately."})
	field("ReleasePlan", "version", map[string]any{"minimum": 1, "description": "Mutable decision CAS version, independent of immutable revision and sha256."})
	field("ReleasePlan", "revision", map[string]any{"minimum": 1, "description": "Immutable content revision; v1 always 1 and content edits create a new plan."})
	field("ReleaseDeployment", "effect_state", map[string]any{"enum": []string{"not_started", "applied", "unknown"}})
	field("ReleaseDeployment", "health_state", map[string]any{"enum": []string{"not_checked", "passed", "failed", "unknown"}})
	field("HealthPolicy", "checks", map[string]any{"minItems": 1, "maxItems": 8})
	field("HealthPolicy", "timeout_ms", map[string]any{"minimum": 100, "maximum": 10000})
	field("HealthPolicy", "attempts", map[string]any{"minimum": 1, "maximum": 5})
	field("HealthCheck", "expected_status", map[string]any{"const": 200})
	field("HealthCheck", "expected_sha256", map[string]any{"description": "Frozen from actual manifest in plan; must be absent/empty in environment configuration requests."})
	field("ArtifactManifest", "schema_version", map[string]any{"const": 1})
	field("ArtifactManifest", "kind", map[string]any{"const": rc.StaticSiteKind})
	field("ArtifactManifest", "files", map[string]any{"minItems": 1, "maxItems": rc.MaxFiles, "description": "Canonical portable paths, no duplicate case variants or file/directory collisions; total <=64 MiB."})
	field("ArtifactFile", "size_bytes", map[string]any{"minimum": 0, "maximum": rc.MaxFileBytes})
	field("ReleaseAuthorization", "authorization_ticket", map[string]any{"readOnly": true, "description": "Short-lived opaque secret delivered only to current trusted assignment via this dedicated response. Never include in Assignment, Config, journal, browser storage or evidence."})
	for _, model := range []string{"DeploymentRequest", "ConsumeDeploymentAuthorization"} {
		field(model, "authorization_ticket", map[string]any{"writeOnly": true, "description": "Body-only short-lived opaque secret, memory transport only; not graph authority."})
	}
}
