package platform

import "net/http"

// This adapter owns only parsing and dispatch. All current authority, receipt,
// version and transaction decisions stay in the application services.
func (s *Server) executionPoolsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, path string, parts []string) bool {

	if len(parts) == 0 || parts[0] != "execution-pools" && parts[0] != "project-execution" {
		return false
	}
	if scope != "" {
		writeError(w, forbidden())
		return true
	}
	if s.poolRunnerReadinessHTTP(w, r, actor, parts) {
		return true
	}
	var body map[string]any
	var err error
	if r.Method == http.MethodPost {
		body, err = decode(r)
		if err != nil {
			writeError(w, err)
			return true
		}
	}
	decodeAs := func(target any) bool {
		if err := strictAutomation(body, target); err != nil {
			writeError(w, err)
			return false
		}
		return true
	}
	page := func() (int, bool) {
		limit, err := schedulingPageLimit(r)
		if err != nil {
			writeError(w, err)
			return 0, false
		}
		return limit, true
	}
	ctx := r.Context()
	switch {
	case path == "execution-pools" && r.Method == http.MethodGet:
		limit, ok := page()
		if !ok {
			return true
		}
		out, err := s.Service.ListExecutionPools(ctx, s.Engine, actor, limit, r.URL.Query().Get("cursor"))
		respond(w, 200, out, err)
	case path == "execution-pools" && r.Method == http.MethodPost:
		var request ExecutionPoolCreateRequest
		if !poolPolicyEnvelope(body["policy"]) {
			writeError(w, invalid("请显式提供完整资源池策略及空值语义"))
			return true
		}
		if !decodeAs(&request) {
			return true
		}
		out, err := s.Service.CreateExecutionPool(ctx, s.Engine, actor, request)
		respond(w, 201, out, err)
	case len(parts) == 2 && parts[0] == "execution-pools" && r.Method == http.MethodGet:
		out, err := s.Service.GetExecutionPool(ctx, s.Engine, actor, parts[1])
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[0] == "execution-pools" && parts[2] == "commands" && r.Method == http.MethodPost:
		var request ExecutionPoolCommandRequest
		if !decodeAs(&request) {
			return true
		}
		out, err := s.Service.CommandExecutionPool(ctx, s.Engine, actor, parts[1], request)
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[0] == "execution-pools" && parts[2] == "policy-previews" && r.Method == http.MethodPost:
		var request ExecutionPoolPolicyPreviewRequest
		if body["expected_version"] == nil || !poolPolicyEnvelope(body["policy"]) {
			writeError(w, invalid("请显式提供当前策略版本及完整资源池策略"))
			return true
		}
		if !decodeAs(&request) {
			return true
		}
		out, err := s.Service.CreateExecutionPoolPolicyPreview(ctx, s.Engine, actor, parts[1], request)
		respond(w, 201, out, err)
	case len(parts) == 4 && parts[0] == "execution-pools" && parts[2] == "policy-previews" && r.Method == http.MethodGet:
		out, err := s.Service.GetExecutionPoolPolicyPreview(ctx, s.Engine, actor, parts[1], parts[3])
		respond(w, 200, out, err)
	case len(parts) == 5 && parts[0] == "execution-pools" && parts[2] == "policy-previews" && parts[4] == "commit" && r.Method == http.MethodPost:
		var request ExecutionPreviewCommitRequest
		if !decodeAs(&request) {
			return true
		}
		out, err := s.Service.CommitExecutionPoolPolicyPreview(ctx, s.Engine, actor, parts[1], parts[3], request)
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[0] == "execution-pools" && parts[2] == "grants" && r.Method == http.MethodGet:
		limit, ok := page()
		if !ok {
			return true
		}
		out, err := s.Service.ListOwnedPoolGrants(ctx, actor, parts[1], limit, r.URL.Query().Get("cursor"))
		respond(w, 200, out, err)
	case len(parts) == 5 && parts[0] == "execution-pools" && parts[2] == "grants" && parts[4] == "commands" && r.Method == http.MethodPost:
		var request ExecutionPoolGrantCommand
		if !decodeAs(&request) {
			return true
		}
		if request.ProjectID != "" {
			writeError(w, invalid("拥有方操作不接受项目上下文"))
			return true
		}
		out, err := s.Service.CommandOwnedPoolGrant(ctx, s.Engine, actor, parts[1], parts[3], request)
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[0] == "execution-pools" && parts[2] == "runners" && r.Method == http.MethodGet:
		w.Header().Set("Cache-Control", "no-store")
		limit, ok := page()
		if !ok {
			return true
		}
		out, err := s.Service.ListExecutionPoolRunners(ctx, actor, parts[1], limit, r.URL.Query().Get("cursor"))
		respond(w, 200, out, err)
	case len(parts) == 4 && parts[0] == "execution-pools" && parts[2] == "runners" && r.Method == http.MethodGet:
		w.Header().Set("Cache-Control", "no-store")
		out, err := s.Service.readExecutionPoolRunner(ctx, actor, parts[1], parts[3], func() error {
			if r.URL.RawQuery != "" || r.URL.ForceQuery {
				return invalid("输入无效")
			}
			return nil
		})
		respond(w, http.StatusOK, out, err)
	case len(parts) == 3 && parts[0] == "execution-pools" && parts[2] == "runners" && r.Method == http.MethodPost:
		var request ExecutionPoolRunnerCreateRequest
		if !decodeAs(&request) {
			return true
		}
		out, err := s.Service.RegisterExecutionPoolRunner(ctx, actor, parts[1], request)
		respond(w, 201, out, err)
	case len(parts) == 5 && parts[0] == "execution-pools" && parts[2] == "runners" && parts[4] == "credentials" && r.Method == http.MethodPost:
		var request ExecutionPoolCredentialRequest
		if !decodeAs(&request) {
			return true
		}
		out, err := s.Service.RotateExecutionPoolCredential(ctx, actor, parts[1], parts[3], request)
		respond(w, 201, out, err)
	case len(parts) == 5 && parts[0] == "execution-pools" && parts[2] == "runners" && parts[4] == "commands" && r.Method == http.MethodPost:
		var request ExecutionPoolCommandRequest
		if !decodeAs(&request) {
			return true
		}
		out, err := s.Service.RevokeExecutionPoolRunner(ctx, actor, parts[1], parts[3], request)
		respond(w, 200, out, err)
	case path == "project-execution/grants" && r.Method == http.MethodGet:
		limit, ok := page()
		if !ok {
			return true
		}
		out, err := s.Service.ListProjectPoolGrants(ctx, actor, r.URL.Query().Get("project_id"), limit, r.URL.Query().Get("cursor"))
		respond(w, 200, out, err)
	case path == "project-execution/grants" && r.Method == http.MethodPost:
		var request ExecutionPoolGrantRequest
		if !decodeAs(&request) {
			return true
		}
		out, err := s.Service.RequestExecutionPoolGrant(ctx, actor, request)
		respond(w, 201, out, err)
	case len(parts) == 4 && parts[0] == "project-execution" && parts[1] == "grants" && parts[3] == "commands" && r.Method == http.MethodPost:
		var request ExecutionPoolGrantCommand
		if !decodeAs(&request) {
			return true
		}
		out, err := s.Service.WithdrawProjectPoolGrant(ctx, s.Engine, actor, parts[2], request)
		respond(w, 200, out, err)
	case path == "project-execution/route" && r.Method == http.MethodGet:
		out, err := s.Service.GetProjectExecutionRoute(ctx, s.Engine, actor, r.URL.Query().Get("project_id"))
		respond(w, 200, out, err)
	case path == "project-execution/runners" && r.Method == http.MethodGet:
		out, err := s.Service.ListProjectPoolRunners(ctx, actor, r.URL.Query().Get("project_id"))
		respond(w, 200, out, err)
	case path == "project-execution/route-previews" && r.Method == http.MethodPost:
		var request ExecutionRoutePreviewRequest
		// Version zero is the real initial route, so absence must be rejected
		// before decoding into an integer's indistinguishable zero value.
		if body["expected_version"] == nil {
			writeError(w, invalid("请显式提供当前执行路由版本"))
			return true
		}
		if !decodeAs(&request) {
			return true
		}
		out, err := s.Service.CreateExecutionRoutePreview(ctx, s.Engine, actor, request)
		respond(w, 201, out, err)
	case len(parts) == 3 && parts[0] == "project-execution" && parts[1] == "route-previews" && r.Method == http.MethodGet:
		out, err := s.Service.GetExecutionRoutePreview(ctx, s.Engine, actor, r.URL.Query().Get("project_id"), parts[2])
		respond(w, 200, out, err)
	case len(parts) == 4 && parts[0] == "project-execution" && parts[1] == "route-previews" && parts[3] == "commit" && r.Method == http.MethodPost:
		var request ExecutionPreviewCommitRequest
		if !decodeAs(&request) {
			return true
		}
		out, err := s.Service.CommitExecutionRoutePreview(ctx, s.Engine, actor, parts[2], request)
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}

// Null or omitted fields must not silently become unlimited/default quotas.
func poolPolicyEnvelope(raw any) bool {
	policy, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	_, budget := policy["budget_limit"]
	_, resources := policy["resource_limits"].(map[string]any)
	_, weights := policy["tenant_weights"].(map[string]any)
	return policy["concurrency_limit"] != nil && budget && resources && weights
}
