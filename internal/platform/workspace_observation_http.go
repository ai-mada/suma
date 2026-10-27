package platform

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

func decodeWorkspaceObservationHTTP(r *http.Request, out any) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
	if err != nil || woc.Decode(raw, out) != nil {
		return invalid(workspaceObservationInvalid)
	}
	return nil
}
func workspaceObservationQuery(r *http.Request, detail bool) (url.Values, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 4096 {
		return nil, invalid(workspaceObservationInvalid)
	}
	for key, values := range q {
		if (key != "project_id" && !(detail && key == "metadata")) || len(values) != 1 || values[0] == "" {
			return nil, invalid(workspaceObservationInvalid)
		}
	}
	if detail && q.Get("metadata") != "" && q.Get("metadata") != "true" {
		return nil, invalid(workspaceObservationInvalid)
	}
	return q, nil
}

// This route is entered after authentication and the original browser-view
// guard, but before the generic map decoder can lose strict numeric lexemes.
func (s *Server) workspaceObservationHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject, path string, parts []string) bool {
	public := path == "workspace/targets" || path == "workspace/observations" || (len(parts) == 3 && parts[0] == "workspace" && parts[1] == "observations")
	machine := path == "runner/workspace-observations/claim" || path == "runner/workspace-observations/result"
	if !public && !machine {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if machine {
		if scope != "runner" || r.Method != http.MethodPost || r.URL.RawQuery != "" {
			writeError(w, forbidden())
			return true
		}
		if strings.HasSuffix(path, "/claim") {
			var req woc.ClaimRequest
			if err := decodeWorkspaceObservationHTTP(r, &req); err != nil {
				writeError(w, err)
				return true
			}
			if req.ProjectID != boundProject {
				writeError(w, forbidden())
				return true
			}
			lease, err := s.Service.ClaimWorkspaceObservation(r.Context(), actor, boundProject, req)
			respond(w, 200, map[string]any{"lease": lease}, err)
		} else {
			var req woc.ResultRequest
			if err := decodeWorkspaceObservationHTTP(r, &req); err != nil {
				writeError(w, err)
				return true
			}
			if req.ProjectID != boundProject {
				writeError(w, forbidden())
				return true
			}
			out, err := s.Service.CompleteWorkspaceObservation(r.Context(), actor, boundProject, req)
			respond(w, 200, out, err)
		}
		return true
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	detail := len(parts) == 3
	q, err := workspaceObservationQuery(r, detail)
	if err != nil {
		writeError(w, err)
		return true
	}
	project := q.Get("project_id")
	if path == "workspace/observations" && r.Method == http.MethodPost {
		var req woc.CreateRequest
		if err = decodeWorkspaceObservationHTTP(r, &req); err != nil {
			writeError(w, err)
			return true
		}
		if project != "" && project != req.ProjectID || scope == "api" && req.ProjectID != boundProject {
			writeError(w, forbidden())
			return true
		}
		out, e := s.Service.CreateWorkspaceObservation(r.Context(), actor, req.ProjectID, req)
		respond(w, 201, out, e)
		return true
	}
	if r.Method != http.MethodGet {
		writeError(w, missing())
		return true
	}
	if project == "" || scope == "api" && project != boundProject {
		writeError(w, forbidden())
		return true
	}
	if path == "workspace/targets" {
		out, e := s.Service.GetWorkspaceObservationTargets(r.Context(), actor, project)
		respond(w, 200, out, e)
		return true
	}
	if detail {
		out, e := s.Service.GetWorkspaceObservation(r.Context(), actor, project, parts[2])
		// Metadata polling repeats the same current authorization and scope checks
		// without repeatedly transmitting source text already displayed by a view.
		if q.Get("metadata") == "true" {
			out.Result = nil
		}
		respond(w, 200, out, e)
		return true
	}
	writeError(w, missing())
	return true
}

func (s *Server) poolWorkspaceObservationHTTP(w http.ResponseWriter, r *http.Request, token, path string) bool {
	if path != "pool-runner/workspace-observations/claim" && path != "pool-runner/workspace-observations/result" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		writeError(w, forbidden())
		return true
	}
	if strings.HasSuffix(path, "/claim") {
		var req PoolWorkspaceObservationClaimRequest
		if err := decodeWorkspaceObservationHTTP(r, &req); err != nil {
			writeError(w, err)
			return true
		}
		lease, err := s.Service.ClaimPoolWorkspaceObservation(r.Context(), token, req)
		respond(w, 200, map[string]any{"lease": lease}, err)
	} else {
		var req PoolWorkspaceObservationResultRequest
		if err := decodeWorkspaceObservationHTTP(r, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CompletePoolWorkspaceObservation(r.Context(), token, req)
		respond(w, 200, out, err)
	}
	return true
}
