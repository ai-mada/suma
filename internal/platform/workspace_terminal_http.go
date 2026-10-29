package platform

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	term "github.com/ai-mada/suma/internal/terminalcontract"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

func decodeWorkspaceTerminalHTTP(r *http.Request, out any) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
	if err != nil || woc.Decode(raw, out) != nil {
		return invalid(workspaceTerminalInvalid)
	}
	return nil
}

func (s *Server) workspaceTerminalHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject, path string, parts []string) bool {
	public := len(parts) >= 2 && parts[0] == "workspace" && parts[1] == "terminal-sessions"
	machine := len(parts) >= 2 && parts[0] == "runner" && parts[1] == "terminal-sessions"
	if !public && !machine {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if machine {
		if scope != "runner" || r.Method != http.MethodPost || r.URL.RawQuery != "" || len(parts) != 4 || parts[3] != "exchange" || !handoffID(parts[2], false) {
			writeError(w, forbidden())
			return true
		}
		var req term.ExchangeRequest
		if err := decodeWorkspaceTerminalHTTP(r, &req); err != nil {
			writeError(w, err)
			return true
		}
		if req.SessionID != parts[2] {
			writeError(w, forbidden())
			return true
		}
		out, err := s.Service.ExchangeWorkspaceTerminal(r.Context(), actor, boundProject, req)
		respond(w, 200, out, err)
		return true
	}
	if !taskConversationActor(actor) || scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	bad := func() { writeError(w, invalid(workspaceTerminalInvalid)) }
	if r.Method == http.MethodPost {
		if r.URL.RawQuery != "" || len(parts) != 2 && (len(parts) != 4 || parts[3] != "commands" || !handoffID(parts[2], false)) {
			bad()
			return true
		}
		if len(parts) == 2 {
			var req WorkspaceTerminalCreateRequest
			if err := decodeWorkspaceTerminalHTTP(r, &req); err != nil {
				writeError(w, err)
				return true
			}
			if scope == "api" && (boundProject == "" || req.ProjectID != boundProject) {
				writeError(w, forbidden())
				return true
			}
			out, err := s.Service.CreateWorkspaceTerminal(r.Context(), s.Engine, actor, req.ProjectID, req)
			respond(w, 201, out, err)
			return true
		}
		var req WorkspaceTerminalCommandRequest
		if err := decodeWorkspaceTerminalHTTP(r, &req); err != nil {
			writeError(w, err)
			return true
		}
		if scope == "api" && (boundProject == "" || req.ProjectID != boundProject) {
			writeError(w, forbidden())
			return true
		}
		out, err := s.Service.CommandWorkspaceTerminal(r.Context(), actor, req.ProjectID, parts[2], req)
		respond(w, 200, out, err)
		return true
	}
	if r.Method != http.MethodGet || len(parts) < 2 || len(parts) > 4 || len(parts) >= 3 && !handoffID(parts[2], false) || len(parts) == 4 && parts[3] != "output" {
		writeError(w, missing())
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 4096 {
		bad()
		return true
	}
	for key, values := range query {
		if len(values) != 1 || values[0] == "" || key != "project_id" && !(len(parts) == 2 && key == "task_id") && !(len(parts) == 4 && key == "after") {
			bad()
			return true
		}
	}
	project := query.Get("project_id")
	if !handoffID(project, false) || len(parts) == 2 && !handoffID(query.Get("task_id"), false) {
		bad()
		return true
	}
	if scope == "api" && (boundProject == "" || project != boundProject) {
		writeError(w, forbidden())
		return true
	}
	if len(parts) == 2 {
		out, err := s.Service.ListWorkspaceTerminals(r.Context(), actor, project, query.Get("task_id"))
		respond(w, 200, out, err)
		return true
	}
	if len(parts) == 3 {
		out, err := s.Service.GetWorkspaceTerminal(r.Context(), actor, project, parts[2])
		respond(w, 200, out, err)
		return true
	}
	after := int64(0)
	if query.Has("after") {
		after, err = strconv.ParseInt(query.Get("after"), 10, 64)
		if err != nil || strconv.FormatInt(after, 10) != query.Get("after") {
			bad()
			return true
		}
	}
	out, err := s.Service.WorkspaceTerminalOutputs(r.Context(), actor, project, parts[2], after)
	respond(w, 200, out, err)
	return true
}

func (s *Server) poolWorkspaceTerminalHTTP(w http.ResponseWriter, r *http.Request, token, path string) bool {
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] != "pool-runner" || parts[1] != "terminal-sessions" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost || r.URL.RawQuery != "" || len(parts) != 4 || parts[3] != "exchange" || !handoffID(parts[2], false) {
		writeError(w, forbidden())
		return true
	}
	var req PoolWorkspaceTerminalExchangeRequest
	if err := decodeWorkspaceTerminalHTTP(r, &req); err != nil {
		writeError(w, err)
		return true
	}
	if req.SessionID != parts[2] {
		writeError(w, forbidden())
		return true
	}
	out, err := s.Service.ExchangePoolWorkspaceTerminal(r.Context(), token, req)
	respond(w, 200, out, err)
	return true
}
