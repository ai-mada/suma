package runner

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func nativeWorkspaceSchemas() map[string]map[string]any {
	text := func(max int) map[string]any {
		return map[string]any{"type": "string", "minLength": 1, "maxLength": max}
	}
	integer := func(max int) map[string]any { return map[string]any{"type": "integer", "minimum": 1, "maximum": max} }
	object := func(properties map[string]any, required ...string) map[string]any {
		schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	}
	sha := map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$", "minLength": 64, "maxLength": 64}
	write := object(map[string]any{"path": text(1024), "content": map[string]any{"type": "string", "maxLength": nativeWorkspaceFileLimit}, "expected_sha256": sha, "expected_absent": map[string]any{"type": "boolean", "const": true}}, "path", "content")
	write["oneOf"] = []any{map[string]any{"required": []string{"expected_sha256"}, "not": map[string]any{"required": []string{"expected_absent"}}}, map[string]any{"required": []string{"expected_absent"}, "not": map[string]any{"required": []string{"expected_sha256"}}}}
	return map[string]map[string]any{
		"list":   object(map[string]any{"directory": text(1024), "limit": integer(200), "cursor": text(4096)}),
		"read":   object(map[string]any{"path": text(1024), "start_line": integer(nativeWorkspaceFileLimit + 1), "max_lines": integer(1000)}, "path"),
		"write":  write,
		"edit":   object(map[string]any{"path": text(1024), "expected_sha256": sha, "edits": map[string]any{"type": "array", "minItems": 1, "maxItems": 32, "items": object(map[string]any{"old_text": text(nativeWorkspaceFileLimit), "new_text": map[string]any{"type": "string", "maxLength": nativeWorkspaceFileLimit}}, "old_text", "new_text")}}, "path", "expected_sha256", "edits"),
		"remove": object(map[string]any{"path": text(1024), "expected_sha256": sha}, "path", "expected_sha256"),
		"check":  object(map[string]any{"check_id": text(100)}, "check_id"),
	}
}

func nativeRepairSchemas() map[string]map[string]any {
	sha := map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$", "minLength": 64, "maxLength": 64}
	resolution := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"path": map[string]any{"type": "string", "minLength": 1, "maxLength": 1024}, "expected_sha256": sha, "expected_absent": map[string]any{"type": "boolean", "const": true}}, "required": []string{"path"}, "oneOf": []any{map[string]any{"required": []string{"expected_sha256"}, "not": map[string]any{"required": []string{"expected_absent"}}}, map[string]any{"required": []string{"expected_absent"}, "not": map[string]any{"required": []string{"expected_sha256"}}}}}
	return map[string]map[string]any{"merge_status": {"type": "object", "additionalProperties": false, "properties": map[string]any{}}, "merge_continue": {"type": "object", "additionalProperties": false, "properties": map[string]any{"expected_step": map[string]any{"type": "integer", "minimum": 1, "maximum": 15}, "expected_generation": map[string]any{"type": "integer", "minimum": 0, "maximum": 2048}, "expected_state_sha256": sha, "resolutions": map[string]any{"type": "array", "minItems": 1, "maxItems": 2000, "items": resolution}}, "required": []string{"expected_step", "expected_generation", "expected_state_sha256", "resolutions"}}}
}

type nativeWorkspaceFailure struct {
	Error *NativeWorkspaceError `json:"error"`
}

func nativeWorkspaceMCPError(err error) *mcp.CallToolResult {
	public := nativePublicError(err)
	result := nativeWorkspaceFailure{Error: public}
	raw, _ := json.Marshal(result)
	return &mcp.CallToolResult{IsError: true, StructuredContent: result, Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}
}

