package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/ai-mada/suma/internal/localization"
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const toolMatchesUsage = "tools matches requires either --file REQUEST_JSON|- or --worker WORKER with --command COMMAND; use tools --help"
const toolProfilesUsage = "tools requires list, catalog, inventory, doctor, matches, replace or replacements; use tools --help"
const toolMatchesInvalid = "tool matches request requires the current project, worker_id and one complete exact profile"
const toolMatchesResponseInvalid = "tool matches response must preserve the requested scope, complete profile and full result count"
const toolInventoryMatchUnusable = "the selected worker has no current fresh report; use tools inventory to inspect the report and refresh it before matching"
const toolInventoryMatchMissing = "the selected command is absent from the worker's fresh report; use tools inventory to inspect reported commands"
const toolInventoryMatchUnavailable = "the selected command is unavailable in the worker's fresh report; use tools inventory to inspect the reported reason"
const toolMatchesInputLimit = 256 << 10

func toolProfilesCommand(ctx context.Context, c *client, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") || len(args) == 2 && (args[0] == "list" || args[0] == "catalog" || args[0] == "inventory" || args[0] == "matches" || args[0] == "doctor" || args[0] == "replace" || args[0] == "replacements") && (args[1] == "help" || args[1] == "--help") {
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
	if len(args) > 0 && args[0] == "catalog" {
		return toolCatalogCommand(ctx, c, args[1:], out)
	}
	if len(args) > 0 && args[0] == "inventory" {
		return toolInventoryCommand(ctx, c, args[1:], out)
	}
	if len(args) > 0 && args[0] == "replace" {
		return toolReplacementCommand(ctx, c, args[1:], in, out)
	}
	if len(args) > 0 && args[0] == "replacements" {
		return toolReplacementHistoryCommand(ctx, c, args[1:], out)
	}
	if len(args) == 0 || args[0] != "matches" {
		return usage(toolProfilesUsage)
	}
	flags := commandFlags("tools matches")
	file := flags.String("file", "", "complete match request JSON file, or - for standard input")
	var worker, command string
	onceID := func(dst *string) func(string) error {
		return func(value string) error {
			if *dst != "" || !tc.ValidCommandID(value) {
				return usage(toolMatchesUsage)
			}
			*dst = value
			return nil
		}
	}
	flags.Func("worker", "exact worker ID from the current project", onceID(&worker))
	flags.Func("command", "exact command ID from the worker's fresh report", onceID(&command))
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 {
		return usage(toolMatchesUsage)
	}
	fileSet := false
	flags.Visit(func(option *flag.Flag) { fileSet = fileSet || option.Name == "file" })
	if fileSet && (*file == "" || worker != "" || command != "") || !fileSet && (worker == "" || command == "") {
		return usage(toolMatchesUsage)
	}
	if c.project == "" {
		return usage("--project is required for this command")
	}
	if !fileSet {
		return matchToolInventory(ctx, c, worker, command, out)
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
	return queryToolProfileMatches(ctx, c, request, time.Time{}, out)
}

func matchToolInventory(ctx context.Context, c *client, worker, command string, out io.Writer) error {
	inventory, deadline, err := readToolInventory(ctx, c, worker)
	if err != nil {
		return err
	}
	if inventory.State != "fresh" || !time.Now().Before(deadline) {
		return &failure{Code: "inventory_unusable", Message: toolInventoryMatchUnusable}
	}
	for _, observation := range inventory.Items {
		if observation.Profile.CommandID != command {
			continue
		}
		if !observation.Available {
			return &failure{Code: "inventory_command_unavailable", Message: toolInventoryMatchUnavailable}
		}
		request := platform.ToolProfileMatchRequest{ProjectID: c.project, WorkerID: worker, Profile: observation.Profile}
		return queryToolProfileMatches(ctx, c, request, deadline, out)
	}
	return &failure{Code: "inventory_command_not_found", Message: toolInventoryMatchMissing}
}

func queryToolProfileMatches(ctx context.Context, c *client, request platform.ToolProfileMatchRequest, inventoryDeadline time.Time, out io.Writer) error {
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
	if !inventoryDeadline.IsZero() && !time.Now().Before(inventoryDeadline) {
		return &failure{Code: "inventory_unusable", Message: toolInventoryMatchUnusable}
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
       [--worker WORKER] [--query TEXT] [--limit 1..200] [--cursor NEXT_CURSOR]
  Default: active (enabled and paused), 50 items. Query: at most 120 UTF-8 bytes.
  --worker matches the full worker ID, including letter case.
  Reuse the exact cursor with the same filters and original account/project view.
  Profile changes retain the explicit enable/pause/revoke command API; revoke keeps history.

tools catalog — find adapter entry points for the current project
  suma --project PROJECT tools catalog [--direction all|suma_to_tool|tool_to_suma] [--query TEXT]
  Default: both directions. Each search term must match the product, entry point,
  ID, adapter or protocol, ignoring case. Query: at most 120 UTF-8 bytes.
  JSON shows the catalog's implementation, capabilities and limits.

tools inventory — read the selected worker's reported inventory
  suma --project PROJECT --expected-user USER [--expected-organization ORG] tools inventory --worker WORKER
  Use an existing login session (SUMA_TOKEN). JSON distinguishes fresh, stale and unreported inventories.
  This read does not scan the worker or execute a tool.

tools doctor — read current tool profile diagnostics
  suma --project PROJECT --expected-user USER [--expected-organization ORG] tools doctor --id PROFILE_ID [--format json|table]
  This GET reports four current facts and their display deadline of at most 60 seconds.
  The separate maintenance suggestion locates the exact worker and original inspection/recovery.
  Default JSON preserves machine facts; table labels use the selected language.
  It never runs a suggestion, creates a request, enables a profile or accesses tool credentials.
  A valid worker identity does not prove online state, model login or execution.

tools replace — explicitly prepare a replacement configuration
  suma --project PROJECT --expected-user USER [--expected-organization ORG] tools replace --id ORIGINAL_PROFILE --file request.json --confirm
  --file - reads stdin. Keep the original JSON (at most 256 KiB) and operation key.
  Supply project_id, expected_version, worker_id, full profile, pause_original and idempotency_key.
  Create: supply name; reuse: supply reuse:{profile_id,expected_version} and omit name.
  A newly created target is paused; reuse preserves its existing state. No auto-enable,
  task migration, process stop or workspace/model execution occurs.
  An unconfirmed receipt requires the same original account, organization, target ID,
  JSON body and key. No automatic retry, identity/version refresh or new key is generated.

tools replacements — read committed historical handoffs
  suma --project PROJECT tools replacements --id PROFILE [--direction outgoing|incoming]
       [--limit 1..100] [--cursor ORIGINAL_CURSOR]
  Default: outgoing, 20 items. The opaque cursor is at most 2048 UTF-8 bytes.
  Snapshots describe the committed handoff, not current profile health or authorization.

tools matches — find complete existing tool profiles
  suma --project PROJECT --expected-user USER [--expected-organization ORG] tools matches --file request.json
  suma --project PROJECT --expected-user USER [--expected-organization ORG] tools matches --worker WORKER --command COMMAND
  Use --file - to read standard input; input is limited to 256 KiB.
  With --worker and --command, use one available command from the worker's fresh report.

Use an existing login session (SUMA_TOKEN), not a project API, Runner or Pool token.
JSON contains worker_id and the full profile; project_id may be omitted or must
match --project. Profile names and hashes alone are insufficient.
This POST only reads current matches. No operation key or --confirm is needed.
Paused/enabled matches require explicit reuse; revoked entries are history.
This command never creates, enables, connects or executes a tool.
`
const toolProfilesHelpZH = `tools list — 查看当前工具接入或已移除记录
  suma --project 项目ID tools list [--state active|enabled|paused|revoked|all]
       [--worker 节点ID] [--query 检索文本] [--limit 1..200] [--cursor 原分页游标]
  默认 active（已启用和暂停），每页 50 项；检索文本最多 120 个 UTF-8 字节。
  --worker 按完整节点 ID 匹配，区分字母大小写。
  翻页保持原筛选、账号和项目，使用同一列表返回的原游标。
  配置变更仍使用原 enable/pause/revoke 命令 API；revoke 移除接入并保留历史。

tools catalog — 查找当前项目的工具接入入口
  suma --project 项目ID tools catalog [--direction all|suma_to_tool|tool_to_suma] [--query 检索文本]
  默认查询两个方向。每个检索词都须匹配产品、入口、ID、适配器或协议，忽略大小写。
  检索文本最多 120 个 UTF-8 字节。JSON 保留目录中的实现状态、能力和限制。

tools inventory — 读取指定节点的工具上报
  suma --project PROJECT --expected-user USER [--expected-organization ORG] tools inventory --worker WORKER
  使用已有登录会话（SUMA_TOKEN）。JSON 区分新鲜、过期和未上报状态。
  此读取不扫描节点，也不运行工具。

tools doctor — 读取当前工具档案体检
  suma --project PROJECT --expected-user USER [--expected-organization ORG] tools doctor --id PROFILE_ID [--format json|table]
  此 GET 展示四项当前事实与最多 60 秒的展示期限。
  独立维护建议定位准确节点及原检查／核对记录。
  默认 JSON 保留机器事实；table 表格标题随选择语言展示。
  不运行建议、不创建请求、不启用档案、不读取工具凭据。
  节点身份有效不代表机器在线、模型已登录或已执行。

tools replace — 明确准备替换配置
  suma --project PROJECT --expected-user USER [--expected-organization ORG] tools replace --id ORIGINAL_PROFILE --file request.json --confirm
  --file - 从标准输入读取；保留原 JSON（最多 256 KiB）和操作标识。
  提供 project_id、expected_version、worker_id、完整 profile、pause_original 和 idempotency_key。
  新建时提供 name；复用时提供 reuse:{profile_id,expected_version} 并省略 name。
  新建目标保持暂停；复用保留已有状态。不自动启用、迁移任务、停止进程或执行工作区／模型。
  回执未确认时，保留相同原账户、组织、目标 ID、JSON 正文和操作 key。
  不自动重试、不更新身份／版本、不生成新 key。

tools replacements — 读取已提交的历史换新关系
  suma --project PROJECT tools replacements --id PROFILE [--direction outgoing|incoming]
       [--limit 1..100] [--cursor ORIGINAL_CURSOR]
  默认 outgoing，每页 20 项；原不透明游标最多 2048 个 UTF-8 字节。
  快照描述已提交交接，不代表当前配置健康或当前授权。

tools matches — 查询完整匹配的已有工具档案
  suma --project PROJECT --expected-user USER [--expected-organization ORG] tools matches --file request.json
  suma --project PROJECT --expected-user USER [--expected-organization ORG] tools matches --worker WORKER --command COMMAND
  --file - 从标准输入读取；正文最多 256 KiB。
  使用 --worker 和 --command 时，从节点的新鲜上报中读取指定的可用命令。

使用已有登录会话（SUMA_TOKEN），不接受项目 API、Runner 或 Pool 令牌。
JSON 包含 worker_id 和完整 profile；project_id 可省略，提供时须与 --project 一致。
名称或局部摘要不能替代完整档案。此 POST 只读，不添加操作 key 或 --confirm。
暂停／已启用档案由用户明确复用；已撤销项仅供查看历史。
本命令不创建、启用、连接或执行工具。
`
