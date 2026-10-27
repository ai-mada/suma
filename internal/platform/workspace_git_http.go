package platform

import (
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Workspace source control shares the original project read authority. No Git
// write operation is exposed through these endpoints or arbitrary subcommands.
func (s *Server) workspaceGitHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string) bool {
	if len(parts) < 2 || len(parts) > 3 || parts[0] != "workspace" || parts[1] != "git" || len(parts) == 3 && parts[2] != "diff" {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	if r.Method != http.MethodGet {
		writeError(w, missing())
		return true
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		writeError(w, invalid(workspaceGitInvalid))
		return true
	}
	diff := len(parts) == 3
	for key, values := range q {
		if key != "project_id" && !(diff && (key == "path" || key == "source")) || len(values) != 1 || values[0] == "" {
			writeError(w, invalid(workspaceGitInvalid))
			return true
		}
	}
	if q.Get("project_id") == "" || q.Get("project_id") != project {
		writeError(w, invalid(workspaceGitInvalid))
		return true
	}
	if !diff {
		value, e := s.Service.GetWorkspaceGitStatus(r.Context(), actor, project)
		respond(w, http.StatusOK, value, e)
		return true
	}
	path, source := q.Get("path"), q.Get("source")
	if source == "" {
		source = "worktree"
	}
	if path == "" || len(path) > 4096 || !utf8.ValidString(path) || strings.IndexFunc(path, unicode.IsControl) >= 0 || source != "worktree" && source != "index" {
		writeError(w, invalid(workspaceGitInvalid))
		return true
	}
	value, e := s.Service.GetWorkspaceGitDiff(r.Context(), actor, project, path, source)
	respond(w, http.StatusOK, value, e)
	return true
}
