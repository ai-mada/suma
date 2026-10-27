package platform

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	wwc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

func decodeWorkspaceWriteHTTP(r *http.Request, out any) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
	if err != nil || wwc.Decode(raw, out) != nil {
		return invalid("Invalid workspace save request")
	}
	return nil
}

func workspaceWriteQuery(r *http.Request) (url.Values, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 4096 {
		return nil, invalid("Invalid workspace save request")
	}
	for name, values := range q {
		if name != "project_id" || len(values) != 1 || values[0] == "" {
			return nil, invalid("Invalid workspace save request")
		}
	}
	return q, nil
}

// Entry is after authentication and original-view verification, before generic
// map decoding. The exact content, numeric tokens and request scope survive.
func (s *Server) workspaceWriteHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject, path string, parts []string) bool {
	public := path == "workspace/writes" || (len(parts) == 3 && parts[0] == "workspace" && parts[1] == "writes") || (len(parts) == 4 && parts[0] == "workspace" && parts[1] == "writes" && parts[3] == "inspect")
	machine := path == "runner/workspace-writes/claim" || path == "runner/workspace-writes/result"
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
			var req wwc.ClaimRequest
			if err := decodeWorkspaceWriteHTTP(r, &req); err != nil {
				writeError(w, err)
				return true
			}
			if req.ProjectID != boundProject {
				writeError(w, forbidden())
				return true
			}
			lease, err := s.Service.ClaimWorkspaceWrite(r.Context(), actor, boundProject, req)
			respond(w, 200, map[string]any{"lease": lease}, err)
		} else {
			var req wwc.ResultRequest
			if err := decodeWorkspaceWriteHTTP(r, &req); err != nil {
				writeError(w, err)
				return true
			}
			if req.ProjectID != boundProject {
				writeError(w, forbidden())
				return true
			}
			receipt, err := s.Service.CompleteWorkspaceWrite(r.Context(), actor, boundProject, req)
			respond(w, 200, receipt, err)
		}
		return true
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	q, err := workspaceWriteQuery(r)
	if err != nil {
		writeError(w, err)
		return true
	}
	project := q.Get("project_id")
	if path == "workspace/writes" && r.Method == http.MethodPost {
		var req wwc.CreateRequest
		if err = decodeWorkspaceWriteHTTP(r, &req); err != nil {
			writeError(w, err)
			return true
		}
		if project != "" && project != req.ProjectID || scope == "api" && req.ProjectID != boundProject {
			writeError(w, forbidden())
			return true
		}
		result, e := s.Service.CreateWorkspaceWrite(r.Context(), actor, req.ProjectID, req)
		respond(w, 201, result, e)
		return true
	}
	if len(parts) == 4 && r.Method == http.MethodPost {
		var req wwc.InspectionRequest
		if err = decodeWorkspaceWriteHTTP(r, &req); err != nil {
			writeError(w, err)
			return true
		}
		if project != "" && project != req.ProjectID || scope == "api" && req.ProjectID != boundProject {
			writeError(w, forbidden())
			return true
		}
		result, e := s.Service.RequestWorkspaceWriteInspection(r.Context(), actor, req.ProjectID, parts[2], req)
		respond(w, 200, result, e)
		return true
	}
	if path == "workspace/writes" && r.Method == http.MethodGet {
		if project == "" || scope == "api" && project != boundProject {
			writeError(w, forbidden())
			return true
		}
		result, e := s.Service.ListWorkspaceWrites(r.Context(), actor, project)
		respond(w, 200, result, e)
		return true
	}
	if len(parts) == 3 && r.Method == http.MethodGet {
		if project == "" || scope == "api" && project != boundProject {
			writeError(w, forbidden())
			return true
		}
		result, e := s.Service.GetWorkspaceWrite(r.Context(), actor, project, parts[2])
		respond(w, 200, result, e)
		return true
	}
	writeError(w, missing())
	return true
}

func (s *Server) poolWorkspaceWriteHTTP(w http.ResponseWriter, r *http.Request, token, path string) bool {
	if path != "pool-runner/workspace-writes/claim" && path != "pool-runner/workspace-writes/result" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		writeError(w, forbidden())
		return true
	}
	if strings.HasSuffix(path, "/claim") {
		var req PoolWorkspaceWriteClaimRequest
		if err := decodeWorkspaceWriteHTTP(r, &req); err != nil {
			writeError(w, err)
			return true
		}
		lease, err := s.Service.ClaimPoolWorkspaceWrite(r.Context(), token, req)
		respond(w, 200, map[string]any{"lease": lease}, err)
	} else {
		var req PoolWorkspaceWriteResultRequest
		if err := decodeWorkspaceWriteHTTP(r, &req); err != nil {
			writeError(w, err)
			return true
		}
		receipt, err := s.Service.CompletePoolWorkspaceWrite(r.Context(), token, req)
		respond(w, 200, receipt, err)
	}
	return true
}
