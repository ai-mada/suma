package platform

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/releasecontract"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Server struct {
	// BuildVersion comes from the executable build metadata, not a wire/API schema version.
	BuildVersion   string
	Service        *Service
	Engine         *dag.Engine
	WebDir         string
	AllowedOrigins []string
	SetupToken     string
	// PublicURL is an operator-controlled origin. Never derive credential
	// destinations from client Host or forwarding headers.
	PublicURL       string
	PublicURLSource string
	// Maintenance freezes business commands while operators verify an upgrade.
	// Authentication remains available; the control process also pauses projectors.
	Maintenance bool
	mux         *http.ServeMux
}

func (s *Server) Handler() http.Handler {
	s.mux = http.NewServeMux()
	s.mux.Handle("/api/v1/mcp/projects/", s.NewMCPHandler())
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Service.DB.PingContext(r.Context()); err != nil {
			writeError(w, err)
			return
		}
		buildVersion := s.BuildVersion
		if buildVersion == "" {
			buildVersion = "1.0.0"
		}
		writeJSON(w, 200, map[string]any{"status": "ok", "database": "connected", "version": buildVersion, "maintenance": s.Maintenance})
	})
	s.mux.HandleFunc("/api/v1/", s.api)
	s.mux.HandleFunc("/", s.web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setResponseLanguage(w, r)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		defer func() {
			if e := recover(); e != nil {
				slog.Error("request panic", "path", r.URL.Path)
				writeError(w, fmt.Errorf("internal error"))
			}
		}()
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Origin") != "" {
			o, err := url.Parse(r.Header.Get("Origin"))
			allowed := err == nil && o.Host == r.Host
			for _, v := range s.AllowedOrigins {
				if r.Header.Get("Origin") == v {
					allowed = true
				}
			}
			if !allowed {
				writeError(w, &APIError{403, "origin_rejected", "请求来源不在允许范围", nil})
				return
			}
		}
		if s.Maintenance && strings.HasPrefix(r.URL.Path, "/api/v1/") && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && !(r.Method == http.MethodPost && (r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/auth/logout")) {
			w.Header().Set("Retry-After", "5")
			writeError(w, &APIError{http.StatusLocked, "maintenance_mode", "服务正在升级核验，尚未执行本次操作，请稍后重试", nil})
			return
		}
		bodyLimit := int64(2 << 20)
		if r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/api/v1/runner/artifact-uploads/") && strings.Contains(r.URL.Path, "/blobs/") {
			bodyLimit = releasecontract.MaxFileBytes + 1
		}
		if r.Method == http.MethodPut && (strings.HasPrefix(r.URL.Path, "/api/v1/runner/candidate-transfers/") || strings.HasPrefix(r.URL.Path, "/api/v1/pool-runner/candidate-transfers/")) && strings.HasSuffix(r.URL.Path, "/bundle") {
			bodyLimit = cc.MaxBundleBytes + 1
		}
		r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
		s.mux.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(localizePresentation(w, v))
}
func writeError(w http.ResponseWriter, err error) {
	e := &APIError{500, "internal_error", "服务暂时无法完成操作", nil}
	var ae *APIError
	if errors.As(err, &ae) {
		e = ae
	} else {
		switch {
		case errors.Is(err, dag.ErrNotFound):
			e = &APIError{404, "not_found", "运行或流程不存在", nil}
		case errors.Is(err, dag.ErrInvalid):
			e = &APIError{400, "invalid_graph", err.Error(), nil}
		case errors.Is(err, dag.ErrConflict), errors.Is(err, dag.ErrStale):
			e = &APIError{409, "version_conflict", err.Error(), nil}
		case errors.Is(err, dag.ErrUnsupported):
			e = &APIError{422, "unsupported", err.Error(), nil}
		default:
			slog.Error("request failed", "error", err)
		}
	}
	writeJSON(w, e.Status, map[string]any{"error": localizedAPIError(w, e, err), "setup_required": e.Code == "unauthenticated" && e.Details != nil})
}
func decode(r *http.Request) (map[string]any, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, invalid("请求无法读取或超过大小限制")
	}
	if err = RejectDuplicateReleaseJSON(raw); err != nil {
		return nil, err
	}
	var m map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	// These routes decode into strict integer DTOs after the public envelope.
	// Preserve numeric lexemes so a fractional counter near 2^53 cannot round
	// into a valid integer before that validation. Legacy map-backed routes
	// retain their existing number representation until their DTO migration.
	if strings.HasPrefix(r.URL.Path, "/api/v1/candidate-compositions") || (strings.HasPrefix(r.URL.Path, "/api/v1/tasks/") && strings.HasSuffix(r.URL.Path, "/code-baseline")) || strings.HasPrefix(r.URL.Path, "/api/v1/failure-diagnosis") || strings.HasPrefix(r.URL.Path, "/api/v1/repository-provider/") || strings.HasPrefix(r.URL.Path, "/api/v1/repository-integrations") || strings.HasPrefix(r.URL.Path, "/api/v1/repository-integration-targets") || strings.HasPrefix(r.URL.Path, "/api/v1/expert-teams") || strings.HasPrefix(r.URL.Path, "/api/v1/expert-team-plans") || strings.HasPrefix(r.URL.Path, "/api/v1/knowledge-intelligence") || strings.HasPrefix(r.URL.Path, "/api/v1/skill-catalog") || strings.HasPrefix(r.URL.Path, "/api/v1/usage") || strings.HasPrefix(r.URL.Path, "/api/v1/planning/analyses") || strings.HasPrefix(r.URL.Path, "/api/v1/planning/alerts") || (strings.HasPrefix(r.URL.Path, "/api/v1/requirements/") && strings.Contains(r.URL.Path, "/design")) || strings.HasPrefix(r.URL.Path, "/api/v1/task-dispatch-plans") || strings.HasPrefix(r.URL.Path, "/api/v1/task-handoffs") || strings.HasPrefix(r.URL.Path, "/api/v1/project-knowledge") || r.URL.Path == "/api/v1/agent/tools/invoke" || strings.HasPrefix(r.URL.Path, "/api/v1/scheduling/") || strings.HasPrefix(r.URL.Path, "/api/v1/runner/") || strings.HasPrefix(r.URL.Path, "/api/v1/execution-pools") || strings.HasPrefix(r.URL.Path, "/api/v1/project-execution/") {
		d.UseNumber()
	}
	if err := d.Decode(&m); err != nil {
		return nil, invalid("请求必须是 JSON 对象")
	}
	if m == nil {
		return nil, invalid("请求必须是 JSON 对象")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, invalid("请求包含多余 JSON 内容")
	}
	return m, nil
}
func tokenOf(r *http.Request) string {
	if v := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); v != r.Header.Get("Authorization") {
		return v
	}
	c, err := r.Cookie("suma_session")
	if err != nil {
		return ""
	}
	return c.Value
}
func cookie(w http.ResponseWriter, r *http.Request, token string) {
	maxAge := 7 * 24 * 3600
	if token == "" {
		maxAge = -1
	}
	http.SetCookie(w, &http.Cookie{Name: "suma_session", Value: token, Path: "/", HttpOnly: true, Secure: r.TLS != nil || os.Getenv("SUMA_SECURE_COOKIES") == "true", SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}
func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
	parts := strings.Split(path, "/")
	if s.automationIngressPublicHTTP(w, r, path, parts) {
		return
	}
	if s.repositoryProviderHTTP(w, r, path) {
		return
	}
	if s.releaseProviderHTTP(w, r, path) {
		return
	}
	if s.poolCodeInputHTTP(w, r, path) {
		return
	}
	if s.poolCandidateTransferHTTP(w, r, path) {
		return
	}
	if s.poolRunnerHTTP(w, r, path) {
		return
	}
	if localizationCatalogHTTP(w, r, path) {
		return
	}
	if path == "auth/setup" && r.Method == "POST" {
		if s.SetupToken != "" && r.Header.Get("X-Suma-Setup-Token") != s.SetupToken {
			writeError(w, forbidden())
			return
		}
		m, err := decode(r)
		if err != nil {
			writeError(w, err)
			return
		}
		_, token, err := s.Service.Setup(ctx, m)
		if err != nil {
			writeError(w, err)
			return
		}
		cookie(w, r, token)
		s.sessionForToken(w, r, token)
		return
	}
	if path == "auth/login" && r.Method == "POST" {
		m, err := decode(r)
		if err != nil {
			writeError(w, err)
			return
		}
		_, token, err := s.Service.Login(ctx, str(m, "email"), str(m, "password"))
		if err != nil {
			writeError(w, err)
			return
		}
		cookie(w, r, token)
		s.sessionForToken(w, r, token)
		return
	}
	if path == "auth/invitations/accept" && r.Method == "POST" {
		if kind, _ := s.Service.TokenScope(ctx, tokenOf(r)); kind != "" {
			writeError(w, forbidden())
			return
		}
		m, e := decode(r)
		if e != nil {
			writeError(w, e)
			return
		}
		_, token, e := s.Service.AcceptInvitation(ctx, m)
		if e != nil {
			writeError(w, e)
			return
		}
		cookie(w, r, token)
		s.sessionForToken(w, r, token)
		return
	}
	// Logout only withdraws the credential selected by the real request secret.
	// It stays idempotent for expired/unknown credentials and never grants read
	// or command authority through a successful empty response.
	if path == "auth/logout" && r.Method == http.MethodPost {
		target, err := s.Service.ResolveLogoutTarget(ctx, tokenOf(r))
		if err == nil {
			err = s.Service.Logout(ctx, target)
		}
		if err != nil {
			writeError(w, err)
			return
		}
		cookie(w, r, "")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	actor, err := s.Service.AuthenticateActor(ctx, tokenOf(r))
	if err != nil {
		writeError(w, err)
		return
	}
	p := actor.Projection()
	scope, boundProject := "", actor.credential.projectID
	switch actor.kind {
	case actorProjectAPI:
		scope = "api"
	case actorProjectRunner:
		scope = "runner"
	case actorAgentRead:
		scope = "agent"
	}
	if actor.IsBrowser() && BrowserCommandRequiresView(r.Method, path) {
		if err := validateOriginalView(r, p, path == "account/preferences"); err != nil {
			writeError(w, err)
			return
		}
	}
	s.applyLocalePreferences(w, r, actor)
	if s.localePreferencesHTTP(w, r, actor, path) {
		return
	}
	if scope != "" && strings.HasPrefix(path, "auth/") && path != "auth/logout" {
		writeError(w, forbidden())
		return
	}
	if scope != "" && path == "projects" {
		if scope != "api" || r.Method != "GET" {
			writeError(w, forbidden())
			return
		}
		items, e := s.Service.Projects(ctx, actor)
		if e != nil {
			writeError(w, e)
			return
		}
		filtered := []Record{}
		for _, item := range items {
			if item.ID == boundProject {
				filtered = append(filtered, item)
			}
		}
		writeJSON(w, 200, map[string]any{"items": filtered, "total": len(filtered)})
		return
	}
	if path == "auth/session" && r.Method == "GET" {
		s.session(w, r, actor)
		return
	}
	if s.executionPoolsHTTP(w, r, actor, scope, path, parts) {
		return
	}
	if path == "auth/switch" && r.Method == "POST" {
		m, e := decode(r)
		if e != nil {
			writeError(w, e)
			return
		}
		_, token, e := s.Service.SwitchOrganization(ctx, actor, m)
		if e != nil {
			writeError(w, e)
			return
		}
		cookie(w, r, token)
		s.sessionForToken(w, r, token)
		return
	}
	if path == "organizations" || parts[0] == "organization" || parts[0] == "teams" || parts[0] == "project-access" {
		if scope != "" {
			writeError(w, forbidden())
			return
		}
		var m map[string]any
		if r.Method == "POST" || r.Method == "PATCH" {
			m, err = decode(r)
			if err != nil {
				writeError(w, err)
				return
			}
		}
		if !s.governance(w, r, actor, path, m) {
			writeError(w, missing())
		}
		return
	}
	if path == "auth/tokens" && r.Method == http.MethodPost {
		m, e := decode(r)
		if e != nil {
			writeError(w, e)
			return
		}
		for key := range m {
			if key != "project_id" && key != "scope" && key != "label" && key != "idempotency_key" {
				writeError(w, invalid("无效凭证范围"))
				return
			}
		}
		scope := str(m, "scope")
		if scope == "" {
			scope = "runner"
		}
		out, e := s.Service.IssueProjectCredential(ctx, actor, ProjectCredentialIssueRequest{
			ProjectID: str(m, "project_id"), Scope: scope, Label: str(m, "label"), IdempotencyKey: str(m, "idempotency_key"),
		})
		respond(w, http.StatusCreated, out, e)
		return
	}
	if path == "projects" {
		if r.Method == "GET" {
			items, e := s.Service.Projects(ctx, actor)
			respondList(w, items, e)
			return
		}
		if r.Method == "POST" {
			m, e := decode(r)
			if e != nil {
				writeError(w, e)
				return
			}
			item, e := s.Service.CreateProject(ctx, actor, m)
			respond(w, 201, item, e)
			return
		}
	}
	project := r.URL.Query().Get("project_id")
	var m map[string]any
	if r.Method == "POST" || r.Method == "PATCH" || r.Method == http.MethodPut && len(parts) == 3 && parts[0] == "requirements" && parts[2] == "design" {
		if path == "runner/progress" {
			m, err = decodeRunnerProgressEnvelope(r)
		} else {
			m, err = decode(r)
		}
		if err != nil {
			writeError(w, err)
			return
		}
		if v := str(m, "project_id"); v != "" {
			project = v
		}
	}
	if kind := scope; kind != "" {
		if kind != "runner" && kind != "agent" && kind != "api" {
			writeError(w, forbidden())
			return
		}
		if boundProject != project {
			writeError(w, forbidden())
			return
		}
		if kind == "runner" && parts[0] != "runner" {
			writeError(w, forbidden())
			return
		}
		if kind == "agent" && parts[0] != "agent" {
			writeError(w, forbidden())
			return
		}
	}
	// This read has a strict query contract. Validate after authentication and
	// token boundaries, before the common lookup reports malformed scope as missing.
	if r.Method == "GET" && len(parts) == 7 && parts[0] == "runs" && parts[2] == "nodes" && parts[4] == "attempts" && parts[6] == "tool-events" {
		if _, _, _, _, queryErr := attemptHistoryQuery(r, parts[5]); queryErr != nil {
			writeError(w, queryErr)
			return
		}
	}
	if s.taskDesignViewHTTP(w, r, actor, project, parts) {
		return
	}
	if path == "developer-connection" && r.Method == http.MethodGet {
		out, err := s.DeveloperConnection(ctx, actor, project)
		respond(w, http.StatusOK, out, err)
		return
	}
	if s.requirementDesignHTTP(w, r, actor, project, path, parts, m) {
		return
	}
	if s.skillCatalogHTTP(w, r, actor, project, parts, m) {
		return
	}
	if s.projectKnowledgeHTTP(w, r, actor, project, path, parts, m) {
		return
	}
	if s.codeInputHTTP(w, r, actor, project, path, m) {
		return
	}
	if s.candidateTransferHTTP(w, r, actor, project, path, m) {
		return
	}
	if s.artifactHTTP(w, r, actor, project, path, m) {
		return
	}
	if s.toolProfilesHTTP(w, r, actor, scope, path, parts, project, m) {
		return
	}
	if s.taskDispatchHTTP(w, r, actor, scope, path, parts, project, m) {
		return
	}
	if s.taskHandoffHTTP(w, r, actor, scope, parts, project, m) {
		return
	}
	if s.schedulingHTTP(w, r, actor, scope, path, parts, project, m) {
		return
	}
	if s.releaseDeploymentHTTP(w, r, actor, project, path, m) {
		return
	}
	if s.usageAccountingHTTP(w, r, actor, scope, parts, project, m) {
		return
	}
	if s.repositoryIntegrationHTTP(w, r, actor, scope, project, parts, m) {
		return
	}
	if s.expertTeamHTTP(w, r, actor, scope, project, parts, m) {
		return
	}
	if s.knowledgeIntelligenceHTTP(w, r, actor, scope, project, parts, m) {
		return
	}
	if s.candidateCompositionHTTP(w, r, actor, scope, project, parts, m) {
		return
	}
	if s.failureDiagnosisHTTP(w, r, actor, scope, project, parts, m) {
		return
	}
	if s.intelligentPlanningHTTP(w, r, actor, scope, parts, project, m) {
		return
	}
	if s.planning(w, r, actor, project, path, m) {
		return
	}
	if s.automationIngressHTTP(w, r, actor, scope, path, parts, project, m) {
		return
	}
	if s.automationHTTP(w, r, actor, scope, path, parts, project, m) {
		return
	}
	switch {
	case parts[0] == "workspace":
		s.workspace(w, r, actor, project, path, m)
	case path == "overview" && r.Method == "GET":
		// Filesystem/health probes finish before taking the bounded SQL proof.
		capabilities := s.capabilities(ctx)
		s.projectReadJSON(w, r, actor, project, func(ctx context.Context, auth *authorizedCommand) (any, error) {
			out, err := s.Service.overviewTx(ctx, auth, project)
			if err != nil {
				return nil, err
			}
			runs, total, _, err := s.Engine.ListRunsPageTx(ctx, auth.tx, auth.currentPrincipal.TenantID, project, 10, "")
			if err != nil {
				return nil, err
			}
			out["runs"] = runs
			out["counts"].(map[string]int)["runs"] = total
			out["run_history_limit"] = 10
			out["capabilities"] = capabilities
			status, err := s.Service.indexStatusTx(ctx, auth, project)
			if err != nil {
				return nil, err
			}
			out["index_status"] = status
			return out, nil
		})
	case path == "capabilities" && r.Method == "GET":
		capabilities := s.capabilities(ctx)
		s.projectReadJSON(w, r, actor, project, func(context.Context, *authorizedCommand) (any, error) { return capabilities, nil })
	case path == "search" && r.Method == "POST":
		out, e := s.Service.SearchIndex(ctx, actor, project, str(m, "query"))
		respond(w, 200, out, e)
	case len(parts) > 0 && tables[parts[0]]:
		kind := parts[0]
		if len(parts) == 1 && r.Method == "GET" {
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			items, total, next, e := s.Service.ListPage(ctx, actor, project, kind, limit, r.URL.Query().Get("cursor"))
			if e != nil {
				writeError(w, e)
				return
			}
			writeJSON(w, 200, map[string]any{"items": items, "total": total, "next_cursor": next})
			return
		}
		if len(parts) == 1 && r.Method == "POST" {
			item, e := s.Service.Create(ctx, actor, project, kind, m)
			respond(w, 201, item, e)
			return
		}
		if len(parts) == 2 && r.Method == "GET" {
			item, e := s.Service.Get(ctx, actor, project, kind, parts[1])
			respond(w, 200, item, e)
			return
		}
		if len(parts) == 2 && r.Method == "PATCH" {
			item, e := s.Service.Update(ctx, actor, project, kind, parts[1], m)
			respond(w, 200, item, e)
			return
		}
		if len(parts) == 3 && parts[0] == "requirements" && parts[2] == "decompose" && r.Method == "POST" {
			out, e := s.Service.Decompose(ctx, actor, project, parts[1], m)
			respondList(w, out, e)
			return
		}
		if len(parts) == 3 && parts[0] == "tasks" && parts[2] == "review" && r.Method == "POST" {
			out, e := s.Service.Review(ctx, actor, project, parts[1], m, s.Engine)
			respond(w, 200, out, e)
			return
		}
		if len(parts) == 3 && parts[0] == "issues" && parts[2] == "intake" && r.Method == "POST" {
			out, e := s.Service.IssueToRequirement(ctx, actor, project, parts[1], m)
			respond(w, 201, out, e)
			return
		}
		if len(parts) == 3 && parts[0] == "issues" && parts[2] == "intake" && r.Method == "GET" {
			if e := validateFailureIntakeReadQuery(r, project); e != nil {
				writeError(w, e)
				return
			}
			out, e := s.Service.IssueRequirement(ctx, actor, project, parts[1])
			respond(w, 200, out, e)
			return
		}
		writeError(w, missing())
	case parts[0] == "workflows":
		if len(parts) == 1 && r.Method == "GET" {
			s.projectReadJSON(w, r, actor, project, func(ctx context.Context, auth *authorizedCommand) (any, error) {
				items, e := s.Engine.WorkflowsTx(ctx, auth.tx, auth.target.tenantID, project)
				return readListEnvelope(items), e
			})
			return
		}
		if r.Method == "POST" {
			raw := m["workflow"]
			if raw == nil {
				raw = m
			}
			var workflow dag.Workflow
			if e := json.Unmarshal([]byte(stringify(raw)), &workflow); e != nil {
				writeError(w, invalid("流程格式错误"))
				return
			}
			if len(parts) == 2 && parts[1] == "validate" {
				validation := workflowValidation(w, workflow)
				s.projectReadJSON(w, r, actor, project, func(context.Context, *authorizedCommand) (any, error) { return validation, nil })
				return
			}
			if len(parts) == 1 {
				if len(str(m, "idempotency_key")) < 8 {
					writeError(w, invalid("缺少有效操作标识"))
					return
				}
				authority := s.Service.engineAuthority(ctx, actor, commandTarget{tenantID: p.TenantID, projectID: project, resourceID: workflow.ID}, purposeProjectWrite)
				defer authority.close()
				item, e := s.Engine.PublishWithAuthority(authority.ctx, p.TenantID, project, workflow, p.ID+":"+str(m, "idempotency_key"), authority.before)
				respond(w, 201, item, e)
				return
			}
		}
		if len(parts) == 2 && r.Method == "GET" {
			version, _ := strconv.Atoi(r.URL.Query().Get("version"))
			s.projectReadJSON(w, r, actor, project, func(ctx context.Context, auth *authorizedCommand) (any, error) {
				return s.Engine.GetWorkflowTx(ctx, auth.tx, auth.target.tenantID, project, parts[1], version)
			})
			return
		}
		writeError(w, missing())
	case parts[0] == "runs":
		if len(parts) == 1 && r.Method == "GET" {
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			s.projectReadJSON(w, r, actor, project, func(ctx context.Context, auth *authorizedCommand) (any, error) {
				items, total, next, e := s.Engine.ListRunsPageTx(ctx, auth.tx, auth.target.tenantID, project, limit, r.URL.Query().Get("cursor"))
				return map[string]any{"items": items, "total": total, "next_cursor": next}, e
			})
			return
		}
		if len(parts) == 1 && r.Method == "POST" {
			expected := number(m, "expected_version")
			if str(m, "task_id") != "" {
				task, e := s.Service.Get(ctx, actor, project, "tasks", str(m, "task_id"))
				if e != nil {
					writeError(w, e)
					return
				}
				if expected == 0 {
					expected = task.Version
				}
			}
			if len(str(m, "idempotency_key")) < 8 {
				writeError(w, invalid("缺少有效操作标识"))
				return
			}
			bindingKey := str(m, "task_id") + ":" + stringify(m)
			authority := s.Service.engineAuthority(ctx, actor, commandTarget{tenantID: p.TenantID, projectID: project, resourceID: str(m, "task_id")}, purposeProjectWrite)
			defer authority.close()
			item, e := s.Engine.CreateRunBoundAuthorized(authority.ctx, p.TenantID, project, str(m, "workflow_id"), number(m, "workflow_version"), p.ID+":"+str(m, "idempotency_key"), bindingKey, authority.before, func(tx *sql.Tx, runID string) error {
				return s.Service.BindDirectRunTx(authority.ctx, tx, authority.proof, project, str(m, "task_id"), runID, expected)
			})
			respond(w, 201, item, e)
			return
		}
		if len(parts) == 2 && r.Method == "GET" {
			s.projectReadJSON(w, r, actor, project, func(ctx context.Context, auth *authorizedCommand) (any, error) {
				return s.Engine.GetRunTx(ctx, auth.tx, auth.target.tenantID, project, parts[1])
			})
			return
		}
		if len(parts) == 4 && parts[2] == "events" && parts[3] == "stream" && r.Method == "GET" {
			s.runEventStream(w, r, actor, project, parts[1])
			return
		}
		if len(parts) == 3 && parts[2] == "events" && r.Method == "GET" {
			after, e := runStreamCursor(r)
			if e != nil {
				writeError(w, e)
				return
			}
			items, e := s.runStreamRead(ctx, actor, project, parts[1], after, true)
			if e != nil {
				writeError(w, e)
				return
			}
			next := after
			if len(items) > 0 {
				next = items[len(items)-1].Sequence
			}
			writeJSON(w, 200, map[string]any{"items": items, "next_cursor": next})
			return
		}
		if len(parts) == 3 && parts[2] == "commands" && r.Method == "POST" {
			managed, e := s.Engine.ManagedRun(ctx, p.TenantID, project, parts[1])
			if e != nil {
				writeError(w, e)
				return
			}
			payload, _ := m["payload"].(map[string]any)
			purpose := purposeProjectWrite
			if strings.Contains(path, "/commands") && (str(m, "action") == "approve" || str(m, "action") == "approval" || str(m, "action") == "reconcile" || str(m, "action") == "compensate") {
				purpose = purposeProjectApprove
			}
			authority := s.Service.engineAuthority(ctx, actor, commandTarget{tenantID: p.TenantID, projectID: project, resourceID: parts[1]}, purpose)
			defer authority.close()
			out, e := s.Engine.CommandWithAuthority(authority.ctx, p.TenantID, project, parts[1], str(m, "action"), str(m, "idempotency_key"), number(m, "expected_version"), payload, func(tx *sql.Tx) error {
				if err := authority.before(tx); err != nil {
					return err
				}
				if managed {
					task, err := s.Service.TaskDispatchRunTx(ctx, tx, p.TenantID, project, parts[1])
					if err != nil {
						return err
					}
					if !task {
						return invalid("发布执行必须从发布计划核对或恢复，不能使用通用运行命令")
					}
				}
				return nil
			}, func(tx *sql.Tx, run dag.Run) error {
				if str(m, "action") == "compensate" {
					return s.Service.InvalidateCompensationTx(authority.ctx, tx, authority.proof, project, run.ID)
				}
				return nil
			})
			respond(w, 200, out, e)
			return
		}
		writeError(w, missing())
	case parts[0] == "runner":
		s.runner(w, r, actor, project, path, m)
	case parts[0] == "agent":
		s.agent(w, r, actor, project, path, m)
	default:
		writeError(w, missing())
	}
}
func (s *Server) sessionForToken(w http.ResponseWriter, r *http.Request, token string) {
	actor, err := s.Service.AuthenticateActor(r.Context(), token)
	if err != nil {
		writeError(w, err)
		return
	}
	s.applyLocalePreferences(w, r, actor)
	s.session(w, r, actor)
}

func (s *Server) session(w http.ResponseWriter, r *http.Request, actor Actor) {
	var envelope map[string]any
	err := s.Service.withCurrentRead(r.Context(), actor, commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}, purposeOrganizationRead, func(auth *authorizedCommand) error {
		ctx, cancel := context.WithDeadline(r.Context(), auth.deadline)
		defer cancel()
		projects, err := s.Service.identityProjectsTx(ctx, auth)
		if err != nil {
			return err
		}
		var name string
		if err = auth.tx.QueryRowContext(ctx, s.Service.q(`SELECT name FROM suma_organizations WHERE id=?`), auth.target.tenantID).Scan(&name); err != nil {
			return err
		}
		envelope = map[string]any{"user": auth.currentPrincipal, "organization": map[string]any{"id": auth.target.tenantID, "name": name}, "projects": projects, "setup_required": false}
		return nil
	})
	respond(w, http.StatusOK, envelope, err)
}
func respond(w http.ResponseWriter, status int, out any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, status, out)
}
func respondList[T any](w http.ResponseWriter, items []T, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	if items == nil {
		items = []T{}
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": len(items)})
}
func (s *Server) runner(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, m map[string]any) {
	p := actor.Projection()
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		writeError(w, &APIError{405, "method_not_allowed", "执行节点请求必须使用 POST", nil})
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) == "" {
		writeError(w, &APIError{401, "runner_token_required", "执行节点需要独立 Bearer 凭证", nil})
		return
	}
	if actor.kind != actorProjectRunner || actor.credential.projectID != project {
		writeError(w, forbidden())
		return
	}
	switch path {
	case "runner/tool-inventory":
		var request ToolInventoryRequest
		if err := strictAutomation(m, &request); err != nil {
			writeError(w, err)
			return
		}
		out, err := s.Service.ReportToolInventory(r.Context(), actor, project, request)
		respond(w, 200, out, err)
	case "runner/progress":
		var assignment dag.Assignment
		if e := json.Unmarshal([]byte(stringify(m["assignment"])), &assignment); e != nil {
			writeError(w, invalid("执行进度身份无效"))
			return
		}
		event, err := decodeRunnerProgressEvent(m["tool_event"])
		if err != nil {
			writeError(w, err)
			return
		}
		authority := s.Service.engineAuthority(r.Context(), actor, commandTarget{tenantID: p.TenantID, projectID: project, resourceID: assignment.WorkerID}, purposeRunnerWrite)
		defer authority.close()
		e := s.Engine.ProgressWithAuthority(authority.ctx, p.TenantID, project, assignment, str(m, "operation_id"), str(m, "message"), event, func(tx *sql.Tx) error {
			if err := authority.beforeRunner(tx, assignment.WorkerID); err != nil {
				return err
			}
			if err := s.Service.projectRunnerExecutionAuthorityTx(authority.ctx, tx, authority.proof, assignment.WorkerID, assignment.RunID); err != nil {
				return err
			}
			return s.Service.validateToolProgressTx(authority.ctx, tx, authority.proof, assignment, event)
		})
		respond(w, 200, map[string]bool{"acknowledged": e == nil}, e)
	case "runner/repository-integrations/authorize":
		var req ri.RepositoryAuthorizationRequest
		if err := repositoryDecodeBody(m, &req); err != nil {
			writeError(w, err)
			return
		}
		if req.ProjectID != project {
			writeError(w, forbidden())
			return
		}
		out, err := s.Service.AuthorizeRepositoryIntegration(r.Context(), actor, req)
		respond(w, 200, out, err)
	case "runner/claim":
		var claim RunnerClaimRequest
		if err := strictAutomation(m, &claim); err != nil {
			writeError(w, err)
			return
		}
		if len(claim.IdempotencyKey) < 8 || len(claim.IdempotencyKey) > 128 {
			writeError(w, invalid("领取任务必须提供 8–128 字符的稳定 idempotency_key"))
			return
		}
		if claim.ProjectID != project || claim.ProjectID == "" || m["capabilities"] == nil {
			writeError(w, invalid("领取任务必须提供精确项目及实际能力列表"))
			return
		}
		authority := s.Service.engineAuthority(r.Context(), actor, commandTarget{tenantID: p.TenantID, projectID: project, resourceID: claim.WorkerID}, purposeRunnerWrite)
		defer authority.close()
		a, e := s.Engine.ClaimWithOperation(authority.ctx, p.TenantID, project, claim.WorkerID, claim.IdempotencyKey, claim.Capabilities, func(tx *sql.Tx) error {
			return authority.beforeRunner(tx, claim.WorkerID)
		}, func(tx *sql.Tx, run, node string) (bool, error) {
			allowed, err := s.Service.releaseClaimEligibleTx(authority.ctx, tx, authority.proof, claim.WorkerID, run)
			if err != nil || !allowed {
				return allowed, err
			}
			if err := s.Service.checkPlanningExecutionTx(authority.ctx, tx, p.TenantID, project, run); err != nil {
				var apiErr *APIError
				if automationAuthorityFailure(err) || errors.As(err, &apiErr) && apiErr.Code == "planning_basis_changed" {
					return false, nil
				}
				return false, err
			}
			if err := s.Service.checkKnowledgeIntelligenceExecutionTx(authority.ctx, tx, p.TenantID, project, run, true); err != nil {
				var apiErr *APIError
				if automationAuthorityFailure(err) || errors.As(err, &apiErr) && apiErr.Code == "knowledge_intelligence_basis_changed" {
					return false, nil
				}
				return false, err
			}
			if err := s.Service.checkFailureDiagnosisExecutionTx(authority.ctx, tx, p.TenantID, project, run, true); err != nil {
				var apiErr *APIError
				if automationAuthorityFailure(err) || errors.As(err, &apiErr) && apiErr.Code == "failure_diagnosis_basis_changed" {
					return false, nil
				}
				return false, err
			}
			if err := s.Service.checkRepositoryIntegrationExecutionTx(authority.ctx, tx, p.TenantID, project, run); err != nil {
				var apiErr *APIError
				if automationAuthorityFailure(err) || errors.As(err, &apiErr) && strings.HasPrefix(apiErr.Code, "repository_integration_") {
					return false, nil
				}
				return false, err
			}
			if err := s.Service.checkCodeInputExecutionTx(authority.ctx, tx, p.TenantID, project, run, node, true); err != nil {
				var api *APIError
				if automationAuthorityFailure(err) || errors.As(err, &api) && strings.HasPrefix(api.Code, "candidate_composition_") {
					return false, nil
				}
				return false, err
			}
			return s.Service.toolClaimEligibleTx(authority.ctx, tx, authority.proof, claim.WorkerID, run, node)
		})
		if e == nil && a != nil && a.DispatchState == "active" {
			a.ReleaseExecution, e = s.Service.ReleaseExecutionForAssignment(r.Context(), actor, project, *a)
			if e == nil {
				e = s.Service.decorateRepositoryAssignment(r.Context(), actor, project, a)
			}
			if e == nil {
				e = s.Service.decorateCodeInputAssignment(r.Context(), projectCandidateSubject(actor, project), a)
			}
		}
		respond(w, 200, RunnerClaimResponse{Assignment: a}, e)
	case "runner/heartbeat":
		var a dag.Assignment
		if json.Unmarshal([]byte(stringify(m["assignment"])), &a) != nil {
			writeError(w, invalid("无效执行信封"))
			return
		}
		authority := s.Service.engineAuthority(r.Context(), actor, commandTarget{tenantID: p.TenantID, projectID: project, resourceID: a.WorkerID}, purposeRunnerWrite)
		defer authority.close()
		e := s.Engine.HeartbeatWithAuthority(authority.ctx, p.TenantID, project, a, func(tx *sql.Tx) error {
			if err := authority.beforeRunner(tx, a.WorkerID); err != nil {
				return err
			}
			if err := s.Service.projectRunnerExecutionAuthorityTx(authority.ctx, tx, authority.proof, a.WorkerID, a.RunID); err != nil {
				return err
			}
			if err := s.Service.checkPlanningExecutionTx(authority.ctx, tx, p.TenantID, project, a.RunID); err != nil {
				return err
			}
			if err := s.Service.checkKnowledgeIntelligenceExecutionTx(authority.ctx, tx, p.TenantID, project, a.RunID, false); err != nil {
				return err
			}
			if err := s.Service.checkFailureDiagnosisExecutionTx(authority.ctx, tx, p.TenantID, project, a.RunID, false); err != nil {
				return err
			}
			if err := s.Service.checkCodeInputExecutionTx(authority.ctx, tx, p.TenantID, project, a.RunID, a.NodeID, false); err != nil {
				return err
			}
			return s.Service.checkRepositoryIntegrationExecutionTx(authority.ctx, tx, p.TenantID, project, a.RunID)
		})
		if errors.Is(e, dag.ErrCancelled) {
			writeJSON(w, 200, map[string]any{"cancel_requested": true})
			return
		}
		respond(w, 200, map[string]any{"cancel_requested": false}, e)
	case "runner/results":
		var result dag.Result
		if json.Unmarshal([]byte(stringify(m["result"])), &result) != nil {
			writeError(w, invalid("无效执行回执"))
			return
		}
		authority := s.Service.engineAuthority(r.Context(), actor, commandTarget{tenantID: p.TenantID, projectID: project, resourceID: result.WorkerID}, purposeRunnerWrite)
		defer authority.close()
		e := s.Engine.CompleteBound(authority.ctx, p.TenantID, project, result, func(tx *sql.Tx) error {
			if err := authority.beforeRunner(tx, result.WorkerID); err != nil {
				return err
			}
			return s.Service.projectRunnerExecutionAuthorityTx(authority.ctx, tx, authority.proof, result.WorkerID, result.RunID)
		}, func(tx *sql.Tx, state string) error {
			if err := s.Service.validateToolResultTx(authority.ctx, tx, authority.proof, result); err != nil {
				return err
			}
			if err := s.Service.verifyArtifactResultTx(authority.ctx, tx, authority.proof, result, state); err != nil {
				return err
			}
			if err := s.Service.ValidateReleaseResultTx(authority.ctx, tx, authority.proof, result, state); err != nil {
				return err
			}
			authorize := func(current *sql.Tx) error { return authority.beforeRunner(current, result.WorkerID) }
			if err := s.Service.recordPlanningOutcomeTx(authority.ctx, tx, p.TenantID, project, result, authorize); err != nil {
				return err
			}
			if err := s.Service.recordKnowledgeIntelligenceOutcomeTx(authority.ctx, tx, p.TenantID, project, result, authorize); err != nil {
				return err
			}
			if err := s.Service.recordFailureDiagnosisOutcomeTx(authority.ctx, tx, p.TenantID, project, result, authorize); err != nil {
				return err
			}
			if err := s.Service.validateCandidateCompositionResultTx(authority.ctx, tx, p.TenantID, project, result); err != nil {
				return err
			}
			if err := s.Service.validateCodeBaselineResultTx(authority.ctx, tx, p.TenantID, project, result); err != nil {
				return err
			}
			if err := s.Service.validateRepositoryIntegrationResultTx(authority.ctx, tx, p.TenantID, project, result); err != nil {
				return err
			}
			return s.Service.recordRepositoryIntegrationOutcomeTx(authority.ctx, tx, p.TenantID, project, result)
		})
		if e == nil {
			run, re := s.Engine.GetRun(r.Context(), p.TenantID, project, result.RunID)
			if re == nil {
				if ie := s.Service.FailureIssue(r.Context(), p.TenantID, project, run); ie != nil {
					slog.Error("failure issue pending", "run_id", run.ID, "error", ie)
				}
			}
		}
		respond(w, 200, map[string]bool{"acknowledged": e == nil}, e)
	default:
		writeError(w, missing())
	}
}
func (s *Server) capabilities(ctx context.Context) map[string]any {
	connected := s.Service.DB.PingContext(ctx) == nil
	var seq int
	dagAvailable := s.Service.DB.QueryRowContext(ctx, `SELECT seq FROM dag_lock WHERE id=1`).Scan(&seq) == nil
	info, fileErr := os.Stat(s.Service.DataDir)
	filesAvailable := fileErr == nil && info.IsDir()
	items := []map[string]any{{"id": "transactional_store", "name": "业务事务库", "configured": true, "available": connected, "verified": false, "backend": s.Service.Dialect, "checks": map[string]any{"connection": connected, "transaction": "not_run", "backup_restore": "not_run"}, "reason": "仅实时探测连接；事务、迁移和恢复验收见交付报告"}, {"id": "dag", "name": "持久化 DAG", "configured": true, "available": dagAvailable, "verified": false, "reason": "已探测调度表；完整运行验收见交付证据"}, {"id": "files", "name": "本地文件", "configured": true, "available": filesAvailable, "verified": false, "reason": "目录存在性探测，不代表写入、备份或恢复已验证"}, {"id": "semantic_search", "name": "多模态语义检索", "configured": os.Getenv("SUMA_KNOWLEDGE_URL") != "", "available": false, "verified": false, "reason": "需要连接检索服务并执行读写与检索验证"}, {"id": "payments", "name": "支付结算", "configured": false, "available": false, "verified": false, "reason": "尚未配置商户与支付适配器"}, {"id": "collaboration", "name": "多人实时协作", "configured": false, "available": false, "verified": false, "reason": "协作服务尚未部署"}}
	return map[string]any{"items": items, "as_of": now()}
}

