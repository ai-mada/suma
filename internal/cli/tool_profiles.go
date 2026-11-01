package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"unicode"

	"github.com/ai-mada/suma/internal/localization"
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const toolMatchesUsage = "tools matches requires --file REQUEST_JSON or --file -; use tools --help"
const toolProfilesUsage = "tools requires list, doctor or matches; use tools --help"
const toolMatchesInvalid = "tool matches request requires the current project, worker_id and one complete exact profile"
const toolMatchesResponseInvalid = "tool matches response must preserve the requested scope, complete profile and full result count"
const toolMatchesInputLimit = 256 << 10

func toolProfilesCommand(ctx context.Context, c *client, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") || len(args) == 2 && (args[0] == "list" || args[0] == "matches" || args[0] == "doctor") && (args[1] == "help" || args[1] == "--help") {
		text := toolProfilesHelpZH
		if c.locale == localization.EnUS {
			text = toolProfilesHelpEN
		}
		_, err := io.WriteString(out, text)
		return err
	}
	if len(args) > 0 && args[0] == "doctor" {
		return toolDoctorCommand(ctx, c, args[1:], out)
	}
	if len(args) > 0 && args[0] == "list" {
		return toolListCommand(ctx, c, args[1:], out)
	}
	if len(args) == 0 || args[0] != "matches" {
		return usage(toolProfilesUsage)
	}
	flags := commandFlags("tools matches")
	file := flags.String("file", "", "complete match request JSON file, or - for standard input")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || *file == "" {
		return usage(toolMatchesUsage)
	}
	if c.project == "" {
		return usage("--project is required for this command")
	}
	reader := in
	if *file != "-" {
		input, err := os.Open(*file)
		if err != nil {
			return usage("cannot open JSON file")
		}
		defer input.Close()
		reader = input
	}
	if reader == nil {
		return usage("cannot read JSON input")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, toolMatchesInputLimit+1))
	if err != nil || len(raw) > toolMatchesInputLimit {
		return usage(toolMatchesInvalid)
	}
	// Validate original bytes before adding the global project. Re-encoding
	// first would discard duplicate keys and malformed Unicode evidence.
	var fields map[string]json.RawMessage
	if strictjson.Decode(raw, &fields, toolMatchesInputLimit) != nil || fields == nil {
		return usage(toolMatchesInvalid)
	}
	if _, present := fields["project_id"]; !present {
		fields["project_id"], _ = json.Marshal(c.project)
		raw, _ = json.Marshal(fields)
	}
	var request platform.ToolProfileMatchRequest
	if strictjson.Decode(raw, &request, toolMatchesInputLimit) != nil || request.ProjectID != c.project || len(request.WorkerID) > 100 || strings.TrimSpace(request.WorkerID) != request.WorkerID || !contextID(request.WorkerID) || strings.ContainsFunc(request.WorkerID, unicode.IsControl) || tc.ValidateProfile(request.Profile, true) != nil {
		return usage(toolMatchesInvalid)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return usage(toolMatchesInvalid)
	}
	var body map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if decoder.Decode(&body) != nil {
		return usage(toolMatchesInvalid)
	}
	// A read-only POST never receives an operation key or confirm flag. The
	// original client retains the login session/view and Accept-Language.
	response, err := c.request(ctx, http.MethodPost, "/api/v1/tool-profiles/matches", body, true)
	if err != nil {
		return err
	}
	var matches platform.ToolProfileMatchList
	if strictjson.Decode(response, &matches, 16<<20) != nil || matches.ProjectID != request.ProjectID || matches.WorkerID != request.WorkerID || !tc.EqualProfile(matches.Profile, request.Profile) || matches.Items == nil || len(matches.Items) > 1000 || matches.Total != len(matches.Items) {
		return &failure{Code: "response", Message: toolMatchesResponseInvalid}
	}
	seen := map[string]bool{}
	for _, item := range matches.Items {
		if !contextID(item.ID) || seen[item.ID] || item.Version < 1 || item.ProjectID != request.ProjectID || item.WorkerID != request.WorkerID || !tc.EqualProfile(item.Profile, request.Profile) || (item.State != "paused" && item.State != "enabled" && item.State != "revoked") {
			return &failure{Code: "response", Message: toolMatchesResponseInvalid}
		}
		seen[item.ID] = true
	}
	return writeJSON(out, response, c.token)
}

const toolProfilesHelpEN = `tools list — read current tool connections or their history
  suma --project PROJECT tools list [--state active|enabled|paused|revoked|all]
       [--query TEXT] [--limit 1..200] [--cursor NEXT_CURSOR]
  Default: active (enabled and paused), 50 items. Query: at most 120 UTF-8 bytes.
  Reuse the exact cursor with the same filters and original account/project view.
  Profile changes retain the explicit enable/pause/revoke command API; revoke keeps history.

tools doctor — read current tool profile diagnostics
  suma --project PROJECT --expected-user USER [--expected-organization ORG] tools doctor --id PROFILE_ID
  This GET reports four current facts and their display deadline of at most 60 seconds.
  It never runs a suggestion, enables a profile or accesses tool credentials.
  A valid worker identity does not prove online state, model login or execution.

tools matches — find complete existing tool profiles
  suma --project PROJECT --expected-user USER [--expected-organization ORG] tools matches --file request.json
  Use --file - to read standard input; input is limited to 256 KiB.

Use an existing login session (SUMA_TOKEN), not a project API, Runner or Pool token.
JSON contains worker_id and the full profile; project_id may be omitted or must
match --project. Profile names and hashes alone are insufficient.
This POST only reads current matches. No operation key or --confirm is needed.
Paused/enabled matches require explicit reuse; revoked entries are history.
This command never creates, enables, connects or executes a tool.
`
const toolProfilesHelpZH = `tools list — 查看当前工具接入或已移除记录
  suma --project 项目ID tools list [--state active|enabled|paused|revoked|all]
       [--query 检索文本] [--limit 1..200] [--cursor 原分页游标]
  默认 active（已启用和暂停），每页 50 项；检索文本最多 120 个 UTF-8 字节。
  翻页保持原筛选、账号和项目，使用同一列表返回的原游标。
  配置变更仍使用原 enable/pause/revoke 命令 API；revoke 移除接入并保留历史。

tools doctor — 读取当前工具档案体检
  suma --project PROJECT --expected-user USER [--expected-organization ORG] tools doctor --id PROFILE_ID
  此 GET 展示四项当前事实与最多 60 秒的展示期限。
  不运行建议、不启用档案、不读取工具凭据。
  节点身份有效不代表机器在线、模型已登录或已执行。

tools matches — 查询完整匹配的已有工具档案
  suma --project PROJECT --expected-user USER [--expected-organization ORG] tools matches --file request.json
  --file - 从标准输入读取；正文最多 256 KiB。

使用已有登录会话（SUMA_TOKEN），不接受项目 API、Runner 或 Pool 令牌。
JSON 包含 worker_id 和完整 profile；project_id 可省略，提供时须与 --project 一致。
名称或局部摘要不能替代完整档案。此 POST 只读，不添加操作 key 或 --confirm。
暂停／已启用档案由用户明确复用；已撤销项仅供查看历史。
本命令不创建、启用、连接或执行工具。
`
