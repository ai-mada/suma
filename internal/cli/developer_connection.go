package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const developerCheckResponseLimit = 1 << 20

type developerCheckTool struct {
	Name     string `json:"name"`
	ReadOnly bool   `json:"read_only"`
}

// The diagnostic never exposes schemas, server instructions, tool output or
// credentials. Its evidence applies only to this completed invocation.
type developerCheckReport struct {
	ProjectID       string               `json:"project_id"`
	Endpoint        string               `json:"endpoint"`
	CheckedAt       string               `json:"checked_at"`
	Scope           string               `json:"scope"`
	Transport       string               `json:"transport"`
	ProtocolVersion string               `json:"protocol_version"`
	Server          mcp.Implementation   `json:"server"`
	Tools           []developerCheckTool `json:"tools"`
	ReadProbe       struct {
		Tool           string `json:"tool"`
		ProjectID      string `json:"project_id"`
		ResponseSHA256 string `json:"response_sha256"`
		ResponseBytes  int    `json:"response_bytes"`
	} `json:"read_probe"`
}

func developerCheckFailure(c *client, code, zh, en string) error {
	return &failure{Code: code, Message: c.text(zh, en)}
}

func developerConnectionCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) != 1 || args[0] != "check" || !developerCheckScope(c) {
		return developerCheckFailure(c, "usage", "developer check 需要有效的 --project 和项目 API 或 Agent 凭证；远端服务必须使用 HTTPS。", "developer check requires a valid --project and project API or Agent credential; remote control requires HTTPS.")
	}
	endpoint := c.base + "/api/v1/mcp/projects/" + c.project
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	hc := &http.Client{Transport: developerCheckTransport{base: c.http.Transport, endpoint: endpoint, token: c.token, language: string(c.locale)}, Timeout: c.timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	sdk := mcp.NewClient(&mcp.Implementation{Name: "suma-developer-check", Version: "1.0.0"}, &mcp.ClientOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	// Explicit legacy initialization is supported by the current server. Disable
	// retries and standalone subscriptions for this one-shot read diagnostic.
	session, err := sdk.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: hc, MaxRetries: -1, DisableStandaloneSSE: true}, &mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
	if err != nil {
		return developerCheckFailure(c, "mcp_initialize_failed", "MCP 初始化失败；请检查服务地址、当前项目凭证及服务端访问许可。", "MCP initialization failed; check the control origin, current project credential and server access policy.")
	}
	defer session.Close()
	initial := session.InitializeResult()
	if initial == nil || initial.ServerInfo == nil || initial.ServerInfo.Name != "suma" || initial.ProtocolVersion == "" {
		return developerCheckFailure(c, "mcp_server_invalid", "MCP 服务未返回预期的速码身份与协议。", "MCP server did not return the expected suma identity and protocol.")
	}
	report := developerCheckReport{ProjectID: c.project, Endpoint: endpoint, Scope: "single_cli_invocation", Transport: "streamable_http", ProtocolVersion: initial.ProtocolVersion, Server: *initial.ServerInfo, Tools: []developerCheckTool{}}
	seenNames, seenCursors := map[string]bool{}, map[string]bool{}
	cursor := ""
	probeAllowed := false
	for page := 0; ; page++ {
		if page >= 10 {
			return developerCheckFailure(c, "mcp_catalog_invalid", "MCP 工具目录超过诊断限制或分页无效。", "MCP tool catalogue exceeds diagnostic bounds or has invalid pagination.")
		}
		list, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil || list == nil {
			return developerCheckFailure(c, "mcp_catalog_failed", "MCP 当前工具目录读取失败；请检查当前项目权限。", "MCP current tool catalogue could not be read; check current project authority.")
		}
		for _, tool := range list.Tools {
			if tool == nil || tool.Name == "" || len(tool.Name) > 128 || strings.ContainsAny(tool.Name, "\r\n\x00") || seenNames[tool.Name] || len(report.Tools) >= 200 {
				return developerCheckFailure(c, "mcp_catalog_invalid", "MCP 工具目录超过诊断限制或分页无效。", "MCP tool catalogue exceeds diagnostic bounds or has invalid pagination.")
			}
			seenNames[tool.Name] = true
			readonly := tool.Annotations != nil && tool.Annotations.ReadOnlyHint
			report.Tools = append(report.Tools, developerCheckTool{Name: tool.Name, ReadOnly: readonly})
			if tool.Name == "suma_project_overview" && readonly {
				probeAllowed = true
			}
		}
		if list.NextCursor == "" {
			break
		}
		if len(list.NextCursor) > 4096 || seenCursors[list.NextCursor] {
			return developerCheckFailure(c, "mcp_catalog_invalid", "MCP 工具目录超过诊断限制或分页无效。", "MCP tool catalogue exceeds diagnostic bounds or has invalid pagination.")
		}
		seenCursors[list.NextCursor] = true
		cursor = list.NextCursor
	}
	if !probeAllowed {
		return developerCheckFailure(c, "mcp_probe_unavailable", "当前 MCP 目录未声明项目概览只读工具，未执行诊断调用。", "The current MCP catalogue does not declare the read-only project overview tool; no probe was called.")
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "suma_project_overview", Arguments: map[string]any{}})
	if err != nil || result == nil || result.IsError || result.StructuredContent == nil {
		return developerCheckFailure(c, "mcp_probe_failed", "MCP 项目概览只读调用失败；未确认本次连接。", "The MCP read-only project overview call failed; this connection was not verified.")
	}
	raw, err := json.Marshal(result.StructuredContent)
	var overview struct {
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	if err != nil || len(raw) > 128<<10 || json.Unmarshal(raw, &overview) != nil || overview.Project.ID != c.project {
		return developerCheckFailure(c, "mcp_probe_scope", "MCP 只读结果未证明当前项目范围，未确认本次连接。", "The MCP read result did not establish the current project scope; this connection was not verified.")
	}
	hash := sha256.Sum256(raw)
	report.ReadProbe.Tool = "suma_project_overview"
	report.ReadProbe.ProjectID = overview.Project.ID
	report.ReadProbe.ResponseSHA256 = hex.EncodeToString(hash[:])
	report.ReadProbe.ResponseBytes = len(raw)
	report.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
	sort.Slice(report.Tools, func(i, j int) bool { return report.Tools[i].Name < report.Tools[j].Name })
	return writeJSON(out, report, c.token)
}

func developerCheckScope(c *client) bool {
	if c.project == "" || len(c.project) > 64 || c.token == "" || strings.TrimSpace(c.token) != c.token || strings.ContainsAny(c.token, "\r\n\t ") {
		return false
	}
	for _, char := range c.project {
		if char != '-' && char != '_' && !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') {
			return false
		}
	}
	u, err := url.Parse(c.base)
	if err != nil || u.RawPath != "" || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return u.Scheme == "https" || u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())
}

type developerCheckTransport struct {
	base                      http.RoundTripper
	endpoint, token, language string
}

func (t developerCheckTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.String() != t.endpoint {
		return nil, errors.New("MCP destination rejected")
	}
	copy := req.Clone(req.Context())
	copy.Header = req.Header.Clone()
	copy.Header.Set("Authorization", "Bearer "+t.token)
	copy.Header.Set("X-Suma-Language", t.language)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(copy)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, developerCheckResponseLimit+1))
	resp.Body.Close()
	if err != nil || len(raw) > developerCheckResponseLimit {
		return nil, errors.New("MCP response exceeds diagnostic bounds")
	}
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	return resp, nil
}
