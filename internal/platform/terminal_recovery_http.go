package platform

import (
	"net/http"
	"net/url"
)

// Recovery has its own narrow route. It never accepts a generic scheduler
// outcome, machine credential, replacement worker, or shell input.
func (s *Server) workspaceTerminalRecoveryHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	if len(parts) != 4 || parts[0] != "workspace" || parts[1] != "terminal-sessions" || parts[3] != "recovery" && parts[3] != "recover" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if !taskConversationActor(actor) || scope != "" && scope != "api" || !handoffID(parts[2], false) {
		writeError(w, forbidden())
		return true
	}
	if parts[3] == "recovery" && r.Method == http.MethodGet {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(r.URL.RawQuery) > 256 || len(query) != 1 || len(query["project_id"]) != 1 || !handoffID(query.Get("project_id"), false) {
			writeError(w, invalid(workspaceTerminalInvalid))
			return true
		}
		project := query.Get("project_id")
		if scope == "api" && (boundProject == "" || project != boundProject) {
			writeError(w, forbidden())
			return true
		}
		out, err := s.Service.GetWorkspaceTerminalRecovery(r.Context(), actor, project, parts[2])
		respond(w, http.StatusOK, out, err)
		return true
	}
	if parts[3] != "recover" || r.Method != http.MethodPost || r.URL.RawQuery != "" {
		writeError(w, missing())
		return true
	}
	var req WorkspaceTerminalRecoveryRequest
	if err := decodeWorkspaceTerminalHTTP(r, &req); err != nil {
		writeError(w, err)
		return true
	}
	if scope == "api" && (boundProject == "" || req.ProjectID != boundProject) {
		writeError(w, forbidden())
		return true
	}
	out, err := s.Service.RecoverWorkspaceTerminal(r.Context(), s.Engine, actor, req.ProjectID, parts[2], req)
	respond(w, http.StatusOK, out, err)
	return true
}