func (n *nativeWorkspace) StartMCP() (NativeWorkspaceBridge, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if err := n.activeLocked(context.Background()); err != nil {
		return NativeWorkspaceBridge{}, err
	}
	if n.server != nil {
		return n.bridge, nil
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return NativeWorkspaceBridge{}, nativeError("native_workspace_unavailable")
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		listener.Close()
		return NativeWorkspaceBridge{}, nativeError("native_workspace_unavailable")
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	host := listener.Addr().String()
	server := mcp.NewServer(&mcp.Implementation{Name: "suma-native-workspace", Version: NativeWorkspaceVersion}, &mcp.ServerOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), PageSize: 10, SetCacheable: func(_ context.Context, _ mcp.Request, c *mcp.Cacheable) { c.CacheScope = "private"; c.TTLMs = 0 }})
	descriptions := map[string]string{"list": "List one directory in the current attempt workspace.", "read": "Read bounded UTF-8 source text with its full content hash.", "write": "Create or replace one text file using a content-hash precondition.", "edit": "Apply unique non-overlapping exact text replacements using a content-hash precondition.", "remove": "Remove one regular file using a content-hash precondition.", "check": "Run one administrator-pinned development check."}
	schemas := nativeWorkspaceSchemas()
	if n.workspace.ConflictRepair != nil {
		for name, schema := range nativeRepairSchemas() {
			schemas[name] = schema
		}
		descriptions["merge_status"] = "Read the host-fixed merge cursor and exact unresolved conflict stages."
		descriptions["merge_continue"] = "Explicitly resolve every current conflict using file hashes; the host preserves parents and processes remaining fixed sources until the next conflict."
	}
	for name, schema := range schemas {
		name := name
		readOnly := name == "list" || name == "read" || name == "merge_status"
		destructive := !readOnly
		closed := false
		mcp.AddTool[map[string]json.RawMessage, any](server, &mcp.Tool{Name: "suma_workspace_" + name, Description: descriptions[name], InputSchema: schema, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, IdempotentHint: true, DestructiveHint: &destructive, OpenWorldHint: &closed}}, func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]json.RawMessage) (*mcp.CallToolResult, any, error) {
			id, _ := req.Params.GetMeta()["suma/operation_id"].(string)
			result, err := n.invoke(ctx, name, req.Params.Arguments, id)
			if err != nil {
				return nativeWorkspaceMCPError(err), nil, nil
			}
			return &mcp.CallToolResult{StructuredContent: result, Content: []mcp.Content{}}, nil, nil
		})
	}
	server.AddReceivingMiddleware(mcp.Middleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if n.ctx.Err() != nil {
				return nativeWorkspaceMCPError(nativeError("native_workspace_closed")), nil
			}
			if method == "tools/call" {
				call, ok := req.(*mcp.CallToolRequest)
				if !ok || call.Params == nil {
					return nativeWorkspaceMCPError(nativeError("native_workspace_arguments")), nil
				}
				name := strings.TrimPrefix(call.Params.Name, "suma_workspace_")
				if _, err := nativeArguments(name, call.Params.Arguments); err != nil {
					// Invalid known-tool arguments are real refused invocations too.
					// Observe them through the same host-only durable trace instead
					// of losing them before the typed SDK tool handler is reached.
					id, _ := call.Params.GetMeta()["suma/operation_id"].(string)
					_, observed := n.invoke(ctx, name, call.Params.Arguments, id)
					if observed != nil {
						err = observed
					}
					return nativeWorkspaceMCPError(err), nil
				}
			}
			result, err := next(ctx, method, req)
			if method == "tools/call" {
				if err != nil {
					return nativeWorkspaceMCPError(nativeError("native_workspace_arguments")), nil
				}
				if call, ok := result.(*mcp.CallToolResult); ok && call.IsError {
					if _, public := call.StructuredContent.(nativeWorkspaceFailure); !public {
						return nativeWorkspaceMCPError(nativeError("native_workspace_arguments")), nil
					}
				}
			}
			return result, err
		}
	}))
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 2 << 20, PropagateRequestCancellation: true})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if n.ctx.Err() != nil {
			http.Error(w, "Workspace execution scope has ended.", http.StatusGone)
			return
		}
		if r.Host != host || r.URL.Path != "/mcp" || r.URL.RawQuery != "" || len(r.Header.Values("Origin")) != 0 || len(r.Header.Values("Cookie")) != 0 {
			http.Error(w, "Workspace request is not allowed.", http.StatusForbidden)
			return
		}
		values := r.Header.Values("Authorization")
		if len(values) != 1 || subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+token)) != 1 {
			http.Error(w, "Workspace authorization is required.", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Workspace method is not allowed.", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 65*time.Second)
		stop := context.AfterFunc(n.ctx, cancel)
		defer func() { stop(); cancel() }()
		transport.ServeHTTP(w, r.WithContext(ctx))
	})
	n.server = &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 70 * time.Second, WriteTimeout: 70 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10}
	n.bridge = NativeWorkspaceBridge{Version: 1, Endpoint: "http://" + host + "/mcp", Token: token, PolicySHA256: n.policySHA, RequestID: digest([]byte(key(n.assignment)))}
	if n.workspace.ConflictRepair != nil {
		n.bridge.ConflictRepairProtocol = ccc.CandidateConflictRepairProtocol
	}
	go func() { _ = n.server.Serve(listener) }()
	return n.bridge, nil
}
