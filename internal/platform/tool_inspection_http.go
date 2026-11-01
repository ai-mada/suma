package platform

import (
	"net/http"

	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

func (s *Server) toolInspectionsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope string, parts []string, project string, m map[string]any) bool {
	single := len(parts) == 2 && parts[0] == "tool-inspections"
	profile := len(parts) == 3 && parts[0] == "tool-profiles" && parts[2] == "inspections"
	if !single && !profile {
		return false
	}
	if scope != "" {
		writeError(w, forbidden())
		return true
	}
	switch {
	case single && r.Method == http.MethodGet:
		out, err := s.Service.GetToolInspection(r.Context(), actor, project, parts[1])
		respond(w, 200, out, err)
	case profile && r.Method == http.MethodGet:
		out, err := s.Service.ListToolInspections(r.Context(), actor, project, parts[1])
		respond(w, 200, out, err)
	case profile && r.Method == http.MethodPost:
		var req tic.CreateRequest
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, toolInspectionInvalid())
			return true
		}
		out, err := s.Service.CreateToolInspection(r.Context(), actor, project, parts[1], req)
		respond(w, 201, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
func (s *Server) runnerToolInspectionsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, m map[string]any) bool {
	switch path {
	case "runner/tool-inspections/claim":
		var req tic.ClaimRequest
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, toolInspectionInvalid())
			return true
		}
		out, err := s.Service.ClaimToolInspection(r.Context(), actor, project, req)
		respond(w, 200, out, err)
		return true
	case "runner/tool-inspections/results":
		var req tic.FailureRequest
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, toolInspectionInvalid())
			return true
		}
		out, err := s.Service.FailToolInspection(r.Context(), actor, project, req)
		respond(w, 200, out, err)
		return true
	}
	return false
}
func (s *Server) poolToolInspectionsHTTP(w http.ResponseWriter, r *http.Request, token, path string) bool {
	if path != "pool-runner/tool-inspections/claim" && path != "pool-runner/tool-inspections/results" {
		return false
	}
	if r.Method != http.MethodPost {
		writeError(w, &APIError{405, "method_not_allowed", "资源池机器更新必须使用 POST", nil})
		return true
	}
	if path == "pool-runner/tool-inspections/claim" {
		var req tic.PoolToolInspectionClaimRequest
		if err := toolPermissionDecodeHTTP(r, &req, "tenant_id", "project_id", "runner_id", "claim_id"); err != nil {
			writeError(w, toolInspectionInvalid())
			return true
		}
		out, err := s.Service.ClaimPoolToolInspection(r.Context(), token, req)
		respond(w, 200, out, err)
	} else {
		var req tic.PoolToolInspectionFailureRequest
		if err := toolPermissionDecodeHTTP(r, &req, "tenant_id", "project_id", "runner_id", "inspection", "idempotency_key", "outcome", "failure_code"); err != nil {
			writeError(w, toolInspectionInvalid())
			return true
		}
		out, err := s.Service.FailPoolToolInspection(r.Context(), token, req)
		respond(w, 200, out, err)
	}
	return true
}
