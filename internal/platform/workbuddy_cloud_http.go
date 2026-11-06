package platform

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func cloudHTTPBody(r *http.Request, out any) error {
	if r.Body == nil || r.URL.RawQuery != "" {
		return cloudInvalid()
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 || wc.Decode(raw, out) != nil {
		return cloudInvalid()
	}
	return nil
}

func cloudHTTPHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// The normal API entry authenticates Actor and validates the browser's original
// view before calling this handler. Domain commands recheck current authority.
func (s *Server) workBuddyCloudHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string, c *WorkBuddyConnections) bool {
	if len(parts) == 1 && parts[0] == "workbuddy-cloud-progress" {
		workBuddyCloudProgressListHTTP(w, r, actor, scope, boundProject, c)
		return true
	}
	if len(parts) == 5 && parts[0] == "runs" && parts[2] == "nodes" && parts[4] == "workbuddy-status" {
		workBuddyCloudProgressHTTP(w, r, actor, scope, boundProject, parts[1], parts[3], c)
		return true
	}
	machine := len(parts) >= 2 && parts[0] == "runner" && parts[1] == "workbuddy-cloud-submissions"
	browser := len(parts) >= 1 && (parts[0] == "workbuddy-cloud-delegations" || parts[0] == "workbuddy-cloud-submissions" || parts[0] == "workbuddy-cloud-nodes" || parts[0] == "workbuddy-cloud-history-loads")
	if !machine && !browser {
		return false
	}
	cloudHTTPHeaders(w)
	if machine && (actor.kind != actorProjectRunner || scope != "runner") || browser && (!actor.IsBrowser() || scope != "") {
		writeError(w, forbidden())
		return true
	}
	if c == nil {
		writeError(w, &APIError{503, workBuddyUnavailable, workBuddyUnavailable, nil})
		return true
	}
	if s.workBuddyCloudHistorySharingHTTP(w, r, actor, scope, boundProject, parts, c) {
		return true
	}
	if s.workBuddyCloudCollaborationHTTP(w, r, actor, scope, boundProject, parts, c) {
		return true
	}
	if machine {
		if r.Method != http.MethodPost {
			writeError(w, missing())
			return true
		}
		switch {
		case len(parts) == 2:
			var req wc.CloudCreateRequest
			if cloudHTTPBody(r, &req) != nil || req.ProjectID != boundProject {
				writeError(w, cloudInvalid())
				return true
			}
			out, err := c.CreateCloudTask(r.Context(), actor, boundProject, req)
			respond(w, http.StatusCreated, out, err)
		case len(parts) == 3 && parts[2] == "lookup":
			var req wc.CloudLookupRequest
			if cloudHTTPBody(r, &req) != nil || req.ProjectID != boundProject {
				writeError(w, cloudInvalid())
				return true
			}
			out, err := c.LookupCloudSubmission(r.Context(), actor, boundProject, req)
			respond(w, http.StatusOK, out, err)
		case len(parts) == 4 && parts[3] == "refresh":
			var req wc.CloudRefreshRequest
			if cloudHTTPBody(r, &req) != nil || req.ProjectID != boundProject {
				writeError(w, cloudInvalid())
				return true
			}
			out, err := c.RefreshCloudTask(r.Context(), actor, boundProject, parts[2], req)
			respond(w, http.StatusOK, out, err)
		default:
			writeError(w, missing())
		}
		return true
	}
	if r.Method == http.MethodPost {
		switch {
		case len(parts) == 1 && parts[0] == "workbuddy-cloud-delegations":
			var req wc.CloudDelegateRequest
			if cloudHTTPBody(r, &req) != nil {
				writeError(w, cloudInvalid())
				return true
			}
			out, err := c.DelegateCloud(r.Context(), actor, req.ProjectID, req)
			respond(w, http.StatusCreated, out, err)
		case len(parts) == 3 && parts[0] == "workbuddy-cloud-delegations" && parts[2] == "revoke":
			var req wc.CloudRevokeRequest
			if cloudHTTPBody(r, &req) != nil {
				writeError(w, cloudInvalid())
				return true
			}
			out, err := c.RevokeCloudDelegation(r.Context(), actor, req.ProjectID, parts[1], req)
			respond(w, http.StatusOK, out, err)
		case len(parts) == 3 && parts[0] == "workbuddy-cloud-submissions" && parts[2] == "observe":
			var req wc.CloudObserveRequest
			if cloudHTTPBody(r, &req) != nil {
				writeError(w, cloudInvalid())
				return true
			}
			out, err := c.ObserveCloudTask(r.Context(), actor, req.ProjectID, parts[1], req)
			respond(w, http.StatusOK, out, err)
		default:
			writeError(w, missing())
		}
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
	for key, values := range query {
		if len(values) != 1 || values[0] == "" || (key != "project_id" && !(parts[0] == "workbuddy-cloud-nodes" && (key == "run_id" || key == "node_id"))) {
			writeError(w, cloudInvalid())
			return true
		}
	}
	project := query.Get("project_id")
	if !handoffID(project, false) {
		writeError(w, cloudInvalid())
		return true
	}
	switch {
	case len(parts) == 1 && parts[0] == "workbuddy-cloud-nodes":
		out, err := c.GetCloudNode(r.Context(), actor, project, query.Get("run_id"), query.Get("node_id"))
		respond(w, http.StatusOK, out, err)
	case len(parts) == 2 && parts[0] == "workbuddy-cloud-delegations":
		out, err := c.GetCloudDelegation(r.Context(), actor, project, parts[1])
		respond(w, http.StatusOK, out, err)
	case len(parts) == 2 && parts[0] == "workbuddy-cloud-submissions":
		out, err := c.GetCloudSubmission(r.Context(), actor, project, parts[1])
		respond(w, http.StatusOK, out, err)
	default:
		writeError(w, missing())
	}
	return true
}

// Both progress reads share the same browser or scoped project API boundary.
func workBuddyCloudProgressQuery(w http.ResponseWriter, r *http.Request, actor Actor, scope string) (url.Values, string, bool) {
	cloudHTTPHeaders(w)
	if !(actor.IsBrowser() && scope == "" || actor.kind == actorProjectAPI && scope == "api") {
		writeError(w, forbidden())
		return nil, "", false
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, &APIError{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "Method not allowed"})
		return nil, "", false
	}
	if len(r.URL.RawQuery) > 8192 {
		writeError(w, cloudInvalid())
		return nil, "", false
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	project := query.Get("project_id")
	if err != nil || len(query["project_id"]) != 1 || !handoffID(project, false) {
		writeError(w, cloudInvalid())
		return nil, "", false
	}
	return query, project, true
}

func workBuddyCloudProgressHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject, run, node string, c *WorkBuddyConnections) {
	query, project, ok := workBuddyCloudProgressQuery(w, r, actor, scope)
	if !ok {
		return
	}
	if len(query) != 1 {
		writeError(w, cloudInvalid())
		return
	}
	if scope == "api" && project != boundProject {
		writeError(w, forbidden())
		return
	}
	if c == nil {
		writeError(w, &APIError{503, workBuddyUnavailable, workBuddyUnavailable, nil})
		return
	}
	out, err := c.GetCloudNodeProgress(r.Context(), actor, project, run, node)
	respond(w, http.StatusOK, out, err)
}

func workBuddyCloudProgressListHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, c *WorkBuddyConnections) {
	query, project, ok := workBuddyCloudProgressQuery(w, r, actor, scope)
	if !ok {
		return
	}
	limit, before, filter, taskID := cloudProgressDefaultLimit, "", "", ""
	for key, values := range query {
		if len(values) != 1 || values[0] == "" {
			writeError(w, cloudInvalid())
			return
		}
		switch key {
		case "project_id":
		case "limit":
			n, err := strconv.Atoi(values[0])
			if err != nil || n < 1 || n > cloudProgressMaxLimit || strconv.Itoa(n) != values[0] {
				writeError(w, cloudInvalid())
				return
			}
			limit = n
		case "before":
			if len(values[0]) > 1024 {
				writeError(w, cloudInvalid())
				return
			}
			before = values[0]
		case "filter":
			if values[0] != wc.CloudProgressAttentionFilter {
				writeError(w, cloudInvalid())
				return
			}
			filter = values[0]
		case "task_id":
			if !wc.ValidCloudProgressTaskID(values[0]) {
				writeError(w, cloudInvalid())
				return
			}
			taskID = values[0]
		default:
			writeError(w, cloudInvalid())
			return
		}
	}
	if scope == "api" && project != boundProject {
		writeError(w, forbidden())
		return
	}
	if c == nil {
		writeError(w, &APIError{503, workBuddyUnavailable, workBuddyUnavailable, nil})
		return
	}
	out, err := c.ListCloudNodeProgress(r.Context(), actor, project, limit, before, filter, taskID)
	respond(w, http.StatusOK, out, err)
}

// Pool machines retain their real Bearer proof; no human Actor is fabricated.
// Invoke from poolRunnerHTTP after its Bearer-header check, before its decoder.
func (s *Server) poolWorkBuddyCloudHTTP(w http.ResponseWriter, r *http.Request, token, path string, c *WorkBuddyConnections) bool {
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] != "pool-runner" || parts[1] != "workbuddy-cloud-submissions" {
		return false
	}
	cloudHTTPHeaders(w)
	if strings.TrimSpace(token) == "" {
		writeError(w, unauthenticated())
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
	switch {
	case len(parts) == 2:
		var req wc.CloudCreateRequest
		if cloudHTTPBody(r, &req) != nil {
			writeError(w, cloudInvalid())
			return true
		}
		out, err := c.CreatePoolCloudTask(r.Context(), token, req)
		respond(w, http.StatusCreated, out, err)
	case len(parts) == 3 && parts[2] == "lookup":
		var req wc.CloudLookupRequest
		if cloudHTTPBody(r, &req) != nil {
			writeError(w, cloudInvalid())
			return true
		}
		out, err := c.LookupPoolCloudSubmission(r.Context(), token, req)
		respond(w, http.StatusOK, out, err)
	case len(parts) == 4 && parts[3] == "refresh":
		var req wc.CloudRefreshRequest
		if cloudHTTPBody(r, &req) != nil {
			writeError(w, cloudInvalid())
			return true
		}
		out, err := c.RefreshPoolCloudTask(r.Context(), token, parts[2], req)
		respond(w, http.StatusOK, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
