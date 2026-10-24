package platform

import (
	"database/sql"
	"net/http"
	"strconv"
)

func schedulingPageLimit(r *http.Request) (int, error) {
	if r.URL.Query().Get("limit") == "" {
		return 50, nil
	}
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n < 1 || n > 100 {
		return 0, invalid("limit 必须是 1–100 的整数")
	}
	return n, nil
}

func (s *Server) schedulingHTTP(w http.ResponseWriter, r *http.Request, p Principal, scope, path string, parts []string, project string, m map[string]any) bool {
	isScheduling := parts[0] == "scheduling"
	isNode := len(parts) >= 5 && parts[0] == "runs" && parts[2] == "nodes" && (parts[4] == "scheduling" || parts[4] == "attempts")
	if !isScheduling && !isNode {
		return false
	}
	if r.Method != "GET" && scope != "" {
		writeError(w, forbidden())
		return true
	}
	switch {
	case path == "scheduling/policy" && r.Method == "GET":
		out, err := s.Service.SchedulingPolicy(r.Context(), s.Engine, p, project, scope == "")
		respond(w, 200, out, err)
	case path == "scheduling/policy-previews" && r.Method == "POST":
		var request SchedulingPolicyPreviewRequest
		policy, ok := m["policy"].(map[string]any)
		_, hasBudget := policy["budget_limit"]
		_, hasVersion := m["expected_version"]
		if !ok || policy["concurrency_limit"] == nil || policy["resource_limits"] == nil || !hasBudget || !hasVersion || m["expected_version"] == nil {
			writeError(w, invalid("请显式提供 expected_version、concurrency_limit、resource_limits 及 budget_limit（null 表示不限制）"))
			return true
		}
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CreateSchedulingPreview(r.Context(), s.Engine, p, project, request, tokenOf(r))
		respond(w, 201, out, err)
	case len(parts) == 3 && parts[0] == "scheduling" && parts[1] == "policy-previews" && r.Method == "GET":
		out, err := s.Service.GetSchedulingPreview(r.Context(), s.Engine, p, project, parts[2])
		respond(w, 200, out, err)
	case len(parts) == 4 && parts[0] == "scheduling" && parts[1] == "policy-previews" && parts[3] == "commit" && r.Method == "POST":
		var request SchedulingPolicyCommitRequest
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CommitSchedulingPreview(r.Context(), s.Engine, p, project, parts[2], request, tokenOf(r))
		respond(w, 200, out, err)
	case path == "scheduling/queue" && r.Method == "GET":
		limit, err := schedulingPageLimit(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Engine.QueuePage(r.Context(), p.TenantID, project, limit, r.URL.Query().Get("cursor"))
		respond(w, 200, out, err)
	case isNode && len(parts) == 5 && parts[4] == "scheduling" && r.Method == "GET":
		tx, err := s.Service.DB.BeginTx(r.Context(), &sql.TxOptions{ReadOnly: true})
		if err != nil {
			writeError(w, err)
			return true
		}
		defer tx.Rollback()
		out, err := s.Engine.AdmissionSnapshotTx(r.Context(), tx, p.TenantID, project, parts[1], parts[3])
		if err == nil {
			err = tx.Commit()
		}
		respond(w, 200, out, err)
	case isNode && len(parts) == 5 && parts[4] == "attempts" && r.Method == "GET":
		limit, err := schedulingPageLimit(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Engine.AttemptsPage(r.Context(), p.TenantID, project, parts[1], parts[3], limit, r.URL.Query().Get("cursor"))
		respond(w, 200, out, err)
	case isNode && len(parts) == 6 && parts[4] == "attempts" && r.Method == "GET":
		attempt, err := strconv.Atoi(parts[5])
		if err != nil || attempt < 1 {
			writeError(w, invalid("attempt 必须是正整数"))
			return true
		}
		out, err := s.Engine.GetAttempt(r.Context(), p.TenantID, project, parts[1], parts[3], attempt)
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
