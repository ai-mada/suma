package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ai-mada/suma/internal/localization"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpProjectPrefix = "/api/v1/mcp/projects/"
const mcpMaxRequestBytes = 1 << 20
const mcpMaxOutputBytes = 128 << 10
const mcpCallTimeout = 30 * time.Second

type mcpRequestKey struct{}
type mcpRequestState struct {
	actor   Actor
	project string
	tools   map[string]bool
	headers http.Header
	locale  localization.Locale
}

// NewMCPHandler registers only the fixed application catalogue. Every HTTP
// request supplies a fresh credential and fixed project; SDK sessions grant no
// platform authority, and each application call rechecks its domain transaction.
func (s *Server) NewMCPHandler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "suma", Version: "0.1.0"}, &mcp.ServerOptions{
		PageSize:     100,
		SetCacheable: func(_ context.Context, _ mcp.Request, c *mcp.Cacheable) { c.CacheScope = "private"; c.TTLMs = 0 },
	})
	for _, definition := range DeclaredApplicationTools() {
		name, readOnly := definition.Name, definition.ReadOnly
		closed := false
		destructive := name == "project.requirement_design_save"
		mcp.AddTool[map[string]json.RawMessage, any](server, &mcp.Tool{
			Name: definition.MCPName, Description: definition.Description, InputSchema: definition.Parameters,
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, IdempotentHint: true, DestructiveHint: &destructive, OpenWorldHint: &closed},
		}, func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]json.RawMessage) (*mcp.CallToolResult, any, error) {
			state, ok := ctx.Value(mcpRequestKey{}).(*mcpRequestState)
			if !ok {
				return mcpFailure(ctx, forbidden()), nil, nil
			}
			if !readOnly {
				if err := validateOriginalView(&http.Request{Header: state.headers}, state.actor.Projection(), false); err != nil {
					return mcpFailure(ctx, err), nil, nil
				}
			}
			// Keep the original numeric lexemes and duplicate-key evidence. The typed
			// map is the SDK's validated input, never a replacement for original bytes.
			arguments := req.Params.Arguments
			if len(arguments) == 0 {
				arguments = json.RawMessage(`{}`)
			}
			out, err := s.Service.InvokeApplicationTool(ctx, state.actor, ApplicationToolCall{ProjectID: state.project, Tool: name, Arguments: arguments})
			if err != nil {
				return mcpFailure(ctx, err), nil, nil
			}
			raw, err := json.Marshal(out)
			if err != nil {
				return mcpFailure(ctx, errors.New("tool result encoding failed")), nil, nil
			}
			if len(raw) > mcpMaxOutputBytes {
				return mcpFailure(ctx, &APIError{413, "tool_output_too_large", "工具输出超过限制，请缩小页面或引用范围", nil}), nil, nil
			}
			return nil, out, nil
		})
	}
	server.AddReceivingMiddleware(mcp.Middleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			ctx, cancel := context.WithTimeout(ctx, mcpCallTimeout)
			defer cancel()
			state, ok := ctx.Value(mcpRequestKey{}).(*mcpRequestState)
			if !ok {
				return nil, forbidden()
			}
			if method == "tools/call" {
				call, ok := req.(*mcp.CallToolRequest)
				if !ok || call.Params == nil || !state.tools[call.Params.Name] {
					return mcpFailure(ctx, forbidden()), nil
				}
				raw := bytes.TrimSpace(call.Params.Arguments)
				if len(raw) > 0 && (raw[0] != '{' || RejectDuplicateReleaseJSON(raw) != nil) {
					return mcpFailure(ctx, mcpInvalidArguments()), nil
				}
			}
			result, err := next(ctx, method, req)
			if method == "tools/call" {
				if err != nil {
					return mcpFailure(ctx, err), nil
				}
				if call, ok := result.(*mcp.CallToolResult); ok && call.IsError {
					if _, public := call.StructuredContent.(mcpPublicFailure); !public {
						// SDK schema diagnostics may include offending values. Keep its
						// validation decision, expose only a controlled public error.
						return mcpFailure(ctx, mcpInvalidArguments()), nil
					}
				}
			}
			if list, ok := result.(*mcp.ListToolsResult); ok && err == nil {
				visible := make([]*mcp.Tool, 0, len(list.Tools))
				for _, tool := range list.Tools {
					if state.tools[tool.Name] {
						copy := *tool
						if key, found := localization.LookupSource(copy.Description); found {
							text, _ := localization.WithSource(state.locale, key, nil, copy.Description)
							copy.Description = text.Text
						}
						visible = append(visible, &copy)
					}
				}
				list.Tools = visible
			}
			return result, err
		}
	}))
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: mcpMaxRequestBytes, PropagateRequestCancellation: true,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setResponseLanguage(w, r)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !s.mcpRequestOriginAllowed(r) {
			writeError(w, &APIError{403, "origin_rejected", "请求来源或主机不在允许范围", nil})
			return
		}
		project := strings.TrimPrefix(r.URL.Path, mcpProjectPrefix)
		if !strings.HasPrefix(r.URL.Path, mcpProjectPrefix) || !localePreferenceScopeID.MatchString(project) {
			writeError(w, missing())
			return
		}
		values := r.Header.Values("Authorization")
		if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, unauthenticated())
			return
		}
		token := strings.TrimPrefix(values[0], "Bearer ")
		if token == "" || len(token) > 4096 || strings.IndexFunc(token, unicode.IsSpace) >= 0 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, unauthenticated())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), mcpCallTimeout)
		defer cancel()
		actor, err := s.Service.AuthenticateActor(ctx, token)
		if err != nil {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, mcpSafeAPIError(err))
			return
		}
		if (actor.kind != actorProjectAPI && actor.kind != actorAgentRead) || actor.credential.projectID != project {
			writeError(w, forbidden())
			return
		}
		tools, err := s.Service.ApplicationTools(ctx, actor, project)
		if err != nil {
			writeError(w, mcpSafeAPIError(err))
			return
		}
		s.applyLocalePreferences(w, r, actor)
		locale, ok := localization.Normalize(w.Header().Get("Content-Language"))
		if !ok {
			locale = localization.DefaultLocale
		}
		state := &mcpRequestState{actor: actor, project: project, tools: make(map[string]bool, len(tools.Items)), headers: r.Header.Clone(), locale: locale}
		for _, tool := range tools.Items {
			state.tools[tool.MCPName] = true
		}
		transport.ServeHTTP(w, r.WithContext(context.WithValue(ctx, mcpRequestKey{}, state)))
	})
}

