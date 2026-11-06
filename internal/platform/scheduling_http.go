package platform

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ai-mada/suma/internal/dag"
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

func (s *Server) schedulingHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, path string, parts []string, project string, m map[string]any) bool {
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
	case isNode && len(parts) == 7 && parts[4] == "attempts" && parts[6] == "tool-events" && r.Method == "GET":
		s.attemptToolHistoryHTTP(w, r, actor, parts, project)
	case path == "scheduling/policy" && r.Method == "GET":
		out, err := s.Service.SchedulingPolicy(r.Context(), s.Engine, actor, project, scope == "")
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
		out, err := s.Service.CreateSchedulingPreview(r.Context(), s.Engine, actor, project, request)
		respond(w, 201, out, err)
	case len(parts) == 3 && parts[0] == "scheduling" && parts[1] == "policy-previews" && r.Method == "GET":
		out, err := s.Service.GetSchedulingPreview(r.Context(), s.Engine, actor, project, parts[2])
		respond(w, 200, out, err)
	case len(parts) == 4 && parts[0] == "scheduling" && parts[1] == "policy-previews" && parts[3] == "commit" && r.Method == "POST":
		var request SchedulingPolicyCommitRequest
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CommitSchedulingPreview(r.Context(), s.Engine, actor, project, parts[2], request)
		respond(w, 200, out, err)
	case path == "scheduling/queue" && r.Method == "GET":
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			writeError(w, invalid("队列查询参数无效"))
			return true
		}
		filter := dag.ReadyQueueFilter{Route: query.Get("route"), RunID: query.Get("run_id")}
		if route, present := query["route"]; present && (len(route) != 1 || filter.Route == "") {
			writeError(w, invalid("route 必须是单个 project 或 pool"))
			return true
		}
		if runID, present := query["run_id"]; present && (len(runID) != 1 || filter.RunID == "") {
			writeError(w, invalid("run_id 必须是单个运行 ID"))
			return true
		}
		if !filter.Valid() {
			writeError(w, invalid("队列筛选参数无效"))
			return true
		}
		limit, err := schedulingPageLimit(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		var out dag.ReadyPage
		err = s.Service.schedulingRead(r.Context(), actor, project, "scheduling-queue", purposeProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
			tenant := auth.target.tenantID
			var err error
			out, err = s.Engine.QueuePageTx(ctx, auth.tx, tenant, project, filter, limit, query.Get("cursor"))
			if err != nil {
				return err
			}
			poolBlockers := make(map[string][]dag.SchedulingBlocker)
			toolProfiles := make(map[string]schedulingToolProfile)
			for _, item := range out.Items {
				if item.Admission == nil {
					continue
				}
				if err = s.Service.decoratePoolAdmissionTx(ctx, auth.tx, tenant, project, item.Admission, poolBlockers); err != nil {
					return err
				}
				if err = s.Service.decorateToolAdmissionTx(ctx, auth.tx, tenant, project, item.Admission, toolProfiles); err != nil {
					return err
				}
			}
			return nil
		})
		respond(w, 200, out, err)
	case isNode && len(parts) == 5 && parts[4] == "scheduling" && r.Method == "GET":
		var out dag.NodeAdmissionSnapshot
		err := s.Service.schedulingRead(r.Context(), actor, project, parts[1]+":"+parts[3], purposeProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
			var err error
			out, err = s.Engine.AdmissionSnapshotTx(ctx, auth.tx, auth.target.tenantID, project, parts[1], parts[3])
			if err != nil {
				return err
			}
			tenant := auth.target.tenantID
			if err = s.Service.decoratePoolAdmissionTx(ctx, auth.tx, tenant, project, &out, nil); err != nil {
				return err
			}
			return s.Service.decorateToolAdmissionTx(ctx, auth.tx, tenant, project, &out, nil)
		})
		respond(w, 200, out, err)
	case isNode && len(parts) == 5 && parts[4] == "attempts" && r.Method == "GET":
		limit, err := schedulingPageLimit(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		var out dag.AttemptPage
		err = s.Service.schedulingRead(r.Context(), actor, project, parts[1]+":"+parts[3], purposeProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
			var err error
			out, err = s.Engine.AttemptsPageTx(ctx, auth.tx, auth.target.tenantID, project, parts[1], parts[3], limit, r.URL.Query().Get("cursor"))
			return err
		})
		respond(w, 200, out, err)
	case isNode && len(parts) == 6 && parts[4] == "attempts" && r.Method == "GET":
		attempt, err := strconv.Atoi(parts[5])
		if err != nil || attempt < 1 {
			writeError(w, invalid("attempt 必须是正整数"))
			return true
		}
		var out dag.ExecutionAttempt
		err = s.Service.schedulingRead(r.Context(), actor, project, parts[1]+":"+parts[3]+":"+parts[5], purposeProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
			var err error
			out, err = s.Engine.GetAttemptTx(ctx, auth.tx, auth.target.tenantID, project, parts[1], parts[3], attempt)
			return err
		})
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
