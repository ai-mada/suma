package platform

import (
	"net/http"
	"net/url"
	"strconv"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// Called by the authenticated cloud API router. Domain methods independently
// re-establish the current human authority for every protected operation.
func (s *Server) workBuddyCloudCollaborationHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string, c *WorkBuddyConnections) bool {
	match := len(parts) > 0 && parts[0] == "workbuddy-cloud-history-loads" || len(parts) == 3 && parts[0] == "workbuddy-cloud-submissions" && (parts[2] == "collaboration" || parts[2] == "history-loads")
	if !match {
		return false
	}
	cloudHTTPHeaders(w)
	if !actor.IsBrowser() || scope != "" {
		writeError(w, forbidden())
		return true
	}
	if c == nil {
		writeError(w, &APIError{503, workBuddyUnavailable, workBuddyUnavailable, nil})
		return true
	}
	if len(parts) == 3 && parts[2] == "history-loads" {
		if r.Method != http.MethodPost {
			writeError(w, missing())
			return true
		}
		var req wc.CloudHistoryLoadRequest
		if cloudHTTPBody(r, &req) != nil || boundProject != "" && req.ProjectID != boundProject {
			writeError(w, cloudInvalid())
			return true
		}
		out, err := c.LoadCloudHistory(r.Context(), actor, req.ProjectID, parts[1], req)
		respond(w, http.StatusOK, out, err)
		return true
	}
	if r.Method != http.MethodGet || len(r.URL.RawQuery) > 8192 {
		writeError(w, cloudInvalid())
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, cloudInvalid())
		return true
	}
	page := len(parts) == 2 && parts[0] == "workbuddy-cloud-history-loads"
	for key, values := range query {
		if len(values) != 1 || values[0] == "" || (key != "project_id" && !(page && (key == "after" || key == "limit"))) {
			writeError(w, cloudInvalid())
			return true
		}
	}
	project := query.Get("project_id")
	if !handoffID(project, false) || boundProject != "" && project != boundProject {
		writeError(w, cloudInvalid())
		return true
	}
	if page {
		after, limit := 0, 50
		for _, item := range []struct {
			key    string
			target *int
		}{{"after", &after}, {"limit", &limit}} {
			if raw := query.Get(item.key); raw != "" {
				for _, digit := range raw {
					if digit < '0' || digit > '9' {
						writeError(w, cloudInvalid())
						return true
					}
				}
				n, e := strconv.Atoi(raw)
				if e != nil {
					writeError(w, cloudInvalid())
					return true
				}
				*item.target = n
			}
		}
		if limit < 1 || limit > 100 {
			writeError(w, cloudInvalid())
			return true
		}
		out, e := c.CloudHistory(r.Context(), actor, project, parts[1], after, limit)
		respond(w, http.StatusOK, out, e)
		return true
	}
	if len(parts) == 3 && parts[0] == "workbuddy-cloud-submissions" && parts[2] == "collaboration" {
		out, e := c.CloudCollaboration(r.Context(), actor, project, parts[1])
		respond(w, http.StatusOK, out, e)
		return true
	}
	writeError(w, missing())
	return true
}