func mcpInvalidArguments() error {
	return &APIError{400, "invalid_tool_arguments", "工具参数无效，请按工具声明提供完整且无重复字段的参数", nil}
}

type mcpPublicFailure struct {
	Error map[string]any `json:"error"`
}

func mcpSafeAPIError(err error) *APIError {
	var api *APIError
	if errors.As(err, &api) {
		return &APIError{Status: api.Status, Code: api.Code, Message: api.Message}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &APIError{408, "request_timeout", "工具调用超过时间限制，请核对原操作后重试", nil}
	}
	if errors.Is(err, context.Canceled) {
		return &APIError{408, "request_cancelled", "工具调用已取消，请核对原操作状态", nil}
	}
	return &APIError{500, "internal_error", "服务暂时无法完成操作", nil}
}

func mcpFailure(ctx context.Context, err error) *mcp.CallToolResult {
	api := mcpSafeAPIError(err)
	locale := localization.DefaultLocale
	if state, ok := ctx.Value(mcpRequestKey{}).(*mcpRequestState); ok {
		locale = state.locale
	}
	key, found := localization.LookupSource(api.Message)
	if !found {
		key = "untranslated." + api.Code
	}
	text, _ := localization.WithSource(locale, key, nil, api.Message)
	failure := mcpPublicFailure{map[string]any{"status": api.Status, "code": api.Code, "message": text.Text, "message_key": text.Key, "locale": string(locale), "localized": text.Localized}}
	raw, _ := json.Marshal(failure)
	return &mcp.CallToolResult{IsError: true, StructuredContent: failure, Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}
}

