package platform

import (
	"net/http"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func (s *Server) workBuddyCloudHistorySharingHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string, c *WorkBuddyConnections) bool {
	if len(parts) != 3 || parts[0] != "workbuddy-cloud-history-loads" || (parts[2] != "share-preview" && parts[2] != "shares") {
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
	if r.Method != http.MethodPost {
		writeError(w, missing())
		return true
	}
	if parts[2] == "share-preview" {
		var req wc.CloudHistorySharePreviewRequest
		if cloudHTTPBody(r, &req) != nil || boundProject != "" && boundProject != req.ProjectID {
			writeError(w, historyShareInvalid())
			return true
		}
		out, err := c.PreviewCloudHistoryShare(r.Context(), actor, req.ProjectID, parts[1], req)
		respond(w, http.StatusOK, out, err)
		return true
	}
	var req wc.CloudHistoryShareRequest
	if cloudHTTPBody(r, &req) != nil || boundProject != "" && boundProject != req.ProjectID {
		writeError(w, historyShareInvalid())
		return true
	}
	out, err := c.ShareCloudHistory(r.Context(), actor, req.ProjectID, parts[1], req)
	respond(w, http.StatusCreated, out, err)
	return true
}