// The native read-only adapter projects the shared application catalogue without
// broadening its legacy four-tool contract or routing writes through a POST query.
func (s *Server) agent(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, m map[string]any) {
	if path == "agent/tools" && r.Method == http.MethodGet {
		catalogue, err := s.Service.ApplicationTools(r.Context(), actor, project)
		if err != nil {
			writeError(w, err)
			return
		}
		items := []map[string]any{}
		for _, definition := range catalogue.Items {
			if !legacyApplicationTool(definition.Name) {
				continue
			}
			items = append(items, map[string]any{"name": definition.Name, "description": presentationSource(w, definition.Description), "parameters": definition.Parameters})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}
	if path == "agent/tools/invoke" && r.Method == http.MethodPost {
		name := str(m, "tool")
		if !legacyApplicationTool(name) {
			writeError(w, &APIError{422, "tool_not_allowed", "该工具尚未授权或未实现", nil})
			return
		}
		raw, err := json.Marshal(m["arguments"])
		if err != nil {
			writeError(w, invalid("工具参数必须是有效的 JSON 对象"))
			return
		}
		out, err := s.Service.InvokeApplicationTool(r.Context(), actor, ApplicationToolCall{ProjectID: project, Tool: name, Arguments: raw})
		respond(w, http.StatusOK, out, err)
		return
	}
	writeError(w, missing())
}

func legacyApplicationTool(name string) bool {
	switch name {
	case "project.overview", "project.tasks", "project.search", "project.knowledge":
		return true
	}
	return false
}

func (s *Server) web(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.NotFound(w, r)
		return
	}
	root, err := filepath.Abs(s.WebDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	requested := filepath.Clean(filepath.Join(root, filepath.FromSlash(r.URL.Path)))
	relative, err := filepath.Rel(root, requested)
	if err != nil || strings.HasPrefix(relative, "..") {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(requested)
	if err == nil && !info.IsDir() {
		http.ServeFile(w, r, requested)
		return
	}
	index := filepath.Join(root, "index.html")
	if _, err = os.Stat(index); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<!doctype html><meta charset=utf-8><title>速码</title><h1>速码控制面已启动</h1><p>请先构建浏览器界面，或启动前端开发服务。</p>")
		return
	}
	http.ServeFile(w, r, index)
}