// ResolveMCPPublicURL derives credential destinations only from deployment
// configuration or an explicit loopback listener, never request/proxy headers.
func ResolveMCPPublicURL(configured, listenAddr string) (origin, source string, err error) {
	if configured != "" {
		u, e := url.Parse(configured)
		if e != nil || strings.TrimSpace(configured) != configured || u.User != nil || u.Opaque != "" || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" || strings.ContainsAny(configured, "?#") || u.Scheme != "https" && u.Scheme != "http" || u.Scheme == "http" && !mcpLoopback(u.Host) || !mcpValidPort(u.Host) {
			return "", "", &APIError{400, "invalid_public_url", "公开服务地址必须为 HTTPS 源地址，本地回环开发地址可使用 HTTP", nil}
		}
		if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsUnspecified() {
			return "", "", &APIError{400, "invalid_public_url", "公开服务地址不能使用通配监听地址", nil}
		}
		return u.Scheme + "://" + u.Host, "configured", nil
	}
	if !mcpLoopback(listenAddr) || !mcpValidPort(listenAddr) {
		return "", "unconfigured", nil
	}
	if _, port, e := net.SplitHostPort(listenAddr); e != nil || port == "" {
		return "", "unconfigured", nil
	}
	return "http://" + listenAddr, "listener_loopback", nil
}

func mcpValidPort(host string) bool {
	u, err := url.Parse("https://" + host)
	if err != nil || u.Hostname() == "" || strings.HasSuffix(host, ":") {
		return false
	}
	if p := u.Port(); p != "" {
		n, e := strconv.Atoi(p)
		return e == nil && n > 0 && n <= 65535
	}
	return true
}

func mcpLoopback(host string) bool {
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	name = strings.Trim(name, "[]")
	if name == "localhost" {
		return true
	}
	ip := net.ParseIP(name)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) mcpRequestOriginAllowed(r *http.Request) bool {
	expected := ""
	if s.PublicURL != "" {
		origin, _, err := ResolveMCPPublicURL(s.PublicURL, "")
		if err != nil {
			return false
		}
		u, _ := url.Parse(origin)
		if r.Host != u.Host {
			return false
		}
		expected = origin
	} else {
		if !mcpLoopback(r.Host) {
			return false
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		expected = scheme + "://" + r.Host
	}
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	}
	if len(origins) != 1 {
		return false
	}
	if origins[0] == expected {
		return true
	}
	for _, allowed := range s.AllowedOrigins {
		if origin, _, err := ResolveMCPPublicURL(allowed, ""); err == nil && origins[0] == origin {
			return true
		}
	}
	return false
}

func (s *Server) DeveloperConnection(ctx context.Context, actor Actor, projectID string) (DeveloperConnectionMetadata, error) {
	var out DeveloperConnectionMetadata
	if actor.kind != actorBrowser && actor.kind != actorProjectAPI {
		return out, forbidden()
	}
	if !localePreferenceScopeID.MatchString(projectID) {
		return out, invalid("请选择有效项目")
	}
	err := s.Service.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: projectID}, purposeProjectRead, func(auth *authorizedCommand) error {
		out = DeveloperConnectionMetadata{ProjectID: projectID, EndpointPath: mcpProjectPrefix + projectID, EndpointSource: "unconfigured", Transport: "streamable_http", Stateless: true, SDKName: "github.com/modelcontextprotocol/go-sdk", SDKVersion: "v1.8.0", TokenEnvironment: "SUMA_MCP_TOKEN", ExpectedUserID: auth.currentPrincipal.ID, ExpectedOrganizationID: auth.currentPrincipal.TenantID, ClientConnectionVerified: false, Tools: DeclaredApplicationTools()}
		if s.PublicURL != "" {
			origin, source, e := ResolveMCPPublicURL(s.PublicURL, "")
			if e != nil {
				return e
			}
			if s.PublicURLSource == "listener_loopback" {
				source = s.PublicURLSource
			}
			out.EndpointURL = origin + out.EndpointPath
			out.EndpointSource = source
		}
		return nil
	})
	return out, err
}
