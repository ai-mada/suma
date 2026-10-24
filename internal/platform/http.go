package platform

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/releasecontract"
	"github.com/ai-mada/suma/internal/toolcontract"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	Service        *Service
	Engine         *dag.Engine
	WebDir         string
	AllowedOrigins []string
	SetupToken     string
	mux            *http.ServeMux
}

func (s *Server) Handler() http.Handler {
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Service.DB.PingContext(r.Context()); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"status": "ok", "database": "connected", "version": "0.1.0"})
	})
	s.mux.HandleFunc("/api/v1/", s.api)
	s.mux.HandleFunc("/", s.web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		bodyLimit := int64(2 << 20)
		if r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/api/v1/runner/artifact-uploads/") && strings.Contains(r.URL.Path, "/blobs/") {
			bodyLimit = releasecontract.MaxFileBytes + 1
		}
		r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
		s.mux.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
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
	writeJSON(w, e.Status, map[string]any{"error": map[string]any{"code": e.Code, "message": e.Message, "details": e.Details}, "setup_required": e.Code == "unauthenticated" && e.Details != nil})
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
	if s.releaseProviderHTTP(w, r, path) {
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
		p, token, err := s.Service.Setup(ctx, m)
		if err != nil {
			writeError(w, err)
			return
		}
		cookie(w, r, token)
		s.session(w, r, p)
		return
	}
	if path == "auth/login" && r.Method == "POST" {
		m, err := decode(r)
		if err != nil {
			writeError(w, err)
			return
		}
		p, token, err := s.Service.Login(ctx, str(m, "email"), str(m, "password"))
		if err != nil {
			writeError(w, err)
			return
		}
		cookie(w, r, token)
		s.session(w, r, p)
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
		p, token, e := s.Service.AcceptInvitation(ctx, m)
		if e != nil {
			writeError(w, e)
			return
		}
		cookie(w, r, token)
		s.session(w, r, p)
		return
	}
	p, err := s.Service.Authenticate(ctx, tokenOf(r))
	if err != nil {
		writeError(w, err)
		return
	}
	scope, boundProject := s.Service.TokenScope(ctx, tokenOf(r))
	if scope != "" && strings.HasPrefix(path, "auth/") && path != "auth/logout" {
		writeError(w, forbidden())
		return
	}
	if scope != "" && path == "projects" {
		if scope != "api" || r.Method != "GET" {
			writeError(w, forbidden())
			return
		}
		if err = s.Service.Authorize(ctx, p, boundProject, false, false); err != nil {
			writeError(w, err)
			return
		}
		items, e := s.Service.Projects(ctx, p)
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
		s.session(w, r, p)
		return
	}
	if path == "auth/switch" && r.Method == "POST" {
		m, e := decode(r)
		if e != nil {
			writeError(w, e)
			return
		}
		next, token, e := s.Service.SwitchOrganization(ctx, p, m)
		if e != nil {
			writeError(w, e)
			return
		}
		cookie(w, r, token)
		s.session(w, r, next)
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
		if !s.governance(w, r, p, path, m) {
			writeError(w, missing())
		}
		return
	}
	if path == "auth/logout" && r.Method == "POST" {
		if err = s.Service.Logout(ctx, tokenOf(r)); err != nil {
			writeError(w, err)
			return
		}
		cookie(w, r, "")
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if path == "auth/tokens" && r.Method == "POST" {
		if p.Role != "owner" && p.Role != "admin" {
			writeError(w, forbidden())
			return
		}
		if kind, _ := s.Service.TokenScope(ctx, tokenOf(r)); kind != "" {
			writeError(w, forbidden())
			return
		}
		m, e := decode(r)
		if e != nil {
			writeError(w, e)
			return
		}
		label := str(m, "label")
		if label == "" {
			label = "Runner"
		}
		if len(label) > 200 {
			writeError(w, invalid("凭证名称过长"))
			return
		}
		kind := str(m, "scope")
		if kind == "" {
			kind = "runner"
		}
		if kind != "runner" && kind != "agent" && kind != "api" {
			writeError(w, invalid("无效凭证范围"))
			return
		}
		tokenProject := str(m, "project_id")
		if e = s.Service.Authorize(ctx, p, tokenProject, true, false); e != nil {
			writeError(w, e)
			return
		}
		token, e := s.Service.CreateToken(ctx, p, label, 24*time.Hour)
		if e != nil {
			writeError(w, e)
			return
		}
		if _, e = s.Service.DB.ExecContext(ctx, s.Service.q(`INSERT INTO suma_token_scopes(token_hash,kind,project_id)VALUES(?,?,?)`), digest(token), kind, tokenProject); e != nil {
			s.Service.Logout(ctx, token)
			writeError(w, e)
			return
		}
		writeJSON(w, 201, map[string]any{"token": token, "expires_at": time.Now().UTC().Add(24 * time.Hour), "label": label, "scope": kind, "project_id": tokenProject})
		return
	}
	if path == "projects" {
		if r.Method == "GET" {
			items, e := s.Service.Projects(ctx, p)
			respondList(w, items, e)
			return
		}
		if r.Method == "POST" {
			m, e := decode(r)
			if e != nil {
				writeError(w, e)
				return
			}
			item, e := s.Service.CreateProject(ctx, p, m)
			respond(w, 201, item, e)
			return
		}
	}
	project := r.URL.Query().Get("project_id")
	var m map[string]any
	if r.Method == "POST" || r.Method == "PATCH" {
		m, err = decode(r)
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
	write := r.Method != "GET"
	approve := strings.Contains(path, "/review") || (strings.Contains(path, "/commands") && (str(m, "action") == "approve" || str(m, "action") == "approval" || str(m, "action") == "reconcile" || str(m, "action") == "compensate"))
	if err = s.Service.Authorize(ctx, p, project, write, approve); err != nil {
		writeError(w, err)
		return
	}
	if s.artifactHTTP(w, r, p, project, path, m) {
		return
	}
	if s.toolProfilesHTTP(w, r, p, scope, path, parts, project, m) {
		return
	}
	if s.taskDispatchHTTP(w, r, p, scope, path, parts, project, m) {
		return
	}
	if s.releaseDeploymentHTTP(w, r, p, project, path, m) {
		return
	}
	if s.planning(w, r, p, project, path, m) {
		return
	}
	if s.automationIngressHTTP(w, r, p, scope, path, parts, project, m) {
		return
	}
	if s.automationHTTP(w, r, p, scope, path, parts, project, m) {
		return
	}
	switch {
	case parts[0] == "workspace":
		s.workspace(w, r, p, project, path, m)
	case path == "overview" && r.Method == "GET":
		out, e := s.Service.Overview(ctx, p, project)
		if e == nil {
			runs, total, _, re := s.Engine.ListRunsPage(ctx, p.TenantID, project, 10, "")
			if re != nil {
				writeError(w, re)
				return
			}
			out["runs"] = runs
			out["counts"].(map[string]int)["runs"] = total
			out["run_history_limit"] = 10
			out["capabilities"] = s.capabilities(ctx)
			indexStatus, ie := s.Service.IndexStatus(ctx, p, project)
			if ie != nil {
				writeError(w, ie)
				return
			}
			out["index_status"] = indexStatus
		}
		respond(w, 200, out, e)
	case path == "capabilities" && r.Method == "GET":
		writeJSON(w, 200, s.capabilities(ctx))
	case path == "search" && r.Method == "POST":
		out, e := s.Service.SearchIndex(ctx, p, project, str(m, "query"))
		respond(w, 200, out, e)
	case len(parts) > 0 && tables[parts[0]]:
		kind := parts[0]
		if len(parts) == 1 && r.Method == "GET" {
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			items, total, next, e := s.Service.ListPage(ctx, p, project, kind, limit, r.URL.Query().Get("cursor"))
			if e != nil {
				writeError(w, e)
				return
			}
			writeJSON(w, 200, map[string]any{"items": items, "total": total, "next_cursor": next})
			return
		}
		if len(parts) == 1 && r.Method == "POST" {
			item, e := s.Service.Create(ctx, p, project, kind, m)
			respond(w, 201, item, e)
			return
		}
		if len(parts) == 2 && r.Method == "GET" {
			item, e := s.Service.Get(ctx, p, project, kind, parts[1])
			respond(w, 200, item, e)
			return
		}
		if len(parts) == 2 && r.Method == "PATCH" {
			item, e := s.Service.Update(ctx, p, project, kind, parts[1], m)
			respond(w, 200, item, e)
			return
		}
		if len(parts) == 3 && parts[0] == "requirements" && parts[2] == "decompose" && r.Method == "POST" {
			out, e := s.Service.Decompose(ctx, p, project, parts[1], m)
			respondList(w, out, e)
			return
		}
		if len(parts) == 3 && parts[0] == "tasks" && parts[2] == "review" && r.Method == "POST" {
			out, e := s.Service.Review(ctx, p, project, parts[1], m, s.Engine)
			respond(w, 200, out, e)
			return
		}
		if len(parts) == 3 && parts[0] == "issues" && parts[2] == "intake" && r.Method == "POST" {
			out, e := s.Service.IssueToRequirement(ctx, p, project, parts[1], m)
			respond(w, 201, out, e)
			return
		}
		writeError(w, missing())
	case parts[0] == "workflows":
		if len(parts) == 1 && r.Method == "GET" {
			items, e := s.Engine.Workflows(ctx, p.TenantID, project)
			respondList(w, items, e)
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
				if e := dag.Compile(workflow); e != nil {
					writeJSON(w, 200, map[string]any{"valid": false, "errors": []string{e.Error()}})
				} else {
					writeJSON(w, 200, map[string]any{"valid": true, "errors": []string{}})
				}
				return
			}
			if len(parts) == 1 {
				if len(str(m, "idempotency_key")) < 8 {
					writeError(w, invalid("缺少有效操作标识"))
					return
				}
				item, e := s.Engine.PublishWithOperation(ctx, p.TenantID, project, workflow, p.ID+":"+str(m, "idempotency_key"))
				respond(w, 201, item, e)
				return
			}
		}
		if len(parts) == 2 && r.Method == "GET" {
			version, _ := strconv.Atoi(r.URL.Query().Get("version"))
			item, e := s.Engine.GetWorkflow(ctx, p.TenantID, project, parts[1], version)
			respond(w, 200, item, e)
			return
		}
		writeError(w, missing())
	case parts[0] == "runs":
		if len(parts) == 1 && r.Method == "GET" {
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			items, total, next, e := s.Engine.ListRunsPage(ctx, p.TenantID, project, limit, r.URL.Query().Get("cursor"))
			if e != nil {
				writeError(w, e)
				return
			}
			writeJSON(w, 200, map[string]any{"items": items, "total": total, "next_cursor": next})
			return
		}
		if len(parts) == 1 && r.Method == "POST" {
			expected := number(m, "expected_version")
			if str(m, "task_id") != "" {
				task, e := s.Service.Get(ctx, p, project, "tasks", str(m, "task_id"))
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
			item, e := s.Engine.CreateRunBoundAuthorized(ctx, p.TenantID, project, str(m, "workflow_id"), number(m, "workflow_version"), p.ID+":"+str(m, "idempotency_key"), bindingKey, func(tx *sql.Tx) error {
				return s.Service.authorizeProjectTx(ctx, tx, p, project, true, false)
			}, func(tx *sql.Tx, runID string) error {
				return s.Service.BindDirectRunTx(ctx, tx, p, project, str(m, "task_id"), runID, expected)
			})
			respond(w, 201, item, e)
			return
		}
		if len(parts) == 2 && r.Method == "GET" {
			item, e := s.Engine.GetRun(ctx, p.TenantID, project, parts[1])
			respond(w, 200, item, e)
			return
		}
		if len(parts) == 4 && parts[2] == "events" && parts[3] == "stream" && r.Method == "GET" {
			s.runEventStream(w, r, p, scope, project, parts[1])
			return
		}
		if len(parts) == 3 && parts[2] == "events" && r.Method == "GET" {
			after, e := runStreamCursor(r)
			if e != nil {
				writeError(w, e)
				return
			}
			items, e := s.runStreamRead(ctx, tokenOf(r), p, scope, project, parts[1], after, true)
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
			out, e := s.Engine.CommandWithAuthority(ctx, p.TenantID, project, parts[1], str(m, "action"), str(m, "idempotency_key"), number(m, "expected_version"), payload, func(tx *sql.Tx) error {
				if err := s.Service.authorizeProjectTx(ctx, tx, p, project, true, approve); err != nil {
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
					return s.Service.InvalidateCompensationTx(ctx, tx, p, project, run.ID)
				}
				return nil
			})
			respond(w, 200, out, e)
			return
		}
		writeError(w, missing())
	case parts[0] == "runner":
		s.runner(w, r, p, project, path, m)
	case parts[0] == "agent":
		s.agent(w, r, p, project, path, m)
	default:
		writeError(w, missing())
	}
}
func (s *Server) session(w http.ResponseWriter, r *http.Request, p Principal) {
	projects, err := s.Service.Projects(r.Context(), p)
	if err != nil {
		writeError(w, err)
		return
	}
	var name string
	if err = s.Service.DB.QueryRowContext(r.Context(), s.Service.q(`SELECT name FROM suma_organizations WHERE id=?`), p.TenantID).Scan(&name); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"user": p, "organization": map[string]any{"id": p.TenantID, "name": name}, "projects": projects, "setup_required": false})
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
func (s *Server) runner(w http.ResponseWriter, r *http.Request, p Principal, project, path string, m map[string]any) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		writeError(w, &APIError{405, "method_not_allowed", "执行节点请求必须使用 POST", nil})
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) == "" {
		writeError(w, &APIError{401, "runner_token_required", "执行节点需要独立 Bearer 凭证", nil})
		return
	}
	if kind, boundProject := s.Service.TokenScope(r.Context(), tokenOf(r)); kind != "runner" || boundProject != project {
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
		out, err := s.Service.ReportToolInventory(r.Context(), p, project, tokenOf(r), request)
		respond(w, 200, out, err)
	case "runner/progress":
		var assignment dag.Assignment
		if e := json.Unmarshal([]byte(stringify(m["assignment"])), &assignment); e != nil {
			writeError(w, invalid("执行进度身份无效"))
			return
		}
		var event *toolcontract.Event
		if m["tool_event"] != nil {
			raw, ok := m["tool_event"].(map[string]any)
			if !ok {
				writeError(w, invalid("工具进度必须是受限事件对象"))
				return
			}
			event = &toolcontract.Event{}
			if err := strictAutomation(raw, event); err != nil {
				writeError(w, err)
				return
			}
		}
		e := s.Engine.ProgressWithAuthority(r.Context(), p.TenantID, project, assignment, str(m, "operation_id"), str(m, "message"), event, func(tx *sql.Tx) error {
			return s.Service.validateToolProgressTx(r.Context(), tx, p, project, tokenOf(r), assignment, event)
		})
		respond(w, 200, map[string]bool{"acknowledged": e == nil}, e)
	case "runner/claim":
		a, e := s.Engine.ClaimEligible(r.Context(), p.TenantID, project, str(m, "worker_id"), stringsAt(m, "capabilities"), func(tx *sql.Tx) error {
			return s.Service.runnerIdentityTx(r.Context(), tx, p, project, tokenOf(r), str(m, "worker_id"))
		}, func(tx *sql.Tx, run, node string) (bool, error) {
			allowed, err := s.Service.releaseClaimEligibleTx(r.Context(), tx, p, project, tokenOf(r), str(m, "worker_id"), run)
			if err != nil || !allowed {
				return allowed, err
			}
			return s.Service.toolClaimEligibleTx(r.Context(), tx, p, project, tokenOf(r), str(m, "worker_id"), run, node)
		})
		if e == nil && a != nil {
			a.ReleaseExecution, e = s.Service.ReleaseExecutionForAssignment(r.Context(), p, project, *a)
		}
		respond(w, 200, map[string]any{"assignment": a}, e)
	case "runner/heartbeat":
		var a dag.Assignment
		if json.Unmarshal([]byte(stringify(m["assignment"])), &a) != nil {
			writeError(w, invalid("无效执行信封"))
			return
		}
		e := s.Engine.HeartbeatWithAuthority(r.Context(), p.TenantID, project, a, func(tx *sql.Tx) error {
			return s.Service.runnerIdentityTx(r.Context(), tx, p, project, tokenOf(r), a.WorkerID)
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
		e := s.Engine.CompleteBound(r.Context(), p.TenantID, project, result, func(tx *sql.Tx) error {
			return s.Service.runnerIdentityTx(r.Context(), tx, p, project, tokenOf(r), result.WorkerID)
		}, func(tx *sql.Tx, state string) error {
			if err := s.Service.validateToolResultTx(r.Context(), tx, p, project, tokenOf(r), result); err != nil {
				return err
			}
			if err := s.Service.verifyArtifactResultTx(r.Context(), tx, p, project, tokenOf(r), result, state); err != nil {
				return err
			}
			return s.Service.ValidateReleaseResultTx(r.Context(), tx, p, project, result, state)
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
func (s *Server) agent(w http.ResponseWriter, r *http.Request, p Principal, project, path string, m map[string]any) {
	if path == "agent/tools" && r.Method == "GET" {
		writeJSON(w, 200, map[string]any{"items": []map[string]any{{"name": "project.overview", "description": "读取当前授权项目的真实状态与活动", "parameters": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}, {"name": "project.tasks", "description": "读取当前项目任务", "parameters": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}, {"name": "project.search", "description": "检索当前项目资料并返回来源", "parameters": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}, "additionalProperties": false}}}})
		return
	}
	if path == "agent/tools/invoke" && r.Method == "POST" {
		args, _ := m["arguments"].(map[string]any)
		switch str(m, "tool") {
		case "project.overview":
			out, e := s.Service.Overview(r.Context(), p, project)
			respond(w, 200, out, e)
		case "project.tasks":
			out, total, next, e := s.Service.ListPage(r.Context(), p, project, "tasks", number(args, "limit"), str(args, "cursor"))
			if e != nil {
				writeError(w, e)
				return
			}
			writeJSON(w, 200, map[string]any{"items": out, "total": total, "next_cursor": next})
		case "project.search":
			out, e := s.Service.Search(r.Context(), p, project, str(args, "query"))
			respond(w, 200, out, e)
		default:
			writeError(w, &APIError{422, "tool_not_allowed", "该工具尚未授权或未实现", nil})
		}
		return
	}
	writeError(w, missing())
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
