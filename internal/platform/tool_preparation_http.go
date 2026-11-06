package platform

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ai-mada/suma/internal/strictjson"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

func preparationDecode(m map[string]any, out any) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return toolPreparationInvalid()
	}
	return strictjson.Decode(raw, out, 128<<10)
}

// Raw presence is checked before Go decoding: explicit false/zero proofs are
// facts, whereas missing fields and null optional objects are not evidence.
func preparationRawProof(value any) bool {
	if !installationRawObject(value, []string{"protocol", "preparation", "source_sha256", "receipt_sha256", "probe_returned", "all_native_trees_stopped", "native_process_count", "stopped_process_count", "observed_at"}) {
		return false
	}
	return installationRawObject(value.(map[string]any)["preparation"], []string{"preparation_id", "claim_id", "fence"})
}

func preparationRawNonNull(value any) bool {
	if value == nil {
		return false
	}
	switch row := value.(type) {
	case map[string]any:
		for _, child := range row {
			if !preparationRawNonNull(child) {
				return false
			}
		}
	case []any:
		for _, child := range row {
			if !preparationRawNonNull(child) {
				return false
			}
		}
	}
	return true
}

func preparationRawResult(m map[string]any, pool bool) bool {
	required := []string{"project_id", "worker_id", "preparation", "source_sha256", "idempotency_key", "outcome", "failure_code"}
	if pool {
		required = []string{"tenant_id", "project_id", "runner_id", "preparation", "source_sha256", "idempotency_key", "outcome", "failure_code"}
	}
	if !installationRawObject(m, required, "receipt", "stop_proof") || !installationRawObject(m["preparation"], []string{"preparation_id", "claim_id", "fence"}) {
		return false
	}
	if proof, present := m["stop_proof"]; present && !preparationRawProof(proof) {
		return false
	}
	if receipt, present := m["receipt"]; present {
		if !installationRawObject(receipt, []string{"parent_generation_sha256", "generation_sha256", "command_id", "artifact_sha256", "local_receipt_sha256", "profile", "stop_proof"}) || !preparationRawNonNull(receipt) {
			return false
		}
		if !preparationRawProof(receipt.(map[string]any)["stop_proof"]) {
			return false
		}
	}
	return true
}

func validateToolPreparationInventoryFields(m map[string]any) bool {
	// encoding/json accepts case aliases. Only the new preparation-bearing
	// branch uses this closed contract; old inventory reports retain their
	// existing decoder and do not acquire a new wire mode implicitly.
	for key := range m {
		if strings.EqualFold(key, "preparation") && key != "preparation" {
			return false
		}
	}
	_, present := m["preparation"]
	if !present {
		return true
	}
	if _, inspection := m["inspection"]; inspection {
		return false
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return false
	}
	// Decode the complete original envelope, including observation booleans
	// and full nested profiles. Number lexemes are preserved by both callers.
	if _, pool := m["runner_id"]; pool {
		var request PoolToolInventoryRequest
		return strictjson.Decode(raw, &request, 2<<20) == nil && request.Preparation != nil && tp.ValidateInventoryBinding(*request.Preparation) == nil
	}
	var request ToolInventoryRequest
	return strictjson.Decode(raw, &request, 2<<20) == nil && request.Preparation != nil && tp.ValidateInventoryBinding(*request.Preparation) == nil
}

func preparationRawConfiguration(value any) bool {
	if !installationRawObject(value, []string{"model", "credential_env_ref"}, "template_id") {
		return false
	}
	if selected, present := value.(map[string]any)["template_id"]; present {
		id, ok := selected.(string)
		return ok && id != ""
	}
	return true
}

