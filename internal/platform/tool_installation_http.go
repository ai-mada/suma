package platform

import (
	"io"
	"net/http"
	"strconv"

	"github.com/ai-mada/suma/internal/strictjson"
	tii "github.com/ai-mada/suma/internal/toolinstallationcontract"
)

// New optional declarations must be omitted or complete; explicit null must
// not silently turn a current scan capability into an older default value.
func installationMaintenanceRaw(value any) bool {
	if !installationRawObject(value, []string{"protocol", "instance_id"}, "installed_discovery", "managed_preparation") {
		return false
	}
	row := value.(map[string]any)
	if capability, present := row["installed_discovery"]; present {
		if !installationRawObject(capability, []string{"protocol", "adapter_ids"}) {
			return false
		}
	}
	if capability, present := row["managed_preparation"]; present {
		if !installationRawObject(capability, []string{"protocol", "adapter_ids", "generation_sha256"}, "codex_template_ids") {
			return false
		}
		if value, present := capability.(map[string]any)["codex_template_ids"]; present {
			ids, ok := value.([]any)
			return ok && len(ids) > 0
		}
	}
	return true
}

func installationRawObject(value any, required []string, optional ...string) bool {
	row, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, name := range required {
		if value, present := row[name]; !present || value == nil {
			return false
		}
	}
	for name, value := range row {
		allowed := false
		for _, key := range required {
			if key == name {
				allowed = true
			}
		}
		for _, key := range optional {
			if key == name {
				allowed = true
			}
		}
		if !allowed || value == nil {
			return false
		}
	}
	return true
}
func installationRawResult(m map[string]any, pool bool) bool {
	required := []string{"project_id", "worker_id", "scan", "idempotency_key", "outcome", "items", "failure_code"}
	if pool {
		required = []string{"tenant_id", "project_id", "runner_id", "scan", "idempotency_key", "outcome", "items", "failure_code"}
	}
	if !installationRawObject(m, required, "stop_proof") || !installationRawObject(m["scan"], []string{"scan_id", "claim_id", "fence"}) {
		return false
	}
	items, ok := m["items"].([]any)
	if !ok || len(items) > 9 {
		return false
	}
	for _, item := range items {
		if !installationRawObject(item, []string{"adapter_id", "protocol", "installed", "launch_supported", "reason_code"}, "version", "executable_sha256", "diagnostic") {
			return false
		}
		if diagnostic, present := item.(map[string]any)["diagnostic"]; present && !installationRawObject(diagnostic, []string{"stage", "code", "next_action"}) {
			return false
		}
	}
	if value, present := m["stop_proof"]; present {
		if !installationRawObject(value, []string{"protocol", "scan", "source_sha256", "receipt_sha256", "probe_returned", "all_native_trees_stopped", "native_process_count", "stopped_process_count", "observed_at"}) {
			return false
		}
		proof := value.(map[string]any)
		if !installationRawObject(proof["scan"], []string{"scan_id", "claim_id", "fence"}) {
			return false
		}
	}
	return true
}
func (s *Server) toolInstallationScansHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope string, parts []string, project string, m map[string]any) bool {
	if len(parts) < 1 || parts[0] != "tool-installation-scans" || len(parts) > 2 {
		return false
	}
	if scope != "" {
		writeError(w, forbidden())
		return true
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodPost:
		var req tii.CreateRequest
		if !installationRawObject(m, []string{"project_id", "worker_id", "adapter_ids", "idempotency_key"}) || strictAutomation(m, &req) != nil {
			writeError(w, toolInstallationInvalid())
			return true
		}
		out, err := s.Service.CreateToolInstallationScan(r.Context(), actor, project, req)
		respond(w, 201, out, err)
	case len(parts) == 1 && r.Method == http.MethodGet:
		query := r.URL.Query()
		q := ToolInstallationScanListQuery{WorkerID: query.Get("worker_id"), Cursor: query.Get("cursor")}
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
		out, err := s.Service.ListToolInstallationScans(r.Context(), actor, project, q)
		respond(w, 200, out, err)
	case len(parts) == 2 && r.Method == http.MethodGet:
		out, err := s.Service.GetToolInstallationScan(r.Context(), actor, project, parts[1])
		if err == nil {
			for name, values := range r.URL.Query() {
				if name != "project_id" || len(values) != 1 {
					err = toolInstallationInvalid()
				}
			}
		}
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
func (s *Server) runnerToolInstallationScansHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, m map[string]any) bool {
	if path != "runner/tool-installation-scans/claim" && path != "runner/tool-installation-scans/results" {
		return false
	}
	if r.Method != http.MethodPost {
		writeError(w, &APIError{405, "method_not_allowed", "工具安装检测更新必须使用POST", nil})
		return true
	}
	if path == "runner/tool-installation-scans/claim" {
		var req tii.ClaimRequest
		if !installationRawObject(m, []string{"project_id", "worker_id", "claim_id"}) || strictAutomation(m, &req) != nil {
			writeError(w, toolInstallationInvalid())
			return true
		}
		out, err := s.Service.ClaimToolInstallationScan(r.Context(), actor, project, req)
		respond(w, 200, out, err)
	} else {
		var req tii.ResultRequest
		if !installationRawResult(m, false) || strictAutomation(m, &req) != nil {
			writeError(w, toolInstallationInvalid())
			return true
		}
		out, err := s.Service.CompleteToolInstallationScan(r.Context(), actor, project, req)
		respond(w, 200, out, err)
	}
	return true
}
func (s *Server) poolToolInstallationScansHTTP(w http.ResponseWriter, r *http.Request, token, path string) bool {
	if path != "pool-runner/tool-installation-scans/claim" && path != "pool-runner/tool-installation-scans/results" {
		return false
	}
	if r.Method != http.MethodPost {
		writeError(w, &APIError{405, "method_not_allowed", "工具安装检测更新必须使用POST", nil})
		return true
	}
	var m map[string]any
	raw, err := io.ReadAll(io.LimitReader(r.Body, (128<<10)+1))
	if err != nil || r.URL.RawQuery != "" || strictjson.Decode(raw, &m, 128<<10) != nil {
		writeError(w, toolInstallationInvalid())
		return true
	}
	if path == "pool-runner/tool-installation-scans/claim" {
		var req tii.PoolClaimRequest
		if !installationRawObject(m, []string{"tenant_id", "project_id", "runner_id", "claim_id"}) || strictAutomation(m, &req) != nil {
			writeError(w, toolInstallationInvalid())
			return true
		}
		out, err := s.Service.ClaimPoolToolInstallationScan(r.Context(), token, req)
		respond(w, 200, out, err)
	} else {
		var req tii.PoolResultRequest
		if !installationRawResult(m, true) || strictAutomation(m, &req) != nil {
			writeError(w, toolInstallationInvalid())
			return true
		}
		out, err := s.Service.CompletePoolToolInstallationScan(r.Context(), token, req)
		respond(w, 200, out, err)
	}
	return true
}
