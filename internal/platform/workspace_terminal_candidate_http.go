package platform

import (
	"net/http"
	"net/url"
)

func (s *Server) workspaceTerminalCandidateHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject, path string, parts []string) bool {
	preview := len(parts) == 4 && parts[0] == "workspace" && parts[1] == "terminal-sessions" && parts[3] == "candidate-plans"
	plan := len(parts) >= 2 && parts[0] == "workspace" && parts[1] == "terminal-candidate-plans"
	if !preview && !plan {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if !taskConversationActor(actor) || scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	if len(parts) < 3 || !handoffID(parts[2], false) {
		writeError(w, invalid(workspaceTerminalInvalid))
		return true
	}
	if preview && r.Method == http.MethodGet {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(query) != 1 || len(query["project_id"]) != 1 || !handoffID(query.Get("project_id"), false) {
			writeError(w, invalid(workspaceTerminalInvalid))
			return true
		}
		project := query.Get("project_id")
		if scope == "api" && (boundProject == "" || project != boundProject) {
			writeError(w, forbidden())
			return true
		}
		out, err := s.Service.ListWorkspaceTerminalCandidatePlans(r.Context(), s.Engine, actor, project, parts[2])
		respond(w, 200, out, err)
		return true
	}
	if r.Method == http.MethodPost && r.URL.RawQuery == "" {
		if preview {
			var req WorkspaceTerminalCandidatePreviewRequest
			if err := decodeWorkspaceTerminalHTTP(r, &req); err != nil {
				writeError(w, err)
				return true
			}
			if scope == "api" && (boundProject == "" || req.ProjectID != boundProject) {
				writeError(w, forbidden())
				return true
			}
			out, err := s.Service.CreateWorkspaceTerminalCandidatePlan(r.Context(), s.Engine, actor, req.ProjectID, parts[2], req)
			respond(w, 201, out, err)
			return true
		}
		if len(parts) == 4 && parts[3] == "start" {
			var req TaskDispatchStartRequest
			if err := decodeWorkspaceTerminalHTTP(r, &req); err != nil {
				writeError(w, err)
				return true
			}
			if scope == "api" && (boundProject == "" || req.ProjectID != boundProject) {
				writeError(w, forbidden())
				return true
			}
			out, err := s.Service.StartWorkspaceTerminalCandidatePlan(r.Context(), s.Engine, actor, req.ProjectID, parts[2], req)
			respond(w, 201, out, err)
			return true
		}
	}
	if plan && len(parts) == 3 && r.Method == http.MethodGet {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(query) != 1 || len(query["project_id"]) != 1 || !handoffID(query.Get("project_id"), false) {
			writeError(w, invalid(workspaceTerminalInvalid))
			return true
		}
		project := query.Get("project_id")
		if scope == "api" && (boundProject == "" || project != boundProject) {
			writeError(w, forbidden())
			return true
		}
		out, err := s.Service.GetWorkspaceTerminalCandidatePlan(r.Context(), s.Engine, actor, project, parts[2])
		respond(w, 200, out, err)
		return true
	}
	writeError(w, invalid(workspaceTerminalInvalid))
	return true
}
