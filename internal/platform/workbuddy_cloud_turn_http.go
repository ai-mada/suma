package platform

import (
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
	"net/http"
	"net/url"
	"strconv"
)

func (s *Server) workBuddyCloudTurnsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string, c *WorkBuddyConnections) bool {
	match := len(parts) > 0 && parts[0] == "workbuddy-cloud-turns" || len(parts) == 3 && parts[0] == "workbuddy-cloud-submissions" && parts[2] == "turns"
	if !match {
		return false
	}
	cloudHTTPHeaders(w)
	fail := func(e error) bool { writeError(w, e); return true }
	if !actor.IsBrowser() || scope != "" {
		return fail(forbidden())
	}
	if c == nil {
		return fail(turnUnavailable())
	}
	if len(parts) == 3 && parts[0] == "workbuddy-cloud-submissions" && r.Method == http.MethodPost {
		var req wc.CloudTurnPromptRequest
		if cloudHTTPBody(r, &req) != nil || boundProject != "" && boundProject != req.ProjectID {
			return fail(turnInvalid())
		}
		out, e := c.PromptCloudTurn(r.Context(), actor, req.ProjectID, parts[1], req)
		respond(w, 202, out, e)
		return true
	}
	if parts[0] == "workbuddy-cloud-turns" && r.Method == http.MethodPost {
		if len(parts) == 5 && parts[2] == "permissions" && parts[4] == "reply" {
			var req wc.CloudTurnPermissionReplyRequest
			if cloudHTTPBody(r, &req) != nil || boundProject != "" && boundProject != req.ProjectID {
				return fail(turnInvalid())
			}
			out, e := c.ReplyCloudTurnPermission(r.Context(), actor, req.ProjectID, parts[1], parts[3], req)
			respond(w, 202, out, e)
			return true
		}
		if len(parts) == 3 && (parts[2] == "cancel" || parts[2] == "reconcile") {
			var req wc.CloudTurnControlRequest
			if cloudHTTPBody(r, &req) != nil || boundProject != "" && boundProject != req.ProjectID {
				return fail(turnInvalid())
			}
			var out wc.CloudTurnCommand
			var e error
			status := 202
			if parts[2] == "cancel" {
				out, e = c.CancelCloudTurn(r.Context(), actor, req.ProjectID, parts[1], req)
			} else {
				out, e = c.ReconcileCloudTurn(r.Context(), actor, req.ProjectID, parts[1], req)
				status = 200
			}
			respond(w, status, out, e)
			return true
		}
		return fail(missing())
	}
	if r.Method != http.MethodGet || len(r.URL.RawQuery) > 8192 {
		return fail(turnInvalid())
	}
	page := len(parts) == 2 && parts[0] == "workbuddy-cloud-turns"
	view := len(parts) == 3 && parts[0] == "workbuddy-cloud-submissions" && parts[2] == "turns"
	if !page && !view {
		return fail(missing())
	}
	query, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return fail(turnInvalid())
	}
	for key, values := range query {
		if len(values) != 1 || values[0] == "" || key != "project_id" && !(page && (key == "after" || key == "limit")) {
			return fail(turnInvalid())
		}
	}
	project := query.Get("project_id")
	if !handoffID(project, false) || boundProject != "" && boundProject != project {
		return fail(turnInvalid())
	}
	if view {
		out, e := c.CloudTurns(r.Context(), actor, project, parts[1])
		respond(w, 200, out, e)
		return true
	}
	after, limit := 0, 50
	for _, item := range []struct {
		key    string
		target *int
	}{{"after", &after}, {"limit", &limit}} {
		if raw := query.Get(item.key); raw != "" {
			for _, digit := range raw {
				if digit < '0' || digit > '9' {
					return fail(turnInvalid())
				}
			}
			n, e := strconv.Atoi(raw)
			if e != nil {
				return fail(turnInvalid())
			}
			*item.target = n
		}
	}
	if limit < 1 || limit > 200 {
		return fail(turnInvalid())
	}
	out, e := c.GetCloudTurn(r.Context(), actor, project, parts[1], after, limit)
	respond(w, 200, out, e)
	return true
}