func (s *Server) toolPreparationsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope string, parts []string, project string, m map[string]any) bool {
	if len(parts) < 1 || parts[0] != "tool-preparations" || len(parts) > 2 {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if scope != "" {
		writeError(w, forbidden())
		return true
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodPost:
		var request tp.CreateRequest
		if r.URL.RawQuery != "" || !installationRawObject(m, []string{"project_id", "worker_id", "installation_scan_id", "expected_scan_version", "adapter_id", "configuration", "expected_generation_sha256", "acknowledge_configuration_write", "idempotency_key"}) || !preparationRawConfiguration(m["configuration"]) || preparationDecode(m, &request) != nil {
			writeError(w, toolPreparationInvalid())
			return true
		}
		out, err := s.Service.CreateToolPreparation(r.Context(), actor, project, request)
		respond(w, 201, out, err)
	case len(parts) == 1 && r.Method == http.MethodGet:
		query := r.URL.Query()
		q := ToolPreparationListQuery{WorkerID: query.Get("worker_id"), Cursor: query.Get("cursor")}
		for name, values := range query {
			if len(values) != 1 || (name != "project_id" && name != "worker_id" && name != "limit" && name != "cursor") {
				q.Limit = -1
			}
		}
		if values, present := query["limit"]; present && q.Limit != -1 {
			n, err := strconv.Atoi(values[0])
			if err != nil || n < 1 {
				q.Limit = -1
			} else {
				q.Limit = n
			}
		}
		out, err := s.Service.ListToolPreparations(r.Context(), actor, project, q)
		respond(w, 200, out, err)
	case len(parts) == 2 && r.Method == http.MethodGet:
		out, err := s.Service.GetToolPreparation(r.Context(), actor, project, parts[1])
		if err == nil {
			for name, values := range r.URL.Query() {
				if name != "project_id" || len(values) != 1 {
					err = toolPreparationInvalid()
				}
			}
		}
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}

func (s *Server) runnerToolPreparationsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, m map[string]any) bool {
	if path != "runner/tool-preparations/claim" && path != "runner/tool-preparations/results" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeError(w, &APIError{405, "method_not_allowed", "Managed tool preparation updates require POST", nil})
		return true
	}
	if r.URL.RawQuery != "" {
		writeError(w, toolPreparationInvalid())
		return true
	}
	if path == "runner/tool-preparations/claim" {
		var request tp.ClaimRequest
		if !installationRawObject(m, []string{"project_id", "worker_id", "claim_id"}) || preparationDecode(m, &request) != nil {
			writeError(w, toolPreparationInvalid())
			return true
		}
		out, err := s.Service.ClaimToolPreparation(r.Context(), actor, project, request)
		respond(w, 200, out, err)
	} else {
		var request tp.ResultRequest
		if !preparationRawResult(m, false) || preparationDecode(m, &request) != nil {
			writeError(w, toolPreparationInvalid())
			return true
		}
		out, err := s.Service.CompleteToolPreparation(r.Context(), actor, project, request)
		respond(w, 200, out, err)
	}
	return true
}

func (s *Server) poolToolPreparationsHTTP(w http.ResponseWriter, r *http.Request, token, path string) bool {
	if path != "pool-runner/tool-preparations/claim" && path != "pool-runner/tool-preparations/results" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeError(w, &APIError{405, "method_not_allowed", "Managed tool preparation updates require POST", nil})
		return true
	}
	var m map[string]any
	raw, err := io.ReadAll(io.LimitReader(r.Body, (128<<10)+1))
	if err != nil || r.URL.RawQuery != "" || strictjson.Decode(raw, &m, 128<<10) != nil {
		writeError(w, toolPreparationInvalid())
		return true
	}
	if path == "pool-runner/tool-preparations/claim" {
		var request tp.PoolClaimRequest
		if !installationRawObject(m, []string{"tenant_id", "project_id", "runner_id", "claim_id"}) || strictjson.Decode(raw, &request, 128<<10) != nil {
			writeError(w, toolPreparationInvalid())
			return true
		}
		out, err := s.Service.ClaimPoolToolPreparation(r.Context(), token, request)
		respond(w, 200, out, err)
	} else {
		var request tp.PoolResultRequest
		if !preparationRawResult(m, true) || strictjson.Decode(raw, &request, 128<<10) != nil {
			writeError(w, toolPreparationInvalid())
			return true
		}
		out, err := s.Service.CompletePoolToolPreparation(r.Context(), token, request)
		respond(w, 200, out, err)
	}
	return true
}
