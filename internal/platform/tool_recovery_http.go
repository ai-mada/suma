package platform

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

// Zero owned process trees is a possible fact. Missing counters are not that
// fact, so preserve JSON field presence before converting into Go zero values.
func completeToolStopProofObject(value any) bool {
	proof, ok := value.(map[string]any)
	if !ok || len(proof) != 10 {
		return false
	}
	for _, key := range []string{"protocol", "inspection", "source_sha256", "receipt_sha256", "probe_returned", "all_native_trees_stopped", "no_pending_inventory", "native_process_count", "stopped_process_count", "observed_at"} {
		if value, present := proof[key]; !present || value == nil {
			return false
		}
	}
	ref, ok := proof["inspection"].(map[string]any)
	if !ok || len(ref) != 3 {
		return false
	}
	for _, key := range []string{"inspection_id", "claim_id", "fence"} {
		if value, present := ref[key]; !present || value == nil {
			return false
		}
	}
	return true
}
func decodeToolRecoveryHTTP(r *http.Request, out any, fields ...string) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return toolInspectionInvalid()
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) != nil {
		return toolInspectionInvalid()
	}
	if proof, present := object["proof"]; present && !completeToolStopProofObject(proof) {
		return toolInspectionInvalid()
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	return toolPermissionDecodeHTTP(r, out, fields...)
}

func (s *Server) toolRecoveriesHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope string, parts []string, project string, m map[string]any) bool {
	list := len(parts) == 3 && parts[0] == "tool-inspections" && parts[2] == "recoveries"
	single := len(parts) == 2 && parts[0] == "tool-inspection-recoveries"
	if !list && !single {
		return false
	}
	if scope != "" {
		writeError(w, forbidden())
		return true
	}
	switch {
	case list && r.Method == http.MethodPost:
		var req tic.ToolInspectionRecoveryCreateRequest
		if strictAutomation(m, &req) != nil {
			writeError(w, toolInspectionInvalid())
			return true
		}
		out, err := s.Service.CreateToolInspectionRecovery(r.Context(), actor, project, parts[1], req)
		respond(w, 201, out, err)
	case list && r.Method == http.MethodGet:
		out, err := s.Service.ListToolInspectionRecoveries(r.Context(), actor, project, parts[1])
		respond(w, 200, out, err)
	case single && r.Method == http.MethodGet:
		out, err := s.Service.GetToolInspectionRecovery(r.Context(), actor, project, parts[1])
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
func (s *Server) runnerToolRecoveriesHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, m map[string]any) bool {
	switch path {
	case "runner/tool-inspections/recovery":
		var req tic.ToolInspectionRecoveryPollRequest
		if strictAutomation(m, &req) != nil {
			writeError(w, toolInspectionInvalid())
			return true
		}
		out, err := s.Service.PollToolInspectionRecovery(r.Context(), actor, project, req)
		respond(w, 200, out, err)
	case "runner/tool-inspections/recovery/proofs":
		var req tic.ToolInspectionRecoveryProofRequest
		if !completeToolStopProofObject(m["proof"]) {
			writeError(w, toolInspectionInvalid())
			return true
		}
		if strictAutomation(m, &req) != nil {
			writeError(w, toolInspectionInvalid())
			return true
		}
		out, err := s.Service.StoreToolInspectionRecoveryProof(r.Context(), actor, project, req)
		respond(w, 200, out, err)
	case "runner/tool-inspections/recovery/acknowledgements":
		var req tic.ToolInspectionRecoveryAcknowledgementRequest
		if strictAutomation(m, &req) != nil {
			writeError(w, toolInspectionInvalid())
			return true
		}
		out, err := s.Service.AcknowledgeToolInspectionRecovery(r.Context(), actor, project, req)
		respond(w, 200, out, err)
	default:
		return false
	}
	return true
}
func (s *Server) poolToolRecoveriesHTTP(w http.ResponseWriter, r *http.Request, token, path string) bool {
	if path != "pool-runner/tool-inspections/recovery" && path != "pool-runner/tool-inspections/recovery/proofs" && path != "pool-runner/tool-inspections/recovery/acknowledgements" {
		return false
	}
	if r.Method != http.MethodPost {
		writeError(w, &APIError{405, "method_not_allowed", "资源池机器更新必须使用 POST", nil})
		return true
	}
	switch path {
	case "pool-runner/tool-inspections/recovery":
		var req tic.PoolToolInspectionRecoveryPollRequest
		if decodeToolRecoveryHTTP(r, &req, "tenant_id", "project_id", "runner_id", "inspection") != nil {
			writeError(w, toolInspectionInvalid())
			return true
		}
		out, err := s.Service.PollPoolToolInspectionRecovery(r.Context(), token, req)
		respond(w, 200, out, err)
	case "pool-runner/tool-inspections/recovery/proofs":
		var req tic.PoolToolInspectionRecoveryProofRequest
		if decodeToolRecoveryHTTP(r, &req, "tenant_id", "project_id", "runner_id", "recovery_id", "expected_version", "idempotency_key", "proof") != nil {
			writeError(w, toolInspectionInvalid())
			return true
		}
		out, err := s.Service.StorePoolToolInspectionRecoveryProof(r.Context(), token, req)
		respond(w, 200, out, err)
	case "pool-runner/tool-inspections/recovery/acknowledgements":
		var req tic.PoolToolInspectionRecoveryAcknowledgementRequest
		if decodeToolRecoveryHTTP(r, &req, "tenant_id", "project_id", "runner_id", "recovery_id", "expected_version", "idempotency_key", "proof_sha256") != nil {
			writeError(w, toolInspectionInvalid())
			return true
		}
		out, err := s.Service.AcknowledgePoolToolInspectionRecovery(r.Context(), token, req)
		respond(w, 200, out, err)
	}
	return true
}
